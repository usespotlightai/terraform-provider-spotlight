package client

import (
	"context"

	"github.com/google/uuid"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/client/apigen"
)

func teamOf(dto apigen.TeamResourceResponseDto) Team {
	return Team{
		Meta:        meta(dto.Id, dto.Version, dto.Address, dto.Warnings),
		WorkspaceID: dto.WorkspaceId,
		Key:         dto.Key,
		Name:        dto.Name,
	}
}

// CreateTeam creates a team in a workspace.
func (c *Client) CreateTeam(ctx context.Context, workspaceID string, in TeamInput) (Team, error) {
	resp, err := c.api.CreateTeamWithResponse(ctx, workspaceID,
		&apigen.CreateTeamParams{IdempotencyKey: uuid.NewString()},
		apigen.CreateTeamJSONRequestBody{Key: in.Key, Name: in.Name})
	if err != nil {
		return Team{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON201)

	return teamOf(dto), err
}

// ReadTeam reads the team with an id.
func (c *Client) ReadTeam(ctx context.Context, id string) (Team, error) {
	resp, err := c.api.ReadTeamWithResponse(ctx, id, &apigen.ReadTeamParams{})
	if err != nil {
		return Team{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON200)

	return teamOf(dto), err
}

// UpdateTeam writes the team over version, the one last read.
func (c *Client) UpdateTeam(ctx context.Context, id string, version int64, in TeamInput) (Team, error) {
	resp, err := c.api.UpdateTeamWithResponse(ctx, id,
		&apigen.UpdateTeamParams{IfMatch: entityTag(version)},
		apigen.UpdateTeamJSONRequestBody{Key: in.Key, Name: in.Name})
	if err != nil {
		return Team{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON200)

	return teamOf(dto), err
}

// DeleteTeam deletes version of the team, the one last read.
func (c *Client) DeleteTeam(ctx context.Context, id string, version int64) error {
	resp, err := c.api.DeleteTeamWithResponse(ctx, id, &apigen.DeleteTeamParams{IfMatch: entityTag(version)})
	if err != nil {
		return err
	}

	return done(resp.HTTPResponse, resp.Body)
}

// ImportTeam finds the team at an address in a workspace.
func (c *Client) ImportTeam(ctx context.Context, workspaceID, address string) (Team, error) {
	resp, err := c.api.ImportTeamWithResponse(ctx, workspaceID, &apigen.ImportTeamParams{Address: address})
	if err != nil {
		return Team{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON200)

	return teamOf(dto), err
}

// ListTeams lists every team in a workspace, in address order.
func (c *Client) ListTeams(ctx context.Context, workspaceID string) ([]Team, error) {
	resp, err := c.api.ListTeamsWithResponse(ctx, workspaceID, &apigen.ListTeamsParams{})
	if err != nil {
		return nil, err
	}

	listed, err := answered(resp.HTTPResponse, resp.Body, resp.JSON200)
	if err != nil {
		return nil, err
	}

	out := make([]Team, 0, len(listed.Items))
	for _, each := range listed.Items {
		out = append(out, teamOf(each))
	}

	return out, nil
}
