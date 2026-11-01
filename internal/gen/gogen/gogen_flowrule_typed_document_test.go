package gogen_test

import (
	"testing"

	"zeroparse.dev/zp/internal/gen"
	"zeroparse.dev/zp/internal/gentest"
)

func TestGenerateDirectTypedDocument(t *testing.T) {
	t.Parallel()
	gentest.DirectTypedDocument(t, gen.GenOptions{})
}
