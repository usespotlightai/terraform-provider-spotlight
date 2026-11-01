package client

import (
	"context"

	"github.com/google/uuid"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/client/apigen"
)

func workspaceOf(dto apigen.WorkspaceResourceResponseDto) Workspace {
	return Workspace{
		Meta:          meta(dto.Id, dto.Version, dto.Address, dto.Warnings),
		Name:          dto.Name,
		Slug:          dto.Slug,
		OutboundHosts: orNone(dto.OutboundHosts),
	}
}

// CreateWorkspace creates a workspace.
func (c *Client) CreateWorkspace(ctx context.Context, in WorkspaceInput) (Workspace, error) {
	hosts := orNone(in.OutboundHosts)
	resp, err := c.api.CreateWorkspaceWithResponse(ctx,
		&apigen.CreateWorkspaceParams{IdempotencyKey: uuid.NewString()},
		apigen.CreateWorkspaceJSONRequestBody{Name: in.Name, Slug: in.Slug, OutboundHosts: &hosts})
	if err != nil {
		return Workspace{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON201)

	return workspaceOf(dto), err
}

// ReadWorkspace reads the workspace with an id.
func (c *Client) ReadWorkspace(ctx context.Context, id string) (Workspace, error) {
	resp, err := c.api.ReadWorkspaceWithResponse(ctx, id, &apigen.ReadWorkspaceParams{})
	if err != nil {
		return Workspace{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON200)

	return workspaceOf(dto), err
}

// UpdateWorkspace writes the workspace over version, the version from the last
// read.
func (c *Client) UpdateWorkspace(ctx context.Context, id string, version int64, in WorkspaceInput) (Workspace, error) {
	hosts := orNone(in.OutboundHosts)
	resp, err := c.api.UpdateWorkspaceWithResponse(ctx, id,
		&apigen.UpdateWorkspaceParams{IfMatch: entityTag(version)},
		apigen.UpdateWorkspaceJSONRequestBody{Name: in.Name, Slug: in.Slug, OutboundHosts: &hosts})
	if err != nil {
		return Workspace{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON200)

	return workspaceOf(dto), err
}

// DeleteWorkspace deletes version of the workspace, the version from the last
// read.
func (c *Client) DeleteWorkspace(ctx context.Context, id string, version int64) error {
	resp, err := c.api.DeleteWorkspaceWithResponse(ctx, id, &apigen.DeleteWorkspaceParams{IfMatch: entityTag(version)})
	if err != nil {
		return err
	}

	return done(resp.HTTPResponse, resp.Body)
}

// ImportWorkspace finds the workspace with a slug.
func (c *Client) ImportWorkspace(ctx context.Context, slug string) (Workspace, error) {
	resp, err := c.api.ImportWorkspaceWithResponse(ctx, &apigen.ImportWorkspaceParams{Address: slug})
	if err != nil {
		return Workspace{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON200)

	return workspaceOf(dto), err
}

// ListWorkspaces lists every workspace that the caller can read.
func (c *Client) ListWorkspaces(ctx context.Context) ([]Workspace, error) {
	resp, err := c.api.ListWorkspacesWithResponse(ctx, &apigen.ListWorkspacesParams{})
	if err != nil {
		return nil, err
	}

	listed, err := answered(resp.HTTPResponse, resp.Body, resp.JSON200)
	if err != nil {
		return nil, err
	}

	out := make([]Workspace, 0, len(listed.Items))
	for _, each := range listed.Items {
		out = append(out, workspaceOf(each))
	}

	return out, nil
}
