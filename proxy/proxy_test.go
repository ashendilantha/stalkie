package proxy

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestBuildClient(t *testing.T) {
	clientDefault, err := BuildClient(Config{Type: "none"}, 5*time.Second)
	if err != nil {
		t.Fatalf("BuildClient failed for default config: %v", err)
	}
	if clientDefault.Timeout != 5*time.Second {
		t.Errorf("expected timeout 5s, got %v", clientDefault.Timeout)
	}
	if _, ok := clientDefault.Transport.(*UTLSTransport); !ok {
		t.Errorf("expected UTLSTransport by default, got %T", clientDefault.Transport)
	}

	clientNoUTLS, err := BuildClient(Config{Type: "none", DisableUTLS: true}, 5*time.Second)
	if err != nil {
		t.Fatalf("BuildClient failed for disabled uTLS: %v", err)
	}
	if _, ok := clientNoUTLS.Transport.(*http.Transport); !ok {
		t.Errorf("expected standard http.Transport when DisableUTLS is true, got %T", clientNoUTLS.Transport)
	}

	_, err = BuildClient(Config{Type: "invalid_type"}, 5*time.Second)
	if err == nil {
		t.Errorf("expected error for invalid proxy type, got nil")
	}
}

func TestUTLSTransportHTTP(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
	defer server.Close()

	tr := NewUTLSTransport(nil)
	defer tr.CloseIdleConnections()

	client := &http.Client{Transport: tr, Timeout: 5 * time.Second}
	req, err := http.NewRequest(http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatalf("failed creating request: %v", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("HTTP request through UTLSTransport failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
}

func TestNextClientHello(t *testing.T) {
	tr := NewUTLSTransport(func(ctx context.Context, network, addr string) (net.Conn, error) {
		return nil, nil
	})
	h1 := tr.nextClientHello()
	h2 := tr.nextClientHello()
	if h1.Client == "" || h2.Client == "" {
		t.Errorf("expected valid ClientHello IDs, got empty")
	}
}

