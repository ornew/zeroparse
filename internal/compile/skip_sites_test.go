package compile

import (
	"slices"
	"testing"

	"github.com/ornew/zeroparse/grammar"
)

// TestSkipSites checks which repetitions are skip-run sites: each condition of skip_sites.go
// excludes a repetition that the case beside it, which differs only in that condition, marks.
func TestSkipSites(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		sites     []string // the formatted sites, in order
	}{
		{"scan", `rule main = ("b" #recover(skip=[a]+)) "c"`, []string{"[a]+"}},
		{"element", `rule main = ("(" main ")") #recover(skip=[a(]+)`, []string{"[a(]+"}},
		{"outermost only", `rule main = "b" #recover(skip=("x" [a]*)*)`, []string{`("x" [a]*)*`}},
		{"in a sequence", `rule main = "b" #recover(skip="x" [a]* "y"+)`, []string{"[a]*", `"y"+`}},
		{"bounded", `rule main = "b" #recover(skip=[a] #repeat(1, 3))`, nil},
		{"bounded outer, inner site", `rule main = "b" #recover(skip=([a]*) #repeat(0, 3))`, []string{"[a]*"}},
		{"outside skips", `rule main = [a]* ("b" #recover(skip=[a]+))`, []string{"[a]+"}},
		{"variable read", "var v int\nrule main = \"b\" #recover(skip=([a] {v > 0})*)", nil},
		{"variable bound", "var v int\nrule main = \"b\" #recover(skip=([a] {v := 1})*)", nil},
		{"predicate without variables", "rule main = \"b\" #recover(skip=([a] {1 > 0})*)", []string{"([a] {1 > 0})*"}},
		{"callee reads a variable", "var v int\nrule main = \"b\" #recover(skip=s*)\nrule s = [a] {v > 0}", nil},
		{"callee binds a variable", "var v int\nrule main = \"b\" #recover(skip=s*)\nrule s = [a] {v := 1}", []string{"s*"}},
		{"callee recovers", "rule main = \"b\" #recover(skip=s*)\nrule s = t\nrule t = \"x\" #recover(skip=[a])", nil},
		{"callee is left-recursive", "rule main = \"b\" #recover(skip=s*)\nrule s = s \"a\" / \"a\"", nil},
		{"callee reaches left recursion", "rule main = \"b\" #recover(skip=u*)\nrule u = s\nrule s = s \"a\" / \"a\"", nil},
		{"plain callee", "rule main = \"b\" #recover(skip=s*)\nrule s = \"a\" / \"x\"", []string{"s*"}},
		{"escaping cut", `rule main = "b" #recover(skip=("a" #cut "x")*)`, nil},
		{"cut in a choice", `rule main = "b" #recover(skip=("a" #cut "x" / "y")*)`, []string{`("a" #cut "x" / "y")*`}},
		{"cut in a lookahead", `rule main = "b" #recover(skip=(&("a" #cut) "a")*)`, []string{`(&("a" #cut) "a")*`}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			prog := compileBare(t, tc.src)
			var got []string
			n := 0
			for _, r := range prog.RuleList {
				WalkExpr(r.Def.Expr, func(x grammar.Expr) {
					if rep, ok := x.(*grammar.Repeat); ok {
						if site := prog.SkipSite(rep); site != 0 {
							n++
							if site != n {
								t.Errorf("site %s numbered %d, want %d", grammar.FormatExpr(rep), site, n)
							}
							got = append(got, grammar.FormatExpr(rep))
						}
					}
				})
			}
			if !slices.Equal(got, tc.sites) || prog.NumSkipSites != len(tc.sites) {
				t.Errorf("sites %q (%d), want %q", got, prog.NumSkipSites, tc.sites)
			}
		})
	}
}
