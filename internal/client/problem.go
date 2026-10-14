package client

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// Problem types that the API returns. They are stable and safe to branch on.
const (
	ProblemInvalid       = "urn:spotlight:problem:invalid"
	ProblemMovedOn       = "urn:spotlight:problem:moved-on"
	ProblemAlreadyExists = "urn:spotlight:problem:already-exists"
	ProblemInUse         = "urn:spotlight:problem:in-use"
	ProblemNotFound      = "urn:spotlight:problem:not-found"
	ProblemForbidden     = "urn:spotlight:problem:forbidden"
)

// Problem is an RFC 9457 problem that the API returned. Permission is the
// permission that a 403 says the caller does not have. It is empty if the
// 403 does not identify a permission.
type Problem struct {
	Status         int
	Type           string
	Title          string
	Detail         string
	Errors         []FieldError
	CurrentVersion *int64
	Permission     Permission
}

// Error writes the problem with each field error that it contains.
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

// ProblemOf returns the Problem in err, if err contains one.
func ProblemOf(err error) (*Problem, bool) {
	var problem *Problem

	return problem, errors.As(err, &problem)
}

// IsNotFound reports if err says that the resource is not there.
func IsNotFound(err error) bool {
	problem, ok := ProblemOf(err)

	return ok && (problem.Status == http.StatusNotFound || problem.Type == ProblemNotFound)
}

// IsForbidden reports if err says that the caller cannot do this operation.
func IsForbidden(err error) bool {
	problem, ok := ProblemOf(err)

	return ok && (problem.Status == http.StatusForbidden || problem.Type == ProblemForbidden)
}
