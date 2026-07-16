package fix

import (
	"net/http/httptest"
	"testing"
)

func TestFix(t *testing.T) {
	srv := httptest.NewServer(nil) // want `httptest server is never closed`
	_ = srv.URL
}
