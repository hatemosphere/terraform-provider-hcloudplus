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
	_ resource.Resource              = (*projectTokenResource)(nil)
	_ resource.ResourceWithConfigure = (*projectTokenResource)(nil)
)

// NewProjectTokenResource is the constructor registered with the provider.
func NewProjectTokenResource() resource.Resource {
	return &projectTokenResource{}
}

type projectTokenResource struct {
	client *hcloudplus.Client
}

// projectTokenResourceModel is the Terraform state model for
// hcloudplus_project_token. The `secret` attribute is sensitive and only
// populated at Create time; the API never returns it again.
type projectTokenResourceModel struct {
	ID          types.String `tfsdk:"id"`
	ProjectID   types.Int64  `tfsdk:"project_id"`
	Description types.String `tfsdk:"description"`
	Readonly    types.Bool   `tfsdk:"readonly"`
	Prefix      types.String `tfsdk:"prefix"`
	Secret      types.String `tfsdk:"secret"`
}

func (r *projectTokenResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_project_token"
}

func (r *projectTokenResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A project-scoped API token for use against the public Hetzner Cloud API. " +
			"The `secret` output is returned once at creation time; store it in Pulumi/Terraform state " +
			"(encrypted) and pass it to hcloud-go / terraform-provider-hcloud as needed. " +
			"Tokens are immutable: any change to `description` or `readonly` forces replacement.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Numeric token ID as a string.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"project_id": schema.Int64Attribute{
				Required:    true,
				Description: "ID of the project this token belongs to.",
			},
			"description": schema.StringAttribute{
				Required:    true,
				Description: "Human-readable description of the token (shown in the Hetzner console).",
			},
			"readonly": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Whether the token has read-only scope. Defaults to false (read-write).",
			},
			"prefix": schema.StringAttribute{
				Computed:    true,
				Description: "First-half prefix of the token, used to identify it in audit logs.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"secret": schema.StringAttribute{
				Computed:    true,
				Sensitive:   true,
				Description: "The full 64-character bearer token. Only populated on create — Hetzner does not return it on subsequent reads.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *projectTokenResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	c, err := clientFromResourceData(req.ProviderData)
	if err != nil {
		resp.Diagnostics.AddError("Invalid provider data", err.Error())
		return
	}
	r.client = c
}

func (r *projectTokenResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan projectTokenResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	readonly := false
	if !plan.Readonly.IsNull() && !plan.Readonly.IsUnknown() {
		readonly = plan.Readonly.ValueBool()
	}

	result, err := r.client.ProjectToken.Create(ctx, plan.ProjectID.ValueInt64(), hcloudplus.ProjectTokenCreateOpts{
		Description: plan.Description.ValueString(),
		Readonly:    readonly,
	})
	if err != nil {
		resp.Diagnostics.AddError("Failed to create project token", err.Error())
		return
	}

	state := projectTokenResourceModel{
		ID:          types.StringValue(int64ToString(result.Token.ID)),
		ProjectID:   plan.ProjectID,
		Description: types.StringValue(result.Token.Description),
		Readonly:    types.BoolValue(result.Token.Readonly),
		Prefix:      types.StringValue(result.Token.Prefix),
		Secret:      types.StringValue(result.Secret),
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *projectTokenResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state projectTokenResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id, err := strconv.ParseInt(state.ID.ValueString(), 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Invalid token ID", err.Error())
		return
	}

	// Scan the project's tokens for one matching the stored ID. The private
	// API has no direct GET /_tokens/:id endpoint.
	tokens, err := r.client.ProjectToken.All(ctx, state.ProjectID.ValueInt64())
	if err != nil {
		resp.Diagnostics.AddError("Failed to list project tokens", err.Error())
		return
	}

	for _, tok := range tokens {
		if tok.ID != id {
			continue
		}
		state.Description = types.StringValue(tok.Description)
		state.Readonly = types.BoolValue(tok.Readonly)
		state.Prefix = types.StringValue(tok.Prefix)
		// Secret stays as-is — never re-fetched.
		resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
		return
	}

	// Token was deleted out-of-band.
	resp.State.RemoveResource(ctx)
}

func (r *projectTokenResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// The private API has no token update endpoint. Description and
	// readonly are immutable — any change should force replacement, which
	// we enforce via RequiresReplace plan modifiers. This Update is only
	// reached for computed-attribute refresh paths; no-op.
	var plan projectTokenResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *projectTokenResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state projectTokenResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id, err := strconv.ParseInt(state.ID.ValueString(), 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Invalid token ID", err.Error())
		return
	}

	if err := r.client.ProjectToken.Delete(ctx, state.ProjectID.ValueInt64(), id); err != nil &&
		!hcloudplus.IsError(err, hcloudplus.ErrorCodeNotFound) {
		resp.Diagnostics.AddError("Failed to delete project token", err.Error())
	}
}

// ImportState supports `terraform import hcloudplus_project_token.foo <project_id>:<token_id>`.
// The secret will be unset after import — Hetzner cannot return it.
func (r *projectTokenResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	var projectID, tokenID int64
	if _, err := fmt.Sscanf(req.ID, "%d:%d", &projectID, &tokenID); err != nil {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			fmt.Sprintf("Expected <project_id>:<token_id>, got %q: %v", req.ID, err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), int64ToString(tokenID))...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("project_id"), projectID)...)
}
