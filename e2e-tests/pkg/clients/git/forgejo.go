package git

import (
	"github.com/konflux-ci/integration-service/e2e-tests/pkg/clients/forgejo"
)

// ForgejoClient adapts forgejo.ForgejoClient to the git Client interface.
type ForgejoClient struct {
	*forgejo.ForgejoClient
}

// NewForgejoClient wraps a Forgejo API client.
func NewForgejoClient(fc *forgejo.ForgejoClient) *ForgejoClient {
	return &ForgejoClient{fc}
}

// CreateBranch creates branchName from baseBranchName (revision is ignored for Forgejo).
func (f *ForgejoClient) CreateBranch(repository, baseBranchName, _, branchName string) error {
	return f.ForgejoClient.CreateBranch(repository, branchName, baseBranchName)
}

// BranchExists reports whether a branch exists.
func (f *ForgejoClient) BranchExists(repository, branchName string) (bool, error) {
	return f.ExistsBranch(repository, branchName)
}

// ListPullRequests lists open pull requests.
func (f *ForgejoClient) ListPullRequests(projectID string) ([]*PullRequest, error) {
	prs, err := f.GetPullRequests(projectID)
	if err != nil {
		return nil, err
	}
	var pullRequests []*PullRequest
	for _, pr := range prs {
		if pr.Head == nil || pr.Base == nil {
			continue
		}
		pullRequests = append(pullRequests, &PullRequest{
			Number:       int(pr.Index),
			SourceBranch: pr.Head.Ref,
			TargetBranch: pr.Base.Ref,
			HeadSHA:      pr.Head.Sha,
		})
	}
	return pullRequests, nil
}

// CreateFile creates a file on a branch.
func (f *ForgejoClient) CreateFile(repository, pathToFile, content, branchName string) (*RepositoryFile, error) {
	fileResp, err := f.ForgejoClient.CreateFile(repository, pathToFile, content, branchName)
	if err != nil {
		return nil, err
	}
	resultFile := &RepositoryFile{}
	if fileResp != nil && fileResp.Commit != nil {
		resultFile.CommitSHA = fileResp.Commit.SHA
	}
	return resultFile, nil
}

// GetFile fetches file contents from a branch.
func (f *ForgejoClient) GetFile(repository, pathToFile, branchName string) (*RepositoryFile, error) {
	content, contentsResp, err := f.ForgejoClient.GetFile(repository, pathToFile, branchName)
	if err != nil {
		return nil, err
	}
	resultFile := &RepositoryFile{
		Content: content,
	}
	if contentsResp != nil {
		resultFile.CommitSHA = contentsResp.SHA
	}
	return resultFile, nil
}

// MergePullRequest merges a pull request by number.
func (f *ForgejoClient) MergePullRequest(repository string, prNumber int) (*PullRequest, error) {
	pr, err := f.ForgejoClient.MergePullRequest(repository, int64(prNumber))
	if err != nil {
		return nil, err
	}
	mergeCommitSHA := ""
	if pr.MergedCommitID != nil {
		mergeCommitSHA = *pr.MergedCommitID
	}
	headSha := ""
	if pr.Head != nil {
		headSha = pr.Head.Sha
	}
	src, tgt := "", ""
	if pr.Head != nil {
		src = pr.Head.Ref
	}
	if pr.Base != nil {
		tgt = pr.Base.Ref
	}
	return &PullRequest{
		Number:         int(pr.Index),
		SourceBranch:   src,
		TargetBranch:   tgt,
		HeadSHA:        headSha,
		MergeCommitSHA: mergeCommitSHA,
	}, nil
}

// CreatePullRequest creates a new pull request.
func (f *ForgejoClient) CreatePullRequest(repository, title, body, head, base string) (*PullRequest, error) {
	pr, err := f.ForgejoClient.CreatePullRequest(repository, title, body, head, base)
	if err != nil {
		return nil, err
	}
	headSha := ""
	if pr.Head != nil {
		headSha = pr.Head.Sha
	}
	src, tgt := "", ""
	if pr.Head != nil {
		src = pr.Head.Ref
	}
	if pr.Base != nil {
		tgt = pr.Base.Ref
	}
	return &PullRequest{
		Number:       int(pr.Index),
		SourceBranch: src,
		TargetBranch: tgt,
		HeadSHA:      headSha,
	}, nil
}

// UpdatePullRequestBranch merges the base branch into the PR branch.
func (f *ForgejoClient) UpdatePullRequestBranch(repository string, prNumber int) error {
	return f.ForgejoClient.UpdatePullRequestBranch(repository, int64(prNumber))
}

// CleanupWebhooks removes webhooks matching the cluster app domain.
func (f *ForgejoClient) CleanupWebhooks(repository, clusterAppDomain string) error {
	return f.DeleteWebhooks(repository, clusterAppDomain)
}

// DeleteBranchAndClosePullRequest deletes the PR source branch and closes the PR.
func (f *ForgejoClient) DeleteBranchAndClosePullRequest(repository string, prNumber int) error {
	prs, err := f.GetPullRequests(repository)
	if err != nil {
		return err
	}
	var sourceBranch string
	for _, pr := range prs {
		if int(pr.Index) == prNumber && pr.Head != nil {
			sourceBranch = pr.Head.Ref
			break
		}
	}
	if sourceBranch != "" {
		if err := f.ForgejoClient.DeleteBranch(repository, sourceBranch); err != nil {
			return err
		}
	}
	return f.ClosePullRequest(repository, int64(prNumber))
}

// ForkRepository migrates/clones source into a new repository name.
func (f *ForgejoClient) ForkRepository(sourceRepoName, targetRepoName string) error {
	_, err := f.ForgejoClient.ForkRepository(sourceRepoName, targetRepoName)
	return err
}

// DeleteRepositoryIfExists deletes a repository if present.
func (f *ForgejoClient) DeleteRepositoryIfExists(repoName string) error {
	return f.ForgejoClient.DeleteRepositoryIfExists(repoName)
}

// DeleteBranch deletes a branch.
func (f *ForgejoClient) DeleteBranch(repository, branchName string) error {
	return f.ForgejoClient.DeleteBranch(repository, branchName)
}

// GetCommitStatusConclusion returns the commit status state string for a scenario.
func (f *ForgejoClient) GetCommitStatusConclusion(statusName, projectID, commitSHA string, prNumber int) string {
	return f.ForgejoClient.GetCommitStatusConclusion(statusName, projectID, commitSHA, int64(prNumber))
}
