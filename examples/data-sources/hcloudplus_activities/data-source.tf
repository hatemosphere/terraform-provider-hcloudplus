data "hcloudplus_activities" "recent_token_ops" {
  include_user_activities = false
  types                   = ["token.create", "token.delete"]
  created_after           = timeadd(timestamp(), "-24h")
}

output "recent_token_events" {
  value = data.hcloudplus_activities.recent_token_ops.activities
}
