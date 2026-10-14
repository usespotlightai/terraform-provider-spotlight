package client

import (
	"context"

	"github.com/google/uuid"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/client/apigen"
)

func runnerTrustOf(dto apigen.RunnerTrustResourceResponseDto) RunnerTrust {
	return RunnerTrust{
		Meta:        meta(dto.Id, dto.Version, dto.Address, dto.Warnings),
		WorkspaceID: dto.WorkspaceId,
		RunnerTrustInput: RunnerTrustInput{
			Key:      dto.Key,
			Issuer:   dto.Issuer,
			Subject:  dto.Subject,
			Audience: dto.Audience,
			Labels:   orNone(dto.Labels),
		},
	}
}

// CreateRunnerTrust creates a runner trust in a workspace.
func (c *Client) CreateRunnerTrust(ctx context.Context, workspaceID string, in RunnerTrustInput) (RunnerTrust, error) {
	body, err := jsonBody(in)
	if err != nil {
		return RunnerTrust{}, err
	}

	resp, err := c.api.CreateRunnerTrustWithBodyWithResponse(ctx, workspaceID,
		&apigen.CreateRunnerTrustParams{IdempotencyKey: uuid.NewString()}, jsonContent, body)
	if err != nil {
		return RunnerTrust{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON201)

	return runnerTrustOf(dto), err
}

// ReadRunnerTrust reads the runner trust with an id.
func (c *Client) ReadRunnerTrust(ctx context.Context, id string) (RunnerTrust, error) {
	resp, err := c.api.ReadRunnerTrustWithResponse(ctx, id, &apigen.ReadRunnerTrustParams{})
	if err != nil {
		return RunnerTrust{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON200)

	return runnerTrustOf(dto), err
}

// UpdateRunnerTrust writes the runner trust over version, the version from the
// last read.
func (c *Client) UpdateRunnerTrust(ctx context.Context, id string, version int64, in RunnerTrustInput) (RunnerTrust, error) {
	body, err := jsonBody(in)
	if err != nil {
		return RunnerTrust{}, err
	}

	resp, err := c.api.UpdateRunnerTrustWithBodyWithResponse(ctx, id,
		&apigen.UpdateRunnerTrustParams{IfMatch: entityTag(version)}, jsonContent, body)
	if err != nil {
		return RunnerTrust{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON200)

	return runnerTrustOf(dto), err
}

// DeleteRunnerTrust deletes version of the runner trust, the version from the
// last read.
func (c *Client) DeleteRunnerTrust(ctx context.Context, id string, version int64) error {
	resp, err := c.api.DeleteRunnerTrustWithResponse(ctx, id, &apigen.DeleteRunnerTrustParams{IfMatch: entityTag(version)})
	if err != nil {
		return err
	}

	return done(resp.HTTPResponse, resp.Body)
}

// ImportRunnerTrust finds the runner trust at an address in a workspace.
func (c *Client) ImportRunnerTrust(ctx context.Context, workspaceID, address string) (RunnerTrust, error) {
	resp, err := c.api.ImportRunnerTrustWithResponse(ctx, workspaceID, &apigen.ImportRunnerTrustParams{Address: address})
	if err != nil {
		return RunnerTrust{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON200)

	return runnerTrustOf(dto), err
}

// ListRunnerTrusts lists every runner trust in a workspace, in address order.
func (c *Client) ListRunnerTrusts(ctx context.Context, workspaceID string) ([]RunnerTrust, error) {
	resp, err := c.api.ListRunnerTrustsWithResponse(ctx, workspaceID, &apigen.ListRunnerTrustsParams{})
	if err != nil {
		return nil, err
	}

	listed, err := answered(resp.HTTPResponse, resp.Body, resp.JSON200)
	if err != nil {
		return nil, err
	}

	out := make([]RunnerTrust, 0, len(listed.Items))
	for _, each := range listed.Items {
		out = append(out, runnerTrustOf(each))
	}

	return out, nil
}
