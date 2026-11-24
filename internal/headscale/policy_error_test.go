package headscale

import (
	"net/http"
	"strings"
	"testing"
)

func TestFilePolicyFailureHasSpecificError(t *testing.T) {
	c := fixture(t, "0.29.5", "file", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) })
	r := call(c, "GET", "/api/policy", "")
	assertStatus(t, r, 503)
	if !strings.Contains(r.Body.String(), "policy_file_unavailable") {
		t.Fatal(r.Body.String())
	}
}
func TestFilePolicyUnauthorizedDoesNotMasqueradeAsMissingFile(t *testing.T) {
	c := fixture(t, "0.29.5", "file", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(401) })
	r := call(c, "GET", "/api/policy", "")
	assertStatus(t, r, 502)
	if !strings.Contains(r.Body.String(), "headscale_unauthorized") {
		t.Fatal(r.Body.String())
	}
}
