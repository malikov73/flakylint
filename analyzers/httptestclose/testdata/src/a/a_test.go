package a

import (
	"net/http/httptest"
	"testing"
)

func use(*httptest.Server) {}

func TestLeak(t *testing.T) {
	srv := httptest.NewServer(nil) // want `httptest server is never closed`
	_ = srv.URL
}

func TestTLSLeak(t *testing.T) {
	srv := httptest.NewTLSServer(nil) // want `httptest server is never closed`
	_ = srv.URL
}

func TestUnstartedLeak(t *testing.T) {
	srv := httptest.NewUnstartedServer(nil) // want `httptest server is never closed`
	srv.Start()
}

func TestCleanup(t *testing.T) {
	srv := httptest.NewServer(nil)
	t.Cleanup(srv.Close)
	_ = srv.URL
}

func TestDefer(t *testing.T) {
	srv := httptest.NewServer(nil)
	defer srv.Close()
	_ = srv.URL
}

func TestDirectClose(t *testing.T) {
	srv := httptest.NewServer(nil)
	srv.Close()
}

func TestEscapeArg(t *testing.T) {
	srv := httptest.NewServer(nil) // escapes as argument: silent
	use(srv)
}

func newServer(t *testing.T) *httptest.Server {
	srv := httptest.NewServer(nil) // escapes via return: silent
	return srv
}

func TestEscapeAssign(t *testing.T) {
	var keep *httptest.Server
	srv := httptest.NewServer(nil) // escapes via reassignment: silent
	keep = srv
	_ = keep
}

func TestSubtestLeak(t *testing.T) {
	t.Run("sub", func(t *testing.T) {
		srv := httptest.NewServer(nil) // want `httptest server is never closed`
		_ = srv.URL
	})
}

func TestInitializer(t *testing.T) {
	// Constructor in an if-initializer: still diagnosed, but no fix is offered
	// because a cleanup statement cannot be spliced into the clause header.
	if srv := httptest.NewServer(nil); srv.URL != "" { // want `httptest server is never closed`
		_ = srv.URL
	}
}
