resource "hcloudplus_project" "example" {
  name = "infra-prod"
}

# Read-write token for use with the upstream hcloud provider.
resource "hcloudplus_project_token" "rw" {
  project_id  = hcloudplus_project.example.id
  description = "pulumi-rw"
  readonly    = false
}

# Hand the secret to the upstream hcloud provider.
provider "hcloud" {
  alias = "infra_prod"
  token = hcloudplus_project_token.rw.secret
}
