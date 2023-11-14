//go:build tools

package tools

import (
	_ "github.com/hashicorp/terraform-plugin-docs"
)

//go:generate go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs
