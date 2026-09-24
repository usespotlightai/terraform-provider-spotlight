package resourcekit

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/fieldpath"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/spotlight"
)

const warning = "warning"

// Failed adds the diagnostics for a failed call: each field error of a
// problem on the attribute it names, anything else on the resource.
func Failed(diags *diag.Diagnostics, doing string, err error, at fieldpath.Locator) {
	problem, ok := spotlight.ProblemOf(err)
	if !ok {
		diags.AddError(doing, err.Error())

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
	switch problem.Type {
	case spotlight.ProblemMovedOn:
		return problem.Detail + "\n\nSomething changed it since Terraform last read it. Run terraform plan again to see the change, then apply."
	case spotlight.ProblemAlreadyExists:
		return problem.Detail + "\n\nImport it with an import block or terraform import to manage it from here."
	default:
		if problem.Detail == "" {
			return problem.Title
		}

		return problem.Detail
	}
}
