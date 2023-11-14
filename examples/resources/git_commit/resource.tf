resource "git_commit" "example_write" {
  url            = "https://example.com/repo-name"
  branch         = "main"
  message        = "Create txt and JSON files"
  update_message = "Update txt and JSON files"
  delete_message = "Delete txt and JSON files"

  add {
    path    = "path/to/file.txt"
    content = "Hello, World!"
  }

  add {
    path    = "path/to/file.json"
    content = jsonencode({ hello = "world" })
  }

  prune = true
}

output "commit_sha" {
  value = git_commit.example_write.sha
}

output "is_new" {
  value = git_commit.example_write.new
}