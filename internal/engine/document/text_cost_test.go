//go:build !zp_budget_controls_reference

package document_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	. "github.com/ornew/zeroparse/internal/engine"
	"github.com/ornew/zeroparse/internal/engine/core"
	"github.com/ornew/zeroparse/internal/limits"
)

// The grammar takes the text of every level of a nesting. An edit of the center splits the
// document into pieces, so the texts of all levels but the innermost cross pieces; they sum to
// the square of the depth, and the parse is linear only if a text costs a constant.
func TestTextCostIsConstantInDocumentPieces(t *testing.T) {
	t.Parallel()
	if !core.DocumentPieceInput {
		t.Skip("piece input is disabled")
	}
	prog := mustCompile(t, `rule main = v:a $$ => $v
rule a = s:("(" a ")" / [^()]) => text($s)`)
	for _, b := range []Backend{Closure, Bytecode, BytecodeIterative} {
		for _, u := range []PositionUnit{Bytes, CodePoints} {
			t.Run(fmt.Sprintf("%v/%v", b, u), func(t *testing.T) {
				t.Parallel()
				parse := func(depth int, budget uint64) error {
					o := ParseOptions{Backend: b, Unit: u}
					text := strings.Repeat("(", depth) + "x" + strings.Repeat(")", depth)
					d, err := prog.NewDocumentWith("main", text, o)
					if err != nil {
						t.Fatal(err)
					}
					if err := d.Edit(depth, depth+1, "é"); err != nil {
						t.Fatal(err)
					}
					if pieces := d.Input().Pieces; pieces == nil || len(pieces.Parts) < 3 {
						t.Fatal("the edit left the document in one piece")
					}
					o.Limits.Budget = budget
					n, err := d.ParseWith(o)
					if err == nil && n.Value() != any(text[:depth]+"é"+text[depth+1:]) {
						t.Fatalf("value %v", n)
					}
					return err
				}
				var costs []uint64
				depths := []int{4096, 8192, 16384}
				for _, depth := range depths {
					costs = append(costs, leastDocumentBudget(t, func(budget uint64) error { return parse(depth, budget) }))
				}
				// Doubling the depth at most a little more than doubles the cost, where texts that
				// copy would quadruple it.
				for i := 1; i < len(costs); i++ {
					if ratio := float64(costs[i]) / float64(costs[i-1]); ratio > 2.3 {
						t.Fatalf("cost at depths %v is %v: doubling the depth multiplies it by %.2f", depths, costs, ratio)
					}
				}
			})
		}
	}
}

// leastDocumentBudget returns the smallest budget with which parse finishes, by bisection.
func leastDocumentBudget(t *testing.T, parse func(budget uint64) error) uint64 {
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
