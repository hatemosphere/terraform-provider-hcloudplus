data "hcloudplus_limits" "account" {}

# Example: expose current usage + quota as outputs for a dashboard.
output "server_quota" {
  value = data.hcloudplus_limits.account.limits["server"]
}

output "primary_ip_quota" {
  value = data.hcloudplus_limits.account.limits["primary_ip"]
}
