package lint

import (
	"testing"

	"zeroparse.dev/zp/internal/engine"
	"zeroparse.dev/zp/internal/starttest"
	"zeroparse.dev/zp/internal/syntax"
)

func TestMissingTokenDoesNotProveAChoiceShadowed(t *testing.T) {
	// Insertion permits the first alternative to match a different source
	// prefix. A strict literal-prefix proof must not discard that alternative.
	g, err := syntax.Parse(`rule main = "a" / "ab" #missing`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Compile(starttest.MarkAll(g), engine.Options{}); err != nil {
		t.Fatal(err)
	}
	for _, finding := range runUnchecked(g, "main", options{}) {
		if finding.Check == "shadowed-alternative" {
			t.Fatal(finding)
		}
	}
}
