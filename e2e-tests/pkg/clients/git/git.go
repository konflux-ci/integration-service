package git

// PullRequest is a provider-agnostic pull/merge request.
type PullRequest struct {
	Number int
	// SourceBranch includes the changes made in the pull request
	SourceBranch string
	// TargetBranch is the base branch on top of which the changes are merged
	TargetBranch string
	// MergeCommitSHA is the revision of the commit which merged the PullRequest
	MergeCommitSHA string
	// HeadSHA is the revision of the commit on top of the SourceBranch
	HeadSHA string
}

// RepositoryFile is a provider-agnostic file in a repository.
type RepositoryFile struct {
	CommitSHA string
	Content   string
}

// Client is implemented by provider-specific git clients used in e2e tests.
type Client interface {
	CreateBranch(repository, baseBranchName, revision, branchName string) error
	DeleteBranch(repository, branchName string) error
	BranchExists(repository, branchName string) (bool, error)
	ListPullRequests(repository string) ([]*PullRequest, error)
	CreateFile(repository, pathToFile, content, branchName string) (*RepositoryFile, error)
	GetFile(repository, pathToFile, branchName string) (*RepositoryFile, error)
	CreatePullRequest(repository, title, body, head, base string) (*PullRequest, error)
	MergePullRequest(repository string, prNumber int) (*PullRequest, error)
	UpdatePullRequestBranch(repository string, prNumber int) error
	DeleteBranchAndClosePullRequest(repository string, prNumber int) error
	CleanupWebhooks(repository, clusterAppDomain string) error
	ForkRepository(sourceRepoName, targetRepoName string) error
	DeleteRepositoryIfExists(repoName string) error
}
