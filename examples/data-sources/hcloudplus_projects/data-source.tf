data "hcloudplus_projects" "all" {}

output "all_projects" {
  value = data.hcloudplus_projects.all.projects
}

output "first_project_name" {
  value = data.hcloudplus_projects.all.projects[0].name
}
