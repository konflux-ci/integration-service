//go:build coverage

package main

// This file is only included when building with -tags=coverage.
// It starts a coverage HTTP server that allows collecting coverage data
// from the running binary during E2E tests.

import _ "github.com/konflux-ci/coverport/instrumentation/go" // starts coverage server via init()
