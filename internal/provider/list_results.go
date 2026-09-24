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
		MarkdownDescription: "Lists only what this team holds; every team's when left out.",
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

func limited[T any](held []T, limit int64) []T {
	if limit > 0 && int64(len(held)) > limit {
		return held[:limit]
	}

	return held
}

func heldBy[T any](held []T, teamID string, team func(T) string) []T {
	if teamID == "" {
		return held
	}

	out := make([]T, 0, len(held))
	for _, each := range held {
		if team(each) == teamID {
			out = append(out, each)
		}
	}

	return out
}
