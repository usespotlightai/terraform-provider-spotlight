package client

import (
	"context"

	"github.com/google/uuid"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/client/apigen"
)

func inferenceEndpointOf(dto apigen.InferenceEndpointResourceResponseDto) InferenceEndpoint {
	endpoint := InferenceEndpoint{
		Meta:        meta(dto.Id, dto.Version, dto.Address, dto.Warnings),
		WorkspaceID: dto.WorkspaceId,
		APIKeySet:   dto.ApiKeySet,
		InferenceEndpointInput: InferenceEndpointInput{
			BaseURL:                 dto.BaseUrl,
			Model:                   dto.Model,
			SupportsReasoningEffort: dto.SupportsReasoningEffort,
		},
	}

	if dto.ApiKeyVersion != nil {
		version := int64(*dto.ApiKeyVersion)
		endpoint.APIKeyVersion = &version
	}

	return endpoint
}

func (c *Client) CreateInferenceEndpoint(ctx context.Context, workspaceID string, in InferenceEndpointInput) (InferenceEndpoint, error) {
	body, err := jsonBody(in)
	if err != nil {
		return InferenceEndpoint{}, err
	}

	resp, err := c.api.CreateInferenceEndpointWithBodyWithResponse(ctx, workspaceID,
		&apigen.CreateInferenceEndpointParams{IdempotencyKey: uuid.NewString()}, jsonContent, body)
	if err != nil {
		return InferenceEndpoint{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON201)

	return inferenceEndpointOf(dto), err
}

func (c *Client) ReadInferenceEndpoint(ctx context.Context, id string) (InferenceEndpoint, error) {
	resp, err := c.api.ReadInferenceEndpointWithResponse(ctx, id, &apigen.ReadInferenceEndpointParams{})
	if err != nil {
		return InferenceEndpoint{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON200)

	return inferenceEndpointOf(dto), err
}

func (c *Client) UpdateInferenceEndpoint(ctx context.Context, id string, version int64, in InferenceEndpointInput) (InferenceEndpoint, error) {
	body, err := jsonBody(in)
	if err != nil {
		return InferenceEndpoint{}, err
	}

	resp, err := c.api.UpdateInferenceEndpointWithBodyWithResponse(ctx, id,
		&apigen.UpdateInferenceEndpointParams{IfMatch: entityTag(version)}, jsonContent, body)
	if err != nil {
		return InferenceEndpoint{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON200)

	return inferenceEndpointOf(dto), err
}

func (c *Client) DeleteInferenceEndpoint(ctx context.Context, id string, version int64) error {
	resp, err := c.api.DeleteInferenceEndpointWithResponse(ctx, id, &apigen.DeleteInferenceEndpointParams{IfMatch: entityTag(version)})
	if err != nil {
		return err
	}

	return done(resp.HTTPResponse, resp.Body)
}

func (c *Client) ImportInferenceEndpoint(ctx context.Context, workspaceID, address string) (InferenceEndpoint, error) {
	resp, err := c.api.ImportInferenceEndpointWithResponse(ctx, workspaceID, &apigen.ImportInferenceEndpointParams{Address: address})
	if err != nil {
		return InferenceEndpoint{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON200)

	return inferenceEndpointOf(dto), err
}

func (c *Client) ListInferenceEndpoints(ctx context.Context, workspaceID string) ([]InferenceEndpoint, error) {
	resp, err := c.api.ListInferenceEndpointsWithResponse(ctx, workspaceID, &apigen.ListInferenceEndpointsParams{})
	if err != nil {
		return nil, err
	}

	listed, err := answered(resp.HTTPResponse, resp.Body, resp.JSON200)
	if err != nil {
		return nil, err
	}

	out := make([]InferenceEndpoint, 0, len(listed.Items))
	for _, each := range listed.Items {
		out = append(out, inferenceEndpointOf(each))
	}

	return out, nil
}
