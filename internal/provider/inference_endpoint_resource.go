package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/client"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/fieldpath"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/writeonly"
)

var (
	_ resource.Resource                = &InferenceEndpointResource{}
	_ resource.ResourceWithConfigure   = &InferenceEndpointResource{}
	_ resource.ResourceWithImportState = &InferenceEndpointResource{}
	_ resource.ResourceWithIdentity    = &InferenceEndpointResource{}
)

func NewInferenceEndpointResource() resource.Resource {
	return &InferenceEndpointResource{}
}

type InferenceEndpointResource struct {
	client *client.Client
}

type InferenceEndpointResourceModel struct {
	ID                      types.String `tfsdk:"id"`
	Version                 types.Int64  `tfsdk:"version"`
	Address                 types.String `tfsdk:"address"`
	WorkspaceID             types.String `tfsdk:"workspace_id"`
	BaseURL                 types.String `tfsdk:"base_url"`
	Model                   types.String `tfsdk:"model"`
	SupportsReasoningEffort types.Bool   `tfsdk:"supports_reasoning_effort"`
	APIKey                  types.String `tfsdk:"api_key_wo"`
	APIKeyVersion           types.Int64  `tfsdk:"api_key_wo_version"`
	APIKeySet               types.Bool   `tfsdk:"api_key_set"`
}

var inferenceEndpointFields = fieldpath.Locator{Schema: inferenceEndpointSchema().Type()}

func (r *InferenceEndpointResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_inference_endpoint"
}

func (r *InferenceEndpointResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = inferenceEndpointSchema()
}

func inferenceEndpointSchema() schema.Schema {
	attributes := writeonly.InferenceAPIKey.Attributes("The API key that Spotlight sends to the endpoint. Spotlight stores it and never returns it. " +
		"Leave it out to keep the stored key. To remove the key, replace the endpoint.")
	attributes["id"] = idAttribute("inference endpoint")
	attributes["version"] = versionAttribute("inference endpoint")
	attributes["address"] = addressAttribute("acme")
	attributes["workspace_id"] = ownerAttribute("workspace", "inference endpoint")
	attributes["base_url"] = schema.StringAttribute{
		Required:            true,
		MarkdownDescription: "The base URL of an OpenAI-compatible API, as in `https://api.openai.com/v1`. Spotlight calls its `/chat/completions`.",
	}
	attributes["model"] = schema.StringAttribute{
		Required:            true,
		MarkdownDescription: "The one model that every Spotlight feature of the workspace uses.",
	}
	attributes["supports_reasoning_effort"] = schema.BoolAttribute{
		Optional:            true,
		Computed:            true,
		Default:             booldefault.StaticBool(false),
		MarkdownDescription: "Whether the model accepts the `reasoning_effort` parameter. The default is `false`.",
	}
	attributes["api_key_set"] = schema.BoolAttribute{
		Computed:            true,
		MarkdownDescription: "Whether Spotlight holds an API key for the endpoint.",
	}

	return schema.Schema{
		MarkdownDescription: "The OpenAI-compatible inference endpoint and the model that a workspace uses for its Spotlight features. A workspace has one. " +
			"The API key is write-only, so it never enters the plan or the state. It needs Terraform 1.11 or later. " +
			"Writing the endpoint needs a token granted the `secrets` scope. Import it as `<workspace id>/<workspace slug>`.",
		Attributes: attributes,
	}
}

func (r *InferenceEndpointResource) IdentitySchema(_ context.Context, _ resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = idIdentitySchema("inference endpoint")
}

func (r *InferenceEndpointResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientOf(req.ProviderData, &resp.Diagnostics)
}

func (r *InferenceEndpointResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan InferenceEndpointResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	in := inferenceEndpointInput(plan)
	resp.Diagnostics.Append(sendInferenceAPIKey(ctx, req.Config, req.Plan, nil, &in)...)
	if resp.Diagnostics.HasError() {
		return
	}

	endpoint, err := r.client.CreateInferenceEndpoint(ctx, plan.WorkspaceID.ValueString(), in)
	if err != nil {
		addAPIError(&resp.Diagnostics, "Could not create the inference endpoint", err, inferenceEndpointFields, client.ScopeSecrets)
		return
	}

	addAPIWarnings(&resp.Diagnostics, "inference endpoint", endpoint.Warnings, inferenceEndpointFields)
	resp.Diagnostics.Append(resp.State.Set(ctx, inferenceEndpointModel(endpoint, &plan))...)
	resp.Diagnostics.Append(setIdentity(ctx, resp.Identity, endpoint.ID)...)
	resp.Diagnostics.Append(rememberAnswer(ctx, resp.Private, inferenceEndpointSettable(endpoint))...)
}

func (r *InferenceEndpointResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state InferenceEndpointResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	endpoint, err := r.client.ReadInferenceEndpoint(ctx, state.ID.ValueString())
	if client.IsNotFound(err) {
		removed(ctx, resp, state.ID.ValueString())
		return
	}
	if err != nil {
		addAPIError(&resp.Diagnostics, "Could not read the inference endpoint", err, inferenceEndpointFields, client.ScopeRead)
		return
	}

	addAPIWarnings(&resp.Diagnostics, "inference endpoint", endpoint.Warnings, inferenceEndpointFields)

	prior := &state
	if !answerUnchanged(ctx, req.Private, inferenceEndpointSettable(endpoint)) {
		prior = nil
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, inferenceEndpointModel(endpoint, prior))...)
	resp.Diagnostics.Append(setIdentity(ctx, resp.Identity, endpoint.ID)...)
	resp.Diagnostics.Append(rememberAnswer(ctx, resp.Private, inferenceEndpointSettable(endpoint))...)
}

func (r *InferenceEndpointResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state InferenceEndpointResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	in := inferenceEndpointInput(plan)
	resp.Diagnostics.Append(sendInferenceAPIKey(ctx, req.Config, req.Plan, &req.State, &in)...)
	if resp.Diagnostics.HasError() {
		return
	}

	endpoint, err := r.client.UpdateInferenceEndpoint(ctx, state.ID.ValueString(), state.Version.ValueInt64(), in)
	if err != nil {
		addAPIError(&resp.Diagnostics, "Could not update the inference endpoint", err, inferenceEndpointFields, client.ScopeSecrets)
		return
	}

	addAPIWarnings(&resp.Diagnostics, "inference endpoint", endpoint.Warnings, inferenceEndpointFields)
	resp.Diagnostics.Append(resp.State.Set(ctx, inferenceEndpointModel(endpoint, &plan))...)
	resp.Diagnostics.Append(setIdentity(ctx, resp.Identity, endpoint.ID)...)
	resp.Diagnostics.Append(rememberAnswer(ctx, resp.Private, inferenceEndpointSettable(endpoint))...)
}

func (r *InferenceEndpointResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state InferenceEndpointResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteInferenceEndpoint(ctx, state.ID.ValueString(), state.Version.ValueInt64())
	if err != nil && !client.IsNotFound(err) {
		addAPIError(&resp.Diagnostics, "Could not delete the inference endpoint", err, inferenceEndpointFields, client.ScopeSecrets)
	}
}

func (r *InferenceEndpointResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	target, ok := importTargetOf(ctx, req, &resp.Diagnostics, "<workspace id>/acme")
	if !ok {
		return
	}

	var endpoint client.InferenceEndpoint
	var err error
	if target.byID() {
		endpoint, err = r.client.ReadInferenceEndpoint(ctx, target.id)
	} else {
		endpoint, err = r.client.ImportInferenceEndpoint(ctx, target.workspaceID, target.address)
	}
	if err != nil {
		addAPIError(&resp.Diagnostics, "Could not import the inference endpoint at "+target.String(), err, fieldpath.Locator{}, client.ScopeRead)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, inferenceEndpointModel(endpoint, nil))...)
	resp.Diagnostics.Append(setIdentity(ctx, resp.Identity, endpoint.ID)...)
	resp.Diagnostics.Append(rememberAnswer(ctx, resp.Private, inferenceEndpointSettable(endpoint))...)
}

func inferenceEndpointSettable(endpoint client.InferenceEndpoint) any {
	return map[string]any{
		"baseUrl":                 endpoint.BaseURL,
		"model":                   endpoint.Model,
		"supportsReasoningEffort": endpoint.SupportsReasoningEffort,
		"apiKeyWoVersion":         endpoint.APIKeyVersion,
	}
}

func inferenceEndpointInput(plan InferenceEndpointResourceModel) client.InferenceEndpointInput {
	return client.InferenceEndpointInput{
		BaseURL:                 plan.BaseURL.ValueString(),
		Model:                   plan.Model.ValueString(),
		SupportsReasoningEffort: plan.SupportsReasoningEffort.ValueBool(),
		APIKeyVersion:           plan.APIKeyVersion.ValueInt64Pointer(),
	}
}

func sendInferenceAPIKey(ctx context.Context, config tfsdk.Config, plan tfsdk.Plan, state *tfsdk.State, in *client.InferenceEndpointInput) diag.Diagnostics {
	value, diags := writeonly.InferenceAPIKey.Pending(ctx, config, plan, state)
	if !value.IsNull() {
		in.APIKey = value.ValueString()
	}

	return diags
}

func inferenceEndpointModel(endpoint client.InferenceEndpoint, prior *InferenceEndpointResourceModel) InferenceEndpointResourceModel {
	model := InferenceEndpointResourceModel{
		ID:                      types.StringValue(endpoint.ID),
		Version:                 types.Int64Value(endpoint.Version),
		Address:                 types.StringValue(endpoint.Address),
		WorkspaceID:             types.StringValue(endpoint.WorkspaceID),
		BaseURL:                 types.StringValue(endpoint.BaseURL),
		Model:                   types.StringValue(endpoint.Model),
		SupportsReasoningEffort: types.BoolValue(endpoint.SupportsReasoningEffort),
		APIKey:                  types.StringNull(),
		APIKeyVersion:           types.Int64PointerValue(endpoint.APIKeyVersion),
		APIKeySet:               types.BoolValue(endpoint.APIKeySet),
	}

	if prior != nil {
		model.BaseURL = prior.BaseURL
		model.Model = prior.Model
		model.SupportsReasoningEffort = prior.SupportsReasoningEffort
		model.APIKeyVersion = prior.APIKeyVersion
	}

	return model
}
