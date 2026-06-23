// TEMPORARY: CI lint smoke test — delete after verifying linters. Do not merge.
package lintsmoke

import (
	"net/http"
	"os"
	"testing"

	. "github.com/onsi/ginkgo/v2"
)

// --- errcheck (+ golangci gosec G104 on test files): unchecked error return ---
// Standalone gosec skips *_test.go, so these are visible to golangci-lint only.
func uncheckedErrors() {
	os.Remove("/tmp/lint-smoke-delete-me")
	http.Get("http://example.com")
}

// --- ginkgolinter: focused containers are forbidden (must live in *_test.go) ---
var _ = FDescribe("lint smoke focused container", func() {
	FIt("should be flagged by ginkgolinter", func() {})
})

// Compile-only test; does not call uncheckedErrors() to avoid network/filesystem side effects.
func TestLintSmoke(t *testing.T) {}
