package helpers

const (
	// CreateSnapshotAnnotationName contains metadata of snapshot creation failure or success
	CreateSnapshotAnnotationName = "test.appstudio.openshift.io/create-snapshot-status"

	// SnapshotCreationReportAnnotation contains metadata of snapshot creation status reporting to git provider
	// to initialize integration test or set it to cancelled or failed
	SnapshotCreationReportAnnotation = "test.appstudio.openshift.io/snapshot-creation-report"
)
