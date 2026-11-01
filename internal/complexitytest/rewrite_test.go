package complexitytest

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"zeroparse.dev/zp"
	"zeroparse.dev/zp/grammar"
)

func controlByName(t *testing.T, name string) Control {
	t.Helper()
	for _, c := range Controls() {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("no control %q", name)
	return Control{}
}

// rank orders growth: linear below every polynomial, by degree, below exponential.
func rank(o Order) int {
	switch o.Kind {
	case KindLinear:
		return 1
	case KindPoly:
		return 1 + o.Degree
	default:
		return 1000
	}
}

// TestRewritePairs checks that every rewrite of the recipe grows slower than its original, and
// that a rewrite of a pattern the recipe calls linear measures linear.
func TestRewritePairs(t *testing.T) {
	t.Parallel()
	for _, rw := range Rewrites() {
		t.Run(rw.Pattern, func(t *testing.T) {
			orig, repl := controlByName(t, rw.Original), controlByName(t, rw.Rewrite)
			if rank(repl.Order) >= rank(orig.Order) {
				t.Errorf("%s grows as %v, not slower than %s as %v", repl.Name, repl.Order, orig.Name, orig.Order)
			}
			if repl.Order.Kind != KindLinear {
				t.Errorf("%s: a rewrite measures linear, got %v", repl.Name, repl.Order)
			}
		})
	}
}

// TestRewriteLanguage checks, on the inputs listed, that a rewrite that is meant to accept the
// same language as its original does.
func TestRewriteLanguage(t *testing.T) {
	t.Parallel()
	for _, rw := range Rewrites() {
		if len(rw.Language) == 0 {
			continue
		}
		t.Run(rw.Pattern, func(t *testing.T) {
			po, err := Compile(controlByName(t, rw.Original).Source)
			if err != nil {
				t.Fatal(err)
			}
			pr, err := Compile(controlByName(t, rw.Rewrite).Source)
			if err != nil {
				t.Fatal(err)
			}
			for _, in := range rw.Language {
				_, eo := po.Parse(in, zp.WithBackend(zp.Closure))
				_, er := pr.Parse(in, zp.WithBackend(zp.Closure))
				if (eo == nil) != (er == nil) {
					t.Errorf("input %q: original error %v, rewrite error %v", in, eo, er)
				}
			}
		})
	}
}

var zpFence = regexp.MustCompile("(?s)```zp\n(.*?)```")

// normalized formats a grammar source without the attributes of its rules, so that the grammars of
// the recipe and the controls compare equal when they differ in layout and the @start line only.
func normalized(t *testing.T, src string) string {
	t.Helper()
	g, err := zp.ParseGrammar(src)
	if err != nil {
		t.Fatalf("%v\n%s", err, src)
	}
	for _, rd := range g.Rules() {
		rd.Attrs = nil
	}
	return strings.ReplaceAll(grammar.Format(g), "\n\n", "\n") // blank lines are layout
}

// TestRewriteRecipe checks that the recipe quotes the grammar of every control of Rewrites, so
// that the claims it makes about their cost are the ones measured.
func TestRewriteRecipe(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile("../../docs/cookbook/linear-rewrites.md")
	if err != nil {
		t.Fatal(err)
	}
	var quoted []string
	for _, m := range zpFence.FindAllStringSubmatch(string(data), -1) {
		src := m[1]
		if strings.HasPrefix(src, "// ") { // the name of the file to save
			_, src, _ = strings.Cut(src, "\n")
		}
		if strings.Contains(src, "...") {
			continue
		}
		quoted = append(quoted, normalized(t, src))
	}
	for _, rw := range Rewrites() {
		for _, name := range []string{rw.Original, rw.Rewrite} {
			want := normalized(t, controlByName(t, name).Source)
			found := false
			for _, q := range quoted {
				found = found || q == want
			}
			if !found {
				t.Errorf("the recipe does not quote the grammar of %q:\n%s", name, want)
			}
		}
	}
}
