package provider

import (
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func Provider() *schema.Provider {
	return &schema.Provider{
		ResourcesMap: map[string]*schema.Resource{
			"git_commit": resourceCommit(),
		},
		DataSourcesMap: map[string]*schema.Resource{
			"git_repository": dataRepository(),
			"git_file":       dataFile(),
		},
		ConfigureFunc: providerConfigure,
	}
}

func providerConfigure(data *schema.ResourceData) (interface{}, error) {
	config := &providerConfig{}
	return config, nil
}

type providerConfig struct{}
