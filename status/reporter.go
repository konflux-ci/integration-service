/*
Copyright 2024 Red Hat Inc.

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

//go:generate mockgen -destination mock_reporter.go -package status github.com/redhat-appstudio/integration-service/status ReporterInterface

import (
	"context"
	"time"

	applicationapiv1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"

	intgteststat "github.com/redhat-appstudio/integration-service/pkg/integrationteststatus"
)

type TestReport struct {
	// FullName describing the snapshot and integration test
	FullName string
	// Name of scenario
	ScenarioName string
	// text with details of test results
	Text string
	// test status
	Status intgteststat.IntegrationTestStatus
	// short summary of test results
	Summary string
	// time when test started
	StartTime *time.Time
	// time when test completed
	CompletionTime *time.Time
}

type ReporterInterface interface {
	// Detect if the reporter can be used with the snapshot
	Detect(*applicationapiv1alpha1.Snapshot) bool
	// Initialize reporter to be able update statuses (authenticate, fetching metadata)
	Initialize(context.Context, *applicationapiv1alpha1.Snapshot) error
	// Get plain reporter name
	GetReporterName() string
	// Update status of the integration test
	ReportStatus(context.Context, TestReport) error
}
