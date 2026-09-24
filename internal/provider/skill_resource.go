package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/client"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/fieldpath"
)

var (
	_ resource.Resource                = &SkillResource{}
	_ resource.ResourceWithConfigure   = &SkillResource{}
	_ resource.ResourceWithImportState = &SkillResource{}
	_ resource.ResourceWithIdentity    = &SkillResource{}
)

// NewSkillResource returns the spotlight_skill resource.
func NewSkillResource() resource.Resource {
	return &SkillResource{}
}

// SkillResource is the spotlight_skill resource.
type SkillResource struct {
	client *client.Client
}

// SkillResourceModel is a spotlight_skill as Terraform holds it.
type SkillResourceModel struct {
	ID          types.String `tfsdk:"id"`
	Version     types.Int64  `tfsdk:"version"`
	Address     types.String `tfsdk:"address"`
	TeamID      types.String `tfsdk:"team_id"`
	Key         types.String `tfsdk:"key"`
	Files       types.Map    `tfsdk:"files"`
	Description types.String `tfsdk:"description"`
	Secrets     types.List   `tfsdk:"secrets"`
	Tools       types.List   `tfsdk:"tools"`
}

var skillSecretType = types.ObjectType{AttrTypes: map[string]attr.Type{
	"name":        types.StringType,
	"description": types.StringType,
}}

var skillFields = fieldpath.Locator{Schema: skillSchema().Type()}

func (r *SkillResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_skill"
}

func (r *SkillResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = skillSchema()
}

func skillSchema() schema.Schema {
	return schema.Schema{
		MarkdownDescription: "A skill a team's stations can give their agents: a folder of files read from `SKILL.md`. Import it as `<workspace id>/<team key>/<key>`.",
		Attributes: map[string]schema.Attribute{
			"id":      idAttribute("skill"),
			"version": versionAttribute("skill"),
			"address": addressAttribute("core/grafana"),
			"team_id": ownerAttribute("team", "skill"),
			"key":     fixedAttribute("What stations give the skill by.", "skill"),
			"files": schema.MapAttribute{
				Required:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Each file in the skill's folder, by its path within it, usually read with `file()`. `SKILL.md` is the one the skill is read from.",
			},
			"description": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "What `SKILL.md` says the skill is for.",
			},
			"secrets": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "The secrets `SKILL.md` asks for.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name":        schema.StringAttribute{Computed: true, MarkdownDescription: "The secret's name."},
						"description": schema.StringAttribute{Computed: true, MarkdownDescription: "What the secret is for."},
					},
				},
			},
			"tools": schema.ListAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "The tools `SKILL.md` uses.",
			},
		},
	}
}

func (r *SkillResource) IdentitySchema(_ context.Context, _ resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = idIdentitySchema("skill")
}

func (r *SkillResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientOf(req.ProviderData, &resp.Diagnostics)
}

func (r *SkillResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan SkillResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	in, diags := skillInput(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	skill, err := r.client.CreateSkill(ctx, plan.TeamID.ValueString(), in)
	if err != nil {
		addAPIError(&resp.Diagnostics, "Could not create the skill", err, skillFields, client.ScopeConfigure)
		return
	}

	addAPIWarnings(&resp.Diagnostics, "skill", skill.Warnings, skillFields)
	resp.Diagnostics.Append(resp.State.Set(ctx, skillModel(skill, &plan))...)
	resp.Diagnostics.Append(setIdentity(ctx, resp.Identity, skill.ID)...)
	resp.Diagnostics.Append(rememberAnswer(ctx, resp.Private, skillSettable(skill))...)
}

func (r *SkillResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state SkillResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	skill, err := r.client.ReadSkill(ctx, state.ID.ValueString())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		addAPIError(&resp.Diagnostics, "Could not read the skill", err, skillFields, client.ScopeRead)
		return
	}

	kept := &state
	if !answerUnchanged(ctx, req.Private, skillSettable(skill)) {
		kept = nil
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, skillModel(skill, kept))...)
	resp.Diagnostics.Append(setIdentity(ctx, resp.Identity, skill.ID)...)
	resp.Diagnostics.Append(rememberAnswer(ctx, resp.Private, skillSettable(skill))...)
}

func (r *SkillResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state SkillResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	in, diags := skillInput(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	skill, err := r.client.UpdateSkill(ctx, state.ID.ValueString(), state.Version.ValueInt64(), in)
	if err != nil {
		addAPIError(&resp.Diagnostics, "Could not update the skill", err, skillFields, client.ScopeConfigure)
		return
	}

	addAPIWarnings(&resp.Diagnostics, "skill", skill.Warnings, skillFields)
	resp.Diagnostics.Append(resp.State.Set(ctx, skillModel(skill, &plan))...)
	resp.Diagnostics.Append(setIdentity(ctx, resp.Identity, skill.ID)...)
	resp.Diagnostics.Append(rememberAnswer(ctx, resp.Private, skillSettable(skill))...)
}

func (r *SkillResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state SkillResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteSkill(ctx, state.ID.ValueString(), state.Version.ValueInt64())
	if err != nil && !client.IsNotFound(err) {
		addAPIError(&resp.Diagnostics, "Could not delete the skill", err, skillFields, client.ScopeConfigure)
	}
}

func (r *SkillResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	target, ok := importTargetOf(ctx, req, &resp.Diagnostics, "<workspace id>/core/grafana")
	if !ok {
		return
	}

	var skill client.Skill
	var err error
	if target.byID() {
		skill, err = r.client.ReadSkill(ctx, target.id)
	} else {
		skill, err = r.client.ImportSkill(ctx, target.workspaceID, target.address)
	}
	if err != nil {
		addAPIError(&resp.Diagnostics, "Could not import the skill at "+target.String(), err, fieldpath.Locator{}, client.ScopeRead)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, skillModel(skill, nil))...)
	resp.Diagnostics.Append(setIdentity(ctx, resp.Identity, skill.ID)...)
	resp.Diagnostics.Append(rememberAnswer(ctx, resp.Private, skillSettable(skill))...)
}

func skillInput(ctx context.Context, plan SkillResourceModel) (client.SkillInput, diag.Diagnostics) {
	files := map[string]string{}
	diags := plan.Files.ElementsAs(ctx, &files, false)

	return client.SkillInput{Key: plan.Key.ValueString(), Files: files}, diags
}

func skillSettable(skill client.Skill) any {
	return client.SkillInput{Key: skill.Key, Files: skill.Files}
}

func skillModel(skill client.Skill, kept *SkillResourceModel) SkillResourceModel {
	secrets := make([]attr.Value, 0, len(skill.Secrets))
	for _, each := range skill.Secrets {
		secrets = append(secrets, types.ObjectValueMust(skillSecretType.AttrTypes, map[string]attr.Value{
			"name":        types.StringValue(each.Name),
			"description": types.StringValue(each.Description),
		}))
	}

	model := SkillResourceModel{
		ID:          types.StringValue(skill.ID),
		Version:     types.Int64Value(skill.Version),
		Address:     types.StringValue(skill.Address),
		TeamID:      types.StringValue(skill.TeamID),
		Key:         types.StringValue(skill.Key),
		Files:       stringMap(skill.Files),
		Description: types.StringValue(skill.Description),
		Secrets:     types.ListValueMust(skillSecretType, secrets),
		Tools:       stringList(skill.Tools),
	}

	if kept != nil {
		model.Key = kept.Key
		model.Files = kept.Files
	}

	return model
}
