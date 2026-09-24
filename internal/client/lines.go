package client

import (
	"context"

	"github.com/google/uuid"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/client/apigen"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/spotlight"
)

type lines struct{ api *apigen.ClientWithResponses }

func lineOf(dto apigen.LineResourceResponseDto) spotlight.Line {
	return spotlight.Line{
		Meta:       meta(dto.Id, dto.Version, dto.Address, dto.Warnings),
		TeamID:     dto.TeamId,
		Key:        dto.Key,
		Definition: dto.Definition,
	}
}

func (l lines) Create(ctx context.Context, teamID string, in spotlight.Definition) (spotlight.Line, error) {
	resp, err := l.api.CreateLineWithResponse(ctx, teamID,
		&apigen.CreateLineParams{IdempotencyKey: uuid.NewString()},
		apigen.CreateLineJSONRequestBody(in))
	if err != nil {
		return spotlight.Line{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON201)

	return lineOf(dto), err
}

func (l lines) Read(ctx context.Context, id string) (spotlight.Line, error) {
	resp, err := l.api.ReadLineWithResponse(ctx, id, &apigen.ReadLineParams{})
	if err != nil {
		return spotlight.Line{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON200)

	return lineOf(dto), err
}

func (l lines) Update(ctx context.Context, id string, version int64, in spotlight.Definition) (spotlight.Line, error) {
	resp, err := l.api.UpdateLineWithResponse(ctx, id,
		&apigen.UpdateLineParams{IfMatch: entityTag(version)},
		apigen.UpdateLineJSONRequestBody(in))
	if err != nil {
		return spotlight.Line{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON200)

	return lineOf(dto), err
}

func (l lines) Delete(ctx context.Context, id string, version int64) error {
	resp, err := l.api.DeleteLineWithResponse(ctx, id, &apigen.DeleteLineParams{IfMatch: entityTag(version)})
	if err != nil {
		return err
	}

	return done(resp.HTTPResponse, resp.Body)
}

func (l lines) Import(ctx context.Context, workspaceID, address string) (spotlight.Line, error) {
	resp, err := l.api.ImportLineWithResponse(ctx, workspaceID, &apigen.ImportLineParams{Address: address})
	if err != nil {
		return spotlight.Line{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON200)

	return lineOf(dto), err
}
