package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/url"
	"os"
	"path/filepath"
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

type credentials struct {
	Logins map[string]struct {
		Token string `json:"token"`
		User  string `json:"user"`
	} `json:"logins"`
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
// It reports false when there is no login for that address.
func StoredLogin(raw string) (Login, bool, error) {
	address, err := Address(raw)
	if err != nil {
		return Login{}, false, err
	}

	path, err := CredentialsPath()
	if err != nil {
		return Login{}, false, err
	}

	content, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Login{}, false, nil
	}

	if err != nil {
		return Login{}, false, fmt.Errorf("could not read %s: %w", path, err)
	}

	var stored credentials
	if err := json.Unmarshal(content, &stored); err != nil {
		return Login{}, false, fmt.Errorf("%s is not a Spotlight credentials file: %v; run spotlight login to write it again", path, err)
	}

	entry, ok := stored.Logins[address]
	if !ok || entry.Token == "" || !secure(address) {
		return Login{}, false, nil
	}

	return Login{Address: address, Token: entry.Token, User: entry.User}, true, nil
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
	if s.login.User == "" {
		return fmt.Errorf("the control plane did not accept the login for %s; run spotlight login to sign in again", s.login.Address)
	}

	return fmt.Errorf("the control plane did not accept the login of %s for %s; run spotlight login to sign in again", s.login.User, s.login.Address)
}
