# 132. Records of the Walks of Recovery Skips

- **Status**: Implemented
- **Date**: 2026-10-31

## Problem and Mechanism

The skip expression of a `#recover` runs at every position where its base fails. When a repetition retries a
failing recovery at each position, every skip walks forward over the input that the skips before it walked. With

```
main = (x / "a")* $$
x = (y #recover(skip=[a]+)) "c"
y = "b"
```

and the input `a` n times, `y` fails at every position, `[a]+` walks to the end of the input, `"c"` fails, and the
recovery is abandoned: the parse takes time quadratic in n on every backend. Nested recoveries, where each skip
starts one position before the one before it, and walks that alternate between two regions of the input do the
same.

An unbounded repetition in a skip expression is now a **skip-run site** when the rest of its walk from a position
depends only on that position and the input (the conditions are listed in
[Skip Runs](../../spec/runtime/parsing.md#skip-runs)). The walk from a position ends where the walk from any later
position it passes ends, so the parser keeps a slot per position, per site and lookahead state, that is fresh,
visited or recorded:

- A walk starts plain and marks the position of each iteration it moves past. It pays for nothing else, so a
  parse whose skips walk over each position once costs what it did, and a walk that fails at once, as the skips
  of an error-free parse do at the end of a list, touches no slot.
- At its first iteration at a position that an earlier walk visited, the walk starts recording. When it ends, at a
  failed or empty iteration or at a recorded position, it stores the rest of the walk for every position it
  visited while recording, from the last back to the first: the end, the farthest failure with its expectations,
  and the input range the rest examined.
- A recording walk that has matched its minimum looks up each position before iterating there. At a recorded
  position it takes the end from the record, records the expectations again and extends the examined range, as a
  memo hit does.

Records are stored only when a walk ends, so a parse that a limit or a cancellation stops in the middle of a walk
leaves none behind. A position is evaluated at most once while its slot is fresh and once more before it is
recorded, plus the minimum number of iterations and one more per walk, whatever the order of the walks:
[the proof](../../spec/proofs/skip-runs.md) shows that a reused record equals the rest of the walk and gives the
bound. A slot that is not fresh is marked by a bit over the span of the positions walked, and only recorded slots
hold a record, kept in a map by position.

The compiler marks the sites (`SKIPREPEAT` in bytecode, `SkipBegin`, `SkipVisit`, `SkipHit` and `SkipEnd` in Flow
IR), and the engine, the generated Go runtime and the TypeScript runtime keep the same slots and charge the same
budget units.

## Applicability

| Execution profile | Applied / Partial / Not applied / Not applicable | Reason |
|:--|:--|:--|
| Closure, bytecode, iterative bytecode | Applied | `SkipBegin`, `SkipVisit`, `SkipHit` and `SkipEnd` of the engine core |
| Generated Go, ordinary and budget-controlled | Applied | The same operations in the runtime, emitted from Flow IR |
| Generated typed Go | Applied | Shares the runtime operations |
| Generated TypeScript | Applied | The same operations in the TypeScript runtime |
| Documents | Applied | Slots last for one parse; results examine what the walks behind them examined |
| Streams | Applied | The slots before a commit are dropped |
| Missing-token insertion | Not applied | A parse that inserts tokens keeps no slots |

A repetition in a rule that a skip expression calls is not a site: the rule can be called from outside skips, where
its walks are not abandoned in the same way. Such a grammar stays quadratic, which
[the classification](../../spec/runtime/parsing.md#classes-of-growth) shows with its own row.

## Reproduction

Base `049b5245`, Go 1.27.1, Apple M3 Max, benchlock, each run started with a load below 8. Test binaries of the
`bench` package, one per revision, ran interleaved for 11 rounds with `-benchmem` over `Parse` and `Recognize` (JSON,
left-recursive calculator, Minilang, Recovery), `Incremental` and `Stream`; benchstat compared them. Tests:
`TestSkipRunsMatchParsesWithoutRecords`, `TestSkipRunsNegativeControls`, `TestSkipRunsBudgetAbort`,
`TestSkipRunsDocument`, `TestSkipRunsStream`, `TestSkipRunsWideMinimum`, `TestSkipRunsAbortStoresNothing`,
`TestSkipRunsAbortControl`, `TestSkipRunsNeverRecordControl`, `TestSkipRunsLinearMemory`,
`TestSkipRunsGeneratedMatchEngine`, `TestSkipRunsGeneratedStream`, `TestSkipRunsGeneratedDocuments`,
`TestSkipRunsGeneratedLinear`, `TestSkipRunsTSMatchEngine`, `TestSkipRunsTSLinear` and the E8 rows of
`TestControls`.

## Results

Steps of the engine (`Steps` of the complexity suite, the same on the three backends up to a few percent):

| Grammar, input | n = 250 | 500 | 1000 | Before, n = 1000 |
|:--|--:|--:|--:|--:|
| Repeated skips (above), `a` n times | 11,416 | 22,912 | 45,952 | 1,036,288 |
| Nested skips, `(` n times then `a` | 10,864 | 21,616 | 44,128 | 1,039,360 |
| Alternating regions, `a` n times, `b`, `a` n times | 30,688 | 61,504 | 125,056 | 4,108,288 |

The nested grammar is `main = (s "!" / [a(]*) $$; s = ("(" s ")") #recover(skip=[a(]+)` and the alternating one is
`main = (p / "a" / "b")* $$; p = x "b" x "z"; x = "c" #recover(skip=[a]+)`. Generated Go and TypeScript spend
budget units in the same proportion at n = 200, 400 and 800 (slopes at most 1.2), and walks that never record
give slopes of at least 1.7. The memory a parse allocates grows linearly in n up to 16,000 on the nested grammar.

Speed and allocation (medians of 11 interleaved samples against the base; changes benchstat reports as significant,
p < 0.05):

| Benchmark | Time | Bytes allocated | Allocations |
|:--|--:|--:|--:|
| Parse, Recovery, closure and bytecode | ~ | +0.3% | +12 (+0.6%, +0.7%) |
| Parse, Recovery, iterative | +2.8% | ~ | +12 (+0.7%) |
| Parse, Recovery, generated | +4.2% | +0.5% | +12 (+1.0%) |
| Recognize, Recovery, all four backends | ~ | +27 to +30 KiB (+11% to +19%) | +12 except closure (+4.6% to +7.7%) |
| Recognize, JSON, iterative | +2.9% | ~ | ~ |
| Recognize, JSON and calculator, closure and bytecode | ~ | +0.3 to +0.4 KiB (+1.7% to +4.5%) | 0 or +1 |
| Recognize, Minilang, closure and bytecode | ~ | -2.6%, +3.5% | ~ |
| Incremental, document, engine backends | ~ | +0.4% | ~ |
| Every other case, including Stream | ~ | ~ | ~ |
| Geometric mean | +0.7% | +1.6% | +0.7% |

The Recovery input is the only benchmark input with errors to recover from, and its skips walk each position
once: the added allocations are the slot bits and the map of the sites that walk, and the time grows by at most
4.2%, on the generated parser. The other changes are below 4.5% and within the spread of the samples; the byte
counts of Stream differ by less than 0.01%.

Budget totals of the budget-controlled form grow on the corpus grammars whose skips have sites: an iteration of a
site is a loop that charges a step per element, where the scan of a character class charged only for the
characters it read, and a recording walk charges its lookups and records. The results of every case are unchanged.

## Correctness and Tradeoffs

A record is reused only by a recording walk, and only once the walk has matched its minimum, so a reuse never
changes whether a walk succeeds. Every record stored is the rest of a walk, by induction on the order of storing,
so whether a walk is plain or recording decides only which positions get records, never the result.

Recording from the first walk would make every skip pay for keeping expectations and storing records, including
the skips of inputs with one error. Marking first defers the cost to positions that a second walk reaches. Storing
records when their iteration ends instead of when the walk ends would leave records that hold only one iteration;
the negative control that does so changes the results of the nested grammar on every backend. The control that
never records turns the E8 grammars quadratic on every backend.

Slots kept densely, from the least to the greatest position a site visited, hold the input between walks that
are far apart. Minilang tries its skip at the end of every block, where it fails at once; with dense slots marking
those positions, a parse of the error-free benchmark input allocated 36 MiB instead of 10 MiB, parses and recognitions took 10% to 36%
longer, and a recognition allocated 26 MiB instead of 79 KiB. Marking only the positions a walk moves past, and
keeping the slots in a map, removes both costs; a map lookup costs more than an index on the long walks of the
families above, which stay linear.

## Decision and Limits

Adopted on every backend. Not covered: repetitions in rules that skip expressions call, which stay quadratic in
the families above.
