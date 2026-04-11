package provider

import (
	"context"
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hatemosphere/hcloud-go-plus/hcloudplus"
)

var (
	_ resource.Resource              = (*projectInvitationResource)(nil)
	_ resource.ResourceWithConfigure = (*projectInvitationResource)(nil)
)

// NewProjectInvitationResource is the constructor registered with the provider.
func NewProjectInvitationResource() resource.Resource {
	return &projectInvitationResource{}
}

type projectInvitationResource struct {
	client *hcloudplus.Client
}

type projectInvitationResourceModel struct {
	ID               types.String `tfsdk:"id"`
	ProjectID        types.Int64  `tfsdk:"project_id"`
	Email            types.String `tfsdk:"email"`
	Role             types.String `tfsdk:"role"`
	Created          types.String `tfsdk:"created"`
	Accepted         types.String `tfsdk:"accepted"`
	Expired          types.String `tfsdk:"expired"`
	ReinviteOnExpiry types.Bool   `tfsdk:"reinvite_on_expiry"`
}

func (r *projectInvitationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_project_invitation"
}

func (r *projectInvitationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A pending project membership invitation. Invitations are immutable: changing email or role forces replacement (Hetzner does not expose an update endpoint). Accepted invitations continue to exist as project members, which this resource does not manage.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Numeric invitation ID as a string.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"project_id": schema.Int64Attribute{
				Required:    true,
				Description: "ID of the project to invite the user into.",
			},
			"email": schema.StringAttribute{
				Required:    true,
				Description: "Email address of the invitee. Hetzner accepts any syntactically valid email; it does not validate deliverability.",
			},
			"role": schema.StringAttribute{
				Required:    true,
				Description: "Role to grant on acceptance. One of: admin, member, restricted.",
			},
			"created": schema.StringAttribute{
				Computed:    true,
				Description: "RFC 3339 timestamp when the invitation was sent.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"accepted": schema.StringAttribute{
				Computed:    true,
				Description: "RFC 3339 timestamp when the invitation was accepted, or empty if still pending.",
			},
			"expired": schema.StringAttribute{
				Computed: true,
				Description: "RFC 3339 timestamp when the invitation expired (Hetzner times invitations out after some period), " +
					"or empty if still valid. A non-empty value on refresh means the invite can no longer be accepted by the recipient. " +
					"Combine with `terraform apply -replace` or the `reinvite_on_expiry` attribute to mint a fresh invitation.",
			},
			"reinvite_on_expiry": schema.BoolAttribute{
				Optional: true,
				Description: "If true, Read will check whether the invitation has expired; if so, the resource is marked " +
					"for replacement on the next plan (a fresh invitation is created and the stale one is revoked). " +
					"Defaults to false. This is the only way to get automatic re-invitation without a manual `-replace`.",
			},
		},
	}
}

func (r *projectInvitationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	c, err := clientFromResourceData(req.ProviderData)
	if err != nil {
		resp.Diagnostics.AddError("Invalid provider data", err.Error())
		return
	}
	r.client = c
}

func (r *projectInvitationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan projectInvitationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	inv, err := r.client.ProjectInvitation.Create(ctx, plan.ProjectID.ValueInt64(), hcloudplus.ProjectInvitationCreateOpts{
		Email: plan.Email.ValueString(),
		Role:  plan.Role.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Failed to create project invitation", err.Error())
		return
	}

	state := projectInvitationResourceModel{
		ID:               types.StringValue(int64ToString(inv.ID)),
		ProjectID:        plan.ProjectID,
		Email:            types.StringValue(inv.Email),
		Role:             types.StringValue(inv.Role),
		Created:          types.StringValue(inv.Created.UTC().Format("2006-01-02T15:04:05Z")),
		Accepted:         types.StringValue(""),
		Expired:          types.StringValue(""),
		ReinviteOnExpiry: plan.ReinviteOnExpiry,
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *projectInvitationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state projectInvitationResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id, err := strconv.ParseInt(state.ID.ValueString(), 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Invalid invitation ID", err.Error())
		return
	}

	invs, err := r.client.ProjectInvitation.All(ctx, state.ProjectID.ValueInt64())
	if err != nil {
		resp.Diagnostics.AddError("Failed to list invitations", err.Error())
		return
	}

	for _, inv := range invs {
		if inv.ID != id {
			continue
		}
		state.Email = types.StringValue(inv.Email)
		state.Role = types.StringValue(inv.Role)
		state.Created = types.StringValue(inv.Created.UTC().Format("2006-01-02T15:04:05Z"))
		if inv.Accepted != nil {
			state.Accepted = types.StringValue(inv.Accepted.UTC().Format("2006-01-02T15:04:05Z"))
		} else {
			state.Accepted = types.StringValue("")
		}
		if inv.Expired != nil {
			state.Expired = types.StringValue(inv.Expired.UTC().Format("2006-01-02T15:04:05Z"))
			// When reinvite_on_expiry is enabled, drop the resource from
			// state so the next plan creates a fresh invitation. The stale
			// one is effectively abandoned on Hetzner's side (already
			// expired, no-op to delete), and the replacement will be a
			// brand-new Create.
			if !state.ReinviteOnExpiry.IsNull() && state.ReinviteOnExpiry.ValueBool() {
				resp.State.RemoveResource(ctx)
				return
			}
		} else {
			state.Expired = types.StringValue("")
		}
		resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
		return
	}

	// Invitation was accepted or revoked out-of-band.
	resp.State.RemoveResource(ctx)
}

func (r *projectInvitationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// No update endpoint exists on the private API. Treat as no-op — the
	// only way email/role change in plan is via RequiresReplace.
	var plan projectInvitationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *projectInvitationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state projectInvitationResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id, err := strconv.ParseInt(state.ID.ValueString(), 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Invalid invitation ID", err.Error())
		return
	}

	if err := r.client.ProjectInvitation.Delete(ctx, state.ProjectID.ValueInt64(), id); err != nil &&
		!hcloudplus.IsError(err, hcloudplus.ErrorCodeNotFound) {
		resp.Diagnostics.AddError("Failed to delete project invitation", err.Error())
	}
}

// ImportState supports `terraform import hcloudplus_project_invitation.foo <project_id>:<invitation_id>`.
func (r *projectInvitationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	var projectID, invitationID int64
	if _, err := fmt.Sscanf(req.ID, "%d:%d", &projectID, &invitationID); err != nil {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			fmt.Sprintf("Expected <project_id>:<invitation_id>, got %q: %v", req.ID, err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), int64ToString(invitationID))...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("project_id"), projectID)...)
}
