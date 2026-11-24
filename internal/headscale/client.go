// Package headscale is a fixed-target REST adapter. Its Handler requires an
// authenticated, CSRF-checked parent router; it does not implement console auth.
package headscale

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/mail"
	"net/netip"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const maxBody = 4 << 20

type Client struct {
	base, publicURL *url.URL
	key, policyMode string
	http            *http.Client
	mutation        sync.Mutex
}
type apiError struct {
	code   string
	status int
}

func (e *apiError) Error() string { return e.code }

var unavailable = &apiError{"headscale_unavailable", 502}
var noDatabasePolicy = errors.New("database_policy_not_initialized")
var validation = &apiError{"validation_error", 400}
var conflict = &apiError{"conflict", 409}
var unsupported = &apiError{"unsupported_version", 422}
var readOnly = &apiError{"policy_read_only", 409}
var versionPattern = regexp.MustCompile(`^v?0\.(28|29)\.[0-9]+$`)
var identifierPattern = regexp.MustCompile(`^[1-9][0-9]{0,19}$`)
var namePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,62}$`)
var nodeNamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9-]{0,62}$`)
var tagPattern = regexp.MustCompile(`^tag:[a-zA-Z][a-zA-Z0-9-]{0,62}$`)

// New never makes a network request. URLs are deploy-time configuration, not user input.
// publicURL is a public HTTPS origin for client enrollment, not the Docker URL.
func New(baseURL, apiKey, publicURL, policyMode string) (*Client, error) {
	parse := func(s string) (*url.URL, error) {
		u, e := url.Parse(s)
		if e != nil || u == nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.RawPath != "" || (u.Path != "" && u.Path != "/") || (u.Scheme != "https" && u.Scheme != "http") {
			return nil, errors.New("invalid Headscale URL")
		}
		u.Path = ""
		return u, nil
	}
	base, e := parse(baseURL)
	if e != nil {
		return nil, e
	}
	public, e := parse(publicURL)
	if e != nil {
		return nil, errors.New("invalid public enrollment URL")
	}
	host := strings.ToLower(public.Hostname())
	ip, ipErr := netip.ParseAddr(host)
	if public.Scheme != "https" || host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".internal") || (!strings.Contains(host, ".") && ipErr != nil) || (ipErr == nil && (!ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast())) {
		return nil, errors.New("public enrollment URL must use public HTTPS host")
	}
	if apiKey == "" || strings.TrimSpace(apiKey) != apiKey || len(apiKey) > 4096 || strings.ContainsAny(apiKey, "\r\n\x00") {
		return nil, errors.New("invalid management API key")
	}
	if policyMode != "database" && policyMode != "file" && policyMode != "unknown" {
		return nil, errors.New("invalid policy mode")
	}
	transport := &http.Transport{Proxy: nil, DialContext: (&net.Dialer{Timeout: 8 * time.Second, KeepAlive: 30 * time.Second}).DialContext, TLSHandshakeTimeout: 8 * time.Second, ResponseHeaderTimeout: 8 * time.Second, MaxIdleConns: 20, MaxIdleConnsPerHost: 10, IdleConnTimeout: 60 * time.Second}
	return &Client{base: base, publicURL: public, key: apiKey, policyMode: policyMode, http: &http.Client{Transport: transport, Timeout: 8 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}
func (c *Client) request(ctx context.Context, method, path string, payload, out any) error {
	var body io.Reader
	if payload != nil {
		b, e := json.Marshal(payload)
		if e != nil {
			return validation
		}
		if len(b) > maxBody {
			return validation
		}
		body = bytes.NewReader(b)
	}
	target := *c.base
	target.Path = path
	req, e := http.NewRequestWithContext(ctx, method, target.String(), body)
	if e != nil {
		return unavailable
	}
	req.Header.Set("Authorization", "Bearer "+c.key)
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, e := c.http.Do(req)
	if e != nil {
		return unavailable
	}
	defer res.Body.Close()
	data, e := io.ReadAll(io.LimitReader(res.Body, maxBody+1))
	if e != nil || len(data) > maxBody {
		return unavailable
	}
	// Headscale v0.29.7 permits an uninitialized database policy at startup,
	// but GetPolicy returns this exact RPC error. Never treat generic 5xx as empty.
	if method == "GET" && path == "/api/v1/policy" && c.policyMode == "database" && (res.StatusCode == 500 || res.StatusCode == 404) {
		var rpc struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		}
		if json.Unmarshal(data, &rpc) == nil && (rpc.Code == 2 || rpc.Code == 5 || rpc.Code == 13) && (rpc.Message == "loading ACL from database: acl policy not found" || rpc.Message == "acl policy not found") {
			return noDatabasePolicy
		}
	}
	switch {
	case res.StatusCode == 401 || res.StatusCode == 403:
		return &apiError{"headscale_unauthorized", 502}
	case res.StatusCode == 409:
		return conflict
	case res.StatusCode == 400 || res.StatusCode == 422:
		return validation
	case res.StatusCode < 200 || res.StatusCode >= 300:
		return unavailable
	}
	if out != nil {
		if len(data) == 0 || json.Unmarshal(data, out) != nil {
			return unavailable
		}
	}
	return nil
}
func (c *Client) version(ctx context.Context) (string, int, error) {
	var v struct {
		Version string `json:"version"`
	}
	if e := c.request(ctx, "GET", "/version", nil, &v); e != nil {
		return "", 0, e
	}
	if !versionPattern.MatchString(v.Version) {
		return "", 0, unsupported
	}
	minor := 28
	if strings.HasPrefix(strings.TrimPrefix(v.Version, "v"), "0.29.") {
		minor = 29
	}
	return v.Version, minor, nil
}
func respond(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func fail(w http.ResponseWriter, e error) {
	a := unavailable
	var typed *apiError
	if errors.As(e, &typed) {
		a = typed
	}
	messages := map[string]string{"headscale_unavailable": "无法连接 Headscale 服务。", "headscale_unauthorized": "Headscale 管理凭据无效或已过期。", "unsupported_version": "仅支持 Headscale 0.28.x 与 0.29.x 正式版本。", "validation_error": "请求参数或策略校验未通过。", "conflict": "数据已发生变化，请刷新后重试。", "policy_read_only": "当前策略模式或版本不允许发布策略。"}
	respond(w, a.status, map[string]string{"error": a.code, "message": messages[a.code]})
}
func decode(w http.ResponseWriter, r *http.Request, out any) error {
	if strings.Split(r.Header.Get("Content-Type"), ";")[0] != "application/json" {
		return validation
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBody)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if d.Decode(out) != nil {
		return validation
	}
	if d.Decode(&struct{}{}) != io.EOF {
		return validation
	}
	return nil
}
func validID(id string) bool {
	if !identifierPattern.MatchString(id) {
		return false
	}
	_, e := strconv.ParseUint(id, 10, 64)
	return e == nil
}
func revision(policy string) string {
	s := sha256.Sum256([]byte(policy))
	return hex.EncodeToString(s[:])
}
func canonicalRoutes(raw []string) ([]string, error) {
	if len(raw) > 256 {
		return nil, validation
	}
	seen := map[string]bool{}
	out := []string{}
	for _, s := range raw {
		p, e := netip.ParsePrefix(s)
		if e != nil || p.String() != s || p.Masked() != p || seen[s] {
			return nil, validation
		}
		seen[s] = true
		out = append(out, s)
	}
	return out, nil
}
func sameSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	m := map[string]bool{}
	for _, s := range a {
		m[s] = true
	}
	for _, s := range b {
		if !m[s] {
			return false
		}
	}
	return true
}

type stringID string

func (id *stringID) UnmarshalJSON(b []byte) error {
	var s string
	if len(b) > 0 && b[0] == '"' {
		if e := json.Unmarshal(b, &s); e != nil {
			return e
		}
	} else {
		s = string(b)
	}
	if s == "0" || s == "" {
		*id = ""
		return nil
	}
	if !validID(s) {
		return errors.New("invalid id")
	}
	*id = stringID(s)
	return nil
}

type rawUser struct {
	ID          stringID `json:"id"`
	Name        string   `json:"name"`
	DisplayName string   `json:"displayName"`
	Email       string   `json:"email"`
}
type rawNode struct {
	ID        stringID `json:"id"`
	Name      string   `json:"name"`
	GivenName string   `json:"givenName"`
	IPs       []string `json:"ipAddresses"`
	User      rawUser  `json:"user"`
	Online    *bool    `json:"online"`
	Expiry    *string  `json:"expiry"`
	Last      *string  `json:"lastSeen"`
	Tags      []string `json:"tags"`
	Available []string `json:"availableRoutes"`
	Approved  []string `json:"approvedRoutes"`
	Serving   []string `json:"subnetRoutes"`
}
type rawKey struct {
	ID       stringID `json:"id"`
	User     rawUser  `json:"user"`
	Key      string   `json:"key"`
	Reusable bool     `json:"reusable"`
	Used     bool     `json:"used"`
	Expiry   *string  `json:"expiration"`
	Tags     []string `json:"aclTags"`
}

func strs(in []string) []string {
	if in == nil {
		return []string{}
	}
	return in
}
func normalizedTime(raw *string) *string {
	if raw == nil || *raw == "" {
		return nil
	}
	t, e := time.Parse(time.RFC3339Nano, *raw)
	if e != nil || t.IsZero() {
		return nil
	}
	v := t.UTC().Format(time.RFC3339Nano)
	return &v
}
func normalizeUser(u rawUser) map[string]any {
	initial := []rune(u.DisplayName)
	if len(initial) == 0 {
		initial = []rune(u.Name)
	}
	if len(initial) > 2 {
		initial = initial[:2]
	}
	return map[string]any{"id": string(u.ID), "name": u.Name, "displayName": u.DisplayName, "email": u.Email, "initial": strings.ToUpper(string(initial))}
}
func normalizeNode(n rawNode) map[string]any {
	name := n.GivenName
	if name == "" {
		name = n.Name
	}
	ip := ""
	for _, s := range n.IPs {
		a, e := netip.ParseAddr(s)
		if e == nil && (ip == "" || a.Is4()) {
			ip = s
			if a.Is4() {
				break
			}
		}
	}
	tags := []string{}
	for _, s := range n.Tags {
		tags = append(tags, strings.TrimPrefix(s, "tag:"))
	}
	return map[string]any{"id": string(n.ID), "name": name, "ip": ip, "ipAddresses": strs(n.IPs), "owner": n.User.Name, "userId": string(n.User.ID), "online": n.Online, "os": "未采集", "platform": "Unknown", "version": "未采集", "tags": tags, "expiry": normalizedTime(n.Expiry), "last": normalizedTime(n.Last), "routes": strs(n.Available), "approvedRoutes": strs(n.Approved), "servingRoutes": strs(n.Serving)}
}
func (c *Client) nodes(ctx context.Context) ([]rawNode, error) {
	var out struct {
		Nodes []rawNode `json:"nodes"`
	}
	e := c.request(ctx, "GET", "/api/v1/node", nil, &out)
	return out.Nodes, e
}
func (c *Client) node(ctx context.Context, id string) (rawNode, error) {
	var out struct {
		Node rawNode `json:"node"`
	}
	e := c.request(ctx, "GET", "/api/v1/node/"+id, nil, &out)
	return out.Node, e
}
func (c *Client) policy(ctx context.Context) (string, error) {
	var out struct {
		Policy string `json:"policy"`
	}
	e := c.request(ctx, "GET", "/api/v1/policy", nil, &out)
	if errors.Is(e, noDatabasePolicy) {
		return "", nil
	}
	return out.Policy, e
}
func normalizedKey(k rawKey) map[string]any {
	suffix := ""
	if len(k.Key) > 4 {
		suffix = k.Key[len(k.Key)-4:]
	}
	expiry := normalizedTime(k.Expiry)
	status := "有效"
	if k.Used && !k.Reusable {
		status = "已使用"
	}
	if expiry != nil {
		t, _ := time.Parse(time.RFC3339Nano, *expiry)
		if !t.After(time.Now()) {
			status = "已过期"
		}
	}
	tags := []string{}
	for _, s := range k.Tags {
		tags = append(tags, strings.TrimPrefix(s, "tag:"))
	}
	return map[string]any{"id": string(k.ID), "name": "预授权密钥 #" + string(k.ID), "prefix": "•••• " + suffix, "owner": k.User.Name, "userId": string(k.User.ID), "reusable": k.Reusable, "expiry": expiry, "status": status, "tags": tags}
}

// Handler exposes only the enumerated business routes. Parent must wrap it in auth.Require.
func (c *Client) Handler() http.Handler { return http.HandlerFunc(c.serveHTTP) }
func (c *Client) serveHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.URL.RawQuery != "" || strings.ContainsAny(r.URL.Path, "\\\x00") {
		fail(w, validation)
		return
	}
	if r.URL.Path == "/api/connection" && r.Method == "GET" {
		c.connection(w, r)
		return
	}
	_, minor, e := c.version(r.Context())
	if e != nil {
		fail(w, e)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/")
	parts := strings.Split(path, "/")
	switch {
	case path == "devices" && r.Method == "GET":
		nodes, e := c.nodes(r.Context())
		if e != nil {
			fail(w, e)
			return
		}
		out := []map[string]any{}
		for _, n := range nodes {
			out = append(out, normalizeNode(n))
		}
		respond(w, 200, map[string]any{"devices": out})
	case path == "routes" && r.Method == "GET":
		nodes, e := c.nodes(r.Context())
		if e != nil {
			fail(w, e)
			return
		}
		out := []map[string]any{}
		for _, n := range nodes {
			for _, cidr := range n.Available {
				approved := false
				for _, a := range n.Approved {
					if a == cidr {
						approved = true
					}
				}
				var serving *bool
				if n.Serving != nil {
					v := false
					for _, a := range n.Serving {
						if a == cidr {
							v = true
						}
					}
					serving = &v
				}
				out = append(out, map[string]any{"id": string(n.ID) + "|" + cidr, "nodeId": string(n.ID), "cidr": cidr, "description": "", "approved": approved, "serving": serving})
			}
		}
		respond(w, 200, map[string]any{"routes": out})
	case path == "users" && r.Method == "GET":
		var out struct {
			Users []rawUser `json:"users"`
		}
		if e := c.request(r.Context(), "GET", "/api/v1/user", nil, &out); e != nil {
			fail(w, e)
			return
		}
		users := []map[string]any{}
		for _, u := range out.Users {
			users = append(users, normalizeUser(u))
		}
		respond(w, 200, map[string]any{"users": users})
	case path == "users" && r.Method == "POST":
		c.createUser(w, r)
	case len(parts) >= 2 && parts[0] == "devices" && validID(parts[1]):
		c.deviceMutation(w, r, parts)
	case len(parts) == 2 && parts[0] == "routes" && validID(parts[1]) && r.Method == "POST":
		c.setRoutes(w, r, parts[1])
	case path == "keys" && r.Method == "GET":
		var out struct {
			Keys []rawKey `json:"preAuthKeys"`
		}
		if e := c.request(r.Context(), "GET", "/api/v1/preauthkey", nil, &out); e != nil {
			fail(w, e)
			return
		}
		keys := []map[string]any{}
		for _, k := range out.Keys {
			keys = append(keys, normalizedKey(k))
		}
		respond(w, 200, map[string]any{"keys": keys})
	case path == "keys" && r.Method == "POST":
		c.createKey(w, r)
	case len(parts) == 3 && parts[0] == "keys" && validID(parts[1]) && parts[2] == "expire" && r.Method == "POST":
		var b struct{}
		if decode(w, r, &b) != nil {
			fail(w, validation)
			return
		}
		if e := c.request(r.Context(), "POST", "/api/v1/preauthkey/expire", map[string]string{"id": parts[1]}, nil); e != nil {
			fail(w, e)
			return
		}
		respond(w, 200, map[string]bool{"ok": true})
	case path == "policy" && r.Method == "GET":
		p, e := c.policy(r.Context())
		if e != nil {
			if c.policyMode == "file" && errors.Is(e, unavailable) {
				respond(w, 503, map[string]string{"error": "policy_file_unavailable", "message": "Headscale 使用 file 策略模式，但策略文件无法通过 API 读取。请核对服务端 policy.path 是否为空、文件存在与读取权限。"})
				return
			}
			fail(w, e)
			return
		}
		respond(w, 200, map[string]any{"policy": p, "revision": revision(p), "mode": c.policyMode, "writable": minor == 29 && c.policyMode == "database"})
	case path == "policy/check" && r.Method == "POST":
		if minor != 29 {
			fail(w, unsupported)
			return
		}
		c.checkPolicy(w, r)
	case path == "policy" && r.Method == "PUT":
		if minor != 29 || c.policyMode != "database" {
			fail(w, readOnly)
			return
		}
		c.setPolicy(w, r)
	default:
		respond(w, 404, map[string]string{"error": "not_found", "message": "接口不存在或请求方法不支持。"})
	}
}

// Verify checks the fixed authenticated health endpoint after a config apply.
// It does not infer reachability of devices or data-plane services.
func (c *Client) Verify(ctx context.Context) error {
	var health struct {
		Database bool `json:"databaseConnectivity"`
	}
	if err := c.request(ctx, "GET", "/api/v1/health", nil, &health); err != nil {
		return err
	}
	if !health.Database {
		return unavailable
	}
	return nil
}

func (c *Client) connection(w http.ResponseWriter, r *http.Request) {
	v, minor, e := c.version(r.Context())
	if e != nil {
		fail(w, e)
		return
	}
	var health struct {
		Database bool `json:"databaseConnectivity"`
	}
	if e = c.request(r.Context(), "GET", "/api/v1/health", nil, &health); e != nil {
		fail(w, e)
		return
	}
	respond(w, 200, map[string]any{"url": c.base.String(), "publicUrl": c.publicURL.String(), "apiKeyConfigured": true, "configured": true, "mode": "live", "policyMode": c.policyMode, "version": v, "connected": health.Database, "capabilities": map[string]bool{"read": health.Database, "write": health.Database, "policyCheck": health.Database && minor == 29, "policyWrite": health.Database && minor == 29 && c.policyMode == "database", "keys": health.Database}})
}
func (c *Client) createUser(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	}
	if decode(w, r, &b) != nil || !namePattern.MatchString(b.Name) || !validEmail(b.Email) {
		fail(w, validation)
		return
	}
	var out struct {
		User rawUser `json:"user"`
	}
	if e := c.request(r.Context(), "POST", "/api/v1/user", map[string]string{"name": b.Name, "email": b.Email}, &out); e != nil {
		fail(w, e)
		return
	}
	respond(w, 200, map[string]any{"user": normalizeUser(out.User)})
}
func validEmail(s string) bool {
	if len(s) > 254 || strings.ContainsAny(s, "\r\n <>\t") {
		return false
	}
	address, err := mail.ParseAddress(s)
	if err != nil || address.Address != s {
		return false
	}
	at := strings.LastIndexByte(s, '@')
	return at > 0 && at < len(s)-1 && strings.Contains(s[at+1:], ".") && strings.Count(s, "@") == 1
}
func (c *Client) deviceMutation(w http.ResponseWriter, r *http.Request, p []string) {
	id := p[1]
	if len(p) == 2 && r.Method == "DELETE" {
		if e := c.request(r.Context(), "DELETE", "/api/v1/node/"+id, nil, nil); e != nil {
			fail(w, e)
			return
		}
		respond(w, 200, map[string]bool{"ok": true})
		return
	}
	if len(p) != 3 || r.Method != "POST" {
		respond(w, 404, map[string]string{"error": "not_found", "message": "接口不存在或请求方法不支持。"})
		return
	}
	target := "/api/v1/node/" + id
	var payload any
	switch p[2] {
	case "rename":
		var b struct {
			Name string `json:"name"`
		}
		if decode(w, r, &b) != nil || !nodeNamePattern.MatchString(b.Name) {
			fail(w, validation)
			return
		}
		target += "/rename/" + b.Name
	case "tags":
		var b struct {
			Tags []string `json:"tags"`
		}
		if decode(w, r, &b) != nil || b.Tags == nil || len(b.Tags) > 64 {
			fail(w, validation)
			return
		}
		seen := map[string]bool{}
		tags := []string{}
		for _, t := range b.Tags {
			if !strings.HasPrefix(t, "tag:") {
				t = "tag:" + t
			}
			if !tagPattern.MatchString(t) || seen[t] {
				fail(w, validation)
				return
			}
			seen[t] = true
			tags = append(tags, t)
		}
		target += "/tags"
		payload = map[string]any{"tags": tags}
	case "expire":
		var b struct{}
		if decode(w, r, &b) != nil {
			fail(w, validation)
			return
		}
		target += "/expire"
	default:
		respond(w, 404, map[string]string{"error": "not_found", "message": "接口不存在或请求方法不支持。"})
		return
	}
	var out struct {
		Node rawNode `json:"node"`
	}
	if e := c.request(r.Context(), "POST", target, payload, &out); e != nil {
		fail(w, e)
		return
	}
	respond(w, 200, map[string]any{"device": normalizeNode(out.Node)})
}
func (c *Client) setRoutes(w http.ResponseWriter, r *http.Request, id string) {
	var b struct {
		Routes   []string `json:"routes"`
		Expected []string `json:"expectedRoutes"`
	}
	if decode(w, r, &b) != nil || b.Routes == nil || b.Expected == nil {
		fail(w, validation)
		return
	}
	routes, e := canonicalRoutes(b.Routes)
	if e != nil {
		fail(w, e)
		return
	}
	expected, e := canonicalRoutes(b.Expected)
	if e != nil {
		fail(w, e)
		return
	}
	c.mutation.Lock()
	defer c.mutation.Unlock()
	n, e := c.node(r.Context(), id)
	if e != nil {
		fail(w, e)
		return
	}
	if !sameSet(expected, n.Approved) {
		fail(w, conflict)
		return
	}
	for _, s := range routes {
		found := false
		for _, a := range n.Available {
			if a == s {
				found = true
			}
		}
		if !found {
			fail(w, validation)
			return
		}
	}
	var out struct {
		Node rawNode `json:"node"`
	}
	if e = c.request(r.Context(), "POST", "/api/v1/node/"+id+"/approve_routes", map[string]any{"routes": routes}, &out); e != nil {
		fail(w, e)
		return
	}
	respond(w, 200, map[string]any{"device": normalizeNode(out.Node)})
}
func (c *Client) createKey(w http.ResponseWriter, r *http.Request) {
	var b struct {
		UserID   string `json:"userId"`
		Name     string `json:"name"`
		TTL      int    `json:"ttlDays"`
		Reusable bool   `json:"reusable"`
	}
	if decode(w, r, &b) != nil || !validID(b.UserID) || len([]rune(b.Name)) == 0 || len([]rune(b.Name)) > 80 || strings.ContainsAny(b.Name, "\r\n\x00") || (b.TTL != 1 && b.TTL != 7 && b.TTL != 30) {
		fail(w, validation)
		return
	}
	expiry := time.Now().UTC().Add(time.Duration(b.TTL) * 24 * time.Hour).Format(time.RFC3339)
	var out struct {
		Key rawKey `json:"preAuthKey"`
	}
	payload := map[string]any{"user": b.UserID, "expiration": expiry, "reusable": b.Reusable, "ephemeral": false, "aclTags": []string{}}
	if e := c.request(r.Context(), "POST", "/api/v1/preauthkey", payload, &out); e != nil {
		fail(w, e)
		return
	}
	if out.Key.Key == "" || len(out.Key.Key) > 4096 || strings.ContainsAny(out.Key.Key, "\r\n\x00") {
		fail(w, unavailable)
		return
	}
	cmd := "tailscale up --login-server=" + shellQuote(c.publicURL.String()) + " --authkey=" + shellQuote(out.Key.Key)
	respond(w, 200, map[string]any{"key": out.Key.Key, "command": cmd, "id": string(out.Key.ID), "expiry": normalizedTime(out.Key.Expiry)})
}
func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }
func (c *Client) checkPolicy(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Policy string `json:"policy"`
	}
	if decode(w, r, &b) != nil || strings.TrimSpace(b.Policy) == "" {
		fail(w, validation)
		return
	}
	if e := c.request(r.Context(), "POST", "/api/v1/policy/check", map[string]string{"policy": b.Policy}, nil); e != nil {
		fail(w, e)
		return
	}
	respond(w, 200, map[string]bool{"valid": true})
}
func (c *Client) setPolicy(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Policy   string `json:"policy"`
		Revision string `json:"revision"`
	}
	if decode(w, r, &b) != nil || strings.TrimSpace(b.Policy) == "" || len(b.Revision) != 64 {
		fail(w, validation)
		return
	}
	if _, e := hex.DecodeString(b.Revision); e != nil {
		fail(w, validation)
		return
	}
	c.mutation.Lock()
	defer c.mutation.Unlock()
	old, e := c.policy(r.Context())
	if e != nil {
		fail(w, e)
		return
	}
	if revision(old) != b.Revision {
		fail(w, conflict)
		return
	}
	payload := map[string]string{"policy": b.Policy}
	if e = c.request(r.Context(), "POST", "/api/v1/policy/check", payload, nil); e != nil {
		fail(w, e)
		return
	}
	// Check again after server validation, reducing the window for external editors.
	current, e := c.policy(r.Context())
	if e != nil {
		fail(w, e)
		return
	}
	if revision(current) != b.Revision {
		fail(w, conflict)
		return
	}
	var out struct {
		Policy string `json:"policy"`
	}
	if e = c.request(r.Context(), "PUT", "/api/v1/policy", payload, &out); e != nil {
		fail(w, e)
		return
	}
	respond(w, 200, map[string]any{"policy": out.Policy, "revision": revision(out.Policy), "mode": c.policyMode, "writable": true})
}
