package client

import "time"

// Definition is a resource's definition as the API writes and reads it,
// with camelCase field names.
type Definition = map[string]any

// FieldError names an attribute of a request and what is wrong with it.
type FieldError struct {
	Path     string
	Message  string
	Severity string
}

// Meta is what every resource carries besides its own fields.
type Meta struct {
	ID       string
	Version  int64
	Address  string
	Warnings []FieldError
}

// Workspace is a workspace resource.
type Workspace struct {
	Meta
	Name string
	Slug string
}

// WorkspaceInput is what a workspace is written from.
type WorkspaceInput struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

// Team is a team resource.
type Team struct {
	Meta
	WorkspaceID string
	Key         string
	Name        string
}

// TeamInput is what a team is written from.
type TeamInput struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

// Line is a line resource.
type Line struct {
	Meta
	TeamID     string
	Key        string
	Definition Definition
}

// Skill is a skill resource.
type Skill struct {
	Meta
	TeamID      string
	Key         string
	Files       map[string]string
	Description string
	Secrets     []SkillSecret
	Tools       []string
}

// SkillSecret is a secret a skill's SKILL.md asks for.
type SkillSecret struct {
	Name        string
	Description string
}

// SkillInput is what a skill is written from.
type SkillInput struct {
	Key   string            `json:"key"`
	Files map[string]string `json:"files"`
}

// Integration is an integration resource. Tools names each tool it
// offers, hand-written or generated from the API description it names, by
// its name within the integration, as stations and hooks name it, as in
// grafana.search.
type Integration struct {
	Meta
	TeamID     string
	Source     string
	Definition Definition
	Tools      map[string]string
	Described  *Described
}

// Described is the API description an integration's tools are generated
// from, as Spotlight last read it.
type Described struct {
	Kind    string
	Hash    *string
	ReadAt  time.Time
	Problem *string
	Tools   int64
}

// Hook is a team's hook: steps Spotlight takes when an event matches.
type Hook struct {
	Meta
	TeamID string
	HookInput
}

// HookInput is what a hook is written from. Each step is held as the JSON
// it is written as, since what a step gives its tool or sends is any JSON.
// Signing.Secret is sent only when the hook is to sign with it from now
// on, and never answered.
type HookInput struct {
	Key            string            `json:"key"`
	On             string            `json:"on"`
	Match          HookMatch         `json:"match"`
	When           *string           `json:"when,omitempty"`
	Vars           map[string]string `json:"vars,omitempty"`
	Steps          []any             `json:"steps"`
	Signing        *HookSigning      `json:"signing"`
	Failure        string            `json:"failure,omitempty"`
	TimeoutSeconds int64             `json:"timeoutSeconds,omitempty"`
}

// HookMatch narrows the events a hook fires on; an empty list narrows
// nothing.
type HookMatch struct {
	Tools        []string `json:"tools,omitempty"`
	Lines        []string `json:"lines,omitempty"`
	Stations     []string `json:"stations,omitempty"`
	Repositories []string `json:"repositories,omitempty"`
	Paths        []string `json:"paths,omitempty"`
}

// HookSigning is how a hook signs its requests: with an HMAC secret, or
// with a short-lived OIDC token for an audience.
type HookSigning struct {
	Secret        string    `json:"secret,omitempty"`
	SecretVersion *int64    `json:"secretVersion,omitempty"`
	OIDC          *HookOIDC `json:"oidc,omitempty"`
}

// HookOIDC is the audience a hook's OIDC tokens are for.
type HookOIDC struct {
	Audience string `json:"audience"`
}

// Secret is a secret resource. Its value is written, never read back.
type Secret struct {
	Meta
	WorkspaceID  string
	Name         string
	Source       SecretSource
	Bindings     []SecretBinding
	ValueVersion *int64
	RotatedAt    string
}

// SecretSource says where a secret's value lives: kept by Spotlight, or
// read from AWS Secrets Manager whenever it is used.
type SecretSource struct {
	Kind     string `json:"kind"`
	SecretID string `json:"secretId,omitempty"`
	Field    string `json:"field,omitempty"`
}

// The places a secret's value lives.
const (
	SourceBuiltIn           = "built-in"
	SourceAWSSecretsManager = "aws-secrets-manager"
)

// SecretBinding is a host a secret may be sent to, and how it is worn there.
type SecretBinding struct {
	Host    string `json:"host"`
	Carrier string `json:"carrier"`
	Name    string `json:"name,omitempty"`
	Prefix  string `json:"prefix"`
}

// SecretInput is what a secret is written from. Value is sent only when
// Spotlight is to keep it, or keep a new one.
type SecretInput struct {
	Name         string          `json:"name"`
	Source       SecretSource    `json:"source"`
	Bindings     []SecretBinding `json:"bindings"`
	Value        *string         `json:"valueWo,omitempty"`
	ValueVersion *int64          `json:"valueWoVersion,omitempty"`
}

// RunnerTrust is a workload whose platform-issued tokens let a host claim
// runs through it.
type RunnerTrust struct {
	Meta
	WorkspaceID string
	RunnerTrustInput
}

// RunnerTrustInput is what a runner trust is written from.
type RunnerTrustInput struct {
	Key      string   `json:"key"`
	Issuer   string   `json:"issuer"`
	Subject  string   `json:"subject"`
	Audience string   `json:"audience"`
	Labels   []string `json:"labels"`
}

// WorkspaceTokenTrust is a CI workload whose OIDC tokens are exchanged for
// short-lived workspace tokens, with no token stored in the CI platform.
type WorkspaceTokenTrust struct {
	Meta
	WorkspaceID string
	DefinedBy   string
	WorkspaceTokenTrustInput
}

// WorkspaceTokenTrustInput is what a workspace token trust is written from.
type WorkspaceTokenTrustInput struct {
	Key             string            `json:"key"`
	Issuer          string            `json:"issuer"`
	Audience        string            `json:"audience"`
	Subject         string            `json:"subject"`
	Claims          map[string]string `json:"claims"`
	Scopes          []string          `json:"scopes"`
	LifetimeMinutes int64             `json:"lifetimeMinutes"`
}

// RunnerPool is a pool of one-run runners: pulled from by hosts that claim
// with its token, or pushed to by Spotlight calling its provision
// URL. Mode, pull or push, is fixed for its life.
type RunnerPool struct {
	Meta
	WorkspaceID string
	Mode        string
	RunnerPoolInput
}

// RunnerPoolInput is what a runner pool is written from. Token and
// Provision.Secret are sent only when the pool is to use them from now on,
// and never answered.
type RunnerPoolInput struct {
	Key          string         `json:"key"`
	Labels       []string       `json:"labels"`
	MaxRuns      *int64         `json:"maxRuns,omitempty"`
	Token        string         `json:"token,omitempty"`
	TokenVersion *int64         `json:"tokenVersion,omitempty"`
	Provision    *PoolProvision `json:"provision,omitempty"`
}

// PoolProvision is where Spotlight calls to start a runner for each run a
// push pool is to serve, and the secret it signs those calls with.
type PoolProvision struct {
	URL           string `json:"url"`
	Secret        string `json:"secret,omitempty"`
	SecretVersion *int64 `json:"secretVersion,omitempty"`
}

// WorkspaceRunners is a workspace's one declaration of the runner trusts
// and pools it lets work through.
type WorkspaceRunners struct {
	Meta
	WorkspaceID string
	WorkspaceRunnersInput
}

// WorkspaceRunnersInput is what the declaration is written from.
type WorkspaceRunnersInput struct {
	Trusts []string `json:"trusts"`
	Pools  []string `json:"pools"`
}

// HarnessProfile is a team's settings, told to every harness in its own
// terms.
type HarnessProfile struct {
	Meta
	TeamID string
	HarnessProfileInput
}

// HarnessProfileInput is what a harness profile is written from.
type HarnessProfileInput struct {
	Key         string         `json:"key"`
	Permissions Permissions    `json:"permissions"`
	MCPServers  []MCPServer    `json:"mcpServers"`
	Native      map[string]any `json:"native"`
}

// Permissions are what a harness may do, what it asks a person about
// first, and what it may never do.
type Permissions struct {
	Allow []string `json:"allow"`
	Ask   []string `json:"ask"`
	Deny  []string `json:"deny"`
}

// MCPServer is a stdio MCP server handed to each harness.
type MCPServer struct {
	Name    string            `json:"name"`
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
}

// Position is a pattern naming where an expression is written, with [*]
// for any index or key.
type Position struct {
	Position string
	Returns  string
}

// ExpressionError is something wrong with an expression.
type ExpressionError struct {
	Message    string
	Span       *Span
	Suggestion string
}

// Span is a part of an expression, in characters, end exclusive.
type Span struct {
	Start int
	End   int
}

// ExpressionWarning is something the definition does that an expression may
// not expect.
type ExpressionWarning struct {
	Position string
	Message  string
}

// ExpressionCheck is the verdict on one expression.
type ExpressionCheck struct {
	Type     string
	Errors   []ExpressionError
	Warnings []ExpressionWarning
}
