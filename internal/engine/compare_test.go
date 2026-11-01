package engine

import (
	"strings"
	"testing"

	"zeroparse.dev/zp/internal/compile"
	"zeroparse.dev/zp/internal/gentest"
)

func TestCompareAcrossBackends(t *testing.T) {
	t.Parallel()
	for _, fixture := range gentest.CompareCases() {
		t.Run(fixture.Name, func(t *testing.T) {
			program := mustCompile(t, fixture.Src)
			for _, unit := range []PositionUnit{CodePoints, Bytes} {
				for _, input := range fixture.Inputs {
					baseline := gentest.ResultJSON(program.ParseWith("main", input, ParseOptions{Backend: Closure, Unit: unit}))
					for _, backend := range []Backend{Bytecode, BytecodeIterative} {
						actual := gentest.ResultJSON(program.ParseWith("main", input, ParseOptions{Backend: backend, Unit: unit}))
						if actual != baseline {
							t.Fatalf("%s/%s input=%q: %s / %s", backend, unit, input, actual, baseline)
						}
					}
				}
			}
		})
	}
}

// TestCompareValues checks the result of == and != for the values that the cases compare, on every
// backend.
func TestCompareValues(t *testing.T) {
	t.Parallel()
	const hint = "nodes and other structured values cannot be compared; use text(x), or .Start and .End"
	byName := map[string]string{}
	for _, fixture := range gentest.CompareCases() {
		byName[fixture.Name] = fixture.Src
	}
	for _, tc := range []struct{ name, input, want string }{
		{"scalars", "33a a", `"node":true`},
		{"scalars", "33a b", `"node":false`},
		{"nil", "", `"node":true`},
		{"nil", "a", `"node":false`},
		{"node and nil", "a", `"node":true`},
		{"node and nil", "ab", `"node":false`},
		{"nodes", "ab", "invalid operands node[string] and node[string] for ==: " + hint},
		{"same node", "a", "invalid operands node[string] and node[string] for ==: " + hint},
		{"node and string", "a", "invalid operands node[string] and string for !=: " + hint},
		{"string and node", "a", "invalid operands string and node[string] for ==: " + hint},
		{"node and int", "a", "invalid operands node[string] and int for ==: " + hint},
		{"int and string", "a", "invalid operands int and string for ==\""},
		{"int and bool", "a", "invalid operands int and bool for !=\""},
		{"list node", "ab", "for ==: " + hint},
		{"struct", "a", "invalid operands S and S for ==: " + hint},
		{"struct and nil", "a", `"node":true`},
		{"tuple", "a", "for ==: " + hint},
		{"list", "a", "for ==: " + hint},
		{"predicate fails", "ab?", `"value":"?"`},
		{"predicate with nil", "ab?", `"value":"?"`},
		{"predicate with nil", "a!", `"value":"!"`},
		{"nested lookahead, left recursion", "1+2", `"end":3,"value":[{"start":0,"end":3`},
		{"nested lookahead, repetition", "1+2", `"end":3,"value":[{"start":0,"end":3`},
	} {
		program := mustCompile(t, byName[tc.name])
		for _, backend := range []Backend{Closure, Bytecode, BytecodeIterative} {
			got := gentest.ResultJSON(program.ParseWith("main", tc.input, ParseOptions{Backend: backend}))
			if !strings.Contains(got, tc.want) {
				t.Errorf("%s %q on %s: %s, want %s", tc.name, tc.input, backend, got, tc.want)
			}
		}
	}
}

// TestCompareStaticErrors checks the comparisons that the types show to be errors, and those that
// are checked at run time. Two grammars that differed only in whether a left-recursive rule was
// memoized used to decide a comparison of nodes differently; both are rejected now.
func TestCompareStaticErrors(t *testing.T) {
	t.Parallel()
	const hint = "nodes and other structured values cannot be compared; use text(x), or .Start and .End"
	for _, tc := range []struct{ src, want string }{
		{`rule main = a:[a-z] b:[a-z] {$a == $b}`, "invalid operands node[string] and node[string] for ==: " + hint},
		{`rule main = a:[a-z] {$a != "a"}`, "invalid operands node[string] and string for !=: " + hint},
		{`rule main = a:[a-z] {"a" == $a}`, "invalid operands string and node[string] for ==: " + hint},
		{`rule main = a:[a-z]* {$a == $a}`, "for ==: " + hint},
		{`rule main = a:[a-z] => (1, 2) == (1, 2)`, "invalid operands (int, int) and (int, int) for ==: " + hint},
		{`rule main = a:[a-z] => 1 == "a"`, "invalid operands int and string for =="},
		{`rule main = a:[a-z] => true != 1`, "invalid operands bool and int for !="},
		{"rule main = &(&(a:e) b:e {$a == $b}) e\nrule e = e \"+\" n / n\nrule n = [0-9] #atomic\n", "for ==: " + hint},
		{"rule main = &(&(a:e) b:e {$a == $b}) e\nrule e = n (\"+\" n)*\nrule n = [0-9] #atomic\n", "for ==: " + hint},
	} {
		g, err := parseTest(tc.src)
		if err != nil {
			t.Fatalf("%q: %v", tc.src, err)
		}
		if _, err := Compile(g, compile.Options{}); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%q: got %v, want %q", tc.src, err, tc.want)
		}
	}
	// Comparisons that are valid, or are checked when they run, compile.
	for _, src := range []string{
		`rule main = a:[a-z] {text($a) == "a"}`,
		`rule main = a:[a-z]? {$a == nil}`,
		`rule main = a:[a-z] {$a != nil}`,
		`rule main = a:[a-z]? {$a == "a"}`,
		`rule main = a:[a-z] b:[a-z] {$a.Start == $b.Start && $a.End != $b.End}`,
		`rule main = a:[a-z] {len($a) == 1 && true != false && nil == nil}`,
		`rule main = a:[a-z] {$a.(_) == $a.(_)}`,
	} {
		g, err := parseTest(src)
		if err != nil {
			t.Fatalf("%q: %v", src, err)
		}
		if _, err := Compile(g, compile.Options{}); err != nil {
			t.Errorf("%q: %v", src, err)
		}
	}
}
