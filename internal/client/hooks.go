package client

import (
	"context"

	"github.com/google/uuid"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/client/apigen"
)

// hookAnswer is a hook as the API answers it, with each step read as the
// JSON it is: a request's body is any JSON, not only the object the
// schema names.
type hookAnswer struct {
	apigen.HookResourceResponseDto
	Steps []any `json:"steps"`
}

type hookListAnswer struct {
	Items []hookAnswer `json:"items"`
}

func hookOf(dto hookAnswer) Hook {
	hook := Hook{
		Meta:   meta(dto.Id, dto.Version, dto.Address, dto.Warnings),
		TeamID: dto.TeamId,
		HookInput: HookInput{
			Key: dto.Key,
			On:  string(dto.On),
			Match: HookMatch{
				Tools:        orNone(dto.Match.Tools),
				Lines:        orNone(dto.Match.Lines),
				Stations:     orNone(dto.Match.Stations),
				Repositories: orNone(dto.Match.Repositories),
				Paths:        orNone(dto.Match.Paths),
			},
			When:           dto.When,
			Vars:           dto.Vars,
			Steps:          orNone(dto.Steps),
			Failure:        string(dto.Failure),
			TimeoutSeconds: int64(dto.TimeoutSeconds),
		},
	}

	if hook.Vars == nil {
		hook.Vars = map[string]string{}
	}

	if dto.Signing != nil {
		hook.Signing = &HookSigning{}
		if dto.Signing.SecretVersion != nil {
			version := int64(*dto.Signing.SecretVersion)
			hook.Signing.SecretVersion = &version
		}
		if dto.Signing.Oidc != nil {
			hook.Signing.OIDC = &HookOIDC{Audience: dto.Signing.Oidc.Audience}
		}
	}

	return hook
}

// CreateHook creates a hook in a team.
func (c *Client) CreateHook(ctx context.Context, teamID string, in HookInput) (Hook, error) {
	body, err := jsonBody(in)
	if err != nil {
		return Hook{}, err
	}

	dto, err := readAnswer[hookAnswer](c.api.CreateHookWithBody(ctx, teamID,
		&apigen.CreateHookParams{IdempotencyKey: uuid.NewString()}, jsonContent, body))

	return hookOf(dto), err
}

// ReadHook reads the hook with an id.
func (c *Client) ReadHook(ctx context.Context, id string) (Hook, error) {
	dto, err := readAnswer[hookAnswer](c.api.ReadHook(ctx, id, &apigen.ReadHookParams{}))

	return hookOf(dto), err
}

// UpdateHook writes the hook over version, the one last read.
func (c *Client) UpdateHook(ctx context.Context, id string, version int64, in HookInput) (Hook, error) {
	body, err := jsonBody(in)
	if err != nil {
		return Hook{}, err
	}

	dto, err := readAnswer[hookAnswer](c.api.UpdateHookWithBody(ctx, id,
		&apigen.UpdateHookParams{IfMatch: entityTag(version)}, jsonContent, body))

	return hookOf(dto), err
}

// DeleteHook deletes version of the hook, the one last read.
func (c *Client) DeleteHook(ctx context.Context, id string, version int64) error {
	resp, err := c.api.DeleteHookWithResponse(ctx, id, &apigen.DeleteHookParams{IfMatch: entityTag(version)})
	if err != nil {
		return err
	}

	return done(resp.HTTPResponse, resp.Body)
}

// ImportHook finds the hook at an address in a workspace.
func (c *Client) ImportHook(ctx context.Context, workspaceID, address string) (Hook, error) {
	dto, err := readAnswer[hookAnswer](c.api.ImportHook(ctx, workspaceID, &apigen.ImportHookParams{Address: address}))

	return hookOf(dto), err
}

// ListHooks lists every hook in a workspace, in address order.
func (c *Client) ListHooks(ctx context.Context, workspaceID string) ([]Hook, error) {
	listed, err := readAnswer[hookListAnswer](c.api.ListHooks(ctx, workspaceID, &apigen.ListHooksParams{}))
	if err != nil {
		return nil, err
	}

	out := make([]Hook, 0, len(listed.Items))
	for _, each := range listed.Items {
		out = append(out, hookOf(each))
	}

	return out, nil
}
