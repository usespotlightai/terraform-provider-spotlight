package client

import (
	"context"

	"github.com/google/uuid"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/client/apigen"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/spotlight"
)

type runnerTrusts struct{ api *apigen.ClientWithResponses }

func runnerTrustOf(dto apigen.RunnerTrustResourceResponseDto) spotlight.RunnerTrust {
	return spotlight.RunnerTrust{
		Meta:        meta(dto.Id, dto.Version, dto.Address, dto.Warnings),
		WorkspaceID: dto.WorkspaceId,
		RunnerTrustInput: spotlight.RunnerTrustInput{
			Key:      dto.Key,
			Issuer:   dto.Issuer,
			Subject:  dto.Subject,
			Audience: dto.Audience,
			Labels:   orNone(dto.Labels),
		},
	}
}

func (r runnerTrusts) Create(ctx context.Context, workspaceID string, in spotlight.RunnerTrustInput) (spotlight.RunnerTrust, error) {
	body, err := jsonBody(in)
	if err != nil {
		return spotlight.RunnerTrust{}, err
	}

	resp, err := r.api.CreateRunnerTrustWithBodyWithResponse(ctx, workspaceID,
		&apigen.CreateRunnerTrustParams{IdempotencyKey: uuid.NewString()}, jsonContent, body)
	if err != nil {
		return spotlight.RunnerTrust{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON201)

	return runnerTrustOf(dto), err
}

func (r runnerTrusts) Read(ctx context.Context, id string) (spotlight.RunnerTrust, error) {
	resp, err := r.api.ReadRunnerTrustWithResponse(ctx, id, &apigen.ReadRunnerTrustParams{})
	if err != nil {
		return spotlight.RunnerTrust{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON200)

	return runnerTrustOf(dto), err
}

func (r runnerTrusts) Update(ctx context.Context, id string, version int64, in spotlight.RunnerTrustInput) (spotlight.RunnerTrust, error) {
	body, err := jsonBody(in)
	if err != nil {
		return spotlight.RunnerTrust{}, err
	}

	resp, err := r.api.UpdateRunnerTrustWithBodyWithResponse(ctx, id,
		&apigen.UpdateRunnerTrustParams{IfMatch: entityTag(version)}, jsonContent, body)
	if err != nil {
		return spotlight.RunnerTrust{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON200)

	return runnerTrustOf(dto), err
}

func (r runnerTrusts) Delete(ctx context.Context, id string, version int64) error {
	resp, err := r.api.DeleteRunnerTrustWithResponse(ctx, id, &apigen.DeleteRunnerTrustParams{IfMatch: entityTag(version)})
	if err != nil {
		return err
	}

	return done(resp.HTTPResponse, resp.Body)
}

func (r runnerTrusts) Import(ctx context.Context, workspaceID, address string) (spotlight.RunnerTrust, error) {
	resp, err := r.api.ImportRunnerTrustWithResponse(ctx, workspaceID, &apigen.ImportRunnerTrustParams{Address: address})
	if err != nil {
		return spotlight.RunnerTrust{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON200)

	return runnerTrustOf(dto), err
}
