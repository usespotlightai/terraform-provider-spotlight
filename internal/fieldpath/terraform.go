package fieldpath

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
)

// Located is the Terraform attribute that an API path identifies, as far as
// the schema can address it. It also contains the address as Terraform users
// write it.
type Located struct {
	Path    path.Path
	Written string
}

// IsRoot reports if the path identifies the whole resource.
func (l Located) IsRoot() bool { return len(l.Path.Steps()) == 0 }

// Locator finds the Terraform attribute that an API path identifies. It
// changes names to snake_case, but Renames can set a different name, for
// example for a list of blocks (stations becomes station). Locator puts Under
// before each API path, for resources whose API body is one Terraform
// attribute. In a dynamic attribute, Locator does not change the names. A
// step that the schema cannot address, for example an element of a set,
// stops the path at its parent.
type Locator struct {
	Schema  attr.Type
	Renames map[string]string
	Under   Path
}

// Locate finds the attribute that the path in written identifies. If it
// cannot parse written, it returns the whole resource.
func (l Locator) Locate(written string) Located {
	parsed, err := Parse(written)
	if err != nil {
		return Located{Path: path.Empty()}
	}

	return l.LocatePath(parsed)
}

// LocatePath finds the attribute that a parsed path identifies.
func (l Locator) LocatePath(api Path) Located {
	at := path.Empty()
	var written strings.Builder
	current := l.Schema

	for _, step := range l.Under.Then(api...) {
		if step.IsAny() {
			break
		}

		next, ok := l.stepInto(current, step)
		if !ok {
			break
		}

		switch {
		case next.name != "":
			at = at.AtName(next.name)
			writeName(&written, next.name)
		case next.key != "":
			at = at.AtMapKey(next.key)
			writeKey(&written, next.key)
		default:
			at = at.AtListIndex(step.Index)
			fmt.Fprintf(&written, "[%d]", step.Index)
		}

		current = next.into
	}

	return Located{Path: at, Written: written.String()}
}

type stepped struct {
	name string
	key  string
	into attr.Type
}

func (l Locator) stepInto(current attr.Type, step Segment) (stepped, bool) {
	switch typed := current.(type) {
	case basetypes.DynamicType, nil:
		if step.IsIndex() {
			return stepped{into: basetypes.DynamicType{}}, true
		}

		return stepped{name: step.Key, into: basetypes.DynamicType{}}, true
	case basetypes.ObjectType:
		if step.IsIndex() {
			return stepped{}, false
		}

		name := l.nameOf(step.Key)
		into, ok := typed.AttrTypes[name]

		return stepped{name: name, into: into}, ok
	case basetypes.ListType:
		return stepped{into: typed.ElemType}, step.IsIndex()
	case basetypes.MapType:
		return stepped{key: step.Key, into: typed.ElemType}, !step.IsIndex() && step.Key != ""
	default:
		return stepped{}, false
	}
}

func (l Locator) nameOf(key string) string {
	if renamed, ok := l.Renames[key]; ok {
		return renamed
	}

	return SnakeCase(key)
}

// SnakeCase writes a camelCase API name as a Terraform attribute name.
func SnakeCase(name string) string {
	var out strings.Builder

	for _, r := range name {
		if unicode.IsUpper(r) {
			out.WriteByte('_')
			out.WriteRune(unicode.ToLower(r))

			continue
		}

		out.WriteRune(r)
	}

	return out.String()
}

func writeName(out *strings.Builder, name string) {
	if !IsIdentifier(name) {
		writeKey(out, name)

		return
	}

	if out.Len() > 0 {
		out.WriteByte('.')
	}

	out.WriteString(name)
}

func writeKey(out *strings.Builder, key string) {
	quoted, _ := json.Marshal(key)
	fmt.Fprintf(out, "[%s]", quoted)
}
