data "git_file" "example_read" {
  url  = "https://example.com/repo-name"
  ref  = "v1.0.0"
  path = "path/to/file.txt"
}

output "file_content" {
  value = data.git_file.example_read.content
}