/*
Copyright 2026 Red Hat Inc.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
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

	// PAC GitOps comment event-type label values (from pipelines-as-code opscomments).

	PipelineAsCodeTestCommentType      = "test-comment"
	PipelineAsCodeTestAllCommentType   = "test-all-comment"
	PipelineAsCodeRetestCommentType    = "retest-comment"
	PipelineAsCodeRetestAllCommentType = "retest-all-comment"
	PipelineAsCodeCancelCommentType    = "cancel-comment"
	PipelineAsCodeCancelAllCommentType = "cancel-all-comment"
	PipelineAsCodeOnCommentType        = "on-comment"
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

// IsPACPushStyleGitOpsComment reports whether a PipelineRun looks like a
// GitOps command on a pushed commit (commit_comment), as opposed to the same
// command on a pull request (issue_comment).
//
// PAC rewrites EventType to ops strings for both paths, and may attach a
// pull-request label to commit-comment runs when the SHA is linked to a PR.
// Commit-comment handlers set HeadBranch == BaseBranch; PR handlers do not.
func IsPACPushStyleGitOpsComment(eventType, targetBranch, sourceBranch string) bool {
	if targetBranch == "" || sourceBranch == "" || targetBranch != sourceBranch {
		return false
	}
	switch eventType {
	case PipelineAsCodeTestCommentType,
		PipelineAsCodeTestAllCommentType,
		PipelineAsCodeRetestCommentType,
		PipelineAsCodeRetestAllCommentType,
		PipelineAsCodeCancelCommentType,
		PipelineAsCodeCancelAllCommentType,
		PipelineAsCodeOnCommentType:
		return true
	default:
		return false
	}
}
