package client_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/client"
)

type seen struct {
	Method  string
	Path    string
	Query   string
	Headers http.Header
	Body    string
}

type recorder struct {
	mu   sync.Mutex
	seen []seen
}

func (r *recorder) all() []seen {
	r.mu.Lock()
	defer r.mu.Unlock()

	return append([]seen(nil), r.seen...)
}

func serve(t *testing.T, answer func(w http.ResponseWriter, r *http.Request, attempt int)) (*client.Client, *recorder) {
	t.Helper()

	recorded := &recorder{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)

		recorded.mu.Lock()
		recorded.seen = append(recorded.seen, seen{r.Method, r.URL.Path, r.URL.RawQuery, r.Header.Clone(), string(body)})
		attempt := len(recorded.seen)
		recorded.mu.Unlock()

		answer(w, r, attempt)
	}))
	t.Cleanup(server.Close)

	api, err := client.New(client.Config{URL: server.URL, Token: "secret-token", ProviderVersion: "1.2.3", Retries: 2})
	if err != nil {
		t.Fatal(err)
	}

	return api, recorded
}

func reply(w http.ResponseWriter, status int, contentType string, body any) {
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

var workspace = map[string]any{
	"id": "ws-1", "version": 3, "address": "acme", "warnings": []any{}, "name": "Acme", "slug": "acme",
}

func TestEveryRequestSaysWhoIsCalling(t *testing.T) {
	api, recorded := serve(t, func(w http.ResponseWriter, _ *http.Request, _ int) {
		reply(w, http.StatusOK, "application/json", workspace)
	})

	if _, err := api.ReadWorkspace(context.Background(), "ws-1"); err != nil {
		t.Fatal(err)
	}

	got := recorded.all()[0]
	if got.Path != "/api/resources/workspaces/ws-1" {
		t.Errorf("path %q", got.Path)
	}

	for header, want := range map[string]string{
		"Authorization":         "Bearer secret-token",
		"Spotlight-Client":      "terraform-provider/1.2.3",
		"Spotlight-Api-Version": client.APIVersion,
	} {
		if value := got.Headers.Get(header); value != want {
			t.Errorf("%s = %q, want %q", header, value, want)
		}
	}
}

func TestCreateCarriesOneIdempotencyKeyAcrossRetries(t *testing.T) {
	api, recorded := serve(t, func(w http.ResponseWriter, _ *http.Request, attempt int) {
		if attempt == 1 {
			reply(w, http.StatusServiceUnavailable, "application/problem+json", map[string]any{"type": "urn:spotlight:problem:internal-error", "title": "Down", "status": 503, "detail": "down"})
			return
		}

		reply(w, http.StatusCreated, "application/json", workspace)
	})

	created, err := api.CreateWorkspace(context.Background(), client.WorkspaceInput{Name: "Acme", Slug: "acme"})
	if err != nil {
		t.Fatal(err)
	}

	if created.ID != "ws-1" || created.Version != 3 || created.Address != "acme" {
		t.Errorf("created %+v", created)
	}

	attempts := recorded.all()
	if len(attempts) != 2 {
		t.Fatalf("%d attempts, want 2", len(attempts))
	}

	first, second := attempts[0].Headers.Get("Idempotency-Key"), attempts[1].Headers.Get("Idempotency-Key")
	if first == "" || first != second {
		t.Errorf("idempotency keys %q and %q, want one key repeated", first, second)
	}

	if attempts[1].Body != attempts[0].Body || !strings.Contains(attempts[1].Body, `"slug":"acme"`) {
		t.Errorf("retried body %q, first %q", attempts[1].Body, attempts[0].Body)
	}
}

func TestTwoCreatesUseTwoKeys(t *testing.T) {
	api, recorded := serve(t, func(w http.ResponseWriter, _ *http.Request, _ int) {
		reply(w, http.StatusCreated, "application/json", workspace)
	})

	for range 2 {
		if _, err := api.CreateWorkspace(context.Background(), client.WorkspaceInput{Name: "Acme", Slug: "acme"}); err != nil {
			t.Fatal(err)
		}
	}

	attempts := recorded.all()
	if attempts[0].Headers.Get("Idempotency-Key") == attempts[1].Headers.Get("Idempotency-Key") {
		t.Error("two creates shared an idempotency key")
	}
}

func TestUpdateIsConditionalOnTheVersionHeld(t *testing.T) {
	api, recorded := serve(t, func(w http.ResponseWriter, _ *http.Request, _ int) {
		reply(w, http.StatusOK, "application/json", map[string]any{
			"id": "team-1", "version": 5, "address": "core", "warnings": []any{},
			"workspaceId": "ws-1", "key": "core", "name": "Core",
		})
	})

	updated, err := api.UpdateTeam(context.Background(), "team-1", 4, client.TeamInput{Key: "core", Name: "Core"})
	if err != nil {
		t.Fatal(err)
	}

	got := recorded.all()[0]
	if got.Method != http.MethodPut || got.Headers.Get("If-Match") != `"4"` {
		t.Errorf("%s with If-Match %q", got.Method, got.Headers.Get("If-Match"))
	}

	if updated.Version != 5 || updated.WorkspaceID != "ws-1" {
		t.Errorf("updated %+v", updated)
	}
}

func TestAnUpdateIsNotRetried(t *testing.T) {
	api, recorded := serve(t, func(w http.ResponseWriter, _ *http.Request, _ int) {
		reply(w, http.StatusServiceUnavailable, "application/problem+json", map[string]any{"type": "urn:spotlight:problem:internal-error", "title": "Down", "status": 503, "detail": "down"})
	})

	_, err := api.UpdateTeam(context.Background(), "team-1", 4, client.TeamInput{Key: "core", Name: "Core"})
	if err == nil {
		t.Fatal("expected the update to fail")
	}

	if attempts := len(recorded.all()); attempts != 1 {
		t.Errorf("%d attempts, want 1", attempts)
	}
}

func TestImportAsksByAddress(t *testing.T) {
	api, recorded := serve(t, func(w http.ResponseWriter, _ *http.Request, _ int) {
		reply(w, http.StatusOK, "application/json", map[string]any{
			"id": "line-1", "version": 1, "address": "core/delivery", "warnings": []any{},
			"teamId": "team-1", "key": "delivery", "definition": map[string]any{"key": "delivery", "name": "Delivery"},
		})
	})

	found, err := api.ImportLine(context.Background(), "ws-1", "core/delivery")
	if err != nil {
		t.Fatal(err)
	}

	got := recorded.all()[0]
	if got.Path != "/api/resources/workspaces/ws-1/lines/import" || got.Query != "address=core%2Fdelivery" {
		t.Errorf("asked %s?%s", got.Path, got.Query)
	}

	if found.Definition["name"] != "Delivery" {
		t.Errorf("definition %v", found.Definition)
	}
}

func TestProblemsNameTheirFields(t *testing.T) {
	api, _ := serve(t, func(w http.ResponseWriter, _ *http.Request, _ int) {
		reply(w, http.StatusUnprocessableEntity, "application/problem+json", map[string]any{
			"type": client.ProblemInvalid, "title": "The line is not valid", "status": 422,
			"detail": "stations[2].passesTo: names nowhere",
			"errors": []any{map[string]any{"path": "stations[2].passesTo", "message": "names nowhere", "severity": "error"}},
		})
	})

	_, err := api.CreateLine(context.Background(), "team-1", client.Definition{"key": "delivery"})

	problem, ok := client.ProblemOf(err)
	if !ok {
		t.Fatalf("expected a problem, got %v", err)
	}

	if problem.Status != 422 || problem.Type != client.ProblemInvalid || len(problem.Errors) != 1 || problem.Errors[0].Path != "stations[2].passesTo" {
		t.Errorf("problem %+v", problem)
	}
}

func TestAConflictSaysWhereTheResourceIsNow(t *testing.T) {
	api, _ := serve(t, func(w http.ResponseWriter, _ *http.Request, _ int) {
		reply(w, http.StatusConflict, "application/problem+json", map[string]any{
			"type": client.ProblemVersionConflict, "title": "The skill changed since it was read", "status": 409,
			"detail": "The skill is at version 7", "currentVersion": 7,
		})
	})

	err := api.DeleteSkill(context.Background(), "skill-1", 6)

	problem, ok := client.ProblemOf(err)
	if !ok || problem.CurrentVersion == nil || *problem.CurrentVersion != 7 {
		t.Errorf("problem %+v", problem)
	}
}

func TestNotFoundIsRecognised(t *testing.T) {
	api, _ := serve(t, func(w http.ResponseWriter, _ *http.Request, _ int) {
		reply(w, http.StatusNotFound, "application/problem+json", map[string]any{
			"type": client.ProblemNotFound, "title": "No such integration", "status": 404, "detail": "gone",
		})
	})

	_, err := api.ReadIntegration(context.Background(), "integration-1")
	if !client.IsNotFound(err) {
		t.Errorf("expected not found, got %v", err)
	}
}

func TestAnAnswerThatIsNotAProblemStillFails(t *testing.T) {
	api, _ := serve(t, func(w http.ResponseWriter, _ *http.Request, _ int) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = io.WriteString(w, "upstream down")
	})

	_, err := api.ReadWorkspace(context.Background(), "ws-1")

	problem, ok := client.ProblemOf(err)
	if !ok || problem.Status != http.StatusBadGateway || problem.Detail != "upstream down" {
		t.Errorf("problem %+v", problem)
	}
}

func TestExpressionsAreCheckedWhereTheyAreWritten(t *testing.T) {
	api, recorded := serve(t, func(w http.ResponseWriter, r *http.Request, _ int) {
		if r.Method == http.MethodGet {
			reply(w, http.StatusOK, "application/json", []any{
				map[string]any{"position": "integration.webhook.events[*].when", "returns": "bool", "sees": []any{}, "alsoSees": []any{"item"}},
			})
			return
		}

		reply(w, http.StatusOK, "application/json", map[string]any{
			"type":     "bool",
			"errors":   []any{map[string]any{"message": "No such key: tpye", "span": map[string]any{"start": 5, "end": 9}, "suggestion": "body.type"}},
			"warnings": []any{map[string]any{"position": "integration.webhook.events[0].lookup.capture.body", "message": "shadows the built-in body"}},
		})
	})

	positions, err := api.ExpressionPositions(context.Background())
	if err != nil || len(positions) != 1 || positions[0].Returns != "bool" {
		t.Fatalf("positions %+v, %v", positions, err)
	}

	check, err := api.CheckExpression(context.Background(), "integration.webhook.events[0].when", "body.tpye", client.Definition{"integration": "linear"})
	if err != nil {
		t.Fatal(err)
	}

	if len(check.Errors) != 1 || check.Errors[0].Suggestion != "body.type" || check.Errors[0].Span.Start != 5 || len(check.Warnings) != 1 {
		t.Errorf("check %+v", check)
	}

	var sent map[string]any
	_ = json.Unmarshal([]byte(recorded.all()[1].Body), &sent)
	if sent["position"] != "integration.webhook.events[0].when" || sent["expression"] != "body.tpye" {
		t.Errorf("sent %v", sent)
	}
}

func TestConfigIsChecked(t *testing.T) {
	cases := map[string]client.Config{
		"no endpoint": {Token: "token"},
		"not a URL":   {URL: "spotlight", Token: "token"},
		"no token":    {URL: "https://client.example.com"},
	}

	for name, cfg := range cases {
		if _, err := client.New(cfg); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestURLMayNameTheAPIPath(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		reply(w, http.StatusOK, "application/json", workspace)
	}))
	defer server.Close()

	for _, endpoint := range []string{server.URL, server.URL + "/", server.URL + "/api", server.URL + "/api/"} {
		api, err := client.New(client.Config{URL: endpoint, Token: "token"})
		if err != nil {
			t.Fatal(err)
		}

		if _, err := api.ReadWorkspace(context.Background(), "ws-1"); err != nil {
			t.Fatal(err)
		}
	}

	for _, each := range paths {
		if each != "/api/resources/workspaces/ws-1" {
			t.Errorf("asked %s", each)
		}
	}
}

func TestARefusalSaysWhatPermissionTheCallerLacked(t *testing.T) {
	api, _ := serve(t, func(w http.ResponseWriter, _ *http.Request, _ int) {
		reply(w, http.StatusForbidden, "application/problem+json", map[string]any{
			"type": client.ProblemForbidden, "title": "Not allowed to change the secret", "status": 403,
			"detail": "You may read this secret", "permission": "manage-secrets",
		})
	})

	_, err := api.ReadSecret(context.Background(), "secret-1")

	problem, ok := client.ProblemOf(err)
	if !ok || !client.IsForbidden(err) || problem.Permission != client.PermissionManageSecrets {
		t.Errorf("problem %+v", problem)
	}
}

var pool = map[string]any{
	"id": "pool-1", "version": 1, "address": "ephemeral", "warnings": []any{}, "workspaceId": "ws-1",
	"key": "ephemeral", "labels": []any{}, "maxRuns": 4, "mode": "pull", "tokenVersion": 2, "provision": nil,
}

func TestAPoolTokenSpotlightIssuesIsDroppedUnread(t *testing.T) {
	const issued = "issued-pool-token-that-must-never-be-kept-anywhere"

	api, _ := serve(t, func(w http.ResponseWriter, _ *http.Request, _ int) {
		w.Header().Set(client.PoolTokenHeader, issued)
		reply(w, http.StatusCreated, "application/json", pool)
	})

	created, err := api.CreateRunnerPool(context.Background(), "ws-1", client.RunnerPoolInput{Key: "ephemeral", Labels: []string{}})
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(fmt.Sprintf("%#v", created), issued) || *created.MaxRuns != 4 || *created.TokenVersion != 2 || created.Mode != "pull" {
		t.Errorf("pool %#v", created)
	}
}

var egress = map[string]any{
	"id": "egress-1", "version": 1, "address": "shared", "warnings": []any{}, "workspaceId": "ws-1",
	"key": "shared", "url": "https://egress.example.com", "tokenVersion": 3,
}

func TestAnEgressTokenSpotlightIssuesIsDroppedUnread(t *testing.T) {
	const issued = "issued-egress-token-that-must-never-be-kept-anywhere"

	api, recorded := serve(t, func(w http.ResponseWriter, _ *http.Request, _ int) {
		w.Header().Set(client.EgressTokenHeader, issued)
		reply(w, http.StatusCreated, "application/json", egress)
	})

	created, err := api.CreateEgress(context.Background(), "ws-1", client.EgressInput{Key: "shared", URL: "https://egress.example.com"})
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(fmt.Sprintf("%#v", created), issued) || created.URL != "https://egress.example.com" || *created.TokenVersion != 3 {
		t.Errorf("egress %#v", created)
	}

	if body := recorded.all()[0].Body; strings.Contains(body, `"token`) {
		t.Errorf("body %s", body)
	}
}

func TestAPushPoolNamesItsEgress(t *testing.T) {
	answer := maps.Clone(pool)
	answer["mode"] = "push"
	answer["provision"] = map[string]any{"url": "https://provision.example.com/runs", "secretVersion": 1, "egress": client.PlatformEgress}

	api, recorded := serve(t, func(w http.ResponseWriter, _ *http.Request, _ int) {
		reply(w, http.StatusCreated, "application/json", answer)
	})

	egress := client.PlatformEgress
	created, err := api.CreateRunnerPool(context.Background(), "ws-1", client.RunnerPoolInput{
		Key: "ephemeral", Labels: []string{},
		Provision: &client.PoolProvision{URL: "https://provision.example.com/runs", Secret: "whsec_c2VjcmV0", Egress: &egress},
	})
	if err != nil {
		t.Fatal(err)
	}

	if body := recorded.all()[0].Body; !strings.Contains(body, `"egress":"spotlight"`) {
		t.Errorf("body %s", body)
	}

	if created.Provision == nil || created.Provision.Egress == nil || *created.Provision.Egress != client.PlatformEgress {
		t.Errorf("pool %#v", created)
	}
}

func TestWriteOnlyValuesAreSentOnlyWhenGiven(t *testing.T) {
	api, recorded := serve(t, func(w http.ResponseWriter, _ *http.Request, _ int) {
		reply(w, http.StatusCreated, "application/json", pool)
	})

	token := strings.Repeat("t", 40)
	for _, in := range []client.RunnerPoolInput{{Key: "ephemeral"}, {Key: "ephemeral", Token: token}} {
		if _, err := api.CreateRunnerPool(context.Background(), "ws-1", in); err != nil {
			t.Fatal(err)
		}
	}

	seen := recorded.all()
	if strings.Contains(seen[0].Body, `"token"`) || !strings.Contains(seen[1].Body, `"token":"`+token+`"`) {
		t.Errorf("bodies %q and %q", seen[0].Body, seen[1].Body)
	}
}

func TestAPushPoolSendsItsSecretAndNoToken(t *testing.T) {
	api, recorded := serve(t, func(w http.ResponseWriter, _ *http.Request, _ int) {
		reply(w, http.StatusCreated, "application/json", pool)
	})

	version := int64(1)
	_, err := api.CreateRunnerPool(context.Background(), "ws-1", client.RunnerPoolInput{
		Key: "ephemeral", Labels: []string{},
		Provision: &client.PoolProvision{URL: "https://provision.example.com/runs", Secret: "whsec_c2VjcmV0", SecretVersion: &version},
	})
	if err != nil {
		t.Fatal(err)
	}

	body := recorded.all()[0].Body
	if strings.Contains(body, `"token`) || strings.Contains(body, `"maxRuns"`) ||
		!strings.Contains(body, `"provision":{"url":"https://provision.example.com/runs","secret":"whsec_c2VjcmV0","secretVersion":1}`) {
		t.Errorf("body %s", body)
	}
}
