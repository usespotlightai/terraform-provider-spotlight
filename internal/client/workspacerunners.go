package client

import (
	"context"

	"github.com/google/uuid"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/client/apigen"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/spotlight"
)

type workspaceRunners struct{ api *apigen.ClientWithResponses }

func workspaceRunnersOf(dto apigen.WorkspaceRunnersResourceResponseDto) spotlight.WorkspaceRunners {
	return spotlight.WorkspaceRunners{
		Meta:        meta(dto.Id, dto.Version, dto.Address, dto.Warnings),
		WorkspaceID: dto.WorkspaceId,
		WorkspaceRunnersInput: spotlight.WorkspaceRunnersInput{
			Runners: orNone(dto.Runners),
			Trusts:  orNone(dto.Trusts),
		},
		Drift: orNone(dto.Drift),
	}
}

func (w workspaceRunners) Create(ctx context.Context, workspaceID string, in spotlight.WorkspaceRunnersInput) (spotlight.WorkspaceRunners, error) {
	body, err := jsonBody(in)
	if err != nil {
		return spotlight.WorkspaceRunners{}, err
	}

	resp, err := w.api.CreateWorkspaceRunnersWithBodyWithResponse(ctx, workspaceID,
		&apigen.CreateWorkspaceRunnersParams{IdempotencyKey: uuid.NewString()}, jsonContent, body)
	if err != nil {
		return spotlight.WorkspaceRunners{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON201)

	return workspaceRunnersOf(dto), err
}

func (w workspaceRunners) Read(ctx context.Context, id string) (spotlight.WorkspaceRunners, error) {
	resp, err := w.api.ReadWorkspaceRunnersWithResponse(ctx, id, &apigen.ReadWorkspaceRunnersParams{})
	if err != nil {
		return spotlight.WorkspaceRunners{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON200)

	return workspaceRunnersOf(dto), err
}

func (w workspaceRunners) Update(ctx context.Context, id string, version int64, in spotlight.WorkspaceRunnersInput) (spotlight.WorkspaceRunners, error) {
	body, err := jsonBody(in)
	if err != nil {
		return spotlight.WorkspaceRunners{}, err
	}

	resp, err := w.api.UpdateWorkspaceRunnersWithBodyWithResponse(ctx, id,
		&apigen.UpdateWorkspaceRunnersParams{IfMatch: entityTag(version)}, jsonContent, body)
	if err != nil {
		return spotlight.WorkspaceRunners{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON200)

	return workspaceRunnersOf(dto), err
}

func (w workspaceRunners) Delete(ctx context.Context, id string, version int64) error {
	resp, err := w.api.DeleteWorkspaceRunnersWithResponse(ctx, id, &apigen.DeleteWorkspaceRunnersParams{IfMatch: entityTag(version)})
	if err != nil {
		return err
	}

	return done(resp.HTTPResponse, resp.Body)
}

func (w workspaceRunners) Import(ctx context.Context, workspaceID, address string) (spotlight.WorkspaceRunners, error) {
	resp, err := w.api.ImportWorkspaceRunnersWithResponse(ctx, workspaceID, &apigen.ImportWorkspaceRunnersParams{Address: address})
	if err != nil {
		return spotlight.WorkspaceRunners{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON200)

	return workspaceRunnersOf(dto), err
}
