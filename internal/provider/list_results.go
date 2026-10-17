package provider

import (
	"context"
	"iter"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/list"
	"github.com/hashicorp/terraform-plugin-framework/list/schema"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/client"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/fieldpath"
)

func workspaceIDListAttribute() schema.StringAttribute {
	return schema.StringAttribute{
		Required:            true,
		MarkdownDescription: "The workspace to list in.",
	}
}

func teamIDListAttribute() schema.StringAttribute {
	return schema.StringAttribute{
		Optional:            true,
		MarkdownDescription: "Lists only what this team has; every team's when left out.",
	}
}

func listFailed(doing string, err error) iter.Seq[list.ListResult] {
	var diags diag.Diagnostics
	addAPIError(&diags, doing, err, fieldpath.Locator{}, client.ScopeRead)

	return list.ListResultsStreamDiagnostics(diags)
}

func listResult(ctx context.Context, req list.ListRequest, id, address string, model any) list.ListResult {
	result := req.NewListResult(ctx)
	result.DisplayName = address
	result.Diagnostics.Append(setIdentity(ctx, result.Identity, id)...)

	if req.IncludeResource {
		result.Diagnostics.Append(result.Resource.Set(ctx, model)...)
	}

	return result
}

func limited[T any](items []T, limit int64) []T {
	if limit > 0 && int64(len(items)) > limit {
		return items[:limit]
	}

	return items
}

func ofTeam[T any](items []T, teamID string, team func(T) string) []T {
	if teamID == "" {
		return items
	}

	out := make([]T, 0, len(items))
	for _, each := range items {
		if team(each) == teamID {
			out = append(out, each)
		}
	}

	return out
}
