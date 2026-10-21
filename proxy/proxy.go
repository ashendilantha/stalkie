package proxy

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	utls "github.com/refraction-networking/utls"
	"golang.org/x/net/http2"
	"golang.org/x/net/proxy"
)

type Config struct {
	Type        string
	Address     string
	DisableUTLS bool
}

type UTLSTransport struct {
	rawDialer  func(ctx context.Context, network, addr string) (net.Conn, error)
	h1         *http.Transport
	h2         *http2.Transport
	mu         sync.Mutex
	h2Conns    map[string]*http2.ClientConn
	helloIndex uint64
}

var clientHelloIDs = []utls.ClientHelloID{
	utls.HelloChrome_120,
	utls.HelloChrome_106_Shuffle,
	utls.HelloFirefox_120,
}

func (t *UTLSTransport) nextClientHello() utls.ClientHelloID {
	idx := atomic.AddUint64(&t.helloIndex, 1)
	return clientHelloIDs[idx%uint64(len(clientHelloIDs))]
}

func NewUTLSTransport(rawDialer func(ctx context.Context, network, addr string) (net.Conn, error)) *UTLSTransport {
	if rawDialer == nil {
		stdDialer := &net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}
		rawDialer = stdDialer.DialContext
	}
	return &UTLSTransport{
		rawDialer: rawDialer,
		h1: &http.Transport{
			ForceAttemptHTTP2:   false,
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
		},
		h2: &http2.Transport{
			AllowHTTP: false,
		},
		h2Conns: make(map[string]*http2.ClientConn),
	}
}

func (t *UTLSTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Scheme == "http" {
		return t.h1.RoundTrip(req)
	}

	hostname := req.URL.Hostname()
	port := req.URL.Port()
	if port == "" {
		port = "443"
	}
	targetAddr := net.JoinHostPort(hostname, port)

	t.mu.Lock()
	if cc, ok := t.h2Conns[targetAddr]; ok && cc.CanTakeNewRequest() {
		t.mu.Unlock()
		return cc.RoundTrip(req)
	}
	t.mu.Unlock()

	ctx := req.Context()
	rawConn, err := t.rawDialer(ctx, "tcp", targetAddr)
	if err != nil {
		return nil, err
	}

	if deadline, ok := ctx.Deadline(); ok {
		rawConn.SetDeadline(deadline)
	}

	uConfig := &utls.Config{
		ServerName: hostname,
	}
	uConn := utls.UClient(rawConn, uConfig, t.nextClientHello())
	if err := uConn.HandshakeContext(ctx); err != nil {
		rawConn.Close()
		return nil, err
	}

	state := uConn.ConnectionState()
	if state.NegotiatedProtocol == "h2" {
		cc, err := t.h2.NewClientConn(uConn)
		if err != nil {
			uConn.Close()
			return nil, err
		}
		t.mu.Lock()
		t.h2Conns[targetAddr] = cc
		t.mu.Unlock()
		return cc.RoundTrip(req)
	}

	done := make(chan struct{})
	defer close(done)

	go func() {
		select {
		case <-ctx.Done():
			uConn.Close()
		case <-done:
		}
	}()

	if deadline, ok := ctx.Deadline(); ok {
		uConn.SetDeadline(deadline)
	}

	if err := req.Write(uConn); err != nil {
		uConn.Close()
		return nil, err
	}

	resp, err := http.ReadResponse(bufio.NewReader(uConn), req)
	if err != nil {
		uConn.Close()
		return nil, err
	}

	return resp, nil
}

func (t *UTLSTransport) CloseIdleConnections() {
	t.mu.Lock()
	for _, cc := range t.h2Conns {
		cc.Close()
	}
	t.h2Conns = make(map[string]*http2.ClientConn)
	t.mu.Unlock()
	t.h1.CloseIdleConnections()
	t.h2.CloseIdleConnections()
}

func BuildClient(cfg Config, timeout time.Duration) (*http.Client, error) {
	var rawDialer func(ctx context.Context, network, addr string) (net.Conn, error)
	normalizedType := strings.ToLower(strings.TrimSpace(cfg.Type))

	switch normalizedType {
	case "tor", "socks", "socks5":
		addr := cfg.Address
		if addr == "" {
			addr = "127.0.0.1:9050"
		}
		addr = strings.TrimPrefix(addr, "socks5://")
		dialer, err := proxy.SOCKS5("tcp", addr, nil, proxy.Direct)
		if err != nil {
			return nil, fmt.Errorf("socks5 initialization error: %w", err)
		}
		if contextDialer, ok := dialer.(proxy.ContextDialer); ok {
			rawDialer = contextDialer.DialContext
		} else {
			rawDialer = func(ctx context.Context, network, address string) (net.Conn, error) {
				return dialer.Dial(network, address)
			}
		}

	case "http", "https":
		if cfg.Address == "" {
			return nil, fmt.Errorf("proxy address required for type %s", cfg.Type)
		}
		raw := cfg.Address
		if !strings.HasPrefix(raw, "http://") && !strings.HasPrefix(raw, "https://") {
			raw = "http://" + raw
		}
		parsedURL, err := url.Parse(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid proxy url: %w", err)
		}
		rawDialer = func(ctx context.Context, network, addr string) (net.Conn, error) {
			proxyConn, err := (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, "tcp", parsedURL.Host)
			if err != nil {
				return nil, err
			}
			connectReq := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", addr, addr)
			if _, err := proxyConn.Write([]byte(connectReq)); err != nil {
				proxyConn.Close()
				return nil, err
			}
			resp, err := http.ReadResponse(bufio.NewReader(proxyConn), nil)
			if err != nil {
				proxyConn.Close()
				return nil, err
			}
			resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				proxyConn.Close()
				return nil, fmt.Errorf("proxy connect failed with status: %d", resp.StatusCode)
			}
			return proxyConn, nil
		}

	case "none", "":
		stdDialer := &net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}
		rawDialer = stdDialer.DialContext

	default:
		return nil, fmt.Errorf("unsupported proxy type: %s", cfg.Type)
	}

	var transport http.RoundTripper
	if cfg.DisableUTLS {
		stdTransport := &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
			DialContext:         rawDialer,
		}
		transport = stdTransport
	} else {
		transport = NewUTLSTransport(rawDialer)
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}

	return client, nil
}
