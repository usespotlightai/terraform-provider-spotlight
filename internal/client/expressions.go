package client

import (
	"context"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/client/apigen"
)

// ExpressionPositions lists the positions in a definition that can contain
// expressions.
func (c *Client) ExpressionPositions(ctx context.Context) ([]Position, error) {
	resp, err := c.api.ListExpressionPositionsWithResponse(ctx, &apigen.ListExpressionPositionsParams{})
	if err != nil {
		return nil, err
	}

	held, err := answered(resp.HTTPResponse, resp.Body, resp.JSON200)
	if err != nil {
		return nil, err
	}

	positions := make([]Position, 0, len(held))
	for _, each := range held {
		position := Position{Position: each.Position}
		if each.Returns != nil {
			position.Returns = string(*each.Returns)
		}
		positions = append(positions, position)
	}

	return positions, nil
}

// CheckExpression type-checks an expression at its position in a definition.
func (c *Client) CheckExpression(ctx context.Context, position, expression string, definition Definition) (ExpressionCheck, error) {
	resp, err := c.api.CheckExpressionWithResponse(ctx, &apigen.CheckExpressionParams{}, apigen.CheckExpressionJSONRequestBody{
		Position:   position,
		Expression: expression,
		Definition: definition,
	})
	if err != nil {
		return ExpressionCheck{}, err
	}

	held, err := answered(resp.HTTPResponse, resp.Body, resp.JSON200)
	if err != nil {
		return ExpressionCheck{}, err
	}

	check := ExpressionCheck{}
	if held.Type != nil {
		check.Type = *held.Type
	}

	for _, each := range held.Errors {
		found := ExpressionError{Message: each.Message}
		if each.Span != nil {
			found.Span = &Span{Start: int(each.Span.Start), End: int(each.Span.End)}
		}
		if each.Suggestion != nil {
			found.Suggestion = *each.Suggestion
		}
		check.Errors = append(check.Errors, found)
	}

	for _, each := range held.Warnings {
		check.Warnings = append(check.Warnings, ExpressionWarning{Position: each.Position, Message: each.Message})
	}

	return check, nil
}
