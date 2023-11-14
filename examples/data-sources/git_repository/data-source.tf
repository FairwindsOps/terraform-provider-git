data "git_repository" "example_repo" {
  url = "https://example.com/repo-name"
}

output "head_sha" {
  value = data.git_repository.example_repo.head.sha
}

output "branch_names" {
  value = data.git_repository.example_repo.branches.*.name
}

output "branch_shas" {
  value = data.git_repository.example_repo.branches.*.sha
}

output "tag_names" {
  value = data.git_repository.example_repo.tags.*.name
}

output "tag_shas" {
  value = data.git_repository.example_repo.tags.*.sha
}