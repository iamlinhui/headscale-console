package headscale

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestVerifyAuthenticatedHealthRequiresDatabaseConnectivity(t *testing.T) {
	for _, tc := range []struct {
		status  int
		body    string
		success bool
	}{{200, `{"databaseConnectivity":true}`, true}, {200, `{"databaseConnectivity":false}`, false}, {200, `{}`, false}, {500, `{"databaseConnectivity":true}`, false}, {401, `unauthorized`, false}, {200, `not JSON`, false}} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/api/v1/health" || r.Method != "GET" || r.Header.Get("Authorization") != "Bearer "+testSecret {
				t.Error("unexpected request")
			}
			w.WriteHeader(tc.status)
			_, _ = io.WriteString(w, tc.body)
		}))
		c, err := New(srv.URL, testSecret, "https://headscale.example.com", "file")
		if err != nil {
			t.Fatal(err)
		}
		err = c.Verify(context.Background())
		srv.Close()
		if (err == nil) != tc.success {
			t.Fatalf("status %d body %q: %v", tc.status, tc.body, err)
		}
	}
}
