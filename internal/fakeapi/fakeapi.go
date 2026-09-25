// Package fakeapi is an in-memory Spotlight control plane for tests. It
// follows the resource conventions the provider relies on: bearer tokens and
// their scopes, idempotent creates, If-Match versions, problem answers that
// name fields, reads that fill in defaults, imports by address, lists in
// address order, write-only values that are never answered, and the
// expression check.
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
)

// Token is the API token the fake accepts.
const Token = "test-token"

const apiVersion = "2026-09-23"

// PoolTokenHeader is where the fake answers a token it issued a pull pool.
const PoolTokenHeader = "Spotlight-Pool-Token"

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
	held       map[string]*resource
	idempotent map[string]string
	requests   []Request
	failNext   []int
	scopes     map[string]bool
	kept       map[string]string
	given      map[string]struct{}
}

type resource struct {
	kind    string
	id      string
	version int64
	owner   string
	fields  map[string]any
}

// New starts a fake control plane whose token is granted every scope; it
// stops when the test ends.
func New(t interface{ Cleanup(func()) }) *Server {
	s := &Server{
		held:       map[string]*resource{},
		idempotent: map[string]string{},
		kept:       map[string]string{},
		given:      map[string]struct{}{},
	}
	s.Grant("configure", "secrets", "runners")

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/resources/workspaces", s.listWorkspaces)
	mux.HandleFunc("POST /api/resources/workspaces", s.create("workspace"))
	mux.HandleFunc("GET /api/resources/workspaces/import", s.importWorkspace)
	mux.HandleFunc("GET /api/resources/workspaces/{id}", s.read("workspace"))
	mux.HandleFunc("PUT /api/resources/workspaces/{id}", s.update("workspace"))
	mux.HandleFunc("DELETE /api/resources/workspaces/{id}", s.remove("workspace"))
	mux.HandleFunc("POST /api/resources/workspaces/{owner}/{kinds}", s.createHeld("workspace"))
	mux.HandleFunc("GET /api/resources/workspaces/{workspace}/{kinds}", s.listHeld)
	mux.HandleFunc("GET /api/resources/workspaces/{workspace}/{kinds}/import", s.importHeld)
	mux.HandleFunc("POST /api/resources/teams/{owner}/{kinds}", s.createHeld("team"))
	mux.HandleFunc("GET /api/resources/{kinds}/{id}", s.readHeld)
	mux.HandleFunc("PUT /api/resources/{kinds}/{id}", s.updateHeld)
	mux.HandleFunc("DELETE /api/resources/{kinds}/{id}", s.removeHeld)
	mux.HandleFunc("GET /api/expressions/positions", s.positions)
	mux.HandleFunc("POST /api/expressions/check", s.check)
	mux.HandleFunc("/", noRoute)

	s.Server = httptest.NewServer(s.guard(mux))
	t.Cleanup(s.Close)

	return s
}

// Grant makes the token carry exactly these scopes from now on.
func (s *Server) Grant(scopes ...string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.scopes = map[string]bool{}
	for _, each := range scopes {
		s.scopes[each] = true
	}
}

// Requests lists every request received so far.
func (s *Server) Requests() []Request {
	s.mu.Lock()
	defer s.mu.Unlock()

	return append([]Request(nil), s.requests...)
}

// FailNext answers the next requests with these statuses before serving any.
func (s *Server) FailNext(statuses ...int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.failNext = append(s.failNext, statuses...)
}

// Count reports how many resources of a kind the fake holds.
func (s *Server) Count(kind string) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	count := 0
	for _, each := range s.held {
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

	for _, each := range s.held {
		if each.kind == kind && s.addressOf(each) == address {
			return each.id
		}
	}

	return ""
}

// Kept answers the write-only value the fake keeps for a resource: a
// secret's value, a pull pool's token, or a push pool's
// provision secret.
func (s *Server) Kept(id string) string {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.kept[id]
}

// Change changes a resource behind Terraform's back, as another client
// would, and bumps its version.
func (s *Server) Change(id string, change func(fields map[string]any)) {
	s.mu.Lock()
	defer s.mu.Unlock()

	held := s.held[id]
	change(held.fields)
	held.version++
}

// Forget removes a resource behind Terraform's back.
func (s *Server) Forget(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.held, id)
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

		if r.Header.Get("Authorization") != "Bearer "+Token {
			problem(w, http.StatusUnauthorized, "unauthenticated", "No valid credentials", "Send a Spotlight API token as a bearer token", nil)
			return
		}

		if !strings.HasPrefix(r.Header.Get("Spotlight-Client"), "terraform-provider/") {
			problem(w, http.StatusBadRequest, "bad-request", "The request is malformed", "Spotlight-Client is missing", nil)
			return
		}

		if asked := r.Header.Get("Spotlight-Api-Version"); asked != "" && asked != apiVersion {
			problem(w, http.StatusBadRequest, "unsupported-api-version", "This API version is not served here", "Asked for "+asked, nil)
			return
		}

		r.Body = io.NopCloser(strings.NewReader(string(raw)))
		next.ServeHTTP(w, r)
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
	"workspace-runners": "workspace-runners",
}

func ownerKind(kind string) string {
	switch kind {
	case "line", "skill", "integration", "harness-profile":
		return "team"
	default:
		return "workspace"
	}
}

var grants = map[string]string{
	"secret":            "secrets",
	"runner-trust":      "runners",
	"runner-pool":       "runners",
	"workspace-runners": "runners",
}

func (s *Server) allowed(w http.ResponseWriter, kind string) bool {
	if kind == "workspace" {
		return true
	}

	needs, ok := grants[kind]
	if !ok {
		needs = "configure"
	}

	if s.scopes[needs] {
		return true
	}

	problem(w, http.StatusForbidden, "forbidden", "Not allowed to change the "+kind,
		"You may read this "+kind+", but your role in its workspace, or your token’s scopes, do not let you change it", nil)

	return false
}

func (s *Server) createHeld(owner string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		kind, ok := kinds[r.PathValue("kinds")]
		if !ok || ownerKind(kind) != owner {
			noRoute(w, r)
			return
		}

		s.create(kind)(w, r)
	}
}

func (s *Server) readHeld(w http.ResponseWriter, r *http.Request) {
	if kind, ok := kinds[r.PathValue("kinds")]; ok {
		s.read(kind)(w, r)
		return
	}

	noRoute(w, r)
}

func (s *Server) updateHeld(w http.ResponseWriter, r *http.Request) {
	if kind, ok := kinds[r.PathValue("kinds")]; ok {
		s.update(kind)(w, r)
		return
	}

	noRoute(w, r)
}

func (s *Server) removeHeld(w http.ResponseWriter, r *http.Request) {
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
			s.answer(w, http.StatusCreated, s.held[id])
			return
		}

		owner := r.PathValue("owner")
		if kind != "workspace" && !s.holds(owner, ownerKind(kind)) {
			problem(w, http.StatusNotFound, "not-found", "No such "+kind, "There is no "+kind+" here that you may see", nil)
			return
		}

		if !s.allowed(w, kind) {
			return
		}

		fields, errs := normalized(kind, body)
		value, more := written(kind, nil, fields, body)
		errs = append(errs, more...)
		if len(errs) == 0 {
			errs = s.refusedPoolToken(kind, "", value)
		}
		if len(errs) > 0 {
			invalid(w, kind, errs)
			return
		}

		s.next++
		held := &resource{kind: kind, id: fmt.Sprintf("%s-%04d", kind, s.next), version: 1, owner: owner, fields: fields}

		for _, each := range s.held {
			if each.kind == kind && each.owner == owner && s.addressOf(each) == s.addressOf(held) {
				problem(w, http.StatusConflict, "already-exists", "The "+kind+" already exists", "There is already a "+kind+" at "+s.addressOf(held), nil)
				return
			}
		}

		if value == "" && givesPoolToken(kind, fields) {
			value = issue(w, held)
		}

		s.keep(held, value)
		s.held[held.id] = held
		s.idempotent[key] = held.id
		s.answer(w, http.StatusCreated, held)
	}
}

func (s *Server) read(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()

		held, ok := s.found(w, kind, r.PathValue("id"))
		if ok {
			s.answer(w, http.StatusOK, held)
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

		held, ok := s.found(w, kind, r.PathValue("id"))
		if !ok || !s.allowed(w, kind) || !s.current(w, r, held) {
			return
		}

		fields, errs := normalized(kind, body)
		for _, fixed := range fixedFields[kind] {
			if fields[fixed] != held.fields[fixed] {
				errs = append(errs, fieldError{fixed, "is fixed for the " + kind + "'s life; replace the " + kind + " to change it"})
			}
		}

		value, more := written(kind, held, fields, body)
		errs = append(errs, more...)
		if len(errs) == 0 {
			errs = s.refusedPoolToken(kind, held.id, value)
		}

		if len(errs) > 0 {
			invalid(w, kind, errs)
			return
		}

		s.keep(held, value)

		if kind == "secret" && objectOf(fields["source"])["kind"] != "built-in" {
			delete(s.kept, held.id)
		}

		held.fields = fields
		held.version++
		s.answer(w, http.StatusOK, held)
	}
}

func (s *Server) remove(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()

		held, ok := s.found(w, kind, r.PathValue("id"))
		if !ok || !s.allowed(w, kind) || !s.current(w, r, held) {
			return
		}

		for _, each := range s.held {
			if each.owner == held.id {
				problem(w, http.StatusConflict, "in-use", "The "+kind+" is still in use", "It still holds a "+each.kind, nil)
				return
			}
		}

		delete(s.held, held.id)
		delete(s.kept, held.id)
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Server) keep(held *resource, value string) {
	if value == "" {
		return
	}

	s.kept[held.id] = value
	if givesPoolToken(held.kind, held.fields) {
		s.given[value] = struct{}{}
	}
}

func (s *Server) refusedPoolToken(kind, claimant, value string) []fieldError {
	if kind != "runner-pool" || value == "" {
		return nil
	}

	if _, ever := s.given[value]; !ever {
		return nil
	}

	for id, kept := range s.kept {
		if held := s.held[id]; kept == value && held != nil && givesPoolToken(held.kind, held.fields) {
			if id == claimant {
				return nil
			}

			return []fieldError{{"token", poolTokenTaken}}
		}
	}

	return []fieldError{{"token", poolTokenSpent}}
}

func (s *Server) importWorkspace(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	address := r.URL.Query().Get("address")
	for _, each := range s.held {
		if each.kind == "workspace" && s.addressOf(each) == address {
			s.answer(w, http.StatusOK, each)
			return
		}
	}

	problem(w, http.StatusNotFound, "not-found", "No such workspace", "There is no workspace here that you may see", nil)
}

func (s *Server) importHeld(w http.ResponseWriter, r *http.Request) {
	kind, ok := kinds[r.PathValue("kinds")]
	if !ok {
		noRoute(w, r)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	workspace, address := r.PathValue("workspace"), r.URL.Query().Get("address")
	for _, each := range s.held {
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

	s.list(w, func(held *resource) bool { return held.kind == "workspace" })
}

func (s *Server) listHeld(w http.ResponseWriter, r *http.Request) {
	kind, ok := kinds[r.PathValue("kinds")]
	if !ok {
		noRoute(w, r)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	workspace := r.PathValue("workspace")
	if !s.holds(workspace, "workspace") {
		problem(w, http.StatusNotFound, "not-found", "No such workspace", "There is no workspace here that you may see", nil)
		return
	}

	s.list(w, func(held *resource) bool { return held.kind == kind && s.workspaceOf(held) == workspace })
}

func (s *Server) list(w http.ResponseWriter, listed func(*resource) bool) {
	var found []*resource
	for _, each := range s.held {
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
	held, ok := s.held[id]
	if !ok || held.kind != kind {
		problem(w, http.StatusNotFound, "not-found", "No such "+kind, "There is no "+kind+" here that you may see", nil)
		return nil, false
	}

	return held, true
}

func (s *Server) current(w http.ResponseWriter, r *http.Request, held *resource) bool {
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

	if version != held.version {
		w.Header().Set("ETag", fmt.Sprintf(`"%d"`, held.version))
		writeJSON(w, http.StatusConflict, "application/problem+json", map[string]any{
			"type":           "urn:spotlight:problem:moved-on",
			"title":          "The " + held.kind + " changed since it was read",
			"status":         http.StatusConflict,
			"detail":         fmt.Sprintf("The %s is at version %d", held.kind, held.version),
			"currentVersion": held.version,
		})
		return false
	}

	return true
}

func (s *Server) holds(id, kind string) bool {
	held, ok := s.held[id]

	return ok && held.kind == kind
}

var fixedFields = map[string][]string{
	"workspace":       {"slug"},
	"team":            {"key"},
	"line":            {"key"},
	"skill":           {"key"},
	"integration":     {"source"},
	"harness-profile": {"key"},
	"secret":          {"name"},
	"runner-trust":    {"key"},
	"runner-pool":     {"key"},
}

func givesPoolToken(kind string, fields map[string]any) bool {
	return kind == "runner-pool" && fields["mode"] == "pull"
}

func issue(w http.ResponseWriter, held *resource) string {
	issued := fmt.Sprintf("issued-%s-%s", held.id, strings.Repeat("x", 32))
	w.Header().Set(PoolTokenHeader, issued)

	return issued
}

func (s *Server) addressOf(held *resource) string {
	switch held.kind {
	case "workspace":
		return textOf(held.fields["slug"])
	case "team", "runner-trust", "runner-pool":
		return textOf(held.fields["key"])
	case "secret":
		return textOf(held.fields["name"])
	case "workspace-runners":
		return s.slugOf(held.owner)
	case "integration":
		return s.teamKeyOf(held) + "/" + textOf(held.fields["source"])
	default:
		return s.teamKeyOf(held) + "/" + textOf(held.fields["key"])
	}
}

func (s *Server) slugOf(workspaceID string) string {
	if workspace, ok := s.held[workspaceID]; ok {
		return textOf(workspace.fields["slug"])
	}

	return ""
}

func (s *Server) teamKeyOf(held *resource) string {
	if team, ok := s.held[held.owner]; ok {
		return textOf(team.fields["key"])
	}

	return ""
}

func (s *Server) workspaceOf(held *resource) string {
	switch {
	case held.kind == "workspace":
		return held.id
	case ownerKind(held.kind) == "workspace":
		return held.owner
	default:
		if team, ok := s.held[held.owner]; ok {
			return team.owner
		}
		return ""
	}
}

func (s *Server) answer(w http.ResponseWriter, status int, held *resource) {
	w.Header().Set("ETag", fmt.Sprintf(`"%d"`, held.version))
	writeJSON(w, status, "application/json", roundTripped(s.bodyOf(held)))
}

func (s *Server) bodyOf(held *resource) map[string]any {
	body := maps.Clone(held.fields)
	body["id"] = held.id
	body["version"] = held.version
	body["address"] = s.addressOf(held)
	body["warnings"] = []any{}

	switch ownerKind(held.kind) {
	case "team":
		body["teamId"] = held.owner
	default:
		if held.kind != "workspace" {
			body["workspaceId"] = held.owner
		}
	}

	if held.kind == "workspace-runners" {
		body["warnings"] = s.unansweredOf(held)
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
