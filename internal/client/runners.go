package client

import (
	"context"

	"github.com/google/uuid"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/client/apigen"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/spotlight"
)

// RunnerTokenHeader is where Spotlight answers the token it issues a runner
// created without one, that once and never again, not even to a retry. The
// client drops it unread.
const RunnerTokenHeader = "Spotlight-Runner-Token"

type runners struct{ api *apigen.ClientWithResponses }

func runnerOf(dto apigen.RunnerResourceResponseDto) spotlight.Runner {
	runner := spotlight.Runner{
		Meta:        meta(dto.Id, dto.Version, dto.Address, dto.Warnings),
		WorkspaceID: dto.WorkspaceId,
		Name:        dto.Name,
		Labels:      orNone(dto.Labels),
	}

	if dto.Trust != nil {
		runner.Trust = *dto.Trust
	}

	return runner
}

func (r runners) Create(ctx context.Context, workspaceID string, in spotlight.RunnerInput) (spotlight.Runner, error) {
	body, err := jsonBody(in)
	if err != nil {
		return spotlight.Runner{}, err
	}

	resp, err := r.api.CreateRunnerWithBodyWithResponse(ctx, workspaceID,
		&apigen.CreateRunnerParams{IdempotencyKey: uuid.NewString()}, jsonContent, body)
	if err != nil {
		return spotlight.Runner{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON201)
	if err != nil {
		return spotlight.Runner{}, err
	}

	resp.HTTPResponse.Header.Del(RunnerTokenHeader)

	runner := runnerOf(dto)
	runner.IssuedToken = in.Token == ""

	return runner, nil
}

func (r runners) Read(ctx context.Context, id string) (spotlight.Runner, error) {
	resp, err := r.api.ReadRunnerWithResponse(ctx, id, &apigen.ReadRunnerParams{})
	if err != nil {
		return spotlight.Runner{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON200)

	return runnerOf(dto), err
}

func (r runners) Update(ctx context.Context, id string, version int64, in spotlight.RunnerInput) (spotlight.Runner, error) {
	body, err := jsonBody(in)
	if err != nil {
		return spotlight.Runner{}, err
	}

	resp, err := r.api.UpdateRunnerWithBodyWithResponse(ctx, id,
		&apigen.UpdateRunnerParams{IfMatch: entityTag(version)}, jsonContent, body)
	if err != nil {
		return spotlight.Runner{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON200)

	return runnerOf(dto), err
}

func (r runners) Delete(ctx context.Context, id string, version int64) error {
	resp, err := r.api.DeleteRunnerWithResponse(ctx, id, &apigen.DeleteRunnerParams{IfMatch: entityTag(version)})
	if err != nil {
		return err
	}

	return done(resp.HTTPResponse, resp.Body)
}

func (r runners) Import(ctx context.Context, workspaceID, address string) (spotlight.Runner, error) {
	resp, err := r.api.ImportRunnerWithResponse(ctx, workspaceID, &apigen.ImportRunnerParams{Address: address})
	if err != nil {
		return spotlight.Runner{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON200)

	return runnerOf(dto), err
}
