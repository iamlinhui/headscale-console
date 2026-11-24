package headscale

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestDatabasePolicyCanBeInitializedOnlyForExactMissingRecord(t *testing.T) {
	policy := ""
	writes := 0
	c := fixture(t, "0.29.7", "database", func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/policy" && r.Method == "GET":
			if policy == "" {
				w.WriteHeader(500)
				w.Write([]byte(`{"code":2,"message":"loading ACL from database: acl policy not found"}`))
				return
			}
			json.NewEncoder(w).Encode(map[string]string{"policy": policy})
		case r.URL.Path == "/api/v1/policy/check":
			w.Write([]byte(`{}`))
		case r.URL.Path == "/api/v1/policy" && r.Method == "PUT":
			var b struct {
				Policy string `json:"policy"`
			}
			json.NewDecoder(r.Body).Decode(&b)
			policy = b.Policy
			writes++
			json.NewEncoder(w).Encode(map[string]string{"policy": policy})
		default:
			w.WriteHeader(404)
		}
	})
	r := call(c, "GET", "/api/policy", "")
	assertStatus(t, r, 200)
	if !strings.Contains(r.Body.String(), `"policy":""`) {
		t.Fatal(r.Body.String())
	}
	body, _ := json.Marshal(map[string]string{"policy": `{"grants":[]}`, "revision": revision("")})
	r = call(c, "PUT", "/api/policy", string(body))
	assertStatus(t, r, 200)
	if writes != 1 {
		t.Fatal("initial publish failed")
	}
}
func TestDatabasePolicyDoesNotConvertOtherErrorsToEmpty(t *testing.T) {
	for _, response := range []string{`{"code":2,"message":"database connection failed"}`, `{"code":13,"message":"permission denied"}`, `not json`} {
		c := fixture(t, "0.29.7", "database", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500); w.Write([]byte(response)) })
		r := call(c, "GET", "/api/policy", "")
		assertStatus(t, r, 502)
	}
}
