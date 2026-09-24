package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/client"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/fieldpath"
)

var (
	_ resource.Resource                = &TeamResource{}
	_ resource.ResourceWithConfigure   = &TeamResource{}
	_ resource.ResourceWithImportState = &TeamResource{}
	_ resource.ResourceWithIdentity    = &TeamResource{}
)

// NewTeamResource returns the spotlight_team resource.
func NewTeamResource() resource.Resource {
	return &TeamResource{}
}

// TeamResource is the spotlight_team resource.
type TeamResource struct {
	client *client.Client
}

// TeamResourceModel is a spotlight_team as Terraform holds it.
type TeamResourceModel struct {
	ID          types.String `tfsdk:"id"`
	Version     types.Int64  `tfsdk:"version"`
	Address     types.String `tfsdk:"address"`
	WorkspaceID types.String `tfsdk:"workspace_id"`
	Key         types.String `tfsdk:"key"`
	Name        types.String `tfsdk:"name"`
}

var teamFields = fieldpath.Locator{Schema: teamSchema().Type()}

func (r *TeamResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_team"
}

func (r *TeamResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = teamSchema()
}

func teamSchema() schema.Schema {
	return schema.Schema{
		MarkdownDescription: "A team in a Spotlight workspace, holding its lines, skills and integrations. Import it as `<workspace id>/<key>`.",
		Attributes: map[string]schema.Attribute{
			"id":           idAttribute("team"),
			"version":      versionAttribute("team"),
			"address":      addressAttribute("core"),
			"workspace_id": ownerAttribute("workspace", "team"),
			"key":          fixedAttribute("Names the team in the addresses of everything it holds.", "team"),
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "What the team is called.",
			},
		},
	}
}

func (r *TeamResource) IdentitySchema(_ context.Context, _ resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = idIdentitySchema("team")
}

func (r *TeamResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientOf(req.ProviderData, &resp.Diagnostics)
}

func (r *TeamResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan TeamResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	team, err := r.client.CreateTeam(ctx, plan.WorkspaceID.ValueString(), teamInput(plan))
	if err != nil {
		addAPIError(&resp.Diagnostics, "Could not create the team", err, teamFields, client.ScopeConfigure)
		return
	}

	addAPIWarnings(&resp.Diagnostics, "team", team.Warnings, teamFields)
	resp.Diagnostics.Append(resp.State.Set(ctx, teamModel(team, &plan))...)
	resp.Diagnostics.Append(setIdentity(ctx, resp.Identity, team.ID)...)
	resp.Diagnostics.Append(rememberAnswer(ctx, resp.Private, teamSettable(team))...)
}

func (r *TeamResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state TeamResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	team, err := r.client.ReadTeam(ctx, state.ID.ValueString())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		addAPIError(&resp.Diagnostics, "Could not read the team", err, teamFields, client.ScopeRead)
		return
	}

	kept := &state
	if !answerUnchanged(ctx, req.Private, teamSettable(team)) {
		kept = nil
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, teamModel(team, kept))...)
	resp.Diagnostics.Append(setIdentity(ctx, resp.Identity, team.ID)...)
	resp.Diagnostics.Append(rememberAnswer(ctx, resp.Private, teamSettable(team))...)
}

func (r *TeamResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state TeamResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	team, err := r.client.UpdateTeam(ctx, state.ID.ValueString(), state.Version.ValueInt64(), teamInput(plan))
	if err != nil {
		addAPIError(&resp.Diagnostics, "Could not update the team", err, teamFields, client.ScopeConfigure)
		return
	}

	addAPIWarnings(&resp.Diagnostics, "team", team.Warnings, teamFields)
	resp.Diagnostics.Append(resp.State.Set(ctx, teamModel(team, &plan))...)
	resp.Diagnostics.Append(setIdentity(ctx, resp.Identity, team.ID)...)
	resp.Diagnostics.Append(rememberAnswer(ctx, resp.Private, teamSettable(team))...)
}

func (r *TeamResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state TeamResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteTeam(ctx, state.ID.ValueString(), state.Version.ValueInt64())
	if err != nil && !client.IsNotFound(err) {
		addAPIError(&resp.Diagnostics, "Could not delete the team", err, teamFields, client.ScopeConfigure)
	}
}

func (r *TeamResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	target, ok := importTargetOf(ctx, req, &resp.Diagnostics, "<workspace id>/core")
	if !ok {
		return
	}

	var team client.Team
	var err error
	if target.byID() {
		team, err = r.client.ReadTeam(ctx, target.id)
	} else {
		team, err = r.client.ImportTeam(ctx, target.workspaceID, target.address)
	}
	if err != nil {
		addAPIError(&resp.Diagnostics, "Could not import the team at "+target.String(), err, fieldpath.Locator{}, client.ScopeRead)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, teamModel(team, nil))...)
	resp.Diagnostics.Append(setIdentity(ctx, resp.Identity, team.ID)...)
	resp.Diagnostics.Append(rememberAnswer(ctx, resp.Private, teamSettable(team))...)
}

func teamInput(plan TeamResourceModel) client.TeamInput {
	return client.TeamInput{Key: plan.Key.ValueString(), Name: plan.Name.ValueString()}
}

func teamSettable(team client.Team) any {
	return client.TeamInput{Key: team.Key, Name: team.Name}
}

func teamModel(team client.Team, kept *TeamResourceModel) TeamResourceModel {
	model := TeamResourceModel{
		ID:          types.StringValue(team.ID),
		Version:     types.Int64Value(team.Version),
		Address:     types.StringValue(team.Address),
		WorkspaceID: types.StringValue(team.WorkspaceID),
		Key:         types.StringValue(team.Key),
		Name:        types.StringValue(team.Name),
	}

	if kept != nil {
		model.Key = kept.Key
		model.Name = kept.Name
	}

	return model
}
