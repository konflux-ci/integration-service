/*
Copyright 2022 Red Hat Inc.

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

package status

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"text/template"

	"github.com/go-logr/logr"
	"github.com/konflux-ci/integration-service/gitops"
	"github.com/konflux-ci/integration-service/helpers"
	"knative.dev/pkg/apis"
)

const commentTemplate = `### {{ .Title }}

{{ .Summary }}`

const commentTemplateForSuccessfulTest = `### :heavy_check_mark: {{ .Title }}

{{ .Summary }}`

// summaryTemplate is a template used to generate a markdown summary for a test details with pipelinerun details and TaskRun details for unsuccessful tests.
const summaryTemplate = `
{{- $pipelineRunName := .PipelineRunName -}} {{ $namespace := .Namespace -}} {{ $logger := .Logger -}}
<ul>
<li><b>Pipelinerun</b>: <a href="{{ formatPipelineURL $pipelineRunName $namespace $logger }}">{{ $pipelineRunName }}</a></li>
</ul>
<hr>

| Task | Duration | Test Suite | Status | Details |
| --- | --- | --- | --- | --- |
{{- range $tr := .TaskRuns }}
| <a href="{{ formatTaskLogURL $tr $pipelineRunName $namespace $logger }}">{{ formatTaskName $tr }}</a> | {{ $tr.GetDuration.String }} | {{ formatNamespace $tr }} | {{ formatStatus $tr }} | {{ formatDetails $tr }} |
{{- end }}

{{ formatFootnotes .TaskRuns }}
{{ if .ComponentSnapshotInfos}}
The group snapshot is generated for pr group {{ .PRGroup }} and the component snasphots as below:
| Component | Snapshot | BuildPipelineRun | PullRequest |
| --- | --- | --- | --- |
{{- range $cs := .ComponentSnapshotInfos }}
| {{ $cs.Component }} | {{ $cs.Snapshot }} | <a href="{{ formatPipelineURL $cs.BuildPipelineRun $namespace $logger }}">{{ $cs.BuildPipelineRun }}</a> | <a href="{{ formatPullRequestURL $cs.RepoUrl $cs.PullRequestNumber }}">{{ formatRepoURL $cs.RepoUrl }}</a> |
{{- end }}
{{end}}`

// ShortSummaryTemplate is a more concise version of SummaryTemplate with pipelinerun details but without TaskRun details used to show for passed tests.
const shortSummaryTemplate = `
{{- $pipelineRunName := .PipelineRunName -}} {{ $namespace := .Namespace -}} {{ $logger := .Logger -}}
<ul>
<li><b>Pipelinerun</b>: <a href="{{ formatPipelineURL $pipelineRunName $namespace $logger }}">{{ $pipelineRunName }}</a></li>
</ul>

{{ if .ComponentSnapshotInfos}}
The group snapshot is generated for pr group {{ .PRGroup }} and the component snasphots as below:
| Component | Snapshot | BuildPipelineRun | PullRequest |
| --- | --- | --- | --- |
{{- range $cs := .ComponentSnapshotInfos }}
| {{ $cs.Component }} | {{ $cs.Snapshot }} | <a href="{{ formatPipelineURL $cs.BuildPipelineRun $namespace $logger }}">{{ $cs.BuildPipelineRun }}</a> | <a href="{{ formatPullRequestURL $cs.RepoUrl $cs.PullRequestNumber }}">{{ formatRepoURL $cs.RepoUrl }}</a> |
{{- end }}
{{end}}`

// SummaryTemplateData holds the data necessary to construct a PipelineRun summary.
type SummaryTemplateData struct {
	TaskRuns               []*helpers.TaskRun
	PipelineRunName        string
	Namespace              string
	PRGroup                string
	ComponentSnapshotInfos []*gitops.ComponentSnapshotInfo
	Logger                 logr.Logger
}

// TaskLogTemplateData holds the data necessary to construct a Task log URL.
type TaskLogTemplateData struct {
	TaskName        string
	PipelineRunName string
	Namespace       string
}

// CommentTemplateData holds the data necessary to construct a PipelineRun comment.
type CommentTemplateData struct {
	Title   string
	Summary string
}

// FormatTestsSummary builds a markdown summary for a list of integration TaskRuns.
func FormatTestsSummary(taskRuns []*helpers.TaskRun, pipelineRunName string, namespace string, componentSnapshotInfos []*gitops.ComponentSnapshotInfo, pr_group string, logger logr.Logger) (string, error) {
	funcMap := template.FuncMap{
		"formatTaskName":       FormatTaskName,
		"formatNamespace":      FormatNamespace,
		"formatStatus":         FormatStatus,
		"formatDetails":        FormatDetails,
		"formatPipelineURL":    FormatPipelineURL,
		"formatTaskLogURL":     FormatTaskLogURL,
		"formatFootnotes":      FormatFootnotes,
		"formatPullRequestURL": FormatPullRequestURL,
		"formatRepoURL":        FormatRepoURL,
	}
	buf := bytes.Buffer{}
	data := SummaryTemplateData{TaskRuns: taskRuns, PipelineRunName: pipelineRunName, Namespace: namespace, PRGroup: pr_group, ComponentSnapshotInfos: componentSnapshotInfos, Logger: logger}
	t := template.Must(template.New("").Funcs(funcMap).Parse(summaryTemplate))
	if err := t.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// FormatShortTestsSummary builds a markdown summary for a list of integration TaskRuns.
func FormatShortTestsSummary(pipelineRunName string, namespace string, componentSnapshotInfos []*gitops.ComponentSnapshotInfo, pr_group string) (string, error) {
	funcMap := template.FuncMap{
		"formatNamespace":      FormatNamespace,
		"formatStatus":         FormatStatus,
		"formatDetails":        FormatDetails,
		"formatPipelineURL":    FormatPipelineURL,
		"formatFootnotes":      FormatFootnotes,
		"formatPullRequestURL": FormatPullRequestURL,
		"formatRepoURL":        FormatRepoURL,
	}
	buf := bytes.Buffer{}
	data := SummaryTemplateData{PipelineRunName: pipelineRunName, Namespace: namespace, PRGroup: pr_group, ComponentSnapshotInfos: componentSnapshotInfos}
	t := template.Must(template.New("").Funcs(funcMap).Parse(shortSummaryTemplate))
	if err := t.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// FormatComment build a markdown comment with the details in text for unsuccessful tests.
func FormatComment(title, text string) (string, error) {
	buf := bytes.Buffer{}
	data := CommentTemplateData{Title: title, Summary: text}
	t := template.Must(template.New("").Parse(commentTemplate))
	if err := t.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// FormatCommentForSuccessfulTest build a markdown comment with the details in text for successful tests.
func FormatCommentForSuccessfulTest(title, text string) (string, error) {
	buf := bytes.Buffer{}
	data := CommentTemplateData{Title: title, Summary: text}
	t := template.Must(template.New("").Parse(commentTemplateForSuccessfulTest))
	if err := t.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// FormatStatus accepts a TaskRun and returns a Markdown friendly representation of its overall status, if any.
func FormatStatus(taskRun *helpers.TaskRun) (string, error) {
	result, err := taskRun.GetTestResult()
	if err != nil {
		return "", err
	}

	var emoji string
	if result == nil || result.TestOutput == nil {
		taskSucceededReason := taskRun.GetStatusCondition(string(apis.ConditionSucceeded)).GetReason()
		switch taskSucceededReason {
		case "Succeeded":
			emoji = ":heavy_check_mark:"
		case "Failed":
			emoji = ":x:"
		default:
			emoji = ":question:"
		}
		return fmt.Sprintf(emoji+" Reason: %s", taskSucceededReason), nil
	}

	switch result.TestOutput.Result {
	case helpers.AppStudioTestOutputSuccess:
		emoji = ":heavy_check_mark:"
	case helpers.AppStudioTestOutputFailure:
		emoji = ":x:"
	case helpers.AppStudioTestOutputWarning:
		emoji = ":warning:"
	case helpers.AppStudioTestOutputSkipped:
		emoji = ":white_check_mark:"
	case helpers.AppStudioTestOutputError:
		emoji = ":heavy_exclamation_mark:"
	default:
		emoji = ":question:"
	}

	return emoji + " " + result.TestOutput.Result, nil
}

// FormatTaskName accepts a TaskRun and returns a Markdown friendly representation of its name.
func FormatTaskName(taskRun *helpers.TaskRun) (string, error) {
	result, err := taskRun.GetTestResult()
	if err != nil {
		return "", err
	}

	name := taskRun.GetPipelineTaskName()

	if result == nil || result.TestOutput == nil {
		return name, nil
	}

	if result.TestOutput.Note == "" {
		return name, nil
	}

	return name + "[^" + name + "]", nil
}

// FormatNamespace accepts a TaskRun and returns a Markdown friendly representation of its test suite, if any.
func FormatNamespace(taskRun *helpers.TaskRun) (string, error) {
	result, err := taskRun.GetTestResult()
	if err != nil {
		return "", err
	}
	if result == nil || result.TestOutput == nil {
		return "", nil
	}

	return result.TestOutput.Namespace, nil
}

// FormatDetails accepts a TaskRun and returns a Markdown friendly representation of its detailed test results, if any.
func FormatDetails(taskRun *helpers.TaskRun) (string, error) {
	result, err := taskRun.GetTestResult()
	if err != nil {
		return "", err
	}

	if result == nil {
		var emoji string
		taskSucceededReason := taskRun.GetStatusCondition(string(apis.ConditionSucceeded)).GetReason()
		switch taskSucceededReason {
		case "Succeeded":
			emoji = ":heavy_check_mark:"
		case "Failed":
			emoji = ":x:"
		default:
			emoji = ":question:"
		}
		return fmt.Sprintf(emoji+" Reason: %s", taskSucceededReason), nil
	}

	if result.ValidationError != nil {
		return fmt.Sprintf("Invalid result: %s", result.ValidationError), nil
	}
	details := []string{}
	if result.TestOutput.Successes > 0 {
		details = append(details, fmt.Sprint(":heavy_check_mark: ", result.TestOutput.Successes, " success(es)"))
	}

	if result.TestOutput.Warnings > 0 {
		details = append(details, fmt.Sprint(":warning: ", result.TestOutput.Warnings, " warning(s)"))
	}

	if result.TestOutput.Failures > 0 {
		details = append(details, fmt.Sprint(":x: ", result.TestOutput.Failures, " failure(s)"))
	}

	return strings.Join(details, "<br>"), nil
}

// FormatResults accepts a list of TaskRuns and returns a Markdown friendly representation of their footnotes, if any.
func FormatFootnotes(taskRuns []*helpers.TaskRun) (string, error) {
	footnotes := []string{}
	for _, tr := range taskRuns {
		result, err := tr.GetTestResult()
		if err != nil {
			return "", err
		}

		if result == nil || result.TestOutput == nil {
			continue
		}

		if result.TestOutput.Note != "" {
			footnotes = append(footnotes, "[^"+tr.GetPipelineTaskName()+"]: "+result.TestOutput.Note)
		}
	}
	return strings.Join(footnotes, "\n"), nil
}

// FormatPipelineURL accepts a name of application, pipelinerun, namespace and returns a complete pipelineURL.
func FormatPipelineURL(pipelinerun string, namespace string, logger logr.Logger) string {
	console_url := os.Getenv("CONSOLE_URL")
	if console_url == "" {
		return "https://CONSOLE_URL_NOT_AVAILABLE"
	}
	buf := bytes.Buffer{}
	data := SummaryTemplateData{PipelineRunName: pipelinerun, Namespace: namespace}
	t := template.Must(template.New("").Parse(console_url))
	if err := t.Execute(&buf, data); err != nil {
		logger.Error(err, "Error occured when executing template.")
	}
	return buf.String()
}

// FormatPullRequestURL accepts a name of application, pipelinerun, namespace and returns a complete pipelineURL.
func FormatPullRequestURL(repoUrl string, pullRequestNumber string) string {
	pullRequestUrl := "https://PULLREQUEST_URL_NOT_AVAILABLE"

	if strings.Contains(repoUrl, "https://github") {
		pullRequestUrl = repoUrl + "/pull/" + pullRequestNumber
	} else if strings.Contains(repoUrl, "https://gitlab") {
		pullRequestUrl = repoUrl + "/-/merge_requests/" + pullRequestNumber
	}
	return pullRequestUrl
}

func FormatRepoURL(repoUrl string) string {
	repoName := "NOT_AVAILABLE"
	if repoUrl != "" {
		repoUrlStrings := strings.Split(repoUrl, "/")
		repoName = repoUrlStrings[len(repoUrlStrings)-1]
	}
	return repoName
}

// FormatTaskLogURL accepts name of pipelinerun, task, namespace and returns a complete task log URL.
func FormatTaskLogURL(taskRun *helpers.TaskRun, pipelinerun string, namespace string, logger logr.Logger) string {
	consoleTaskLogURL := os.Getenv("CONSOLE_URL_TASKLOG")
	if consoleTaskLogURL == "" {
		return "https://CONSOLE_URL_TASKLOG_NOT_AVAILABLE"
	}

	taskName := taskRun.GetPipelineTaskName()
	buf := bytes.Buffer{}
	data := TaskLogTemplateData{PipelineRunName: pipelinerun, TaskName: taskName, Namespace: namespace}
	t := template.Must(template.New("").Parse(consoleTaskLogURL))
	if err := t.Execute(&buf, data); err != nil {
		logger.Error(err, "Error occured when executing task log template.")
	}
	return buf.String()
}
