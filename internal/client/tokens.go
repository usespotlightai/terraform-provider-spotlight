package client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/client/apigen"
)

// RenewBefore is how long before a minted token expires the client mints
// the next one; a token minted for less than twice as long is renewed
// halfway through its life.
const RenewBefore = time.Minute

// TokenSource answers the bearer token each call is made with.
type TokenSource interface {
	// Token answers the token to call with now.
	Token(ctx context.Context) (string, error)
	// Refused is told that the control plane answered token 401, and
	// reports whether Token may now answer a different one.
	Refused(token string) bool
}

// StaticToken is a token source that always answers the one token, as a
// stored workspace token is.
type StaticToken string

// Token answers the token.
func (t StaticToken) Token(context.Context) (string, error) {
	if t == "" {
		return "", errors.New("a Spotlight API token is needed")
	}

	return string(t), nil
}

// Refused reports false: there is no other token to try.
func (t StaticToken) Refused(string) bool {
	return false
}

// OIDC says how to authenticate through a workspace token trust: the
// trust's id, the audience its workload tokens are issued for, and where
// those tokens come from.
type OIDC struct {
	Trust    string
	Audience string
	Workload WorkloadTokens
}

// DefaultAudience is the audience a workload token is issued for unless a
// trust names another.
const DefaultAudience = "spotlight"

type minted struct {
	token     string
	expiresAt time.Time
}

type exchanging struct {
	oidc OIDC
	api  *apigen.ClientWithResponses
	now  func() time.Time

	mu      sync.Mutex
	held    string
	renewAt time.Time
}

func (e *exchanging) Token(ctx context.Context) (string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.held != "" && e.now().Before(e.renewAt) {
		return e.held, nil
	}

	fresh, err := e.exchange(ctx)
	if err != nil {
		return "", err
	}

	e.held = fresh.token
	e.renewAt = renewalOf(fresh.expiresAt, e.now())

	return e.held, nil
}

func renewalOf(expiresAt, now time.Time) time.Time {
	if remaining := expiresAt.Sub(now); remaining < 2*RenewBefore {
		return now.Add(remaining / 2)
	}

	return expiresAt.Add(-RenewBefore)
}

func (e *exchanging) Refused(token string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.held == token {
		e.held = ""
	}

	return true
}

func (e *exchanging) exchange(ctx context.Context) (minted, error) {
	workload, err := e.oidc.Workload.WorkloadToken(ctx, e.oidc.Audience)
	if err != nil {
		return minted{}, fmt.Errorf("could not get an OIDC token for the audience %s: %w", e.oidc.Audience, err)
	}

	body, err := jsonBody(apigen.ExchangeWorkloadTokenRequestDto{Trust: e.oidc.Trust, Token: workload})
	if err != nil {
		return minted{}, err
	}

	resp, err := e.api.ExchangeWorkloadTokenWithBodyWithResponse(ctx, jsonContent, body)
	if err != nil {
		return minted{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON200)
	if err != nil {
		return minted{}, fmt.Errorf("the workspace token trust %s did not admit the OIDC token: %v", e.oidc.Trust, err)
	}

	expiresAt, err := time.Parse(time.RFC3339, dto.ExpiresAt)
	if err != nil {
		return minted{}, fmt.Errorf("the minted token's expiry %q is not a time: %w", dto.ExpiresAt, err)
	}

	return minted{token: dto.Token, expiresAt: expiresAt}, nil
}

type authenticating struct {
	next   apigen.HttpRequestDoer
	tokens TokenSource
}

func (a *authenticating) Do(req *http.Request) (*http.Response, error) {
	token, err := a.tokens.Token(req.Context())
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := a.next.Do(req)
	if err != nil || resp.StatusCode != http.StatusUnauthorized || !a.tokens.Refused(token) {
		return resp, err
	}

	renewed, err := a.tokens.Token(req.Context())
	if err == nil && renewed == token {
		return resp, nil
	}

	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()

	if err != nil {
		return nil, err
	}

	if err := rewind(req); err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+renewed)

	return a.next.Do(req)
}
