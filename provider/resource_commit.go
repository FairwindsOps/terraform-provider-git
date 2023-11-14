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
	"github.com/go-git/go-git/v5/storage/memory"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
)

func resourceCommit() *schema.Resource {
	return &schema.Resource{
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
			},
			"branch": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"message": {
				Type:     schema.TypeString,
				Optional: true,
				Default:  "Committed with Terraform",
			},
			"update_message": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"delete_message": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"add": {
				Type:     schema.TypeList,
				Required: true,
				MinItems: 1,
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
				Type:     schema.TypeList,
				Required: false,
				MinItems: 0,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"path": {
							Type:     schema.TypeString,
							Required: true,
						},
					},
				},
			},
			"prune": {
				Type:     schema.TypeBool,
				Optional: true,
				Default:  false,
			},
			"auth": authSchema(),

			"sha": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"new": {
				Type:     schema.TypeBool,
				Computed: true,
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

	// Clone repository
	auth, err := getAuth(d)
	if err != nil {
		return diag.Errorf("failed to prepare authentication: %s", err)
	}

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

		path = worktree.Filesystem.Join(path)

		// Remove the file
		_, err := worktree.Remove(path)
		if err != nil {
			diag.Errorf("failed to remove file %s: %s", path, err)
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
		d.Set("sha", sha.String())
		d.Set("new", false)

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
	d.Set("sha", commitSha.String())
	d.Set("new", true)

	return nil
}

func resourceCommitRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	url := d.Get("url").(string)
	branch := d.Get("branch").(string)
	items := d.Get("add").([]interface{})

	// Clone repository
	auth, err := getAuth(d)
	if err != nil {
		return diag.Errorf("failed to prepare authentication: %s", err)
	}

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
	d.Set("sha", sha.String())
	d.Set("new", false)

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

	// Clone repository
	auth, err := getAuth(d)
	if err != nil {
		return diag.Errorf("failed to prepare authentication: %s", err)
	}

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
		d.Set("sha", sha.String())
		d.Set("new", false)

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
	d.Set("sha", commitSha.String())
	d.Set("new", true)

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

	// Clone repository
	auth, err := getAuth(d)
	if err != nil {
		return diag.Errorf("failed to prepare authentication: %s", err)
	}

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
