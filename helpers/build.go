package helpers

import (
	"errors"
	"time"
)

const (
	// CreateSnapshotAnnotationName contains metadata of snapshot creation failure or success
	CreateSnapshotAnnotationName = "test.appstudio.openshift.io/create-snapshot-status"

	// SnapshotCreationReportAnnotation contains metadata of snapshot creation status reporting to git provider
	// to initialize integration test or set it to cancelled or failed
	SnapshotCreationReportAnnotation = "test.appstudio.openshift.io/snapshot-creation-report"

	// ChainsSignedCheckTimeout is how long after PLR completion we wait for Chains to sign before treating it as a failure
	ChainsSignedCheckTimeout = 5 * time.Minute
)

// ChainsNotSignedError indicates the PipelineRun is not yet signed by Chains.
// This is a transient condition that should not cause a permanent failure annotation.
type ChainsNotSignedError struct {
	Message string
}

func (e *ChainsNotSignedError) Error() string {
	return e.Message
}

// IsChainsNotSignedError returns true if the error is a transient Chains-not-signed error.
func IsChainsNotSignedError(err error) bool {
	var target *ChainsNotSignedError
	return errors.As(err, &target)
}
