A [Terraform](http://terraform.io) provider to manage files in any remote Git repository.

Available on the Terraform registry as [fairwindsops/git](https://registry.terraform.io/providers/fairwindsops/git).

[Documentation](https://registry.terraform.io/providers/FairwindsOps/git/latest/docs)

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