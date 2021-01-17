// +build all git resource_git_repository_file
// +build !exclude_git !exclude_resource_git_repository_file

package git

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/microsoft/terraform-provider-azuredevops/azuredevops/internal/utils/converter"
	"github.com/stretchr/testify/require"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/hashicorp/terraform-plugin-sdk/helper/schema"
	"github.com/microsoft/azure-devops-go-api/azuredevops/git"
	"github.com/microsoft/terraform-provider-azuredevops/azdosdkmocks"
	"github.com/microsoft/terraform-provider-azuredevops/azuredevops/internal/client"
)

var testRepositoryID = uuid.New()
var testCommitID = uuid.New()

// verifies that the create operation is considered failed if the initial API
// call fails.
func TestGitRepoFile_Create_DoesNotSwallowErrorFromFailedPushCall(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	resourceData := schema.TestResourceDataRaw(t, ResourceGitRepositoryFile().Schema, nil)
	configureResourceGitRepositoryFile(resourceData)
	reposClient := azdosdkmocks.NewMockGitClient(ctrl)
	clients := &client.AggregatedClient{GitReposClient: reposClient, Ctx: context.Background()}
	mockReposClientGetCommits(reposClient)
	mockReposClientGetBranch(reposClient)
	mockReposClientGetItem(reposClient, gomock.Any(), nil)
	expectedArgs, _ := resourceGitRepositoryPushArgs(
		resourceData,
		testCommitID.String(),
		git.VersionControlChangeTypeValues.Add)
	reposClient.
		EXPECT().
		CreatePush(gomock.Any(), gomock.Eq(*expectedArgs)).
		Return(nil, errors.New("CreateGitRepositoryFile() Failed")).
		Times(1)

	err := resourceGitRepositoryFileCreate(resourceData, clients)

	require.Regexp(t, ".*CreateGitRepositoryFile\\(\\) Failed$", err.Error())
}

// verifies that the update operation is considered failed if the initial API
// call fails.
func TestGitRepoFile_Update_DoesNotSwallowErrorFromFailedPushCall(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	resourceData := schema.TestResourceDataRaw(t, ResourceGitRepositoryFile().Schema, nil)
	configureResourceGitRepositoryFile(resourceData)
	reposClient := azdosdkmocks.NewMockGitClient(ctrl)
	clients := &client.AggregatedClient{GitReposClient: reposClient, Ctx: context.Background()}
	mockReposClientGetCommits(reposClient)
	mockReposClientGetBranch(reposClient)
	expectedArgs, _ := resourceGitRepositoryPushArgs(
		resourceData,
		testCommitID.String(),
		git.VersionControlChangeTypeValues.Edit)
	reposClient.
		EXPECT().
		CreatePush(gomock.Any(), gomock.Eq(*expectedArgs)).
		Return(nil, errors.New("UpdateGitRepositoryFile() Failed")).
		Times(1)

	err := resourceGitRepositoryFileUpdate(resourceData, clients)

	require.Regexp(t, ".*UpdateGitRepositoryFile\\(\\) Failed$", err.Error())
}

// verifies that the read operation is considered failed if the initial API
// call fails.
func TestGitRepoFile_Read_DoesNotSwallowErrorFromFailedReadCall(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	resourceData := schema.TestResourceDataRaw(t, ResourceGitRepositoryFile().Schema, nil)
	configureResourceGitRepositoryFile(resourceData)
	reposClient := azdosdkmocks.NewMockGitClient(ctrl)
	clients := &client.AggregatedClient{GitReposClient: reposClient, Ctx: context.Background()}
	mockReposClientGetBranch(reposClient)
	mockReposClientGetItem(reposClient, git.GetItemArgs{
		RepositoryId:   converter.String(testRepositoryID.String()),
		Path:           converter.String("file"),
		IncludeContent: boolPointer(true),
		VersionDescriptor: &git.GitVersionDescriptor{
			Version:     converter.String("main"),
			VersionType: &git.GitVersionTypeValues.Branch,
		},
	},
		&git.GitItem{
			Content:  converter.String("content"),
			CommitId: converter.String(testCommitID.String()),
		})
	mockReposClientGetCommit(reposClient, "ReadGitRepositoryFile() Failed")

	err := resourceGitRepositoryFileRead(resourceData, clients)

	require.Regexp(t, ".*ReadGitRepositoryFile\\(\\) Failed$", err.Error())
}

// verifies that the delete operation is considered failed if the initial API
// call fails.
func TestGitRepoFile_Delete_DoesNotSwallowErrorFromFailedDeleteCall(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	resourceData := schema.TestResourceDataRaw(t, ResourceGitRepositoryFile().Schema, nil)
	configureResourceGitRepositoryFile(resourceData)
	reposClient := azdosdkmocks.NewMockGitClient(ctrl)
	clients := &client.AggregatedClient{GitReposClient: reposClient, Ctx: context.Background()}
	mockReposClientGetCommits(reposClient)
	change := git.GitChange{
		ChangeType: &git.VersionControlChangeTypeValues.Delete,
		Item: git.GitItem{
			Path: converter.String("file"),
		},
	}
	expectedArgs := createGitPushArgs(
		resourceData,
		converter.String(testCommitID.String()),
		converter.String("Delete file"),
		&change)
	reposClient.
		EXPECT().
		CreatePush(gomock.Any(), gomock.Eq(*expectedArgs)).
		Return(nil, errors.New("DeleteGitRepositoryFile() Failed")).
		Times(1)

	err := resourceGitRepositoryFileDelete(resourceData, clients)

	require.Regexp(t, ".*DeleteGitRepositoryFile\\(\\) Failed$", err.Error())

}

func mockReposClientGetCommits(reposClient *azdosdkmocks.MockGitClient) {
	gitCommitArgs := git.GetCommitsArgs{
		RepositoryId:   converter.String(testRepositoryID.String()),
		Top:            intPointer(1),
		SearchCriteria: &git.GitQueryCommitsCriteria{},
	}
	reposClient.
		EXPECT().
		GetCommits(gomock.Any(), gomock.Eq(gitCommitArgs)).
		Return(&[]git.GitCommitRef{
			{
				CommitId: converter.String(testCommitID.String()),
			}}, nil).
		Times(1)
}

func mockReposClientGetBranch(reposClient *azdosdkmocks.MockGitClient) *gomock.Call {
	return reposClient.
		EXPECT().
		GetBranch(gomock.Any(), gomock.Any()).
		Return(&git.GitBranchStats{}, nil).
		Times(1)
}

func mockReposClientGetItem(reposClient *azdosdkmocks.MockGitClient, getItemArgs interface{}, item *git.GitItem) *gomock.Call {
	return reposClient.
		EXPECT().
		GetItem(gomock.Any(), getItemArgs).
		Return(item, nil).
		Times(1)
}

func mockReposClientGetCommit(reposClient *azdosdkmocks.MockGitClient, errorText string) {
	gitCommitArgs := git.GetCommitArgs{
		RepositoryId: converter.String(testRepositoryID.String()),
		CommitId:     converter.String(testCommitID.String()),
	}

	reposClient.
		EXPECT().
		GetCommit(gomock.Any(), gomock.Eq(gitCommitArgs)).
		Return(nil, errors.New(errorText)).
		Times(1)
}

func configureResourceGitRepositoryFile(d *schema.ResourceData) {
	d.SetId(testRepositoryID.String() + "/file")
	d.Set("repository_id", testRepositoryID.String())
	d.Set("file", "file")
	d.Set("content", "content")
	d.Set("commit_message", "commit_message")
	d.Set("branch", "main")
}
