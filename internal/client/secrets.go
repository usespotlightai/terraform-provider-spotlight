package client

import (
	"context"

	"github.com/google/uuid"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/client/apigen"
)

func secretOf(dto apigen.SecretResourceResponseDto) Secret {
	bindings := make([]SecretBinding, 0, len(dto.Bindings))
	for _, each := range dto.Bindings {
		bindings = append(bindings, SecretBinding{Host: each.Host, Header: each.Header, Prefix: each.Prefix})
	}

	secret := Secret{
		Meta:        meta(dto.Id, dto.Version, dto.Address, dto.Warnings),
		WorkspaceID: dto.WorkspaceId,
		Name:        dto.Name,
		Source:      SecretSource{Kind: string(dto.Source.Kind)},
		Bindings:    bindings,
		RotatedAt:   dto.RotatedAt,
	}

	if dto.Source.SecretId != nil {
		secret.Source.SecretID = *dto.Source.SecretId
	}

	if dto.Source.Field != nil {
		secret.Source.Field = *dto.Source.Field
	}

	if dto.ValueWoVersion != nil {
		version := int64(*dto.ValueWoVersion)
		secret.ValueVersion = &version
	}

	return secret
}

// CreateSecret creates a secret in a workspace.
func (c *Client) CreateSecret(ctx context.Context, workspaceID string, in SecretInput) (Secret, error) {
	body, err := jsonBody(in)
	if err != nil {
		return Secret{}, err
	}

	resp, err := c.api.CreateSecretWithBodyWithResponse(ctx, workspaceID,
		&apigen.CreateSecretParams{IdempotencyKey: uuid.NewString()}, jsonContent, body)
	if err != nil {
		return Secret{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON201)

	return secretOf(dto), err
}

// ReadSecret reads the secret with an id.
func (c *Client) ReadSecret(ctx context.Context, id string) (Secret, error) {
	resp, err := c.api.ReadSecretWithResponse(ctx, id, &apigen.ReadSecretParams{})
	if err != nil {
		return Secret{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON200)

	return secretOf(dto), err
}

// UpdateSecret writes the secret over version, the one last read.
func (c *Client) UpdateSecret(ctx context.Context, id string, version int64, in SecretInput) (Secret, error) {
	body, err := jsonBody(in)
	if err != nil {
		return Secret{}, err
	}

	resp, err := c.api.UpdateSecretWithBodyWithResponse(ctx, id,
		&apigen.UpdateSecretParams{IfMatch: entityTag(version)}, jsonContent, body)
	if err != nil {
		return Secret{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON200)

	return secretOf(dto), err
}

// DeleteSecret deletes version of the secret, the one last read.
func (c *Client) DeleteSecret(ctx context.Context, id string, version int64) error {
	resp, err := c.api.DeleteSecretWithResponse(ctx, id, &apigen.DeleteSecretParams{IfMatch: entityTag(version)})
	if err != nil {
		return err
	}

	return done(resp.HTTPResponse, resp.Body)
}

// ImportSecret finds the secret at an address in a workspace.
func (c *Client) ImportSecret(ctx context.Context, workspaceID, address string) (Secret, error) {
	resp, err := c.api.ImportSecretWithResponse(ctx, workspaceID, &apigen.ImportSecretParams{Address: address})
	if err != nil {
		return Secret{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON200)

	return secretOf(dto), err
}

// ListSecrets lists every secret in a workspace, in address order.
func (c *Client) ListSecrets(ctx context.Context, workspaceID string) ([]Secret, error) {
	resp, err := c.api.ListSecretsWithResponse(ctx, workspaceID, &apigen.ListSecretsParams{})
	if err != nil {
		return nil, err
	}

	listed, err := answered(resp.HTTPResponse, resp.Body, resp.JSON200)
	if err != nil {
		return nil, err
	}

	out := make([]Secret, 0, len(listed.Items))
	for _, each := range listed.Items {
		out = append(out, secretOf(each))
	}

	return out, nil
}
