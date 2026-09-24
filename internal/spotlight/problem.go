package spotlight

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// Problem types the API answers, stable and safe to branch on.
const (
	ProblemInvalid       = "urn:spotlight:problem:invalid"
	ProblemMovedOn       = "urn:spotlight:problem:moved-on"
	ProblemAlreadyExists = "urn:spotlight:problem:already-exists"
	ProblemInUse         = "urn:spotlight:problem:in-use"
	ProblemNotFound      = "urn:spotlight:problem:not-found"
)

// Problem is an RFC 9457 problem the API answered.
type Problem struct {
	Status         int
	Type           string
	Title          string
	Detail         string
	Errors         []FieldError
	CurrentVersion *int64
}

// Error writes the problem with each field error it names.
func (p *Problem) Error() string {
	parts := []string{fmt.Sprintf("%d %s", p.Status, p.Title)}

	if p.Detail != "" && len(p.Errors) == 0 {
		parts = append(parts, p.Detail)
	}

	for _, each := range p.Errors {
		if each.Path == "" {
			parts = append(parts, each.Message)
		} else {
			parts = append(parts, each.Path+": "+each.Message)
		}
	}

	return strings.Join(parts, "\n")
}

// ProblemOf returns the Problem err carries, if any.
func ProblemOf(err error) (*Problem, bool) {
	var problem *Problem

	return problem, errors.As(err, &problem)
}

// IsNotFound reports whether err says the resource is not there.
func IsNotFound(err error) bool {
	problem, ok := ProblemOf(err)

	return ok && (problem.Status == http.StatusNotFound || problem.Type == ProblemNotFound)
}
