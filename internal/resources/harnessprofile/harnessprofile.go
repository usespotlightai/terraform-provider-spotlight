// Package harnessprofile is the spotlight_harness_profile resource.
package harnessprofile

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

	"github.com/usespotlightai/terraform-provider-spotlight/internal/dynamic"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/resourcekit"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/spotlight"
)

const kind = "harness profile"

type model struct {
	ID          types.String  `tfsdk:"id"`
	Version     types.Int64   `tfsdk:"version"`
	Address     types.String  `tfsdk:"address"`
	TeamID      types.String  `tfsdk:"team_id"`
	Key         types.String  `tfsdk:"key"`
	Permissions permissions   `tfsdk:"permissions"`
	MCPServers  []mcpServer   `tfsdk:"mcp_servers"`
	Native      types.Dynamic `tfsdk:"native"`
}

type permissions struct {
	Allow types.List `tfsdk:"allow"`
	Ask   types.List `tfsdk:"ask"`
	Deny  types.List `tfsdk:"deny"`
}

type mcpServer struct {
	Name    types.String `tfsdk:"name"`
	Command types.String `tfsdk:"command"`
	Args    types.List   `tfsdk:"args"`
	Env     types.Map    `tfsdk:"env"`
}

var (
	rules          = types.ListType{ElemType: types.StringType}
	permissionType = map[string]attr.Type{"allow": rules, "ask": rules, "deny": rules}
	serverType     = types.ObjectType{AttrTypes: map[string]attr.Type{
		"name":    types.StringType,
		"command": types.StringType,
		"args":    rules,
		"env":     types.MapType{ElemType: types.StringType},
	}}
	noNative = types.DynamicValue(types.ObjectValueMust(map[string]attr.Type{}, map[string]attr.Value{}))
)

// New returns the spotlight_harness_profile resource.
func New() resource.Resource {
	return resourcekit.NewManaged(resourcekit.Spec[spotlight.HarnessProfileInput, spotlight.HarnessProfile, model]{
		Kind:        "harness_profile",
		Schema:      schemaOf(),
		AddressedAs: "core/careful",
		Resources: func(api spotlight.API) spotlight.Resources[spotlight.HarnessProfileInput, spotlight.HarnessProfile] {
			return api.HarnessProfiles
		},
		Owner:    func(m model) string { return m.TeamID.ValueString() },
		Input:    inputOf,
		Stored:   stored,
		Meta:     func(p spotlight.HarnessProfile) spotlight.Meta { return p.Meta },
		Settable: func(p spotlight.HarnessProfile) any { return p.HarnessProfileInput },
		Identity: func(m model) (string, int64) { return m.ID.ValueString(), m.Version.ValueInt64() },
	})
}

func rulesAttribute(description string) schema.ListAttribute {
	return resourcekit.StringListAttribute(description + " Each is `read`, `edit` or `execute`, alone or with a pattern, as in `execute(npm test *)` or `read(.env)`.")
}

func schemaOf() schema.Schema {
	return schema.Schema{
		MarkdownDescription: "A team's harness settings, which a station names in `profile`: permissions every harness is told in its own terms, MCP servers, and each harness's own settings passed through as it reads them. " +
			"Where a harness cannot say a rule, the answer's warnings name the approximation. Import it as `<workspace id>/<team key>/<key>`.",
		Attributes: map[string]schema.Attribute{
			"id":      resourcekit.IDAttribute(kind),
			"version": resourcekit.VersionAttribute(kind),
			"address": resourcekit.AddressAttribute("core/careful"),
			"team_id": resourcekit.OwnerAttribute("team", kind),
			"key":     resourcekit.FixedAttribute("What stations name the profile by, as in `careful`.", kind),
			"permissions": schema.SingleNestedAttribute{
				Optional: true,
				Computed: true,
				Default: objectdefault.StaticValue(types.ObjectValueMust(permissionType, map[string]attr.Value{
					"allow": resourcekit.StringList(nil),
					"ask":   resourcekit.StringList(nil),
					"deny":  resourcekit.StringList(nil),
				})),
				MarkdownDescription: "What the harness may do, what it asks a person about first, and what it may never do.",
				Attributes: map[string]schema.Attribute{
					"allow": rulesAttribute("What the harness may do without asking."),
					"ask":   rulesAttribute("What the harness asks a person about first; the ask reaches their inbox as an approval."),
					"deny":  rulesAttribute("What the harness may never do."),
				},
			},
			"mcp_servers": schema.ListNestedAttribute{
				Optional:            true,
				Computed:            true,
				Default:             listdefault.StaticValue(types.ListValueMust(serverType, []attr.Value{})),
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
						"args": resourcekit.StringListAttribute("What the command is given."),
						"env": schema.MapAttribute{
							Optional:            true,
							Computed:            true,
							ElementType:         types.StringType,
							Default:             mapdefault.StaticValue(types.MapValueMust(types.StringType, map[string]attr.Value{})),
							MarkdownDescription: "The server's environment, never credentials: these values are stored and shown in plain text, in Spotlight and in the Terraform state. A server that needs a key reads it from the runner's own environment.",
						},
					},
				},
			},
			"native": schema.DynamicAttribute{
				Optional:            true,
				Computed:            true,
				Default:             dynamicdefault.StaticValue(noNative),
				MarkdownDescription: "Each harness's own settings by harness, passed through as it reads them: Claude Code settings as an object, Codex `config.toml` as text, Cursor CLI config as an object; as in `{ claude-code = { includeCoAuthoredBy = false } }`.",
			},
		},
	}
}

func inputOf(plan model) (spotlight.HarnessProfileInput, diag.Diagnostics) {
	var diags diag.Diagnostics

	in := spotlight.HarnessProfileInput{
		Key: plan.Key.ValueString(),
		Permissions: spotlight.Permissions{
			Allow: resourcekit.StringsIn(plan.Permissions.Allow, &diags),
			Ask:   resourcekit.StringsIn(plan.Permissions.Ask, &diags),
			Deny:  resourcekit.StringsIn(plan.Permissions.Deny, &diags),
		},
		MCPServers: make([]spotlight.MCPServer, 0, len(plan.MCPServers)),
		Native:     map[string]any{},
	}

	for _, each := range plan.MCPServers {
		env := map[string]string{}
		if !each.Env.IsNull() && !each.Env.IsUnknown() {
			diags.Append(each.Env.ElementsAs(context.Background(), &env, false)...)
		}

		in.MCPServers = append(in.MCPServers, spotlight.MCPServer{
			Name:    each.Name.ValueString(),
			Command: each.Command.ValueString(),
			Args:    resourcekit.StringsIn(each.Args, &diags),
			Env:     env,
		})
	}

	if plan.Native.IsNull() {
		return in, diags
	}

	held, err := dynamic.ToJSON(plan.Native)
	if err != nil {
		diags.AddAttributeError(path.Root("native"), "Native settings not known", err.Error())

		return in, diags
	}

	native, ok := held.(map[string]any)
	if !ok {
		diags.AddAttributeError(path.Root("native"), "Native settings are not an object", "Write each harness's settings under its name, as in { claude-code = { includeCoAuthoredBy = false } }.")

		return in, diags
	}

	in.Native = native

	return in, diags
}

func stored(answer spotlight.HarnessProfile, kept *model) (model, diag.Diagnostics) {
	held := model{
		ID:      types.StringValue(answer.ID),
		Version: types.Int64Value(answer.Version),
		Address: types.StringValue(answer.Address),
		TeamID:  types.StringValue(answer.TeamID),
		Key:     types.StringValue(answer.Key),
		Permissions: permissions{
			Allow: resourcekit.StringList(answer.Permissions.Allow),
			Ask:   resourcekit.StringList(answer.Permissions.Ask),
			Deny:  resourcekit.StringList(answer.Permissions.Deny),
		},
		MCPServers: make([]mcpServer, 0, len(answer.MCPServers)),
		Native:     dynamic.FromJSON(answer.Native),
	}

	for _, each := range answer.MCPServers {
		env := map[string]attr.Value{}
		for name, value := range each.Env {
			env[name] = types.StringValue(value)
		}

		held.MCPServers = append(held.MCPServers, mcpServer{
			Name:    types.StringValue(each.Name),
			Command: types.StringValue(each.Command),
			Args:    resourcekit.StringList(each.Args),
			Env:     types.MapValueMust(types.StringType, env),
		})
	}

	if kept != nil {
		held.Key = kept.Key
		held.Permissions = kept.Permissions
		held.MCPServers = kept.MCPServers
		held.Native = dynamic.Resolve(kept.Native, answer.Native)
	}

	return held, nil
}
