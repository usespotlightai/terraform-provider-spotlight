package client

import (
	"context"

	"github.com/google/uuid"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/client/apigen"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/spotlight"
)

type integrations struct{ api *apigen.ClientWithResponses }

func integrationOf(dto apigen.IntegrationResourceResponseDto) spotlight.Integration {
	return spotlight.Integration{
		Meta:       meta(dto.Id, dto.Version, dto.Address, dto.Warnings),
		TeamID:     dto.TeamId,
		Source:     dto.Source,
		Definition: dto.Definition,
	}
}

func (i integrations) Create(ctx context.Context, teamID string, in spotlight.Definition) (spotlight.Integration, error) {
	resp, err := i.api.CreateIntegrationWithResponse(ctx, teamID,
		&apigen.CreateIntegrationParams{IdempotencyKey: uuid.NewString()},
		apigen.CreateIntegrationJSONRequestBody(in))
	if err != nil {
		return spotlight.Integration{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON201)

	return integrationOf(dto), err
}

func (i integrations) Read(ctx context.Context, id string) (spotlight.Integration, error) {
	resp, err := i.api.ReadIntegrationWithResponse(ctx, id, &apigen.ReadIntegrationParams{})
	if err != nil {
		return spotlight.Integration{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON200)

	return integrationOf(dto), err
}

func (i integrations) Update(ctx context.Context, id string, version int64, in spotlight.Definition) (spotlight.Integration, error) {
	resp, err := i.api.UpdateIntegrationWithResponse(ctx, id,
		&apigen.UpdateIntegrationParams{IfMatch: entityTag(version)},
		apigen.UpdateIntegrationJSONRequestBody(in))
	if err != nil {
		return spotlight.Integration{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON200)

	return integrationOf(dto), err
}

func (i integrations) Delete(ctx context.Context, id string, version int64) error {
	resp, err := i.api.DeleteIntegrationWithResponse(ctx, id, &apigen.DeleteIntegrationParams{IfMatch: entityTag(version)})
	if err != nil {
		return err
	}

	return done(resp.HTTPResponse, resp.Body)
}

func (i integrations) Import(ctx context.Context, workspaceID, address string) (spotlight.Integration, error) {
	resp, err := i.api.ImportIntegrationWithResponse(ctx, workspaceID, &apigen.ImportIntegrationParams{Address: address})
	if err != nil {
		return spotlight.Integration{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON200)

	return integrationOf(dto), err
}
