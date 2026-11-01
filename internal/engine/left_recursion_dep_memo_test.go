package engine

import (
	"fmt"
	"strings"
	"testing"

	"zeroparse.dev/zp/internal/compile"
	"zeroparse.dev/zp/internal/engine/core"
	"zeroparse.dev/zp/internal/gentest"
)

// depMemoGrammars returns the grammars that the dependency memo tests compare: a grammar where
// one growth step of a leader at a position has two different results with the same end, in two
// growths of the leader within the growth of another leader there (B at 0 grows to ("a" "a")@B and,
// with the seed "a"@A of A, to ("a"@A "a")@B), random grammars whose cycles have several leaders
// and members, read inside and outside lookaheads, with and without cuts, and random grammars whose
// items recover from errors.
func depMemoGrammars(seeds int) []gentest.Case {
	cases := []gentest.Case{{
		Name: "equal ends of different results",
		Src: `@start
rule main = A $$
rule A = C / "a" / C / "b" C C
rule B = B "b" / A "a" / C / "a" "a"
rule C = B A / &A "a" "c"`,
		Inputs: []string{"aaa", "aa", "aaab", "aac"},
	}}
	for s := int64(0); s < int64(seeds); s++ {
		cases = append(cases, gentest.RandomCycleGrammar(s, false), gentest.RandomCycleGrammar(s, true), gentest.RandomRecoveringCycleGrammar(s))
	}
	return cases
}

// depMemoDiffers compares, on every backend, the results of the grammar with the dependency
// memo and the results of the grammar compiled without a memo that evaluates every call in a
// context again (ParseOptions.ContextMemoOff), and returns the differences, ignoring an input that
// exhausts a limit in either. It returns false if the grammar does not compile.
func depMemoDiffers(t *testing.T, c gentest.Case, limit int) ([]string, bool) {
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
				o := ParseOptions{Backend: backend, Recognize: recognize}
				got, err := memo.ParseWith("", in, o)
				o.ContextMemoOff = true
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
		diffs, ok := depMemoDiffers(t, c, 3)
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
		name     string
		mutation int
	}{
		{"seed identities", core.CtxIgnoreSeeds},
		{"observations", core.CtxIgnoreObs},
		{"present growths", core.CtxIgnorePresent},
		{"canonical seeds", core.CtxNoCanonical},
	} {
		t.Run(m.name, func(t *testing.T) {
			core.ContextMemoMutation = m.mutation
			defer func() { core.ContextMemoMutation = 0 }()
			for _, c := range cases {
				if diffs, _ := depMemoDiffers(t, c, 1); len(diffs) > 0 {
					return
				}
			}
			t.Errorf("no difference in %d grammars", len(cases))
		})
	}
}

// depMemoShapes are grammars whose leaders grow inside the growth of another leader at the same
// position, with an input family whose parse evaluates a number of calls linear in its length.
var depMemoShapes = []struct {
	name, rules string
	input       func(n int) string
}{
	{"lookahead regrowth", `S $$
rule S = &R S "q" / "y"
rule R = R ("q" / "r") / S "t" / "y"`, func(n int) string { return "y" + strings.Repeat("q", n) }},
	{"nested reads", `S $$
rule S = S "q" / R "s" / "y"
rule R = R "r" / S "t"`, func(n int) string { return "y" + strings.Repeat("ts", n) }},
	{"mutual", `A $$
rule A = A "a" / B "b" / "x"
rule B = B "c" / A "d" / "y"`, func(n int) string { return "x" + strings.Repeat("dcb", n) }},
	{"every step reads a success", `S $$
rule S = &R S "q" / "y"
rule R = &S R "q" / "y"`, func(n int) string { return "y" + strings.Repeat("q", n) }},
	{"every step reads an end", `S $$
rule S = &R S "q" / "y"
rule R = &(S "q") R "q" / "y"`, func(n int) string { return "y" + strings.Repeat("q", n) }},
	{"three leaders", `S $$
rule S = &R S "q" / "y"
rule R = &T R "q" / "y"
rule T = T "q" / S "z" / "y"`, func(n int) string { return "y" + strings.Repeat("q", n) }},
}

// TestLeftRecursionDependencyMemoLinear checks that the shapes of depMemoShapes evaluate a number
// of calls linear in the length of the input on every backend: doubling the input at most a little
// more than doubles the evaluations.
func TestLeftRecursionDependencyMemoLinear(t *testing.T) {
	t.Parallel()
	for _, s := range depMemoShapes {
		prog := mustCompile(t, "@start\nrule main = "+s.rules, compile.Options{SourceInfo: true})
		for _, b := range []Backend{Closure, Bytecode, BytecodeIterative} {
			evals := func(n int) int {
				var pr Profile
				if _, err := prog.ParseWith("", s.input(n), ParseOptions{Backend: b, Trace: pr.Trace}); err != nil {
					t.Fatalf("%s %s n=%d: %v", s.name, b, n, err)
				}
				return pr.Evals
			}
			small, large := evals(80), evals(160)
			if large > small*5/2 {
				t.Errorf("%s %s: %d evaluations at 80, %d at 160", s.name, b, small, large)
			}
		}
	}
}
