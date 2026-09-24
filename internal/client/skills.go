package client

import (
	"context"

	"github.com/google/uuid"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/client/apigen"
)

func skillOf(dto apigen.SkillResourceResponseDto) Skill {
	secrets := make([]SkillSecret, 0, len(dto.Secrets))
	for _, each := range dto.Secrets {
		secrets = append(secrets, SkillSecret{Name: each.Name, Description: each.Description})
	}

	return Skill{
		Meta:        meta(dto.Id, dto.Version, dto.Address, dto.Warnings),
		TeamID:      dto.TeamId,
		Key:         dto.Key,
		Files:       dto.Files,
		Description: dto.Description,
		Secrets:     secrets,
		Tools:       dto.Tools,
	}
}

// CreateSkill creates a skill in a team.
func (c *Client) CreateSkill(ctx context.Context, teamID string, in SkillInput) (Skill, error) {
	resp, err := c.api.CreateSkillWithResponse(ctx, teamID,
		&apigen.CreateSkillParams{IdempotencyKey: uuid.NewString()},
		apigen.CreateSkillJSONRequestBody{Key: in.Key, Files: in.Files})
	if err != nil {
		return Skill{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON201)

	return skillOf(dto), err
}

// ReadSkill reads the skill with an id.
func (c *Client) ReadSkill(ctx context.Context, id string) (Skill, error) {
	resp, err := c.api.ReadSkillWithResponse(ctx, id, &apigen.ReadSkillParams{})
	if err != nil {
		return Skill{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON200)

	return skillOf(dto), err
}

// UpdateSkill writes the skill over version, the one last read.
func (c *Client) UpdateSkill(ctx context.Context, id string, version int64, in SkillInput) (Skill, error) {
	resp, err := c.api.UpdateSkillWithResponse(ctx, id,
		&apigen.UpdateSkillParams{IfMatch: entityTag(version)},
		apigen.UpdateSkillJSONRequestBody{Key: in.Key, Files: in.Files})
	if err != nil {
		return Skill{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON200)

	return skillOf(dto), err
}

// DeleteSkill deletes version of the skill, the one last read.
func (c *Client) DeleteSkill(ctx context.Context, id string, version int64) error {
	resp, err := c.api.DeleteSkillWithResponse(ctx, id, &apigen.DeleteSkillParams{IfMatch: entityTag(version)})
	if err != nil {
		return err
	}

	return done(resp.HTTPResponse, resp.Body)
}

// ImportSkill finds the skill at an address in a workspace.
func (c *Client) ImportSkill(ctx context.Context, workspaceID, address string) (Skill, error) {
	resp, err := c.api.ImportSkillWithResponse(ctx, workspaceID, &apigen.ImportSkillParams{Address: address})
	if err != nil {
		return Skill{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON200)

	return skillOf(dto), err
}

// ListSkills lists every skill in a workspace, in address order.
func (c *Client) ListSkills(ctx context.Context, workspaceID string) ([]Skill, error) {
	resp, err := c.api.ListSkillsWithResponse(ctx, workspaceID, &apigen.ListSkillsParams{})
	if err != nil {
		return nil, err
	}

	listed, err := answered(resp.HTTPResponse, resp.Body, resp.JSON200)
	if err != nil {
		return nil, err
	}

	out := make([]Skill, 0, len(listed.Items))
	for _, each := range listed.Items {
		out = append(out, skillOf(each))
	}

	return out, nil
}
