// Local E2E mock; this executable is never included in production images.
package main

import (
	"encoding/json"
	"io/fs"
	"log"
	"meshconsole/internal/auth"
	"meshconsole/internal/captcha"
	"meshconsole/internal/config"
	"meshconsole/internal/webassets"
	"meshconsole/internal/webserver"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
)

func main() {
	var mu sync.Mutex
	node := map[string]any{"id": "9007199254740993", "givenName": "real-gateway", "name": "gateway", "ipAddresses": []string{"100.64.0.41"}, "online": true, "user": map[string]any{"id": "1", "name": "operator"}, "tags": []string{"tag:prod"}, "availableRoutes": []string{"10.10.0.0/24"}, "approvedRoutes": []string{}, "subnetRoutes": []string{}, "preAuthKey": map[string]any{"key": "UPSTREAM_PREAUTH_MUST_NOT_LEAK"}, "machineKey": "UPSTREAM_MACHINE_KEY_MUST_NOT_LEAK"}
	users := []map[string]any{{"id": "1", "name": "operator", "email": "operator@example.com"}}
	policy := `{"grants":[{"src":["operator@"],"dst":["tag:prod"],"ip":["tcp:22"]}]}`
	var createdKey string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if r.Header.Get("Authorization") != "Bearer INTEGRATION_MANAGEMENT_SECRET" {
			w.WriteHeader(401)
			return
		}
		encode := func(v any) { _ = json.NewEncoder(w).Encode(v) }
		switch r.URL.Path {
		case "/version":
			encode(map[string]string{"version": "0.29.5"})
		case "/api/v1/auth/register":
			var b map[string]string
			_ = json.NewDecoder(r.Body).Decode(&b)
			if b["authId"] != "hskey-authreq-VALIDTEST123" || b["user"] != "operator" {
				w.WriteHeader(400)
				encode(map[string]string{"message": "invalid auth request"})
				return
			}
			encode(map[string]any{"node": node})
		case "/api/v1/health":
			encode(map[string]bool{"databaseConnectivity": true})
		case "/api/v1/node":
			encode(map[string]any{"nodes": []any{node}})
		case "/api/v1/node/9007199254740993":
			encode(map[string]any{"node": node})
		case "/api/v1/node/9007199254740993/rename/renamed-gateway":
			node["givenName"] = "renamed-gateway"
			encode(map[string]any{"node": node})
		case "/api/v1/node/9007199254740993/tags":
			var b map[string]any
			_ = json.NewDecoder(r.Body).Decode(&b)
			node["tags"] = b["tags"]
			encode(map[string]any{"node": node})
		case "/api/v1/node/9007199254740993/approve_routes":
			var b map[string]any
			_ = json.NewDecoder(r.Body).Decode(&b)
			node["approvedRoutes"] = b["routes"]
			encode(map[string]any{"node": node})
		case "/api/v1/node/9007199254740993/expire":
			node["expiry"] = "2020-01-01T00:00:00Z"
			encode(map[string]any{"node": node})
		case "/api/v1/user":
			if r.Method == "POST" {
				var b map[string]any
				_ = json.NewDecoder(r.Body).Decode(&b)
				u := map[string]any{"id": "2", "name": b["name"], "email": b["email"]}
				users = append(users, u)
				encode(map[string]any{"user": u})
			} else {
				encode(map[string]any{"users": users})
			}
		case "/api/v1/preauthkey":
			if r.Method == "POST" {
				createdKey = "LIVE_MOCK_ENROLLMENT_KEY"
				encode(map[string]any{"preAuthKey": map[string]any{"id": "7", "key": createdKey, "user": users[0], "expiration": "2099-01-01T00:00:00Z"}})
			} else {
				items := []any{}
				if createdKey != "" {
					items = append(items, map[string]any{"id": "7", "key": createdKey, "user": users[0], "expiration": "2099-01-01T00:00:00Z"})
				}
				encode(map[string]any{"preAuthKeys": items})
			}
		case "/api/v1/preauthkey/expire":
			encode(map[string]any{})
		case "/api/v1/policy":
			if r.Method == "PUT" {
				var b struct {
					Policy string `json:"policy"`
				}
				_ = json.NewDecoder(r.Body).Decode(&b)
				policy = b.Policy
			}
			encode(map[string]any{"policy": policy})
		case "/api/v1/policy/check":
			encode(map[string]any{})
		default:
			w.WriteHeader(404)
			encode(map[string]string{"error": "unknown_mock_route"})
		}
	}))
	defer upstream.Close()
	hash, _ := auth.HashPassword("Live-fixture-only!")
	values := map[string]string{"APP_ENV": "development", "HOST": "127.0.0.1", "PORT": "3082", "CONSOLE_ORIGIN": "http://127.0.0.1:3082", "COOKIE_SECURE": "false", "DEMO_MODE": "false", "CONSOLE_USERNAME": "live-test", "CONSOLE_PASSWORD_HASH": hash, "HEADSCALE_URL": upstream.URL, "HEADSCALE_PUBLIC_URL": "https://headscale.example.com", "HEADSCALE_API_KEY": "INTEGRATION_MANAGEMENT_SECRET", "HEADSCALE_POLICY_MODE": "database"}
	cfg, err := config.Load(func(k string) string { return values[k] })
	if err != nil {
		log.Fatal(err)
	}
	front, _ := fs.Sub(webassets.Frontend, "static/dist")
	handler, err := webserver.New(cfg, front, auth.WithCaptchaFactory(func() (string, []byte, error) { return captcha.Generate("TEST") }))
	if err != nil {
		log.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:3082")
	if err != nil {
		log.Fatal(err)
	}
	log.Fatal(http.Serve(listener, handler))
}
