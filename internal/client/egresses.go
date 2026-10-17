package client

import (
	"context"

	"github.com/google/uuid"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/client/apigen"
)

// EgressTokenHeader is the header where Spotlight returns the token that it
// issues to an egress created without a token. Spotlight returns it one time
// only and does not return it again to a retry. The client does not read it.
const EgressTokenHeader = "Spotlight-Egress-Token"

func egressOf(dto apigen.EgressResourceResponseDto) Egress {
	return Egress{
		Meta:        meta(dto.Id, dto.Version, dto.Address, dto.Warnings),
		WorkspaceID: dto.WorkspaceId,
		EgressInput: EgressInput{
			Key:          dto.Key,
			URL:          dto.Url,
			TokenVersion: wholeOrNone(dto.TokenVersion),
		},
	}
}

// CreateEgress creates an egress in a workspace.
func (c *Client) CreateEgress(ctx context.Context, workspaceID string, in EgressInput) (Egress, error) {
	body, err := jsonBody(in)
	if err != nil {
		return Egress{}, err
	}

	resp, err := c.api.CreateEgressWithBodyWithResponse(ctx, workspaceID,
		&apigen.CreateEgressParams{IdempotencyKey: uuid.NewString()}, jsonContent, body)
	if err != nil {
		return Egress{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON201)
	if err != nil {
		return Egress{}, err
	}

	resp.HTTPResponse.Header.Del(EgressTokenHeader)

	return egressOf(dto), nil
}

// ReadEgress reads the egress with an id.
func (c *Client) ReadEgress(ctx context.Context, id string) (Egress, error) {
	resp, err := c.api.ReadEgressWithResponse(ctx, id, &apigen.ReadEgressParams{})
	if err != nil {
		return Egress{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON200)

	return egressOf(dto), err
}

// UpdateEgress writes the egress over version, the version from the last
// read.
func (c *Client) UpdateEgress(ctx context.Context, id string, version int64, in EgressInput) (Egress, error) {
	body, err := jsonBody(in)
	if err != nil {
		return Egress{}, err
	}

	resp, err := c.api.UpdateEgressWithBodyWithResponse(ctx, id,
		&apigen.UpdateEgressParams{IfMatch: entityTag(version)}, jsonContent, body)
	if err != nil {
		return Egress{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON200)

	return egressOf(dto), err
}

// DeleteEgress deletes version of the egress, the version from the last
// read. Spotlight does not delete it while a runner pool names the egress.
func (c *Client) DeleteEgress(ctx context.Context, id string, version int64) error {
	resp, err := c.api.DeleteEgressWithResponse(ctx, id, &apigen.DeleteEgressParams{IfMatch: entityTag(version)})
	if err != nil {
		return err
	}

	return done(resp.HTTPResponse, resp.Body)
}

// ImportEgress finds the egress at an address in a workspace.
func (c *Client) ImportEgress(ctx context.Context, workspaceID, address string) (Egress, error) {
	resp, err := c.api.ImportEgressWithResponse(ctx, workspaceID, &apigen.ImportEgressParams{Address: address})
	if err != nil {
		return Egress{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON200)

	return egressOf(dto), err
}

// ListEgresses lists every egress in a workspace, in address order.
func (c *Client) ListEgresses(ctx context.Context, workspaceID string) ([]Egress, error) {
	resp, err := c.api.ListEgressesWithResponse(ctx, workspaceID, &apigen.ListEgressesParams{})
	if err != nil {
		return nil, err
	}

	listed, err := answered(resp.HTTPResponse, resp.Body, resp.JSON200)
	if err != nil {
		return nil, err
	}

	out := make([]Egress, 0, len(listed.Items))
	for _, each := range listed.Items {
		out = append(out, egressOf(each))
	}

	return out, nil
}
