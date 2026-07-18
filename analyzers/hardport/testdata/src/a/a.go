package a

// This non-test file repeats the flagged patterns to lock the InTestFile
// guard: none of these must be reported because the file is not a _test.go.

import (
	"net"
	"net/http"
)

func listenProd() {
	ln, _ := net.Listen("tcp", ":8080")
	_ = ln
	pc, _ := net.ListenPacket("udp", "127.0.0.1:4242")
	_ = pc
	go func() {
		_ = http.ListenAndServe(":8081", nil)
	}()
	srv := &http.Server{Addr: "127.0.0.1:8443"}
	_ = srv
}
