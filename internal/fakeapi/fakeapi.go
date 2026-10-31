// Package fakeapi is an in-memory Spotlight control plane for tests. It
// follows the resource conventions that the provider uses:
//   - Bearer tokens and their scopes.
//   - Idempotent creates and If-Match versions.
//   - Problem answers that identify fields.
//   - Reads that add default values.
//   - Imports by address and lists in address order.
//   - Write-only values that the fake never returns.
//   - The expression check.
//   - The exchange of the OIDC token of a workload for an issued token.
package fakeapi

import (
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Token is the API token the fake accepts.
const Token = "test-token"

// WorkloadToken is the only OIDC token that the OIDC trusts of
// the fake accept.
const WorkloadToken = "test-workload-token"

// Admin is the person who created each OIDC trust in the fake.
const Admin = "test-admin"

const apiVersion = "2026-09-23"

// PoolTokenHeader is the header where the fake returns a token that it
// issued to a pull pool.
const PoolTokenHeader = "Spotlight-Pool-Token"

// EgressTokenHeader is the header where the fake returns a token that it
// issued to an egress.
const EgressTokenHeader = "Spotlight-Egress-Token"

// PlatformEgress names the egress that Spotlight operates. The fake always
// has it.
const PlatformEgress = "spotlight"

// Request is a request the fake received.
type Request struct {
	Method  string
	Path    string
	Headers http.Header
	Body    map[string]any
}

// Server is a fake control plane.
type Server struct {
	*httptest.Server

	mu         sync.Mutex
	next       int
	stored     map[string]*resource
	idempotent map[string]string
	requests   []Request
	failNext   []int
	scopes     map[string]bool
	writeOnly  map[string]string
	used       map[usedToken]struct{}
	issued     map[string]string
	issuedFor  time.Duration
}

type resource struct {
	kind    string
	id      string
	version int64
	owner   string
	fields  map[string]any
}

// New starts a fake control plane whose token has all scopes. The fake stops
// when the test ends.
func New(t interface{ Cleanup(func()) }) *Server {
	s := &Server{
		stored:     map[string]*resource{},
		idempotent: map[string]string{},
		writeOnly:  map[string]string{},
		used:       map[usedToken]struct{}{},
		issued:     map[string]string{},
		issuedFor:  15 * time.Minute,
	}
	s.Grant("configure", "secrets", "runners", "trusts")

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/resources/workspaces", s.listWorkspaces)
	mux.HandleFunc("POST /api/v1/resources/workspaces", s.create("workspace"))
	mux.HandleFunc("GET /api/v1/resources/workspaces/import", s.importWorkspace)
	mux.HandleFunc("GET /api/v1/resources/workspaces/{id}", s.read("workspace"))
	mux.HandleFunc("PUT /api/v1/resources/workspaces/{id}", s.update("workspace"))
	mux.HandleFunc("DELETE /api/v1/resources/workspaces/{id}", s.remove("workspace"))
	mux.HandleFunc("POST /api/v1/resources/workspaces/{owner}/{kinds}", s.createByKind("workspace"))
	mux.HandleFunc("GET /api/v1/resources/workspaces/{workspace}/{kinds}", s.listByKind)
	mux.HandleFunc("GET /api/v1/resources/workspaces/{workspace}/{kinds}/import", s.importByKind)
	mux.HandleFunc("POST /api/v1/resources/teams/{owner}/{kinds}", s.createByKind("team"))
	mux.HandleFunc("GET /api/v1/resources/{kinds}/{id}", s.readByKind)
	mux.HandleFunc("PUT /api/v1/resources/{kinds}/{id}", s.updateByKind)
	mux.HandleFunc("DELETE /api/v1/resources/{kinds}/{id}", s.removeByKind)
	mux.HandleFunc("GET /api/v1/expressions/positions", s.positions)
	mux.HandleFunc("POST /api/v1/expressions/check", s.check)
	mux.HandleFunc("POST /api/v1/workspace-tokens/exchange", s.exchange)
	mux.HandleFunc("/", noRoute)

	s.Server = httptest.NewServer(s.guard(mux))
	t.Cleanup(s.Close)

	return s
}

// Grant limits the token to these scopes for all later calls.
func (s *Server) Grant(scopes ...string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.scopes = map[string]bool{}
	for _, each := range scopes {
		s.scopes[each] = true
	}
}

// IssueFor makes the tokens that the fake issues after this call expire after
// d.
func (s *Server) IssueFor(d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.issuedFor = d
}

// Requests lists each request that the fake received.
func (s *Server) Requests() []Request {
	s.mu.Lock()
	defer s.mu.Unlock()

	return append([]Request(nil), s.requests...)
}

// FailNext returns these statuses to the next requests before it serves
// requests again.
func (s *Server) FailNext(statuses ...int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.failNext = append(s.failNext, statuses...)
}

// Count reports the number of resources of a kind that the fake stores.
func (s *Server) Count(kind string) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	count := 0
	for _, each := range s.stored {
		if each.kind == kind {
			count++
		}
	}

	return count
}

// IDOf finds the id of the resource of a kind at an address.
func (s *Server) IDOf(kind, address string) string {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, each := range s.stored {
		if each.kind == kind && s.addressOf(each) == address {
			return each.id
		}
	}

	return ""
}

// WriteOnly returns the write-only value that the fake stores for a resource:
//   - The value of a secret.
//   - The token of a pull pool.
//   - The token of an egress.
//   - The provision secret of a push pool.
//   - The secret that a hook signs with.
func (s *Server) WriteOnly(id string) string {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.writeOnly[id]
}

// Change changes a resource without Terraform, as a different client does,
// and increases its version.
func (s *Server) Change(id string, change func(fields map[string]any)) {
	s.mu.Lock()
	defer s.mu.Unlock()

	stored := s.stored[id]
	change(stored.fields)
	stored.version++
}

// Delete removes a resource without Terraform.
func (s *Server) Delete(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.stored, id)
}

func (s *Server) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = r.Body.Close()

		var body map[string]any
		_ = json.Unmarshal(raw, &body)

		s.mu.Lock()
		s.requests = append(s.requests, Request{Method: r.Method, Path: r.URL.Path, Headers: r.Header.Clone(), Body: body})
		failing := 0
		if len(s.failNext) > 0 {
			failing, s.failNext = s.failNext[0], s.failNext[1:]
		}
		s.mu.Unlock()

		w.Header().Set("Spotlight-Api-Version", apiVersion)

		if failing != 0 {
			problem(w, failing, "unavailable", "Try again", "The fake was told to fail", nil)
			return
		}

		if r.URL.Path != exchangePath && !s.authenticates(r.Header.Get("Authorization")) {
			problem(w, http.StatusUnauthorized, "unauthenticated", "No valid credentials", "Send a Spotlight API token as a bearer token", nil)
			return
		}

		if !strings.HasPrefix(r.Header.Get("Spotlight-Client"), "terraform-provider/") {
			problem(w, http.StatusBadRequest, "bad-request", "The request is malformed", "Spotlight-Client is missing", nil)
			return
		}

		if requested := r.Header.Get("Spotlight-Api-Version"); requested != "" && requested != apiVersion {
			problem(w, http.StatusBadRequest, "unsupported-api-version", "This API version is not served here", "Requested "+requested, nil)
			return
		}

		r.Body = io.NopCloser(strings.NewReader(string(raw)))
		next.ServeHTTP(w, r)
	})
}

const exchangePath = "/api/v1/workspace-tokens/exchange"

func (s *Server) authenticates(authorization string) bool {
	bearer, ok := strings.CutPrefix(authorization, "Bearer ")
	if !ok {
		return false
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	_, issued := s.issued[bearer]

	return bearer == Token || issued
}

func (s *Server) revokeIssued(trust string) {
	for token, issuedBy := range s.issued {
		if issuedBy == trust {
			delete(s.issued, token)
		}
	}
}

func (s *Server) exchange(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Trust string `json:"trust"`
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		problem(w, http.StatusBadRequest, "bad-request", "The request is malformed", err.Error(), nil)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	trust, ok := s.stored[body.Trust]
	if !ok || trust.kind != "oidc-trust" || body.Token != WorkloadToken {
		problem(w, http.StatusUnauthorized, "unauthenticated", "The workload token was not accepted", "No trust here matches that workload", nil)
		return
	}

	s.next++
	token := fmt.Sprintf("issued-%04d-%s", s.next, strings.Repeat("m", 32))
	s.issued[token] = trust.id

	writeJSON(w, http.StatusOK, "application/json", map[string]any{
		"token":       token,
		"tokenId":     fmt.Sprintf("token-%04d", s.next),
		"workspaceId": trust.owner,
		"scopes":      trust.fields["scopes"],
		"expiresAt":   time.Now().Add(s.issuedFor).UTC().Format(time.RFC3339Nano),
	})
}

var kinds = map[string]string{
	"teams":             "team",
	"lines":             "line",
	"skills":            "skill",
	"integrations":      "integration",
	"harness-profiles":  "harness-profile",
	"secrets":           "secret",
	"runner-trusts":     "runner-trust",
	"runner-pools":      "runner-pool",
	"egresses":          "egress",
	"workspace-runners": "workspace-runners",
	"oidc-trusts":       "oidc-trust",
	"hooks":             "hook",
}

func ownerKind(kind string) string {
	switch kind {
	case "line", "skill", "integration", "harness-profile", "hook":
		return "team"
	default:
		return "workspace"
	}
}

var grants = map[string]string{
	"secret":            "secrets",
	"runner-trust":      "runners",
	"runner-pool":       "runners",
	"egress":            "runners",
	"workspace-runners": "runners",
}

func (s *Server) allowed(w http.ResponseWriter, kind string) bool {
	if kind == "workspace" {
		return true
	}

	if kind == "oidc-trust" {
		if s.scopes["trusts"] {
			return true
		}

		forbidTrust(w, cannotChange("oidc-trust"))

		return false
	}

	needs, ok := grants[kind]
	if !ok {
		needs = "configure"
	}

	if s.scopes[needs] {
		return true
	}

	problem(w, http.StatusForbidden, "forbidden", "Not allowed to change the "+kind,
		cannotChange(kind), nil)

	return false
}

var implied = map[string][]string{
	"read":      {"read"},
	"plan":      {"plan", "read"},
	"configure": {"configure", "plan", "read"},
	"secrets":   {"secrets", "read"},
	"runners":   {"runners", "read"},
	"trusts":    {"trusts", "read"},
}

func (s *Server) grants(w http.ResponseWriter, scopes ...any) bool {
	owned := map[string]bool{}
	for granted := range s.scopes {
		for _, each := range implied[granted] {
			owned[each] = true
		}
	}

	for _, listed := range scopes {
		for _, each := range texts(listed) {
			if !owned[textOf(each)] {
				forbidTrust(w, cannotChange("oidc-trust"))
				return false
			}
		}
	}

	return true
}

func forbidTrust(w http.ResponseWriter, detail string) {
	writeJSON(w, http.StatusForbidden, "application/problem+json", map[string]any{
		"type":       "urn:spotlight:problem:forbidden",
		"title":      "Not allowed to change the oidc-trust",
		"status":     http.StatusForbidden,
		"detail":     detail,
		"permission": "trusts",
	})
}

func (s *Server) createByKind(owner string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		kind, ok := kinds[r.PathValue("kinds")]
		if !ok || ownerKind(kind) != owner {
			noRoute(w, r)
			return
		}

		s.create(kind)(w, r)
	}
}

func (s *Server) readByKind(w http.ResponseWriter, r *http.Request) {
	if kind, ok := kinds[r.PathValue("kinds")]; ok {
		s.read(kind)(w, r)
		return
	}

	noRoute(w, r)
}

func (s *Server) updateByKind(w http.ResponseWriter, r *http.Request) {
	if kind, ok := kinds[r.PathValue("kinds")]; ok {
		s.update(kind)(w, r)
		return
	}

	noRoute(w, r)
}

func (s *Server) removeByKind(w http.ResponseWriter, r *http.Request) {
	if kind, ok := kinds[r.PathValue("kinds")]; ok {
		s.remove(kind)(w, r)
		return
	}

	noRoute(w, r)
}

func (s *Server) create(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("Idempotency-Key")
		if key == "" {
			problem(w, http.StatusBadRequest, "idempotency-key-required", "A create must carry an Idempotency-Key", "Send one", nil)
			return
		}

		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			problem(w, http.StatusBadRequest, "bad-request", "The request is malformed", err.Error(), nil)
			return
		}

		s.mu.Lock()
		defer s.mu.Unlock()

		if id, ok := s.idempotent[key]; ok {
			w.Header().Set("Idempotent-Replayed", "true")
			s.answer(w, http.StatusCreated, s.stored[id])
			return
		}

		owner := r.PathValue("owner")
		if kind != "workspace" && !s.exists(owner, ownerKind(kind)) {
			problem(w, http.StatusNotFound, "not-found", "No such "+kind, "There is no "+kind+" here that you may see", nil)
			return
		}

		if !s.allowed(w, kind) {
			return
		}

		fields, errs := normalized(kind, body)
		errs = append(errs, s.unknownEgress(kind, owner, fields)...)
		value, more := written(kind, nil, fields, body)
		errs = append(errs, more...)
		if len(errs) == 0 {
			errs = s.tokenProblems(kind, "", value)
		}
		if len(errs) > 0 {
			invalid(w, kind, errs)
			return
		}

		if kind == "oidc-trust" && !s.grants(w, fields["scopes"]) {
			return
		}

		s.next++
		created := &resource{kind: kind, id: fmt.Sprintf("%s-%04d", kind, s.next), version: 1, owner: owner, fields: fields}

		for _, each := range s.stored {
			if each.kind == kind && each.owner == owner && s.addressOf(each) == s.addressOf(created) {
				problem(w, http.StatusConflict, "already-exists", "The "+kind+" already exists", "There is already a "+kind+" at "+s.addressOf(created), nil)
				return
			}
		}

		if value == "" && hasToken(kind, fields) {
			value = issue(w, created)
		}

		s.storeWriteOnly(created, value)
		s.stored[created.id] = created
		s.idempotent[key] = created.id
		s.answer(w, http.StatusCreated, created)
	}
}

func (s *Server) read(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()

		existing, ok := s.found(w, kind, r.PathValue("id"))
		if ok {
			s.answer(w, http.StatusOK, existing)
		}
	}
}

func (s *Server) update(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			problem(w, http.StatusBadRequest, "bad-request", "The request is malformed", err.Error(), nil)
			return
		}

		s.mu.Lock()
		defer s.mu.Unlock()

		existing, ok := s.found(w, kind, r.PathValue("id"))
		if !ok || !s.allowed(w, kind) || !s.current(w, r, existing) {
			return
		}

		fields, errs := normalized(kind, body)
		for _, fixed := range fixedFields[kind] {
			if fields[fixed] != existing.fields[fixed] {
				errs = append(errs, fieldError{fixed, "is fixed for the " + kind + "'s life; replace the " + kind + " to change it"})
			}
		}

		errs = append(errs, s.unknownEgress(kind, existing.owner, fields)...)
		value, more := written(kind, existing, fields, body)
		errs = append(errs, more...)
		if len(errs) == 0 {
			errs = s.tokenProblems(kind, existing.id, value)
		}

		if len(errs) == 0 && kind == "oidc-trust" && !s.grants(w, existing.fields["scopes"], fields["scopes"]) {
			return
		}

		if len(errs) > 0 {
			invalid(w, kind, errs)
			return
		}

		s.storeWriteOnly(existing, value)

		if kind == "secret" && objectOf(fields["source"])["kind"] != "built-in" {
			delete(s.writeOnly, existing.id)
		}

		existing.fields = fields
		existing.version++
		s.revokeIssued(existing.id)
		s.answer(w, http.StatusOK, existing)
	}
}

func (s *Server) remove(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()

		existing, ok := s.found(w, kind, r.PathValue("id"))
		if !ok || !s.allowed(w, kind) || !s.current(w, r, existing) {
			return
		}

		var dependents []string
		for _, each := range s.stored {
			if each.owner == existing.id {
				dependents = append(dependents, dependentName(each))
			}
		}

		if len(dependents) > 0 {
			slices.Sort(dependents)
			problem(w, http.StatusConflict, "in-use", "The "+kind+" is still in use", notEmpty(kind, existing)+" It still has: "+strings.Join(dependents, ", "), nil)
			return
		}

		if naming := s.poolsNaming(existing); len(naming) > 0 {
			problem(w, http.StatusConflict, "in-use", "The "+kind+" is still in use",
				fmt.Sprintf("Egress %s is in use. Runner pools that name it: %s. Please remove provision.egress from these pools first.", textOf(existing.fields["key"]), strings.Join(naming, ", ")), nil)
			return
		}

		delete(s.stored, existing.id)
		delete(s.writeOnly, existing.id)
		s.revokeIssued(existing.id)
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Server) storeWriteOnly(stored *resource, value string) {
	if value == "" {
		return
	}

	s.writeOnly[stored.id] = value
	if hasToken(stored.kind, stored.fields) {
		s.used[usedToken{stored.kind, value}] = struct{}{}
	}
}

type usedToken struct{ kind, token string }

func (s *Server) tokenProblems(kind, claimant, value string) []fieldError {
	if (kind != "runner-pool" && kind != "egress") || value == "" {
		return nil
	}

	if _, ever := s.used[usedToken{kind, value}]; !ever {
		return nil
	}

	for id, token := range s.writeOnly {
		if stored := s.stored[id]; token == value && stored != nil && stored.kind == kind && hasToken(stored.kind, stored.fields) {
			if id == claimant {
				return nil
			}

			return []fieldError{{"token", tokenInUse[kind]}}
		}
	}

	return []fieldError{{"token", tokenAlreadyUsed[kind]}}
}

func (s *Server) unknownEgress(kind, workspace string, fields map[string]any) []fieldError {
	if kind != "runner-pool" {
		return nil
	}

	named, ok := objectOf(fields["provision"])["egress"].(string)
	if !ok || named == PlatformEgress {
		return nil
	}

	if stored, found := s.stored[named]; found && stored.kind == "egress" && stored.owner == workspace {
		return nil
	}

	return []fieldError{{"provision.egress", fmt.Sprintf("names %s. Egress %s doesn't exist in this workspace", named, named)}}
}

func (s *Server) poolsNaming(stored *resource) []string {
	if stored.kind != "egress" {
		return nil
	}

	var naming []string
	for _, each := range s.stored {
		if each.kind == "runner-pool" && objectOf(each.fields["provision"])["egress"] == stored.id {
			naming = append(naming, textOf(each.fields["key"]))
		}
	}
	slices.Sort(naming)

	return naming
}

func (s *Server) importWorkspace(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	address := r.URL.Query().Get("address")
	for _, each := range s.stored {
		if each.kind == "workspace" && s.addressOf(each) == address {
			s.answer(w, http.StatusOK, each)
			return
		}
	}

	problem(w, http.StatusNotFound, "not-found", "No such workspace", "There is no workspace here that you may see", nil)
}

func (s *Server) importByKind(w http.ResponseWriter, r *http.Request) {
	kind, ok := kinds[r.PathValue("kinds")]
	if !ok {
		noRoute(w, r)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	workspace, address := r.PathValue("workspace"), r.URL.Query().Get("address")
	for _, each := range s.stored {
		if each.kind == kind && s.workspaceOf(each) == workspace && s.addressOf(each) == address {
			s.answer(w, http.StatusOK, each)
			return
		}
	}

	problem(w, http.StatusNotFound, "not-found", "No such "+kind, "There is no "+kind+" here that you may see", nil)
}

func (s *Server) listWorkspaces(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.list(w, func(stored *resource) bool { return stored.kind == "workspace" })
}

func (s *Server) listByKind(w http.ResponseWriter, r *http.Request) {
	kind, ok := kinds[r.PathValue("kinds")]
	if !ok {
		noRoute(w, r)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	workspace := r.PathValue("workspace")
	if !s.exists(workspace, "workspace") {
		problem(w, http.StatusNotFound, "not-found", "No such workspace", "There is no workspace here that you may see", nil)
		return
	}

	s.list(w, func(stored *resource) bool { return stored.kind == kind && s.workspaceOf(stored) == workspace })
}

func (s *Server) list(w http.ResponseWriter, listed func(*resource) bool) {
	var found []*resource
	for _, each := range s.stored {
		if listed(each) {
			found = append(found, each)
		}
	}

	slices.SortFunc(found, func(a, b *resource) int { return strings.Compare(s.addressOf(a), s.addressOf(b)) })

	items := make([]any, 0, len(found))
	for _, each := range found {
		items = append(items, s.bodyOf(each))
	}

	writeJSON(w, http.StatusOK, "application/json", roundTripped(map[string]any{"items": items}))
}

func (s *Server) found(w http.ResponseWriter, kind, id string) (*resource, bool) {
	stored, ok := s.stored[id]
	if !ok || stored.kind != kind {
		problem(w, http.StatusNotFound, "not-found", "No such "+kind, "There is no "+kind+" here that you may see", nil)
		return nil, false
	}

	return stored, true
}

func (s *Server) current(w http.ResponseWriter, r *http.Request, stored *resource) bool {
	tag := r.Header.Get("If-Match")
	if tag == "" {
		problem(w, http.StatusPreconditionRequired, "precondition-required", "The request must say which version it changes", "Send If-Match", nil)
		return false
	}

	version, err := strconv.ParseInt(strings.Trim(tag, `"`), 10, 64)
	if err != nil {
		problem(w, http.StatusBadRequest, "malformed-if-match", "If-Match names no version this API issued", tag, nil)
		return false
	}

	if version != stored.version {
		w.Header().Set("ETag", fmt.Sprintf(`"%d"`, stored.version))
		writeJSON(w, http.StatusConflict, "application/problem+json", map[string]any{
			"type":           "urn:spotlight:problem:version-conflict",
			"title":          "The " + stored.kind + " changed since it was read",
			"status":         http.StatusConflict,
			"detail":         fmt.Sprintf("The %s is at version %d", stored.kind, stored.version),
			"currentVersion": stored.version,
		})
		return false
	}

	return true
}

func (s *Server) exists(id, kind string) bool {
	stored, ok := s.stored[id]

	return ok && stored.kind == kind
}

var fixedFields = map[string][]string{
	"workspace":       {"slug"},
	"team":            {"key"},
	"line":            {"key"},
	"skill":           {"key"},
	"integration":     {"integration"},
	"harness-profile": {"key"},
	"secret":          {"name"},
	"runner-trust":    {"key"},
	"runner-pool":     {"key"},
	"egress":          {"key"},
	"oidc-trust":      {"key"},
	"hook":            {"key"},
}

func hasToken(kind string, fields map[string]any) bool {
	return kind == "egress" || (kind == "runner-pool" && fields["mode"] == "pull")
}

func issue(w http.ResponseWriter, stored *resource) string {
	issued := fmt.Sprintf("issued-%s-%s", stored.id, strings.Repeat("x", 32))
	header := PoolTokenHeader
	if stored.kind == "egress" {
		header = EgressTokenHeader
	}
	w.Header().Set(header, issued)

	return issued
}

func (s *Server) addressOf(stored *resource) string {
	switch stored.kind {
	case "workspace":
		return textOf(stored.fields["slug"])
	case "team", "runner-trust", "runner-pool", "egress", "oidc-trust":
		return textOf(stored.fields["key"])
	case "secret":
		return textOf(stored.fields["name"])
	case "workspace-runners":
		return s.slugOf(stored.owner)
	case "integration":
		return s.teamKeyOf(stored) + "/" + textOf(stored.fields["integration"])
	default:
		return s.teamKeyOf(stored) + "/" + textOf(stored.fields["key"])
	}
}

func (s *Server) slugOf(workspaceID string) string {
	if workspace, ok := s.stored[workspaceID]; ok {
		return textOf(workspace.fields["slug"])
	}

	return ""
}

func (s *Server) teamKeyOf(stored *resource) string {
	if team, ok := s.stored[stored.owner]; ok {
		return textOf(team.fields["key"])
	}

	return ""
}

func (s *Server) workspaceOf(stored *resource) string {
	switch {
	case stored.kind == "workspace":
		return stored.id
	case ownerKind(stored.kind) == "workspace":
		return stored.owner
	default:
		if team, ok := s.stored[stored.owner]; ok {
			return team.owner
		}
		return ""
	}
}

func (s *Server) answer(w http.ResponseWriter, status int, stored *resource) {
	w.Header().Set("ETag", fmt.Sprintf(`"%d"`, stored.version))
	writeJSON(w, status, "application/json", roundTripped(s.bodyOf(stored)))
}

func (s *Server) bodyOf(stored *resource) map[string]any {
	body := maps.Clone(stored.fields)
	body["id"] = stored.id
	body["version"] = stored.version
	body["address"] = s.addressOf(stored)
	body["warnings"] = []any{}

	switch ownerKind(stored.kind) {
	case "team":
		body["teamId"] = stored.owner
	default:
		if stored.kind != "workspace" {
			body["workspaceId"] = stored.owner
		}
	}

	if stored.kind == "workspace-runners" {
		body["warnings"] = s.unansweredOf(stored)
	}

	if stored.kind == "oidc-trust" {
		body["createdBy"] = Admin
	}

	if stored.kind == "egress" && strings.HasPrefix(textOf(stored.fields["url"]), "http:") {
		body["warnings"] = []any{warned("url", "is not https. Runners send their proxy credential to it. A production deployment does not provision runners that use it")}
	}

	if stored.kind == "integration" {
		body["tools"] = provided(stored.fields)
		body["spec"] = spec(stored.fields)
	}

	return body
}

func roundTripped(body map[string]any) map[string]any {
	raw, _ := json.Marshal(body)

	var out map[string]any
	_ = json.Unmarshal(raw, &out)

	return out
}

type fieldError struct {
	path    string
	message string
}

func invalid(w http.ResponseWriter, kind string, errs []fieldError) {
	lines := make([]string, 0, len(errs))
	for _, each := range errs {
		if each.path == "" {
			lines = append(lines, each.message)
		} else {
			lines = append(lines, each.path+": "+each.message)
		}
	}

	problem(w, http.StatusUnprocessableEntity, "invalid", "The "+kind+" is not valid", strings.Join(lines, "\n"), errs)
}

func problem(w http.ResponseWriter, status int, slug, title, detail string, errs []fieldError) {
	body := map[string]any{
		"type":   "urn:spotlight:problem:" + slug,
		"title":  title,
		"status": status,
		"detail": detail,
	}

	if errs != nil {
		listed := make([]any, 0, len(errs))
		for _, each := range errs {
			listed = append(listed, map[string]any{"path": each.path, "message": each.message, "severity": "error"})
		}
		body["errors"] = listed
	}

	writeJSON(w, status, "application/problem+json", body)
}

func writeJSON(w http.ResponseWriter, status int, contentType string, body any) {
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func noRoute(w http.ResponseWriter, r *http.Request) {
	problem(w, http.StatusNotFound, "not-found", "No such resource", "Cannot "+r.Method+" "+r.URL.Path, nil)
}

func notEmpty(kind string, existing *resource) string {
	switch kind {
	case "team":
		return "Team " + textOf(existing.fields["key"]) + " is not empty."
	case "workspace":
		return "Workspace " + textOf(existing.fields["slug"]) + " is not empty."
	default:
		return "The " + kind + " is not empty."
	}
}

func cannotChange(kind string) string {
	return "You can read this " + kind + " but not change it. Your workspace role or token scopes do not allow it."
}

func dependentName(each *resource) string {
	for _, field := range []string{"key", "slug", "name"} {
		if named := textOf(each.fields[field]); named != "" {
			return strings.ReplaceAll(each.kind, "-", " ") + " " + named
		}
	}

	return strings.ReplaceAll(each.kind, "-", " ")
}
