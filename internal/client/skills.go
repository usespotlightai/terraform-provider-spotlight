package client

import (
	"context"

	"github.com/google/uuid"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/client/apigen"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/spotlight"
)

type skills struct{ api *apigen.ClientWithResponses }

func skillOf(dto apigen.SkillResourceResponseDto) spotlight.Skill {
	secrets := make([]spotlight.SkillSecret, 0, len(dto.Secrets))
	for _, each := range dto.Secrets {
		secrets = append(secrets, spotlight.SkillSecret{Name: each.Name, Description: each.Description})
	}

	return spotlight.Skill{
		Meta:        meta(dto.Id, dto.Version, dto.Address, dto.Warnings),
		TeamID:      dto.TeamId,
		Key:         dto.Key,
		Files:       dto.Files,
		Description: dto.Description,
		Secrets:     secrets,
		Tools:       dto.Tools,
	}
}

func (s skills) Create(ctx context.Context, teamID string, in spotlight.SkillInput) (spotlight.Skill, error) {
	resp, err := s.api.CreateSkillWithResponse(ctx, teamID,
		&apigen.CreateSkillParams{IdempotencyKey: uuid.NewString()},
		apigen.CreateSkillJSONRequestBody{Key: in.Key, Files: in.Files})
	if err != nil {
		return spotlight.Skill{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON201)

	return skillOf(dto), err
}

func (s skills) Read(ctx context.Context, id string) (spotlight.Skill, error) {
	resp, err := s.api.ReadSkillWithResponse(ctx, id, &apigen.ReadSkillParams{})
	if err != nil {
		return spotlight.Skill{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON200)

	return skillOf(dto), err
}

func (s skills) Update(ctx context.Context, id string, version int64, in spotlight.SkillInput) (spotlight.Skill, error) {
	resp, err := s.api.UpdateSkillWithResponse(ctx, id,
		&apigen.UpdateSkillParams{IfMatch: entityTag(version)},
		apigen.UpdateSkillJSONRequestBody{Key: in.Key, Files: in.Files})
	if err != nil {
		return spotlight.Skill{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON200)

	return skillOf(dto), err
}

func (s skills) Delete(ctx context.Context, id string, version int64) error {
	resp, err := s.api.DeleteSkillWithResponse(ctx, id, &apigen.DeleteSkillParams{IfMatch: entityTag(version)})
	if err != nil {
		return err
	}

	return done(resp.HTTPResponse, resp.Body)
}

func (s skills) Import(ctx context.Context, workspaceID, address string) (spotlight.Skill, error) {
	resp, err := s.api.ImportSkillWithResponse(ctx, workspaceID, &apigen.ImportSkillParams{Address: address})
	if err != nil {
		return spotlight.Skill{}, err
	}

	dto, err := answered(resp.HTTPResponse, resp.Body, resp.JSON200)

	return skillOf(dto), err
}
