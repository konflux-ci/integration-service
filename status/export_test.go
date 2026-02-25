package status

import "k8s.io/apimachinery/pkg/util/wait"

// SetReporterRetryBackoff overrides the retry backoff for testing.
func SetReporterRetryBackoff(b wait.Backoff) { reporterRetryBackoff = b }

// GetReporterRetryBackoff returns the current retry backoff.
func GetReporterRetryBackoff() wait.Backoff { return reporterRetryBackoff }

// SetUpdater sets the StatusUpdater on a GitHubReporter for testing.
func (r *GitHubReporter) SetUpdater(u StatusUpdater) { r.updater = u }
