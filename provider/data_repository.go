package provider

import (
	"context"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/storage/memory"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
)

func dataRepository() *schema.Resource {
	return &schema.Resource{
		Description: "A remote git repository.",
		ReadContext: dataRepositoryRead,
		Schema: map[string]*schema.Schema{
			"url": {
				Description:  "The URL of the git repository. Must be http, https, or ssh.",
				Type:         schema.TypeString,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: validation.IsURLWithScheme([]string{"http", "https", "ssh"}),
			},
			"auth": authSchema(),

			"head": {
				Description: "The head of the git repository.",
				Type:        schema.TypeList,
				Computed:    true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"sha": {
							Type:     schema.TypeString,
							Computed: true,
						},
					},
				},
			},
			"branches": {
				Description: "A list of branches in the remote repository.",
				Type:        schema.TypeList,
				Computed:    true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"name": {
							Type:     schema.TypeString,
							Computed: true,
						},
						"sha": {
							Type:     schema.TypeString,
							Computed: true,
						},
					},
				},
			},
			"tags": {
				Description: "A list of tags in the remote repository.",
				Type:        schema.TypeList,
				Computed:    true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"name": {
							Type:     schema.TypeString,
							Computed: true,
						},
						"sha": {
							Type:     schema.TypeString,
							Computed: true,
						},
					},
				},
			},
		},
	}
}

func dataRepositoryRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	url := d.Get("url").(string)

	// Clone repository
	auth, err := getAuth(d)
	if err != nil {
		return diag.Errorf("failed to prepare authentication: %s", err)
	}

	repo, err := gogit.CloneContext(ctx, memory.NewStorage(), nil, &gogit.CloneOptions{
		URL:  url,
		Auth: auth,
	})
	if err != nil {
		return diag.Errorf("failed to clone repository: %s", err)
	}

	d.SetId(url)

	// Set the HEAD sha output
	head, err := repo.Head()
	if err != nil {
		return diag.Errorf("failed to get HEAD: %s", err)
	}
	d.Set("head", []map[string]string{
		{
			"sha": head.String(),
		},
	})

	// Fetch all remote refs
	remote, err := repo.Remote("origin")
	if err != nil {
		return diag.Errorf("failed to retrieve remote: %s", err)
	}

	refs, err := remote.ListContext(ctx, &gogit.ListOptions{
		Auth: auth,
	})
	if err != nil {
		return diag.Errorf("failed to list remote refs: %s", err)
	}

	// Separate branch and tag refs
	var branchesData []map[string]string
	var tagsData []map[string]string
	for _, branch := range refs {
		if branch.Name().IsBranch() {
			branchesData = append(branchesData, map[string]string{
				"name": branch.Name().String()[len("refs/heads/"):],
				"sha":  branch.Hash().String(),
			})
		} else if branch.Name().IsTag() {
			tagsData = append(tagsData, map[string]string{
				"name": branch.Name().String()[len("refs/tags/"):],
				"sha":  branch.Hash().String(),
			})
		}
	}
	d.Set("branches", branchesData)
	d.Set("tags", tagsData)

	return nil
}
