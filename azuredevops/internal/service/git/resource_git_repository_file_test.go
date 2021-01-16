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
	mockReposClient(reposClient)
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

func mockReposClient(reposClient *azdosdkmocks.MockGitClient) {
	reposClient.
		EXPECT().
		GetBranch(gomock.Any(), gomock.Any()).
		Return(&git.GitBranchStats{}, nil).
		Times(1)
	reposClient.
		EXPECT().
		GetItem(gomock.Any(), gomock.Any()).
		Return(nil, nil).
		Times(1)
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
func configureResourceGitRepositoryFile(d *schema.ResourceData) {
	d.Set("repository_id", testRepositoryID.String())
	d.Set("file", "file")
	d.Set("content", "content")
	d.Set("commit_message", "commit_message")
	d.Set("branch", "main")
}
