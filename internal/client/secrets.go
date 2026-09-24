package client

import (
	"context"

	"github.com/google/uuid"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/client/apigen"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/spotlight"
)

type secrets struct{ api *apigen.ClientWithResponses }

func secretOf(dto apigen.SecretResourceResponseDto) spotlight.Secret {
	bindings := make([]spotlight.SecretBinding, 0, len(dto.Bindings))
	for _, each := range dto.Bindings {
		bindings = append(bindings, spotlight.SecretBinding{Host: each.Host, Header: each.Header, Prefix: each.Prefix})
	}

	secret := spotlight.Secret{
		Meta:        meta(dto.Id, dto.Version, dto.Address, dto.Warnings),
		WorkspaceID: dto.WorkspaceId,
		Name:        dto.Name,
		Source:      spotlight.SecretSource{Kind: string(dto.Source.Kind)},
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

func (s secrets) Create(ctx context.Context, workspaceID string, in spotlight.SecretInput) (spotlight.Secret, error) {
	body, err := jsonBody(in)
	if err != nil {
		return spotlight.Secret{}, err
	}

	resp, err := s.api.CreateSecretWithBodyWithResponse(ctx, workspaceID,
		&apigen.CreateSecretParams{IdempotencyKey: uuid.NewString()}, jsonContent, body)
	if err != nil {
		return spotlight.Secret{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON201)

	return secretOf(dto), err
}

func (s secrets) Read(ctx context.Context, id string) (spotlight.Secret, error) {
	resp, err := s.api.ReadSecretWithResponse(ctx, id, &apigen.ReadSecretParams{})
	if err != nil {
		return spotlight.Secret{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON200)

	return secretOf(dto), err
}

func (s secrets) Update(ctx context.Context, id string, version int64, in spotlight.SecretInput) (spotlight.Secret, error) {
	body, err := jsonBody(in)
	if err != nil {
		return spotlight.Secret{}, err
	}

	resp, err := s.api.UpdateSecretWithBodyWithResponse(ctx, id,
		&apigen.UpdateSecretParams{IfMatch: entityTag(version)}, jsonContent, body)
	if err != nil {
		return spotlight.Secret{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON200)

	return secretOf(dto), err
}

func (s secrets) Delete(ctx context.Context, id string, version int64) error {
	resp, err := s.api.DeleteSecretWithResponse(ctx, id, &apigen.DeleteSecretParams{IfMatch: entityTag(version)})
	if err != nil {
		return err
	}

	return done(resp.HTTPResponse, resp.Body)
}

func (s secrets) Import(ctx context.Context, workspaceID, address string) (spotlight.Secret, error) {
	resp, err := s.api.ImportSecretWithResponse(ctx, workspaceID, &apigen.ImportSecretParams{Address: address})
	if err != nil {
		return spotlight.Secret{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON200)

	return secretOf(dto), err
}
