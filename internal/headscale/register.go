package headscale

import (
	"errors"
	"net/http"
	"regexp"
)

var authIDPattern = regexp.MustCompile(`^hskey-authreq-[A-Za-z0-9_-]{8,256}$`)

func (c *Client) registerDevice(w http.ResponseWriter, r *http.Request, minor int) {
	if minor != 29 {
		fail(w, unsupported)
		return
	}
	var b struct {
		AuthID string `json:"authId"`
		UserID string `json:"userId"`
	}
	if decode(w, r, &b) != nil || !authIDPattern.MatchString(b.AuthID) || !validID(b.UserID) {
		fail(w, validation)
		return
	}
	// Resolve the authoritative user name; never confuse a preauth key with an Auth ID.
	var users struct {
		Users []rawUser `json:"users"`
	}
	if err := c.request(r.Context(), "GET", "/api/v1/user", nil, &users); err != nil {
		fail(w, err)
		return
	}
	username := ""
	for _, u := range users.Users {
		if string(u.ID) == b.UserID {
			username = u.Name
			break
		}
	}
	if username == "" {
		fail(w, validation)
		return
	}
	var result struct {
		Node rawNode `json:"node"`
	}
	if err := c.request(r.Context(), "POST", "/api/v1/auth/register", map[string]string{"user": username, "authId": b.AuthID}, &result); err != nil {
		if errors.Is(err, unavailable) || errors.Is(err, validation) {
			respond(w, 502, map[string]string{"error": "registration_failed", "message": "注册未完成：请确认 Auth ID 属于当前服务器、尚未过期，且用户有权注册请求的标签。"})
			return
		}
		fail(w, err)
		return
	}
	if !validID(string(result.Node.ID)) {
		respond(w, 502, map[string]string{"error": "registration_failed", "message": "Headscale未返回有效注册设备，请刷新设备列表确认。"})
		return
	}
	respond(w, 200, map[string]any{"device": normalizeNode(result.Node)})
}
