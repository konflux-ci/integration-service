// TEMPORARY: CI lint smoke test — delete after verifying linters. Do not merge.
//
// Verification plan (two pushes to the same PR):
//
//	Push A — restore gosec triggers below (md5 + unchecked errors), delete lint_smoke_test.go
//	         → expect standalone gosec to FAIL (already verified on PR #1602).
//	Push B — this file + lint_smoke_test.go as committed now
//	         → gosec passes (no violations in non-test files)
//	         → golangci-lint runs (errcheck, gosec, staticcheck, ginkgolinter via .golangci.yml)
//	         → staticcheck standalone runs
package lintsmoke

// --- staticcheck SA4006: ineffectual assignment ---
func ineffectualAssignment() int {
	x := 1
	x = 2 // first value of x is never used
	return x
}

// Unused export keeps the compiler from dropping the function during analysis.
var _ = ineffectualAssignment
