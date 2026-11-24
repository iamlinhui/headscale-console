package configstore

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUnconfiguredDNSHasSpecificError(t *testing.T) {
	s, e := New(Options{})
	if e != nil {
		t.Fatal(e)
	}
	w := httptest.NewRecorder()
	s.ServeHTTP(w, httptest.NewRequest("GET", "/api/config/dns", nil), "admin")
	if w.Code != 503 || !strings.Contains(w.Body.String(), "configuration_not_configured") {
		t.Fatal(w.Body.String())
	}
}
