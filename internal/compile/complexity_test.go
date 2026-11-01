package compile

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"zeroparse.dev/zp/internal/starttest"
	"zeroparse.dev/zp/internal/syntax"
)

// complexityMemoSources are grammars with the shapes that decide memoization: left-recursion
// cycles with leaders and other members, transient rules, rules called once, #missing in a rule
// and in a callee, and inline actions, which are lowered to hidden rules.
var complexityMemoSources = []string{
	"rule main = e $$\nrule e = e \"+\" t / t\nrule t = t \"*\" f / f\nrule f = \"x\" / \"(\" e \")\"\n",
	"rule main = c $$\nrule a = c \"+\" / \"n\"\nrule c = \"(\" c \")\" \"y\" / \"(\" c \")\" \"z\" / a\n",
	"rule main = x y y z\nrule x = \"x\"\nrule y = \"y\" x\nrule z = (\"z\" #missing) / w\nrule w = z \"w\" / \"w\"\n",
	"rule main = p p r r\nrule p = q \"p\"\nrule q = \"q\" #missing\nrule r = (\";\" #missing) p\n",
	"rule main = a:(\"a\" => 1) b:(\"b\" => 2) => $a + $b\n",
	"rule main = (e \"q\" / \"x\")* $$\nrule e = #operators {\n    operand \"x\"\n    level add { infix left \"+\" }\n}\n",
}

// TestComplexityMemoPolicy checks that the rules the classifier assumes the memo answers are the
// rules that every parse memoizes: a leader, or a rule that is memoized, not transient and not
// subject to missing tokens (Parser.Memoizes under every option). A member of a left-recursion
// cycle is memoized only outside the growths it reads, so the classifier counts it as not
// memoized.
func TestComplexityMemoPolicy(t *testing.T) {
	srcs := map[string]string{}
	for i, src := range complexityMemoSources {
		srcs[string(rune('a'+i))] = src
	}
	for _, pattern := range []string{"../../testdata/grammars/*/*.zp", "../../examples/*/*.zp"} {
		paths, err := filepath.Glob(pattern)
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range paths {
			b, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			srcs[p] = string(b)
		}
	}
	for name, src := range srcs {
		t.Run(name, func(t *testing.T) {
			g, err := syntax.Parse(src)
			if err != nil {
				t.Fatal(err)
			}
			g = starttest.MarkAll(g)
			prog, err := compileStatic(g, Options{}, nil)
			if err != nil {
				t.Fatal(err)
			}
			c, err := ClassifyComplexity(g, "", nil)
			if err != nil {
				t.Fatal(err)
			}
			for _, r := range prog.RuleList {
				always := r.Leader || r.Memo && !r.Transient && !r.Missing && !r.Member
				if c.Memoized[r.Name] != always {
					t.Errorf("rule %s: classifier assumes memoized=%v, compiled rule memoizes in every parse=%v (Leader %v Member %v Memo %v Transient %v Missing %v)",
						r.Name, c.Memoized[r.Name], always, r.Leader, r.Member, r.Memo, r.Transient, r.Missing)
				}
			}
			for n := range c.Memoized {
				if prog.ByName[n] == nil && !strings.HasPrefix(n, "_") {
					t.Errorf("classifier rule %s is not a compiled rule", n)
				}
			}
		})
	}
}

func classifySource(t *testing.T, src string) *Complexity {
	t.Helper()
	g, err := syntax.Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	c, err := ClassifyComplexity(starttest.MarkAll(g), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// TestClassifyComplexity checks the class of grammars that each exercise one part of the
// conditions for linear cost.
func TestClassifyComplexity(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		class     ComplexityClass
		degree    int
	}{
		{"committing alternative", "rule main = (a / \"a\")* $$\nrule a = \"a\" \"c\"\n", ClassLinear, 0},
		{"restarted scan", "rule main = (a / \"a\")* $$\nrule a = \"a\"* \"c\"\n", ClassPoly, 2},
		{"nested restarted scans", "rule main = (a / \"a\")* $$\nrule a = (b / \"a\")* \"c\"\nrule b = \"a\"* \"d\"\n", ClassPoly, 3},
		// The iteration fails only at "\"\"", before its scan, so it need not be committing.
		{"unit failing before its source", "rule main = (s \";\")* $$\nrule s = \"\\\"\" [^\"]* (\"\\\"\\\"\" [^\"]*)* \"\\\"\"\n", ClassLinear, 0},
		{"unit failing after its source", "rule main = (s \";\")* $$\nrule s = \"\\\"\" [^\"]* (\"\\\"\" [^\"]* \"!\")* \"\\\"\"\n", ClassPoly, 2},
		// The operands overlap, but their only source is the shared skip ws.
		{"source inside a shared call", "rule main = e $$\nrule e = #operators {\n    skip ws\n    operand \"true\"\n    operand [a-z]+\n    level { infix left \"+\" }\n}\nrule ws = [ \\t]*\n", ClassLinear, 0},
		// A list built from the elements of a repetition costs its number of elements.
		{"list of a repetition", "rule main = l $$\nrule l = \"[\" ms:m? \"]\" => concat($ms)\nrule m = f:v rs:(\",\" x:v)* => concat(list($f), map($rs, (r) => r.$x))\nrule v = \"x\" / l\n", ClassLinear, 0},
		{"list built by right recursion", "rule main = l $$\nrule l = x:\"a\" t:l? => concat(list($x), $t)\n", ClassPoly, 2},
		// The rule that starts the recursion of r is walked before m, whose captures decide that
		// the value of r includes the value of its own recursion.
		{"list built by right recursion after its user", "rule main = s $$\nrule s []string = \"\\\"\" t:r => concat(list(\"q\"), $t)\nrule r []string = m / l\nrule m []string = e:i \",\" t:r => concat(list($e), $t)\nrule l []string = e:i \"\\\"\" => list($e)\nrule i string = x:(\"(\" s \")\" / [a-z]+) => text($x)\n", ClassPoly, 2},
		{"tail passed on by right recursion after its user", "rule main = s $$\nrule s []string = \"\\\"\" t:r => $t\nrule r []string = m / l\nrule m []string = i \",\" t:r => $t\nrule l []string = e:i \"\\\"\" => list($e)\nrule i string = x:(\"(\" s \")\" / [a-z]+) => text($x)\n", ClassLinear, 0},
		// A lambda over a list with at most a constant number of elements runs at most that many
		// times: its copies from outside it and of a fold accumulator are a constant number of
		// copies. Over a list that is not bounded, or inside a lambda over one, they are not, and
		// neither are those of a fold whose accumulator doubles, by a factor exponential in the
		// bound.
		{"fold over a bounded list", "rule main = a:\"x\"* b:\"y\" #repeat(,2) => foldl(text($a), $b, (acc, y) => acc + text($a))\n", ClassLinear, 0},
		{"doubling fold over a bounded list", "rule main = a:\"x\"* b:\"y\" #repeat(,2) => foldl(text($a), $b, (acc, y) => acc + acc + text($a))\n", ClassExp, 0},
		{"fold over a bounded list that doubles an element", "rule main = a:\"x\"* b:\"y\" #repeat(,2) => foldl(list(text($a)), $b, (acc, y) => concat(list(acc[0] + acc[0]), acc))\n", ClassExp, 0},
		{"fold over a bounded list that builds a doubled element", "rule main = a:\"x\"* b:\"y\" #repeat(,2) => foldl(list(text($a)), $b, (acc, y) => list(acc[0] + acc[0]))\n", ClassExp, 0},
		{"fold over a bounded list that doubles each element", "rule main = a:\"x\"* b:\"y\" #repeat(,2) => foldl(list(text($a)), $b, (acc, y) => map(acc, (e) => e + e))\n", ClassExp, 0},
		{"fold over a bounded list that extends each element", "rule main = a:\"x\"* b:\"y\" #repeat(,2) => foldl(list(text($a)), $b, (acc, y) => map(acc, (e) => e + text(y)))\n", ClassLinear, 0},
		{"bounded elements inside a map", "rule main = zs:\"z\"* b:\"y\" #repeat(,2) => map($zs, (z) => foldl(\"\", $b, (acc, y) => acc + text(y)))\n", ClassLinear, 0},
		{"doubling fold over a list", "rule main = a:\"x\"* b:\"y\"* => foldl(text($a), $b, (acc, y) => acc + acc)\n", ClassExp, 0},
		{"bounded map copying a value from outside", "rule main = a:\"x\"* b:\"y\" #repeat(,2) => map($b, (y) => text($a) + text(y))\n", ClassLinear, 0},
		{"map copying a value from outside", "rule main = a:\"x\"* b:\"y\"* => map($b, (y) => text($a) + text(y))\n", ClassUnknown, 0},
		{"bounded fold inside a map", "rule main = a:\"x\"* zs:(b:\"y\" #repeat(,1) \"z\")* => map($zs, (z) => foldl(\"\", z.$b, (acc, y) => acc + text($a)))\n", ClassUnknown, 0},
		{"bounded fold over the elements of a map", "rule main = a:\"x\"* zs:(b:\"y\" #repeat(,1) \"z\")* => map($zs, (z) => foldl(\"\", z.$b, (acc, y) => acc + text(y)))\n", ClassLinear, 0},
		{"bounded fold that adds a recursive list on each step", "rule main = l $$\nrule l []string = x:\"a\" t:l? b:\"y\" #repeat(,2) => foldl(list(), $b, (acc, y) => concat(acc, $t))\n", ClassExp, 0},
		{"bounded fold of a list built by right recursion", "rule main = l $$\nrule l []string = x:\"a\" t:l? b:\"y\" #repeat(,1) => foldl($t, $b, (acc, y) => concat(acc, list(text(y))))\n", ClassPoly, 2},
		// Comparing two strings examines the shorter one: against a literal it is bounded, between
		// two values of the recursion it is not.
		{"comparison with a literal", "rule main = l $$\nrule l string = (\"(\" x:l \")\" {$x != \"\"} => $x) / a\nrule a string = \"a\" => \"a\"\n", ClassLinear, 0},
		{"comparison of recursive values", "rule main = l $$\nrule l string = (\"(\" x:l y:l \")\" {$x == $y} => $x) / a\nrule a string = \"a\" => \"a\"\n", ClassPoly, 2},
		// The text of a capture is a range of the input and costs a constant, but joining it
		// copies it.
		{"text of a recursive capture", "rule main = a $$\nrule a = s:(\"(\" a \")\" / \"x\") => text($s)\n", ClassLinear, 0},
		{"join of the text of a recursive capture", "rule main = a $$\nrule a = s:(\"(\" a \")\" / \"x\") => text($s) + \"!\"\n", ClassPoly, 2},
		// An element of a list holds anything the list can, so indexing keeps the size.
		{"element of a recursive list", "rule main = r $$\nrule r []string = (\"a\" x:r => list($x[0] + \"a\")) / (\"\" => list(\"\"))\n", ClassPoly, 2},
		{"list of literal elements", "rule main = r $$\nrule r []string = (\"a\" x:r => list(\"a\")) / (\"\" => list(\"\"))\n", ClassLinear, 0},
		{"comparison of recursive elements", "rule main = l $$\nrule l []string = (\"(\" x:l y:l \")\" {$x[0] == $y[0]} => $x) / (t:[a-z]+ => list(text($t)))\n", ClassPoly, 2},
		// A comparison examines no more than one of its operands.
		{"comparison of a recursive element with itself", "rule main = l $$\nrule l []string = (\"(\" x:l \")\" {$x[0] == $x[0]} => $x) / (t:[a-z]+ => list(text($t)))\n", ClassPoly, 2},
		// len examines all of a string or a list.
		{"len of a failing unit", "rule main = (v \"q\" / \"x\")* $$\nrule v = x:e #atomic {len($x.Value) > 0}\nrule e = \"x\" e / \"x\"\n", ClassPoly, 2},
		{"len of a recursive capture", "rule main = r $$\nrule r = x:(\"a\" r / \"\") #atomic {len($x.Value) >= 0}\n", ClassPoly, 2},
		// The length of a node is its range, which len reads in constant time.
		{"len of the node of a failing unit", "rule main = (x:e #atomic {len($x) > 0} \"q\" / \"x\")* $$\nrule e = \"x\" e / \"x\"\n", ClassLinear, 0},
		{"len of the node of a recursive capture", "rule main = r $$\nrule r = x:(\"a\" r / \"\") #atomic {len($x) >= 0}\n", ClassLinear, 0},
		{"len of a list node of a recursive capture", "rule main = r $$\nrule r = x:(\"a\" r)? {len($x) >= 0}\n", ClassLinear, 0},
		{"nil test of a recursive capture", "rule main = r $$\nrule r = x:(\"a\" r / \"\") #atomic {$x != nil}\n", ClassLinear, 0},
		// A repetition ends at its first empty iteration, so it can fail after scanning even
		// when its expression always succeeds.
		{"repetition below its minimum", "rule main = (p / \"a\")* $$\nrule p = (\"a\"*) #repeat(3)\n", ClassPoly, 2},
		// With a minimum of two, "a"* fails only when its first iteration is empty, after a
		// constant examination; the classifier does not tell this from a body that can be empty
		// after scanning, as &"a"* can, and stays pessimistic.
		{"repetition of two below its minimum", "rule main = (p / \"a\")* $$\nrule p = (\"a\"*) #repeat(2)\n", ClassPoly, 2},
		{"repetition below its minimum without a retry", "rule main = (\"a\"*) #repeat(3) $$\n", ClassLinear, 0},
		{"self-growing variable", "var s string = \"a\"\nrule main = r $$\nrule r = \"x\" {s := s + s} r / {len(s) > 0}\n", ClassExp, 0},
		// Only a polynomial class has a degree.
		{"exponential with a polynomial cause", "var s string = \"a\"\nrule main = r (a / \"a\")* $$\nrule r = \"x\" {s := s + s} r / {len(s) > 0}\nrule a = \"a\"* \"c\"\n", ClassExp, 0},
		// A variable whose values lie in a set bounded by a constant adds a constant number of
		// combinations of values, whatever its bindings read; the set of an int comes from its
		// initial value, bindings that read only it, and the guards before them.
		{"counter before a call", "var v int\nrule main = (r / .)* $$\nrule r = \"#\" {v := v + 1} r / \"y\" {v >= 0}\n", ClassExp, 0},
		{"guarded counter before a call", "var v int\nrule main = (r / .)* $$\nrule r = \"#\" {v < 3} {v := v + 1} r / \"y\" {v >= 0}\n", ClassLinear, 0},
		{"counter modulo a constant", "var v int = 1\nrule main = r $$\nrule r = \"#\" {v := (v * 3 + 1) % 7} r / \"y\" {v != 0}\n", ClassLinear, 0},
		{"counter guarded outside a repetition", "var v int\nrule main = r $$\nrule r = \"(\" {v < 3} (\"#\" {v := v + 1})* r \")\" / \"y\" {v >= 0}\n", ClassExp, 0},
		{"guard before another binding", "var v int\nrule main = r $$\nrule r = \"#\" {v < 3} {v := v + 1} {v := v + 1} r / \"y\" {v >= 0}\n", ClassExp, 0},
		{"counter that reads a capture", "var v int\nrule main = r $$\nrule r = x:\"#\" {v < 3} {v := v + len($x)} r / \"y\" {v >= 0}\n", ClassExp, 0},
		{"Boolean toggled before a call", "var b bool\nrule main = r $$\nrule r = x:\"#\" {b := !b && len($x) > 0} r / \"y\" {b}\n", ClassLinear, 0},
		{"counter divided by zero", "var v int\nrule main = r $$\nrule r = \"#\" {v := v + 1 / v} r / \"y\" {v >= 0}\n", ClassLinear, 0},
		{"rebinding after the left call", "var k int\nrule main = e $$\nrule e = e \"x\" {k := k + 1} / \"x\" {k >= 0}\n", ClassLinearEnv, 0},
		{"non-leader cycle members", "rule main = c $$\nrule a = c \"+\" / \"n\"\nrule c = \"(\" c \")\" \"y\" / \"(\" c \")\" \"z\" / a\n", ClassUnknown, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := classifySource(t, tc.src)
			if c.Class != tc.class || c.Degree != tc.degree {
				t.Errorf("class %v degree %d, want %v degree %d\ncauses %v\nreasons %v", c.Class, c.Degree, tc.class, tc.degree, c.Causes, c.Reasons)
			}
			if tc.class == ClassPoly || tc.class == ClassExp {
				if len(c.Causes) == 0 {
					t.Error("no witness")
				}
			}
			if tc.class == ClassUnknown && len(c.Reasons) == 0 {
				t.Error("no reason")
			}
		})
	}
}

// TestClassifyCarriedGrowth checks condition 5: a leader that grows where another leader of its
// cycle grows, from a result that includes the other's growing result, is quadratic when the step
// can drop its result and its growth goes on over input that later steps examine again, and the
// witness names both rules. The linear grammars have leaders that grow inside other growths
// without both.
func TestClassifyCarriedGrowth(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, src string
		class     ComplexityClass
	}{
		// The lookahead drops the growth of R from each seed of S, which goes on over the "q" of
		// later steps.
		{"tree carried in a lookahead", "rule main = S $$\nrule S = &R S \"q\" / \"y\"\nrule R = R \"q\" / S\n", ClassPoly},
		// The growth of R fails at "!" and S grows by its next alternative.
		{"tree carried before a growing alternative", "rule main = S $$\nrule S = R \"!\" / S \"q\" / \"y\"\nrule R = R \"q\" / S\n", ClassPoly},
		// When R "s" fails, S cannot grow, so R grows past the seed of S once.
		{"nested reads", "rule main = S $$\nrule S = S \"q\" / R \"s\" / \"y\"\nrule R = R \"r\" / S \"t\"\n", ClassLinear},
		// R grows from the seed of S only after a "t", where S cannot grow.
		{"lookahead regrowth", "rule main = S $$\nrule S = &R S \"q\" / \"y\"\nrule R = R (\"q\" / \"r\") / S \"t\" / \"y\"\n", ClassLinear},
		{"three leaders", "rule main = S $$\nrule S = &R S \"q\" / \"y\"\nrule R = &T R \"q\" / \"y\"\nrule T = T \"q\" / S \"z\" / \"y\"\n", ClassLinear},
		// R is after S's growing alternative, so a step that drops it does not grow.
		{"tree carried after the growing alternative", "rule main = S $$\nrule S = S \"q\" / R \"!\" / \"y\"\nrule R = R \"q\" / S\n", ClassLinear},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := classifySource(t, tc.src)
			if c.Class != tc.class {
				t.Fatalf("class %v, want %v\ncauses %v\nreasons %v", c.Class, tc.class, c.Causes, c.Reasons)
			}
			if tc.class != ClassPoly {
				return
			}
			if c.Degree != 2 || len(c.Causes) != 1 {
				t.Fatalf("degree %d causes %v, want one cause of degree 2", c.Degree, c.Causes)
			}
			if m := c.Causes[0].Message; !strings.Contains(m, "grows R where S grows") || !strings.Contains(m, "growing result of S") {
				t.Errorf("witness %q does not name R and S", m)
			}
		})
	}
}

// TestClassifyComplexityDeterministic checks that classifying a grammar again gives the same
// result, on a grammar whose units have several callees with sources.
func TestClassifyComplexityDeterministic(t *testing.T) {
	src := "rule main = (a b c d \"q\" / \"x\" / \"y\" / \"z\" / \"w\")* $$\nrule a = \"x\"*\nrule b = \"y\"*\nrule c = \"z\"*\nrule d = \"w\"*\n"
	want := fmt.Sprintf("%+v", *classifySource(t, src))
	for range 50 {
		if got := fmt.Sprintf("%+v", *classifySource(t, src)); got != want {
			t.Fatalf("classification\n%s\nthen\n%s", want, got)
		}
	}
}

// TestComplexityGrowingBindings checks the bindings that the growing-variable check reports.
func TestComplexityGrowingBindings(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		want      int
	}{
		{"doubling before the call", "var s string = \"a\"\nrule main = r $$\nrule r = \"x\" {s := s + s} r / \"\"\n", 1},
		{"bounded extension", "var indent string\nrule main = r $$\nrule r = \"x\" {indent := indent + \"  \"} r / \"\"\n", 0},
		{"after the call", "var s string = \"a\"\nrule main = r $$\nrule r = \"x\" r {s := s + s} / \"\"\n", 0},
		{"different alternative", "var s string = \"a\"\nrule main = r $$\nrule r = {s := s + s} \"q\" / \"x\" r / \"\"\n", 0},
		{"empty variable", "var s string\nrule main = r $$\nrule r = \"x\" {s := s + s} r / \"\"\n", 0},
		{"in a repetition", "var s string = \"a\"\nrule main = (\"x\" {s := s + s})* $$\n", 1},
		{"int variable", "var n int = 1\nrule main = r $$\nrule r = \"x\" {n := n + n} r / \"\"\n", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := classifySource(t, tc.src)
			if len(c.Growing) != tc.want {
				t.Errorf("growing %v, want %d", c.Growing, tc.want)
			}
		})
	}
}

// TestGrowingBindingsAlone checks that GrowingBindings returns the Growing member of the full
// classification, on the grammars of the repository and on bindings in inline actions, whose rule
// is the rule that holds them.
func TestGrowingBindingsAlone(t *testing.T) {
	srcs := map[string]string{
		"recursion":     "var s string = \"a\"\nrule main = r $$\nrule r = \"x\" {s := s + s} r / \"\"\n",
		"inline action": "var s string = \"a\"\nrule main = r $$\nrule r = ((\"x\" {s := s + s})* => 1) \"y\"\n",
		"variables":     "var s string = \"a\"\nvar t string\nrule main = (\"x\" {t := t + t} {s := s + s + \"b\"})* $$\n",
	}
	for _, pattern := range []string{"../../testdata/grammars/*/*.zp", "../../examples/*/*.zp"} {
		paths, err := filepath.Glob(pattern)
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range paths {
			b, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			srcs[p] = string(b)
		}
	}
	for name, src := range srcs {
		t.Run(name, func(t *testing.T) {
			c := classifySource(t, src)
			g, err := syntax.Parse(src)
			if err != nil {
				t.Fatal(err)
			}
			alone, err := GrowingBindings(starttest.MarkAll(g), "", nil)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(alone, c.Growing) {
				t.Errorf("GrowingBindings %v, classification %v", alone, c.Growing)
			}
			if strings.HasPrefix(name, "inline") && (len(alone) != 1 || alone[0].Rule != "r") {
				t.Errorf("inline action: %v, want one binding in r", alone)
			}
			if name == "variables" && len(alone) != 1 {
				t.Errorf("variables: %v, want the binding of s", alone)
			}
		})
	}
}

// TestAbbreviate checks that a shortened source text stays valid UTF-8.
func TestAbbreviate(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"a  b\n c", "a b c"},
		{strings.Repeat("x", 40), strings.Repeat("x", 40)},
		{strings.Repeat("x", 41), strings.Repeat("x", 37) + "..."},
		{strings.Repeat("x", 36) + "ééé", strings.Repeat("x", 36) + "..."},
		{strings.Repeat("x", 35) + "一" + strings.Repeat("y", 10), strings.Repeat("x", 35) + "..."},
		{strings.Repeat("x", 34) + "一" + strings.Repeat("y", 10), strings.Repeat("x", 34) + "一..."},
	} {
		got := abbreviate(tc.in)
		if got != tc.want || !utf8.ValidString(got) {
			t.Errorf("abbreviate(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
