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
	_ resource.Resource                = (*projectResource)(nil)
	_ resource.ResourceWithConfigure   = (*projectResource)(nil)
	_ resource.ResourceWithImportState = (*projectResource)(nil)
)

// NewProjectResource is the constructor registered with the provider.
func NewProjectResource() resource.Resource {
	return &projectResource{}
}

type projectResource struct {
	client *hcloudplus.Client
}

// projectResourceModel is the Terraform state model for hcloudplus_project.
type projectResourceModel struct {
	ID      types.String `tfsdk:"id"`
	Name    types.String `tfsdk:"name"`
	Role    types.String `tfsdk:"role"`
	Created types.String `tfsdk:"created"`
	Owner   types.Object `tfsdk:"owner"`
}

func (r *projectResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_project"
}

func (r *projectResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A Hetzner Cloud project. Managed via the account-level private API.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Numeric project ID, stored as a string to satisfy Terraform's ID contract.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Human-readable project name. Can be changed in place.",
			},
			"role": schema.StringAttribute{
				Computed:    true,
				Description: "Caller's role in the project (typically 'owner' for projects created via this provider).",
			},
			"created": schema.StringAttribute{
				Computed:    true,
				Description: "RFC 3339 timestamp of project creation.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"owner": schema.SingleNestedAttribute{
				Computed:    true,
				Description: "The account that owns the project.",
				Attributes: map[string]schema.Attribute{
					"id":       schema.Int64Attribute{Computed: true},
					"email":    schema.StringAttribute{Computed: true},
					"robot_cn": schema.StringAttribute{Computed: true},
				},
			},
		},
	}
}

func (r *projectResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	c, err := clientFromResourceData(req.ProviderData)
	if err != nil {
		resp.Diagnostics.AddError("Invalid provider data", err.Error())
		return
	}
	r.client = c
}

func (r *projectResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan projectResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	p, err := r.client.Project.Create(ctx, hcloudplus.ProjectCreateOpts{Name: plan.Name.ValueString()})
	if err != nil {
		resp.Diagnostics.AddError("Failed to create project", err.Error())
		return
	}

	state := projectFromAPI(p)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *projectResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state projectResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id, err := strconv.ParseInt(state.ID.ValueString(), 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Invalid project ID", err.Error())
		return
	}

	p, err := r.client.Project.GetByID(ctx, id)
	if err != nil {
		resp.Diagnostics.AddError("Failed to read project", err.Error())
		return
	}
	if p == nil {
		// Deleted out-of-band — drop from state.
		resp.State.RemoveResource(ctx)
		return
	}

	refreshed := projectFromAPI(p)
	resp.Diagnostics.Append(resp.State.Set(ctx, refreshed)...)
}

func (r *projectResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state projectResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id, err := strconv.ParseInt(state.ID.ValueString(), 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Invalid project ID", err.Error())
		return
	}

	p, err := r.client.Project.Update(ctx, id, hcloudplus.ProjectUpdateOpts{Name: plan.Name.ValueString()})
	if err != nil {
		resp.Diagnostics.AddError("Failed to update project", err.Error())
		return
	}

	refreshed := projectFromAPI(p)
	resp.Diagnostics.Append(resp.State.Set(ctx, refreshed)...)
}

func (r *projectResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state projectResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id, err := strconv.ParseInt(state.ID.ValueString(), 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Invalid project ID", err.Error())
		return
	}

	if err := r.client.Project.Delete(ctx, id); err != nil && !hcloudplus.IsError(err, hcloudplus.ErrorCodeNotFound) {
		resp.Diagnostics.AddError("Failed to delete project", err.Error())
	}
}

func (r *projectResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if _, err := strconv.ParseInt(req.ID, 10, 64); err != nil {
		resp.Diagnostics.AddError("Invalid import ID", fmt.Sprintf("project ID must be numeric, got %q: %v", req.ID, err))
		return
	}
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
