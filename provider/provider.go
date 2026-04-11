// Package provider implements the terraform-plugin-framework Provider for
// Hetzner Cloud's undocumented console API, on top of hcloud-go-plus.
package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/hatemosphere/hcloud-go-plus/authbrowser"
	"github.com/hatemosphere/hcloud-go-plus/hcloudplus"
)

// fileCachedTokenSource wraps an authbrowser.NewTokenSource with an on-disk
// cache. Pulumi spawns the provider plugin once per phase (preview, then
// update), so without caching a single `pulumi up` would run
// authbrowser.Login twice — ~40s of headless Chromium work each time.
// This wrapper persists the minted user token to a file on first Login
// and hands it back from disk on subsequent process starts until it is
// close to expiry.
//
// The file is JSON, mode 0600, and only contains the bearer token plus
// its absolute expiry. The bearer token is ~account-scope access for
// ~12h; treat the cache path the same way you'd treat an API key file.
type fileCachedTokenSource struct {
	inner     hcloudplus.TokenSource
	cachePath string

	mu         sync.Mutex
	memToken   string
	memExpires time.Time
}

type cachedTokenFile struct {
	Token   string    `json:"token"`
	Expires time.Time `json:"expires"`
}

// refreshFraction — refresh when less than this much of the original
// lifetime remains. 20% = we start refreshing with ~2.4h left on a 12h
// token, which is plenty of slack for a long `pulumi up` to finish using
// the cached token without rolling it mid-apply.
const cachedTokenRefreshFraction = 0.2

func (f *fileCachedTokenSource) Token(ctx context.Context) (string, time.Time, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	// In-memory fast path (second call within the same process).
	if f.memToken != "" && time.Until(f.memExpires) > 30*time.Second {
		return f.memToken, f.memExpires, nil
	}

	// Disk fast path (fresh subprocess that hasn't minted yet).
	if f.cachePath != "" {
		if entry, err := readCachedToken(f.cachePath); err == nil {
			remaining := time.Until(entry.Expires)
			if remaining > 30*time.Second && remaining > time.Duration(float64(12*time.Hour)*cachedTokenRefreshFraction) {
				tflog.Debug(ctx, "using cached user token from disk", map[string]any{
					"path":           f.cachePath,
					"expires_in_sec": int64(remaining.Seconds()),
				})
				f.memToken = entry.Token
				f.memExpires = entry.Expires
				return entry.Token, entry.Expires, nil
			}
			tflog.Debug(ctx, "cached user token is too close to expiry, refreshing", map[string]any{
				"path":           f.cachePath,
				"expires_in_sec": int64(remaining.Seconds()),
			})
		}
	}

	// Fall through to actual Login.
	tflog.Info(ctx, "no usable cached token, running authbrowser.Login")
	token, expires, err := f.inner.Token(ctx)
	if err != nil {
		return "", time.Time{}, err
	}

	f.memToken = token
	f.memExpires = expires

	// Best-effort persist. A failure to write the cache must not block
	// the caller — they just pay the cold-login cost next time.
	if f.cachePath != "" {
		if err := writeCachedToken(f.cachePath, cachedTokenFile{Token: token, Expires: expires}); err != nil {
			tflog.Warn(ctx, "failed to persist cached user token; continuing without cache", map[string]any{
				"path":  f.cachePath,
				"error": err.Error(),
			})
		} else {
			tflog.Debug(ctx, "persisted cached user token to disk", map[string]any{"path": f.cachePath})
		}
	}

	return token, expires, nil
}

func defaultTokenCachePath() string {
	base, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(base, "hcloud-go-plus", "user-token.json")
}

func readCachedToken(path string) (cachedTokenFile, error) {
	var out cachedTokenFile
	data, err := os.ReadFile(path)
	if err != nil {
		return out, err
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return out, err
	}
	if out.Token == "" || out.Expires.IsZero() {
		return out, fmt.Errorf("cached token file %q is incomplete", path)
	}
	return out, nil
}

func writeCachedToken(path string, entry cachedTokenFile) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("mkdir cache dir: %w", err)
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal cache: %w", err)
	}
	// Write atomically via a temp file next to the target so a crash
	// during write never leaves a half-written token file.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write temp cache: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("rename temp cache: %w", err)
	}
	return nil
}

// Ensure hcloudplusProvider satisfies the provider.Provider interface.
var _ provider.Provider = (*hcloudplusProvider)(nil)

// New returns a factory for the provider at the given version.
func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &hcloudplusProvider{version: version}
	}
}

type hcloudplusProvider struct {
	version string
}

// Metadata returns the provider type name and version.
func (p *hcloudplusProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "hcloudplus"
	resp.Version = p.version
}

// providerModel maps the Terraform provider configuration schema to Go.
type providerModel struct {
	UserToken types.String `tfsdk:"user_token"`
	Username  types.String `tfsdk:"username"`
	Password  types.String `tfsdk:"password"`
	TOTPCmd   types.String `tfsdk:"totp_cmd"`
	Endpoint  types.String `tfsdk:"endpoint"`
}

// Schema declares the provider-level configuration block.
func (p *hcloudplusProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Terraform provider for Hetzner Cloud's undocumented console API (projects, project tokens, members, activities, limits). Wraps hcloud-go-plus.",
		Attributes: map[string]schema.Attribute{
			"user_token": schema.StringAttribute{
				Optional:    true,
				Sensitive:   true,
				Description: "Account-level user token extracted from a logged-in Hetzner Cloud Console session. Mutually exclusive with username/password/totp_cmd. Can also be set via the HCLOUDPLUS_USER_TOKEN environment variable.",
			},
			"username": schema.StringAttribute{
				Optional:    true,
				Description: "Hetzner Cloud Console username (robot login / client number or account email). Required when user_token is not set. Can also be set via HCLOUDPLUS_USERNAME.",
			},
			"password": schema.StringAttribute{
				Optional:    true,
				Sensitive:   true,
				Description: "Hetzner Cloud Console password. Required when user_token is not set. Can also be set via HCLOUDPLUS_PASSWORD.",
			},
			"totp_cmd": schema.StringAttribute{
				Optional:    true,
				Sensitive:   true,
				Description: "Shell command whose stdout (trimmed) is the current 6-digit TOTP code. Example: \"op read 'op://vault/accounts.hetzner.com/one-time password?attribute=totp'\". Required when user_token is not set. Can also be set via HCLOUDPLUS_TOTP_CMD.",
			},
			"endpoint": schema.StringAttribute{
				Optional:    true,
				Description: "Override the API base URL. Defaults to https://api.hetzner.cloud/v1. Useful for recording/replay or tests.",
			},
		},
	}
}

// Configure builds a shared *hcloudplus.Client from the provider config and
// makes it available to every resource and data source via the
// {Resource,DataSource}ConfigureRequest.ProviderData field.
func (p *hcloudplusProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	tflog.Info(ctx, "configuring hcloudplus provider", map[string]any{"version": p.version})

	var cfg providerModel
	diags := req.Config.Get(ctx, &cfg)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Resolve env var fallbacks.
	userToken := stringOrEnv(cfg.UserToken, "HCLOUDPLUS_USER_TOKEN")
	username := stringOrEnv(cfg.Username, "HCLOUDPLUS_USERNAME")
	password := stringOrEnv(cfg.Password, "HCLOUDPLUS_PASSWORD")
	totpCmd := stringOrEnv(cfg.TOTPCmd, "HCLOUDPLUS_TOTP_CMD")
	endpoint := stringOrEnv(cfg.Endpoint, "HCLOUDPLUS_ENDPOINT")

	// Bridge SDK debug output into tflog so `TF_LOG=DEBUG terraform plan`
	// shows everything authbrowser / hcloudplus emit.
	sdkLogWriter := newTflogWriter(ctx, "hcloudplus-sdk")

	var opts []hcloudplus.Option
	if endpoint != "" {
		opts = append(opts, hcloudplus.WithEndpoint(endpoint))
		tflog.Debug(ctx, "using custom endpoint", map[string]any{"endpoint": endpoint})
	}
	opts = append(opts,
		hcloudplus.WithApplication("terraform-provider-hcloudplus", p.version),
		hcloudplus.WithDebugWriter(sdkLogWriter),
	)

	switch {
	case userToken != "":
		if username != "" || password != "" || totpCmd != "" {
			resp.Diagnostics.AddError(
				"Conflicting provider configuration",
				"Set either user_token OR (username + password + totp_cmd). Not both.",
			)
			return
		}
		tflog.Info(ctx, "using BYO user token auth mode")
		opts = append(opts, hcloudplus.WithUserToken(userToken))

	case username != "" && password != "" && totpCmd != "":
		tflog.Info(ctx, "using cold-login auth mode via authbrowser", map[string]any{"username": username})
		creds := authbrowser.Credentials{
			Username: username,
			Password: password,
			TOTP: func(ctx context.Context) (string, error) {
				tflog.Debug(ctx, "invoking totp_cmd", map[string]any{"cmd": totpCmd})
				out, err := exec.CommandContext(ctx, "sh", "-c", totpCmd).Output()
				if err != nil {
					return "", fmt.Errorf("totp_cmd: %w", err)
				}
				code := strings.TrimSpace(string(out))
				if code == "" {
					return "", fmt.Errorf("totp_cmd returned empty output — check that %q works in your shell", totpCmd)
				}
				tflog.Debug(ctx, "totp_cmd produced code", map[string]any{"len": len(code)})
				return code, nil
			},
		}

		// Persist the user token to disk so subsequent provider subprocess
		// starts (Pulumi spawns one per phase: preview then update) reuse
		// the cached token instead of re-running authbrowser.Login. Path
		// can be overridden via HCLOUDPLUS_TOKEN_CACHE; set it to an empty
		// string to disable caching entirely.
		cachePath := defaultTokenCachePath()
		if v, ok := os.LookupEnv("HCLOUDPLUS_TOKEN_CACHE"); ok {
			cachePath = v
		}
		inner := authbrowser.NewTokenSource(creds, authbrowser.WithDebugWriter(sdkLogWriter))
		ts := &fileCachedTokenSource{
			inner:     inner,
			cachePath: cachePath,
		}
		opts = append(opts, hcloudplus.WithTokenSource(ts))

	default:
		resp.Diagnostics.AddError(
			"Missing provider credentials",
			"Set user_token (for pre-minted tokens) OR username + password + totp_cmd (for cold login via headless browser). "+
				"Equivalent HCLOUDPLUS_* environment variables are also honoured.",
		)
		return
	}

	client := hcloudplus.NewClient(opts...)

	// Probe the token with a cheap call so provider init fails fast on bad
	// credentials rather than surfacing the error on the first resource op.
	tflog.Debug(ctx, "probing token via GET /_tokens/current")
	if _, err := client.User.CurrentToken(ctx); err != nil {
		resp.Diagnostics.AddError("Failed to initialise hcloudplus client", err.Error())
		return
	}
	tflog.Info(ctx, "hcloudplus provider configured successfully")

	resp.ResourceData = client
	resp.DataSourceData = client
}

// Resources returns the set of resource types this provider manages.
func (p *hcloudplusProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewProjectResource,
		NewProjectTokenResource,
		NewProjectInvitationResource,
	}
}

// DataSources returns the set of data source types this provider exposes.
func (p *hcloudplusProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewUserDataSource,
		NewLimitsDataSource,
		NewProjectsDataSource,
		NewActivitiesDataSource,
	}
}

// stringOrEnv returns the value of attr if set, otherwise falls back to the
// given environment variable.
func stringOrEnv(attr types.String, env string) string {
	if !attr.IsNull() && !attr.IsUnknown() {
		return attr.ValueString()
	}
	return envOrEmpty(env)
}

// clientFromResourceData unwraps the *hcloudplus.Client passed via
// ProviderData, with a typed error diagnostic on mismatch.
func clientFromResourceData(data any) (*hcloudplus.Client, error) {
	if data == nil {
		return nil, nil
	}
	client, ok := data.(*hcloudplus.Client)
	if !ok {
		return nil, fmt.Errorf("unexpected ProviderData type %T, expected *hcloudplus.Client", data)
	}
	return client, nil
}
