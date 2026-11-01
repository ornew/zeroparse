package engine

import (
	"strings"
	"testing"
)

// TestNestedLeaderResultNotReused checks that a failed optional has no effect on the result of
// a parse. S and R are both leaders of one left-recursion cycle. Evaluating S at 0 grows R at 0
// inside the growth of S, where R reads the growing result of S; that result of R must not be
// reused once the growth of S has moved on, nor by a later call of R at 0.
func TestNestedLeaderResultNotReused(t *testing.T) {
	const rules = `
rule S = S "q" / R "s" / "y"
rule R = R "r" / S "t"`
	cases := []struct{ name, main, input, want string }{
		// R at 0 alone grows to "yt".
		{"R alone", `R $$`, "yt", `("y"@S "t")@R`},
		// The optional fails and has no effect, so the parse is the one above.
		{"after a failed optional", `(S "!")? R $$`, "yt", `("y"@S "t")@R`},
		// Within the growth of S, the call of R in its second step reads the seed "y" of S.
		{"S grows through R", `S $$`, "yts", `(("y"@S "t")@R "s")@S`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			prog := mustCompile(t, "@start\nrule main = "+c.main+rules)
			for _, b := range []Backend{Closure, Bytecode, BytecodeIterative} {
				n, err := prog.ParseWith("", c.input, ParseOptions{Backend: b})
				if err != nil {
					t.Errorf("%s: %v", b, err)
					continue
				}
				if got := n.String(); !strings.Contains(got, c.want) {
					t.Errorf("%s: got %s, want a tree with %s", b, got, c.want)
				}
			}
		})
	}
}
