data "hcloudplus_user" "me" {}

output "caller_email" {
  value = data.hcloudplus_user.me.email
}
