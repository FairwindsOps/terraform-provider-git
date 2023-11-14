A [Terraform](http://terraform.io) provider to manage files in any remote Git repository.

Available on the Terraform registry as [fairwindsops/git](https://registry.terraform.io/providers/fairwindsops/git).

[Documentation](https://registry.terraform.io/providers/FairwindsOps/git/latest/docs)

## Installation

```hcl
terraform {
  required_providers {
    git = {
      source  = "fairwindsops/git"
      version = "~> 0.3"
    }
  }
}
```

Then, run `terraform init`.

## Data Sources

### git_repository
```hcl
# Read metadata of a Git repository
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
```

### git_file
```hcl
# Read an existing file in a Git repository
data "git_file" "example_read" {
  url  = "https://example.com/repo-name"
  ref  = "v1.0.0"
  path = "path/to/file.txt"
}

output "file_content" {
  value = data.git_file.example_read.content
}
```

## Authentication

The `auth` block is supported on all data sources and resources.

### HTTP Bearer

```hcl
# Write to a list of files within a Git repository, then commit and push the changes
resource "git_commit" "example_write" {
  # ...

  auth {
    bearer {
      token = "example_token_123"
    }
  }
}
```

### HTTP Basic

```hcl
# Write to a list of files within a Git repository, then commit and push the changes
resource "git_commit" "example_write" {
  # ...

  auth {
    basic {
      username = "example"
      password = "123"
    }
  }
}
```

### SSH (from file)

```hcl
# Write to a list of files within a Git repository, then commit and push the changes
resource "git_commit" "example_write" {
  # ...

  auth {
    ssh_key {
      username         = "example"
      private_key_path = "/home/user/.ssh/id_rsa"
      password         = "key_passphrase_123"
      known_hosts      = [ "github.com ecdsa-sha2-nistp256 AAAA...=" ]
    }
  }
}
```

### SSH (inline)

```hcl
# Write to a list of files within a Git repository, then commit and push the changes
resource "git_commit" "example_write" {
  # ...

  auth {
    ssh_key {
      username = "example"
      private_key_pem = <<-EOT
      -----BEGIN RSA PRIVATE KEY-----
      ...
      -----END RSA PRIVATE KEY-----
      EOT
      password    = "key_passphrase_123"
      known_hosts = [ "github.com ecdsa-sha2-nistp256 AAAA...=" ]
    }
  }
}
```

## License

Licensed under the Apache License, Version 2.0.\
See the included LICENSE file for more details.
