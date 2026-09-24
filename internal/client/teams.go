package client

import (
	"context"

	"github.com/google/uuid"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/client/apigen"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/spotlight"
)

type teams struct{ api *apigen.ClientWithResponses }

func teamOf(dto apigen.TeamResourceResponseDto) spotlight.Team {
	return spotlight.Team{
		Meta:        meta(dto.Id, dto.Version, dto.Address, dto.Warnings),
		WorkspaceID: dto.WorkspaceId,
		Key:         dto.Key,
		Name:        dto.Name,
	}
}

func (t teams) Create(ctx context.Context, workspaceID string, in spotlight.TeamInput) (spotlight.Team, error) {
	resp, err := t.api.CreateTeamWithResponse(ctx, workspaceID,
		&apigen.CreateTeamParams{IdempotencyKey: uuid.NewString()},
		apigen.CreateTeamJSONRequestBody{Key: in.Key, Name: in.Name})
	if err != nil {
		return spotlight.Team{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON201)

	return teamOf(dto), err
}

func (t teams) Read(ctx context.Context, id string) (spotlight.Team, error) {
	resp, err := t.api.ReadTeamWithResponse(ctx, id, &apigen.ReadTeamParams{})
	if err != nil {
		return spotlight.Team{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON200)

	return teamOf(dto), err
}

func (t teams) Update(ctx context.Context, id string, version int64, in spotlight.TeamInput) (spotlight.Team, error) {
	resp, err := t.api.UpdateTeamWithResponse(ctx, id,
		&apigen.UpdateTeamParams{IfMatch: entityTag(version)},
		apigen.UpdateTeamJSONRequestBody{Key: in.Key, Name: in.Name})
	if err != nil {
		return spotlight.Team{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON200)

	return teamOf(dto), err
}

func (t teams) Delete(ctx context.Context, id string, version int64) error {
	resp, err := t.api.DeleteTeamWithResponse(ctx, id, &apigen.DeleteTeamParams{IfMatch: entityTag(version)})
	if err != nil {
		return err
	}

	return done(resp.HTTPResponse, resp.Body)
}

func (t teams) Import(ctx context.Context, workspaceID, address string) (spotlight.Team, error) {
	resp, err := t.api.ImportTeamWithResponse(ctx, workspaceID, &apigen.ImportTeamParams{Address: address})
	if err != nil {
		return spotlight.Team{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON200)

	return teamOf(dto), err
}
