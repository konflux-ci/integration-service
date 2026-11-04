package statusreport

import (
	applicationapiv1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	"github.com/konflux-ci/integration-service/gitops"
	intgteststat "github.com/konflux-ci/integration-service/pkg/integrationteststatus"
	"github.com/konflux-ci/integration-service/pkg/keys"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("new-style status-report metadata", func() {
	It("reads a new-style group snapshot failure reason", func() {
		snapshot := &applicationapiv1alpha1.Snapshot{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					keys.Test("create-groupsnapshot-status").New: gitops.FailedToCreateGroupSnapshotMsg + ": details",
				},
			},
		}

		detail, err := generateIntgTestStatusDetails(snapshot, intgteststat.GroupSnapshotCreationFailed)
		Expect(err).NotTo(HaveOccurred())
		Expect(detail.Details).To(ContainSubstring("details"))
	})
})
