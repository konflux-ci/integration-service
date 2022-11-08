package status

import (
	"bytes"
	"strconv"
	"text/template"
	"time"

	"github.com/redhat-appstudio/integration-service/helpers"
)

const summaryTemplate = `<table>
	<tr>
		<th>Status</th>
		<th>Task</th>
		<th>Test Suite</th>
		<th>Completion Time</th>
	</tr>
	{{- range $result := .Results }}
	<tr>
		<td>{{ formatStatus $result.Result }}</td>
		<td>{{ $result.PipelineTaskName }}</td>
		<td>{{ $result.Namespace }}</td>
		<td>{{ formatTimestamp $result.Timestamp }}</td>
	</tr>
	{{- end }}
</table>`

// SummaryTemplateData holds the data necessary to construct a PipelineRun summary.
type SummaryTemplateData struct {
	Results []*helpers.HACBSTestResult
}

// FormatSummary builds a markdown summary for a list of HACBS test results.
func FormatSummary(results []*helpers.HACBSTestResult) (string, error) {
	funcMap := template.FuncMap{
		"formatTimestamp": FormatTimestamp,
		"formatStatus":    FormatStatus,
	}
	buf := bytes.Buffer{}
	data := SummaryTemplateData{Results: results}
	t := template.Must(template.New("").Funcs(funcMap).Parse(summaryTemplate))
	if err := t.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// FormatStatus accepts a HACBS result value and returns a Markdown friendly representation.
func FormatStatus(result string) string {
	var emoji string
	switch result {
	case helpers.HACBSTestOutputSuccess:
		emoji = ":heavy_check_mark:"
	case helpers.HACBSTestOutputFailure:
		emoji = ":x:"
	case helpers.HACBSTestOutputWarning:
		emoji = ":warning:"
	case helpers.HACBSTestOutputSkipped:
		emoji = ":white_check_mark:"
	case helpers.HACBSTestOutputError:
		emoji = ":heavy_exclamation_mark:"
	default:
		emoji = ":question:"
	}

	return emoji + " " + result
}

// FormatTimestamp accepts a HACBS result timestamp value and returns a Markdown friendly representation.
func FormatTimestamp(timestamp string) string {
	unixTs, err := strconv.ParseInt(timestamp, 0, 64)
	if err != nil {
		return ""
	}
	return time.Unix(unixTs, 0).UTC().Format(time.RFC3339)
}
