package tsgen_test

import (
	"testing"

	"github.com/ornew/zeroparse/internal/gen"
	"github.com/ornew/zeroparse/internal/gentest"
)

// The grammar takes the text of every level of a nesting: the texts sum to the square of the
// depth, so the parse is linear only if a text costs a constant. The script checks it in bytes,
// in code points and on invalid UTF-8.
func TestGeneratedTSTextCostIsConstant(t *testing.T) {
	t.Parallel()
	g, err := parseTest(`rule main = v:a $$ => $v
rule a = s:("(" a ")" / [^()]) => text($s)`)
	if err != nil {
		t.Fatal(err)
	}
	out := gentest.RunTSGrammarScriptWith(t, g, `import * as p from "./parser.ts";
declare const process: { exit(code: number): never };
function fail(why: string): never { console.log(why); return process.exit(0); }
const finishes = (input: string | Uint8Array, unit: p.PositionUnit, budget: number): boolean => {
  const r = p.parseWithOptions(input, { unit, budget });
  if (r.error instanceof p.ParseInterruptionError && r.error.kind === "budget-limit") return false;
  if (r.error !== null) fail("parse: " + r.error);
  return true;
};
const least = (input: string | Uint8Array, unit: p.PositionUnit): number => {
  let lo = 0, hi = 1;
  while (!finishes(input, unit, hi)) { lo = hi; hi *= 2; }
  while (hi - lo > 1) {
    const mid = lo + Math.floor((hi - lo) / 2);
    if (finishes(input, unit, mid)) hi = mid; else lo = mid;
  }
  return hi;
};
const nest = (depth: number, center: string) => "(".repeat(depth) + center + ")".repeat(depth);
const bytes = (depth: number) => {
  const b = new TextEncoder().encode(nest(depth, "x"));
  b[depth] = 0xff;
  return b;
};
const cases: [string, p.PositionUnit, (depth: number) => string | Uint8Array][] = [
  ["bytes", p.Bytes, (d) => nest(d, "x")],
  ["code points", p.CodePoints, (d) => nest(d, "é")],
  ["invalid UTF-8 in bytes", p.Bytes, bytes],
  ["invalid UTF-8 in code points", p.CodePoints, bytes],
];
for (const [name, unit, input] of cases) {
  const depths = [128, 256, 512]; // the JavaScript stack bounds the nesting
  const costs = depths.map((d) => least(input(d), unit));
  for (let i = 1; i < costs.length; i++) {
    const ratio = costs[i]! / costs[i - 1]!;
    if (ratio > 2.3) fail(name + ": cost at depths " + depths + " is " + costs + ": doubling the depth multiplies it by " + ratio.toFixed(2));
  }
}
console.log("ok");
`, gen.GenOptions{Start: "main", BudgetControls: true})
	if out != "ok\n" {
		t.Fatalf("got %q", out)
	}
}
