package client

import (
	"context"

	"github.com/google/uuid"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/client/apigen"
)

// PoolTokenHeader is the header where Spotlight returns the token that it
// issues to a pull pool created without a token. Spotlight returns it one
// time only and does not return it again to a retry. The client does not
// read it.
const PoolTokenHeader = "Spotlight-Pool-Token"

func wholeOrNone(held *int) *int64 {
	if held == nil {
		return nil
	}

	whole := int64(*held)

	return &whole
}

func runnerPoolOf(dto apigen.RunnerPoolResourceResponseDto) RunnerPool {
	pool := RunnerPool{
		Meta:        meta(dto.Id, dto.Version, dto.Address, dto.Warnings),
		WorkspaceID: dto.WorkspaceId,
		Mode:        string(dto.Mode),
		RunnerPoolInput: RunnerPoolInput{
			Key:          dto.Key,
			Labels:       orNone(dto.Labels),
			MaxRuns:      wholeOrNone(dto.MaxRuns),
			TokenVersion: wholeOrNone(dto.TokenVersion),
		},
	}

	if dto.Provision != nil {
		pool.Provision = &PoolProvision{
			URL:           dto.Provision.Url,
			SecretVersion: wholeOrNone(dto.Provision.SecretVersion),
			Egress:        dto.Provision.Egress,
			EgressTraffic: trafficOf(dto.Provision.EgressTraffic),
		}
	}

	return pool
}

// CreateRunnerPool creates a runner pool in a workspace.
func (c *Client) CreateRunnerPool(ctx context.Context, workspaceID string, in RunnerPoolInput) (RunnerPool, error) {
	body, err := jsonBody(in)
	if err != nil {
		return RunnerPool{}, err
	}

	resp, err := c.api.CreateRunnerPoolWithBodyWithResponse(ctx, workspaceID,
		&apigen.CreateRunnerPoolParams{IdempotencyKey: uuid.NewString()}, jsonContent, body)
	if err != nil {
		return RunnerPool{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON201)
	if err != nil {
		return RunnerPool{}, err
	}

	resp.HTTPResponse.Header.Del(PoolTokenHeader)

	return runnerPoolOf(dto), nil
}

// ReadRunnerPool reads the runner pool with an id.
func (c *Client) ReadRunnerPool(ctx context.Context, id string) (RunnerPool, error) {
	resp, err := c.api.ReadRunnerPoolWithResponse(ctx, id, &apigen.ReadRunnerPoolParams{})
	if err != nil {
		return RunnerPool{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON200)

	return runnerPoolOf(dto), err
}

// UpdateRunnerPool writes the runner pool over version, the version from the
// last read.
func (c *Client) UpdateRunnerPool(ctx context.Context, id string, version int64, in RunnerPoolInput) (RunnerPool, error) {
	body, err := jsonBody(in)
	if err != nil {
		return RunnerPool{}, err
	}

	resp, err := c.api.UpdateRunnerPoolWithBodyWithResponse(ctx, id,
		&apigen.UpdateRunnerPoolParams{IfMatch: entityTag(version)}, jsonContent, body)
	if err != nil {
		return RunnerPool{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON200)

	return runnerPoolOf(dto), err
}

// DeleteRunnerPool deletes version of the runner pool, the version from the
// last read.
func (c *Client) DeleteRunnerPool(ctx context.Context, id string, version int64) error {
	resp, err := c.api.DeleteRunnerPoolWithResponse(ctx, id, &apigen.DeleteRunnerPoolParams{IfMatch: entityTag(version)})
	if err != nil {
		return err
	}

	return done(resp.HTTPResponse, resp.Body)
}

// ImportRunnerPool finds the runner pool at an address in a workspace.
func (c *Client) ImportRunnerPool(ctx context.Context, workspaceID, address string) (RunnerPool, error) {
	resp, err := c.api.ImportRunnerPoolWithResponse(ctx, workspaceID, &apigen.ImportRunnerPoolParams{Address: address})
	if err != nil {
		return RunnerPool{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON200)

	return runnerPoolOf(dto), err
}

// ListRunnerPools lists every runner pool in a workspace, in address order.
func (c *Client) ListRunnerPools(ctx context.Context, workspaceID string) ([]RunnerPool, error) {
	resp, err := c.api.ListRunnerPoolsWithResponse(ctx, workspaceID, &apigen.ListRunnerPoolsParams{})
	if err != nil {
		return nil, err
	}

	listed, err := answered(resp.HTTPResponse, resp.Body, resp.JSON200)
	if err != nil {
		return nil, err
	}

	out := make([]RunnerPool, 0, len(listed.Items))
	for _, each := range listed.Items {
		out = append(out, runnerPoolOf(each))
	}

	return out, nil
}

func trafficOf(traffic apigen.PoolProvisionResponseDtoEgressTraffic) *string {
	sent := string(traffic)
	if sent == "" {
		sent = EgressTrafficAll
	}
	return &sent
}
