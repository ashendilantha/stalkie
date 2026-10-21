package proxy

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/proxy"
)

type Config struct {
	Type    string
	Address string
}

func BuildClient(cfg Config, timeout time.Duration) (*http.Client, error) {
	transport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
	}

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
			transport.DialContext = contextDialer.DialContext
		} else {
			transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
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
		transport.Proxy = http.ProxyURL(parsedURL)

	case "none", "":

	default:
		return nil, fmt.Errorf("unsupported proxy type: %s", cfg.Type)
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
