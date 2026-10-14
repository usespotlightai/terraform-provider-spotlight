package client

import "time"

// Definition is a resource's definition as the API writes and reads it,
// with camelCase field names.
type Definition = map[string]any

// FieldError identifies an attribute of a request and tells what is wrong
// with it.
type FieldError struct {
	Path     string
	Message  string
	Severity string
}

// Meta contains the fields that every resource has in addition to its own
// fields.
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

// WorkspaceInput is the input to write a workspace.
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

// TeamInput is the input to write a team.
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

// SkillSecret is a secret that the SKILL.md of a skill asks for.
type SkillSecret struct {
	Name        string
	Description string
}

// SkillInput is the input to write a skill.
type SkillInput struct {
	Key   string            `json:"key"`
	Files map[string]string `json:"files"`
}

// Integration is an integration resource. Tools maps the name of each tool
// in the integration to the name that stations and hooks use, for example
// grafana.search. A tool can be hand-written or come from the API description
// of the integration.
type Integration struct {
	Meta
	TeamID     string
	Source     string
	Definition Definition
	Tools      map[string]string
	Described  *Described
}

// Described is the API description that the tools of an integration come
// from, as Spotlight last read it.
type Described struct {
	Kind    string
	Hash    *string
	ReadAt  time.Time
	Problem *string
	Tools   int64
}

// Hook is a hook of a team. It contains the steps that Spotlight does when
// an event matches.
type Hook struct {
	Meta
	TeamID string
	HookInput
}

// HookInput is the input to write a hook. Each step is the JSON that the
// configuration contains, because a step can give any JSON to its tool or
// send any JSON. The client sends Signing.Secret only when the hook must
// sign with a new secret. The API never returns it.
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

// HookMatch limits the events that a hook fires on. An empty list does not
// limit the events.
type HookMatch struct {
	Tools        []string `json:"tools,omitempty"`
	Lines        []string `json:"lines,omitempty"`
	Stations     []string `json:"stations,omitempty"`
	Repositories []string `json:"repositories,omitempty"`
	Paths        []string `json:"paths,omitempty"`
}

// HookSigning tells how a hook signs its requests. The hook signs with an
// HMAC secret or with a short-lived OIDC token for an audience.
type HookSigning struct {
	Secret        string    `json:"secret,omitempty"`
	SecretVersion *int64    `json:"secretVersion,omitempty"`
	OIDC          *HookOIDC `json:"oidc,omitempty"`
}

// HookOIDC is the audience of the OIDC tokens of a hook.
type HookOIDC struct {
	Audience string `json:"audience"`
}

// Secret is a secret resource. The client writes its value but never reads
// it back.
type Secret struct {
	Meta
	WorkspaceID  string
	Name         string
	Source       SecretSource
	Bindings     []SecretBinding
	ValueVersion *int64
	RotatedAt    string
}

// SecretSource tells where the value of a secret is. Spotlight keeps the
// value, or Spotlight reads it from AWS Secrets Manager each time it uses it.
type SecretSource struct {
	Kind     string `json:"kind"`
	SecretID string `json:"secretId,omitempty"`
	Field    string `json:"field,omitempty"`
}

// The places where the value of a secret can be.
const (
	SourceBuiltIn           = "built-in"
	SourceAWSSecretsManager = "aws-secrets-manager"
)

// SecretBinding is a host that can receive a secret, and how the requests to
// that host carry the secret.
type SecretBinding struct {
	Host    string `json:"host"`
	Carrier string `json:"carrier"`
	Name    string `json:"name,omitempty"`
	Prefix  string `json:"prefix"`
}

// SecretInput is the input to write a secret. The client sends Value only
// when Spotlight must keep the value or a new value.
type SecretInput struct {
	Name         string          `json:"name"`
	Source       SecretSource    `json:"source"`
	Bindings     []SecretBinding `json:"bindings"`
	Value        *string         `json:"valueWo,omitempty"`
	ValueVersion *int64          `json:"valueWoVersion,omitempty"`
}

// RunnerTrust is a workload whose tokens from its platform let a host claim
// runs through the trust.
type RunnerTrust struct {
	Meta
	WorkspaceID string
	RunnerTrustInput
}

// RunnerTrustInput is the input to write a runner trust.
type RunnerTrustInput struct {
	Key      string   `json:"key"`
	Issuer   string   `json:"issuer"`
	Subject  string   `json:"subject"`
	Audience string   `json:"audience"`
	Labels   []string `json:"labels"`
}

// WorkspaceTokenTrust is a CI workload that exchanges its OIDC tokens for
// short-lived workspace tokens. The CI platform stores no token.
type WorkspaceTokenTrust struct {
	Meta
	WorkspaceID string
	DefinedBy   string
	WorkspaceTokenTrustInput
}

// WorkspaceTokenTrustInput is the input to write a workspace token trust.
type WorkspaceTokenTrustInput struct {
	Key             string            `json:"key"`
	Issuer          string            `json:"issuer"`
	Audience        string            `json:"audience"`
	Subject         string            `json:"subject"`
	Claims          map[string]string `json:"claims"`
	Scopes          []string          `json:"scopes"`
	LifetimeMinutes int64             `json:"lifetimeMinutes"`
}

// RunnerPool is a pool of runners that each do one run. In pull mode, hosts
// claim runs from it with its token. In push mode, Spotlight calls its
// provision URL. The mode does not change during the life of the pool.
type RunnerPool struct {
	Meta
	WorkspaceID string
	Mode        string
	RunnerPoolInput
}

// RunnerPoolInput is the input to write a runner pool. The client sends
// Token and Provision.Secret only when the pool must use new values. The API
// never returns them.
type RunnerPoolInput struct {
	Key          string         `json:"key"`
	Labels       []string       `json:"labels"`
	MaxRuns      *int64         `json:"maxRuns,omitempty"`
	Token        string         `json:"token,omitempty"`
	TokenVersion *int64         `json:"tokenVersion,omitempty"`
	Provision    *PoolProvision `json:"provision,omitempty"`
}

// PoolProvision is the URL that Spotlight calls to start a runner for each
// run of a push pool, and the secret that signs those calls.
type PoolProvision struct {
	URL           string `json:"url"`
	Secret        string `json:"secret,omitempty"`
	SecretVersion *int64 `json:"secretVersion,omitempty"`
}

// WorkspaceRunners is the one declaration of a workspace that tells which
// runner trusts and pools can do its work.
type WorkspaceRunners struct {
	Meta
	WorkspaceID string
	WorkspaceRunnersInput
}

// WorkspaceRunnersInput is the input to write the declaration.
type WorkspaceRunnersInput struct {
	Trusts []string `json:"trusts"`
	Pools  []string `json:"pools"`
}

// HarnessProfile contains the settings of a team. Spotlight gives the
// settings to each harness in the terms of that harness.
type HarnessProfile struct {
	Meta
	TeamID string
	HarnessProfileInput
}

// HarnessProfileInput is the input to write a harness profile.
type HarnessProfileInput struct {
	Key         string         `json:"key"`
	Permissions Permissions    `json:"permissions"`
	MCPServers  []MCPServer    `json:"mcpServers"`
	Native      map[string]any `json:"native"`
}

// Permissions tell what a harness can do, what it must ask a person about
// first, and what it must never do.
type Permissions struct {
	Allow []string `json:"allow"`
	Ask   []string `json:"ask"`
	Deny  []string `json:"deny"`
}

// MCPServer is a stdio MCP server that Spotlight gives to each harness.
type MCPServer struct {
	Name    string            `json:"name"`
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
}

// Position is a pattern that identifies where an expression can be, with
// [*] for any index or key.
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

// Span is a part of an expression, in characters. The span does not contain
// End.
type Span struct {
	Start int
	End   int
}

// ExpressionWarning is something in the definition that an expression can
// fail to expect.
type ExpressionWarning struct {
	Position string
	Message  string
}

// ExpressionCheck is the result of the check of one expression.
type ExpressionCheck struct {
	Type     string
	Errors   []ExpressionError
	Warnings []ExpressionWarning
}
