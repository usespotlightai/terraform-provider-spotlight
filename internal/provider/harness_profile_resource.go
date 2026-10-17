package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/dynamicdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectdefault"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/client"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/dynamic"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/fieldpath"
)

var (
	_ resource.Resource                = &HarnessProfileResource{}
	_ resource.ResourceWithConfigure   = &HarnessProfileResource{}
	_ resource.ResourceWithImportState = &HarnessProfileResource{}
	_ resource.ResourceWithIdentity    = &HarnessProfileResource{}
)

// NewHarnessProfileResource returns the spotlight_harness_profile resource.
func NewHarnessProfileResource() resource.Resource {
	return &HarnessProfileResource{}
}

// HarnessProfileResource is the spotlight_harness_profile resource.
type HarnessProfileResource struct {
	client *client.Client
}

// HarnessProfileResourceModel is a spotlight_harness_profile as Terraform stores it.
type HarnessProfileResourceModel struct {
	ID          types.String              `tfsdk:"id"`
	Version     types.Int64               `tfsdk:"version"`
	Address     types.String              `tfsdk:"address"`
	TeamID      types.String              `tfsdk:"team_id"`
	Key         types.String              `tfsdk:"key"`
	Permissions harnessProfilePermissions `tfsdk:"permissions"`
	MCPServers  []harnessProfileMCPServer `tfsdk:"mcp_servers"`
	Native      types.Dynamic             `tfsdk:"native"`
}

type harnessProfilePermissions struct {
	Allow types.List `tfsdk:"allow"`
	Ask   types.List `tfsdk:"ask"`
	Deny  types.List `tfsdk:"deny"`
}

type harnessProfileMCPServer struct {
	Name    types.String `tfsdk:"name"`
	Command types.String `tfsdk:"command"`
	Args    types.List   `tfsdk:"args"`
	Env     types.Map    `tfsdk:"env"`
}

var (
	harnessProfileRules          = types.ListType{ElemType: types.StringType}
	harnessProfilePermissionType = map[string]attr.Type{"allow": harnessProfileRules, "ask": harnessProfileRules, "deny": harnessProfileRules}
	harnessProfileServerType     = types.ObjectType{AttrTypes: map[string]attr.Type{
		"name":    types.StringType,
		"command": types.StringType,
		"args":    harnessProfileRules,
		"env":     types.MapType{ElemType: types.StringType},
	}}
	harnessProfileNoNative = types.DynamicValue(types.ObjectValueMust(map[string]attr.Type{}, map[string]attr.Value{}))
)

var harnessProfileFields = fieldpath.Locator{Schema: harnessProfileSchema().Type()}

func (r *HarnessProfileResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_harness_profile"
}

func (r *HarnessProfileResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = harnessProfileSchema()
}

func harnessProfileRulesAttribute(description string) schema.ListAttribute {
	return stringListAttribute(description + " Each is `read`, `edit` or `execute`, alone or with a pattern, as in `execute(npm test *)` or `read(.env)`.")
}

func harnessProfileSchema() schema.Schema {
	return schema.Schema{
		MarkdownDescription: "A team's harness settings, which a station names in `profile`: permissions every harness is told in its own terms, MCP servers, and each harness's own settings passed through as it reads them. " +
			"Where a harness cannot say a rule, the answer's warnings name the approximation. Import it as `<workspace id>/<team key>/<key>`.",
		Attributes: map[string]schema.Attribute{
			"id":      idAttribute("harness profile"),
			"version": versionAttribute("harness profile"),
			"address": addressAttribute("core/careful"),
			"team_id": ownerAttribute("team", "harness profile"),
			"key":     fixedAttribute("What stations name the profile by, as in `careful`.", "harness profile"),
			"permissions": schema.SingleNestedAttribute{
				Optional: true,
				Computed: true,
				Default: objectdefault.StaticValue(types.ObjectValueMust(harnessProfilePermissionType, map[string]attr.Value{
					"allow": stringList(nil),
					"ask":   stringList(nil),
					"deny":  stringList(nil),
				})),
				MarkdownDescription: "What the harness may do, what it asks a person about first, and what it may never do.",
				Attributes: map[string]schema.Attribute{
					"allow": harnessProfileRulesAttribute("What the harness may do without asking."),
					"ask":   harnessProfileRulesAttribute("What the harness asks a person about first; the ask goes to their inbox as an approval."),
					"deny":  harnessProfileRulesAttribute("What the harness may never do."),
				},
			},
			"mcp_servers": schema.ListNestedAttribute{
				Optional:            true,
				Computed:            true,
				Default:             listdefault.StaticValue(types.ListValueMust(harnessProfileServerType, []attr.Value{})),
				MarkdownDescription: "Stdio MCP servers, handed to each harness in its own format.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Required:            true,
							MarkdownDescription: "Names the server, in lower case words joined by dashes, as in `linear`.",
						},
						"command": schema.StringAttribute{
							Required:            true,
							MarkdownDescription: "What starts the server, as in `npx`.",
						},
						"args": stringListAttribute("The arguments of the command."),
						"env": schema.MapAttribute{
							Optional:            true,
							Computed:            true,
							ElementType:         types.StringType,
							Default:             mapdefault.StaticValue(types.MapValueMust(types.StringType, map[string]attr.Value{})),
							MarkdownDescription: "The server's environment. A value is plain text, stored and shown as written, or a reference to a workspace secret such as `secret.LINEAR_API_KEY`: the secret must exist, cannot be deleted while referenced, and goes to the server only as a stand-in, since egress injects its value into the server's requests to the hosts it is bound to.",
						},
					},
				},
			},
			"native": schema.DynamicAttribute{
				Optional:            true,
				Computed:            true,
				Default:             dynamicdefault.StaticValue(harnessProfileNoNative),
				MarkdownDescription: "Each harness's own settings by harness, passed through as it reads them: Claude Code settings as an object, Codex `config.toml` as text, Cursor CLI config as an object; as in `{ claude-code = { includeCoAuthoredBy = false } }`.",
			},
		},
	}
}

func (r *HarnessProfileResource) IdentitySchema(_ context.Context, _ resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = idIdentitySchema("harness profile")
}

func (r *HarnessProfileResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientOf(req.ProviderData, &resp.Diagnostics)
}

func (r *HarnessProfileResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan HarnessProfileResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	in, diags := harnessProfileInput(plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	harnessProfile, err := r.client.CreateHarnessProfile(ctx, plan.TeamID.ValueString(), in)
	if err != nil {
		addAPIError(&resp.Diagnostics, "Could not create the harness profile", err, harnessProfileFields, client.ScopeConfigure)
		return
	}

	addAPIWarnings(&resp.Diagnostics, "harness profile", harnessProfile.Warnings, harnessProfileFields)
	resp.Diagnostics.Append(resp.State.Set(ctx, harnessProfileModel(harnessProfile, &plan))...)
	resp.Diagnostics.Append(setIdentity(ctx, resp.Identity, harnessProfile.ID)...)
	resp.Diagnostics.Append(rememberAnswer(ctx, resp.Private, harnessProfileSettable(harnessProfile))...)
}

func (r *HarnessProfileResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state HarnessProfileResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	harnessProfile, err := r.client.ReadHarnessProfile(ctx, state.ID.ValueString())
	if client.IsNotFound(err) {
		removed(ctx, resp, state.ID.ValueString())
		return
	}
	if err != nil {
		addAPIError(&resp.Diagnostics, "Could not read the harness profile", err, harnessProfileFields, client.ScopeRead)
		return
	}

	prior := &state
	if !answerUnchanged(ctx, req.Private, harnessProfileSettable(harnessProfile)) {
		prior = nil
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, harnessProfileModel(harnessProfile, prior))...)
	resp.Diagnostics.Append(setIdentity(ctx, resp.Identity, harnessProfile.ID)...)
	resp.Diagnostics.Append(rememberAnswer(ctx, resp.Private, harnessProfileSettable(harnessProfile))...)
}

func (r *HarnessProfileResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state HarnessProfileResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	in, diags := harnessProfileInput(plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	harnessProfile, err := r.client.UpdateHarnessProfile(ctx, state.ID.ValueString(), state.Version.ValueInt64(), in)
	if err != nil {
		addAPIError(&resp.Diagnostics, "Could not update the harness profile", err, harnessProfileFields, client.ScopeConfigure)
		return
	}

	addAPIWarnings(&resp.Diagnostics, "harness profile", harnessProfile.Warnings, harnessProfileFields)
	resp.Diagnostics.Append(resp.State.Set(ctx, harnessProfileModel(harnessProfile, &plan))...)
	resp.Diagnostics.Append(setIdentity(ctx, resp.Identity, harnessProfile.ID)...)
	resp.Diagnostics.Append(rememberAnswer(ctx, resp.Private, harnessProfileSettable(harnessProfile))...)
}

func (r *HarnessProfileResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state HarnessProfileResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteHarnessProfile(ctx, state.ID.ValueString(), state.Version.ValueInt64())
	if err != nil && !client.IsNotFound(err) {
		addAPIError(&resp.Diagnostics, "Could not delete the harness profile", err, harnessProfileFields, client.ScopeConfigure)
	}
}

func (r *HarnessProfileResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	target, ok := importTargetOf(ctx, req, &resp.Diagnostics, "<workspace id>/core/careful")
	if !ok {
		return
	}

	var harnessProfile client.HarnessProfile
	var err error
	if target.byID() {
		harnessProfile, err = r.client.ReadHarnessProfile(ctx, target.id)
	} else {
		harnessProfile, err = r.client.ImportHarnessProfile(ctx, target.workspaceID, target.address)
	}
	if err != nil {
		addAPIError(&resp.Diagnostics, "Could not import the harness profile at "+target.String(), err, fieldpath.Locator{}, client.ScopeRead)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, harnessProfileModel(harnessProfile, nil))...)
	resp.Diagnostics.Append(setIdentity(ctx, resp.Identity, harnessProfile.ID)...)
	resp.Diagnostics.Append(rememberAnswer(ctx, resp.Private, harnessProfileSettable(harnessProfile))...)
}

func harnessProfileSettable(profile client.HarnessProfile) any {
	return profile.HarnessProfileInput
}

func harnessProfileInput(plan HarnessProfileResourceModel) (client.HarnessProfileInput, diag.Diagnostics) {
	var diags diag.Diagnostics

	in := client.HarnessProfileInput{
		Key: plan.Key.ValueString(),
		Permissions: client.Permissions{
			Allow: stringsIn(plan.Permissions.Allow, &diags),
			Ask:   stringsIn(plan.Permissions.Ask, &diags),
			Deny:  stringsIn(plan.Permissions.Deny, &diags),
		},
		MCPServers: make([]client.MCPServer, 0, len(plan.MCPServers)),
		Native:     map[string]any{},
	}

	for _, each := range plan.MCPServers {
		env := map[string]string{}
		if !each.Env.IsNull() && !each.Env.IsUnknown() {
			diags.Append(each.Env.ElementsAs(context.Background(), &env, false)...)
		}

		in.MCPServers = append(in.MCPServers, client.MCPServer{
			Name:    each.Name.ValueString(),
			Command: each.Command.ValueString(),
			Args:    stringsIn(each.Args, &diags),
			Env:     env,
		})
	}

	if plan.Native.IsNull() {
		return in, diags
	}

	decoded, err := dynamic.ToJSON(plan.Native)
	if err != nil {
		diags.AddAttributeError(path.Root("native"), "Native settings not known", err.Error())

		return in, diags
	}

	native, ok := decoded.(map[string]any)
	if !ok {
		diags.AddAttributeError(path.Root("native"), "Native settings are not an object", "Write each harness's settings under its name, as in { claude-code = { includeCoAuthoredBy = false } }.")

		return in, diags
	}

	in.Native = native

	return in, diags
}

func harnessProfileModel(profile client.HarnessProfile, prior *HarnessProfileResourceModel) HarnessProfileResourceModel {
	model := HarnessProfileResourceModel{
		ID:      types.StringValue(profile.ID),
		Version: types.Int64Value(profile.Version),
		Address: types.StringValue(profile.Address),
		TeamID:  types.StringValue(profile.TeamID),
		Key:     types.StringValue(profile.Key),
		Permissions: harnessProfilePermissions{
			Allow: stringList(profile.Permissions.Allow),
			Ask:   stringList(profile.Permissions.Ask),
			Deny:  stringList(profile.Permissions.Deny),
		},
		MCPServers: make([]harnessProfileMCPServer, 0, len(profile.MCPServers)),
		Native:     dynamic.FromJSON(profile.Native),
	}

	for _, each := range profile.MCPServers {
		env := map[string]attr.Value{}
		for name, value := range each.Env {
			env[name] = types.StringValue(value)
		}

		model.MCPServers = append(model.MCPServers, harnessProfileMCPServer{
			Name:    types.StringValue(each.Name),
			Command: types.StringValue(each.Command),
			Args:    stringList(each.Args),
			Env:     types.MapValueMust(types.StringType, env),
		})
	}

	if prior != nil {
		model.Key = prior.Key
		model.Permissions = prior.Permissions
		model.MCPServers = prior.MCPServers
		model.Native = dynamic.Resolve(prior.Native, profile.Native)
	}

	return model
}
