package provider

import (
	"context"
	"os"

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
				Optional: true,
			},
		},
	}
	p.ConfigureContextFunc = configure(p)
	return p
}

func configure(p *schema.Provider) func(context.Context, *schema.ResourceData) (any, diag.Diagnostics) {
	return func(_ context.Context, d *schema.ResourceData) (any, diag.Diagnostics) {
		// default to environment variable and fall back to a token passed in via the provider config
		token := os.Getenv("GITHUB_TOKEN")

		if token == "" {
			token = d.Get("github_token").(string)
		}

		if token == "" {
			return nil, diag.Errorf("empty github token")
		}

		return &http.BasicAuth{
			Username: "anyuser",
			Password: token,
		}, nil
	}
}
