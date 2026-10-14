package client

import (
	"context"

	"github.com/google/uuid"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/client/apigen"
)

func workspaceTokenTrustOf(dto apigen.WorkspaceTokenTrustResourceResponseDto) WorkspaceTokenTrust {
	scopes := make([]string, 0, len(dto.Scopes))
	for _, each := range dto.Scopes {
		scopes = append(scopes, string(each))
	}

	claims := dto.Claims
	if claims == nil {
		claims = map[string]string{}
	}

	return WorkspaceTokenTrust{
		Meta:        meta(dto.Id, dto.Version, dto.Address, dto.Warnings),
		WorkspaceID: dto.WorkspaceId,
		DefinedBy:   dto.DefinedBy,
		WorkspaceTokenTrustInput: WorkspaceTokenTrustInput{
			Key:             dto.Key,
			Issuer:          dto.Issuer,
			Audience:        dto.Audience,
			Subject:         dto.Subject,
			Claims:          claims,
			Scopes:          scopes,
			LifetimeMinutes: int64(dto.LifetimeMinutes),
		},
	}
}

// CreateWorkspaceTokenTrust creates a workspace token trust in a workspace.
func (c *Client) CreateWorkspaceTokenTrust(ctx context.Context, workspaceID string, in WorkspaceTokenTrustInput) (WorkspaceTokenTrust, error) {
	body, err := jsonBody(in)
	if err != nil {
		return WorkspaceTokenTrust{}, err
	}

	resp, err := c.api.CreateWorkspaceTokenTrustWithBodyWithResponse(ctx, workspaceID,
		&apigen.CreateWorkspaceTokenTrustParams{IdempotencyKey: uuid.NewString()}, jsonContent, body)
	if err != nil {
		return WorkspaceTokenTrust{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON201)

	return workspaceTokenTrustOf(dto), err
}

// ReadWorkspaceTokenTrust reads the workspace token trust with an id.
func (c *Client) ReadWorkspaceTokenTrust(ctx context.Context, id string) (WorkspaceTokenTrust, error) {
	resp, err := c.api.ReadWorkspaceTokenTrustWithResponse(ctx, id, &apigen.ReadWorkspaceTokenTrustParams{})
	if err != nil {
		return WorkspaceTokenTrust{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON200)

	return workspaceTokenTrustOf(dto), err
}

// UpdateWorkspaceTokenTrust writes the workspace token trust over version,
// the version from the last read.
func (c *Client) UpdateWorkspaceTokenTrust(ctx context.Context, id string, version int64, in WorkspaceTokenTrustInput) (WorkspaceTokenTrust, error) {
	body, err := jsonBody(in)
	if err != nil {
		return WorkspaceTokenTrust{}, err
	}

	resp, err := c.api.UpdateWorkspaceTokenTrustWithBodyWithResponse(ctx, id,
		&apigen.UpdateWorkspaceTokenTrustParams{IfMatch: entityTag(version)}, jsonContent, body)
	if err != nil {
		return WorkspaceTokenTrust{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON200)

	return workspaceTokenTrustOf(dto), err
}

// DeleteWorkspaceTokenTrust deletes version of the workspace token trust,
// the version from the last read.
func (c *Client) DeleteWorkspaceTokenTrust(ctx context.Context, id string, version int64) error {
	resp, err := c.api.DeleteWorkspaceTokenTrustWithResponse(ctx, id, &apigen.DeleteWorkspaceTokenTrustParams{IfMatch: entityTag(version)})
	if err != nil {
		return err
	}

	return done(resp.HTTPResponse, resp.Body)
}

// ImportWorkspaceTokenTrust finds the workspace token trust at an address in
// a workspace.
func (c *Client) ImportWorkspaceTokenTrust(ctx context.Context, workspaceID, address string) (WorkspaceTokenTrust, error) {
	resp, err := c.api.ImportWorkspaceTokenTrustWithResponse(ctx, workspaceID, &apigen.ImportWorkspaceTokenTrustParams{Address: address})
	if err != nil {
		return WorkspaceTokenTrust{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON200)

	return workspaceTokenTrustOf(dto), err
}

// ListWorkspaceTokenTrusts lists every workspace token trust in a workspace,
// in address order.
func (c *Client) ListWorkspaceTokenTrusts(ctx context.Context, workspaceID string) ([]WorkspaceTokenTrust, error) {
	resp, err := c.api.ListWorkspaceTokenTrustsWithResponse(ctx, workspaceID, &apigen.ListWorkspaceTokenTrustsParams{})
	if err != nil {
		return nil, err
	}

	listed, err := answered(resp.HTTPResponse, resp.Body, resp.JSON200)
	if err != nil {
		return nil, err
	}

	out := make([]WorkspaceTokenTrust, 0, len(listed.Items))
	for _, each := range listed.Items {
		out = append(out, workspaceTokenTrustOf(each))
	}

	return out, nil
}
