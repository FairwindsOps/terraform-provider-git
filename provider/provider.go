package provider

import (
	"context"
	"os"

	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func Provider() *schema.Provider {
	p := &schema.Provider{
		ResourcesMap: map[string]*schema.Resource{
			"git_commit": resourceCommit(),
		},
		DataSourcesMap: map[string]*schema.Resource{
			"git_repository": dataRepository(),
			"git_file":       dataFile(),
		},
		Schema: map[string]*schema.Schema{
			"github_token": {
				Type:     schema.TypeString,
				Required: false,
			},
		},
	}
	p.ConfigureContextFunc = configure(p)
	return p
}

type apiClient struct {
	// Add whatever fields, client or connection info, etc. here
	// you would need to setup to communicate with the upstream
	// API.
	auth transport.AuthMethod
}

func configure(p *schema.Provider) func(context.Context, *schema.ResourceData) (any, diag.Diagnostics) {
	return func(_ context.Context, d *schema.ResourceData) (any, diag.Diagnostics) {
		// default to environment variable and fall back to a token passed in via the provider config
		token := os.Getenv("GITHUB_TOKEN")

		if token == "" {
			token = d.Get("github_token").(string)
		}

		return &http.TokenAuth{
			Token: token,
		}, nil
	}
}
