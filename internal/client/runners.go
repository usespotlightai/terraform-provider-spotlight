package client

import (
	"context"

	"github.com/google/uuid"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/client/apigen"
)

// RunnerTokenHeader is where Spotlight answers the token it issues a runner
// created without one, that once and never again, not even to a retry. The
// client drops it unread.
const RunnerTokenHeader = "Spotlight-Runner-Token"

func runnerOf(dto apigen.RunnerResourceResponseDto) Runner {
	runner := Runner{
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

// CreateRunner creates a runner in a workspace.
func (c *Client) CreateRunner(ctx context.Context, workspaceID string, in RunnerInput) (Runner, error) {
	body, err := jsonBody(in)
	if err != nil {
		return Runner{}, err
	}

	resp, err := c.api.CreateRunnerWithBodyWithResponse(ctx, workspaceID,
		&apigen.CreateRunnerParams{IdempotencyKey: uuid.NewString()}, jsonContent, body)
	if err != nil {
		return Runner{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON201)
	if err != nil {
		return Runner{}, err
	}

	resp.HTTPResponse.Header.Del(RunnerTokenHeader)

	runner := runnerOf(dto)
	runner.IssuedToken = in.Token == ""

	return runner, nil
}

// ReadRunner reads the runner with an id.
func (c *Client) ReadRunner(ctx context.Context, id string) (Runner, error) {
	resp, err := c.api.ReadRunnerWithResponse(ctx, id, &apigen.ReadRunnerParams{})
	if err != nil {
		return Runner{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON200)

	return runnerOf(dto), err
}

// UpdateRunner writes the runner over version, the one last read.
func (c *Client) UpdateRunner(ctx context.Context, id string, version int64, in RunnerInput) (Runner, error) {
	body, err := jsonBody(in)
	if err != nil {
		return Runner{}, err
	}

	resp, err := c.api.UpdateRunnerWithBodyWithResponse(ctx, id,
		&apigen.UpdateRunnerParams{IfMatch: entityTag(version)}, jsonContent, body)
	if err != nil {
		return Runner{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON200)

	return runnerOf(dto), err
}

// DeleteRunner deletes version of the runner, the one last read.
func (c *Client) DeleteRunner(ctx context.Context, id string, version int64) error {
	resp, err := c.api.DeleteRunnerWithResponse(ctx, id, &apigen.DeleteRunnerParams{IfMatch: entityTag(version)})
	if err != nil {
		return err
	}

	return done(resp.HTTPResponse, resp.Body)
}

// ImportRunner finds the runner at an address in a workspace.
func (c *Client) ImportRunner(ctx context.Context, workspaceID, address string) (Runner, error) {
	resp, err := c.api.ImportRunnerWithResponse(ctx, workspaceID, &apigen.ImportRunnerParams{Address: address})
	if err != nil {
		return Runner{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON200)

	return runnerOf(dto), err
}

// ListRunners lists every runner in a workspace, in address order.
func (c *Client) ListRunners(ctx context.Context, workspaceID string) ([]Runner, error) {
	resp, err := c.api.ListRunnersWithResponse(ctx, workspaceID, &apigen.ListRunnersParams{})
	if err != nil {
		return nil, err
	}

	listed, err := answered(resp.HTTPResponse, resp.Body, resp.JSON200)
	if err != nil {
		return nil, err
	}

	out := make([]Runner, 0, len(listed.Items))
	for _, each := range listed.Items {
		out = append(out, runnerOf(each))
	}

	return out, nil
}
