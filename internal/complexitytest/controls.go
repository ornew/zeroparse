package complexitytest

import (
	"strings"

	"zeroparse.dev/zp/internal/engine"
)

// The classes that the complexity classifier assigns to a start rule.
const (
	ClassLinear    = engine.ClassLinear
	ClassLinearEnv = engine.ClassLinearEnv
	ClassPoly      = engine.ClassPoly
	ClassExp       = engine.ClassExp
	ClassUnknown   = engine.ClassUnknown
)

// Control is a grammar family with a known growth order.
type Control struct {
	Name   string
	Family string // E0 to E10, or a name for a case outside the table
	Source string // ZeroParse source with the start rule main
	// Input builds the input of size n.
	Input func(n int) string
	// Sizes are n of the series n, 2n, 4n; ShortSizes are used with testing.Short.
	Sizes, ShortSizes []int
	// Order is the measured growth order of the budget spend.
	Order Order
	// Capped lets the 4n point reach Cap steps. Only Exp entries may be capped.
	Capped bool
	// Class is the class that the complexity classifier assigns to main (see TestControlClasses).
	Class engine.ComplexityClass
	// Positive marks a control that is linear and guards against a classifier or runtime that
	// reports growth where there is none.
	Positive bool
	// BecomesLinear marks a negative control that the memoization of left-recursion cycle members
	// makes linear in measurement, while its class stays one without a claim of linear cost; see
	// Controls.
	BecomesLinear bool
}

// Cap is the budget that stops an exponential control. It keeps the exponential cases short.
const Cap = 20_000_000

// becomesLinearLanded switches the controls marked BecomesLinear to their linear order: the
// runtime memoizes left-recursion cycle members.
const becomesLinearLanded = true

// parens is the input of E2 and E3: n open parentheses, "n", then n closing ones, each followed
// by "z".
func parens(n int) string { return rep("(", n) + "n" + rep(")z", n) }

func double(n int) []int { return []int{n, 2 * n, 4 * n} }

// Controls returns the controls table. The negative controls record growth that the engine has
// today: when a planned change makes one linear, its entry moves to the linear set by hand. The
// negative E2 control is linear in measurement through the becomesLinearLanded switch, with a
// class that claims no linear cost; the negative E8 controls (repeated, nested and alternating
// walks) are linear in measurement through the records of skip runs, with classes that claim no
// linear cost either; E1, E7 and E9 move when repetition runs are
// reused in batch parses; E6 and E10 stay quadratic. The rebinding variants of E6 are positive
// controls: a binding after the left call adds no growth, and the restart in E6 is the negative
// control. Each source is a grammar from the measured families of the
// grammar complexity design record; "main" is the start rule.
func Controls() []Control {
	cs := []Control{
		{
			Name: "E0 repetition after backtracking, safe", Family: "E0", Positive: true,
			Source: "rule main = (a / \"a\")* $$\nrule a = \"a\" \"c\"\n",
			Input:  func(n int) string { return rep("a", n) },
			Sizes:  double(1000), ShortSizes: double(250), Order: Linear, Class: ClassLinear,
		},
		{
			Name: "E1 repetition after backtracking", Family: "E1",
			Source: "rule main = (a / \"a\")* $$\nrule a = \"a\"* \"c\"\n",
			Input:  func(n int) string { return rep("a", n) },
			Sizes:  double(250), ShortSizes: double(100), Order: Poly(2), Class: ClassPoly,
		},
		{
			Name: "E7 nested scans, degree 3", Family: "E7",
			Source: "rule main = (a / \"a\")* $$\nrule a = (b / \"a\")* \"c\"\nrule b = \"a\"* \"d\"\n",
			Input:  func(n int) string { return rep("a", n) },
			Sizes:  double(50), ShortSizes: double(32), Order: Poly(3), Class: ClassPoly,
		},
		{
			Name: "E9 lookahead over scans", Family: "E9",
			Source: "rule main = (\"a\" !rest)* \"a\"* $$\nrule rest = \"a\"* \"c\"\n",
			Input:  func(n int) string { return rep("a", n) },
			Sizes:  double(250), ShortSizes: double(100), Order: Poly(2), Class: ClassPoly,
		},
		{
			Name: "E8 recovery skips", Family: "E8",
			Source: "rule main = (x / \"a\")* $$\nrule x = (y #recover(skip=[a]+)) \"c\"\nrule y = \"b\"\n",
			Input:  func(n int) string { return rep("a", n) },
			Sizes:  double(250), ShortSizes: double(100), Order: Linear, Class: ClassPoly,
		},
		{
			// Each walk starts one position before the one before it and runs to the end.
			Name: "E8 nested recovery skips", Family: "E8",
			Source: "rule main = (s \"!\" / [a(]*) $$\nrule s = (\"(\" s \")\") #recover(skip=[a(]+)\n",
			Input:  func(n int) string { return rep("(", n) + "a" },
			Sizes:  double(250), ShortSizes: double(100), Order: Linear, Class: ClassUnknown,
		},
		{
			// The walks alternate between the two runs of "a": one from each position of the first
			// and one from the start of the second.
			Name: "E8 recovery skips in alternating regions", Family: "E8",
			Source: "rule main = (p / \"a\" / \"b\")* $$\nrule p = x \"b\" x \"z\"\nrule x = \"c\" #recover(skip=[a]+)\n",
			Input:  func(n int) string { return rep("a", n) + "b" + rep("a", n) },
			Sizes:  double(250), ShortSizes: double(100), Order: Linear, Class: ClassPoly,
		},
		{
			Name: "E6 right recursion, safe", Family: "E6", Positive: true,
			Source: "rule main = (e \"q\" / \"x\")* $$\nrule e = \"x\" e / \"x\"\n",
			Input:  func(n int) string { return rep("x", n) },
			Sizes:  double(500), ShortSizes: double(125), Order: Linear, Class: ClassLinear,
		},
		{
			Name: "E6 left recursion restarted", Family: "E6",
			Source: "rule main = (e \"q\" / \"x\")* $$\nrule e = e \"x\" / \"x\"\n",
			Input:  func(n int) string { return rep("x", n) },
			Sizes:  double(250), ShortSizes: double(100), Order: Poly(2), Class: ClassPoly,
		},
		{
			Name: "LR rebinding after the left call, growing value", Family: "E6 variant", Positive: true,
			Source: "var s string = \"a\"\nrule main = e $$\nrule e = e \"x\" {s := s + s} / \"x\" {len(s) > 0}\n",
			Input:  func(n int) string { return rep("x", n) },
			Sizes:  double(250), ShortSizes: double(100), Order: Linear, Class: ClassLinearEnv,
		},
		{
			Name: "LR rebinding after the left call, counter", Family: "E6 variant", Positive: true,
			Source: "var k int\nrule main = e $$\nrule e = e \"x\" {k := k + 1} / \"x\" {k >= 0}\n",
			Input:  func(n int) string { return rep("x", n) },
			Sizes:  double(250), ShortSizes: double(100), Order: Linear, Class: ClassLinearEnv,
		},
		{
			Name: "E10 Operators Rule restarted", Family: "E10",
			Source: "rule main = (e \"q\" / \"x\")* $$\nrule e = #operators {\n    operand \"x\"\n    level add { infix left \"\" }\n}\n",
			Input:  func(n int) string { return rep("x", n) },
			Sizes:  double(250), ShortSizes: double(100), Order: Poly(2), Class: ClassPoly,
		},
		{
			Name: "Operators Rule skip shared with the follow, safe", Family: "shared skip", Positive: true,
			Source: "rule main = s* $$\nrule s = \"r\" e? ws \";\"\nrule e = #operators {\n    skip ws\n    operand \"x\"\n    level add { infix left \"+\" }\n}\nrule ws = \" \"*\n",
			Input:  func(n int) string { return rep("r x + x ;", n) },
			Sizes:  double(250), ShortSizes: double(100), Order: Linear, Class: ClassLinear,
		},
		{
			Name: "Operators Rule skip shared, restarted", Family: "shared skip",
			Source: "rule main = (e \"q\" / ws \"x\")* $$\nrule e = #operators {\n    skip ws\n    operand \"x\"\n    level add { infix left \"\" }\n}\nrule ws = \" \"*\n",
			Input:  func(n int) string { return rep("x", n) },
			Sizes:  double(250), ShortSizes: double(100), Order: Poly(2), Class: ClassPoly,
		},
		{
			Name: "Operators Rule skip that must consume input, optional", Family: "shared skip",
			Source: "rule main = (e \"(\")* $$\nrule e = #operators {\n    skip \" \"+\n    operand \"y\"\n    level { postfix \"(\" [^;]* \";\" }\n}\n",
			Input:  func(n int) string { return rep("y(", n) },
			Sizes:  double(200), ShortSizes: double(100), Order: Poly(2), Class: ClassPoly,
		},
		{
			Name: "Keyword excluded by a negative lookahead, safe", Family: "word exclusion", Positive: true,
			Source: "rule main = (s / \" \")* $$\nrule s = kw (\" \" / \"let\")* \"q\" / id\nrule kw = \"let\" !c\nrule c = [a-z]\nrule id = !key c+\nrule key = (\"if\" / \"let\") !c\n",
			Input:  func(n int) string { return rep("ab let q ", n) },
			Sizes:  double(200), ShortSizes: double(100), Order: Linear, Class: ClassLinear,
		},
		{
			Name: "Keyword lookahead reading a variable", Family: "word exclusion",
			Source: "var v string\nrule main = (s / \" \")* $$\nrule s = kw (\" \" / \"let\")* \"q\" / id\nrule kw = \"let\" !c\nrule c = [a-z]\nrule id = !key c+\nrule key = (\"if\" / \"let\") !c {v != \"\"}\n",
			Input:  func(n int) string { return rep("let ", n) },
			Sizes:  double(200), ShortSizes: double(100), Order: Poly(2), Class: ClassPoly,
		},
		{
			Name: "Keyword lookahead with a nested lookahead", Family: "word exclusion",
			Source: "rule main = (s / \" \")* $$\nrule s = kw (\" \" / \"let\")* \"q\" / id\nrule kw = \"let\" !c\nrule c = [a-z]\nrule id = !key c+\nrule key = (\"if\" / \"let\" !\" \") !c\n",
			Input:  func(n int) string { return rep("let ", n) },
			Sizes:  double(200), ShortSizes: double(100), Order: Poly(2), Class: ClassPoly,
		},
		{
			Name: "Keyword lookahead under a level restriction", Family: "word exclusion",
			Source: "rule main = (s / \" \")* $$\nrule s = kw (\" \" / \"let\")* \"q\" / id\nrule kw = \"let\" !c\nrule c = [a-z]\nrule id = !key c+\nrule key = level(ops, tight) !c\nrule ops = #operators {\n    operand \"le\"\n    level loose { postfix \"t\" }\n    level tight { postfix \"!\" }\n}\n",
			Input:  func(n int) string { return rep("let ", n) },
			Sizes:  double(200), ShortSizes: double(100), Order: Poly(2), Class: ClassPoly,
		},
		{
			Name: "Keyword lookahead with a shorter earlier keyword", Family: "word exclusion",
			Source: "rule main = (s / \" \")* $$\nrule s = kw (\" \" / \"let\")* \"q\" / id\nrule kw = \"let\" !c\nrule c = [a-z]\nrule id = !key c+\nrule key = (\"le\" / \"let\") !c\n",
			Input:  func(n int) string { return rep("let ", n) },
			Sizes:  double(200), ShortSizes: double(100), Order: Poly(2), Class: ClassPoly,
		},
		{
			Name: "Keyword lookahead with a wider character set", Family: "word exclusion",
			Source: "rule main = (s / \" \")* $$\nrule s = kw [a-z ]* \"q\" / id\nrule kw = \"let\" ![a-m]\nrule c = [a-z]\nrule id = !key c+\nrule key = (\"if\" / \"let\") !c\n",
			Input:  func(n int) string { return rep("letz ", n) },
			Sizes:  double(200), ShortSizes: double(100), Order: Poly(2), Class: ClassPoly,
		},
		{
			Name: "Operator part excluded by the text it consumes, safe", Family: "word exclusion", Positive: true,
			Source: "rule main = (e (\"=\" !\"=\" e)? \";\")* $$\nrule e = #operators {\n    operand \"x\"\n    operand \"(\" e \")\"\n    level { infix none \"==\" }\n}\n",
			Input:  func(n int) string { return rep("x==(", n) + "x" + rep(")", n) + "=x;" },
			Sizes:  double(64), ShortSizes: double(32), Order: Linear, Class: ClassLinear,
		},
		{
			Name: "Literal excluded by the text it shares, safe", Family: "word exclusion", Positive: true,
			Source: "rule main = (s / \" \" / \"x\")* $$\nrule s = \"==\" (\" \" / \"=\")* \"q\" / \"=\" !\"=\" \"x\"\n",
			Input:  func(n int) string { return rep("== q =x ", n) },
			Sizes:  double(200), ShortSizes: double(100), Order: Linear, Class: ClassLinear,
		},
		{
			Name: "Literal lookahead that the text does not match", Family: "word exclusion",
			Source: "rule main = (s / \" \")* $$\nrule s = \"==\" (\" \" / \"=\")* \"q\" / \"=\" !\"y\" \"=\"\n",
			Input:  func(n int) string { return rep("== ", n) },
			Sizes:  double(200), ShortSizes: double(100), Order: Poly(2), Class: ClassPoly,
		},
		{
			Name: "Literal lookahead longer than the text", Family: "word exclusion",
			Source: "rule main = (s / \" \")* $$\nrule s = \"==\" (\" \" / \"=\")* \"q\" / \"=\" !\"==\" \"=\"\n",
			Input:  func(n int) string { return rep("== ", n) },
			Sizes:  double(200), ShortSizes: double(100), Order: Poly(2), Class: ClassPoly,
		},
		{
			Name: "Literal longer than the text", Family: "word exclusion",
			Source: "rule main = (s / \" \")* $$\nrule s = \"let\" [a-z ]* \"q\" / \"lett\" [a-z]*\n",
			Input:  func(n int) string { return rep("lett ", n) },
			Sizes:  double(200), ShortSizes: double(100), Order: Poly(2), Class: ClassPoly,
		},
		{
			Name: "E2 cycle members behind a leader, safe", Family: "E2", Positive: true,
			Source: "rule main = c $$\nrule a = \"n\"\nrule c = \"(\" c \")\" \"y\" / \"(\" c \")\" \"z\" / a\n",
			Input:  parens, Sizes: double(64), ShortSizes: double(32), Order: Linear, Class: ClassLinear,
		},
		{
			Name: "E2 non-leader cycle members", Family: "E2", BecomesLinear: true,
			Source: "rule main = c $$\nrule a = c \"+\" / \"n\"\nrule c = \"(\" c \")\" \"y\" / \"(\" c \")\" \"z\" / a\n",
			Input:  parens, Sizes: double(4), ShortSizes: double(3), Order: Exp, Capped: true, Class: ClassUnknown,
		},
		{
			Name: "E3 environments, safe", Family: "E3", Positive: true,
			Source: "var v string\nrule main = r $$\nrule r = \"(\" {v := \"0\"} r \")\" \"y\" / \"(\" {v := \"1\"} r \")\" \"z\" / \"n\" {len(v) >= 0}\n",
			// len examines all of v, whose size the classifier bounds only by its value.
			Input: parens, Sizes: double(64), ShortSizes: double(32), Order: Linear, Class: ClassLinearEnv,
		},
		{
			Name: "E3 diverging environments", Family: "E3",
			Source: "var v string\nrule main = r $$\nrule r = \"(\" {v := v + \"0\"} r \")\" \"y\" / \"(\" {v := v + \"1\"} r \")\" \"z\" / \"n\" {len(v) >= 0}\n",
			Input:  parens, Sizes: double(3), ShortSizes: double(2), Order: Exp, Capped: true, Class: ClassExp,
		},
		{
			Name: "E4 self-growing variable", Family: "E4",
			Source: "var s string = \"a\"\nrule main = r $$\nrule r = \"x\" {s := s + s} r / {len(s) > 0}\n",
			Input:  func(n int) string { return rep("x", n) },
			Sizes:  double(4), ShortSizes: double(3), Order: Exp, Capped: true, Class: ClassExp,
		},
		{
			Name: "E5 self-growing fold", Family: "E5",
			Source: "rule main = xs:\"x\"* => len(foldl(\"a\", $xs, (acc, x) => acc + acc))\n",
			Input:  func(n int) string { return rep("x", n) },
			Sizes:  double(4), ShortSizes: double(3), Order: Exp, Capped: true, Class: ClassExp,
		},
		// The memo evaluates a rule at most twice at a position, but it does not share the scan of
		// a memoized rule between positions. In both controls every rule that scans is memoized, and
		// a continuation that consumes one character evaluates the scan again one position later:
		// through a repetition, and through a memoized recursion in which the unit that is not
		// committing is nested in itself.
		{
			Name: "Memoized scan restarted by a repetition", Family: "memo bound",
			Source: "rule main = (a \"y\" / \"q\" a / .)* $$\nrule a = x+\nrule x = \"x\"\n",
			Input:  func(n int) string { return rep("x", n) },
			Sizes:  double(250), ShortSizes: double(100), Order: Poly(2), Class: ClassPoly,
		},
		{
			Name: "Memoized scan restarted through a memoized recursion", Family: "memo bound",
			Source: "rule main = t a? $$\nrule t = a t \"!\" / . t / \"\"\nrule a = x+\nrule x = \"x\"\n",
			Input:  func(n int) string { return rep("x", n) },
			Sizes:  double(250), ShortSizes: double(100), Order: Poly(2), Class: ClassUnknown,
		},
		// The rule that starts the right recursion comes before the rule whose action copies the
		// tail, so the classifier needs the captures of a rule that it walks later.
		{
			Name: "Tail copied along a right recursion", Family: "memo bound",
			Source: "rule main = s $$\nrule s []string = \"\\\"\" t:r => concat(list(\"q\"), $t)\nrule r []string = m / l\nrule m []string = e:i \",\" t:r => concat(list($e), $t)\nrule l []string = e:i \"\\\"\" => list($e)\nrule i string = x:(\"(\" s \")\" / [a-z]+) => text($x)\n",
			Input: func(n int) string {
				items := make([]string, n)
				for i := range items {
					items[i] = "a"
				}
				return "\"" + strings.Join(items, ",") + "\""
			},
			Sizes: double(250), ShortSizes: double(100), Order: Poly(2), Class: ClassPoly,
		},
		// A lambda over a list with at most a constant number of elements runs at most that many
		// times, so its copies from outside it, and the copies of a fold accumulator, are linear;
		// inside a lambda over a list that is not bounded, the same fold runs once for each of its
		// elements.
		{
			Name: "Fold over a bounded list", Family: "bounded lambda",
			Source: "rule main = a:\"x\"* b:\"y\" #repeat(,2) => foldl(text($a), $b, (acc, y) => acc + text($a))\n",
			Input:  func(n int) string { return rep("x", n) + "yy" },
			Sizes:  double(250), ShortSizes: double(100), Order: Linear, Class: ClassLinear, Positive: true,
		},
		{
			Name: "Bounded map copying a value from outside", Family: "bounded lambda",
			Source: "rule main = a:\"x\"* b:\"y\" #repeat(,2) => map($b, (y) => text($a) + text(y))\n",
			Input:  func(n int) string { return rep("x", n) + "yy" },
			Sizes:  double(250), ShortSizes: double(100), Order: Linear, Class: ClassLinear, Positive: true,
		},
		{
			Name: "Bounded elements inside a map that is not bounded", Family: "bounded lambda",
			Source: "rule main = zs:\"z\"* b:\"y\" #repeat(,2) => map($zs, (z) => foldl(\"\", $b, (acc, y) => acc + text(y)))\n",
			Input:  func(n int) string { return rep("z", n) + "yy" },
			Sizes:  double(250), ShortSizes: double(100), Order: Linear, Class: ClassLinear, Positive: true,
		},
		{
			Name: "Bounded fold inside a map that is not bounded", Family: "bounded lambda",
			Source: "rule main = a:\"x\"* zs:(b:\"y\" #repeat(,1) \"z\")* => map($zs, (z) => foldl(\"\", z.$b, (acc, y) => acc + text($a)))\n",
			Input:  func(n int) string { return rep("x", n) + rep("yz", n) },
			Sizes:  double(250), ShortSizes: double(100), Order: Poly(2), Class: ClassUnknown,
		},
		// A fold whose accumulator doubles inside an element of the list it builds grows by a
		// factor exponential in the number of steps, which the bound limits but the input reaches.
		// Duplicating the elements of the accumulator, or padding each and joining them, doubles
		// it on each step; joining each element once keeps its size, but the inner fold copies its
		// own accumulator once for each element, which the classifier bounds by an accumulator of
		// n elements, cubic over n steps, though here it holds one and measures linear.
		{
			Name: "Fold duplicating and joining elements", Family: "bounded lambda",
			Source: "rule main = a:\"x\"* b:\"y\"* => foldl(list(text($a)), $b, (acc, y) => foldl(list(), map(acc, (e) => list(e, e)), (s, l) => concat(s, l)))\n",
			Input:  func(n int) string { return "x" + rep("y", n) },
			Sizes:  double(3), ShortSizes: double(3), Order: Exp, Capped: true, Class: ClassExp,
		},
		{
			Name: "Fold padding and joining elements", Family: "bounded lambda",
			Source: "rule main = a:\"x\"* b:\"y\"* => foldl(list(text($a)), $b, (acc, y) => foldl(list(), map(acc, (e) => list(e, \"x\")), (s, l) => concat(s, l)))\n",
			Input:  func(n int) string { return "x" + rep("y", n) },
			Sizes:  double(3), ShortSizes: double(3), Order: Exp, Capped: true, Class: ClassExp,
		},
		{
			Name: "Fold joining elements once", Family: "bounded lambda",
			Source: "rule main = a:\"x\"* b:\"y\"* => foldl(list(text($a)), $b, (acc, y) => foldl(list(), map(acc, (e) => list(e)), (s, l) => concat(s, l)))\n",
			Input:  func(n int) string { return "x" + rep("y", n) },
			Sizes:  double(250), ShortSizes: double(100), Order: Linear, Class: ClassPoly,
		},
		// A value read from outside the fold adds up to n on each of n steps, so the accumulator
		// grows quadratically and copying it costs n^3.
		{
			Name: "Fold adding an outside value", Family: "bounded lambda",
			Source: "rule main = a:\"x\"* b:\"y\"* c:\"z\"* => foldl(text($a), $b, (acc, y) => acc + text($c))\n",
			Input:  func(n int) string { return "x" + rep("y", n) + rep("z", n) },
			Sizes:  double(40), ShortSizes: double(25), Order: Poly(3), Class: ClassPoly,
		},
		// Measuring the first element, of n characters, for each of n elements examines n^2
		// characters on each of n steps.
		{
			Name: "Fold measuring its first element for every element", Family: "bounded lambda",
			Source: "rule main = a:\"x\"* b:\"y\"* => foldl(concat(list(text($a)), map($a, (z) => text(z))), $b, (acc, y) => filter(acc, (e) => len(acc[0]) > 0))\n",
			Input:  func(n int) string { return rep("x", n) + rep("y", n) },
			Sizes:  double(40), ShortSizes: double(25), Order: Poly(3), Class: ClassPoly,
		},
		// A fold that copies its accumulator inside a map over a list as large as its own runs once
		// for each element of the map.
		{
			Name: "Fold inside a map over the same list", Family: "bounded lambda",
			Source: "rule main = b:\"y\"* => map($b, (y) => foldl(\"\", $b, (s, l) => s + text(l)))\n",
			Input:  func(n int) string { return rep("y", n) },
			Sizes:  double(40), ShortSizes: double(25), Order: Poly(3), Class: ClassPoly,
		},
		// Elements that capture the rest of the input in a lookahead overlap, so a fold over the
		// capture of each element runs over up to n elements for each of n elements.
		{
			Name: "Fold over a capture in a lookahead of each element", Family: "bounded lambda",
			Source: "rule main = items:item* => map($items, (it) => foldl(\"\", it.$c, (s, l) => s + text(l)))\nrule item = &(c:\"x\"*) \"x\"\n",
			Input:  func(n int) string { return rep("x", n) },
			Sizes:  double(90), ShortSizes: double(80), Order: Poly(3), Class: ClassPoly,
		},
		// A map that extends every element of an accumulator of n elements on each of n steps
		// costs n per step for each element.
		{
			Name: "Fold extending every element", Family: "bounded lambda",
			Source: "rule main = a:\"x\"* b:\"y\"* => foldl(map($a, (z) => text(z)), $b, (acc, y) => map(acc, (e) => e + \"x\"))\n",
			Input:  func(n int) string { return rep("x", n) + rep("y", n) },
			Sizes:  double(40), ShortSizes: double(25), Order: Poly(3), Class: ClassPoly,
		},
		{
			Name: "Bounded fold doubling an element it keeps", Family: "bounded lambda",
			Source: "rule main = a:\"x\"* b:\"y\" #repeat(,14) => foldl(list(text($a)), $b, (acc, y) => concat(list(acc[0] + acc[0]), acc))\n",
			Input:  func(n int) string { return rep("x", 64) + rep("y", n) },
			Sizes:  double(3), ShortSizes: double(3), Order: Exp, Class: ClassExp,
		},
		{
			Name: "Bounded fold building a doubled element", Family: "bounded lambda",
			Source: "rule main = a:\"x\"* b:\"y\" #repeat(,14) => foldl(list(text($a)), $b, (acc, y) => list(acc[0] + acc[0]))\n",
			Input:  func(n int) string { return rep("x", 64) + rep("y", n) },
			Sizes:  double(3), ShortSizes: double(3), Order: Exp, Class: ClassExp,
		},
		{
			Name: "Bounded fold doubling each element", Family: "bounded lambda",
			Source: "rule main = a:\"x\"* b:\"y\" #repeat(,14) => foldl(list(text($a)), $b, (acc, y) => map(acc, (e) => e + e))\n",
			Input:  func(n int) string { return rep("x", 64) + rep("y", n) },
			Sizes:  double(3), ShortSizes: double(3), Order: Exp, Class: ClassExp,
		},
		// A binding that reads its own earlier value before a recursive call that follows consumed
		// input still reaches each position with a different value from every start, so the
		// number of combinations of values at a position grows with the input. The memo finds the
		// entry of a combination through a hash index, so the cost is the number of (position,
		// value) pairs, quadratic; walking the entries of a position would make it cubic. The
		// sizes start at 100 because a chain is walked until it passes 32 entries.
		{
			Name: "Counter bound before a call after consumed input", Family: "bounded binding",
			Source: "var v int\nrule main = (r / .)* $$\nrule r = \"#\" {v := v + 1} r / \"y\" {v >= 0}\n",
			Input:  func(n int) string { return rep("#", n) },
			Sizes:  double(100), ShortSizes: double(100), Order: Poly(2), Class: ClassExp,
		},
		// A guard that reads only the variable keeps its values in a set bounded by a constant.
		{
			Name: "Counter guarded before a call after consumed input", Family: "bounded binding",
			Source: "var v int\nrule main = (r / .)* $$\nrule r = \"#\" {v < 3} {v := v + 1} r / \"y\" {v >= 0}\n",
			Input:  func(n int) string { return rep("#", n) },
			Sizes:  double(250), ShortSizes: double(100), Order: Linear, Class: ClassLinear, Positive: true,
		},
		// The values that reach a binding are those after the earlier items of its sequence. Here
		// they are the constants of the choice before it and the flags that the recursion clears
		// and sets; a closure over every value found, regardless of order, would add 2 without end.
		{
			Name: "Context flags set after a choice of values", Family: "bounded binding",
			Source: "var ctx int\nrule main = (\"0\" {ctx := 64} / {ctx := 0}) (\"1\" {ctx := ctx + 2})? (r / .)* $$\nrule r = g:\"*\"? \"#\" {ctx := ctx - ctx % 4 + len($g) * 2} r / \"(\" {ctx := ctx - ctx / 4 % 2 * 4 + 4} r / \"y\" {ctx / 4 % 2 == 1}\n",
			Input:  func(n int) string { return "01" + rep("*#(", n) },
			Sizes:  double(250), ShortSizes: double(100), Order: Linear, Class: ClassLinear, Positive: true,
		},
		// A guard after the call does not filter the values that the binding before the call reads,
		// which grow with the depth of the recursion.
		{
			Name: "Counter guarded after a call after consumed input", Family: "bounded binding",
			Source: "var v int\nrule main = (r / .)* $$\nrule r = \"#\" {v := v + 1} r {v < 3} / \"y\" {v >= 0}\n",
			Input:  func(n int) string { return rep("#", n) },
			Sizes:  double(100), ShortSizes: double(100), Order: Poly(2), Class: ClassExp,
		},
		// A plain group of a sequence splices its elements into the tuple around it: len($x) is 3
		// here, so the binding adds 1 on every call, while dividing by 4 adds nothing.
		{
			Name: "Counter adding the length of a spliced tuple", Family: "bounded binding",
			Source: "var v int\nrule main = (r / .)* $$\nrule r = x:(\"#\" (\"a\" \"b\")) {v := v + len($x) / 3} r / \"y\" {v >= 0}\n",
			Input:  func(n int) string { return rep("#ab", n) },
			Sizes:  double(100), ShortSizes: double(100), Order: Poly(2), Class: ClassExp,
		},
		{
			Name: "Counter adding a spliced tuple length below its step", Family: "bounded binding",
			Source: "var v int\nrule main = (r / .)* $$\nrule r = x:(\"#\" (\"a\" \"b\")) {v := v + len($x) / 4} r / \"y\" {v >= 0}\n",
			Input:  func(n int) string { return rep("#ab", n) },
			Sizes:  double(250), ShortSizes: double(100), Order: Linear, Class: ClassLinear, Positive: true,
		},
		// The product of the numbers of values of the variables with finite values is at most 128:
		// seven flag bits, or seven bound Booleans beside one never bound, are exempt; a counter
		// with 130 values, or two with 12 each, are not, though they measure linear.
		{
			Name: "Seven flag bits", Family: "bounded binding",
			Source: "var ctx int\nrule main = (r / .)* $$\nrule r = \"a\" {ctx := ctx - ctx / 1 % 2 * 1 + 1} r / \"b\" {ctx := ctx - ctx / 2 % 2 * 2 + 2} r / \"c\" {ctx := ctx - ctx / 4 % 2 * 4 + 4} r / \"d\" {ctx := ctx - ctx / 8 % 2 * 8 + 8} r / \"e\" {ctx := ctx - ctx / 16 % 2 * 16 + 16} r / \"f\" {ctx := ctx - ctx / 32 % 2 * 32 + 32} r / \"g\" {ctx := ctx - ctx / 64 % 2 * 64 + 64} r / \"y\" {ctx >= 0}\n",
			Input:  func(n int) string { return rep("abcdefg", n) },
			Sizes:  double(100), ShortSizes: double(50), Order: Linear, Class: ClassLinear, Positive: true,
		},
		{
			Name: "Seven bound Booleans and an unbound one", Family: "bounded binding",
			Source: "var b1 bool\nvar b2 bool\nvar b3 bool\nvar b4 bool\nvar b5 bool\nvar b6 bool\nvar b7 bool\nvar b8 bool\nrule main = (r / .)* $$\nrule r = \"a\" {b1 := true} r / \"b\" {b2 := true} r / \"c\" {b3 := true} r / \"d\" {b4 := true} r / \"e\" {b5 := true} r / \"f\" {b6 := true} r / \"g\" {b7 := true} r / \"y\" {b1 || b2 || b3 || b4 || b5 || b6 || b7 || b8}\n",
			Input:  func(n int) string { return rep("abcdefg", n) },
			Sizes:  double(100), ShortSizes: double(50), Order: Linear, Class: ClassLinear, Positive: true,
		},
		{
			Name: "Counter with 130 values", Family: "bounded binding",
			Source: "var v int\nrule main = (r / .)* $$\nrule r = \"#\" {v < 129} {v := v + 1} r / \"y\" {v >= 0}\n",
			Input:  func(n int) string { return rep("##y", n) },
			Sizes:  double(250), ShortSizes: double(100), Order: Linear, Class: ClassExp,
		},
		{
			Name: "Two counters with 144 combinations", Family: "bounded binding",
			Source: "var u int\nvar v int\nrule main = (r / .)* $$\nrule r = \"#\" {u < 11} {u := u + 1} r / \"(\" {v < 11} {v := v + 1} r / \"y\" {u + v >= 0}\n",
			Input:  func(n int) string { return rep("#(", n) },
			Sizes:  double(250), ShortSizes: double(100), Order: Linear, Class: ClassExp,
		},
	}
	cs = append(cs, rewriteControls()...)
	for i := range cs {
		if cs[i].BecomesLinear && becomesLinearLanded {
			// The growth becomes linear, but the classifier counts cycle members as not memoized,
			// since they are memoized only outside the growths they read: its class stays the one
			// it reports, which claims no linear cost, so the control is not positive.
			cs[i].Order, cs[i].Capped = Linear, false
			// A series of a linear control needs sizes where fixed cost no longer dominates: the
			// slopes of the E2 positive control are 0.98 and 0.99 from 64 and 0.97 and 0.98 from 32.
			cs[i].Sizes, cs[i].ShortSizes = double(64), double(32)
		}
	}
	return cs
}
