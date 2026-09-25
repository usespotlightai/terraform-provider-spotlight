package client

// Scope is what an API token is granted. Read, plan and configure are a
// ladder, each allowing what the one before does; secrets, runners and
// trusts are granted beside one of them.
type Scope string

// The scopes a token may carry.
const (
	ScopeRead      Scope = "read"
	ScopePlan      Scope = "plan"
	ScopeConfigure Scope = "configure"
	ScopeSecrets   Scope = "secrets"
	ScopeRunners   Scope = "runners"
	ScopeTrusts    Scope = "trusts"
)

// Permission is what a refused caller lacked, as a 403 problem names it.
type Permission string

// The permissions a 403 problem may name.
const (
	PermissionRead           Permission = "read"
	PermissionPlan           Permission = "plan"
	PermissionConfigure      Permission = "configure"
	PermissionManageSecrets  Permission = "manage-secrets"
	PermissionManageRunners  Permission = "manage-runners"
	PermissionManageTrusts   Permission = "manage-token-trusts"
	PermissionAdminister     Permission = "administer"
	PermissionOpenWorkspaces Permission = "open-workspaces"
)

var grantedBy = map[Permission]Scope{
	PermissionRead:          ScopeRead,
	PermissionPlan:          ScopePlan,
	PermissionConfigure:     ScopeConfigure,
	PermissionManageSecrets: ScopeSecrets,
	PermissionManageRunners: ScopeRunners,
	PermissionManageTrusts:  ScopeTrusts,
}

// Scope answers the token scope that grants the permission, and false for
// one no token is ever granted.
func (p Permission) Scope() (Scope, bool) {
	scope, ok := grantedBy[p]

	return scope, ok
}
