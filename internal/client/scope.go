package client

// Scope is a grant of an API token. Read, plan and configure are a ladder.
// Each scope lets the token do what the scope before it lets. Secrets,
// runners and trusts are granted in addition to one of them.
type Scope string

// The scopes that a token can have.
const (
	ScopeRead      Scope = "read"
	ScopePlan      Scope = "plan"
	ScopeConfigure Scope = "configure"
	ScopeSecrets   Scope = "secrets"
	ScopeRunners   Scope = "runners"
	ScopeTrusts    Scope = "trusts"
)

// Permission is the permission that a forbidden caller did not have, as a 403
// problem identifies it.
type Permission string

// The permissions that a 403 problem can identify.
const (
	PermissionRead             Permission = "read"
	PermissionPlan             Permission = "plan"
	PermissionConfigure        Permission = "configure"
	PermissionSecrets          Permission = "secrets"
	PermissionRunners          Permission = "runners"
	PermissionTrusts           Permission = "trusts"
	PermissionAdminister       Permission = "administer"
	PermissionCreateWorkspaces Permission = "create-workspaces"
)

var grantedBy = map[Permission]Scope{
	PermissionRead:      ScopeRead,
	PermissionPlan:      ScopePlan,
	PermissionConfigure: ScopeConfigure,
	PermissionSecrets:   ScopeSecrets,
	PermissionRunners:   ScopeRunners,
	PermissionTrusts:    ScopeTrusts,
}

// Scope returns the token scope that grants the permission. It returns false
// for a permission that no token can have.
func (p Permission) Scope() (Scope, bool) {
	scope, ok := grantedBy[p]

	return scope, ok
}
