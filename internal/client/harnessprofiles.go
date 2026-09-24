package client

import (
	"context"

	"github.com/google/uuid"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/client/apigen"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/spotlight"
)

type harnessProfiles struct{ api *apigen.ClientWithResponses }

func harnessProfileOf(dto apigen.HarnessProfileResourceResponseDto) spotlight.HarnessProfile {
	servers := make([]spotlight.MCPServer, 0, len(dto.McpServers))
	for _, each := range dto.McpServers {
		env := each.Env
		if env == nil {
			env = map[string]string{}
		}

		servers = append(servers, spotlight.MCPServer{Name: each.Name, Command: each.Command, Args: orNone(each.Args), Env: env})
	}

	native := dto.Native
	if native == nil {
		native = map[string]any{}
	}

	return spotlight.HarnessProfile{
		Meta:   meta(dto.Id, dto.Version, dto.Address, dto.Warnings),
		TeamID: dto.TeamId,
		HarnessProfileInput: spotlight.HarnessProfileInput{
			Key: dto.Key,
			Permissions: spotlight.Permissions{
				Allow: orNone(dto.Permissions.Allow),
				Ask:   orNone(dto.Permissions.Ask),
				Deny:  orNone(dto.Permissions.Deny),
			},
			MCPServers: servers,
			Native:     native,
		},
	}
}

func (h harnessProfiles) Create(ctx context.Context, teamID string, in spotlight.HarnessProfileInput) (spotlight.HarnessProfile, error) {
	body, err := jsonBody(in)
	if err != nil {
		return spotlight.HarnessProfile{}, err
	}

	resp, err := h.api.CreateHarnessProfileWithBodyWithResponse(ctx, teamID,
		&apigen.CreateHarnessProfileParams{IdempotencyKey: uuid.NewString()}, jsonContent, body)
	if err != nil {
		return spotlight.HarnessProfile{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON201)

	return harnessProfileOf(dto), err
}

func (h harnessProfiles) Read(ctx context.Context, id string) (spotlight.HarnessProfile, error) {
	resp, err := h.api.ReadHarnessProfileWithResponse(ctx, id, &apigen.ReadHarnessProfileParams{})
	if err != nil {
		return spotlight.HarnessProfile{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON200)

	return harnessProfileOf(dto), err
}

func (h harnessProfiles) Update(ctx context.Context, id string, version int64, in spotlight.HarnessProfileInput) (spotlight.HarnessProfile, error) {
	body, err := jsonBody(in)
	if err != nil {
		return spotlight.HarnessProfile{}, err
	}

	resp, err := h.api.UpdateHarnessProfileWithBodyWithResponse(ctx, id,
		&apigen.UpdateHarnessProfileParams{IfMatch: entityTag(version)}, jsonContent, body)
	if err != nil {
		return spotlight.HarnessProfile{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON200)

	return harnessProfileOf(dto), err
}

func (h harnessProfiles) Delete(ctx context.Context, id string, version int64) error {
	resp, err := h.api.DeleteHarnessProfileWithResponse(ctx, id, &apigen.DeleteHarnessProfileParams{IfMatch: entityTag(version)})
	if err != nil {
		return err
	}

	return done(resp.HTTPResponse, resp.Body)
}

func (h harnessProfiles) Import(ctx context.Context, workspaceID, address string) (spotlight.HarnessProfile, error) {
	resp, err := h.api.ImportHarnessProfileWithResponse(ctx, workspaceID, &apigen.ImportHarnessProfileParams{Address: address})
	if err != nil {
		return spotlight.HarnessProfile{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON200)

	return harnessProfileOf(dto), err
}
