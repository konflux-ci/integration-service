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

package helpers_test

import (
	"github.com/konflux-ci/integration-service/helpers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Helpers for build", func() {
	Context("IsPACPushStyleGitOpsComment", func() {
		DescribeTable("detects push-style GitOps comments from commit comments",
			func(eventType, targetBranch, sourceBranch string, expected bool) {
				Expect(helpers.IsPACPushStyleGitOpsComment(eventType, targetBranch, sourceBranch)).To(Equal(expected))
			},
			Entry("test-comment with matching branches", helpers.PipelineAsCodeTestCommentType, "main", "main", true),
			Entry("test-comment with matching branches, but different branch prefixes", helpers.PipelineAsCodeTestCommentType, "refs/heads/main", "main", true),
			Entry("test-comment with matching branches, and same branch prefixes", helpers.PipelineAsCodeTestCommentType, "refs/heads/main", "refs/heads/main", true),
			Entry("test-comment with matching branches but different branch prefixes", helpers.PipelineAsCodeTestCommentType, "main", "refs/heads/main", true),
			Entry("test-all-comment with matching branches", helpers.PipelineAsCodeTestAllCommentType, "main", "main", true),
			Entry("retest-comment with matching branches", helpers.PipelineAsCodeRetestCommentType, "main", "main", true),
			Entry("retest-all-comment with matching branches", helpers.PipelineAsCodeRetestAllCommentType, "main", "main", true),
			Entry("cancel-comment with matching branches", helpers.PipelineAsCodeCancelCommentType, "main", "main", true),
			Entry("cancel-all-comment with matching branches", helpers.PipelineAsCodeCancelAllCommentType, "main", "main", true),
			Entry("on-comment with matching branches", helpers.PipelineAsCodeOnCommentType, "main", "main", true),
			Entry("matching branches with refs/heads/ prefix on both", helpers.PipelineAsCodeTestCommentType, "refs/heads/main", "refs/heads/main", true),
			Entry("matching branches with refs/heads/ prefix only on target", helpers.PipelineAsCodeTestCommentType, "refs/heads/main", "main", true),
			Entry("matching branches with refs/heads/ prefix only on source", helpers.PipelineAsCodeTestCommentType, "main", "refs/heads/main", true),
			Entry("matching tag refs", helpers.PipelineAsCodeRetestAllCommentType, "refs/tags/v1.0.0", "refs/tags/v1.0.0", true),
			Entry("empty target branch", helpers.PipelineAsCodeTestCommentType, "", "main", false),
			Entry("empty source branch", helpers.PipelineAsCodeTestCommentType, "main", "", false),
			Entry("both branches empty", helpers.PipelineAsCodeTestCommentType, "", "", false),
			Entry("mismatched branches for PR-style GitOps comment", helpers.PipelineAsCodeTestCommentType, "main", "feature", false),
			Entry("ok-to-test-comment even with matching branches", "ok-to-test-comment", "main", "main", false),
			Entry("native push event type", "push", "main", "main", false),
			Entry("native pull_request event type", "pull_request", "main", "main", false),
			Entry("unknown event type", "something-else", "main", "main", false),
		)
	})
})
