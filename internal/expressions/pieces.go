package expressions

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/client"
)

var (
	whole = regexp.MustCompile(`(?s)^\s*\$\{\{(.+)\}\}\s*$`)
	piece = regexp.MustCompile(`\$\{\{([\s\S]*?)\}\}`)
)

// Pieces lists the bare CEL expressions in a string, in the order of the
// string. If the whole string is one ${{ }}, it lists the whole string. If
// not, it lists each ${{ }} in the string. A string with no ${{ }} contains no
// expression.
func Pieces(text string) []string {
	if found := whole.FindStringSubmatch(text); found != nil {
		return []string{strings.TrimSpace(found[1])}
	}

	var pieces []string
	for _, found := range piece.FindAllStringSubmatch(text, -1) {
		pieces = append(pieces, strings.TrimSpace(found[1]))
	}

	return pieces
}

func explained(expression string, found client.ExpressionError) string {
	var out strings.Builder

	out.WriteString(found.Message)

	if found.Span != nil {
		runes := []rune(expression)
		start := min(max(found.Span.Start, 0), len(runes))
		end := min(max(found.Span.End, start+1), len(runes)+1)

		fmt.Fprintf(&out, "\n\n  %s\n  %s%s", expression, strings.Repeat(" ", start), strings.Repeat("^", end-start))
	}

	if found.Suggestion != "" {
		fmt.Fprintf(&out, "\n\nDid you mean %s?", found.Suggestion)
	}

	return out.String()
}
