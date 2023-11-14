package main

import (
	"github.com/fairwindsops/terraform-provider-git/provider"
	"github.com/hashicorp/terraform-plugin-sdk/v2/plugin"
)

//go:generate go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs
//go:generate terraform fmt -recursive examples/

func main() {
	plugin.Serve(&plugin.ServeOpts{
		ProviderFunc: provider.Provider,
	})
}
