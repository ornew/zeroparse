package runtime

import (
	"errors"
	"strings"
	"testing"
)

// The first text of a code-point input containing invalid UTF-8 decodes it once; later texts are
// substrings of the decoded input that charge nothing.
func TestTextDecodesInvalidInputOnce(t *testing.T) {
	source := strings.Repeat("é\xff", 1000)
	want := strings.ToValidUTF8(source, "�")
	for _, controlled := range []bool{false, true} {
		p := &Parser{unit: CodePoints}
		if controlled {
			p.Control = newBudgetControl(nil, Limits{Budget: 1 << 30})
		}
		p.setSource(source)
		if p.offs != nil {
			t.Fatal("invalid input has an offset table before any text")
		}
		if controlled {
			// The decoding is interruptible and leaves nothing behind.
			p.Control = newBudgetControl(nil, Limits{Budget: 100})
			var err error
			func() {
				defer func() {
					if x := recover(); x != nil {
						err = x.(fatal).err
					}
				}()
				_ = p.Text(0, 2000)
			}()
			var le *LimitError
			if !errors.As(err, &le) || p.offs != nil || p.decoded {
				t.Fatalf("decoding interruption: %v", err)
			}
			p.Control = newBudgetControl(nil, Limits{Budget: 1 << 30})
		}
		if got := p.Text(0, 2000); got != want {
			t.Fatalf("controlled=%t: text differs", controlled)
		}
		var used uint64
		if controlled {
			used = p.Control.budgetUsed()
		}
		if got := p.Text(1, 1999); got != want[len("é"):len(want)-len("�")] {
			t.Fatalf("controlled=%t: decoded substring differs", controlled)
		}
		if controlled && p.Control.budgetUsed() != used {
			t.Fatalf("text of a decoded input spent %d", p.Control.budgetUsed()-used)
		}
		// A new source discards the decoded one.
		p.setSource("a\xfeb")
		if p.decoded || p.offs != nil {
			t.Fatal("a new source kept the decoded one")
		}
		if got := p.Text(0, 3); got != "a�b" {
			t.Fatalf("text of a new source %q", got)
		}
	}
}
