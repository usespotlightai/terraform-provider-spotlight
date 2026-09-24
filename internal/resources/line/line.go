// Package line is the spotlight_line resource.
package line

import (
	"github.com/hashicorp/terraform-plugin-framework/resource"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/resourcekit"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/spotlight"
)

const kind = "line"

// Blocks are the API's lists of objects Terraform writes as repeated blocks.
var Blocks = map[string]string{
	"stations":  "station",
	"reactions": "reaction",
}

// New returns the spotlight_line resource.
func New() resource.Resource {
	return resourcekit.NewManaged(resourcekit.Spec[spotlight.Definition, spotlight.Line, model]{
		Kind:        kind,
		Schema:      schemaOf,
		Renames:     Blocks,
		AddressedAs: "core/delivery",
		Resources:   func(api spotlight.API) spotlight.Resources[spotlight.Definition, spotlight.Line] { return api.Lines },
		Owner:       func(m model) string { return m.TeamID.ValueString() },
		Input:       definitionOf,
		Stored:      stored,
		Meta:        func(l spotlight.Line) spotlight.Meta { return l.Meta },
		Settable:    func(l spotlight.Line) any { return l.Definition },
		Identity:    func(m model) (string, int64) { return m.ID.ValueString(), m.Version.ValueInt64() },
	})
}
