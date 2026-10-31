package quay

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
)

func TestGitErrorsNeverContainTheToken(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	const token = "ghs_secrettoken123"
	t.Setenv("GITHUB_TOKEN", token)
	g := &GitCLI{}
	header := base64.StdEncoding.EncodeToString([]byte("x-access-token:" + token))
	for name, run := range map[string]func() error{
		"unknown command": func() error { _, err := g.git(context.Background(), "no-such-subcommand", token); return err },
		"verify source":   func() error { return g.VerifySource(context.Background(), strings.Repeat("a", 40)) },
		"remote tag": func() error {
			_, err := g.RemoteTag(context.Background(), "toolbox/x", strings.Repeat("a", 40), "m")
			return err
		},
	} {
		err := run()
		if err == nil {
			continue
		}
		for _, secret := range []string{token, header, "extraheader", "AUTHORIZATION"} {
			if strings.Contains(err.Error(), secret) {
				t.Errorf("%s: error %q contains %q", name, err, secret)
			}
		}
	}
}

func TestRegistryErrorsNeverContainTheRobotPassword(t *testing.T) {
	const password = "robot-password-xyz"
	basic := base64.StdEncoding.EncodeToString([]byte("robot:" + password))
	for name, handler := range map[string]http.HandlerFunc{
		"rejected": func(w http.ResponseWriter, r *http.Request) { http.Error(w, "bad "+password+" "+basic, 401) },
		"bad token": func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"token":"bad token ` + password + `"}`))
		},
		"no json": func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(password + basic)) },
		"manifest": func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/v2/auth" {
				w.Write([]byte(`{"token":"tok"}`))
				return
			}
			http.Error(w, password+basic+r.Header.Get("Authorization"), 500)
		},
	} {
		srv := httptest.NewServer(handler)
		c := clientFor(t, srv)
		var errs []error
		reg, err := c.RegistryAs(context.Background(), "toolbox", "robot", password)
		errs = append(errs, err)
		if reg != nil {
			_, err = reg.ManifestRef(context.Background(), "stable")
			errs = append(errs, err)
			_, err = reg.Manifest(context.Background(), "sha256:"+strings.Repeat("a", 64))
			errs = append(errs, err)
		}
		errs = append(errs, c.DeleteTag(context.Background(), "toolbox", "staging-2026.10.01-b4.1-amd64", "robot", password))
		srv.Close()
		for _, err := range errs {
			if err != nil && (strings.Contains(err.Error(), password) || strings.Contains(err.Error(), basic)) {
				t.Errorf("%s: error %q leaks the credential", name, err)
			}
		}
	}
}
