// Package expressions type-checks the CEL expressions written in an
// integration's definition during a plan, with the control plane's check
// endpoint, and attaches what it finds to the attribute that holds each one.
package expressions

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/dynamic"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/fieldpath"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/spotlight"
)

const resource = "integration"

// Attribute is the Terraform attribute an integration's definition is held in.
const Attribute = "definition"

// Checker checks the expressions in integration definitions, remembering
// the positions expressions are written at once it has read them.
type Checker struct {
	api       spotlight.Expressions
	mu        sync.Mutex
	positions []fieldpath.Path
}

// NewChecker returns a Checker that asks api.
func NewChecker(api spotlight.Expressions) *Checker {
	return &Checker{api: api}
}

var (
	inDefinition = fieldpath.Locator{Under: fieldpath.Path{fieldpath.Key(Attribute)}}
	asWritten    = fieldpath.Locator{}
)

// Check type-checks every expression in definition that is known, and
// answers an error or warning on the attribute of each one that needs it.
func (c *Checker) Check(ctx context.Context, definition types.Dynamic) diag.Diagnostics {
	var diags diag.Diagnostics

	if definition.IsNull() || definition.IsUnknown() || definition.IsUnderlyingValueUnknown() {
		return diags
	}

	held, complete := dynamic.ToJSONKnown(definition)
	document, ok := held.(map[string]any)
	if !ok {
		return diags
	}

	positions, err := c.known(ctx)
	if err != nil {
		diags.AddWarning("Expressions were not checked", fmt.Sprintf("Spotlight could not list where expressions are written: %s", err))

		return diags
	}

	warned := map[string]bool{}

	for _, written := range writtenIn(definition.UnderlyingValue(), positions) {
		for _, expression := range written.expressions {
			check, err := c.api.Check(ctx, resource+"."+written.at.String(), expression, document)
			if err != nil {
				stop := c.refused(ctx, err, complete, written, &diags)
				if stop {
					return diags
				}

				continue
			}

			for _, found := range check.Errors {
				at := inDefinition.LocatePath(written.at)
				diags.AddAttributeError(at.Path, "Invalid expression in "+at.Written, explained(expression, found))
			}

			for _, warning := range check.Warnings {
				key := warning.Position + "\x00" + warning.Message
				if warned[key] {
					continue
				}
				warned[key] = true

				at := warningAt(warning.Position, written.at)
				diags.AddAttributeWarning(at.Path, "Expression warning in "+at.Written, warning.Message)
			}
		}
	}

	return diags
}

func (c *Checker) refused(ctx context.Context, err error, complete bool, written written, diags *diag.Diagnostics) bool {
	if spotlight.IsForbidden(err) {
		diags.AddWarning("Expressions were not checked", "The API token's scope does not allow checking expressions. Checking is planning, which needs the plan scope or above; the apply checks them instead.")

		return true
	}

	problem, ok := spotlight.ProblemOf(err)
	if !ok || problem.Type != spotlight.ProblemInvalid {
		diags.AddWarning("Expressions were not checked", fmt.Sprintf("Spotlight could not check the expressions in this definition: %s", err))

		return true
	}

	var aboutDefinition []spotlight.FieldError
	for _, each := range problem.Errors {
		if parsed, err := fieldpath.Parse(each.Path); err == nil && len(parsed) > 0 && parsed[0].Key == Attribute {
			aboutDefinition = append(aboutDefinition, each)
		}
	}

	if len(aboutDefinition) == 0 {
		tflog.Debug(ctx, "the check endpoint found no expression where one was expected", map[string]any{
			"position": written.at.String(),
			"detail":   problem.Detail,
		})

		return false
	}

	if complete {
		for _, each := range aboutDefinition {
			at := asWritten.Locate(each.Path)
			diags.AddAttributeError(at.Path, "Invalid definition at "+at.Written, each.Message)
		}
	}

	return true
}

func warningAt(position string, fallback fieldpath.Path) fieldpath.Located {
	parsed, err := fieldpath.Parse(position)
	if err != nil || len(parsed) == 0 || parsed[0].Key != resource {
		return inDefinition.LocatePath(fallback)
	}

	return inDefinition.LocatePath(parsed[1:])
}

func (c *Checker) known(ctx context.Context) ([]fieldpath.Path, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.positions != nil {
		return c.positions, nil
	}

	listed, err := c.api.Positions(ctx)
	if err != nil {
		return nil, err
	}

	positions := make([]fieldpath.Path, 0, len(listed))
	for _, each := range listed {
		parsed, err := fieldpath.Parse(each.Position)
		if err != nil || len(parsed) < 2 || parsed[0].Key != resource {
			continue
		}
		positions = append(positions, parsed[1:])
	}

	c.positions = positions

	return positions, nil
}

type written struct {
	at          fieldpath.Path
	expressions []string
}

func writtenIn(value attr.Value, positions []fieldpath.Path) []written {
	var found []written

	walk(value, fieldpath.Path{}, func(at fieldpath.Path, text string) {
		pieces := Pieces(text)
		if len(pieces) == 0 || !anyHolds(positions, at) {
			return
		}

		found = append(found, written{at: at, expressions: pieces})
	})

	return found
}

func anyHolds(positions []fieldpath.Path, at fieldpath.Path) bool {
	for _, position := range positions {
		if at.Within(position) {
			return true
		}
	}

	return false
}

func walk(value attr.Value, at fieldpath.Path, visit func(fieldpath.Path, string)) {
	if value == nil || value.IsNull() || value.IsUnknown() {
		return
	}

	switch typed := value.(type) {
	case basetypes.DynamicValue:
		walk(typed.UnderlyingValue(), at, visit)
	case basetypes.StringValue:
		visit(at, typed.ValueString())
	case basetypes.ObjectValue:
		walkNamed(typed.Attributes(), at, visit)
	case basetypes.MapValue:
		walkNamed(typed.Elements(), at, visit)
	case basetypes.TupleValue:
		walkIndexed(typed.Elements(), at, visit)
	case basetypes.ListValue:
		walkIndexed(typed.Elements(), at, visit)
	case basetypes.SetValue:
		walkIndexed(typed.Elements(), at, visit)
	}
}

func walkNamed(held map[string]attr.Value, at fieldpath.Path, visit func(fieldpath.Path, string)) {
	names := make([]string, 0, len(held))
	for name := range held {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		walk(held[name], at.Then(fieldpath.Key(name)), visit)
	}
}

func walkIndexed(held []attr.Value, at fieldpath.Path, visit func(fieldpath.Path, string)) {
	for index, each := range held {
		walk(each, at.Then(fieldpath.Index(index)), visit)
	}
}
