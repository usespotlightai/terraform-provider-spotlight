package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// WorkloadTokens answers the OIDC tokens a CI platform issues the job it
// runs.
type WorkloadTokens interface {
	WorkloadToken(ctx context.Context, audience string) (string, error)
}

// GivenWorkloadToken is an OIDC token handed to the provider, as other CI
// platforms than GitHub Actions do through the environment.
type GivenWorkloadToken string

// WorkloadToken answers the token, whatever audience is asked for.
func (t GivenWorkloadToken) WorkloadToken(context.Context, string) (string, error) {
	return string(t), nil
}

// GitHubActions asks GitHub Actions for an OIDC token issued to the job,
// as a job granted id-token: write may.
type GitHubActions struct {
	RequestURL   string
	RequestToken string
	HTTPClient   *http.Client
}

// WorkloadToken requests a fresh OIDC token for audience.
func (g GitHubActions) WorkloadToken(ctx context.Context, audience string) (string, error) {
	asked, err := url.Parse(g.RequestURL)
	if err != nil {
		return "", fmt.Errorf("ACTIONS_ID_TOKEN_REQUEST_URL is not a URL: %w", err)
	}

	query := asked.Query()
	query.Set("audience", audience)
	asked.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, asked.String(), nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", "Bearer "+g.RequestToken)
	req.Header.Set("Accept", "application/json")

	httpClient := g.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("GitHub Actions did not answer the OIDC token request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub Actions refused the OIDC token request with %d: %s; the job needs permissions: id-token: write",
			resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var answered struct {
		Value string `json:"value"`
	}
	if err := json.Unmarshal(body, &answered); err != nil {
		return "", fmt.Errorf("GitHub Actions answered an OIDC token this client cannot read: %w", err)
	}

	if answered.Value == "" {
		return "", errors.New("GitHub Actions answered no OIDC token")
	}

	return answered.Value, nil
}
