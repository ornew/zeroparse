package engine

import (
	"fmt"
	"strings"
	"testing"

	"zeroparse.dev/zp/internal/compile"
	"zeroparse.dev/zp/internal/engine/core"
	"zeroparse.dev/zp/internal/gentest"
)

// depMemoGrammars returns the grammars that the dependency memo tests compare
// (gentest.DependencyMemoCases).
func depMemoGrammars(seeds int) []gentest.Case {
	return gentest.DependencyMemoCases(seeds)
}

// depMemoDiffers compares, on every backend, the results of the grammar with the dependency
// memo and the results of the grammar compiled without a memo that evaluates every call in a
// context again (ParseOptions.ContextMemoOff), and returns the differences, ignoring an input that
// exhausts a limit in either. With unexamined, the memo leaves examined ranges out of identities
// and outcomes (ParseOptions.ContextMemoUnexamined). It returns false if the grammar does not
// compile.
func depMemoDiffers(t *testing.T, c gentest.Case, limit int, unexamined bool) ([]string, bool) {
	t.Helper()
	g, err := parseTest(c.Src)
	if err != nil {
		return nil, false
	}
	if _, err := Compile(g, compile.Options{}); err != nil {
		return nil, false
	}
	memo, ref := mustCompile(t, c.Src), mustCompile(t, c.Src, compile.Options{DisableMemo: true})
	var diffs []string
	for _, in := range c.Inputs {
		for _, backend := range []Backend{Closure, Bytecode, BytecodeIterative} {
			for _, recognize := range []bool{false, true} {
				o := ParseOptions{Backend: backend, Recognize: recognize, ContextMemoUnexamined: unexamined}
				got, err := memo.ParseWith("", in, o)
				o.ContextMemoUnexamined, o.ContextMemoOff = false, true
				want, wantErr := ref.ParseWith("", in, o)
				a, b := gentest.ResultJSON(got, err), gentest.ResultJSON(want, wantErr)
				if a != b && !strings.Contains(a+b, "budget") && !strings.Contains(a+b, "limit") {
					diffs = append(diffs, fmt.Sprintf("%s %q %s recognize=%v:\n got %s\nwant %s", c.Name, in, backend, recognize, a, b))
					if len(diffs) >= limit {
						return diffs, true
					}
				}
			}
		}
	}
	return diffs, true
}

// TestLeftRecursionDependencyMemoRandom compares the results of random grammars with left
// recursion through several leaders with the results of evaluating every call in a context again.
func TestLeftRecursionDependencyMemoRandom(t *testing.T) {
	t.Parallel()
	seeds := 800
	if testing.Short() {
		seeds = 80
	}
	compiled, n := 0, 0
	for _, c := range depMemoGrammars(seeds) {
		diffs, ok := depMemoDiffers(t, c, 3, false)
		if !ok {
			continue
		}
		compiled++
		for _, d := range diffs {
			if n++; n <= 5 {
				t.Errorf("%s\n%s", c.Src, d)
			}
		}
	}
	if n > 0 || compiled < seeds*2 {
		t.Errorf("%d differences in %d grammars", n, compiled)
	}
}

// TestLeftRecursionDependencyMemoControls checks that the random grammars of
// TestLeftRecursionDependencyMemoRandom detect each check of the dependency memo: with the check
// broken, some result differs from evaluating every call in a context again. It changes a global
// and so does not run in parallel.
func TestLeftRecursionDependencyMemoControls(t *testing.T) {
	seeds := 400
	if testing.Short() {
		seeds = 120
	}
	cases := depMemoGrammars(seeds)
	for _, m := range []struct {
		name       string
		mutation   int
		unexamined bool // a difference shows only without examined ranges
	}{
		{"seed identities", core.CtxIgnoreSeeds, false},
		{"observations", core.CtxIgnoreObs, false},
		{"present growths", core.CtxIgnorePresent, false},
		{"canonical seeds", core.CtxNoCanonical, false},
		{"jumps over observations of the growing seed", core.CtxJumpAnySeed, false},
		{"replayed observations", core.CtxStaleReplay, true},
		{"observations replayed in the current context", core.CtxNoReplayObs, true},
		{"lookaheads with recovered errors", core.CtxLookErrors, false},
	} {
		t.Run(m.name, func(t *testing.T) {
			core.ContextMemoMutation = m.mutation
			defer func() { core.ContextMemoMutation = 0 }()
			for _, c := range cases {
				if diffs, _ := depMemoDiffers(t, c, 1, m.unexamined); len(diffs) > 0 {
					return
				}
			}
			t.Errorf("no difference in %d grammars", len(cases))
		})
	}
}

// TestLeftRecursionDependencyMemoLinear checks that the shapes of gentest.DependencyMemoShapes
// evaluate a number of calls linear in the length of the input on every backend: doubling the input
// at most a little more than doubles the evaluations.
func TestLeftRecursionDependencyMemoLinear(t *testing.T) {
	t.Parallel()
	for _, s := range gentest.DependencyMemoShapes {
		prog := mustCompile(t, "@start\nrule main = "+s.Rules, compile.Options{SourceInfo: true})
		for _, b := range []Backend{Closure, Bytecode, BytecodeIterative} {
			evals := func(n int) int {
				var pr Profile
				if _, err := prog.ParseWith("", s.Input(n), ParseOptions{Backend: b, Trace: pr.Trace}); err != nil {
					t.Fatalf("%s %s n=%d: %v", s.Name, b, n, err)
				}
				return pr.Evals
			}
			small, large := evals(80), evals(160)
			if large > small*5/2 {
				t.Errorf("%s %s: %d evaluations at 80, %d at 160", s.Name, b, small, large)
			}
		}
	}
}

// TestLeftRecursionDependencyMemoDefaultBudget checks that the shapes of
// gentest.DependencyMemoShapes parse a long input within the default budget, which grows linearly
// with the input, on every backend. The budget also counts the work that evaluations do not show,
// such as the scan of the jumps recorded for a seed.
func TestLeftRecursionDependencyMemoDefaultBudget(t *testing.T) {
	t.Parallel()
	for _, s := range gentest.DependencyMemoShapes {
		prog := mustCompile(t, "@start\nrule main = "+s.Rules)
		in := s.Input(10000)
		for _, b := range []Backend{Closure, Bytecode, BytecodeIterative} {
			if _, err := prog.ParseWith("", in, ParseOptions{Backend: b}); err != nil {
				t.Errorf("%s %s n=10000: %v", s.Name, b, err)
			}
		}
	}
}

// TestLeftRecursionDependencyMemoUnexamined checks that leaving the examined range out of the
// identities of growth steps and the outcomes of lookaheads (ParseOptions.ContextMemoUnexamined,
// what generated parsers do outside a Document parse) changes only what is evaluated: the random
// grammars give the results of evaluating every call in a context again, and on mutual leaders it
// takes a step that the examined range keeps apart.
func TestLeftRecursionDependencyMemoUnexamined(t *testing.T) {
	t.Parallel()
	seeds := 200
	if testing.Short() {
		seeds = 40
	}
	n := 0
	compiled := 0
	for _, c := range depMemoGrammars(seeds) {
		if g, err := parseTest(c.Src); err != nil {
			continue
		} else if _, err := Compile(g, compile.Options{}); err != nil {
			continue
		}
		compiled++
		memo, ref := mustCompile(t, c.Src), mustCompile(t, c.Src, compile.Options{DisableMemo: true})
		for _, in := range c.Inputs {
			for _, backend := range []Backend{Closure, Bytecode, BytecodeIterative} {
				got, err := memo.ParseWith("", in, ParseOptions{Backend: backend, ContextMemoUnexamined: true})
				want, wantErr := ref.ParseWith("", in, ParseOptions{Backend: backend, ContextMemoOff: true})
				a, b := gentest.ResultJSON(got, err), gentest.ResultJSON(want, wantErr)
				if a != b && !strings.Contains(a+b, "budget") && !strings.Contains(a+b, "limit") {
					if n++; n <= 5 {
						t.Errorf("%s\n%q %s:\n got %s\nwant %s", c.Src, in, backend, a, b)
					}
				}
			}
		}
	}
	if n > 0 || compiled < seeds*2 {
		t.Errorf("%d differences in %d grammars", n, compiled)
	}
	prog := mustCompile(t, "rule main = a $$\nrule a = a \"x\" / b \"y\" / \"n\"\nrule b = b \"z\" / a \"w\" / \"m\"")
	for _, b := range []Backend{Closure, Bytecode, BytecodeIterative} {
		evals := func(unexamined bool) int {
			var pr Profile
			if _, err := prog.ParseWith("", "mzy", ParseOptions{Backend: b, Trace: pr.Trace, ContextMemoUnexamined: unexamined}); err != nil {
				t.Fatalf("%s: %v", b, err)
			}
			return pr.Evals
		}
		if strict, loose := evals(false), evals(true); loose >= strict {
			t.Errorf("%s: %d evaluations without examined ranges, %d with them", b, loose, strict)
		}
	}
}
