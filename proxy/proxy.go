package proxy

import (
	"fmt"
	"net/http"
	"net/url"
	"time"

	"golang.org/x/net/proxy"
)

type Config struct {
	Type    string //tor, http, socks
	Address string
}

// builds a proxy client based on the config
func BuildClient(cfg Config, timeout time.Duration) (*http.Client, error) {
	transport := &http.Transport{}

	switch cfg.Type {
	case "tor", "socks":
		addr := cfg.Address
		if addr == "" {
			addr = "127.0.0.1:9050"
		}

		dialer, err := proxy.SOCKS5("tcp", addr, nil, proxy.Direct)
		if err != nil {
			return nil, fmt.Errorf("SOCKS5 dialer error: %w", err)
		}
		transport.Dial = dialer.Dial

	case "http":
		proxyURL, err := url.Parse(cfg.Address)

		if err != nil {
			return nil, fmt.Errorf("Invalid proxy url: %w", err)
		}
		transport.Proxy = http.ProxyURL(proxyURL)

	case "none", "":
		//direct connection and no proxy
	default:
		return nil, fmt.Errorf("Unsupported proxy type: %s", cfg.Type)
	}

	return &http.Client{
		Transport: transport,
		Timeout:   timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}, nil

}
