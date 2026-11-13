package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Environment variables that tell where spotlight login keeps its logins.
const (
	ConfigDirEnv     = "SPOTLIGHT_CONFIG_DIR"
	XDGConfigHomeEnv = "XDG_CONFIG_HOME"
)

const credentialsFile = "credentials.json"

var defaultPorts = map[string]string{"http": "80", "https": "443"}

// Login is the sign-in of a person that spotlight login keeps for a Spotlight address.
type Login struct {
	Address string
	Token   string
	User    string
}

// CredentialsPath returns the path of the file where spotlight login keeps its logins.
func CredentialsPath() (string, error) {
	if dir := os.Getenv(ConfigDirEnv); dir != "" {
		return filepath.Join(dir, credentialsFile), nil
	}

	if dir := os.Getenv(XDGConfigHomeEnv); dir != "" {
		return filepath.Join(dir, "spotlight", credentialsFile), nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("could not find the home directory that contains the Spotlight login: %w", err)
	}

	return filepath.Join(home, ".config", "spotlight", credentialsFile), nil
}

// Address returns the Spotlight address of a URL: its scheme and host, and its port when the port is not the default.
func Address(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Hostname() == "" {
		return "", fmt.Errorf("the url %q is not a URL, as in https://spotlight.example.com", raw)
	}

	scheme := strings.ToLower(parsed.Scheme)
	host := strings.ToLower(parsed.Hostname())

	switch port := parsed.Port(); {
	case port != "" && port != defaultPorts[scheme]:
		host = net.JoinHostPort(host, port)
	case strings.Contains(host, ":"):
		host = "[" + host + "]"
	}

	return scheme + "://" + host, nil
}

// StoredLogin returns the login that spotlight login keeps for the Spotlight at a URL.
// With no URL, it returns the only login there is, and fails when there are several.
// It reports false when there is no login to use.
func StoredLogin(raw string) (Login, bool, error) {
	logins, err := storedLogins()
	if err != nil {
		return Login{}, false, err
	}

	if raw == "" {
		return onlyLogin(logins)
	}

	address, err := Address(raw)
	if err != nil {
		return Login{}, false, err
	}

	login, ok := logins[address]
	if !ok || !secure(address) {
		return Login{}, false, nil
	}

	return login, true, nil
}

func onlyLogin(logins map[string]Login) (Login, bool, error) {
	var addresses []string
	for address := range logins {
		if secure(address) {
			addresses = append(addresses, address)
		}
	}

	slices.Sort(addresses)

	switch len(addresses) {
	case 0:
		return Login{}, false, nil
	case 1:
		return logins[addresses[0]], true, nil
	default:
		return Login{}, false, fmt.Errorf("there are logins for %s; set url, or SPOTLIGHT_URL, to the one to use", strings.Join(addresses, ", "))
	}
}

func storedLogins() (map[string]Login, error) {
	path, err := CredentialsPath()
	if err != nil {
		return nil, err
	}

	content, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("could not read %s: %w", path, err)
	}

	var stored struct {
		Logins map[string]json.RawMessage `json:"logins"`
	}
	if err := json.Unmarshal(content, &stored); err != nil {
		return nil, fmt.Errorf("%s is not a Spotlight credentials file: %v; run spotlight login to write it again", path, err)
	}

	logins := map[string]Login{}
	for address, raw := range stored.Logins {
		var entry struct {
			Token string `json:"token"`
			User  string `json:"user"`
		}
		if json.Unmarshal(raw, &entry) != nil || entry.Token == "" {
			continue
		}

		logins[address] = Login{Address: address, Token: entry.Token, User: entry.User}
	}

	return logins, nil
}

func secure(address string) bool {
	parsed, err := url.Parse(address)
	if err != nil {
		return false
	}

	host := parsed.Hostname()
	switch parsed.Scheme {
	case "https":
		return true
	case "http":
		return host == "localhost" || host == "127.0.0.1" || host == "::1" ||
			strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".test")
	default:
		return false
	}
}

type signedIn struct {
	login Login
}

func (s signedIn) Token(context.Context) (string, error) {
	return s.login.Token, nil
}

func (s signedIn) Unauthorized(string) bool {
	return false
}

func (s signedIn) rejected() error {
	whose := "the login"
	if s.login.User != "" {
		whose = "the login of " + s.login.User
	}

	return fmt.Errorf("%s for %s expired or was revoked; run spotlight login --url %s to sign in again", whose, s.login.Address, s.login.Address)
}

type personKey struct{}

func asPerson(req *http.Request) *http.Request {
	return req.WithContext(context.WithValue(req.Context(), personKey{}, true))
}

func byPerson(req *http.Request) bool {
	if req == nil {
		return false
	}

	person, _ := req.Context().Value(personKey{}).(bool)

	return person
}
