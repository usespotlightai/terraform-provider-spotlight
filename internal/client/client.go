// Package client adapts the generated Spotlight API client to the
// provider's port.
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
	"github.com/usespotlightai/terraform-provider-spotlight/internal/spotlight"
)

// APIVersion is the Spotlight API version this client is written against.
const APIVersion = "2026-09-23"

const (
	apiVersionHeader = "Spotlight-Api-Version"
	clientHeader     = "Spotlight-Client"
	tokenPrefix      = "spt_"
)

// DefaultBackoff is how long the first retry waits; each after waits twice
// as long.
const DefaultBackoff = 500 * time.Millisecond

// Config says where the control plane is and who is calling it.
type Config struct {
	Endpoint        string
	Token           string
	ProviderVersion string
	HTTPClient      *http.Client
	Retries         int
	Backoff         time.Duration
}

// New builds the port onto the control plane at cfg.Endpoint.
func New(cfg Config) (spotlight.API, error) {
	base, err := baseURL(cfg.Endpoint)
	if err != nil {
		return spotlight.API{}, err
	}

	if !strings.HasPrefix(cfg.Token, tokenPrefix) {
		return spotlight.API{}, fmt.Errorf("the token is not a Spotlight API token, which starts with %s", tokenPrefix)
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
		return spotlight.API{}, err
	}

	return spotlight.API{
		Workspaces:   workspaces{generated},
		Teams:        teams{generated},
		Lines:        lines{generated},
		Skills:       skills{generated},
		Integrations: integrations{generated},
		Expressions:  expressions{generated},
	}, nil
}

// ClientName is what the provider calls itself in Spotlight-Client.
func ClientName(version string) string {
	if version == "" {
		version = "dev"
	}

	return "terraform-provider/" + version
}

func baseURL(endpoint string) (string, error) {
	endpoint = strings.TrimRight(strings.TrimSpace(endpoint), "/")
	if endpoint == "" {
		return "", errors.New("no endpoint: set endpoint or SPOTLIGHT_URL to the control plane's URL")
	}

	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("the endpoint %q is not a URL, as in https://spotlight.example.com", endpoint)
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
