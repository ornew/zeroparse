package complexitytest

import (
	"os"
	"testing"

	"zeroparse.dev/zp/internal/engine/core"
	"zeroparse.dev/zp/internal/gentest"
)

// skipWithoutBudget skips the rest of t in the reference build, which has no budget controls: the
// tests measure the cost of a parse as the budget it spends.
func skipWithoutBudget(t *testing.T) {
	t.Helper()
	if !core.BudgetControlsEnabled {
		t.Skip("budget controls are unavailable in the reference build")
	}
}

// TestMain gives the go commands that the tests run, which build generated parsers, a build cache
// of their own, removed at the end.
func TestMain(m *testing.M) {
	remove := gentest.SetGoCache()
	code := m.Run()
	remove()
	os.Exit(code)
}
