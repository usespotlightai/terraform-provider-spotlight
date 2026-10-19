package client_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/client"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/fakeapi"
)

type clock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.now
}

func (c *clock) pass(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.now = c.now.Add(d)
}

type issuing struct {
	recorder

	clock        *clock
	lifetime     time.Duration
	unauthorized map[string]bool
	accept       string
	jwts         int
}

func (i *issuing) serve(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	_ = json.NewDecoder(r.Body).Decode(&body)
	raw, _ := json.Marshal(body)

	i.mu.Lock()
	i.seen = append(i.seen, seen{r.Method, r.URL.Path, r.URL.RawQuery, r.Header.Clone(), string(raw)})
	i.mu.Unlock()

	switch r.URL.Path {
	case "/github/token":
		i.mu.Lock()
		i.jwts++
		jwt := fmt.Sprintf("github-jwt-%d", i.jwts)
		i.mu.Unlock()

		reply(w, http.StatusOK, "application/json", map[string]any{"count": 1, "value": jwt})
	case "/api/workspace-tokens/exchange":
		token, _ := body["token"].(string)
		if i.accept != "" && !strings.HasPrefix(token, i.accept) {
			reply(w, http.StatusUnauthorized, "application/problem+json", map[string]any{
				"type": "urn:spotlight:problem:unauthenticated", "title": "Unauthorized", "status": 401, "detail": "The workload is not accepted",
			})
			return
		}

		i.mu.Lock()
		issuedToken := fmt.Sprintf("issued-%d", len(i.exchanges()))
		i.mu.Unlock()

		reply(w, http.StatusOK, "application/json", map[string]any{
			"token": issuedToken, "tokenId": "token-1", "workspaceId": "ws-1", "scopes": []any{"plan"},
			"expiresAt": i.clock.Now().Add(i.lifetime).UTC().Format("2006-01-02T15:04:05.000Z"),
		})
	default:
		i.mu.Lock()
		unauthorized := i.unauthorized[strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")]
		i.mu.Unlock()

		if unauthorized {
			reply(w, http.StatusUnauthorized, "application/problem+json", map[string]any{
				"type": "urn:spotlight:problem:unauthenticated", "title": "No valid credentials", "status": 401, "detail": "Send a Spotlight API token as a bearer token",
			})
			return
		}

		if r.Method == http.MethodPost {
			reply(w, http.StatusCreated, "application/json", workspace)
			return
		}

		reply(w, http.StatusOK, "application/json", workspace)
	}
}

func (i *issuing) exchanges() []seen {
	var out []seen
	for _, each := range i.seen {
		if each.Path == "/api/workspace-tokens/exchange" {
			out = append(out, each)
		}
	}

	return out
}

func (i *issuing) exchanged() []seen {
	i.mu.Lock()
	defer i.mu.Unlock()

	return i.exchanges()
}

func (i *issuing) at(path string) []seen {
	var out []seen
	for _, each := range i.all() {
		if each.Path == path {
			out = append(out, each)
		}
	}

	return out
}

func (i *issuing) unauthorize(tokens ...string) {
	i.mu.Lock()
	defer i.mu.Unlock()

	for _, each := range tokens {
		i.unauthorized[each] = true
	}
}

func exchangingClient(t *testing.T, workload func(serverURL string) client.WorkloadTokens) (*client.Client, *issuing) {
	t.Helper()

	issued := &issuing{clock: &clock{now: time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC)}, lifetime: 15 * time.Minute, unauthorized: map[string]bool{}}
	server := httptest.NewServer(http.HandlerFunc(issued.serve))
	t.Cleanup(server.Close)

	api, err := client.New(client.Config{
		URL:             server.URL,
		OIDC:            &client.OIDC{Trust: "trust-1", Audience: "spotlight-ci", Workload: workload(server.URL)},
		ProviderVersion: "1.2.3",
		Now:             issued.clock.Now,
	})
	if err != nil {
		t.Fatal(err)
	}

	return api, issued
}

func fromGitHub(serverURL string) client.WorkloadTokens {
	return client.GitHubActions{RequestURL: serverURL + "/github/token?api-version=2.0", RequestToken: "github-request-token"}
}

func supplied(string) client.WorkloadTokens {
	return client.SuppliedWorkloadToken("gitlab-jwt")
}

func TestATokenForTheAudienceIsRequestedFromGitHubActions(t *testing.T) {
	api, issued := exchangingClient(t, fromGitHub)

	if _, err := api.ReadWorkspace(context.Background(), "ws-1"); err != nil {
		t.Fatal(err)
	}

	requests := issued.at("/github/token")
	if len(requests) != 1 {
		t.Fatalf("GitHub received %d requests, want 1", len(requests))
	}

	if requests[0].Method != http.MethodGet || requests[0].Query != "api-version=2.0&audience=spotlight-ci" {
		t.Errorf("requested from GitHub %s ?%s", requests[0].Method, requests[0].Query)
	}

	if got := requests[0].Headers.Get("Authorization"); got != "Bearer github-request-token" {
		t.Errorf("requested from GitHub with Authorization %q", got)
	}
}

func TestTheWorkloadTokenIsExchangedThroughTheTrust(t *testing.T) {
	api, issued := exchangingClient(t, fromGitHub)

	if _, err := api.ReadWorkspace(context.Background(), "ws-1"); err != nil {
		t.Fatal(err)
	}

	exchanged := issued.exchanged()
	if len(exchanged) != 1 {
		t.Fatalf("%d exchanges, want 1", len(exchanged))
	}

	var sent map[string]string
	_ = json.Unmarshal([]byte(exchanged[0].Body), &sent)
	if sent["trust"] != "trust-1" || sent["token"] != "github-jwt-1" || len(sent) != 2 {
		t.Errorf("exchanged %v", sent)
	}

	if got := exchanged[0].Headers.Get("Spotlight-Client"); got != "terraform-provider/1.2.3" {
		t.Errorf("exchanged with Spotlight-Client %q", got)
	}

	if got := exchanged[0].Headers.Get("Authorization"); got != "" {
		t.Errorf("exchanged with Authorization %q, want none", got)
	}

	read := issued.at("/api/resources/workspaces/ws-1")
	if got := read[0].Headers.Get("Authorization"); got != "Bearer issued-1" {
		t.Errorf("read with Authorization %q, want the issued token", got)
	}
}

func TestASuppliedWorkloadTokenIsExchangedAsItIs(t *testing.T) {
	api, issued := exchangingClient(t, supplied)

	if _, err := api.ReadWorkspace(context.Background(), "ws-1"); err != nil {
		t.Fatal(err)
	}

	if requests := issued.at("/github/token"); len(requests) != 0 {
		t.Errorf("GitHub received %d requests, want none", len(requests))
	}

	var sent map[string]string
	_ = json.Unmarshal([]byte(issued.exchanged()[0].Body), &sent)
	if sent["token"] != "gitlab-jwt" {
		t.Errorf("exchanged %v", sent)
	}
}

func TestAnIssuedTokenIsReusedUntilAMinuteBeforeItExpires(t *testing.T) {
	api, issued := exchangingClient(t, fromGitHub)
	ctx := context.Background()

	for range 3 {
		if _, err := api.ReadWorkspace(ctx, "ws-1"); err != nil {
			t.Fatal(err)
		}
	}

	issued.clock.pass(13*time.Minute + 59*time.Second)
	if _, err := api.ReadWorkspace(ctx, "ws-1"); err != nil {
		t.Fatal(err)
	}

	if exchanged := len(issued.exchanged()); exchanged != 1 {
		t.Fatalf("%d exchanges before renewal was due, want 1", exchanged)
	}

	issued.clock.pass(2 * time.Second)
	if _, err := api.ReadWorkspace(ctx, "ws-1"); err != nil {
		t.Fatal(err)
	}

	if exchanged := len(issued.exchanged()); exchanged != 2 {
		t.Fatalf("%d exchanges once renewal was due, want 2", exchanged)
	}

	if requests := len(issued.at("/github/token")); requests != 2 {
		t.Errorf("GitHub received %d requests, want a fresh token for each exchange", requests)
	}

	reads := issued.at("/api/resources/workspaces/ws-1")
	if got := reads[len(reads)-1].Headers.Get("Authorization"); got != "Bearer issued-2" {
		t.Errorf("read with Authorization %q, want the renewed token", got)
	}
}

func TestAShortLivedTokenIsRenewedHalfwayThroughItsLife(t *testing.T) {
	api, issued := exchangingClient(t, fromGitHub)
	issued.lifetime = time.Minute
	ctx := context.Background()

	for _, step := range []time.Duration{0, 29 * time.Second, 2 * time.Second} {
		issued.clock.pass(step)
		if _, err := api.ReadWorkspace(ctx, "ws-1"); err != nil {
			t.Fatal(err)
		}
	}

	if exchanged := len(issued.exchanged()); exchanged != 2 {
		t.Errorf("%d exchanges, want one at the start and one halfway", exchanged)
	}
}

func TestAnUnauthorizedIssuedTokenIsRenewedAndTheCallRepeated(t *testing.T) {
	api, issued := exchangingClient(t, fromGitHub)
	issued.unauthorize("issued-1")

	created, err := api.CreateWorkspace(context.Background(), client.WorkspaceInput{Name: "Acme", Slug: "acme"})
	if err != nil {
		t.Fatal(err)
	}

	if created.ID != "ws-1" {
		t.Errorf("created %+v", created)
	}

	creates := issued.at("/api/resources/workspaces")
	if len(creates) != 2 {
		t.Fatalf("%d attempts, want 2", len(creates))
	}

	if creates[0].Headers.Get("Authorization") != "Bearer issued-1" || creates[1].Headers.Get("Authorization") != "Bearer issued-2" {
		t.Errorf("attempted with %q then %q", creates[0].Headers.Get("Authorization"), creates[1].Headers.Get("Authorization"))
	}

	if creates[1].Body != creates[0].Body || creates[1].Headers.Get("Idempotency-Key") != creates[0].Headers.Get("Idempotency-Key") {
		t.Errorf("repeated %q with key %q, first %q with key %q",
			creates[1].Body, creates[1].Headers.Get("Idempotency-Key"), creates[0].Body, creates[0].Headers.Get("Idempotency-Key"))
	}
}

func TestATokenUnauthorizedAgainIsNotRenewedForever(t *testing.T) {
	api, issued := exchangingClient(t, fromGitHub)
	issued.unauthorize("issued-1", "issued-2", "issued-3")

	_, err := api.ReadWorkspace(context.Background(), "ws-1")

	problem, ok := client.ProblemOf(err)
	if !ok || problem.Status != http.StatusUnauthorized {
		t.Fatalf("expected a 401 problem, got %v", err)
	}

	if exchanged := len(issued.exchanged()); exchanged != 2 {
		t.Errorf("%d exchanges, want 2", exchanged)
	}
}

func TestAnUnauthorizedStoredTokenIsNotRetried(t *testing.T) {
	api, recorded := serve(t, func(w http.ResponseWriter, _ *http.Request, _ int) {
		reply(w, http.StatusUnauthorized, "application/problem+json", map[string]any{
			"type": "urn:spotlight:problem:unauthenticated", "title": "No valid credentials", "status": 401, "detail": "Send a Spotlight API token as a bearer token",
		})
	})

	if _, err := api.ReadWorkspace(context.Background(), "ws-1"); err == nil {
		t.Fatal("expected the read to fail")
	}

	if attempts := len(recorded.all()); attempts != 1 {
		t.Errorf("%d attempts, want 1", attempts)
	}
}

func TestAFailedExchangeNamesTheTrust(t *testing.T) {
	api, issued := exchangingClient(t, fromGitHub)
	issued.accept = "nothing"

	_, err := api.ReadWorkspace(context.Background(), "ws-1")
	if err == nil || !strings.Contains(err.Error(), "workspace token trust trust-1 did not accept the OIDC token") || !strings.Contains(err.Error(), "The workload is not accepted") {
		t.Fatalf("got %v", err)
	}

	if reads := issued.at("/api/resources/workspaces/ws-1"); len(reads) != 0 {
		t.Errorf("read %d times without a token", len(reads))
	}
}

func TestGitHubActionsErrorSaysWhatTheJobNeeds(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "Unable to get ACTIONS_ID_TOKEN_REQUEST_URL env variable", http.StatusForbidden)
	}))
	defer server.Close()

	_, err := client.GitHubActions{RequestURL: server.URL + "/token", RequestToken: "t"}.WorkloadToken(context.Background(), "spotlight")
	if err == nil || !strings.Contains(err.Error(), "id-token: write") || !strings.Contains(err.Error(), "403") {
		t.Errorf("got %v", err)
	}
}

func TestOneExchangeServesCallsMadeTogether(t *testing.T) {
	api, issued := exchangingClient(t, fromGitHub)

	var wg sync.WaitGroup
	failures := make(chan error, 20)
	for range 20 {
		wg.Go(func() {
			if _, err := api.ReadWorkspace(context.Background(), "ws-1"); err != nil {
				failures <- err
			}
		})
	}
	wg.Wait()
	close(failures)

	for err := range failures {
		t.Error(err)
	}

	if exchanged := len(issued.exchanged()); exchanged != 1 {
		t.Errorf("%d exchanges, want 1", exchanged)
	}
}

func TestATokenAndATrustAreNotBothUsed(t *testing.T) {
	_, err := client.New(client.Config{
		URL:   "https://spotlight.example.com",
		Token: "token",
		OIDC:  &client.OIDC{Trust: "trust-1", Workload: client.SuppliedWorkloadToken("jwt")},
	})
	if err == nil {
		t.Error("expected an error")
	}
}

func TestAChangeToTheTrustMidApplyIsRecoveredFrom(t *testing.T) {
	fake := fakeapi.New(t)
	ctx := context.Background()

	admin, err := client.New(client.Config{URL: fake.URL, Token: fakeapi.Token, ProviderVersion: "1.2.3"})
	if err != nil {
		t.Fatal(err)
	}

	workspace, err := admin.CreateWorkspace(ctx, client.WorkspaceInput{Name: "Acme", Slug: "acme"})
	if err != nil {
		t.Fatal(err)
	}

	in := client.WorkspaceTokenTrustInput{
		Key: "github-main", Issuer: "https://token.actions.githubusercontent.com", Audience: "spotlight",
		Subject: "repo:acme/platform:ref:refs/heads/main", Claims: map[string]string{}, Scopes: []string{"configure"}, LifetimeMinutes: 15,
	}
	trust, err := admin.CreateWorkspaceTokenTrust(ctx, workspace.ID, in)
	if err != nil {
		t.Fatal(err)
	}

	ci, err := client.New(client.Config{
		URL:             fake.URL,
		OIDC:            &client.OIDC{Trust: trust.ID, Workload: client.SuppliedWorkloadToken(fakeapi.WorkloadToken)},
		ProviderVersion: "1.2.3",
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := ci.CreateTeam(ctx, workspace.ID, client.TeamInput{Key: "core", Name: "Core"}); err != nil {
		t.Fatal(err)
	}

	in.Scopes = []string{"configure", "secrets"}
	if _, err := admin.UpdateWorkspaceTokenTrust(ctx, trust.ID, trust.Version, in); err != nil {
		t.Fatal(err)
	}

	if _, err := ci.CreateTeam(ctx, workspace.ID, client.TeamInput{Key: "platform", Name: "Platform"}); err != nil {
		t.Fatalf("the call after the trust changed failed: %v", err)
	}

	var exchanges, creates []fakeapi.Request
	for _, each := range fake.Requests() {
		switch {
		case each.Path == "/api/workspace-tokens/exchange":
			exchanges = append(exchanges, each)
		case each.Path == "/api/resources/workspaces/"+workspace.ID+"/teams" && each.Body["key"] == "platform":
			creates = append(creates, each)
		}
	}

	if len(exchanges) != 2 {
		t.Errorf("%d exchanges, want one at the start and one after the trust changed", len(exchanges))
	}

	if len(creates) != 2 || creates[0].Headers.Get("Authorization") == creates[1].Headers.Get("Authorization") ||
		creates[0].Headers.Get("Idempotency-Key") != creates[1].Headers.Get("Idempotency-Key") {
		t.Errorf("the unauthorized create was not repeated once with a new token and the same key: %d attempts", len(creates))
	}

	if teams := fake.Count("team"); teams != 2 {
		t.Errorf("%d teams, want 2", teams)
	}
}
