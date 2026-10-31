package gentest

import (
	"math/rand"
	"strings"
)

// SkipRunGrammars are grammars whose recovery skips have skip-run sites (see "Skip Runs" in
// spec/runtime/parsing.md), with the characters their random inputs are made of. Each exercises a
// part of what a record holds: the end of the rest of a walk, its farthest failure and
// expectations, and the input range examined by an element that looks ahead or at the preceding
// character.
var SkipRunGrammars = []struct {
	Name, Src, Alphabet string
}{
	{"repeated recovery", `rule main = (x / "a")* $$
rule x = (y #recover(skip=[a]+)) "c"
rule y = "b"`, "abc"},
	{"nested recovery", `rule main = s $$
rule s = ("(" s ")") #recover(skip=[a(]+)`, "(a)"},
	{"expectations under a label", `rule main = ("q" x @error(message="m") / "q")? (x / "a" / "q" x @error(message="m"))* $$
rule x = ("b" #recover(skip=[a]+)) "c"`, "qabc"},
	{"element with calls and lookaheads", `rule main = (st "!" / [a-z;=0-9 ])* $$
rule st = ("k" "=" num ";") #recover(skip=(word / !";" .)* ";"?)
rule word = [a-z]+ !"="
rule num = [0-9]+`, "k=1;ab !"},
	{"line starts in the element", `rule main = ln* $$
rule ln = (x "!" / [abcz])* "\n"
rule x = ("b" #recover(skip=(^ "a" / "c" "c" / [a\n] !"z")*)) "c"`, "abcz\n!"},
	{"look-ahead past the end", `rule main = ln* $$
rule ln = ("!" x "c" / "?" x "d" / [abcdz!?])* "\n"
rule x = "b" #recover(skip=(^ "a" / "d" "d" / [a\n!?] !"z")*)`, "abcdz!?\n"},
	{"empty elements and minimums", `rule main = (x / .)* $$
rule x = ("b" #recover(skip=("a"? ","?)#repeat(2,) ("." "a"?)*)) "c"`, "a,.bc"},
	{"silent walks", `rule main = (!x "a" / x / .)* $$
rule x = ("b" #recover(skip=[a]+ "!"?)) "c"`, "ab!c"},
}

// SkipRunInputs returns the inputs a grammar is parsed with: n random strings over alphabet, made
// of short runs of a character, and longer runs of each character. Records are reused only by the
// third walk over a position, so the strings are long enough for walks to overlap often.
func SkipRunInputs(alphabet string, n int, rng *rand.Rand) []string {
	var inputs []string
	for _, ch := range alphabet {
		inputs = append(inputs, strings.Repeat(string(ch), 9))
	}
	for i := 0; i < n; i++ {
		var b strings.Builder
		for k := rng.Intn(40); k > 0; k-- {
			ch := alphabet[rng.Intn(len(alphabet))]
			for r := 1 + rng.Intn(3)*rng.Intn(3); r > 0; r-- {
				b.WriteByte(ch)
			}
		}
		inputs = append(inputs, b.String())
	}
	return inputs
}

// SkipRunCases returns the cases of SkipRunGrammars, each with n random inputs.
func SkipRunCases(n int) []Case {
	rng := rand.New(rand.NewSource(75))
	var cases []Case
	for _, g := range SkipRunGrammars {
		cases = append(cases, Case{Name: "skip runs/" + g.Name, Src: g.Src, Inputs: SkipRunInputs(g.Alphabet, n, rng)})
	}
	return cases
}
