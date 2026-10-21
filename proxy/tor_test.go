package proxy

import (
	"bufio"
	"net"
	"strings"
	"testing"
)

func TestTorRenewer(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start mock tor control listener: %v", err)
	}
	defer listener.Close()

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				reader := bufio.NewReader(c)
				for {
					line, err := reader.ReadString('\n')
					if err != nil {
						return
					}
					line = strings.TrimSpace(line)
					if strings.HasPrefix(line, "AUTHENTICATE") {
						c.Write([]byte("250 OK\r\n"))
					} else if line == "SIGNAL NEWNYM" {
						c.Write([]byte("250 OK\r\n"))
					} else {
						c.Write([]byte("510 Bad command\r\n"))
					}
				}
			}(conn)
		}
	}()

	renewer := NewTorRenewer(listener.Addr().String(), "secret")
	if err := renewer.RenewCircuit(); err != nil {
		t.Fatalf("RenewCircuit failed: %v", err)
	}

	if err := renewer.RenewCircuit(); err != nil {
		t.Fatalf("subsequent RenewCircuit should not fail (debounced): %v", err)
	}
}

