package gentest

// CompareCases returns the grammars and inputs of the comparison tests, which every backend runs.
// == and != compare two ints, two strings or two bools, and any value with nil; every other
// comparison is an error. The operands that the cases compare are cast to _, so that their types
// are not known statically and the comparison is checked when it runs.
func CompareCases() []Case {
	const rules = "type S = { A int }\nrule c = [a-z]\nrule w = [a-z]+ #atomic\nrule k int = [0-9] => 3\n"
	return []Case{
		{Name: "scalars", Src: "rule main = x:k y:k s:w \" \" t:w $$ => (($x.(_) == $y.(_)) == (text($s) == text($t))) && $x.(_) != 4 && true == true\n" + rules, Inputs: []string{"33a a", "33a b", "3"}},
		{Name: "nil", Src: "rule main = x:c? y:c? $$ => ($x.(_) == nil) && ($y.(_) == nil) && nil == nil && !(nil != $x.(_))\n" + rules, Inputs: []string{"", "a", "ab"}},
		{Name: "node and nil", Src: "rule main = x:c y:c? $$ => ($x.(_) == nil) || ($x.(_) != nil && $y.(_) == nil)\n" + rules, Inputs: []string{"a", "ab"}},
		{Name: "nodes", Src: "rule main = x:c y:c $$ => $x.(_) == $y.(_)\n" + rules, Inputs: []string{"ab"}},
		{Name: "same node", Src: "rule main = x:c $$ => $x.(_) == $x.(_)\n" + rules, Inputs: []string{"a"}},
		{Name: "node and string", Src: "rule main = x:c $$ => $x.(_) != \"a\"\n" + rules, Inputs: []string{"a"}},
		{Name: "string and node", Src: "rule main = x:c $$ => \"a\" == $x.(_)\n" + rules, Inputs: []string{"a"}},
		{Name: "node and int", Src: "rule main = x:c $$ => $x.(_) == 1\n" + rules, Inputs: []string{"a"}},
		{Name: "int and string", Src: "rule main = x:c $$ => (1).(_) == \"a\"\n" + rules, Inputs: []string{"a"}},
		{Name: "int and bool", Src: "rule main = x:c $$ => (1).(_) != true\n" + rules, Inputs: []string{"a"}},
		{Name: "list node", Src: "rule main = x:c* y:c* $$ => $x.(_) == $y.(_)\n" + rules, Inputs: []string{"", "ab"}},
		{Name: "struct", Src: "rule main = x:c $$ => S{A: 1}.(_) == S{A: 1}.(_)\n" + rules, Inputs: []string{"a"}},
		{Name: "struct and nil", Src: "rule main = x:c $$ => S{A: 1}.(_) != nil\n" + rules, Inputs: []string{"a"}},
		{Name: "tuple", Src: "rule main = x:c $$ => (1, 2).(_) == (1, 2).(_)\n" + rules, Inputs: []string{"a"}},
		{Name: "list", Src: "rule main = x:c $$ => []int{1}.(_) == []int{1}.(_)\n" + rules, Inputs: []string{"a"}},
		{Name: "predicate fails", Src: "rule main = x:c y:c ({$x.(_) == $y.(_)} \"!\" / \"?\") $$\n" + rules, Inputs: []string{"ab!", "ab?"}},
		{Name: "predicate with nil", Src: "rule main = x:c y:c? ({$y.(_) == nil} \"!\" / \"?\") $$\n" + rules, Inputs: []string{"a!", "ab?", "ab!"}},
		{Name: "nested lookahead, left recursion", Src: "rule main = &(&(a:e) b:e ({$a.(_) == $b.(_)} / \"\")) e\nrule e = e \"+\" n / n\nrule n = [0-9] #atomic\n", Inputs: []string{"1+2"}},
		{Name: "nested lookahead, repetition", Src: "rule main = &(&(a:e) b:e ({$a.(_) == $b.(_)} / \"\")) e\nrule e = n (\"+\" n)*\nrule n = [0-9] #atomic\n", Inputs: []string{"1+2"}},
	}
}
