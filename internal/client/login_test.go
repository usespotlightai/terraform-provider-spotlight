package client_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/client"
)

func writeCredentials(t *testing.T, dir, content string) string {
	t.Helper()

	path := filepath.Join(dir, "credentials.json")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	return path
}

func TestAnAddressIsTheSchemeHostAndPort(t *testing.T) {
	for raw, want := range map[string]string{
		"https://app.usespotlight.ai":          "https://app.usespotlight.ai",
		"https://app.usespotlight.ai/":         "https://app.usespotlight.ai",
		"HTTPS://App.UseSpotlight.ai/api/":     "https://app.usespotlight.ai",
		"https://app.usespotlight.ai:443/api":  "https://app.usespotlight.ai",
		"http://localhost:80":                  "http://localhost",
		"http://localhost:3000/":               "http://localhost:3000",
		"https://spotlight.example.com:8443/x": "https://spotlight.example.com:8443",
		"http://[::1]:3000":                    "http://[::1]:3000",
		"http://[::1]":                         "http://[::1]",
	} {
		got, err := client.Address(raw)
		if err != nil || got != want {
			t.Errorf("Address(%q) = %q, %v; want %q", raw, got, err, want)
		}
	}

	if _, err := client.Address("app.usespotlight.ai"); err == nil {
		t.Error("expected an address with no scheme to fail")
	}
}

func TestTheCredentialsPathFollowsTheEnvironment(t *testing.T) {
	home, xdg, config := t.TempDir(), t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv(client.XDGConfigHomeEnv, "")
	t.Setenv(client.ConfigDirEnv, "")

	for _, each := range []struct {
		xdg, config, want string
	}{
		{"", "", filepath.Join(home, ".config", "spotlight", "credentials.json")},
		{xdg, "", filepath.Join(xdg, "spotlight", "credentials.json")},
		{xdg, config, filepath.Join(config, "credentials.json")},
	} {
		t.Setenv(client.XDGConfigHomeEnv, each.xdg)
		t.Setenv(client.ConfigDirEnv, each.config)

		if got, err := client.CredentialsPath(); err != nil || got != each.want {
			t.Errorf("with XDG_CONFIG_HOME %q and SPOTLIGHT_CONFIG_DIR %q, got %q, %v; want %q", each.xdg, each.config, got, err, each.want)
		}
	}
}

func TestAStoredLoginIsFoundByItsAddress(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv(client.ConfigDirEnv, "")
	t.Setenv(client.XDGConfigHomeEnv, xdg)
	writeCredentials(t, filepath.Join(xdg, "spotlight"), `{"logins": {
		"https://app.usespotlight.ai": {"token": "session-1", "user": "person@example.com"},
		"http://localhost:3000": {"token": "session-2", "user": "dev@example.com"}
	}}`)

	login, found, err := client.StoredLogin("https://App.usespotlight.ai/api/")
	if err != nil || !found {
		t.Fatalf("got %v, %v", found, err)
	}

	if want := (client.Login{Address: "https://app.usespotlight.ai", Token: "session-1", User: "person@example.com"}); login != want {
		t.Errorf("got %+v, want %+v", login, want)
	}

	if _, found, err := client.StoredLogin("https://spotlight.example.com"); err != nil || found {
		t.Errorf("found a login for another address: %v, %v", found, err)
	}
}

func TestALoginIsNeverSentOverPlainHTTPToAnotherHost(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv(client.ConfigDirEnv, "")
	t.Setenv(client.XDGConfigHomeEnv, xdg)
	writeCredentials(t, filepath.Join(xdg, "spotlight"), `{"logins": {
		"http://app.example.com": {"token": "session-1", "user": "person@example.com"},
		"http://spotlight.test": {"token": "session-2", "user": "dev@example.com"}
	}}`)

	if _, found, err := client.StoredLogin("http://app.example.com"); err != nil || found {
		t.Errorf("used a login over plain http: %v, %v", found, err)
	}

	if _, found, err := client.StoredLogin("http://spotlight.test"); err != nil || !found {
		t.Errorf("did not use a local login over http: %v, %v", found, err)
	}
}

func TestNoCredentialsFileIsNoLogin(t *testing.T) {
	t.Setenv(client.ConfigDirEnv, t.TempDir())

	if _, found, err := client.StoredLogin("https://app.usespotlight.ai"); err != nil || found {
		t.Errorf("got %v, %v; want no login and no error", found, err)
	}
}

func TestACredentialsFileThatDoesNotParseNamesItsPath(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(client.ConfigDirEnv, dir)
	path := writeCredentials(t, dir, `{"logins": [`)

	_, _, err := client.StoredLogin("https://app.usespotlight.ai")
	if err == nil || !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), "run spotlight login") {
		t.Fatalf("got %v", err)
	}
}

func TestALoginIsSentAsABearerToken(t *testing.T) {
	var got string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("Authorization")
		reply(w, http.StatusOK, "application/json", map[string]any{"id": "ws-1", "version": 1, "address": "acme", "name": "Acme", "slug": "acme", "warnings": []any{}})
	}))
	t.Cleanup(server.Close)

	login := client.Login{Address: server.URL, Token: "session-1", User: "person@example.com"}
	api, err := client.New(client.Config{URL: server.URL, Login: &login})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := api.ReadWorkspace(context.Background(), "ws-1"); err != nil {
		t.Fatal(err)
	}

	if got != "Bearer session-1" {
		t.Errorf("sent Authorization %q", got)
	}
}

func TestARejectedLoginSaysToSignInAgain(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		reply(w, http.StatusUnauthorized, "application/problem+json", map[string]any{
			"type": "urn:spotlight:problem:unauthenticated", "title": "No valid credentials", "status": 401,
		})
	}))
	t.Cleanup(server.Close)

	login := client.Login{Address: server.URL, Token: "expired-session-token", User: "person@example.com"}
	api, err := client.New(client.Config{URL: server.URL, Login: &login, Retries: 2})
	if err != nil {
		t.Fatal(err)
	}

	_, err = api.ReadWorkspace(context.Background(), "ws-1")
	want := "the login of person@example.com for " + server.URL + " expired or was revoked; run spotlight login --url " + server.URL + " to sign in again"
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("got %v, want %q", err, want)
	}

	if strings.Contains(err.Error(), "expired-session-token") {
		t.Error("the error shows the token")
	}

	if attempts != 1 {
		t.Errorf("%d attempts, want 1", attempts)
	}
}

func TestWithNoURLTheOnlyLoginIsUsed(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(client.ConfigDirEnv, dir)
	writeCredentials(t, dir, `{"logins": {
		"https://app.usespotlight.ai": {"token": "session-1", "user": "person@example.com"},
		"https://broken.example.com": {"token": 7},
		"https://empty.example.com": {"token": ""}
	}}`)

	login, found, err := client.StoredLogin("")
	if err != nil || !found {
		t.Fatalf("got %v, %v", found, err)
	}

	if want := (client.Login{Address: "https://app.usespotlight.ai", Token: "session-1", User: "person@example.com"}); login != want {
		t.Errorf("got %+v, want %+v", login, want)
	}
}

func TestWithNoURLSeveralLoginsAreNamed(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(client.ConfigDirEnv, dir)
	writeCredentials(t, dir, `{"logins": {
		"https://spotlight.example.com": {"token": "session-1", "user": "person@example.com"},
		"http://localhost:3000": {"token": "session-2", "user": "dev@example.com"}
	}}`)

	_, found, err := client.StoredLogin("")
	if err == nil || found {
		t.Fatalf("got %v, %v; want an error", found, err)
	}

	for _, want := range []string{"http://localhost:3000, https://spotlight.example.com", "url", "SPOTLIGHT_URL"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("%q does not contain %q", err, want)
		}
	}

	if strings.Contains(err.Error(), "session-") {
		t.Error("the error shows a token")
	}
}

func TestWithNoURLAndNoLoginThereIsNone(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(client.ConfigDirEnv, dir)

	if _, found, err := client.StoredLogin(""); err != nil || found {
		t.Errorf("with no file, got %v, %v", found, err)
	}

	writeCredentials(t, dir, `{"logins": {}, "other": true}`)
	if _, found, err := client.StoredLogin(""); err != nil || found {
		t.Errorf("with no logins, got %v, %v", found, err)
	}
}

func TestALoginThatDoesNotParseIsSkipped(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(client.ConfigDirEnv, dir)
	writeCredentials(t, dir, `{"logins": {
		"https://app.usespotlight.ai": "not a login",
		"https://spotlight.example.com": {"token": "session-1", "user": "person@example.com", "expires": "later"}
	}}`)

	if _, found, err := client.StoredLogin("https://app.usespotlight.ai"); err != nil || found {
		t.Errorf("used a login that does not parse: %v, %v", found, err)
	}

	if login, found, err := client.StoredLogin("https://spotlight.example.com"); err != nil || !found || login.Token != "session-1" {
		t.Errorf("did not use the login beside it: %v, %v", found, err)
	}
}

func TestAForbiddenCallSaysWhetherAPersonMadeIt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reply(w, http.StatusForbidden, "application/problem+json", map[string]any{
			"type": "urn:spotlight:problem:forbidden", "title": "Not allowed", "status": 403,
		})
	}))
	t.Cleanup(server.Close)

	login := client.Login{Address: server.URL, Token: "session-1", User: "person@example.com"}
	for _, each := range []struct {
		cfg    client.Config
		person bool
	}{
		{client.Config{URL: server.URL, Login: &login}, true},
		{client.Config{URL: server.URL, Token: "token-1"}, false},
	} {
		api, err := client.New(each.cfg)
		if err != nil {
			t.Fatal(err)
		}

		_, err = api.ReadWorkspace(context.Background(), "ws-1")
		problem, ok := client.ProblemOf(err)
		if !ok || !client.IsForbidden(problem) {
			t.Fatalf("got %v", err)
		}

		if problem.Person != each.person {
			t.Errorf("with a login %v, Person is %v", each.cfg.Login != nil, problem.Person)
		}
	}
}
