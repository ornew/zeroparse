package gogen_test

import (
	"path/filepath"
	"runtime"
	"testing"

	"zeroparse.dev/zp/internal/gen"
	"zeroparse.dev/zp/internal/gentest"
)

func TestPredicateBlocksGeneratedGoParity(t *testing.T) {
	t.Parallel()
	gentest.EachMode(t, testPredicateBlocksGeneratedGoParity)
}

func testPredicateBlocksGeneratedGoParity(t *testing.T) {
	gentest.GeneratedParsersCorpus(t, filepath.Join(runtime.GOROOT(), "bin", "go"), gentest.PredicateBlockGeneratedCases(), gen.GenOptions{})
}
