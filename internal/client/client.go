// Package client is the Spotlight control plane's API client: the resources
// the provider manages, the problems the API answers, and the calls it makes
// through the client generated from the API's OpenAPI schema.
package client

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/client/apigen"
)

// APIVersion is the Spotlight API version this client is written against.
const APIVersion = "2026-09-23"

const (
	apiVersionHeader = "Spotlight-Api-Version"
	clientHeader     = "Spotlight-Client"
)

// DefaultBackoff is how long the first retry waits; each after waits twice
// as long.
const DefaultBackoff = 500 * time.Millisecond

// Config says where the control plane is and who is calling it.
type Config struct {
	URL             string
	Token           string
	ProviderVersion string
	HTTPClient      *http.Client
	Retries         int
	Backoff         time.Duration
}

// Client calls the control plane.
type Client struct {
	api *apigen.ClientWithResponses
}

// New returns a client for the control plane at cfg.URL.
func New(cfg Config) (*Client, error) {
	base, err := baseURL(cfg.URL)
	if err != nil {
		return nil, err
	}

	if cfg.Token == "" {
		return nil, errors.New("a Spotlight API token is needed")
	}

	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 60 * time.Second}
	}

	doer := &retrying{
		next:    httpClient,
		retries: max(cfg.Retries, 0),
		backoff: cfg.Backoff,
	}

	generated, err := apigen.NewClientWithResponses(
		base,
		apigen.WithHTTPClient(doer),
		apigen.WithRequestEditorFn(identify(cfg.Token, cfg.ProviderVersion)),
	)
	if err != nil {
		return nil, err
	}

	return &Client{api: generated}, nil
}

// ClientName is what the provider calls itself in Spotlight-Client.
func ClientName(version string) string {
	if version == "" {
		version = "dev"
	}

	return "terraform-provider/" + version
}

func baseURL(held string) (string, error) {
	held = strings.TrimRight(strings.TrimSpace(held), "/")
	if held == "" {
		return "", errors.New("no URL: set url or SPOTLIGHT_URL to the control plane's URL")
	}

	parsed, err := url.Parse(held)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("the url %q is not a URL, as in https://spotlight.example.com", held)
	}

	if !strings.HasSuffix(parsed.Path, "/api") {
		parsed.Path += "/api"
	}

	return parsed.String() + "/", nil
}

func identify(token, version string) apigen.RequestEditorFn {
	return func(_ context.Context, req *http.Request) error {
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set(clientHeader, ClientName(version))
		req.Header.Set("User-Agent", ClientName(version))
		req.Header.Set(apiVersionHeader, APIVersion)
		req.Header.Set("Accept", "application/json, application/problem+json")

		return nil
	}
}
