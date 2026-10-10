package client

import (
	"context"

	"github.com/google/uuid"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/client/apigen"
)

func integrationOf(dto apigen.IntegrationResourceResponseDto) Integration {
	return Integration{
		Meta:       meta(dto.Id, dto.Version, dto.Address, dto.Warnings),
		TeamID:     dto.TeamId,
		Source:     dto.Source,
		Definition: dto.Definition,
		Tools:      dto.Tools,
		Described:  describedOf(dto.Described),
	}
}

func describedOf(dto *apigen.DescribedSourceResponseDto) *Described {
	if dto == nil {
		return nil
	}

	return &Described{
		Kind:    string(dto.Kind),
		Hash:    dto.Hash,
		ReadAt:  dto.ReadAt,
		Problem: dto.Problem,
		Tools:   int64(dto.Tools),
	}
}

// CreateIntegration creates a integration in a team.
func (c *Client) CreateIntegration(ctx context.Context, teamID string, in Definition) (Integration, error) {
	resp, err := c.api.CreateIntegrationWithResponse(ctx, teamID,
		&apigen.CreateIntegrationParams{IdempotencyKey: uuid.NewString()},
		apigen.CreateIntegrationJSONRequestBody(in))
	if err != nil {
		return Integration{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON201)

	return integrationOf(dto), err
}

// ReadIntegration reads the integration with an id.
func (c *Client) ReadIntegration(ctx context.Context, id string) (Integration, error) {
	resp, err := c.api.ReadIntegrationWithResponse(ctx, id, &apigen.ReadIntegrationParams{})
	if err != nil {
		return Integration{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON200)

	return integrationOf(dto), err
}

// UpdateIntegration writes the integration over version, the one last read.
func (c *Client) UpdateIntegration(ctx context.Context, id string, version int64, in Definition) (Integration, error) {
	resp, err := c.api.UpdateIntegrationWithResponse(ctx, id,
		&apigen.UpdateIntegrationParams{IfMatch: entityTag(version)},
		apigen.UpdateIntegrationJSONRequestBody(in))
	if err != nil {
		return Integration{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON200)

	return integrationOf(dto), err
}

// DeleteIntegration deletes version of the integration, the one last read.
func (c *Client) DeleteIntegration(ctx context.Context, id string, version int64) error {
	resp, err := c.api.DeleteIntegrationWithResponse(ctx, id, &apigen.DeleteIntegrationParams{IfMatch: entityTag(version)})
	if err != nil {
		return err
	}

	return done(resp.HTTPResponse, resp.Body)
}

// ImportIntegration finds the integration at an address in a workspace.
func (c *Client) ImportIntegration(ctx context.Context, workspaceID, address string) (Integration, error) {
	resp, err := c.api.ImportIntegrationWithResponse(ctx, workspaceID, &apigen.ImportIntegrationParams{Address: address})
	if err != nil {
		return Integration{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON200)

	return integrationOf(dto), err
}

// ListIntegrations lists every integration in a workspace, in address order.
func (c *Client) ListIntegrations(ctx context.Context, workspaceID string) ([]Integration, error) {
	resp, err := c.api.ListIntegrationsWithResponse(ctx, workspaceID, &apigen.ListIntegrationsParams{})
	if err != nil {
		return nil, err
	}

	listed, err := answered(resp.HTTPResponse, resp.Body, resp.JSON200)
	if err != nil {
		return nil, err
	}

	out := make([]Integration, 0, len(listed.Items))
	for _, each := range listed.Items {
		out = append(out, integrationOf(each))
	}

	return out, nil
}
