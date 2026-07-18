package a

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

const addr = "localhost:7070"

func TestListenHardcoded(t *testing.T) {
	ln, _ := net.Listen("tcp", ":8080") // want `test binds hardcoded address`
	_ = ln
	ln2, _ := net.Listen("tcp", "localhost:9090") // want `test binds hardcoded address`
	_ = ln2
	pc, _ := net.ListenPacket("udp", "127.0.0.1:4242") // want `test binds hardcoded address`
	_ = pc
	ln3, _ := net.Listen("tcp", addr) // want `test binds hardcoded address`
	_ = ln3
	ln4, _ := net.Listen("tcp6", "[::1]:8080") // want `test binds hardcoded address`
	_ = ln4
	pc2, _ := net.ListenPacket("udp4", "127.0.0.1:5353") // want `test binds hardcoded address`
	_ = pc2
}

func TestListenAndServe(t *testing.T) {
	go func() {
		_ = http.ListenAndServe(":8081", nil) // want `test binds hardcoded address`
	}()
	go func() {
		_ = http.ListenAndServeTLS(":8444", "cert.pem", "key.pem", nil) // want `test binds hardcoded address`
	}()
}

func TestSilent(t *testing.T) {
	ln, _ := net.Listen("tcp", ":0")
	_ = ln
	ln2, _ := net.Listen("tcp", "127.0.0.1:0")
	_ = ln2
	ln3, _ := net.Listen("unix", "/tmp/x.sock")
	_ = ln3
	computed := fmt.Sprintf(":%d", 8080)
	ln4, _ := net.Listen("tcp", computed)
	_ = ln4
	ln5, _ := net.Listen("tcp", ":http")
	_ = ln5
	conn, _ := net.Dial("tcp", "localhost:8080")
	_ = conn
	srv := httptest.NewServer(nil)
	srv.Close()

	// unix domain sockets bind a path, not a port: not the flake we target.
	ln6, _ := net.Listen("unix", "fixture:8080")
	_ = ln6
	// a non-constant network cannot be validated against tcp/udp — stay silent.
	network := "tcp"
	ln7, _ := net.Listen(network, ":8080")
	_ = ln7
	// port outside 1..65535 is not a real bind port.
	ln8, _ := net.Listen("tcp", ":70000")
	_ = ln8
	// http.Server{Addr:} is a config object, not a bind call — no longer flagged.
	srv2 := &http.Server{Addr: "127.0.0.1:8443"}
	_ = srv2
	srv3 := http.Server{Addr: "localhost:6060"}
	_ = srv3
}
