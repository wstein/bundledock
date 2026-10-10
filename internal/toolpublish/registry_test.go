package toolpublish

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestTagAbsentRequiresAuthenticated404(t *testing.T) {
	for _, test := range []struct {
		name                        string
		tokenStatus, manifestStatus int
		body                        string
		networkAt                   int
		wantErr                     bool
	}{
		{"absent", 200, 404, `{"token":"private-read-bearer"}`, 0, false},
		{"occupied", 200, 200, `{"token":"private-read-bearer"}`, 0, true},
		{"unauthorized", 200, 401, `{"token":"private-read-bearer"}`, 0, true},
		{"forbidden", 200, 403, `{"token":"private-read-bearer"}`, 0, true},
		{"registry-failure", 200, 500, `{"token":"private-read-bearer"}`, 0, true},
		{"token-auth", 401, 404, `{}`, 0, true},
		{"token-not-found", 404, 404, `{}`, 0, true},
		{"token-empty", 200, 404, `{}`, 0, true},
		{"token-malformed", 200, 404, `not-json`, 0, true},
		{"token-network", 200, 404, `{"token":"private-read-bearer"}`, 1, true},
		{"manifest-network", 200, 404, `{"token":"private-read-bearer"}`, 2, true},
		{"token-redirect", 302, 404, `{}`, 0, true},
		{"manifest-redirect", 200, 302, `{"token":"private-read-bearer"}`, 0, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.URL.Scheme != "https" || r.URL.Host != "quay.io" {
					t.Fatalf("credentials sent outside Quay: %s", r.URL)
				}
				status, body := test.tokenStatus, test.body
				if calls == 1 {
					user, password, ok := r.BasicAuth()
					if !ok || user != "example+robot" || password != "robot-secret" || r.Method != "GET" || r.URL.Path != "/v2/auth" || r.URL.Query().Get("scope") != "repository:example/toolbox:pull" || r.URL.Query().Get("service") != "quay.io" {
						t.Fatal("incorrect scoped token request")
					}
				} else if calls == 2 {
					if r.Method != "HEAD" || r.URL.Path != "/v2/example/toolbox/manifests/2026.10.10-b12.4" || r.Header.Get("Authorization") != "Bearer private-read-bearer" || !strings.Contains(r.Header.Get("Accept"), "application/vnd.oci.image.index.v1+json") || !strings.Contains(r.Header.Get("Accept"), "application/vnd.docker.distribution.manifest.list.v2+json") {
						t.Fatal("incorrect authenticated manifest check")
					}
					status, body = test.manifestStatus, ""
				} else {
					t.Fatal("followed redirect or retried failed check")
				}
				if test.networkAt == calls {
					return nil, errors.New("network unavailable")
				}
				return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{"Location": []string{"https://elsewhere.invalid/stolen"}}}, nil
			})}
			err := CheckTagAbsent(context.Background(), client, "quay.io/example/toolbox", "2026.10.10-b12.4", "example+robot", "robot-secret")
			if (err != nil) != test.wantErr {
				t.Fatalf("unexpected absence check: %v", err)
			}
			if err != nil && (strings.Contains(err.Error(), "robot-secret") || strings.Contains(err.Error(), "private-read-bearer")) {
				t.Fatal("error exposes credentials")
			}
		})
	}
}

func TestTagAbsentRejectsInvalidInputWithoutRequests(t *testing.T) {
	for _, test := range []struct{ image, tag, user, token string }{
		{"https://evil.invalid/toolbox", "2026.10.10-b12.4", "robot", "secret"},
		{"quay.io/example/toolbox", "stable", "robot", "secret"},
		{"quay.io/example/toolbox", "2026.10.10-b12.4", "", "secret"},
		{"quay.io/example/toolbox", "2026.10.10-b12.4", "robot", ""},
	} {
		client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			t.Fatal("invalid input contacted registry")
			return nil, nil
		})}
		if err := CheckTagAbsent(context.Background(), client, test.image, test.tag, test.user, test.token); err == nil {
			t.Fatal("invalid identity accepted")
		}
	}
}
