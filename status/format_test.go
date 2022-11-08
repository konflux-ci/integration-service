package status_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/redhat-appstudio/integration-service/helpers"
	"github.com/redhat-appstudio/integration-service/status"
)

const expectedSummary = `<table>
	<tr>
		<th>Status</th>
		<th>Task</th>
		<th>Test Suite</th>
		<th>Completion Time</th>
	</tr>
	<tr>
		<td>:heavy_check_mark: SUCCESS</td>
		<td>example-task</td>
		<td>example-namespace</td>
		<td>2022-01-01T00:00:00Z</td>
	</tr>
</table>`

var _ = Describe("Formatters", func() {
	It("can construct a summary", func() {
		pass := helpers.HACBSTestResult{
			Result:           "SUCCESS",
			Namespace:        "example-namespace",
			Timestamp:        "1640995200",
			PipelineTaskName: "example-task",
		}
		results := []*helpers.HACBSTestResult{&pass}
		summary, err := status.FormatSummary(results)
		Expect(err).To(BeNil())
		Expect(summary).To(Equal(expectedSummary))
	})

	It("can construct a status line", func() {
		Expect(status.FormatStatus("SUCCESS")).To(Equal(":heavy_check_mark: SUCCESS"))
		Expect(status.FormatStatus("FAILURE")).To(Equal(":x: FAILURE"))
		Expect(status.FormatStatus("WARNING")).To(Equal(":warning: WARNING"))
		Expect(status.FormatStatus("SKIPPED")).To(Equal(":white_check_mark: SKIPPED"))
		Expect(status.FormatStatus("ERROR")).To(Equal(":heavy_exclamation_mark: ERROR"))
		Expect(status.FormatStatus("UNEXPECTED")).To(Equal(":question: UNEXPECTED"))
	})

	It("can construct a human readable timestamp", func() {
		Expect(status.FormatTimestamp("1640995200")).To(Equal("2022-01-01T00:00:00Z"))
		Expect(status.FormatTimestamp("foo")).To(Equal(""))
	})
})
