//go:build tools

// Package tools pins build-time tooling deps so they show up in go.mod
// without polluting the runtime binary. Standard Go convention for
// Terraform providers.
package tools

import (
	_ "github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs"
)
