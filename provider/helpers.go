package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/hatemosphere/hcloud-go-plus/hcloudplus"
)

func parseRFC3339(s string) (time.Time, error) {
	return time.Parse(time.RFC3339, s)
}

// tflogWriter adapts an io.Writer to Terraform's structured logger. Every
// newline-delimited chunk becomes a tflog.Debug call on the writer's stored
// context, tagged with the given subsystem so users can filter via
// TF_LOG_PROVIDER_HCLOUDPLUS.
//
// Bridge target: hcloudplus.WithDebugWriter and authbrowser.WithDebugWriter.
// The underlying SDK packages write debug traces as plain text lines; this
// writer forwards each line into Terraform's log stream, where it is
// governed by TF_LOG=DEBUG/TRACE.
//
// Safe for concurrent writes.
type tflogWriter struct {
	ctx       context.Context
	subsystem string

	mu  sync.Mutex
	buf []byte
}

func newTflogWriter(ctx context.Context, subsystem string) *tflogWriter {
	return &tflogWriter{ctx: ctx, subsystem: subsystem}
}

// Write implements io.Writer. It accumulates input until a newline is seen,
// then flushes each complete line to tflog.Debug.
func (w *tflogWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf = append(w.buf, p...)
	for {
		i := bytes.IndexByte(w.buf, '\n')
		if i < 0 {
			break
		}
		line := string(bytes.TrimRight(w.buf[:i], "\r"))
		w.buf = w.buf[i+1:]
		if line == "" {
			continue
		}
		tflog.Debug(w.ctx, line, map[string]any{"subsystem": w.subsystem})
	}
	return len(p), nil
}

// envOrEmpty returns the value of env var `name` or "" if unset.
func envOrEmpty(name string) string {
	return os.Getenv(name)
}

// projectOwnerAttrTypes describes the shape of the Project.Owner nested
// attribute. Must stay in sync with the Schema in project_resource.go and
// projects_data_source.go.
var projectOwnerAttrTypes = map[string]attr.Type{
	"id":       types.Int64Type,
	"email":    types.StringType,
	"robot_cn": types.StringType,
}

// projectFromAPI converts an hcloudplus.Project into the Terraform state
// model used by hcloudplus_project. It keeps the flat-plus-nested-owner
// layout declared in Schema().
func projectFromAPI(p *hcloudplus.Project) projectResourceModel {
	owner, _ := types.ObjectValue(projectOwnerAttrTypes, map[string]attr.Value{
		"id":       types.Int64Value(p.Owner.ID),
		"email":    types.StringValue(p.Owner.Email),
		"robot_cn": types.StringValue(p.Owner.RobotCN),
	})
	return projectResourceModel{
		ID:      types.StringValue(int64ToString(p.ID)),
		Name:    types.StringValue(p.Name),
		Role:    types.StringValue(p.Role),
		Created: types.StringValue(p.Created.UTC().Format("2006-01-02T15:04:05Z")),
		Owner:   owner,
	}
}

func int64ToString(n int64) string {
	return strconv.FormatInt(n, 10)
}

// parseProjectID converts a project_id attribute to the numeric ID the API
// takes. project_id is a string so it can be wired straight from
// hcloudplus_project.id, which is a string like every Terraform ID; Pulumi
// has no implicit string -> number conversion.
func parseProjectID(v types.String) (int64, error) {
	id, err := strconv.ParseInt(v.ValueString(), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("project_id must be a numeric project ID, got %q", v.ValueString())
	}
	return id, nil
}

// projectIDStringUpgraders upgrades schema v0 state, where project_id was
// a number, to v1 where it is a string.
func projectIDStringUpgraders() map[int64]resource.StateUpgrader {
	return map[int64]resource.StateUpgrader{
		0: {
			StateUpgrader: func(_ context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
				dec := json.NewDecoder(bytes.NewReader(req.RawState.JSON))
				dec.UseNumber()
				var raw map[string]any
				if err := dec.Decode(&raw); err != nil {
					resp.Diagnostics.AddError("Unable to upgrade state", err.Error())
					return
				}
				if n, ok := raw["project_id"].(json.Number); ok {
					raw["project_id"] = n.String()
				}
				b, err := json.Marshal(raw)
				if err != nil {
					resp.Diagnostics.AddError("Unable to upgrade state", err.Error())
					return
				}
				resp.DynamicValue = &tfprotov6.DynamicValue{JSON: b}
			},
		},
	}
}
