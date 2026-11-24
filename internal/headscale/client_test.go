package headscale

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

const testSecret = "MANAGEMENT_SECRET_DO_NOT_RETURN"

func fixture(t *testing.T, version, mode string, fn http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+testSecret {
			t.Error("missing server management auth")
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/version" {
			_ = json.NewEncoder(w).Encode(map[string]string{"version": version})
			return
		}
		if r.URL.Path == "/api/v1/health" {
			io.WriteString(w, `{"databaseConnectivity":true}`)
			return
		}
		fn(w, r)
	}))
	t.Cleanup(srv.Close)
	c, e := New(srv.URL, testSecret, "https://headscale.example.com", mode)
	if e != nil {
		t.Fatal(e)
	}
	return c
}
func call(c *Client, method, path, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		r.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	c.Handler().ServeHTTP(w, r)
	return w
}
func assertStatus(t *testing.T, r *httptest.ResponseRecorder, want int) {
	t.Helper()
	if r.Code != want {
		t.Fatalf("status %d want %d: %s", r.Code, want, r.Body.String())
	}
	if strings.Contains(r.Body.String(), testSecret) {
		t.Fatal("secret leaked")
	}
}

const nodeJSON = `{"id":"18446744073709551615","name":"host","givenName":"gateway","ipAddresses":["fd7a::1","100.64.0.1"],"user":{"id":"9007199254740993","name":"infra","displayName":"Infra","email":"infra@example.com","providerId":"SECRET_PROVIDER"},"online":true,"tags":["tag:prod"],"availableRoutes":["10.0.0.0/24"],"approvedRoutes":["10.0.0.0/24"],"subnetRoutes":[],"preAuthKey":{"key":"NESTED_SECRET"},"machineKey":"MACHINE_SECRET","nodeKey":"NODE_SECRET","expiry":"0001-01-01T00:00:00Z"}`

func TestDevicesNormalizeAndStripSecrets(t *testing.T) {
	c := fixture(t, "0.29.5", "database", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/node" {
			t.Fatal(r.URL.Path)
		}
		io.WriteString(w, `{"nodes":[`+nodeJSON+`]}`)
	})
	r := call(c, "GET", "/api/devices", "")
	assertStatus(t, r, 200)
	for _, s := range []string{"NESTED_SECRET", "MACHINE_SECRET", "NODE_SECRET", "SECRET_PROVIDER", "preAuthKey"} {
		if strings.Contains(r.Body.String(), s) {
			t.Fatal("deep secret leaked", s)
		}
	}
	var out struct{ Devices []map[string]any }
	json.Unmarshal(r.Body.Bytes(), &out)
	d := out.Devices[0]
	if d["id"] != "18446744073709551615" || d["userId"] != "9007199254740993" || d["os"] != "未采集" || d["platform"] != "Unknown" || d["version"] != "未采集" || d["ip"] != "100.64.0.1" || d["name"] != "gateway" || d["expiry"] != nil {
		t.Fatal(d)
	}
}
func TestConnectionUsesActualVersionAndHealth(t *testing.T) {
	for _, v := range []string{"0.28.0", "v0.29.5"} {
		c := fixture(t, v, "database", func(w http.ResponseWriter, r *http.Request) { t.Fatal(r.URL.Path) })
		r := call(c, "GET", "/api/connection", "")
		assertStatus(t, r, 200)
		var out struct {
			Version      string
			Connected    bool
			Capabilities struct{ Read, Write, PolicyCheck, PolicyWrite, Keys bool }
		}
		json.Unmarshal(r.Body.Bytes(), &out)
		if out.Version != v || !out.Connected || !out.Capabilities.Read || !out.Capabilities.Keys || out.Capabilities.PolicyWrite != (v == "v0.29.5") {
			t.Fatal(r.Body.String())
		}
	}
}
func TestUnsupportedVersionFailsClosed(t *testing.T) {
	for _, v := range []string{"0.30.0", "latest", "0.29.5-dev", "0.29.5+dirty", "garbage"} {
		c := fixture(t, v, "database", func(w http.ResponseWriter, r *http.Request) { t.Fatal("should never request business endpoint") })
		for _, path := range []string{"/api/devices", "/api/users", "/api/policy"} {
			assertStatus(t, call(c, "POST", path, `{}`), 422)
		}
	}
}
func TestTransportBlocksRedirectAndEnvironmentProxy(t *testing.T) {
	var hits atomic.Int32
	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1) }))
	defer sink.Close()
	c := fixture(t, "0.29.5", "file", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, sink.URL, 302) })
	t.Setenv("HTTP_PROXY", sink.URL)
	r := call(c, "GET", "/api/devices", "")
	assertStatus(t, r, 502)
	if hits.Load() != 0 {
		t.Fatal("followed redirect or inherited proxy")
	}
	if c.http.Transport.(*http.Transport).Proxy != nil {
		t.Fatal("environment proxy enabled")
	}
}
func TestUpstreamErrorsAreSanitizedAndNeverLogout(t *testing.T) {
	for _, status := range []int{401, 403, 500, 400, 409} {
		c := fixture(t, "0.29.5", "database", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
			io.WriteString(w, testSecret+" traceback internal secret")
		})
		r := call(c, "GET", "/api/devices", "")
		if r.Code == 401 {
			t.Fatal("upstream auth must not log console out")
		}
		if strings.Contains(r.Body.String(), "traceback") {
			t.Fatal("raw upstream error leaked")
		}
		if status == 401 || status == 403 {
			assertStatus(t, r, 502)
			if !strings.Contains(r.Body.String(), "headscale_unauthorized") {
				t.Fatal(r.Body.String())
			}
		}
	}
}
func TestNodeWriteMappingsAndValidation(t *testing.T) {
	paths := []string{}
	c := fixture(t, "0.28.0", "file", func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.Path)
		if strings.HasSuffix(r.URL.Path, "/tags") {
			var b struct{ Tags []string }
			json.NewDecoder(r.Body).Decode(&b)
			if len(b.Tags) != 1 || b.Tags[0] != "tag:prod" {
				t.Fatal(b)
			}
		}
		if r.Method == "DELETE" {
			io.WriteString(w, `{}`)
		} else {
			io.WriteString(w, `{"node":`+nodeJSON+`}`)
		}
	})
	for _, x := range []struct{ path, body string }{{"rename", `{"name":"gateway-2"}`}, {"tags", `{"tags":["prod"]}`}, {"expire", `{}`}} {
		assertStatus(t, call(c, "POST", "/api/devices/7/"+x.path, x.body), 200)
	}
	assertStatus(t, call(c, "DELETE", "/api/devices/7", ""), 200)
	want := []string{"POST /api/v1/node/7/rename/gateway-2", "POST /api/v1/node/7/tags", "POST /api/v1/node/7/expire", "DELETE /api/v1/node/7"}
	for i, s := range want {
		if paths[i] != s {
			t.Fatal(paths)
		}
	}
	for _, x := range []struct{ path, body string }{{"/api/devices/0/tags", `{"tags":["prod"]}`}, {"/api/devices/7/tags", `{"tags":["bad/tag"]}`}, {"/api/devices/7/tags", `{"tags":["prod","tag:prod"]}`}, {"/api/devices/7/rename", `{"name":"../foo"}`}, {"/api/devices/7/expire", `{"unknown":true}`}} {
		r := call(c, "POST", x.path, x.body)
		if r.Code == 200 {
			t.Fatal("accepted invalid", x)
		}
	}
}
func TestRoutesCompareCompleteSet(t *testing.T) {
	var writes int
	c := fixture(t, "0.29.5", "file", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/node/7":
			io.WriteString(w, `{"node":`+nodeJSON+`}`)
		case "/api/v1/node/7/approve_routes":
			writes++
			var b struct{ Routes []string }
			json.NewDecoder(r.Body).Decode(&b)
			if b.Routes == nil || len(b.Routes) != 0 {
				t.Fatal(b)
			}
			io.WriteString(w, `{"node":`+nodeJSON+`}`)
		case "/api/v1/node":
			io.WriteString(w, `{"nodes":[`+nodeJSON+`]}`)
		default:
			t.Fatal(r.URL.Path)
		}
	})
	assertStatus(t, call(c, "POST", "/api/routes/7", `{"routes":[],"expectedRoutes":[]}`), 409)
	assertStatus(t, call(c, "POST", "/api/routes/7", `{"routes":["10.0.0.1/24"],"expectedRoutes":["10.0.0.0/24"]}`), 400)
	assertStatus(t, call(c, "POST", "/api/routes/7", `{"routes":["192.168.0.0/24"],"expectedRoutes":["10.0.0.0/24"]}`), 400)
	assertStatus(t, call(c, "POST", "/api/routes/7", `{"routes":[],"expectedRoutes":["10.0.0.0/24"]}`), 200)
	if writes != 1 {
		t.Fatal(writes)
	}
	r := call(c, "GET", "/api/routes", "")
	assertStatus(t, r, 200)
	if !strings.Contains(r.Body.String(), `"serving":false`) {
		t.Fatal(r.Body.String())
	}
}
func TestPreauthKeysVersionFieldsAndMasking(t *testing.T) {
	for _, v := range []string{"0.28.0", "0.29.5"} {
		var made bool
		c := fixture(t, v, "file", func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.URL.Path == "/api/v1/preauthkey" && r.Method == "GET":
				io.WriteString(w, `{"preAuthKeys":[{"id":"44","key":"SECRET_KEY_ABCD","user":{"id":"7","name":"infra"},"reusable":true,"used":true,"expiration":"2099-01-01T00:00:00Z"}]}`)
			case r.URL.Path == "/api/v1/preauthkey" && r.Method == "POST":
				made = true
				var b map[string]any
				json.NewDecoder(r.Body).Decode(&b)
				if b["user"] != "9007199254740993" || b["ephemeral"] != false || b["reusable"] != true || b["aclTags"] == nil || b["expiration"] == nil || b["name"] != nil {
					t.Fatal(b)
				}
				io.WriteString(w, `{"preAuthKey":{"id":"45","key":"ONE_TIME_SECRET","expiration":"2099-01-01T00:00:00Z"}}`)
			case r.URL.Path == "/api/v1/preauthkey/expire":
				var b map[string]string
				json.NewDecoder(r.Body).Decode(&b)
				if b["id"] != "44" {
					t.Fatal(b)
				}
				io.WriteString(w, `{}`)
			default:
				t.Fatal(r.URL.Path)
			}
		})
		r := call(c, "GET", "/api/keys", "")
		assertStatus(t, r, 200)
		if strings.Contains(r.Body.String(), "SECRET_KEY") || !strings.Contains(r.Body.String(), "•••• ABCD") || !strings.Contains(r.Body.String(), "有效") {
			t.Fatal(r.Body.String())
		}
		r = call(c, "POST", "/api/keys", `{"userId":"9007199254740993","name":"接入","ttlDays":7,"reusable":true}`)
		assertStatus(t, r, 200)
		if !made || !strings.Contains(r.Body.String(), "ONE_TIME_SECRET") || !strings.Contains(r.Body.String(), "https://headscale.example.com") || strings.Contains(r.Body.String(), c.base.Host) {
			t.Fatal(r.Body.String())
		}
		assertStatus(t, call(c, "POST", "/api/keys/44/expire", `{}`), 200)
	}
}
func TestPolicyServerCheckAndRevision(t *testing.T) {
	policy := "{/* HuJSON comments allowed */\"grants\":[]}"
	calls := []string{}
	c := fixture(t, "0.29.5", "database", func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		if r.Method == "GET" {
			json.NewEncoder(w).Encode(map[string]string{"policy": policy})
			return
		}
		var b struct{ Policy string }
		json.NewDecoder(r.Body).Decode(&b)
		if b.Policy == "bad" {
			w.WriteHeader(400)
			io.WriteString(w, "PRIVATE DETAILS")
			return
		}
		if r.Method == "PUT" {
			policy = b.Policy
			json.NewEncoder(w).Encode(map[string]string{"policy": policy})
		} else {
			io.WriteString(w, `{}`)
		}
	})
	r := call(c, "GET", "/api/policy", "")
	assertStatus(t, r, 200)
	assertStatus(t, call(c, "POST", "/api/policy/check", `{"policy":"bad"}`), 400)
	payload, _ := json.Marshal(map[string]string{"policy": "{\"grants\":[]}", "revision": strings.Repeat("0", 64)})
	assertStatus(t, call(c, "PUT", "/api/policy", string(payload)), 409)
	payload, _ = json.Marshal(map[string]string{"policy": "{\"grants\":[]}", "revision": revision(policy)})
	assertStatus(t, call(c, "PUT", "/api/policy", string(payload)), 200)
	n := len(calls)
	if calls[n-4] != "GET /api/v1/policy" || calls[n-3] != "POST /api/v1/policy/check" || calls[n-2] != "GET /api/v1/policy" || calls[n-1] != "PUT /api/v1/policy" {
		t.Fatal(calls)
	}
}
func TestPolicyModeAndVersionGates(t *testing.T) {
	for _, x := range []struct{ v, mode string }{{"0.28.0", "database"}, {"0.29.5", "file"}, {"0.29.5", "unknown"}} {
		c := fixture(t, x.v, x.mode, func(w http.ResponseWriter, r *http.Request) { t.Fatal("must not write") })
		assertStatus(t, call(c, "PUT", "/api/policy", `{"policy":"{}","revision":"`+strings.Repeat("0", 64)+`"}`), 409)
	}
}
func TestCreateUserAndStrictPayload(t *testing.T) {
	c := fixture(t, "0.29.5", "file", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/user" || r.Method != "POST" {
			t.Fatal(r.URL.Path)
		}
		var b map[string]string
		json.NewDecoder(r.Body).Decode(&b)
		if b["name"] != "anna" || b["email"] != "anna@example.com" || len(b) != 2 {
			t.Fatal(b)
		}
		io.WriteString(w, `{"user":{"id":"99","name":"anna","email":"anna@example.com"}}`)
	})
	assertStatus(t, call(c, "POST", "/api/users", `{"name":"anna","email":"anna@example.com"}`), 200)
	for _, b := range []string{`{"name":"../x","email":"x@example.com"}`, `{"name":"anna","email":"bad"}`, `{"name":"anna","email":"anna@example.com","target":"http://evil"}`, `{"name":"anna","email":"anna@example.com"} {}`} {
		assertStatus(t, call(c, "POST", "/api/users", b), 400)
	}
}
func TestNewRejectsUnsafePublicAndConfiguredURLs(t *testing.T) {
	for _, public := range []string{"http://headscale.example.com", "https://headscale:9981", "https://localhost", "https://192.168.1.2", "https://headscale.internal", "https://user:password@example.com", "https://example.com/?key=x", "https://example.com/admin"} {
		if _, e := New("http://headscale:9981", testSecret, public, "file"); e == nil {
			t.Fatal(public)
		}
	}
	if _, e := New("http://headscale:9981/api", testSecret, "https://headscale.example.com", "file"); e == nil {
		t.Fatal("base path accepted")
	}
	c := fixture(t, "0.29.5", "file", func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, `{}`) })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if e := c.request(ctx, "GET", "/api/v1/node", nil, nil); e != unavailable {
		t.Fatal(e)
	}
}
func TestResponseSizeBound(t *testing.T) {
	c := fixture(t, "0.29.5", "file", func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, strings.Repeat("x", maxBody+1)) })
	assertStatus(t, call(c, "GET", "/api/devices", ""), 502)
}
