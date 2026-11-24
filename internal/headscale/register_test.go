package headscale

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestAuthIDRegisterUsesActualUsernameAndStripsNodeSecrets(t *testing.T) {
	calls := 0
	c := fixture(t, "0.29.7", "database", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/user":
			w.Write([]byte(`{"users":[{"id":"7","name":"alice"}]}`))
		case "/api/v1/auth/register":
			calls++
			if r.URL.RawQuery != "" {
				t.Error("Auth ID in query")
			}
			var b map[string]string
			json.NewDecoder(r.Body).Decode(&b)
			if b["user"] != "alice" || b["authId"] != "hskey-authreq-ABCDEFGH1234" {
				t.Error(b)
			}
			w.Write([]byte(`{"node":{"id":"42","givenName":"registered-node","user":{"id":"7","name":"alice"},"preAuthKey":{"key":"SENSITIVE_NODE_AUTH_KEY"},"machineKey":"SENSITIVE_MACHINE_KEY"}}`))
		default:
			w.WriteHeader(404)
		}
	})
	r := call(c, "POST", "/api/devices/register", `{"authId":"hskey-authreq-ABCDEFGH1234","userId":"7"}`)
	assertStatus(t, r, 200)
	if calls != 1 || strings.Contains(r.Body.String(), "SENSITIVE") || !strings.Contains(r.Body.String(), "registered-node") {
		t.Fatal(r.Body.String())
	}
}
func TestRegistrationValidationAndVersionGate(t *testing.T) {
	for _, body := range []string{`{"authId":"hskey-api-ABCDEFGH1234","userId":"7"}`, `{"authId":"hskey-authreq-","userId":"7"}`, `{"authId":"hskey-authreq-ABCDEFGH1234","userId":"../7"}`} {
		c := fixture(t, "0.29.7", "database", func(w http.ResponseWriter, r *http.Request) { t.Fatal("invalid input reached upstream") })
		assertStatus(t, call(c, "POST", "/api/devices/register", body), 400)
	}
	c := fixture(t, "0.28.0", "database", func(w http.ResponseWriter, r *http.Request) { t.Fatal("unsupported registration sent") })
	assertStatus(t, call(c, "POST", "/api/devices/register", `{"authId":"hskey-authreq-ABCDEFGH1234","userId":"7"}`), 422)
}
func TestRegistrationFailuresRemainSafeAndUnknownUserRejected(t *testing.T) {
	c := fixture(t, "0.29.7", "database", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/user" {
			w.Write([]byte(`{"users":[{"id":"7","name":"alice"}]}`))
			return
		}
		w.WriteHeader(500)
		w.Write([]byte(`{"message":"SECRET INTERNAL DETAILS"}`))
	})
	r := call(c, "POST", "/api/devices/register", `{"authId":"hskey-authreq-ABCDEFGH1234","userId":"7"}`)
	assertStatus(t, r, 502)
	if !strings.Contains(r.Body.String(), "registration_failed") || strings.Contains(r.Body.String(), "SECRET") {
		t.Fatal(r.Body.String())
	}
	assertStatus(t, call(c, "POST", "/api/devices/register", `{"authId":"hskey-authreq-ABCDEFGH1234","userId":"8"}`), 400)
}
