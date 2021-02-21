package git

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/helper/schema"
	"github.com/microsoft/azure-devops-go-api/azuredevops/git"
	"github.com/microsoft/terraform-provider-azuredevops/azuredevops/internal/client"
	"github.com/microsoft/terraform-provider-azuredevops/azuredevops/internal/utils"
	"github.com/microsoft/terraform-provider-azuredevops/azuredevops/internal/utils/converter"
)

func ResourceGitRepositoryFile() *schema.Resource {
	return &schema.Resource{
		Create: resourceGitRepositoryFileCreate,
		Read:   resourceGitRepositoryFileRead,
		Update: resourceGitRepositoryFileUpdate,
		Delete: resourceGitRepositoryFileDelete,
		Importer: &schema.ResourceImporter{
			State: func(d *schema.ResourceData, m interface{}) ([]*schema.ResourceData, error) {
				parts := strings.Split(d.Id(), ":")
				branch := "refs/heads/master"

				if len(parts) > 2 {
					return nil, fmt.Errorf("Invalid ID specified. Supplied ID must be written as <repository>/<file path> (when branch is \"master\") or <repository>/<file path>:<branch>")
				}

				if len(parts) == 2 {
					branch = parts[1]
				}

				clients := m.(*client.AggregatedClient)
				repo, file := splitRepoFilePath(parts[0])
				if err := checkRepositoryFileExists(clients, repo, file, branch); err != nil {
					return nil, err
				}

				d.SetId(fmt.Sprintf("%s/%s", repo, file))
				d.Set("branch", branch)
				d.Set("overwrite_on_create", false)

				return []*schema.ResourceData{d}, nil
			},
		},

		Schema: map[string]*schema.Schema{
			"repository_id": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "The repository name",
			},
			"file": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "The file path to manage",
			},
			"content": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "The file's content",
			},
			"branch": {
				Type:        schema.TypeString,
				Optional:    true,
				ForceNew:    true,
				Description: "The branch name, defaults to \"master\"",
				Default:     "refs/heads/master",
			},
			"comment": {
				Type:        schema.TypeString,
				Optional:    true,
				Computed:    true,
				Description: "The commit message when creating or updating the file",
			},
			"overwrite_on_create": {
				Type:        schema.TypeBool,
				Optional:    true,
				Description: "Enable overwriting existing files, defaults to \"false\"",
				Default:     false,
			},
		},
		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(1 * time.Minute),
			Read:   schema.DefaultTimeout(5 * time.Second),
		},
	}
}

func resourceGitRepositoryPushArgs(d *schema.ResourceData, objectID string, changeType git.VersionControlChangeType, newContent *git.ItemContent) (*git.CreatePushArgs, error) {
	var message *string
	if commitMessage, hasCommitMessage := d.GetOk("comment"); hasCommitMessage {
		cm := commitMessage.(string)
		message = &cm
	}

	repo := d.Get("repository_id").(string)
	file := d.Get("file").(string)
	branch := d.Get("branch").(string)

	change := git.GitChange{
		ChangeType: &changeType,
		Item: git.GitItem{
			Path: &file,
		},
	}
	if newContent != nil {
		change.NewContent = newContent
	}

	args := &git.CreatePushArgs{
		RepositoryId: &repo,
		Push: &git.GitPush{
			RefUpdates: &[]git.GitRefUpdate{
				{
					Name:        &branch,
					OldObjectId: &objectID,
				},
			},
			Commits: &[]git.GitCommitRef{
				{
					Comment: message,
					Changes: &[]interface{}{change},
				},
			},
		},
	}

	return args, nil
}

func resourceGitRepositoryFileCreate(d *schema.ResourceData, m interface{}) error {
	ctx := context.Background()
	clients := m.(*client.AggregatedClient)

	repo := d.Get("repository_id").(string)
	file := d.Get("file").(string)
	branch := d.Get("branch").(string)
	overwriteOnCreate := d.Get("overwrite_on_create").(bool)

	if err := checkRepositoryBranchExists(clients, repo, branch); err != nil {
		return err
	}

	changeType := git.VersionControlChangeTypeValues.Add
	item, err := clients.GitReposClient.GetItem(ctx, git.GetItemArgs{
		RepositoryId: &repo,
		Path:         &file,
	})
	if err != nil && !utils.ResponseWasNotFound(err) {
		return err
	}

	if item != nil {
		if !overwriteOnCreate {
			return fmt.Errorf("Refusing to overwrite existing file. Configure `overwrite_on_create` to `true` to override.")
		} else {
			changeType = git.VersionControlChangeTypeValues.Edit
		}
	}

	content := d.Get("content").(string)
	newContent := &git.ItemContent{
		Content:     &content,
		ContentType: &git.ItemContentTypeValues.RawText,
	}

	err = waitForFilePush(clients, d, &repo, &branch, &file, changeType, newContent)
	if err != nil {
		return err
	}

	d.SetId(fmt.Sprintf("%s/%s", repo, file))
	return resourceGitRepositoryFileRead(d, m)
}

func resourceGitRepositoryFileRead(d *schema.ResourceData, m interface{}) error {
	ctx := context.Background()
	clients := m.(*client.AggregatedClient)

	repo, file := splitRepoFilePath(d.Id())
	branch := d.Get("branch").(string)

	if err := checkRepositoryBranchExists(clients, repo, branch); err != nil {
		return err
	}

	return resource.Retry(d.Timeout(schema.TimeoutRead), func() *resource.RetryError {
		branch = strings.TrimPrefix(branch, "refs/heads/")
		item, err := clients.GitReposClient.GetItem(ctx, git.GetItemArgs{
			RepositoryId:   &repo,
			Path:           &file,
			IncludeContent: converter.Bool(true),
			VersionDescriptor: &git.GitVersionDescriptor{
				Version:     &branch,
				VersionType: &git.GitVersionTypeValues.Branch,
			},
		})
		if err != nil {
			if utils.ResponseWasNotFound(err) {
				d.SetId("")
				return resource.NonRetryableError(err)
			}
			return resource.NonRetryableError(err)
		}

		d.Set("content", item.Content)
		d.Set("repository_id", repo)
		d.Set("file", file)

		commit, err := clients.GitReposClient.GetCommit(ctx, git.GetCommitArgs{
			RepositoryId: &repo,
			CommitId:     item.CommitId,
		})
		if err != nil {
			return resource.NonRetryableError(err)
		}

		d.Set("comment", commit.Comment)

		return nil
	})
}

func resourceGitRepositoryFileUpdate(d *schema.ResourceData, m interface{}) error {
	clients := m.(*client.AggregatedClient)
	ctx := context.Background()

	repo := d.Get("repository_id").(string)
	file := d.Get("file").(string)
	branch := d.Get("branch").(string)

	if err := checkRepositoryBranchExists(clients, repo, branch); err != nil {
		return err
	}

	objectID, err := getLastCommitId(clients, repo, branch)
	if err != nil {
		return err
	}

	content := d.Get("content").(string)
	newContent := &git.ItemContent{
		Content:     &content,
		ContentType: &git.ItemContentTypeValues.RawText,
	}

	args, err := resourceGitRepositoryPushArgs(d, objectID, git.VersionControlChangeTypeValues.Edit, newContent)
	if err != nil {
		return err
	}

	if *(*args.Push.Commits)[0].Comment == fmt.Sprintf("Add %s", file) {
		m := fmt.Sprintf("Update %s", file)
		(*args.Push.Commits)[0].Comment = &m
	}

	_, err = clients.GitReposClient.CreatePush(ctx, *args)
	if err != nil {
		return err
	}

	return resourceGitRepositoryFileRead(d, m)
}

func resourceGitRepositoryFileDelete(d *schema.ResourceData, m interface{}) error {
	clients := m.(*client.AggregatedClient)

	repo := d.Get("repository_id").(string)
	file := d.Get("file").(string)
	branch := d.Get("branch").(string)

	err := waitForFilePush(clients, d, &repo, &branch, &file, git.VersionControlChangeTypeValues.Delete, nil)
	if err != nil {
		return err
	}

	return nil
}

// waitForFilePush watches an object (repository file) and waits for it to achieve the desired state
func waitForFilePush(clients *client.AggregatedClient, d *schema.ResourceData, repo *string, branch *string, file *string, changeType git.VersionControlChangeType, newContent *git.ItemContent) error {
	ctx := context.Background()

	stateConf := &resource.StateChangeConf{
		Pending: []string{"Waiting"},
		Target:  []string{"Synched"},
		Refresh: func() (interface{}, string, error) {
			state := "Waiting"
			objectID, err := getLastCommitId(clients, *repo, *branch)
			if err != nil {
				return state, state, err
			}

			args, err := resourceGitRepositoryPushArgs(d, objectID, changeType, newContent)
			if err != nil {
				return state, state, err
			}

			if (*args.Push.Commits)[0].Comment == nil {
				m := fmt.Sprintf("%s %s", changeType, *file)
				(*args.Push.Commits)[0].Comment = &m
			}

			push, err := clients.GitReposClient.CreatePush(ctx, *args)
			if err != nil {
				if utils.ResponseContainsStatusMessage(err, "has already been updated by another client") {
					return state, state, nil // return no error here (nil) indicating we want to retry the 'push'
				} else {
					return state, state, err
				}
			}

			if *push.PushId > 0 {
				state = "Synched"
			}

			return state, state, nil
		},
		Timeout:                   600 * time.Second,
		MinTimeout:                2 * time.Second,
		Delay:                     0 * time.Second,
		ContinuousTargetOccurence: 1,
	}
	if _, err := stateConf.WaitForState(); err != nil {
		return fmt.Errorf("Error retrieving expected branch for repository [%s]: %+v", *repo, err)
	}
	return nil
}

// checkRepositoryBranchExists tests if a branch exists in a repository.
func checkRepositoryBranchExists(c *client.AggregatedClient, repo, branch string) error {
	branch = strings.TrimPrefix(branch, "refs/heads/")
	ctx := context.Background()
	_, err := c.GitReposClient.GetBranch(ctx, git.GetBranchArgs{
		RepositoryId: &repo,
		Name:         &branch,
	})
	return err
}

// checkRepositoryFileExists tests if a file exists in a repository.
func checkRepositoryFileExists(c *client.AggregatedClient, repo, file, branch string) error {
	branch = strings.TrimPrefix(branch, "refs/heads/")
	ctx := context.Background()
	_, err := c.GitReposClient.GetItem(ctx, git.GetItemArgs{
		RepositoryId: &repo,
		Path:         &file,
		VersionDescriptor: &git.GitVersionDescriptor{
			Version: &branch,
		},
	})
	if err != nil {
		return err
	}

	return nil
}

// getLastCommitId gets the last commit on a repository and branch.
func getLastCommitId(c *client.AggregatedClient, repo, branch string) (string, error) {
	branch = strings.TrimPrefix(branch, "refs/heads/")
	ctx := context.Background()
	commits, err := c.GitReposClient.GetCommits(ctx, git.GetCommitsArgs{
		RepositoryId: &repo,
		Top:          converter.Int(1),
		SearchCriteria: &git.GitQueryCommitsCriteria{
			ItemVersion: &git.GitVersionDescriptor{
				Version: &branch,
			},
		},
	})
	if err != nil {
		return "", err
	}
	return *(*commits)[0].CommitId, nil
}

// splitRepoFilePath splits a path into 2 parts.
func splitRepoFilePath(path string) (string, string) {
	parts := strings.Split(path, "/")
	return parts[0], strings.Join(parts[1:], "/")
}
