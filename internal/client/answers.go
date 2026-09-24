package client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/client/apigen"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/spotlight"
)

func answered[T any](resp *http.Response, body []byte, value *T) (T, error) {
	var zero T

	if resp == nil {
		return zero, fmt.Errorf("the control plane gave no answer")
	}

	if resp.StatusCode >= http.StatusBadRequest {
		return zero, problemOf(resp, body)
	}

	if value == nil {
		return zero, fmt.Errorf("the control plane answered %d with a body this client cannot read", resp.StatusCode)
	}

	return *value, nil
}

func done(resp *http.Response, body []byte) error {
	if resp == nil {
		return fmt.Errorf("the control plane gave no answer")
	}

	if resp.StatusCode >= http.StatusBadRequest {
		return problemOf(resp, body)
	}

	return nil
}

func problemOf(resp *http.Response, body []byte) *spotlight.Problem {
	var held apigen.ProblemResponseDto

	if err := json.Unmarshal(body, &held); err != nil || held.Type == "" {
		detail := strings.TrimSpace(string(body))
		if detail == "" {
			detail = http.StatusText(resp.StatusCode)
		}

		return &spotlight.Problem{
			Status: resp.StatusCode,
			Title:  http.StatusText(resp.StatusCode),
			Detail: detail,
		}
	}

	problem := &spotlight.Problem{
		Status: resp.StatusCode,
		Type:   held.Type,
		Title:  held.Title,
		Detail: held.Detail,
	}

	if held.Errors != nil {
		problem.Errors = fieldErrors(*held.Errors)
	}

	if held.CurrentVersion != nil {
		version := int64(*held.CurrentVersion)
		problem.CurrentVersion = &version
	}

	return problem
}

func fieldErrors(held []apigen.FieldErrorResponseDto) []spotlight.FieldError {
	out := make([]spotlight.FieldError, 0, len(held))

	for _, each := range held {
		out = append(out, spotlight.FieldError{
			Path:     each.Path,
			Message:  each.Message,
			Severity: string(each.Severity),
		})
	}

	return out
}

func meta(id string, version float32, address string, warnings []apigen.FieldErrorResponseDto) spotlight.Meta {
	return spotlight.Meta{
		ID:       id,
		Version:  int64(version),
		Address:  address,
		Warnings: fieldErrors(warnings),
	}
}

func entityTag(version int64) string {
	return fmt.Sprintf("%q", fmt.Sprint(version))
}
