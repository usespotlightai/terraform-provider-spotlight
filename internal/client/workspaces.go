package client

import (
	"context"

	"github.com/google/uuid"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/client/apigen"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/spotlight"
)

type workspaces struct{ api *apigen.ClientWithResponses }

func workspaceOf(dto apigen.WorkspaceResourceResponseDto) spotlight.Workspace {
	return spotlight.Workspace{
		Meta: meta(dto.Id, dto.Version, dto.Address, dto.Warnings),
		Name: dto.Name,
		Slug: dto.Slug,
	}
}

func (w workspaces) Create(ctx context.Context, _ string, in spotlight.WorkspaceInput) (spotlight.Workspace, error) {
	resp, err := w.api.CreateWorkspaceWithResponse(ctx,
		&apigen.CreateWorkspaceParams{IdempotencyKey: uuid.NewString()},
		apigen.CreateWorkspaceJSONRequestBody{Name: in.Name, Slug: in.Slug})
	if err != nil {
		return spotlight.Workspace{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON201)

	return workspaceOf(dto), err
}

func (w workspaces) Read(ctx context.Context, id string) (spotlight.Workspace, error) {
	resp, err := w.api.ReadWorkspaceWithResponse(ctx, id, &apigen.ReadWorkspaceParams{})
	if err != nil {
		return spotlight.Workspace{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON200)

	return workspaceOf(dto), err
}

func (w workspaces) Update(ctx context.Context, id string, version int64, in spotlight.WorkspaceInput) (spotlight.Workspace, error) {
	resp, err := w.api.UpdateWorkspaceWithResponse(ctx, id,
		&apigen.UpdateWorkspaceParams{IfMatch: entityTag(version)},
		apigen.UpdateWorkspaceJSONRequestBody{Name: in.Name, Slug: in.Slug})
	if err != nil {
		return spotlight.Workspace{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON200)

	return workspaceOf(dto), err
}

func (w workspaces) Delete(ctx context.Context, id string, version int64) error {
	resp, err := w.api.DeleteWorkspaceWithResponse(ctx, id, &apigen.DeleteWorkspaceParams{IfMatch: entityTag(version)})
	if err != nil {
		return err
	}

	return done(resp.HTTPResponse, resp.Body)
}

func (w workspaces) Import(ctx context.Context, _ string, address string) (spotlight.Workspace, error) {
	resp, err := w.api.ImportWorkspaceWithResponse(ctx, &apigen.ImportWorkspaceParams{Address: address})
	if err != nil {
		return spotlight.Workspace{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON200)

	return workspaceOf(dto), err
}
