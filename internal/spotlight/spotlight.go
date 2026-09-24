// Package spotlight is the provider's port onto the Spotlight control plane:
// the resources it manages, the problems it answers, and the operations the
// Terraform resources depend on.
package spotlight

import "context"

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

// Integration is an integration resource.
type Integration struct {
	Meta
	TeamID     string
	Source     string
	Definition Definition
}

// Resources manages one kind of resource. Owner is the id of the resource
// that holds a new one (a team for lines, the workspace for teams) and is
// empty for workspaces; Import takes the workspace id and the address, and
// the workspace id is empty when importing a workspace by its slug.
type Resources[In, Out any] interface {
	Create(ctx context.Context, owner string, in In) (Out, error)
	Read(ctx context.Context, id string) (Out, error)
	Update(ctx context.Context, id string, version int64, in In) (Out, error)
	Delete(ctx context.Context, id string, version int64) error
	Import(ctx context.Context, workspaceID, address string) (Out, error)
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

// Expressions type-checks integration expressions where they are written.
type Expressions interface {
	Positions(ctx context.Context) ([]Position, error)
	Check(ctx context.Context, position, expression string, definition Definition) (ExpressionCheck, error)
}

// API is every part of the control plane the provider uses.
type API struct {
	Workspaces   Resources[WorkspaceInput, Workspace]
	Teams        Resources[TeamInput, Team]
	Lines        Resources[Definition, Line]
	Skills       Resources[SkillInput, Skill]
	Integrations Resources[Definition, Integration]
	Expressions  Expressions
}
