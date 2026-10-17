package provider

import (
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/client"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/fieldpath"
)

const warning = "warning"

const oneTokenForAll = "One token granted configure, secrets, runners and trusts manages everything Terraform can in its workspace."

const unexplained = "The API token's scopes, or the workspace it belongs to, do not allow this. A token acts only inside its own workspace."

var lacking = map[client.Scope]string{
	client.ScopeRead:      "The API token lacks the read scope, which reading anything in the workspace needs.",
	client.ScopePlan:      "The API token lacks the plan scope, which checking expressions needs; grant it plan or configure.",
	client.ScopeConfigure: "The API token lacks the configure scope, which changing the workspace's configuration needs.",
	client.ScopeSecrets:   "The API token lacks the secrets scope, which creating, rebinding, rotating and deleting secrets needs; grant it secrets beside configure.",
	client.ScopeRunners:   "The API token lacks the runners scope, which creating, rekeying and deleting runner pools and runner trusts, and creating, updating and deleting the workspace's runners, needs; grant it runners beside configure.",
	client.ScopeTrusts: "The API token lacks the trusts scope, which creating, updating and deleting workspace token trusts needs, or a scope the trust gives. " +
		"A token can give a trust only the scopes that it has. A token can change a trust only when it has the current scopes and the new scopes of the trust. Grant it trusts beside configure and every scope the trusts give.",
}

var beyondTokens = map[client.Permission]string{
	client.PermissionAdminister:       "Only the workspace's admins may do this, and a token never administers; a person does it in the web app.",
	client.PermissionCreateWorkspaces: "A token acts only inside its own workspace and cannot create one; a person creates a workspace in the web app.",
}

var titled = []struct {
	prefix     string
	permission client.Permission
}{
	{"Not allowed to plan", client.PermissionPlan},
	{"Not allowed to administer", client.PermissionAdminister},
	{"Not allowed to create a workspace", client.PermissionCreateWorkspaces},
}

func forbiddenDetail(problem *client.Problem, needs client.Scope) string {
	return withDetail(problem, lacked(permissionOf(problem), needs)+" "+oneTokenForAll)
}

func permissionOf(problem *client.Problem) client.Permission {
	if problem.Permission != "" {
		return problem.Permission
	}

	for _, each := range titled {
		if strings.HasPrefix(problem.Title, each.prefix) {
			return each.permission
		}
	}

	return ""
}

func lacked(permission client.Permission, needs client.Scope) string {
	if permission == "" {
		return advised(lacking, needs)
	}

	if scope, ok := permission.Scope(); ok {
		return advised(lacking, scope)
	}

	return advised(beyondTokens, permission)
}

func advised[K comparable](table map[K]string, key K) string {
	if advice, ok := table[key]; ok {
		return advice
	}

	return unexplained
}

// addAPIError adds the diagnostics for a failed call. It adds each field
// error of a problem on the attribute that the error identifies, and all
// other errors on the resource. needs is the scope that the call must have.
// The error tells this scope when the token does not have it.
func addAPIError(diags *diag.Diagnostics, doing string, err error, at fieldpath.Locator, needs client.Scope) {
	problem, ok := client.ProblemOf(err)
	if !ok {
		diags.AddError(doing, err.Error())

		return
	}

	if client.IsForbidden(problem) {
		diags.AddError(doing+": "+problem.Title, forbiddenDetail(problem, needs))

		return
	}

	if len(problem.Errors) == 0 {
		diags.AddError(doing, explainedProblem(problem))

		return
	}

	for _, each := range problem.Errors {
		located := at.Locate(each.Path)
		summary := fmt.Sprintf("%s: %s", doing, problem.Title)

		switch {
		case located.IsRoot() && each.Severity == warning:
			diags.AddWarning(summary, each.Message)
		case located.IsRoot():
			diags.AddError(summary, each.Message)
		case each.Severity == warning:
			diags.AddAttributeWarning(located.Path, summary, located.Written+": "+each.Message)
		default:
			diags.AddAttributeError(located.Path, summary, located.Written+": "+each.Message)
		}
	}
}

// addAPIWarnings adds the warnings in the answer for a resource. It adds each
// warning on the attribute that the warning identifies.
func addAPIWarnings(diags *diag.Diagnostics, kind string, warnings []client.FieldError, at fieldpath.Locator) {
	for _, each := range warnings {
		located := at.Locate(each.Path)
		summary := fmt.Sprintf("Spotlight has a suggestion for this %s", kind)

		if located.IsRoot() {
			diags.AddWarning(summary, each.Message)

			continue
		}

		diags.AddAttributeWarning(located.Path, summary, located.Written+": "+each.Message)
	}
}

func explainedProblem(problem *client.Problem) string {
	switch problem.Type {
	case client.ProblemVersionConflict:
		return withDetail(problem, "Something changed it since Terraform last read it. Run terraform plan again to see the change, then apply.")
	case client.ProblemAlreadyExists:
		return withDetail(problem, "Import it with an import block or terraform import to manage it from here.")
	default:
		if problem.Detail == "" {
			return problem.Title
		}

		return problem.Detail
	}
}

func withDetail(problem *client.Problem, advice string) string {
	if problem.Detail == "" {
		return advice
	}

	return problem.Detail + "\n\n" + advice
}
