/*
Copyright 2023 Red Hat Inc.

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
	"bytes"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/redhat-appstudio/integration-service/helpers"
	"github.com/tonglil/buflogr"
	"k8s.io/apimachinery/pkg/api/errors"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Helpers for error handlers", Ordered, func() {
	var (
		log    helpers.IntegrationLogger
		logbuf *bytes.Buffer
	)

	BeforeEach(func() {
		logbuf = new(bytes.Buffer)
		log = helpers.IntegrationLogger{Logger: buflogr.NewWithBuffer(logbuf)}
		Expect(log).NotTo(BeNil())
	})

	Context("Handling errors returned from a loader function", func() {
		It("Returns nil in place of error if the resource was not found", func() {
			err := new(errors.StatusError)
			err.ErrStatus = metav1.Status{
				Message: "Resource Not Found",
				Code:    404,
				Status:  "Failure",
				Reason:  metav1.StatusReasonNotFound,
			}

			_, returnedError := helpers.HandleLoaderError(log, err, "Component", "Snapshot")
			Expect(returnedError).To(BeNil())
			Expect(logbuf.String()).Should(ContainSubstring("Declining to proceed with reconciliation"))
		})

		It("Returns the error if the error is not IsNotFound", func() {
			err := new(errors.StatusError)
			err.ErrStatus = metav1.Status{
				Message: "Unknown error",
				Code:    500,
				Status:  "Failure",
				Reason:  metav1.StatusReasonUnknown,
			}

			_, returnedError := helpers.HandleLoaderError(log, err, "Component", "Snapshot")
			Expect(returnedError).NotTo(BeNil())
			Expect(logbuf.String()).Should(ContainSubstring("Failed to get Component from the Snapshot"))
		})
	})

	Context("Handling customized integration error", func() {
		It("Can define an NewEnvironmentNotInNamespaceError", func() {
			err := helpers.NewEnvironmentNotInNamespaceError("env", "namespace")
			Expect(helpers.IsEnvironmentNotInNamespaceError(err)).To(BeTrue())
			Expect(err.Error()).To(Equal("Environment env not found in namespace namespace"))
		})

		It("Can handle non integration error", func() {
			err := fmt.Errorf("failed")
			Expect(helpers.IsEnvironmentNotInNamespaceError(err)).To(BeFalse())
		})

		It("Can define MissingInfoInPipelineRunError", func() {
			err := helpers.MissingInfoInPipelineRunError("pipelineRunName", "revision")
			Expect(helpers.IsMissingInfoInPipelineRunError(err)).To(BeTrue())
			Expect(err.Error()).To(Equal("Missing info revision from pipelinerun pipelineRunName"))
		})

		It("Can handle non integration error", func() {
			err := fmt.Errorf("failed")
			Expect(helpers.IsMissingInfoInPipelineRunError(err)).To(BeFalse())
		})

	})
})
