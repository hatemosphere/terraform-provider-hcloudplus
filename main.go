// Package main is the entry point for terraform-provider-hcloudplus, a
// Terraform provider wrapping hcloud-go-plus for Hetzner Cloud's
// undocumented console API.
package main

import (
	"context"
	"flag"
	"log"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"

	"github.com/hatemosphere/terraform-provider-hcloudplus/provider"
)

// version is injected at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	var debug bool
	flag.BoolVar(&debug, "debug", false, "run as debug server for terraform plugin debugging")
	flag.Parse()

	err := providerserver.Serve(context.Background(), provider.New(version), providerserver.ServeOpts{
		Address: "registry.terraform.io/hatemosphere/hcloudplus",
		Debug:   debug,
	})
	if err != nil {
		log.Fatal(err)
	}
}
