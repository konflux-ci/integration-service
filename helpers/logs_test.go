/*
Copyright 2023.
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

	applicationapiv1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Helpers for logs", Ordered, func() {
	var (
		app    *applicationapiv1alpha1.Application
		log    helpers.IntegrationLogger
		logbuf *bytes.Buffer
	)

	BeforeAll(func() {
		app = &applicationapiv1alpha1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "application-sample",
				Namespace: "default",
			},
			TypeMeta: metav1.TypeMeta{
				Kind: "Application",
			},
			Spec: applicationapiv1alpha1.ApplicationSpec{
				DisplayName: "application-sample",
				Description: "This is an example application",
			},
		}
	})

	BeforeEach(func() {
		logbuf = new(bytes.Buffer)
		log = helpers.IntegrationLogger{Logger: buflogr.NewWithBuffer(logbuf)}
		Expect(log).NotTo(BeNil())
	})

	Context("LogAuditEvent", func() {
		DescribeTable("Logs action kay-value",
			func(action helpers.LogAction, actionStr string) {
				log.LogAuditEvent("msg", app, action)

				Expect(logbuf.String()).Should(ContainSubstring(fmt.Sprintf("action %s", actionStr)))
			},
			Entry("When action is VIEW", helpers.LogActionView, "VIEW"),
			Entry("When action is ADD", helpers.LogActionAdd, "ADD"),
			Entry("When action is UPDATE", helpers.LogActionUpdate, "UPDATE"),
			Entry("When action is DELETE", helpers.LogActionDelete, "DELETE"),
		)

		It("Logs audit key-value", func() {
			log.LogAuditEvent("test", app, helpers.LogActionAdd)
			Expect(logbuf.String()).Should(ContainSubstring("audit true")) // all audit logs must have this
		})

		It("Logs object metadata key-value", func() {
			log.LogAuditEvent("test", app, helpers.LogActionAdd)
			Expect(logbuf.String()).Should(ContainSubstring("namespace default"))
			Expect(logbuf.String()).Should(ContainSubstring("name application-sample"))
			Expect(logbuf.String()).Should(ContainSubstring("controllerKind Application"))
		})

		It("Logs msg value", func() {
			log.LogAuditEvent("testmsg", app, helpers.LogActionAdd)
			Expect(logbuf.String()).Should(ContainSubstring("testmsg")) // just value
		})

		It("Logs extra args", func() {
			log.LogAuditEvent("test", app, helpers.LogActionAdd, "extra", "var")
			Expect(logbuf.String()).Should(ContainSubstring("extra var"))
		})

	})
})
