package gogen_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/ornew/zeroparse/internal/gen"
	"github.com/ornew/zeroparse/internal/gentest"
	"github.com/ornew/zeroparse/internal/ir/exec/core"
)

// The grammar takes the text of every level of a nesting: the texts sum to the square of the
// depth, so the parse is linear only if a text costs a constant. The consumer checks it for
// whole-input parses and Documents in bytes, in code points and on invalid UTF-8.
func TestGeneratedGoTextCostIsConstant(t *testing.T) {
	t.Parallel()
	gentest.NeedsBudgetControls(t)
	if testing.Short() {
		t.Skip("builds a generated consumer")
	}
	g, err := parseTest(`rule main = v:a $$ => $v
rule a = s:("(" a ")" / [^()]) => text($s)`)
	if err != nil {
		t.Fatal(err)
	}
	code, err := gentest.GenerateCode(t, g, gen.GenOptions{Package: "fixture", Start: "main", Document: true})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	for name, source := range map[string]string{
		"go.mod":         gentest.GoMod("fixture"),
		"parser.go":      string(code),
		"parser_test.go": generatedGoTextCostConsumer,
	} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(source), 0644); err != nil {
			t.Fatal(err)
		}
	}
	cmd := exec.Command("go", "test", "-count=1", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOFLAGS="+core.GoFlags(os.Getenv("GOFLAGS")))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generated text cost: %v\n%s", err, out)
	}
}

const generatedGoTextCostConsumer = `package fixture

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	zprt "github.com/ornew/zeroparse/runtime"
)

func leastBudget(t *testing.T, parse func(budget uint64) error) uint64 {
	finishes := func(budget uint64) bool {
		err := parse(budget)
		var le *zprt.LimitError
		if errors.As(err, &le) && le.Kind == zprt.LimitBudget {
			return false
		}
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		return true
	}
	lo, hi := uint64(0), uint64(1)
	for !finishes(hi) {
		lo, hi = hi, hi*2
	}
	for hi-lo > 1 {
		if mid := lo + (hi-lo)/2; finishes(mid) {
			hi = mid
		} else {
			lo = mid
		}
	}
	return hi
}

func TestTextCostIsConstant(t *testing.T) {
	for _, in := range []struct {
		name   string
		unit   PositionUnit
		center string
	}{{"bytes", Bytes, "x"}, {"code points", CodePoints, "é"}, {"invalid UTF-8", CodePoints, "\xff"}} {
		for _, document := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/document=%t", in.name, document), func(t *testing.T) {
				input := func(depth int) string {
					return strings.Repeat("(", depth) + in.center + strings.Repeat(")", depth)
				}
				parse := func(depth int, budget uint64) error {
					text := input(depth)
					var n *Node
					var err error
					if document {
						d, derr := NewDocument(text, WithUnit(in.unit))
						if derr != nil {
							t.Fatal(derr)
						}
						n, err = d.Parse(WithLimits(Limits{Budget: budget}))
					} else {
						n, err = Parse(text, WithUnit(in.unit), WithLimits(Limits{Budget: budget}))
					}
					if want := strings.ToValidUTF8(text, "�"); err == nil && n.Value() != any(want) {
						t.Fatalf("value %v, want %q", n, want)
					}
					return err
				}
				depths := []int{1024, 2048, 4096}
				var costs []uint64
				for _, depth := range depths {
					costs = append(costs, leastBudget(t, func(budget uint64) error { return parse(depth, budget) }))
				}
				for i := 1; i < len(costs); i++ {
					if ratio := float64(costs[i]) / float64(costs[i-1]); ratio > 2.3 {
						t.Fatalf("cost at depths %v is %v: doubling the depth multiplies it by %.2f", depths, costs, ratio)
					}
				}
			})
		}
	}
}
`
