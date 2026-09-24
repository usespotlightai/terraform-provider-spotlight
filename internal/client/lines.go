package client

import (
	"context"

	"github.com/google/uuid"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/client/apigen"
)

func lineOf(dto apigen.LineResourceResponseDto) Line {
	return Line{
		Meta:       meta(dto.Id, dto.Version, dto.Address, dto.Warnings),
		TeamID:     dto.TeamId,
		Key:        dto.Key,
		Definition: dto.Definition,
	}
}

// CreateLine creates a line in a team.
func (c *Client) CreateLine(ctx context.Context, teamID string, in Definition) (Line, error) {
	resp, err := c.api.CreateLineWithResponse(ctx, teamID,
		&apigen.CreateLineParams{IdempotencyKey: uuid.NewString()},
		apigen.CreateLineJSONRequestBody(in))
	if err != nil {
		return Line{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON201)

	return lineOf(dto), err
}

// ReadLine reads the line with an id.
func (c *Client) ReadLine(ctx context.Context, id string) (Line, error) {
	resp, err := c.api.ReadLineWithResponse(ctx, id, &apigen.ReadLineParams{})
	if err != nil {
		return Line{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON200)

	return lineOf(dto), err
}

// UpdateLine writes the line over version, the one last read.
func (c *Client) UpdateLine(ctx context.Context, id string, version int64, in Definition) (Line, error) {
	resp, err := c.api.UpdateLineWithResponse(ctx, id,
		&apigen.UpdateLineParams{IfMatch: entityTag(version)},
		apigen.UpdateLineJSONRequestBody(in))
	if err != nil {
		return Line{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON200)

	return lineOf(dto), err
}

// DeleteLine deletes version of the line, the one last read.
func (c *Client) DeleteLine(ctx context.Context, id string, version int64) error {
	resp, err := c.api.DeleteLineWithResponse(ctx, id, &apigen.DeleteLineParams{IfMatch: entityTag(version)})
	if err != nil {
		return err
	}

	return done(resp.HTTPResponse, resp.Body)
}

// ImportLine finds the line at an address in a workspace.
func (c *Client) ImportLine(ctx context.Context, workspaceID, address string) (Line, error) {
	resp, err := c.api.ImportLineWithResponse(ctx, workspaceID, &apigen.ImportLineParams{Address: address})
	if err != nil {
		return Line{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON200)

	return lineOf(dto), err
}

// ListLines lists every line in a workspace, in address order.
func (c *Client) ListLines(ctx context.Context, workspaceID string) ([]Line, error) {
	resp, err := c.api.ListLinesWithResponse(ctx, workspaceID, &apigen.ListLinesParams{})
	if err != nil {
		return nil, err
	}

	listed, err := answered(resp.HTTPResponse, resp.Body, resp.JSON200)
	if err != nil {
		return nil, err
	}

	out := make([]Line, 0, len(listed.Items))
	for _, each := range listed.Items {
		out = append(out, lineOf(each))
	}

	return out, nil
}
