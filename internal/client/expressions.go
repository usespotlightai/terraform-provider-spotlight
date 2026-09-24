package client

import (
	"context"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/client/apigen"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/spotlight"
)

type expressions struct{ api *apigen.ClientWithResponses }

func (e expressions) Positions(ctx context.Context) ([]spotlight.Position, error) {
	resp, err := e.api.ListExpressionPositionsWithResponse(ctx, &apigen.ListExpressionPositionsParams{})
	if err != nil {
		return nil, err
	}

	held, err := answered(resp.HTTPResponse, resp.Body, resp.JSON200)
	if err != nil {
		return nil, err
	}

	positions := make([]spotlight.Position, 0, len(held))
	for _, each := range held {
		position := spotlight.Position{Position: each.Position}
		if each.Returns != nil {
			position.Returns = string(*each.Returns)
		}
		positions = append(positions, position)
	}

	return positions, nil
}

func (e expressions) Check(ctx context.Context, position, expression string, definition spotlight.Definition) (spotlight.ExpressionCheck, error) {
	resp, err := e.api.CheckExpressionWithResponse(ctx, &apigen.CheckExpressionParams{}, apigen.CheckExpressionJSONRequestBody{
		Position:   position,
		Expression: expression,
		Definition: definition,
	})
	if err != nil {
		return spotlight.ExpressionCheck{}, err
	}

	held, err := answered(resp.HTTPResponse, resp.Body, resp.JSON200)
	if err != nil {
		return spotlight.ExpressionCheck{}, err
	}

	check := spotlight.ExpressionCheck{}
	if held.Type != nil {
		check.Type = *held.Type
	}

	for _, each := range held.Errors {
		found := spotlight.ExpressionError{Message: each.Message}
		if each.Span != nil {
			found.Span = &spotlight.Span{Start: int(each.Span.Start), End: int(each.Span.End)}
		}
		if each.Suggestion != nil {
			found.Suggestion = *each.Suggestion
		}
		check.Errors = append(check.Errors, found)
	}

	for _, each := range held.Warnings {
		check.Warnings = append(check.Warnings, spotlight.ExpressionWarning{Position: each.Position, Message: each.Message})
	}

	return check, nil
}
