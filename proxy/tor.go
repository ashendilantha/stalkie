package proxy

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"
)

type TorRenewer struct {
	controlAddr string
	password    string
	mu          sync.Mutex
	lastRenew   time.Time
}

func NewTorRenewer(controlAddr, password string) *TorRenewer {
	addr := strings.TrimSpace(controlAddr)
	if addr == "" {
		addr = "127.0.0.1:9051"
	}
	return &TorRenewer{
		controlAddr: addr,
		password:    password,
	}
}

func (tr *TorRenewer) RenewCircuit() error {
	if tr == nil {
		return nil
	}
	tr.mu.Lock()
	defer tr.mu.Unlock()

	if time.Since(tr.lastRenew) < 5*time.Second {
		return nil
	}

	conn, err := net.DialTimeout("tcp", tr.controlAddr, 4*time.Second)
	if err != nil {
		return fmt.Errorf("tor control connect failed: %w", err)
	}
	defer conn.Close()

	reader := bufio.NewReader(conn)

	authMsg := "AUTHENTICATE\r\n"
	if tr.password != "" {
		authMsg = fmt.Sprintf("AUTHENTICATE \"%s\"\r\n", tr.password)
	}
	if _, err := conn.Write([]byte(authMsg)); err != nil {
		return fmt.Errorf("tor authenticate write failed: %w", err)
	}

	resp, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("tor auth response read failed: %w", err)
	}
	if !strings.HasPrefix(resp, "250") {
		return fmt.Errorf("tor auth rejected: %s", strings.TrimSpace(resp))
	}

	if _, err := conn.Write([]byte("SIGNAL NEWNYM\r\n")); err != nil {
		return fmt.Errorf("tor signal newnym write failed: %w", err)
	}

	resp, err = reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("tor signal response read failed: %w", err)
	}
	if !strings.HasPrefix(resp, "250") {
		return fmt.Errorf("tor signal newnym rejected: %s", strings.TrimSpace(resp))
	}

	tr.lastRenew = time.Now()
	return nil
}
