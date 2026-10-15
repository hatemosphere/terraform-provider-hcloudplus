package provider

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
)

func TestProjectIDStringUpgrader(t *testing.T) {
	req := resource.UpgradeStateRequest{RawState: &tfprotov6.RawState{
		JSON: []byte(`{"id":"1181754","project_id":16404333,"email":"a@b","role":"member"}`),
	}}
	var resp resource.UpgradeStateResponse
	projectIDStringUpgraders()[0].StateUpgrader(context.Background(), req, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	var got map[string]any
	if err := json.Unmarshal(resp.DynamicValue.JSON, &got); err != nil {
		t.Fatal(err)
	}
	if got["project_id"] != "16404333" {
		t.Errorf("project_id = %#v, want \"16404333\"", got["project_id"])
	}
	if got["id"] != "1181754" || got["email"] != "a@b" {
		t.Errorf("other attributes changed: %v", got)
	}
}
