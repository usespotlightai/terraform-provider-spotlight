// Package expressions type-checks the CEL expressions in the definition of
// an integration or in a hook during a plan. It uses the check endpoint of
// the control plane. It attaches each result to the attribute that holds the
// expression.
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

	"github.com/usespotlightai/terraform-provider-spotlight/internal/client"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/dynamic"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/fieldpath"
)

const integration = "integration"

// Attribute is the Terraform attribute that holds the definition of an
// integration.
const Attribute = "definition"

// API is the part of the control plane that checks expressions.
type API interface {
	ExpressionPositions(ctx context.Context) ([]client.Position, error)
	CheckExpression(ctx context.Context, position, expression string, definition client.Definition) (client.ExpressionCheck, error)
}

// Checker checks the expressions in definitions. After it reads the
// positions that can contain expressions, it keeps them.
type Checker struct {
	api       API
	mu        sync.Mutex
	positions []fieldpath.Path
}

// NewChecker returns a Checker that asks api.
func NewChecker(api API) *Checker {
	return &Checker{api: api}
}

var inDefinition = fieldpath.Locator{Under: fieldpath.Path{fieldpath.Key(Attribute)}}

// Subject is a definition with expressions to check. It contains:
//   - Resource, the resource name of its positions.
//   - Fields, which finds the attribute of each path in the definition.
//   - Definition, which the checker sends with each expression. If the
//     definition is not Complete, it does not contain the unknown parts.
//   - Written, the expressions in the definition.
type Subject struct {
	Resource   string
	Fields     fieldpath.Locator
	Definition client.Definition
	Complete   bool
	Written    []Written
}

// Written is the expressions in a string, and the position of the string in
// its definition.
type Written struct {
	At          fieldpath.Path
	Expressions []string
}

// Check type-checks each known expression in the definition of an
// integration. It returns an error or a warning on the attribute of each
// expression that has a problem.
func (c *Checker) Check(ctx context.Context, definition types.Dynamic) diag.Diagnostics {
	if definition.IsNull() || definition.IsUnknown() || definition.IsUnderlyingValueUnknown() {
		return nil
	}

	held, complete := dynamic.ToJSONKnown(definition)
	document, ok := held.(map[string]any)
	if !ok {
		return nil
	}

	return c.CheckSubject(ctx, Subject{
		Resource:   integration,
		Fields:     inDefinition,
		Definition: document,
		Complete:   complete,
		Written:    TemplatesIn(definition.UnderlyingValue(), fieldpath.Path{}),
	})
}

// CheckSubject type-checks each expression in subject that is at a position
// for expressions. It returns an error or a warning on the attribute of each
// expression that has a problem.
func (c *Checker) CheckSubject(ctx context.Context, subject Subject) diag.Diagnostics {
	var diags diag.Diagnostics

	positions, err := c.known(ctx)
	if err != nil {
		diags.AddWarning("Expressions were not checked", fmt.Sprintf("Spotlight could not list where expressions are written: %s", err))

		return diags
	}

	warned := map[string]bool{}
	head := fieldpath.Path{fieldpath.Key(subject.Resource)}

	for _, written := range subject.Written {
		position := head.Then(written.At...)
		if !anyHolds(positions, position) {
			continue
		}

		for _, expression := range written.Expressions {
			check, err := c.api.CheckExpression(ctx, position.String(), expression, subject.Definition)
			if err != nil {
				stop := refused(ctx, err, subject, written, &diags)
				if stop {
					return diags
				}

				continue
			}

			for _, found := range check.Errors {
				at := subject.Fields.LocatePath(written.At)
				diags.AddAttributeError(at.Path, "Invalid expression in "+at.Written, explained(expression, found))
			}

			for _, warning := range check.Warnings {
				key := warning.Position + "\x00" + warning.Message
				if warned[key] {
					continue
				}
				warned[key] = true

				at := warningAt(subject, warning.Position, written.At)
				diags.AddAttributeWarning(at.Path, "Expression warning in "+at.Written, warning.Message)
			}
		}
	}

	return diags
}

func refused(ctx context.Context, err error, subject Subject, written Written, diags *diag.Diagnostics) bool {
	if client.IsForbidden(err) {
		diags.AddWarning("Expressions were not checked", "The API token's scope does not allow checking expressions. Checking is planning, which needs the plan scope or above; the apply checks them instead.")

		return true
	}

	problem, ok := client.ProblemOf(err)
	if !ok || problem.Type != client.ProblemInvalid {
		diags.AddWarning("Expressions were not checked", fmt.Sprintf("Spotlight could not check the expressions in this definition: %s", err))

		return true
	}

	var aboutDefinition []client.FieldError
	for _, each := range problem.Errors {
		if parsed, err := fieldpath.Parse(each.Path); err == nil && len(parsed) > 0 && parsed[0].Key == Attribute {
			aboutDefinition = append(aboutDefinition, client.FieldError{Path: parsed[1:].String(), Message: each.Message})
		}
	}

	if len(aboutDefinition) == 0 {
		tflog.Debug(ctx, "the check endpoint found no expression where one was expected", map[string]any{
			"position": written.At.String(),
			"detail":   problem.Detail,
		})

		return false
	}

	if subject.Complete {
		for _, each := range aboutDefinition {
			at := subject.Fields.Locate(each.Path)
			diags.AddAttributeError(at.Path, "Invalid definition at "+at.Written, each.Message)
		}
	}

	return true
}

func warningAt(subject Subject, position string, fallback fieldpath.Path) fieldpath.Located {
	parsed, err := fieldpath.Parse(position)
	if err != nil || len(parsed) == 0 || parsed[0].Key != subject.Resource {
		return subject.Fields.LocatePath(fallback)
	}

	return subject.Fields.LocatePath(parsed[1:])
}

func (c *Checker) known(ctx context.Context) ([]fieldpath.Path, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.positions != nil {
		return c.positions, nil
	}

	listed, err := c.api.ExpressionPositions(ctx)
	if err != nil {
		return nil, err
	}

	positions := make([]fieldpath.Path, 0, len(listed))
	for _, each := range listed {
		parsed, err := fieldpath.Parse(each.Position)
		if err != nil || len(parsed) < 2 {
			continue
		}
		positions = append(positions, parsed)
	}

	c.positions = positions

	return positions, nil
}

// TemplatesIn lists the ${{ }} expressions in each known string in value,
// with the path of the string under at.
func TemplatesIn(value attr.Value, at fieldpath.Path) []Written {
	var found []Written

	walk(value, at, func(at fieldpath.Path, text string) {
		if pieces := Pieces(text); len(pieces) > 0 {
			found = append(found, Written{At: at, Expressions: pieces})
		}
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
