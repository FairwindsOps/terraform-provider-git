package provider

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/go-git/go-billy/v5/memfs"
	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/format/index"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/go-git/go-git/v5/storage/memory"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
)

func resourceCommit() *schema.Resource {
	return &schema.Resource{
		Description:   "A resource to create a git commit with one or more files or removals.",
		CreateContext: resourceCommitCreate,
		ReadContext:   resourceCommitRead,
		UpdateContext: resourceCommitUpdate,
		DeleteContext: resourceCommitDelete,

		Schema: map[string]*schema.Schema{
			"url": {
				Type:         schema.TypeString,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: validation.IsURLWithScheme([]string{"http", "https", "ssh"}),
				Description:  "The URL of the git repository. Must be http, https, or ssh.",
			},
			"branch": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "The git branch to commit to.",
			},
			"message": {
				Type:        schema.TypeString,
				Optional:    true,
				Default:     "Committed with Terraform",
				Description: "The git commit message.",
			},
			"update_message": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "The commit message to use on update.",
			},
			"delete_message": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "The commit message to use on delete.",
			},
			"add": {
				Description: "A file to add. Contains a path and the file content.",
				Type:        schema.TypeList,
				Optional:    true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"path": {
							Type:     schema.TypeString,
							Required: true,
						},
						"content": {
							Type:     schema.TypeString,
							Required: true,
						},
					},
				},
			},
			"remove": {
				Description: "A file to remove. Contains the file path.",
				Type:        schema.TypeList,
				Optional:    true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"path": {
							Type:     schema.TypeString,
							Required: true,
						},
						"recursive": {
							Type:     schema.TypeBool,
							Required: false,
							Default:  false,
						},
					},
				},
			},
			"prune": {
				Type:     schema.TypeBool,
				Optional: true,
				Default:  false,
			},
			"sha": {
				Description: "The git sha of the commit.",
				Type:        schema.TypeString,
				Computed:    true,
			},
			"new": {
				Description: "A boolean to indicate if the commit is newly created.",
				Type:        schema.TypeBool,
				Computed:    true,
			},
		},
	}
}

func resourceCommitCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	url := d.Get("url").(string)
	branch := d.Get("branch").(string)
	message := d.Get("message").(string)
	addItems := d.Get("add").([]interface{})
	removeItems := d.Get("remove").([]interface{})

	auth := meta.(*http.BasicAuth)

	repo, err := gogit.CloneContext(ctx, memory.NewStorage(), memfs.New(), &gogit.CloneOptions{
		URL:  url,
		Auth: auth,
	})
	if err != nil {
		return diag.Errorf("failed to clone repository: %s", err)
	}

	// Get the current worktree
	worktree, err := repo.Worktree()
	if err != nil {
		return diag.Errorf("failed to get worktree: %s", err)
	}

	// Resolve then checkout the specified branch
	sha, err := repo.ResolveRevision(plumbing.Revision(plumbing.NewRemoteReferenceName("origin", branch)))
	if err != nil && errors.Is(err, plumbing.ErrReferenceNotFound) {
		sha, err = repo.ResolveRevision(plumbing.Revision(plumbing.NewBranchReferenceName(branch)))
	}
	if err != nil {
		return diag.Errorf("failed to resolve branch %s: %s", branch, err)
	}

	err = worktree.Checkout(&gogit.CheckoutOptions{
		Hash:  *sha,
		Force: true,
	})
	if err != nil {
		return diag.Errorf("failed to checkout hash %s: %s", sha.String(), err)
	}

	// Write files
	for _, item := range addItems {
		path := item.(map[string]interface{})["path"].(string)
		content := item.(map[string]interface{})["content"].(string)

		path = worktree.Filesystem.Join(path)

		// Create, write then close file
		file, err := worktree.Filesystem.Create(path)
		if err != nil {
			return diag.Errorf("failed to create file %s: %s", path, err)
		}

		_, err = io.WriteString(file, content)
		if err != nil {
			return diag.Errorf("failed to write to file %s: %s", path, err)
		}

		err = file.Close()
		if err != nil {
			return diag.Errorf("failed to close file %s: %s", path, err)
		}
	}

	// Remove files
	for _, item := range removeItems {
		path := item.(map[string]interface{})["path"].(string)
		recursive := item.(map[string]interface{})["recursive"].(bool)

		path = worktree.Filesystem.Join(path)

		// Remove the file
		if recursive {
			err := worktree.RemoveGlob(path)
			if err != nil {
				diag.Errorf("failed to remove file recursively %s: %s", path, err)
			}
		} else {
			_, err := worktree.Remove(path)
			if err != nil {
				diag.Errorf("failed to remove file %s: %s", path, err)
			}
		}

	}

	// Check if worktree is clean
	status, err := worktree.Status()
	if err != nil {
		return diag.Errorf("failed to compute worktree status: %s", err)
	}
	if status.IsClean() {
		sha, err := repo.ResolveRevision(plumbing.Revision(plumbing.HEAD))
		if err != nil {
			return diag.Errorf("failed to get existing commit: %s", err)
		}

		d.SetId(sha.String())
		if err := d.Set("sha", sha.String()); err != nil {
			return diag.Errorf("failed to set sha: %s", err)
		}
		if err := d.Set("new", false); err != nil {
			return diag.Errorf("failed to set new: %s", err)
		}

		return nil
	}

	// Stage worktree
	err = worktree.AddWithOptions(&gogit.AddOptions{
		All: true,
	})
	if err != nil {
		return diag.Errorf("failed to stage worktree: %s", err)
	}

	// Commit
	commitSha, err := worktree.Commit(message, &gogit.CommitOptions{})
	if err != nil {
		return diag.Errorf("failed to commit: %s", err)
	}

	// Update branch
	branchRef := plumbing.NewBranchReferenceName(branch)
	hashRef := plumbing.NewHashReference(branchRef, commitSha)
	err = repo.Storer.SetReference(hashRef)
	if err != nil {
		return diag.Errorf("failed to set branch ref: %s", err)
	}

	// Push
	err = repo.PushContext(ctx, &gogit.PushOptions{
		RefSpecs: []config.RefSpec{
			config.RefSpec(fmt.Sprintf("%s:%s", branchRef, branchRef)),
		},
		Auth: auth,
	})
	if err != nil {
		return diag.Errorf("failed to push: %s", err)
	}

	d.SetId(commitSha.String())
	if err := d.Set("sha", commitSha.String()); err != nil {
		return diag.Errorf("error setting sha: %s", err)
	}
	if err := d.Set("new", true); err != nil {
		return diag.Errorf("error setting new: %s", err)
	}

	return nil
}

func resourceCommitRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	url := d.Get("url").(string)
	branch := d.Get("branch").(string)
	items := d.Get("add").([]interface{})

	auth := meta.(*http.BasicAuth)

	repo, err := gogit.CloneContext(ctx, memory.NewStorage(), memfs.New(), &gogit.CloneOptions{
		URL:  url,
		Auth: auth,
	})
	if err != nil {
		return diag.Errorf("failed to clone repository: %s", err)
	}

	// Get the current worktree
	worktree, err := repo.Worktree()
	if err != nil {
		return diag.Errorf("failed to get worktree: %s", err)
	}

	// Resolve then checkout the specified branch
	sha, err := repo.ResolveRevision(plumbing.Revision(plumbing.NewRemoteReferenceName("origin", branch)))
	if err != nil && errors.Is(err, plumbing.ErrReferenceNotFound) {
		sha, err = repo.ResolveRevision(plumbing.Revision(plumbing.NewBranchReferenceName(branch)))
	}
	if err != nil {
		return diag.Errorf("failed to resolve branch %s: %s", branch, err)
	}

	err = worktree.Checkout(&gogit.CheckoutOptions{
		Hash:  *sha,
		Force: true,
	})
	if err != nil {
		return diag.Errorf("failed to checkout hash %s: %s", sha.String(), err)
	}

	// Write files
	for _, item := range items {
		path := item.(map[string]interface{})["path"].(string)
		content := item.(map[string]interface{})["content"].(string)

		path = worktree.Filesystem.Join(path)

		// Create, write then close file
		file, err := worktree.Filesystem.Create(path)
		if err != nil {
			return diag.Errorf("failed to create file %s: %s", path, err)
		}

		_, err = io.WriteString(file, content)
		if err != nil {
			return diag.Errorf("failed to write to file %s: %s", path, err)
		}

		err = file.Close()
		if err != nil {
			return diag.Errorf("failed to close file %s: %s", path, err)
		}
	}

	// Check if worktree is clean
	status, err := worktree.Status()
	if err != nil {
		return diag.Errorf("failed to compute worktree status: %s", err)
	}
	if !status.IsClean() {
		d.SetId("")
		return nil
	}

	d.SetId(sha.String())
	if err := d.Set("sha", sha.String()); err != nil {
		return diag.Errorf("failed to set sha: %s", err)
	}
	if err := d.Set("new", false); err != nil {
		return diag.Errorf("failed to set new: %s", err)
	}

	return nil
}

func resourceCommitUpdate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	url := d.Get("url").(string)
	branch := d.Get("branch").(string)
	message := d.Get("message").(string)
	items := d.Get("add").([]interface{})
	prune := d.Get("prune").(bool)

	if updateMessage, ok := d.GetOk("update_message"); ok {
		message = updateMessage.(string)
	}

	auth := meta.(*http.BasicAuth)

	repo, err := gogit.CloneContext(ctx, memory.NewStorage(), memfs.New(), &gogit.CloneOptions{
		URL:  url,
		Auth: auth,
	})
	if err != nil {
		return diag.Errorf("failed to clone repository: %s", err)
	}

	// Get the current worktree
	worktree, err := repo.Worktree()
	if err != nil {
		return diag.Errorf("failed to get worktree: %s", err)
	}

	// Resolve then checkout the specified branch
	sha, err := repo.ResolveRevision(plumbing.Revision(plumbing.NewRemoteReferenceName("origin", branch)))
	if err != nil && errors.Is(err, plumbing.ErrReferenceNotFound) {
		sha, err = repo.ResolveRevision(plumbing.Revision(plumbing.NewBranchReferenceName(branch)))
	}
	if err != nil {
		return diag.Errorf("failed to resolve branch %s: %s", branch, err)
	}

	err = worktree.Checkout(&gogit.CheckoutOptions{
		Hash:  *sha,
		Force: true,
	})
	if err != nil {
		return diag.Errorf("failed to checkout hash %s: %s", sha.String(), err)
	}

	// Prune files
	if prune && d.HasChange("add") {
		oldItems, _ := d.GetChange("add")

		for _, item := range oldItems.([]interface{}) {
			path := item.(map[string]interface{})["path"].(string)
			path = worktree.Filesystem.Join(path)

			// Delete old files
			_, err = worktree.Remove(path)
			if err != nil && !errors.Is(err, index.ErrEntryNotFound) {
				return diag.Errorf("failed to delete file %s: %s", path, err)
			}
		}
	}

	// Write files
	for _, item := range items {
		path := item.(map[string]interface{})["path"].(string)
		content := item.(map[string]interface{})["content"].(string)

		path = worktree.Filesystem.Join(path)

		// Create, write then close file
		file, err := worktree.Filesystem.Create(path)
		if err != nil {
			return diag.Errorf("failed to create file %s: %s", path, err)
		}

		_, err = io.WriteString(file, content)
		if err != nil {
			return diag.Errorf("failed to write to file %s: %s", path, err)
		}

		err = file.Close()
		if err != nil {
			return diag.Errorf("failed to close file %s: %s", path, err)
		}
	}

	// Check if worktree is clean
	status, err := worktree.Status()
	if err != nil {
		return diag.Errorf("failed to compute worktree status: %s", err)
	}
	if status.IsClean() {
		sha, err := repo.ResolveRevision(plumbing.Revision(plumbing.HEAD))
		if err != nil {
			return diag.Errorf("failed to get existing commit: %s", err)
		}

		d.SetId(sha.String())
		if err := d.Set("sha", sha.String()); err != nil {
			return diag.Errorf("failed to set sha: %s", err)
		}
		if err := d.Set("new", false); err != nil {
			return diag.Errorf("failed to set new: %s", err)
		}

		return nil
	}

	// Stage worktree
	err = worktree.AddWithOptions(&gogit.AddOptions{
		All: true,
	})
	if err != nil {
		return diag.Errorf("failed to stage worktree: %s", err)
	}

	// Commit
	commitSha, err := worktree.Commit(message, &gogit.CommitOptions{})
	if err != nil {
		return diag.Errorf("failed to commit: %s", err)
	}

	// Update branch
	branchRef := plumbing.NewBranchReferenceName(branch)
	hashRef := plumbing.NewHashReference(branchRef, commitSha)
	err = repo.Storer.SetReference(hashRef)
	if err != nil {
		return diag.Errorf("failed to set branch ref: %s", err)
	}

	// Push
	err = repo.PushContext(ctx, &gogit.PushOptions{
		RefSpecs: []config.RefSpec{
			config.RefSpec(fmt.Sprintf("%s:%s", branchRef, branchRef)),
		},
		Auth: auth,
	})
	if err != nil {
		return diag.Errorf("failed to push: %s", err)
	}

	d.SetId(sha.String())
	if err := d.Set("sha", commitSha.String()); err != nil {
		return diag.Errorf("failed to set sha: %s", err)
	}
	if err := d.Set("new", true); err != nil {
		return diag.Errorf("failed to set new: %s", err)
	}

	return nil
}

func resourceCommitDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	url := d.Get("url").(string)
	branch := d.Get("branch").(string)
	message := d.Get("message").(string)
	items := d.Get("add").([]interface{})
	prune := d.Get("prune").(bool)

	if deleteMessage, ok := d.GetOk("delete_message"); ok {
		message = deleteMessage.(string)
	} else if updateMessage, ok := d.GetOk("update_message"); ok {
		message = updateMessage.(string)
	}
	auth := meta.(*http.BasicAuth)

	repo, err := gogit.CloneContext(ctx, memory.NewStorage(), memfs.New(), &gogit.CloneOptions{
		URL:  url,
		Auth: auth,
	})
	if err != nil {
		return diag.Errorf("failed to clone repository: %s", err)
	}

	// Get the current worktree
	worktree, err := repo.Worktree()
	if err != nil {
		return diag.Errorf("failed to get worktree: %s", err)
	}

	// Resolve then checkout the specified branch
	sha, err := repo.ResolveRevision(plumbing.Revision(plumbing.NewRemoteReferenceName("origin", branch)))
	if err != nil && errors.Is(err, plumbing.ErrReferenceNotFound) {
		sha, err = repo.ResolveRevision(plumbing.Revision(plumbing.NewBranchReferenceName(branch)))
	}
	if err != nil {
		return diag.Errorf("failed to resolve branch %s: %s", branch, err)
	}

	err = worktree.Checkout(&gogit.CheckoutOptions{
		Hash:  *sha,
		Force: true,
	})
	if err != nil {
		return diag.Errorf("failed to checkout hash %s: %s", sha.String(), err)
	}

	// Prune files
	if prune {
		for _, item := range items {
			path := item.(map[string]interface{})["path"].(string)
			path = worktree.Filesystem.Join(path)

			// Delete all files
			_, err = worktree.Remove(path)
			if err != nil && !errors.Is(err, index.ErrEntryNotFound) {
				return diag.Errorf("failed to delete file %s: %s", path, err)
			}
		}
	}

	// Check if worktree is clean
	status, err := worktree.Status()
	if err != nil {
		return diag.Errorf("failed to compute worktree status: %s", err)
	}
	if status.IsClean() {
		return nil
	}

	// Stage worktree
	err = worktree.AddWithOptions(&gogit.AddOptions{
		All: true,
	})
	if err != nil {
		return diag.Errorf("failed to stage worktree: %s", err)
	}

	// Commit
	commitSha, err := worktree.Commit(message, &gogit.CommitOptions{})
	if err != nil {
		return diag.Errorf("failed to commit: %s", err)
	}

	// Update branch
	branchRef := plumbing.NewBranchReferenceName(branch)
	hashRef := plumbing.NewHashReference(branchRef, commitSha)
	err = repo.Storer.SetReference(hashRef)
	if err != nil {
		return diag.Errorf("failed to set branch ref: %s", err)
	}

	// Push
	err = repo.PushContext(ctx, &gogit.PushOptions{
		RefSpecs: []config.RefSpec{
			config.RefSpec(fmt.Sprintf("%s:%s", branchRef, branchRef)),
		},
		Auth: auth,
	})
	if err != nil {
		return diag.Errorf("failed to push: %s", err)
	}

	return nil
}
