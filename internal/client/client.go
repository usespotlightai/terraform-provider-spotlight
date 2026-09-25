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

// Config says where the control plane is and who is calling it: a stored
// Token, or OIDC to exchange a CI workload's tokens for short-lived ones.
// Now tells the time, time.Now when nil.
type Config struct {
	URL             string
	Token           string
	OIDC            *OIDC
	ProviderVersion string
	HTTPClient      *http.Client
	Retries         int
	Backoff         time.Duration
	Now             func() time.Time
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

	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 60 * time.Second}
	}

	doer := &retrying{
		next:    httpClient,
		retries: max(cfg.Retries, 0),
		backoff: cfg.Backoff,
	}

	tokens, err := tokenSourceOf(cfg, base, doer)
	if err != nil {
		return nil, err
	}

	generated, err := apigen.NewClientWithResponses(
		base,
		apigen.WithHTTPClient(&authenticating{next: doer, tokens: tokens}),
		apigen.WithRequestEditorFn(identify(cfg.ProviderVersion)),
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

func tokenSourceOf(cfg Config, base string, doer apigen.HttpRequestDoer) (TokenSource, error) {
	switch {
	case cfg.Token != "" && cfg.OIDC != nil:
		return nil, errors.New("a Spotlight API token and an OIDC trust were both given; authenticate with one")
	case cfg.Token != "":
		return StaticToken(cfg.Token), nil
	case cfg.OIDC == nil:
		return nil, errors.New("a Spotlight API token, or an OIDC trust to exchange a workload token through, is needed")
	case cfg.OIDC.Trust == "" || cfg.OIDC.Workload == nil:
		return nil, errors.New("an OIDC trust needs its id and a source of workload tokens")
	}

	anonymous, err := apigen.NewClientWithResponses(base, apigen.WithHTTPClient(doer), apigen.WithRequestEditorFn(identify(cfg.ProviderVersion)))
	if err != nil {
		return nil, err
	}

	oidc := *cfg.OIDC
	if oidc.Audience == "" {
		oidc.Audience = DefaultAudience
	}

	now := cfg.Now
	if now == nil {
		now = time.Now
	}

	return &exchanging{oidc: oidc, api: anonymous, now: now}, nil
}

func identify(version string) apigen.RequestEditorFn {
	return func(_ context.Context, req *http.Request) error {
		req.Header.Set(clientHeader, ClientName(version))
		req.Header.Set("User-Agent", ClientName(version))
		req.Header.Set(apiVersionHeader, APIVersion)
		req.Header.Set("Accept", "application/json, application/problem+json")

		return nil
	}
}
