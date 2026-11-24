package headscale

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestPolicyChangedDuringCheckCannotOverwrite(t *testing.T) {
	current := `{"grants":[]}`
	originalRevision := revision(current)
	writes := 0
	c := fixture(t, "0.29.5", "database", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "GET":
			_ = json.NewEncoder(w).Encode(map[string]string{"policy": current})
		case "POST":
			current = `{"grants":[{"src":["*"],"dst":["*"],"ip":["*"]}]}`
			_, _ = io.WriteString(w, `{}`)
		case "PUT":
			writes++
			_, _ = io.WriteString(w, `{}`)
		}
	})
	payload, _ := json.Marshal(map[string]string{"policy": `{"grants":[]}`, "revision": originalRevision})
	assertStatus(t, call(c, "PUT", "/api/policy", string(payload)), 409)
	if writes != 0 {
		t.Fatal("overwrote an externally modified policy")
	}
}
func TestShortKeyIsNeverDisclosedByList(t *testing.T) {
	c := fixture(t, "0.29.5", "file", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"preAuthKeys":[{"id":"2","key":"ABCD"}]}`)
	})
	response := call(c, "GET", "/api/keys", "")
	assertStatus(t, response, 200)
	if strings.Contains(response.Body.String(), "ABCD") {
		t.Fatal("abnormal short key fully disclosed")
	}
}
