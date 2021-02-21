// +build all core resource_git_repository_file
// +build !exclude_resource_git_repository_file

package acceptancetests

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/terraform"
	"github.com/microsoft/azure-devops-go-api/azuredevops/git"
	"github.com/microsoft/terraform-provider-azuredevops/azuredevops/internal/acceptancetests/testutils"
	"github.com/microsoft/terraform-provider-azuredevops/azuredevops/internal/client"
)

// TestAccGitRepoFile_CreateUpdateDelete verifies that a file can
// be added to a repository and the contents can be updated
func TestAccGitRepoFile_CreateAndUpdate(t *testing.T) {
	projectName := testutils.GenerateResourceName()
	gitRepoName := testutils.GenerateResourceName()
	tfRepoFileNode := "azuredevops_git_repository_file.file"

	branch := "refs/heads/master"
	file := "foo.txt"
	contentFirst := "bar"
	contentSecond := "baz"

	resource.Test(t, resource.TestCase{
		PreCheck:  func() { testutils.PreCheck(t, nil) },
		Providers: testutils.GetProviders(),
		Steps: []resource.TestStep{
			{
				Config: testutils.HclGitRepoFileResource(projectName, gitRepoName, "Clean", branch, file, contentFirst),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(tfRepoFileNode, "file", file),
					resource.TestCheckResourceAttr(tfRepoFileNode, "content", contentFirst),
					resource.TestCheckResourceAttr(tfRepoFileNode, "branch", branch),
					resource.TestCheckResourceAttrSet(tfRepoFileNode, "comment"),
					checkGitRepoFileContent(contentFirst, tfRepoFileNode),
				),
			},
			{
				Config: testutils.HclGitRepoFileResource(projectName, gitRepoName, "Clean", branch, file, contentSecond),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(tfRepoFileNode, "file", file),
					resource.TestCheckResourceAttr(tfRepoFileNode, "content", contentSecond),
					resource.TestCheckResourceAttr(tfRepoFileNode, "branch", branch),
					resource.TestCheckResourceAttrSet(tfRepoFileNode, "comment"),
					checkGitRepoFileContent(contentSecond, tfRepoFileNode),
				),
			},
			{
				Config: testutils.HclGitRepoResource(projectName, gitRepoName, "Clean"),
				Check: resource.ComposeTestCheckFunc(
					checkGitRepoFileNotExists(file),
				),
			},
		},
	})
}

// TestAccGitRepoFile_CreateMoreThanOneFile verifies that many files can
// be added to a repository
func TestAccGitRepoFile_CreateMoreThanOneFile(t *testing.T) {
	projectName := testutils.GenerateResourceName()
	gitRepoName := testutils.GenerateResourceName()

	gitRepoResource := testutils.HclGitRepoResource(projectName, gitRepoName, "Clean")

	tfRepoFileNode1 := "azuredevops_git_repository_file.file1"
	tfRepoFileNode2 := "azuredevops_git_repository_file.file2"

	branch := "refs/heads/master"
	file := "foo.txt"
	content := "bar"

	gitRepoFileResource := fmt.Sprintf(`
	resource "azuredevops_git_repository_file" "file1" {
		repository_id = azuredevops_git_repository.repository.id
		file          = "%s1"
		content       = "%s"
		branch        = "%s"
	}
	
	resource "azuredevops_git_repository_file" "file2" {
		repository_id = azuredevops_git_repository.repository.id
		file          = "%s2"
		content       = "%s"
		branch        = "%s"
	}
	`, file, content, branch, file, content, branch)

	config := fmt.Sprintf("%s\n%s", gitRepoResource, gitRepoFileResource)

	resource.Test(t, resource.TestCase{
		PreCheck:  func() { testutils.PreCheck(t, nil) },
		Providers: testutils.GetProviders(),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(tfRepoFileNode1, "content", content),
					resource.TestCheckResourceAttr(tfRepoFileNode2, "content", content),
					checkGitRepoFileContent(content, tfRepoFileNode1),
					checkGitRepoFileContent(content, tfRepoFileNode2),
				),
			},
		},
	})
}

// TestAccGitRepoFile_Create_IncorrectBranch verifies a file
// can't be added to a non existant branch
func TestAccGitRepoFile_Create_IncorrectBranch(t *testing.T) {
	projectName := testutils.GenerateResourceName()
	gitRepoName := testutils.GenerateResourceName()

	resource.Test(t, resource.TestCase{
		PreCheck:  func() { testutils.PreCheck(t, nil) },
		Providers: testutils.GetProviders(),
		Steps: []resource.TestStep{
			{
				Config:      testutils.HclGitRepoFileResource(projectName, gitRepoName, "Clean", "foobar", "foo", "bar"),
				ExpectError: regexp.MustCompile(`errors during apply: Branch "foobar" does not exist`),
			},
		},
	})
}

// checkGitRepoFileNotExists checks that the repository mentioned within does not exist.
func checkGitRepoFileNotExists(fileName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		clients := testutils.GetProvider().Meta().(*client.AggregatedClient)

		repo, ok := s.RootModule().Resources["azuredevops_git_repository.repository"]
		if !ok {
			return fmt.Errorf("Did not find a repo definition in the TF state")
		}

		ctx := context.Background()
		_, err := clients.GitReposClient.GetItem(ctx, git.GetItemArgs{
			RepositoryId: &repo.Primary.ID,
			Path:         &fileName,
		})
		if err != nil && !strings.Contains(err.Error(), "could not be found in the repository") {
			return err
		}
		return nil
	}
}

// checkGitRepoFileContent checks that the content of a file is as expected.
func checkGitRepoFileContent(expectedContent, resourceFileIdentifier string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		clients := testutils.GetProvider().Meta().(*client.AggregatedClient)

		gitFile, ok := s.RootModule().Resources[resourceFileIdentifier]
		if !ok {
			return fmt.Errorf("Did not find a repo definition in the TF state")
		}

		fileID := gitFile.Primary.ID
		comps := strings.Split(fileID, "/")
		repoID := comps[0]
		file := comps[1]

		ctx := context.Background()
		r, err := clients.GitReposClient.GetItemContent(ctx, git.GetItemContentArgs{
			RepositoryId: &repoID,
			Path:         &file,
		})
		if err != nil {
			return err
		}

		buf := new(bytes.Buffer)
		if _, err = buf.ReadFrom(r); err != nil {
			return err
		}

		if buf.String() != expectedContent {
			return fmt.Errorf("Unexpected git file content: %v", buf.String())
		}

		return nil
	}
}
