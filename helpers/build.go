package helpers

import "time"

const (
	// CreateSnapshotAnnotationName contains metadata of snapshot creation failure or success
	CreateSnapshotAnnotationName = "test.appstudio.openshift.io/create-snapshot-status"

	// SnapshotCreationReportAnnotation contains metadata of snapshot creation status reporting to git provider
	// to initialize integration test or set it to cancelled or failed
	SnapshotCreationReportAnnotation = "test.appstudio.openshift.io/snapshot-creation-report"

	// ChainsSignedCheckRetryCountAnnotation tracks how many times we've retried waiting for Chains signing
	ChainsSignedCheckRetryCountAnnotation = "test.appstudio.openshift.io/chains-signed-retry-count"

	// ChainsSignedCheckRetryLimit is the maximum number of retries before treating Chains not-signed as a failure
	ChainsSignedCheckRetryLimit = 60

	// ChainsSignedCheckRequeueDelay is the delay between requeues when waiting for Chains signing
	ChainsSignedCheckRequeueDelay = 30 * time.Second
)
