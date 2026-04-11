resource "hcloudplus_project" "example" {
  name = "infra-prod"
}

resource "hcloudplus_project_invitation" "teammate" {
  project_id = hcloudplus_project.example.id
  email      = "alice@example.com"
  role       = "admin"

  # Optional: when true, Terraform refreshes see the invite as expired and
  # recreate it on the next plan so the target user gets a fresh email.
  reinvite_on_expiry = true
}
