package runtime

import (
	"strings"
	"testing"
	"unsafe"
)

// A code-point source too large for int32 offsets still gives text without copying: the first
// text builds a table of int offsets over the source itself (or over the decoded input when the
// source holds invalid UTF-8).
func TestTextOfSourceBeyondInt32Offsets(t *testing.T) {
	old := offsLimit
	offsLimit = 64
	defer func() { offsLimit = old }()
	valid := strings.Repeat("aé世😀", 50)
	invalid := strings.Repeat("aé\xff😀", 50)
	for _, controlled := range []bool{false, true} {
		for _, tc := range []struct {
			name, source string
			shares       bool
		}{{"valid", valid, true}, {"invalid", invalid, false}} {
			p := &Parser{unit: CodePoints}
			if controlled {
				p.Control = newBudgetControl(nil, Limits{Budget: 1 << 30})
			}
			p.setSource(tc.source)
			if p.offs != nil {
				t.Fatalf("%s controlled=%t: int32 table above the limit", tc.name, controlled)
			}
			want := []rune(strings.ToValidUTF8(tc.source, "�"))
			if len(p.In) != len(want) {
				t.Fatalf("%s: %d code points, want %d", tc.name, len(p.In), len(want))
			}
			for _, r := range [][2]int{{0, len(want)}, {3, 40}, {10, 10}, {len(want) - 1, len(want)}} {
				got := p.Text(r[0], r[1])
				if got != string(want[r[0]:r[1]]) {
					t.Fatalf("%s controlled=%t: text%v = %q", tc.name, controlled, r, got)
				}
			}
			if p.wide == nil || p.offs != nil || len(p.wide) != len(want)+1 {
				t.Fatalf("%s controlled=%t: wide table not built", tc.name, controlled)
			}
			if tc.shares {
				got := p.Text(1, len(want)-1)
				if unsafe.StringData(got) != unsafe.StringData(tc.source[len("a"):]) {
					t.Fatalf("controlled=%t: text copied", controlled)
				}
			}
			// Later texts neither allocate nor charge.
			if n := testing.AllocsPerRun(10, func() { _ = p.Text(2, len(want)-2) }); n != 0 {
				t.Fatalf("%s controlled=%t: text allocates %v", tc.name, controlled, n)
			}
			// A source within the limit returns to int32 offsets.
			p.setSource("abc")
			if p.wide != nil || p.Text(1, 3) != "bc" || p.offs == nil {
				t.Fatalf("%s controlled=%t: small source after a wide one", tc.name, controlled)
			}
		}
	}
}
