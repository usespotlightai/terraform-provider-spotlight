package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/client/apigen"
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

// readAnswer reads the answer of a call as a T. Use it for an answer that
// the generated client cannot read.
func readAnswer[T any](resp *http.Response, err error) (T, error) {
	var zero T

	if err != nil {
		return zero, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return zero, fmt.Errorf("could not read the control plane's answer: %w", err)
	}

	var held *T
	if resp.StatusCode < http.StatusBadRequest {
		var value T
		if json.Unmarshal(body, &value) == nil {
			held = &value
		}
	}

	return answered(resp, body, held)
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

func problemOf(resp *http.Response, body []byte) *Problem {
	var held apigen.ProblemResponseDto

	if err := json.Unmarshal(body, &held); err != nil || held.Type == "" {
		detail := strings.TrimSpace(string(body))
		if detail == "" {
			detail = http.StatusText(resp.StatusCode)
		}

		return &Problem{
			Status: resp.StatusCode,
			Title:  http.StatusText(resp.StatusCode),
			Detail: detail,
		}
	}

	problem := &Problem{
		Status: resp.StatusCode,
		Type:   held.Type,
		Title:  held.Title,
		Detail: held.Detail,
	}

	if held.Permission != nil {
		problem.Permission = Permission(*held.Permission)
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

func fieldErrors(held []apigen.FieldErrorResponseDto) []FieldError {
	out := make([]FieldError, 0, len(held))

	for _, each := range held {
		out = append(out, FieldError{
			Path:     each.Path,
			Message:  each.Message,
			Severity: string(each.Severity),
		})
	}

	return out
}

func meta(id string, version float32, address string, warnings []apigen.FieldErrorResponseDto) Meta {
	return Meta{
		ID:       id,
		Version:  int64(version),
		Address:  address,
		Warnings: fieldErrors(warnings),
	}
}

func entityTag(version int64) string {
	return fmt.Sprintf("%q", fmt.Sprint(version))
}

const jsonContent = "application/json"

func jsonBody(in any) (io.Reader, error) {
	raw, err := json.Marshal(in)
	if err != nil {
		return nil, err
	}

	return bytes.NewReader(raw), nil
}

func orNone[T any](held []T) []T {
	if held == nil {
		return []T{}
	}

	return held
}
