terraform {
  required_providers {
    hcloudplus = {
      source = "hatemosphere/hcloudplus"
    }
  }
}

# Name suffix for the throwaway project. Override with:
#   terraform apply -var 'suffix=whatever'
# Default uses the current Terraform workspace name so `terraform workspace
# new hcgp-run1` gives you fresh names without editing this file.
variable "suffix" {
  type    = string
  default = ""
}

locals {
  effective_suffix = var.suffix != "" ? var.suffix : terraform.workspace
}

# Auth: two modes supported, pick one — no HCL change needed for either.
#
# Mode A — full cold login from robot credentials + MFA, unattended. The
# provider launches headless Chromium internally (via authbrowser) and
# mints a user token on Configure. Zero manual steps before `terraform
# plan`.
#
#   export HCLOUDPLUS_USERNAME="$(op read op://vault/accounts.hetzner.com/username)"
#   export HCLOUDPLUS_PASSWORD="$(op read op://vault/accounts.hetzner.com/password)"
#   export HCLOUDPLUS_TOTP_CMD="op read 'op://vault/accounts.hetzner.com/one-time password?attribute=totp'"
#
# Mode B — bring your own pre-minted user token (~12h validity, no browser
# needed per run):
#
#   export HCLOUDPLUS_USER_TOKEN=$(go run ../../../authbrowser/cmd/hcgp-login)
#
# In either mode the `provider` block is empty — all config comes from env
# vars. Explicit attributes (user_token, username, password, totp_cmd) are
# also supported inside the block but committing them is a bad idea.
# provider "hcloudplus" {}

# # ---- Data sources (read-only, safe to run on any account) ----

# data "hcloudplus_user" "me" {}

# data "hcloudplus_limits" "account" {}

# data "hcloudplus_projects" "all" {}

# # ---- Smoke-test resources ----

# resource "hcloudplus_project" "smoke" {
#   name = "tf-smoke-${local.effective_suffix}"
# }

# resource "hcloudplus_project_token" "ro" {
#   project_id  = hcloudplus_project.smoke.id
#   description = "tf-smoke-ro"
#   readonly    = true
# }

# resource "hcloudplus_project_token" "rw" {
#   project_id  = hcloudplus_project.smoke.id
#   description = "tf-smoke-rw"
#   readonly    = false
# }

# resource "hcloudplus_project_invitation" "deleteme" {
#   project_id         = hcloudplus_project.smoke.id
#   email              = "tf-smoke-deleteme@example.invalid"
#   role               = "member"
#   reinvite_on_expiry = true
# }

# # ---- Outputs for eyeballing ----

# output "me" {
#   value = data.hcloudplus_user.me
# }

# output "account_limits_server" {
#   value = data.hcloudplus_limits.account.limits["server"]
# }

# output "project_count" {
#   value = length(data.hcloudplus_projects.all.projects)
# }

# output "smoke_project_id" {
#   value = hcloudplus_project.smoke.id
# }

# output "smoke_project_name" {
#   value = hcloudplus_project.smoke.name
# }

# output "ro_token_prefix" {
#   value = hcloudplus_project_token.ro.prefix
# }

# output "rw_token_secret" {
#   value     = hcloudplus_project_token.rw.secret
#   sensitive = true
# }

# output "invitation_id" {
#   value = hcloudplus_project_invitation.deleteme.id
# }
