package client

import (
	"context"

	"github.com/google/uuid"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/client/apigen"
)

func oidcTrustOf(dto apigen.OidcTrustResourceResponseDto) OidcTrust {
	scopes := make([]string, 0, len(dto.Scopes))
	for _, each := range dto.Scopes {
		scopes = append(scopes, string(each))
	}

	claims := dto.Claims
	if claims == nil {
		claims = map[string]string{}
	}

	return OidcTrust{
		Meta:        meta(dto.Id, dto.Version, dto.Address, dto.Warnings),
		WorkspaceID: dto.WorkspaceId,
		CreatedBy:   dto.CreatedBy,
		OidcTrustInput: OidcTrustInput{
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

// CreateOidcTrust creates an OIDC trust in a workspace.
func (c *Client) CreateOidcTrust(ctx context.Context, workspaceID string, in OidcTrustInput) (OidcTrust, error) {
	body, err := jsonBody(in)
	if err != nil {
		return OidcTrust{}, err
	}

	resp, err := c.api.CreateOidcTrustWithBodyWithResponse(ctx, workspaceID,
		&apigen.CreateOidcTrustParams{IdempotencyKey: uuid.NewString()}, jsonContent, body)
	if err != nil {
		return OidcTrust{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON201)

	return oidcTrustOf(dto), err
}

// ReadOidcTrust reads the OIDC trust with an id.
func (c *Client) ReadOidcTrust(ctx context.Context, id string) (OidcTrust, error) {
	resp, err := c.api.ReadOidcTrustWithResponse(ctx, id, &apigen.ReadOidcTrustParams{})
	if err != nil {
		return OidcTrust{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON200)

	return oidcTrustOf(dto), err
}

// UpdateOidcTrust writes the OIDC trust over version,
// the version from the last read.
func (c *Client) UpdateOidcTrust(ctx context.Context, id string, version int64, in OidcTrustInput) (OidcTrust, error) {
	body, err := jsonBody(in)
	if err != nil {
		return OidcTrust{}, err
	}

	resp, err := c.api.UpdateOidcTrustWithBodyWithResponse(ctx, id,
		&apigen.UpdateOidcTrustParams{IfMatch: entityTag(version)}, jsonContent, body)
	if err != nil {
		return OidcTrust{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON200)

	return oidcTrustOf(dto), err
}

// DeleteOidcTrust deletes version of the OIDC trust,
// the version from the last read.
func (c *Client) DeleteOidcTrust(ctx context.Context, id string, version int64) error {
	resp, err := c.api.DeleteOidcTrustWithResponse(ctx, id, &apigen.DeleteOidcTrustParams{IfMatch: entityTag(version)})
	if err != nil {
		return err
	}

	return done(resp.HTTPResponse, resp.Body)
}

// ImportOidcTrust finds the OIDC trust at an address in
// a workspace.
func (c *Client) ImportOidcTrust(ctx context.Context, workspaceID, address string) (OidcTrust, error) {
	resp, err := c.api.ImportOidcTrustWithResponse(ctx, workspaceID, &apigen.ImportOidcTrustParams{Address: address})
	if err != nil {
		return OidcTrust{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON200)

	return oidcTrustOf(dto), err
}

// ListOidcTrusts lists every OIDC trust in a workspace,
// in address order.
func (c *Client) ListOidcTrusts(ctx context.Context, workspaceID string) ([]OidcTrust, error) {
	resp, err := c.api.ListOidcTrustsWithResponse(ctx, workspaceID, &apigen.ListOidcTrustsParams{})
	if err != nil {
		return nil, err
	}

	listed, err := answered(resp.HTTPResponse, resp.Body, resp.JSON200)
	if err != nil {
		return nil, err
	}

	out := make([]OidcTrust, 0, len(listed.Items))
	for _, each := range listed.Items {
		out = append(out, oidcTrustOf(each))
	}

	return out, nil
}
