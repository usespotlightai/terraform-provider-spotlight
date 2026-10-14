package client

import (
	"context"

	"github.com/google/uuid"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/client/apigen"
)

func harnessProfileOf(dto apigen.HarnessProfileResourceResponseDto) HarnessProfile {
	servers := make([]MCPServer, 0, len(dto.McpServers))
	for _, each := range dto.McpServers {
		env := each.Env
		if env == nil {
			env = map[string]string{}
		}

		servers = append(servers, MCPServer{Name: each.Name, Command: each.Command, Args: orNone(each.Args), Env: env})
	}

	native := dto.Native
	if native == nil {
		native = map[string]any{}
	}

	return HarnessProfile{
		Meta:   meta(dto.Id, dto.Version, dto.Address, dto.Warnings),
		TeamID: dto.TeamId,
		HarnessProfileInput: HarnessProfileInput{
			Key: dto.Key,
			Permissions: Permissions{
				Allow: orNone(dto.Permissions.Allow),
				Ask:   orNone(dto.Permissions.Ask),
				Deny:  orNone(dto.Permissions.Deny),
			},
			MCPServers: servers,
			Native:     native,
		},
	}
}

// CreateHarnessProfile creates a harness profile in a team.
func (c *Client) CreateHarnessProfile(ctx context.Context, teamID string, in HarnessProfileInput) (HarnessProfile, error) {
	body, err := jsonBody(in)
	if err != nil {
		return HarnessProfile{}, err
	}

	resp, err := c.api.CreateHarnessProfileWithBodyWithResponse(ctx, teamID,
		&apigen.CreateHarnessProfileParams{IdempotencyKey: uuid.NewString()}, jsonContent, body)
	if err != nil {
		return HarnessProfile{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON201)

	return harnessProfileOf(dto), err
}

// ReadHarnessProfile reads the harness profile with an id.
func (c *Client) ReadHarnessProfile(ctx context.Context, id string) (HarnessProfile, error) {
	resp, err := c.api.ReadHarnessProfileWithResponse(ctx, id, &apigen.ReadHarnessProfileParams{})
	if err != nil {
		return HarnessProfile{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON200)

	return harnessProfileOf(dto), err
}

// UpdateHarnessProfile writes the harness profile over version, the version
// from the last read.
func (c *Client) UpdateHarnessProfile(ctx context.Context, id string, version int64, in HarnessProfileInput) (HarnessProfile, error) {
	body, err := jsonBody(in)
	if err != nil {
		return HarnessProfile{}, err
	}

	resp, err := c.api.UpdateHarnessProfileWithBodyWithResponse(ctx, id,
		&apigen.UpdateHarnessProfileParams{IfMatch: entityTag(version)}, jsonContent, body)
	if err != nil {
		return HarnessProfile{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON200)

	return harnessProfileOf(dto), err
}

// DeleteHarnessProfile deletes version of the harness profile, the version
// from the last read.
func (c *Client) DeleteHarnessProfile(ctx context.Context, id string, version int64) error {
	resp, err := c.api.DeleteHarnessProfileWithResponse(ctx, id, &apigen.DeleteHarnessProfileParams{IfMatch: entityTag(version)})
	if err != nil {
		return err
	}

	return done(resp.HTTPResponse, resp.Body)
}

// ImportHarnessProfile finds the harness profile at an address in a workspace.
func (c *Client) ImportHarnessProfile(ctx context.Context, workspaceID, address string) (HarnessProfile, error) {
	resp, err := c.api.ImportHarnessProfileWithResponse(ctx, workspaceID, &apigen.ImportHarnessProfileParams{Address: address})
	if err != nil {
		return HarnessProfile{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON200)

	return harnessProfileOf(dto), err
}

// ListHarnessProfiles lists every harness profile in a workspace, in address order.
func (c *Client) ListHarnessProfiles(ctx context.Context, workspaceID string) ([]HarnessProfile, error) {
	resp, err := c.api.ListHarnessProfilesWithResponse(ctx, workspaceID, &apigen.ListHarnessProfilesParams{})
	if err != nil {
		return nil, err
	}

	listed, err := answered(resp.HTTPResponse, resp.Body, resp.JSON200)
	if err != nil {
		return nil, err
	}

	out := make([]HarnessProfile, 0, len(listed.Items))
	for _, each := range listed.Items {
		out = append(out, harnessProfileOf(each))
	}

	return out, nil
}
