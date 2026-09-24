// Package fieldpath reads the attribute paths the Spotlight API writes, as in
// stations[2].passesTo or on["task.opened"].to.thread, and finds the
// Terraform attribute each one names.
package fieldpath

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type kind int

const (
	named kind = iota
	indexed
	anything
)

// Segment is one step of a path: a name or key, an index, or [*].
type Segment struct {
	kind  kind
	Key   string
	Index int
}

// Key is a step to an attribute or map key.
func Key(key string) Segment { return Segment{kind: named, Key: key} }

// Index is a step to a list element.
func Index(index int) Segment { return Segment{kind: indexed, Index: index} }

// Any is the [*] step of a pattern, matching any one step.
func Any() Segment { return Segment{kind: anything} }

// IsIndex reports whether the step is a list index.
func (s Segment) IsIndex() bool { return s.kind == indexed }

// IsAny reports whether the step is [*].
func (s Segment) IsAny() bool { return s.kind == anything }

// Path is a sequence of steps from the root of a resource.
type Path []Segment

var (
	identifier = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]*$`)
	first      = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]*`)
	next       = regexp.MustCompile(`^(?:\.([A-Za-z_][A-Za-z0-9_-]*)|\[(\d+)\]|\[(\*)\]|\[("(?:[^"\\]|\\.)*")\])`)
)

// IsIdentifier reports whether a key is written bare rather than quoted.
func IsIdentifier(key string) bool { return identifier.MatchString(key) }

// Parse reads a path as the API writes it. The empty path is the root.
func Parse(written string) (Path, error) {
	written = strings.TrimSpace(written)
	if written == "" {
		return Path{}, nil
	}

	head := first.FindString(written)
	if head == "" {
		return nil, fmt.Errorf("%q does not start with a name", written)
	}

	path := Path{Key(head)}
	rest := written[len(head):]

	for rest != "" {
		found := next.FindStringSubmatch(rest)
		if found == nil {
			return nil, fmt.Errorf("%q is not a path, at %q", written, rest)
		}

		switch {
		case found[1] != "":
			path = append(path, Key(found[1]))
		case found[2] != "":
			index, err := strconv.Atoi(found[2])
			if err != nil {
				return nil, err
			}
			path = append(path, Index(index))
		case found[3] != "":
			path = append(path, Any())
		default:
			var key string
			if err := json.Unmarshal([]byte(found[4]), &key); err != nil {
				return nil, fmt.Errorf("%q quotes a key badly: %w", written, err)
			}
			path = append(path, Key(key))
		}

		rest = rest[len(found[0]):]
	}

	return path, nil
}

// String writes the path as the API does.
func (p Path) String() string {
	var out strings.Builder

	for at, each := range p {
		switch {
		case each.IsAny():
			out.WriteString("[*]")
		case each.IsIndex():
			fmt.Fprintf(&out, "[%d]", each.Index)
		case IsIdentifier(each.Key):
			if at > 0 {
				out.WriteByte('.')
			}
			out.WriteString(each.Key)
		default:
			quoted, _ := json.Marshal(each.Key)
			fmt.Fprintf(&out, "[%s]", quoted)
		}
	}

	return out.String()
}

// Then returns the path extended by more steps.
func (p Path) Then(more ...Segment) Path {
	out := make(Path, 0, len(p)+len(more))

	return append(append(out, p...), more...)
}

// Within reports whether the path is at or inside what pattern names, [*]
// matching any one step.
func (p Path) Within(pattern Path) bool {
	if len(p) < len(pattern) {
		return false
	}

	for at, want := range pattern {
		have := p[at]

		switch {
		case want.IsAny():
			continue
		case want.IsIndex() != have.IsIndex():
			return false
		case want.IsIndex() && want.Index != have.Index:
			return false
		case !want.IsIndex() && want.Key != have.Key:
			return false
		}
	}

	return true
}
