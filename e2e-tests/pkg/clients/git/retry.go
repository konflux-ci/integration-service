package git

import (
	"fmt"
	"time"

	"k8s.io/klog/v2"
)

// ListPullRequestsWithRetry wraps Client.ListPullRequests with retries on transient errors.
func ListPullRequestsWithRetry(client Client, repository string) ([]*PullRequest, error) {
	const maxRetries = 3
	var prs []*PullRequest
	var err error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		prs, err = client.ListPullRequests(repository)
		if err == nil {
			return prs, nil
		}
		klog.Warningf("error listing PRs in %s (attempt %d/%d): %v", repository, attempt, maxRetries, err)
		if attempt < maxRetries {
			time.Sleep(5 * time.Second)
		}
	}
	return nil, fmt.Errorf("failed to list pull requests for %s after %d retries: %w", repository, maxRetries, err)
}
