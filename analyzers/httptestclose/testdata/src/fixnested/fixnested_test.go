package fixnested

import (
	"net/http/httptest"
	"testing"
)

func TestNested(t *testing.T) {
	t.Run("outer", func(t *testing.T) {
		t.Run("inner", func(t *testing.T) {
			srv := httptest.NewServer(nil) // want `httptest server is never closed`
			_ = srv.URL
		})
	})
}
