package forgejo

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"time"

	"codeberg.org/mvdkleijn/forgejo-sdk/forgejo/v3"
	gomega "github.com/onsi/gomega"

	"github.com/konflux-ci/integration-service/e2e-tests/pkg/utils"
)

// CreateBranch creates a new branch in a Forgejo repository.
// projectID must be "owner/repo".
func (fc *ForgejoClient) CreateBranch(projectID, newBranchName, baseBranch string) error {
	owner, repo := splitProjectID(projectID)

	opts := forgejo.CreateBranchOption{
		BranchName:    newBranchName,
		OldBranchName: baseBranch,
	}

	_, resp, err := fc.client.CreateBranch(owner, repo, opts)
	if err != nil {
		return fmt.Errorf("failed to create branch %s in project %s: %w", newBranchName, projectID, err)
	}
	if resp != nil && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("unexpected status code when creating branch %s: %d", newBranchName, resp.StatusCode)
	}

	gomega.Eventually(func() bool {
		exists, err := fc.ExistsBranch(projectID, newBranchName)
		if err != nil {
			return false
		}
		return exists
	}, 2*time.Minute, 2*time.Second).Should(gomega.BeTrue())

	return nil
}

// ExistsBranch reports whether a branch exists in a Forgejo repository.
func (fc *ForgejoClient) ExistsBranch(projectID, branchName string) (bool, error) {
	owner, repo := splitProjectID(projectID)

	_, resp, err := fc.client.GetRepoBranch(owner, repo, branchName)
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// DeleteBranch deletes a branch from a Forgejo repository.
func (fc *ForgejoClient) DeleteBranch(projectID, branchName string) error {
	owner, repo := splitProjectID(projectID)

	_, _, err := fc.client.DeleteRepoBranch(owner, repo, branchName)
	if err != nil {
		return fmt.Errorf("failed to delete branch %s: %w", branchName, err)
	}
	return nil
}

// GetPullRequests lists open pull requests in a repository.
func (fc *ForgejoClient) GetPullRequests(projectID string) ([]*forgejo.PullRequest, error) {
	owner, repo := splitProjectID(projectID)

	opts := forgejo.ListPullRequestsOptions{
		State: forgejo.StateOpen,
		ListOptions: forgejo.ListOptions{
			Page:     1,
			PageSize: 100,
		},
	}

	prs, _, err := fc.client.ListRepoPullRequests(owner, repo, opts)
	if err != nil {
		return nil, err
	}
	return prs, nil
}

// CreatePullRequest creates a new pull request.
func (fc *ForgejoClient) CreatePullRequest(projectID, title, body, head, base string) (*forgejo.PullRequest, error) {
	owner, repo := splitProjectID(projectID)

	opts := forgejo.CreatePullRequestOption{
		Title: title,
		Body:  body,
		Head:  head,
		Base:  base,
	}

	pr, _, err := fc.client.CreatePullRequest(owner, repo, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to create pull request: %w", err)
	}
	return pr, nil
}

// MergePullRequest merges a pull request.
func (fc *ForgejoClient) MergePullRequest(projectID string, prNumber int64) (*forgejo.PullRequest, error) {
	owner, repo := splitProjectID(projectID)

	opts := forgejo.MergePullRequestOption{
		Style: forgejo.MergeStyleMerge,
	}

	success, _, err := fc.client.MergePullRequest(owner, repo, prNumber, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to merge pull request: %w", err)
	}
	if !success {
		return nil, fmt.Errorf("merge was not successful")
	}

	pr, _, err := fc.client.GetPullRequest(owner, repo, prNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to get merged pull request: %w", err)
	}
	return pr, nil
}

// UpdatePullRequestBranch updates a pull request branch (merge base into PR branch).
func (fc *ForgejoClient) UpdatePullRequestBranch(projectID string, prNumber int64) error {
	owner, repo := splitProjectID(projectID)

	url := fmt.Sprintf("%s/api/v1/repos/%s/%s/pulls/%d/update", strings.TrimSuffix(fc.apiURL, "/"), owner, repo, prNumber)
	req, err := http.NewRequest(http.MethodPost, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request for updating PR branch #%d: %w", prNumber, err)
	}
	req.Header.Set("Authorization", "token "+fc.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to update pull request branch for PR #%d: %w", prNumber, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code when updating PR branch #%d: %d", prNumber, resp.StatusCode)
	}
	return nil
}

// ClosePullRequest closes a pull request without merging.
func (fc *ForgejoClient) ClosePullRequest(projectID string, prNumber int64) error {
	owner, repo := splitProjectID(projectID)

	state := forgejo.StateClosed
	opts := forgejo.EditPullRequestOption{
		State: &state,
	}

	_, _, err := fc.client.EditPullRequest(owner, repo, prNumber, opts)
	if err != nil {
		return fmt.Errorf("failed to close pull request %d: %w", prNumber, err)
	}
	return nil
}

// CreateFile creates a new file in a repository.
func (fc *ForgejoClient) CreateFile(projectID, pathToFile, content, branchName string) (*forgejo.FileResponse, error) {
	owner, repo := splitProjectID(projectID)

	opts := forgejo.CreateFileOptions{
		FileOptions: forgejo.FileOptions{
			Message:    "e2e test commit message",
			BranchName: branchName,
		},
		Content: base64.StdEncoding.EncodeToString([]byte(content)),
	}

	fileResp, _, err := fc.client.CreateFile(owner, repo, pathToFile, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to create file %s: %w", pathToFile, err)
	}
	return fileResp, nil
}

// GetFile returns decoded file contents and metadata for a path on a branch.
func (fc *ForgejoClient) GetFile(projectID, pathToFile, branchName string) (string, *forgejo.ContentsResponse, error) {
	owner, repo := splitProjectID(projectID)

	cr, _, err := fc.client.GetContents(owner, repo, branchName, pathToFile)
	if err != nil {
		return "", nil, fmt.Errorf("failed to get file %s: %w", pathToFile, err)
	}
	if cr == nil || cr.Content == nil {
		return "", cr, nil
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(*cr.Content))
	if err != nil {
		return "", nil, fmt.Errorf("failed to decode file content: %w", err)
	}
	return string(decoded), cr, nil
}

// DeleteWebhooks deletes the first repo webhook whose URL contains clusterAppDomain.
func (fc *ForgejoClient) DeleteWebhooks(projectID, clusterAppDomain string) error {
	if clusterAppDomain == "" {
		return fmt.Errorf("clusterAppDomain is empty")
	}

	owner, repo := splitProjectID(projectID)

	hooks, _, err := fc.client.ListRepoHooks(owner, repo, forgejo.ListHooksOptions{})
	if err != nil {
		return fmt.Errorf("failed to list webhooks: %w", err)
	}

	for _, hook := range hooks {
		if hook.Config == nil {
			continue
		}
		if urlStr, ok := hook.Config["url"]; ok && strings.Contains(urlStr, clusterAppDomain) {
			_, err := fc.client.DeleteRepoHook(owner, repo, hook.ID)
			if err != nil {
				return fmt.Errorf("failed to delete webhook (ID: %d): %w", hook.ID, err)
			}
			break
		}
	}
	return nil
}

// ForkRepository clones sourceProjectID into a new repository targetProjectID via MigrateRepo.
func (fc *ForgejoClient) ForkRepository(sourceProjectID, targetProjectID string) (*forgejo.Repository, error) {
	sourceOwner, sourceRepo := splitProjectID(sourceProjectID)
	targetOwner, targetRepo := splitProjectID(targetProjectID)

	cloneAddr := fmt.Sprintf("%s/%s/%s.git", strings.TrimSuffix(fc.apiURL, "/"), sourceOwner, sourceRepo)

	var migratedRepo *forgejo.Repository
	var lastErr error

	err := utils.WaitUntilWithInterval(func() (bool, error) {
		var resp *forgejo.Response
		var migrateErr error
		migratedRepo, resp, migrateErr = fc.client.MigrateRepo(forgejo.MigrateRepoOption{
			RepoName:  targetRepo,
			RepoOwner: targetOwner,
			CloneAddr: cloneAddr,
			Service:   forgejo.GitServiceForgejo,
			AuthToken: fc.token,
		})
		if migrateErr != nil {
			lastErr = migrateErr
			statusCode := 0
			if resp != nil && resp.Response != nil {
				statusCode = resp.StatusCode
			}
			if statusCode == http.StatusNotFound || statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden {
				return false, fmt.Errorf("error migrating project %s to %s (HTTP %d): %w", sourceProjectID, targetProjectID, statusCode, migrateErr)
			}
			if statusCode == http.StatusConflict {
				existingRepo, _, getErr := fc.client.GetRepo(targetOwner, targetRepo)
				if getErr != nil {
					return false, fmt.Errorf("error migrating project %s to %s: repo already exists but failed to fetch it: %w", sourceProjectID, targetProjectID, getErr)
				}
				migratedRepo = existingRepo
				return true, nil
			}
			return false, nil
		}
		return true, nil
	}, time.Second*10, time.Minute*5)

	if err != nil {
		if lastErr != nil {
			return nil, fmt.Errorf("error migrating project %s to %s (last error: %v): %w", sourceProjectID, targetProjectID, lastErr, err)
		}
		return nil, fmt.Errorf("error migrating project %s to %s: %w", sourceProjectID, targetProjectID, err)
	}
	return migratedRepo, nil
}

// DeleteRepository deletes a repository.
func (fc *ForgejoClient) DeleteRepository(projectID string) error {
	owner, repo := splitProjectID(projectID)

	_, err := fc.client.DeleteRepo(owner, repo)
	if err != nil {
		if strings.Contains(err.Error(), "404") {
			return nil
		}
		return fmt.Errorf("failed to delete repository %s: %w", projectID, err)
	}
	return nil
}

// DeleteRepositoryIfExists deletes a repository if it exists.
func (fc *ForgejoClient) DeleteRepositoryIfExists(projectID string) error {
	owner, repo := splitProjectID(projectID)

	_, resp, err := fc.client.GetRepo(owner, repo)
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			return nil
		}
		return fmt.Errorf("error checking if repository exists: %w", err)
	}
	return fc.DeleteRepository(projectID)
}

// GetCommitStatusConclusion waits for a commit status whose context contains statusName and returns its state string.
func (fc *ForgejoClient) GetCommitStatusConclusion(statusName, projectID, commitSHA string, prNumber int64) string {
	_ = prNumber
	owner, repo := splitProjectID(projectID)
	var matchingStatus *forgejo.CombinedStatus
	timeout := time.Minute * 10

	gomega.Eventually(func() bool {
		combinedStatus, _, err := fc.client.GetCombinedStatus(owner, repo, commitSHA)
		if err != nil {
			return false
		}
		for _, status := range combinedStatus.Statuses {
			if strings.Contains(status.Context, statusName) {
				matchingStatus = combinedStatus
				return true
			}
		}
		return false
	}, timeout, 2*time.Second).Should(gomega.BeTrue(),
		fmt.Sprintf("timed out waiting for the PaC commit status to appear for %s", commitSHA))

	gomega.Eventually(func() bool {
		combinedStatus, _, err := fc.client.GetCombinedStatus(owner, repo, commitSHA)
		if err != nil {
			return false
		}
		for _, status := range combinedStatus.Statuses {
			if strings.Contains(status.Context, statusName) {
				if status.State != forgejo.StatusPending {
					matchingStatus = combinedStatus
					return true
				}
				return false
			}
		}
		return false
	}, timeout, 2*time.Second).Should(gomega.BeTrue(),
		fmt.Sprintf("timed out waiting for the PaC commit status to be completed for %s", commitSHA))

	if matchingStatus == nil {
		return ""
	}
	for _, status := range matchingStatus.Statuses {
		if strings.Contains(status.Context, statusName) {
			return string(status.State)
		}
	}
	return ""
}

func splitProjectID(projectID string) (string, string) {
	parts := strings.SplitN(projectID, "/", 2)
	if len(parts) != 2 {
		return projectID, ""
	}
	return parts[0], parts[1]
}
