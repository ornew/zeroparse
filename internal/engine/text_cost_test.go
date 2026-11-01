//go:build !zp_budget_controls_reference

package engine

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"zeroparse.dev/zp/internal/engine/core"
	"zeroparse.dev/zp/internal/limits"
)

// nestedTextGrammar takes the text of every level of a nesting: the texts sum to the square of
// the depth, so the parse is linear only if a text costs a constant.
const nestedTextGrammar = `rule main = v:a $$ => $v
rule a = s:("(" a ")" / [^()]) => text($s)`

func nestedTextInput(depth int, center string) string {
	return strings.Repeat("(", depth) + center + strings.Repeat(")", depth)
}

// leastParseBudget returns the smallest budget with which parse finishes, by bisection.
func leastParseBudget(t *testing.T, parse func(budget uint64) error) uint64 {
	t.Helper()
	finishes := func(budget uint64) bool {
		err := parse(budget)
		if errors.Is(err, limits.ErrLimit) {
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

// assertLinearText checks that the least budget of a parse of each depth grows in proportion to
// the depth: doubling it at most a little more than doubles the cost, where the cost of texts
// that copy would quadruple it.
func assertLinearText(t *testing.T, depths []int, parse func(depth int, budget uint64) error) {
	t.Helper()
	var costs []uint64
	for _, depth := range depths {
		costs = append(costs, leastParseBudget(t, func(budget uint64) error { return parse(depth, budget) }))
	}
	for i := 1; i < len(costs); i++ {
		if ratio := float64(costs[i]) / float64(costs[i-1]); ratio > 2.3 {
			t.Fatalf("cost at depths %v is %v: doubling the depth multiplies it by %.2f", depths, costs, ratio)
		}
	}
}

func TestTextCostIsConstantInWholeInputs(t *testing.T) {
	t.Parallel()
	prog := mustCompile(t, nestedTextGrammar)
	inputs := []struct {
		name   string
		unit   PositionUnit
		center string
	}{
		{"bytes", Bytes, "x"},
		{"code points", CodePoints, "é"},
		{"invalid UTF-8", CodePoints, "\xff"},
	}
	for _, b := range []Backend{Closure, Bytecode, BytecodeIterative} {
		for _, in := range inputs {
			t.Run(fmt.Sprintf("%v/%s", b, in.name), func(t *testing.T) {
				t.Parallel()
				n, err := prog.ParseWith("main", nestedTextInput(2, in.center), ParseOptions{Backend: b, Unit: in.unit})
				if want := "((" + strings.ToValidUTF8(in.center, "�") + "))"; err != nil || n == nil || n.Value() != any(want) {
					t.Fatalf("value %v, %v; want %q", n, err, want)
				}
				assertLinearText(t, []int{1024, 2048, 4096}, func(depth int, budget uint64) error {
					_, err := prog.ParseWith("main", nestedTextInput(depth, in.center), ParseOptions{Backend: b, Unit: in.unit, Limits: limits.Limits{Budget: budget}})
					return err
				})
			})
		}
	}
}

func TestTextDecodesInvalidInputOnce(t *testing.T) {
	t.Parallel()
	source := strings.Repeat("é\xff", 1000)
	want := strings.ToValidUTF8(source, "�")
	c := core.NewBudgetControl(nil, limits.Limits{Budget: 1 << 30})
	in := core.NewControlledTextInput(source, CodePoints, false, nil, c)

	// The decoding is interruptible and leaves nothing behind.
	c.Limits.Budget = c.Used + 100
	if err := catchFatal(t, func() { _ = in.Text(0, 2000) }); !errors.Is(err, limits.ErrLimit) {
		t.Fatalf("decoding interruption: %v", err)
	}
	c.Limits.Budget = 1 << 30
	if got := in.Text(0, 2000); got != want {
		t.Fatal("text after an interrupted decoding differs")
	}
	used := c.Used
	if got := in.Text(1, 1999); got != want[len("é"):len(want)-len("�")] {
		t.Fatal("decoded substring differs")
	}
	if c.Used != used {
		t.Fatalf("text of a decoded input spent %d", c.Used-used)
	}

	// An edit of a flat Document decodes its source again.
	doc := core.NewTextInput("a\xffb", CodePoints, false)
	if got := doc.Text(0, 3); got != "a�b" {
		t.Fatalf("text %q", got)
	}
	doc.Replace(1, 2, "\xfe\xfd")
	if got := doc.Text(0, 4); got != "a��b" {
		t.Fatalf("text after an edit %q", got)
	}
}

func TestDocumentPieceTextFlattensOnce(t *testing.T) {
	t.Parallel()
	if !core.DocumentPieceInput {
		t.Skip("piece input is disabled")
	}
	for _, unit := range []PositionUnit{Bytes, CodePoints} {
		pieces := core.NewDocumentPieces(strings.Repeat("é", 8192), unit)
		pieces.Replace(10, 11, "x\xff")
		c := core.NewBudgetControl(nil, limits.Limits{Budget: 1 << 30})
		in := core.Input{Unit: unit, Eof: true, BaseLine: 1, BaseCol: 1, Pieces: pieces, Control: c}
		flat := strings.Repeat("é", 8192)
		if unit == Bytes {
			flat = flat[:10] + "x\xff" + flat[11:]
		} else {
			flat = string([]rune(flat)[:10]) + "x�" + string([]rune(flat)[11:])
		}
		at := func(i int) int { // the byte offset of position i of flat
			if unit == Bytes {
				return i
			}
			return len(string([]rune(flat)[:i]))
		}
		n := pieces.N
		// A text within one piece neither copies nor flattens.
		if got := in.Text(100, 200); got != flat[at(100):at(200)] || c.Used != 0 || pieces.FullOK {
			t.Fatalf("%v: text within a piece spent %d (flattened %t)", unit, c.Used, pieces.FullOK)
		}
		// Texts across pieces are copied while the copies stay shorter than the document;
		// the one that would reach its length flattens the document.
		for range (n - 1) / 95 {
			if got := in.Text(5, 100); got != flat[at(5):at(100)] || pieces.FullOK {
				t.Fatalf("%v: crossing text differs or flattened early", unit)
			}
		}
		if got := in.Text(5, 100); got != flat[at(5):at(100)] || !pieces.FullOK {
			t.Fatalf("%v: crossing text differs or did not flatten", unit)
		}
		if c.Used > uint64(4*len(flat)) {
			t.Fatalf("%v: copies and flattening spent %d for a document of %d bytes", unit, c.Used, len(flat))
		}
		used := c.Used
		for _, r := range [][2]int{{0, n}, {3, n - 3}, {9, 12}} {
			if got := in.Text(r[0], r[1]); got != flat[at(r[0]):at(r[1])] {
				t.Fatalf("%v: text %v differs", unit, r)
			}
		}
		if c.Used != used {
			t.Fatalf("%v: texts of a flattened document spent %d", unit, c.Used-used)
		}
		// An edit discards the flattened copy and the count of copies.
		pieces.Replace(0, 1, "y")
		if pieces.FullOK {
			t.Fatalf("%v: an edit kept the flattened copy", unit)
		}
		if got := in.Text(0, 12); got[0] != 'y' || got[1:] != flat[at(1):at(12)] || pieces.FullOK {
			t.Fatalf("%v: text after an edit %q", unit, got)
		}
	}
}
