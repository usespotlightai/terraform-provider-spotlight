package resourcekit

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/fieldpath"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/spotlight"
)

// Spec describes one kind of resource: its schema, how its Terraform model
// is written to the API, and how an answer is held in Terraform.
//
// Stored builds the model Terraform holds for an answer. Kept is what was
// planned, after a write, or what Terraform already held, after a read that
// found nothing changed since; settable attributes are kept from it, with
// anything unknown filled in from the answer. Kept is nil on import and
// after a read that found a change, and then everything comes from the
// answer.
type Spec[In, Out, M any] struct {
	Kind                   string
	Schema                 schema.Schema
	Renames                map[string]string
	Under                  fieldpath.Path
	AddressedAs            string
	Resources              func(spotlight.API) spotlight.Resources[In, Out]
	Owner                  func(M) string
	Input                  func(M) (In, diag.Diagnostics)
	Stored                 func(answer Out, kept *M) (M, diag.Diagnostics)
	Meta                   func(Out) spotlight.Meta
	Settable               func(Out) any
	Identity               func(M) (id string, version int64)
	ImportedByAddressAlone bool
}

// Managed is a resource the provider manages through a Spec.
type Managed[In, Out, M any] struct {
	spec    Spec[In, Out, M]
	clients *Clients
	at      fieldpath.Locator
}

var (
	_ resource.ResourceWithConfigure   = (*Managed[any, any, any])(nil)
	_ resource.ResourceWithImportState = (*Managed[any, any, any])(nil)
)

// NewManaged returns the resource spec describes.
func NewManaged[In, Out, M any](spec Spec[In, Out, M]) *Managed[In, Out, M] {
	return &Managed[In, Out, M]{
		spec: spec,
		at:   fieldpath.Locator{Schema: spec.Schema.Type(), Renames: spec.Renames, Under: spec.Under},
	}
}

// Clients answers the clients the provider configured, or nil before then.
func (m *Managed[In, Out, M]) Clients() *Clients { return m.clients }

// Metadata names the resource spotlight_<kind>.
func (m *Managed[In, Out, M]) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + m.spec.Kind
}

// Schema answers the resource's schema.
func (m *Managed[In, Out, M]) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = m.spec.Schema
}

// Configure takes the clients the provider configured.
func (m *Managed[In, Out, M]) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	m.clients = Configured(req, resp)
}

func (m *Managed[In, Out, M]) api() spotlight.Resources[In, Out] {
	return m.spec.Resources(m.clients.API)
}

// Create creates the resource as planned.
func (m *Managed[In, Out, M]) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	if m.clients == nil {
		Unconfigured(&resp.Diagnostics)
		return
	}

	var plan M
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	in, diags := m.spec.Input(plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	owner := ""
	if m.spec.Owner != nil {
		owner = m.spec.Owner(plan)
	}

	created, err := m.api().Create(ctx, owner, in)
	if err != nil {
		Failed(&resp.Diagnostics, "Could not create the "+m.spec.Kind, err, m.at)
		return
	}

	Warned(&resp.Diagnostics, m.spec.Kind, m.spec.Meta(created).Warnings, m.at)
	m.hold(ctx, created, &plan, resp.Private, &resp.State, &resp.Diagnostics)
}

// Read refreshes the resource, keeping what Terraform holds when nothing
// changed it since it was last written.
func (m *Managed[In, Out, M]) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	if m.clients == nil {
		Unconfigured(&resp.Diagnostics)
		return
	}

	var prior M
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id, _ := m.spec.Identity(prior)

	found, err := m.api().Read(ctx, id)
	if spotlight.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		Failed(&resp.Diagnostics, "Could not read the "+m.spec.Kind, err, m.at)
		return
	}

	if Unchanged(ctx, req.Private, m.spec.Settable(found)) {
		m.hold(ctx, found, &prior, resp.Private, &resp.State, &resp.Diagnostics)
		return
	}

	m.hold(ctx, found, nil, resp.Private, &resp.State, &resp.Diagnostics)
}

// Update writes the plan over the version Terraform last read.
func (m *Managed[In, Out, M]) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	if m.clients == nil {
		Unconfigured(&resp.Diagnostics)
		return
	}

	var plan, prior M
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}

	in, diags := m.spec.Input(plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	id, version := m.spec.Identity(prior)

	updated, err := m.api().Update(ctx, id, version, in)
	if err != nil {
		Failed(&resp.Diagnostics, "Could not update the "+m.spec.Kind, err, m.at)
		return
	}

	Warned(&resp.Diagnostics, m.spec.Kind, m.spec.Meta(updated).Warnings, m.at)
	m.hold(ctx, updated, &plan, resp.Private, &resp.State, &resp.Diagnostics)
}

// Delete deletes the version Terraform last read.
func (m *Managed[In, Out, M]) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	if m.clients == nil {
		Unconfigured(&resp.Diagnostics)
		return
	}

	var prior M
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id, version := m.spec.Identity(prior)

	if err := m.api().Delete(ctx, id, version); err != nil && !spotlight.IsNotFound(err) {
		Failed(&resp.Diagnostics, "Could not delete the "+m.spec.Kind, err, m.at)
	}
}

// ImportState adopts the resource at an address.
func (m *Managed[In, Out, M]) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if m.clients == nil {
		Unconfigured(&resp.Diagnostics)
		return
	}

	workspaceID, address := "", req.ID
	if !m.spec.ImportedByAddressAlone {
		var ok bool
		workspaceID, address, ok = SplitImportID(req.ID)
		if !ok {
			BadImportID(&resp.Diagnostics, req.ID, "<workspace id>/"+m.spec.AddressedAs)
			return
		}
	}

	found, err := m.api().Import(ctx, workspaceID, address)
	if err != nil {
		Failed(&resp.Diagnostics, "Could not import the "+m.spec.Kind+" at "+req.ID, err, fieldpath.Locator{})
		return
	}

	m.hold(ctx, found, nil, resp.Private, &resp.State, &resp.Diagnostics)
}

type settable interface {
	Set(ctx context.Context, value any) diag.Diagnostics
}

func (m *Managed[In, Out, M]) hold(ctx context.Context, answer Out, kept *M, private PrivateState, state settable, diags *diag.Diagnostics) {
	stored, more := m.spec.Stored(answer, kept)
	diags.Append(more...)
	if diags.HasError() {
		return
	}

	diags.Append(state.Set(ctx, stored)...)
	diags.Append(Remember(ctx, private, m.spec.Settable(answer))...)
}
