package complexitytest

// A Rewrite pairs a control whose cost grows faster than its input with a control that rewrites
// the grammar to a lower order, as the cookbook recipe on linear rewrites shows. The recipe
// quotes both grammars, and TestRewriteRecipe checks that it quotes these.
type Rewrite struct {
	Pattern  string // the pattern of the recipe
	Original string // Name of the control that grows
	Rewrite  string // Name of the control that replaces it
	// Language lists inputs on which the two grammars must agree about accepting, when they
	// are meant to accept the same language; empty when the rewrite changes the language.
	Language []string
}

// Rewrites lists the pairs of the cookbook recipe.
func Rewrites() []Rewrite {
	return []Rewrite{
		{"restarted repetition", "E1 repetition after backtracking", "Rewrite: scan once, not per alternative",
			[]string{"", "a", "aaa", "c", "ac", "aaac", "acaa", "caca", "aab", "b"}},
		{"lookahead over a scan", "E9 lookahead over scans", "Rewrite: check once after the scan",
			[]string{"", "a", "aaa", "c", "ac", "aaac", "aca", "aaacaa"}},
		{"recovery skip in a called rule", "Recovery skip walked in a called rule", "E8 recovery skips", nil},
		{"left recursion restarted", "E6 left recursion restarted", "Rewrite: call the left-recursive rule once",
			[]string{"", "x", "xxx", "q", "xq", "xxq", "xqxq", "qx", "xxqxx"}},
		{"Operators Rule restarted", "E10 Operators Rule restarted", "Rewrite: call the Operators Rule once",
			[]string{"", "x", "xxx", "q", "xq", "xxq", "xqxq", "qx", "xxqxx"}},
		{"diverging bindings", "E3 diverging environments", "E3 environments, safe", nil},
		{"self-growing variable", "E4 self-growing variable", "Rewrite: carry the count in the value", nil},
		{"self-growing fold", "E5 self-growing fold", "Rewrite: fold a counter", nil},
	}
}

func rewriteControls() []Control {
	return []Control{
		{
			Name: "Rewrite: scan once, not per alternative", Family: "rewrite", Positive: true,
			Source: "rule main = item* $$\nrule item = \"a\"+ \"c\"? / \"c\"\n",
			Input:  func(n int) string { return rep("a", n) },
			Sizes:  double(1000), ShortSizes: double(250), Order: Linear, Class: ClassLinear,
		},
		{
			Name: "Rewrite: check once after the scan", Family: "rewrite", Positive: true,
			Source: "rule main = \"a\"* !\"c\" $$\n",
			Input:  func(n int) string { return rep("a", n) },
			Sizes:  double(1000), ShortSizes: double(250), Order: Linear, Class: ClassLinear,
		},
		{
			Name: "Recovery skip walked in a called rule", Family: "E8 variant",
			Source: "rule main = (x / \"a\")* $$\nrule x = (y #recover(skip=s)) \"c\"\nrule y = \"b\"\nrule s = [a]+\n",
			Input:  func(n int) string { return rep("a", n) },
			Sizes:  double(250), ShortSizes: double(100), Order: Poly(2), Class: ClassPoly,
		},
		{
			Name: "Rewrite: call the left-recursive rule once", Family: "rewrite", Positive: true,
			Source: "rule main = (e \"q\"?)* $$\nrule e = e \"x\" / \"x\"\n",
			Input:  func(n int) string { return rep("x", n) },
			Sizes:  double(1000), ShortSizes: double(250), Order: Linear, Class: ClassLinear,
		},
		{
			Name: "Rewrite: call the Operators Rule once", Family: "rewrite", Positive: true,
			Source: "rule main = (e \"q\"?)* $$\nrule e = #operators {\n    operand \"x\"\n    level add { infix left \"\" }\n}\n",
			Input:  func(n int) string { return rep("x", n) },
			Sizes:  double(1000), ShortSizes: double(250), Order: Linear, Class: ClassLinear,
		},
		{
			Name: "Rewrite: carry the count in the value", Family: "rewrite", Positive: true,
			Source: "rule main = r $$\nrule r int = (\"x\" n:r => $n + 1) / (\"\" => 0)\n",
			Input:  func(n int) string { return rep("x", n) },
			Sizes:  double(1000), ShortSizes: double(250), Order: Linear, Class: ClassLinear,
		},
		{
			Name: "Rewrite: fold a counter", Family: "rewrite", Positive: true,
			Source: "rule main = xs:\"x\"* => foldl(0, $xs, (acc, x) => acc + 1)\n",
			Input:  func(n int) string { return rep("x", n) },
			Sizes:  double(1000), ShortSizes: double(250), Order: Linear, Class: ClassLinear,
		},
	}
}
