package client

import (
	"context"

	"github.com/google/uuid"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/client/apigen"
)

func workspaceRunnersOf(dto apigen.WorkspaceRunnersResourceResponseDto) WorkspaceRunners {
	return WorkspaceRunners{
		Meta:        meta(dto.Id, dto.Version, dto.Address, dto.Warnings),
		WorkspaceID: dto.WorkspaceId,
		WorkspaceRunnersInput: WorkspaceRunnersInput{
			Trusts: orNone(dto.Trusts),
			Pools:  orNone(dto.Pools),
		},
	}
}

// CreateWorkspaceRunners declares a workspace's runners.
func (c *Client) CreateWorkspaceRunners(ctx context.Context, workspaceID string, in WorkspaceRunnersInput) (WorkspaceRunners, error) {
	body, err := jsonBody(in)
	if err != nil {
		return WorkspaceRunners{}, err
	}

	resp, err := c.api.CreateWorkspaceRunnersWithBodyWithResponse(ctx, workspaceID,
		&apigen.CreateWorkspaceRunnersParams{IdempotencyKey: uuid.NewString()}, jsonContent, body)
	if err != nil {
		return WorkspaceRunners{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON201)

	return workspaceRunnersOf(dto), err
}

// ReadWorkspaceRunners reads the workspace's runner declaration with an id.
func (c *Client) ReadWorkspaceRunners(ctx context.Context, id string) (WorkspaceRunners, error) {
	resp, err := c.api.ReadWorkspaceRunnersWithResponse(ctx, id, &apigen.ReadWorkspaceRunnersParams{})
	if err != nil {
		return WorkspaceRunners{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON200)

	return workspaceRunnersOf(dto), err
}

// UpdateWorkspaceRunners writes the runner declaration over version, the one last read.
func (c *Client) UpdateWorkspaceRunners(ctx context.Context, id string, version int64, in WorkspaceRunnersInput) (WorkspaceRunners, error) {
	body, err := jsonBody(in)
	if err != nil {
		return WorkspaceRunners{}, err
	}

	resp, err := c.api.UpdateWorkspaceRunnersWithBodyWithResponse(ctx, id,
		&apigen.UpdateWorkspaceRunnersParams{IfMatch: entityTag(version)}, jsonContent, body)
	if err != nil {
		return WorkspaceRunners{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON200)

	return workspaceRunnersOf(dto), err
}

// DeleteWorkspaceRunners deletes version of the runner declaration, the one last read.
func (c *Client) DeleteWorkspaceRunners(ctx context.Context, id string, version int64) error {
	resp, err := c.api.DeleteWorkspaceRunnersWithResponse(ctx, id, &apigen.DeleteWorkspaceRunnersParams{IfMatch: entityTag(version)})
	if err != nil {
		return err
	}

	return done(resp.HTTPResponse, resp.Body)
}

// ImportWorkspaceRunners finds a workspace's runner declaration by the workspace's slug.
func (c *Client) ImportWorkspaceRunners(ctx context.Context, workspaceID, address string) (WorkspaceRunners, error) {
	resp, err := c.api.ImportWorkspaceRunnersWithResponse(ctx, workspaceID, &apigen.ImportWorkspaceRunnersParams{Address: address})
	if err != nil {
		return WorkspaceRunners{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON200)

	return workspaceRunnersOf(dto), err
}

// ListWorkspaceRunners lists a workspace's runner declaration, one or none.
func (c *Client) ListWorkspaceRunners(ctx context.Context, workspaceID string) ([]WorkspaceRunners, error) {
	resp, err := c.api.ListWorkspaceRunnersWithResponse(ctx, workspaceID, &apigen.ListWorkspaceRunnersParams{})
	if err != nil {
		return nil, err
	}

	listed, err := answered(resp.HTTPResponse, resp.Body, resp.JSON200)
	if err != nil {
		return nil, err
	}

	out := make([]WorkspaceRunners, 0, len(listed.Items))
	for _, each := range listed.Items {
		out = append(out, workspaceRunnersOf(each))
	}

	return out, nil
}
