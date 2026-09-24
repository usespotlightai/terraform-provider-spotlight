package resourcekit

import (
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/fieldpath"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/spotlight"
)

const warning = "warning"

const oneTokenForAll = "One token granted configure, secrets and runners manages everything Terraform can in its workspace."

const unexplained = "The API token's scopes, or the workspace it belongs to, do not allow this. A token acts only inside its own workspace."

var lacking = map[spotlight.Scope]string{
	spotlight.ScopeRead:      "The API token lacks the read scope, which reading anything in the workspace needs.",
	spotlight.ScopePlan:      "The API token lacks the plan scope, which checking expressions needs; grant it plan or configure.",
	spotlight.ScopeConfigure: "The API token lacks the configure scope, which changing the workspace's configuration needs.",
	spotlight.ScopeSecrets:   "The API token lacks the secrets scope, which setting, rebinding, rotating and deleting secrets needs; grant it secrets beside configure.",
	spotlight.ScopeRunners:   "The API token lacks the runners scope, which enrolling, rekeying and deleting runners and runner trusts, and declaring the workspace's runners, needs; grant it runners beside configure.",
}

var beyondTokens = map[spotlight.Permission]string{
	spotlight.PermissionAdminister:     "Only the workspace's admins may do this, and a token never administers; a person does it in the web app.",
	spotlight.PermissionOpenWorkspaces: "A token acts only inside its own workspace and cannot open one; a person creates a workspace in the web app.",
}

var titled = []struct {
	prefix     string
	permission spotlight.Permission
}{
	{"Not allowed to plan", spotlight.PermissionPlan},
	{"Not allowed to administer", spotlight.PermissionAdminister},
	{"Not allowed to open a workspace", spotlight.PermissionOpenWorkspaces},
}

// Refused explains a 403: what the caller lacked for what it tried. The
// permission the problem names says, when it names one; otherwise needs
// does, the scope a change to this kind of resource takes.
func Refused(problem *spotlight.Problem, needs spotlight.Scope) string {
	return withDetail(problem, lacked(permissionOf(problem), needs)+" "+oneTokenForAll)
}

func permissionOf(problem *spotlight.Problem) spotlight.Permission {
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

func lacked(permission spotlight.Permission, needs spotlight.Scope) string {
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

// Failed adds the diagnostics for a failed call: each field error of a
// problem on the attribute it names, anything else on the resource. needs
// is the scope the call takes, named when the token is refused it.
func Failed(diags *diag.Diagnostics, doing string, err error, at fieldpath.Locator, needs spotlight.Scope) {
	problem, ok := spotlight.ProblemOf(err)
	if !ok {
		diags.AddError(doing, err.Error())

		return
	}

	if spotlight.IsForbidden(problem) {
		diags.AddError(doing+": "+problem.Title, Refused(problem, needs))

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

// Warned adds the warnings a resource was answered with, each on the
// attribute it names.
func Warned(diags *diag.Diagnostics, kind string, warnings []spotlight.FieldError, at fieldpath.Locator) {
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

func explainedProblem(problem *spotlight.Problem) string {
	switch {
	case problem.Type == spotlight.ProblemMovedOn:
		return withDetail(problem, "Something changed it since Terraform last read it. Run terraform plan again to see the change, then apply.")
	case problem.Type == spotlight.ProblemAlreadyExists:
		return withDetail(problem, "Import it with an import block or terraform import to manage it from here.")
	default:
		if problem.Detail == "" {
			return problem.Title
		}

		return problem.Detail
	}
}

func withDetail(problem *spotlight.Problem, advice string) string {
	if problem.Detail == "" {
		return advice
	}

	return problem.Detail + "\n\n" + advice
}
