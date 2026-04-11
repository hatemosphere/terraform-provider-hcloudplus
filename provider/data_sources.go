package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hatemosphere/hcloud-go-plus/hcloudplus"
)

// ---------------------------------------------------------------------------
// hcloudplus_user — GET /_user
// ---------------------------------------------------------------------------

var (
	_ datasource.DataSource              = (*userDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*userDataSource)(nil)
)

func NewUserDataSource() datasource.DataSource { return &userDataSource{} }

type userDataSource struct{ client *hcloudplus.Client }

type userDataSourceModel struct {
	ID        types.Int64  `tfsdk:"id"`
	Email     types.String `tfsdk:"email"`
	RobotCN   types.String `tfsdk:"robot_cn"`
	FirstName types.String `tfsdk:"first_name"`
	LastName  types.String `tfsdk:"last_name"`
	Company   types.String `tfsdk:"company_name"`
	Locale    types.String `tfsdk:"locale"`
	Currency  types.String `tfsdk:"currency"`
}

func (d *userDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_user"
}

func (d *userDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "The authenticated user (account owner). Reads GET /_user on the Hetzner Cloud Console private API.",
		Attributes: map[string]schema.Attribute{
			"id":           schema.Int64Attribute{Computed: true},
			"email":        schema.StringAttribute{Computed: true},
			"robot_cn":     schema.StringAttribute{Computed: true},
			"first_name":   schema.StringAttribute{Computed: true},
			"last_name":    schema.StringAttribute{Computed: true},
			"company_name": schema.StringAttribute{Computed: true},
			"locale":       schema.StringAttribute{Computed: true},
			"currency":     schema.StringAttribute{Computed: true},
		},
	}
}

func (d *userDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	c, err := clientFromResourceData(req.ProviderData)
	if err != nil {
		resp.Diagnostics.AddError("Invalid provider data", err.Error())
		return
	}
	d.client = c
}

func (d *userDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	u, err := d.client.User.Me(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Failed to read current user", err.Error())
		return
	}
	state := userDataSourceModel{
		ID:        types.Int64Value(u.ID),
		Email:     types.StringValue(u.Email),
		RobotCN:   types.StringValue(u.RobotCN),
		FirstName: types.StringValue(u.FirstName),
		LastName:  types.StringValue(u.LastName),
		Company:   types.StringValue(u.CompanyName),
		Locale:    types.StringValue(u.Locale),
		Currency:  types.StringValue(u.Currency),
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

// ---------------------------------------------------------------------------
// hcloudplus_limits — GET /_user/limits
// ---------------------------------------------------------------------------

var (
	_ datasource.DataSource              = (*limitsDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*limitsDataSource)(nil)
)

func NewLimitsDataSource() datasource.DataSource { return &limitsDataSource{} }

type limitsDataSource struct{ client *hcloudplus.Client }

type limitsDataSourceModel struct {
	Limits          types.Map  `tfsdk:"limits"`
	ExtendedSupport types.Bool `tfsdk:"extended_support"`
	RequestAllowed  types.Bool `tfsdk:"request_allowed"`
}

// limitEntryAttrTypes is the nested object shape used inside limits.
var limitEntryAttrTypes = map[string]attr.Type{
	"limit": types.Int64Type,
	"usage": types.Int64Type,
}

func (d *limitsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_limits"
}

func (d *limitsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Account-wide resource limits and usage. Reads GET /_user/limits. Useful for building capacity dashboards or Prometheus exporters.",
		Attributes: map[string]schema.Attribute{
			"limits": schema.MapNestedAttribute{
				Computed:    true,
				Description: "Map of resource name (e.g. 'server', 'bucket') to {limit, usage}.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"limit": schema.Int64Attribute{Computed: true},
						"usage": schema.Int64Attribute{Computed: true},
					},
				},
			},
			"extended_support": schema.BoolAttribute{Computed: true},
			"request_allowed":  schema.BoolAttribute{Computed: true},
		},
	}
}

func (d *limitsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	c, err := clientFromResourceData(req.ProviderData)
	if err != nil {
		resp.Diagnostics.AddError("Invalid provider data", err.Error())
		return
	}
	d.client = c
}

func (d *limitsDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	l, err := d.client.User.Limits(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Failed to read user limits", err.Error())
		return
	}

	entries := make(map[string]attr.Value, len(l.Limits))
	for name, e := range l.Limits {
		obj, diags := types.ObjectValue(limitEntryAttrTypes, map[string]attr.Value{
			"limit": types.Int64Value(int64(e.Limit)),
			"usage": types.Int64Value(int64(e.Usage)),
		})
		resp.Diagnostics.Append(diags...)
		entries[name] = obj
	}
	mapVal, diags := types.MapValue(types.ObjectType{AttrTypes: limitEntryAttrTypes}, entries)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	state := limitsDataSourceModel{
		Limits:          mapVal,
		ExtendedSupport: types.BoolValue(l.Permissions.ExtendedSupport),
		RequestAllowed:  types.BoolValue(l.Permissions.RequestAllowed),
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

// ---------------------------------------------------------------------------
// hcloudplus_projects — GET /_projects (all pages)
// ---------------------------------------------------------------------------

var (
	_ datasource.DataSource              = (*projectsDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*projectsDataSource)(nil)
)

func NewProjectsDataSource() datasource.DataSource { return &projectsDataSource{} }

type projectsDataSource struct{ client *hcloudplus.Client }

type projectsDataSourceModel struct {
	Projects []projectSummary `tfsdk:"projects"`
}

type projectSummary struct {
	ID      types.Int64  `tfsdk:"id"`
	Name    types.String `tfsdk:"name"`
	Role    types.String `tfsdk:"role"`
	Created types.String `tfsdk:"created"`
}

func (d *projectsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_projects"
}

func (d *projectsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "All projects the authenticated user has access to. Reads GET /_projects, walking every page.",
		Attributes: map[string]schema.Attribute{
			"projects": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":      schema.Int64Attribute{Computed: true},
						"name":    schema.StringAttribute{Computed: true},
						"role":    schema.StringAttribute{Computed: true},
						"created": schema.StringAttribute{Computed: true},
					},
				},
			},
		},
	}
}

func (d *projectsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	c, err := clientFromResourceData(req.ProviderData)
	if err != nil {
		resp.Diagnostics.AddError("Invalid provider data", err.Error())
		return
	}
	d.client = c
}

func (d *projectsDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	ps, err := d.client.Project.All(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Failed to list projects", err.Error())
		return
	}
	state := projectsDataSourceModel{Projects: make([]projectSummary, 0, len(ps))}
	for _, p := range ps {
		state.Projects = append(state.Projects, projectSummary{
			ID:      types.Int64Value(p.ID),
			Name:    types.StringValue(p.Name),
			Role:    types.StringValue(p.Role),
			Created: types.StringValue(p.Created.UTC().Format("2006-01-02T15:04:05Z")),
		})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

// ---------------------------------------------------------------------------
// hcloudplus_activities — GET /_activities
// ---------------------------------------------------------------------------

var (
	_ datasource.DataSource              = (*activitiesDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*activitiesDataSource)(nil)
)

func NewActivitiesDataSource() datasource.DataSource { return &activitiesDataSource{} }

type activitiesDataSource struct{ client *hcloudplus.Client }

type activitiesDataSourceModel struct {
	IncludeUserActivities types.Bool      `tfsdk:"include_user_activities"`
	ProjectIds            types.List      `tfsdk:"project_ids"`
	Types                 types.List      `tfsdk:"types"`
	CreatedAfter          types.String    `tfsdk:"created_after"`
	CreatedBefore         types.String    `tfsdk:"created_before"`
	PerPage               types.Int64     `tfsdk:"per_page"`
	Activities            []activityEntry `tfsdk:"activities"`
}

type activityEntry struct {
	ID           types.Int64  `tfsdk:"id"`
	ActivityType types.String `tfsdk:"activity_type"`
	Created      types.String `tfsdk:"created"`
	Status       types.String `tfsdk:"status"`
	SourceIP     types.String `tfsdk:"source_ip"`
	ProjectID    types.Int64  `tfsdk:"project_id"`
	ProjectName  types.String `tfsdk:"project_name"`
	APITokenPref types.String `tfsdk:"api_token_prefix"`
	UserRobotCN  types.String `tfsdk:"user_robot_cn"`
}

func (d *activitiesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_activities"
}

func (d *activitiesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Cross-project audit activities. Reads GET /_activities. Time window is capped at 31 days by Hetzner.",
		Attributes: map[string]schema.Attribute{
			"include_user_activities": schema.BoolAttribute{
				Optional:    true,
				Description: "Include account-level events (logins, etc). At least one of include_user_activities or project_ids is required.",
			},
			"project_ids": schema.ListAttribute{
				Optional:    true,
				ElementType: types.Int64Type,
				Description: "Filter to specific project IDs.",
			},
			"types": schema.ListAttribute{
				Optional:    true,
				ElementType: types.StringType,
				Description: "Filter by activity type (e.g. 'server.create', 'token.create').",
			},
			"created_after": schema.StringAttribute{
				Optional:    true,
				Description: "RFC 3339 lower time bound.",
			},
			"created_before": schema.StringAttribute{
				Optional:    true,
				Description: "RFC 3339 upper time bound. Window capped at 31 days.",
			},
			"per_page": schema.Int64Attribute{
				Optional:    true,
				Description: "Page size (default 25, max 500).",
			},
			"activities": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":               schema.Int64Attribute{Computed: true},
						"activity_type":    schema.StringAttribute{Computed: true},
						"created":          schema.StringAttribute{Computed: true},
						"status":           schema.StringAttribute{Computed: true},
						"source_ip":        schema.StringAttribute{Computed: true},
						"project_id":       schema.Int64Attribute{Computed: true},
						"project_name":     schema.StringAttribute{Computed: true},
						"api_token_prefix": schema.StringAttribute{Computed: true},
						"user_robot_cn":    schema.StringAttribute{Computed: true},
					},
				},
			},
		},
	}
}

func (d *activitiesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	c, err := clientFromResourceData(req.ProviderData)
	if err != nil {
		resp.Diagnostics.AddError("Invalid provider data", err.Error())
		return
	}
	d.client = c
}

func (d *activitiesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg activitiesDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	opts := hcloudplus.ActivityListOpts{}
	if !cfg.IncludeUserActivities.IsNull() {
		opts.IncludeUserActivities = cfg.IncludeUserActivities.ValueBool()
	}
	if !cfg.ProjectIds.IsNull() {
		var ids []int64
		resp.Diagnostics.Append(cfg.ProjectIds.ElementsAs(ctx, &ids, false)...)
		opts.ProjectIDs = ids
	}
	if !cfg.Types.IsNull() {
		var tt []string
		resp.Diagnostics.Append(cfg.Types.ElementsAs(ctx, &tt, false)...)
		opts.Types = tt
	}
	if !cfg.CreatedAfter.IsNull() && cfg.CreatedAfter.ValueString() != "" {
		t, err := parseRFC3339(cfg.CreatedAfter.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Invalid created_after", err.Error())
			return
		}
		opts.CreatedAfter = t
	}
	if !cfg.CreatedBefore.IsNull() && cfg.CreatedBefore.ValueString() != "" {
		t, err := parseRFC3339(cfg.CreatedBefore.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Invalid created_before", err.Error())
			return
		}
		opts.CreatedBefore = t
	}
	if !cfg.PerPage.IsNull() {
		opts.PerPage = int(cfg.PerPage.ValueInt64())
	}
	if resp.Diagnostics.HasError() {
		return
	}

	acts, _, err := d.client.Activity.List(ctx, opts)
	if err != nil {
		resp.Diagnostics.AddError("Failed to list activities", err.Error())
		return
	}

	cfg.Activities = make([]activityEntry, 0, len(acts))
	for _, a := range acts {
		entry := activityEntry{
			ID:           types.Int64Value(a.ID),
			ActivityType: types.StringValue(a.ActivityType),
			Created:      types.StringValue(a.Created.UTC().Format("2006-01-02T15:04:05Z")),
			Status:       types.StringValue(a.Status),
			SourceIP:     types.StringValue(a.SourceIP),
		}
		if a.Project != nil {
			entry.ProjectID = types.Int64Value(a.Project.ID)
			entry.ProjectName = types.StringValue(a.Project.Name)
		} else {
			entry.ProjectID = types.Int64Value(0)
			entry.ProjectName = types.StringValue("")
		}
		if a.APIToken != nil {
			entry.APITokenPref = types.StringValue(a.APIToken.Prefix)
		} else {
			entry.APITokenPref = types.StringValue("")
		}
		if a.User != nil {
			entry.UserRobotCN = types.StringValue(a.User.RobotCN)
		} else {
			entry.UserRobotCN = types.StringValue("")
		}
		cfg.Activities = append(cfg.Activities, entry)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, cfg)...)
}
