terraform {
  required_providers {
    hcloudplus = {
      source = "hatemosphere/hcloudplus"
    }
  }
}

# All auth is driven by HCLOUDPLUS_* environment variables by default.
# See the provider README for the full list.
provider "hcloudplus" {}
