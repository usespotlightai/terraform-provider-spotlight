package provider

import (
	"context"
	"encoding/json"
	"maps"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/client"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/dynamic"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/expressions"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/fieldpath"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/writeonly"
)

var (
	_ resource.Resource                   = &HookResource{}
	_ resource.ResourceWithConfigure      = &HookResource{}
	_ resource.ResourceWithImportState    = &HookResource{}
	_ resource.ResourceWithIdentity       = &HookResource{}
	_ resource.ResourceWithValidateConfig = &HookResource{}
)

// NewHookResource returns the spotlight_hook resource.
func NewHookResource() resource.Resource {
	return &HookResource{}
}

// HookResource is the spotlight_hook resource.
type HookResource struct {
	client      *client.Client
	expressions *expressions.Checker
}

const (
	hookKind     = "hook"
	hookFailure  = "continue"
	hookTimeout  = 10
	hookSteps    = "steps"
	hookWhen     = "when"
	hookSigning  = "signing"
	hookOIDC     = "oidc"
	hookSentFrom = "spotlight"
)

var hookEvents = []string{
	"tool.using", "tool.used", "run.starting", "run.ended", "handover.submitting",
	"pull-request.opening", "pull-request.merged", "task.opened", "task.moved",
}

// HookResourceModel is a spotlight_hook as Terraform stores it.
type HookResourceModel struct {
	ID             types.String  `tfsdk:"id"`
	Version        types.Int64   `tfsdk:"version"`
	Address        types.String  `tfsdk:"address"`
	TeamID         types.String  `tfsdk:"team_id"`
	Key            types.String  `tfsdk:"key"`
	On             types.String  `tfsdk:"on"`
	Match          types.Object  `tfsdk:"match"`
	When           types.String  `tfsdk:"when"`
	Vars           types.Map     `tfsdk:"vars"`
	Steps          types.Dynamic `tfsdk:"steps"`
	Signing        types.Object  `tfsdk:"signing"`
	Failure        types.String  `tfsdk:"failure"`
	TimeoutSeconds types.Int64   `tfsdk:"timeout_seconds"`
}

var (
	hookFields = fieldpath.Locator{Schema: hookSchema().Type(), Renames: map[string]string{
		"secret":        writeonly.SigningSecret.Name,
		"secretVersion": writeonly.SigningSecret.Version,
	}}
	hookMatchTypes   = attributeTypesAt(hookSchema().Type(), "match")
	hookSigningTypes = attributeTypesAt(hookSchema().Type(), hookSigning)
	hookOIDCTypes    = attributeTypesAt(hookSchema().Type(), hookSigning, hookOIDC)
)

func attributeTypesAt(attrType attr.Type, names ...string) map[string]attr.Type {
	for _, name := range names {
		object, _ := attrType.(types.ObjectType)
		attrType = object.AttrTypes[name]
	}

	object, _ := attrType.(types.ObjectType)

	return object.AttrTypes
}

func (r *HookResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_hook"
}

func (r *HookResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = hookSchema()
}

func hookSchema() schema.Schema {
	signing := writeonly.SigningSecret.Attributes("The HMAC secret Spotlight signs each request with, as Standard Webhooks sign: `whsec_` followed by 24 to 64 base64-encoded random bytes, " +
		"as `\"whsec_${base64encode(random_password.hook.result)}\"` makes. Needed with `secret_wo_version`, and sent again whenever it changes.")
	signing[hookOIDC] = schema.SingleNestedAttribute{
		Optional:            true,
		MarkdownDescription: "Signs each request with a short-lived OIDC token Spotlight issues, instead of a secret.",
		Attributes: map[string]schema.Attribute{
			"audience": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Who the token is for, as the endpoint checks it, as in `https://audit.acme.dev`.",
			},
		},
	}

	return schema.Schema{
		MarkdownDescription: "A hook a team has created: steps Spotlight takes whenever an event it fires `on` matches, each calling a tool an integration offers or sending a request. " +
			"Write each expression in a step as `$${{ ... }}`, and `when` bare; `terraform plan` type-checks every one where it is written. " +
			"Import it as `<workspace id>/<team key>/<key>`.",
		Attributes: map[string]schema.Attribute{
			"id":      idAttribute("hook"),
			"version": versionAttribute("hook"),
			"address": addressAttribute("core/migrations-need-a-dba"),
			"team_id": ownerAttribute("team", "hook"),
			"key":     fixedAttribute("Names the hook within its team, as in `migrations-need-a-dba`.", "hook"),
			"on": schema.StringAttribute{
				Required:            true,
				Validators:          []validator.String{stringvalidator.OneOf(hookEvents...)},
				MarkdownDescription: "The event the hook fires on: `" + strings.Join(hookEvents, "`, `") + "`. The `.using`, `.starting`, `.submitting` and `.opening` events fire before what they name, so a hook on one may stop it.",
			},
			"match": schema.SingleNestedAttribute{
				Optional:            true,
				MarkdownDescription: "Narrows the events the hook fires on; each list left out or empty narrows nothing. Left out, the hook fires on every event it is `on`.",
				Attributes: map[string]schema.Attribute{
					"tools":        stringSetAttribute("The tools the hook fires for, as `integration.tool`, as in `spotlight_integration.grafana.tools[\"search\"]`; only on `tool.using` and `tool.used`."),
					"lines":        stringSetAttribute("The lines whose tasks the hook fires for, by key, as in `delivery`."),
					"stations":     stringSetAttribute("The stations whose tasks the hook fires for, by key, as in `implementation`."),
					"repositories": stringSetAttribute("The repositories the hook fires for, as `owner/name`."),
					"paths":        stringSetAttribute("Globs of the changed paths the hook fires for, as in `db/migrations/**`; only on `pull-request.opening` and `pull-request.merged`."),
				},
			},
			hookWhen: schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "A CEL condition the event must meet besides `match`, written bare, as in `event.inputs.query != ''`. It reads `event`, `task`, `run`, `vars` and `hook`.",
			},
			"vars": schema.MapAttribute{
				Optional:            true,
				Computed:            true,
				ElementType:         types.StringType,
				Default:             mapdefault.StaticValue(stringMap(nil)),
				MarkdownDescription: "Values the hook's expressions read as `vars.<name>`, as in `{ migrations = \"db/migrations/\" }`.",
			},
			hookSteps: schema.DynamicAttribute{
				Required: true,
				MarkdownDescription: "What the hook does, in order: a list of at least one step, each an object that either calls a tool or sends a request. " +
					"Every step has a `name`, lowercase words joined by dashes, by which later steps read its outcome as `step.<name>`. " +
					"A step that calls a tool names it in `call`, as in `spotlight_integration.slack.tools[\"post-message\"]`, and sets its inputs in `with`, an object of any JSON. " +
					"A step that sends a request sets `request`: its `url`, any `headers` beside the ones Spotlight signs with, its `body`, which is any JSON and, left out, " +
					"is the event, task and run, and `from`: `spotlight`, the default, to send it from Spotlight, to a host the control plane's `OUTBOUND_ALLOWED_HOSTS` allows, " +
					"or `network`, to have a host of the workspace send it from inside its network. A hook that sends a request signs it, so it needs `signing`. " +
					"Strings in `with`, and a request's `url`, `headers` and `body`, may contain `$${{ ... }}` expressions, which read what `when` does and `step`.",
			},
			hookSigning: schema.SingleNestedAttribute{
				Optional:            true,
				MarkdownDescription: "How the hook signs the requests its steps send: with the HMAC secret `secret_wo`, as Standard Webhooks sign, or with an OIDC token for an `oidc` audience. Needed by a hook with a step that sends a request; a hook whose steps only call tools leaves it out.",
				Attributes:          signing,
			},
			"failure": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString(hookFailure),
				Validators:          []validator.String{stringvalidator.OneOf(hookFailure, "block")},
				MarkdownDescription: "What a failed step does to the event: `continue` lets it go on; `block` stops it, which only an event that fires before what it names can be.",
			},
			"timeout_seconds": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(hookTimeout),
				Validators:          []validator.Int64{int64validator.Between(1, 30)},
				MarkdownDescription: "How many seconds each request may take, and a hook that fires before an event as a whole, from 1 to 30.",
			},
		},
	}
}

func (r *HookResource) IdentitySchema(_ context.Context, _ resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = idIdentitySchema("hook")
}

func (r *HookResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	data := providerDataOf(req.ProviderData, &resp.Diagnostics)
	if data == nil {
		return
	}

	r.client = data.client
	r.expressions = data.expressions
}

// ValidateConfig checks how the hook signs, that its steps are a list, and
// that a hook that sends requests signs them. When the provider is
// configured, as during a plan, it also type-checks the expressions.
func (r *HookResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config HookResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(hookSigningProblems(config.Signing)...)
	resp.Diagnostics.Append(hookStepsProblems(config.Steps, config.Signing)...)

	if r.expressions == nil || resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(r.expressions.CheckSubject(ctx, hookSubject(config))...)
}

func (r *HookResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan HookResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	in, diags := hookInput(plan)
	resp.Diagnostics.Append(diags...)
	resp.Diagnostics.Append(sendHookSecret(ctx, req.Config, req.Plan, nil, &in)...)
	if resp.Diagnostics.HasError() {
		return
	}

	hook, err := r.client.CreateHook(ctx, plan.TeamID.ValueString(), in)
	if err != nil {
		addAPIError(&resp.Diagnostics, "Could not create the hook", err, hookFields, client.ScopeConfigure)
		return
	}

	addAPIWarnings(&resp.Diagnostics, hookKind, hook.Warnings, hookFields)
	resp.Diagnostics.Append(resp.State.Set(ctx, hookModel(hook, &plan))...)
	resp.Diagnostics.Append(setIdentity(ctx, resp.Identity, hook.ID)...)
	resp.Diagnostics.Append(rememberAnswer(ctx, resp.Private, hookSettable(hook))...)
}

func (r *HookResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state HookResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	hook, err := r.client.ReadHook(ctx, state.ID.ValueString())
	if client.IsNotFound(err) {
		removed(ctx, resp, state.ID.ValueString())
		return
	}
	if err != nil {
		addAPIError(&resp.Diagnostics, "Could not read the hook", err, hookFields, client.ScopeRead)
		return
	}

	prior := &state
	if !answerUnchanged(ctx, req.Private, hookSettable(hook)) {
		prior = nil
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, hookModel(hook, prior))...)
	resp.Diagnostics.Append(setIdentity(ctx, resp.Identity, hook.ID)...)
	resp.Diagnostics.Append(rememberAnswer(ctx, resp.Private, hookSettable(hook))...)
}

func (r *HookResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state HookResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	in, diags := hookInput(plan)
	resp.Diagnostics.Append(diags...)
	resp.Diagnostics.Append(sendHookSecret(ctx, req.Config, req.Plan, &req.State, &in)...)
	if resp.Diagnostics.HasError() {
		return
	}

	hook, err := r.client.UpdateHook(ctx, state.ID.ValueString(), state.Version.ValueInt64(), in)
	if err != nil {
		addAPIError(&resp.Diagnostics, "Could not update the hook", err, hookFields, client.ScopeConfigure)
		return
	}

	addAPIWarnings(&resp.Diagnostics, hookKind, hook.Warnings, hookFields)
	resp.Diagnostics.Append(resp.State.Set(ctx, hookModel(hook, &plan))...)
	resp.Diagnostics.Append(setIdentity(ctx, resp.Identity, hook.ID)...)
	resp.Diagnostics.Append(rememberAnswer(ctx, resp.Private, hookSettable(hook))...)
}

func (r *HookResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state HookResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteHook(ctx, state.ID.ValueString(), state.Version.ValueInt64())
	if err != nil && !client.IsNotFound(err) {
		addAPIError(&resp.Diagnostics, "Could not delete the hook", err, hookFields, client.ScopeConfigure)
	}
}

func (r *HookResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	target, ok := importTargetOf(ctx, req, &resp.Diagnostics, "<workspace id>/core/migrations-need-a-dba")
	if !ok {
		return
	}

	var hook client.Hook
	var err error
	if target.byID() {
		hook, err = r.client.ReadHook(ctx, target.id)
	} else {
		hook, err = r.client.ImportHook(ctx, target.workspaceID, target.address)
	}
	if err != nil {
		addAPIError(&resp.Diagnostics, "Could not import the hook at "+target.String(), err, fieldpath.Locator{}, client.ScopeRead)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, hookModel(hook, nil))...)
	resp.Diagnostics.Append(setIdentity(ctx, resp.Identity, hook.ID)...)
	resp.Diagnostics.Append(rememberAnswer(ctx, resp.Private, hookSettable(hook))...)
}

func hookSettable(hook client.Hook) any {
	return hook.HookInput
}

func hookSigningProblems(signing types.Object) diag.Diagnostics {
	var diags diag.Diagnostics

	if signing.IsNull() || signing.IsUnknown() {
		return diags
	}

	attributes := signing.Attributes()
	oidc, secret, version := attributes[hookOIDC], attributes[writeonly.SigningSecret.Name], attributes[writeonly.SigningSecret.Version]

	switch {
	case !oidc.IsNull() && (!secret.IsNull() || !version.IsNull()):
		diags.AddAttributeError(path.Root(hookSigning), "A hook signs one way",
			"signing: signs with an HMAC secret or with an OIDC token, not both. Set secret_wo and secret_wo_version, or oidc.")
	case oidc.IsNull() && version.IsNull():
		diags.AddAttributeError(path.Root(hookSigning).AtName(writeonly.SigningSecret.Version), "A signing secret needs its version",
			"signing.secret_wo_version: says when to send secret_wo again, since Terraform never stores it; set it to 1, and change it whenever the secret changes. Or sign with oidc instead.")
	}

	return diags
}

func hookStepsProblems(steps types.Dynamic, signing types.Object) diag.Diagnostics {
	var diags diag.Diagnostics

	if steps.IsNull() || steps.IsUnknown() || steps.IsUnderlyingValueUnknown() {
		return diags
	}

	switch steps.UnderlyingValue().(type) {
	case basetypes.TupleValue, basetypes.ListValue:
	default:
		diags.AddAttributeError(path.Root(hookSteps), "Steps are a list",
			"steps: is the steps the hook takes, in order, as in [{ name = \"notify\", call = \"slack.post-message\", with = { text = \"...\" } }].")

		return diags
	}

	decoded, _ := dynamic.ToJSONKnown(steps)
	listed, _ := decoded.([]any)
	for _, each := range stepsWithoutNulls(listed) {
		step, _ := each.(map[string]any)
		if _, requests := step["request"]; requests && signing.IsNull() {
			diags.AddAttributeError(path.Root(hookSigning), "A hook that sends requests signs them",
				"signing: is missing; a hook with a step that sends a request signs it, with secret_wo and secret_wo_version, or with oidc.")

			break
		}
	}

	return diags
}

func hookSubject(config HookResourceModel) expressions.Subject {
	in, complete := hookDefinition(config)

	subject := expressions.Subject{
		Resource:   hookKind,
		Fields:     hookFields,
		Definition: definitionOf(in),
		Complete:   complete,
	}

	if !config.When.IsNull() && !config.When.IsUnknown() {
		subject.Written = append(subject.Written, expressions.Written{
			At:          fieldpath.Path{fieldpath.Key(hookWhen)},
			Expressions: []string{strings.TrimSpace(config.When.ValueString())},
		})
	}

	subject.Written = append(subject.Written, expressions.TemplatesIn(config.Steps, fieldpath.Path{fieldpath.Key(hookSteps)})...)

	return subject
}

func definitionOf(in client.HookInput) client.Definition {
	raw, err := json.Marshal(in)
	if err != nil {
		return client.Definition{}
	}

	var definition client.Definition
	if err := json.Unmarshal(raw, &definition); err != nil {
		return client.Definition{}
	}

	return definition
}

func hookInput(plan HookResourceModel) (client.HookInput, diag.Diagnostics) {
	var diags diag.Diagnostics

	in, complete := hookDefinition(plan)
	if !complete {
		diags.AddError("Hook not known", "Part of the hook is not known yet, though Terraform applies a hook only once it is; this is a bug in the provider.")
	}

	return in, diags
}

// knownOnly reads the known values of a model. It records if it left out a
// value.
type knownOnly struct {
	left bool
}

func (k *knownOnly) json(input attr.Value) any {
	value, complete := dynamic.ToJSONKnown(input)
	k.left = k.left || !complete

	return value
}

func (k *knownOnly) text(input types.String) string {
	k.left = k.left || input.IsUnknown()

	return input.ValueString()
}

func (k *knownOnly) texts(input attr.Value) []string {
	listed, _ := k.json(input).([]any)

	out := []string{}
	for _, each := range listed {
		if text, ok := each.(string); ok {
			out = append(out, text)
		}
	}

	return out
}

// hookDefinition writes the hook in a model in the API format, without the
// values that are not known yet. The second result is true if it left out
// no value.
func hookDefinition(model HookResourceModel) (client.HookInput, bool) {
	known := &knownOnly{}

	in := client.HookInput{
		Key:            known.text(model.Key),
		On:             known.text(model.On),
		Failure:        known.text(model.Failure),
		TimeoutSeconds: model.TimeoutSeconds.ValueInt64(),
		Vars:           map[string]string{},
		Steps:          []any{},
	}
	known.left = known.left || model.TimeoutSeconds.IsUnknown()

	if !model.When.IsNull() {
		when := known.text(model.When)
		in.When = &when
	}

	vars, _ := known.json(model.Vars).(map[string]any)
	for name, value := range vars {
		if text, ok := value.(string); ok {
			in.Vars[name] = text
		}
	}

	if steps, ok := known.json(model.Steps).([]any); ok {
		in.Steps = stepsWithoutNulls(steps)
	}

	if !model.Match.IsNull() && !model.Match.IsUnknown() {
		matchAttributes := model.Match.Attributes()
		in.Match = client.HookMatch{
			Tools:        known.texts(matchAttributes["tools"]),
			Lines:        known.texts(matchAttributes["lines"]),
			Stations:     known.texts(matchAttributes["stations"]),
			Repositories: known.texts(matchAttributes["repositories"]),
			Paths:        known.texts(matchAttributes["paths"]),
		}
	}
	known.left = known.left || model.Match.IsUnknown()

	in.Signing = hookSigningOf(model.Signing, known)

	return in, !known.left
}

func hookSigningOf(signing types.Object, known *knownOnly) *client.HookSigning {
	if signing.IsNull() || signing.IsUnknown() {
		known.left = known.left || signing.IsUnknown()

		return nil
	}

	attributes := signing.Attributes()
	out := &client.HookSigning{}

	if version, ok := attributes[writeonly.SigningSecret.Version].(types.Int64); ok {
		known.left = known.left || version.IsUnknown()
		out.SecretVersion = version.ValueInt64Pointer()
	}

	if oidc, ok := attributes[hookOIDC].(types.Object); ok && !oidc.IsNull() {
		audience, _ := oidc.Attributes()["audience"].(types.String)
		out.OIDC = &client.HookOIDC{Audience: known.text(audience)}
		known.left = known.left || oidc.IsUnknown()
	}

	return out
}

func sendHookSecret(ctx context.Context, config tfsdk.Config, plan tfsdk.Plan, state *tfsdk.State, in *client.HookInput) diag.Diagnostics {
	field := writeonly.SigningSecret

	var diags diag.Diagnostics

	if in.Signing == nil || in.Signing.OIDC != nil {
		return diags
	}

	if state != nil {
		rewritten, more := field.Rewritten(ctx, plan, *state)
		diags.Append(more...)

		if !rewritten {
			return diags
		}
	}

	value, more := field.Read(ctx, config)
	diags.Append(more...)

	if value.IsNull() || value.IsUnknown() {
		return diags
	}

	in.Signing.Secret = value.ValueString()

	return diags
}

func hookModel(hook client.Hook, prior *HookResourceModel) HookResourceModel {
	model := HookResourceModel{
		ID:             types.StringValue(hook.ID),
		Version:        types.Int64Value(hook.Version),
		Address:        types.StringValue(hook.Address),
		TeamID:         types.StringValue(hook.TeamID),
		Key:            types.StringValue(hook.Key),
		On:             types.StringValue(hook.On),
		Match:          hookMatchValue(hook.Match),
		When:           types.StringPointerValue(hook.When),
		Vars:           stringMap(hook.Vars),
		Steps:          dynamic.FromJSON(stepsAsWritten(hook.Steps)),
		Signing:        hookSigningValue(hook.Signing),
		Failure:        types.StringValue(hook.Failure),
		TimeoutSeconds: types.Int64Value(hook.TimeoutSeconds),
	}

	if prior != nil {
		model.Key = prior.Key
		model.On = prior.On
		model.Match = prior.Match
		model.When = prior.When
		model.Vars = prior.Vars
		model.Steps = dynamic.Resolve(prior.Steps, hook.Steps)
		model.Signing = prior.Signing
		model.Failure = prior.Failure
		model.TimeoutSeconds = prior.TimeoutSeconds
	}

	return model
}

func hookMatchValue(match client.HookMatch) types.Object {
	sets := map[string][]string{
		"tools":        match.Tools,
		"lines":        match.Lines,
		"stations":     match.Stations,
		"repositories": match.Repositories,
		"paths":        match.Paths,
	}

	narrows := false
	values := make(map[string]attr.Value, len(sets))
	for name, each := range sets {
		narrows = narrows || len(each) > 0
		values[name] = stringSet(each)
	}

	if !narrows {
		return types.ObjectNull(hookMatchTypes)
	}

	return types.ObjectValueMust(hookMatchTypes, values)
}

func hookSigningValue(signing *client.HookSigning) types.Object {
	if signing == nil {
		return types.ObjectNull(hookSigningTypes)
	}

	oidc := types.ObjectNull(hookOIDCTypes)
	if signing.OIDC != nil {
		oidc = types.ObjectValueMust(hookOIDCTypes, map[string]attr.Value{"audience": types.StringValue(signing.OIDC.Audience)})
	}

	return types.ObjectValueMust(hookSigningTypes, map[string]attr.Value{
		writeonly.SigningSecret.Name:    types.StringNull(),
		writeonly.SigningSecret.Version: types.Int64PointerValue(signing.SecretVersion),
		hookOIDC:                        oidc,
	})
}

// stepsWithoutNulls removes the null values from each step and from its
// request. In a list of objects, each step contains the attributes that it does
// not set as null.
func stepsWithoutNulls(steps []any) []any {
	out := make([]any, 0, len(steps))

	for _, each := range steps {
		step, ok := each.(map[string]any)
		if !ok {
			out = append(out, each)
			continue
		}

		step = withoutNulls(step)
		if request, ok := step["request"].(map[string]any); ok {
			step["request"] = withoutNulls(request)
		}

		out = append(out, step)
	}

	return out
}

func withoutNulls(object map[string]any) map[string]any {
	out := make(map[string]any, len(object))
	for name, value := range object {
		if value != nil {
			out[name] = value
		}
	}

	return out
}

// stepsAsWritten removes from each step the default values that Spotlight
// adds. Then an imported hook reads the same as its configuration.
func stepsAsWritten(steps []any) []any {
	out := make([]any, 0, len(steps))

	for _, each := range steps {
		step, ok := each.(map[string]any)
		if !ok {
			out = append(out, each)
			continue
		}

		step = maps.Clone(step)
		if with, ok := step["with"].(map[string]any); ok && len(with) == 0 {
			delete(step, "with")
		}

		if request, ok := step["request"].(map[string]any); ok {
			request = maps.Clone(request)
			if headers, ok := request["headers"].(map[string]any); ok && len(headers) == 0 {
				delete(request, "headers")
			}
			if request["from"] == hookSentFrom {
				delete(request, "from")
			}
			step["request"] = request
		}

		out = append(out, step)
	}

	return out
}
