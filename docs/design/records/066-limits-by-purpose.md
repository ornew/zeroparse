# 066. Limits Chosen by Purpose

- **Status**: Accepted
- **Author**: @ornew
- **Date**: 2026-11-01

## Summary

A parse is bounded by one configuration with one field per **measure**: compute, memory, input length and nesting
depth. A caller selects what bounds the parse by giving the measures it cares about a finite value. Each field takes
the same three kinds of value: zero selects the field's default, `Unlimited` disables it, and any other value is a
cap. The parse fails at the first measure it exhausts, with one error type whose kind names the measure.

```go
type Limits struct {
    Compute uint64 // cost units: steps, inspected input, value construction
    Memory  uint64 // bytes of parser-owned storage
    Input   uint64 // length of the input in the position unit
    Depth   uint64 // nesting of recursive rule calls
}

func WithLimits(l Limits) ParseOption

type LimitError struct {
    Kind      LimitKind // LimitCompute, LimitMemory, LimitInput or LimitDepth
    Limit     uint64
    Used      uint64
    Requested uint64
}
```

Today compute is the only measure that bounds memory, so a caller who wants a tight memory bound must also cut the
time the parse may take. This record adds a **memory** measure that is charged only where parser-owned storage is
allocated. It also moves the depth limit, which today is a separate option with its own error, into the same
configuration. The configuration and its error are renamed once, together. The memory measure is one addition to a
parser-local counter where a storage chunk is refilled, and the cap is checked in the compute slow path. A finite cap
makes that slow path run every 256 units, as a cancelable context does. A prototype in the generated Go runtime shows no
measurable cost on the benchmark grammars with memory unlimited (0.97 to 1.02 times the base). With a finite cap,
parses take up to 1.03 times the base for Node results and 1.06 for typed results, and the cost comes from the poll
interval, not from counting.

This record revises decision 4 of [record 064](064-grammar-complexity.md), "Memory has no bound beyond cost", for the
reasons in [Why Compute Does Not Suffice](#why-compute-does-not-suffice).

## Motivation

### What bounds a parse today

A parse has two limits and one separate option:

| Control | API | Error | Default |
|:--|:--|:--|:--|
| Cost budget | `Limits.Budget` through `WithLimits` | `LimitError`, kind `LimitBudget` | `max(2^20, 1024 · n)` |
| Input length | `Limits.MaxInput` through `WithLimits` | `LimitError`, kind `LimitInput` | `Unlimited` |
| Nesting depth | `WithMaxDepth(int)` | a plain error, `nesting too deep: more than N nested recursive rule calls` | 100,000; 10,000,000 on the iterative VM; the generated default from `zp gen -max-depth` |

The same three controls exist in generated Go (package-local `WithLimits`, `WithMaxDepth`) and generated TypeScript
(the `budget`, `maxInput` and `maxDepth` options, `ParseInterruptionError` with kinds `"budget-limit"` and
`"input-limit"`), and on the command line (`-budget`, `-max-input`, `-max-depth`). The budget is described in
[record 033](033-cancellable-parses-and-budgets.md) and the [parsing specification](../../../spec/runtime/parsing.md#cancellation-and-budget-limits).

The depth limit is a limit in every sense except its API: it bounds a resource (the call stack), it aborts the parse
as a run-time error, and it has a default. A caller cannot match it with `errors.Is(err, ErrLimit)`, and it takes an
`int` where every other limit takes a `uint64` with the `Unlimited` convention.

### Why compute does not suffice

The budget counts work. It bounds memory only because every allocation the parser makes on behalf of a parse is
preceded by a charge, so the bytes allocated by a batch parse are at most k times the compute spent, for a constant k
that depends on the backend and the grammar. The argument, site by site, for the generated Go runtime (the engine has
the same structure in `internal/engine/core/alloc.go` and `memo.go`):

| Storage | Allocated where | Compute charged | Bytes per unit |
|:--|:--|:--|:--|
| Nodes | `NewNode`, `NewTerminal`, `NewBranch` (slab of 256) | one step per node (`Spend`) | 104 |
| Child lists | `Nodes(n)` (pointer slab of 1,024; larger lists alone) | `copySpend` from `Kids`, one unit per element | 8 |
| Field lists | `fields(n)` (slab of 256) | one unit per field copied | 32 |
| Memo entries | `memoTable.alloc` (slab of 256) | the rule call that stores the entry, one step | 128 |
| Memo slots | `memoTable.put` (doubling array by position) | the input inspected to reach the position | ≤ 16 amortized |
| First-call bit sets | `memoTable.markSeen`, `sparsePage` | the input inspected to reach the position | ≤ rules / 8 |
| Decoded input (`CodePoints`) | `setSource` (rune array and offsets) | `setControlledSource`, one unit per byte | ≤ 8 |
| Strings and lists built by actions | `spendValue`, `appendSpend`, `copySpend` | one unit per element or byte | element size |
| Typed values | generated slabs, `tarena` chunks | one step per value | the size of the value type |

So for a batch parse, allocated bytes ≤ k · compute, where k is the largest bytes-per-unit ratio over the sites:
128 for memo entries, or rules / 8 for a grammar with more than about a thousand rules. That bound fails or degrades in
three ways:

1. **It is loose.** The prototype measured charged bytes against compute spent on the bundled benchmark grammars: 3.5
   to 7.2 bytes per unit (table in [Performance and Results](#performance-and-results)). The worst case is 128 bytes per
   unit, so to guarantee M bytes through compute, a caller must allow M / 128 units, about 20 times fewer than
   typical parses of the same memory need. A 1 MiB memory guarantee through compute alone admits 8,192 units. At the
   measured 3.5 to 26 units per input byte, that is 300 to 2,300 bytes of input.
2. **Documents retain storage that a parse does not pay for.** A `Document` keeps memo entries, repetition records and
   input pieces between parses. A parse after a small edit spends little compute and reuses that storage without
   charging it. Its compute bounds only what it allocates now, not the Document's footprint.
3. **Streams release storage that compute keeps counting.** Compute is cumulative over the whole stream, while the
   memory a stream holds is the window since its last commit. A compute bound tight enough to cap the window also caps
   the length of the stream.

A server that parses untrusted input wants the opposite trade-off: a generous time allowance, enforced by a context
deadline or a large compute budget, and a tight bound on memory, because many parses run at once. An editor wants a
bound on a Document's footprint across edits. Neither is expressible today.

## Goals

- One configuration in which a caller chooses what bounds a parse by setting the fields it cares about.
- A memory measure with a guarantee stated in bytes, deterministic for a given artifact, input and options.
- The depth limit as a field of the same configuration, failing with the same error type.
- One rename of the public names (Go engine, generated Go, TypeScript, command line, playground) instead of several.
- No measurable cost when memory is unlimited, including in a parser generated without limit controls.

## Non-Goals

- **Process memory.** The memory measure counts storage the parser allocates for the parse, estimated from the sizes
  of its own types. It does not read `runtime.MemStats`, which is process-wide and depends on the collector, and it does
  not count caller code, the caller's input string, or the Go runtime's own overhead.
- **Elapsed time.** A context deadline bounds time. Compute is the deterministic proxy.
- **Recovery and diagnostics knobs.** `WithMissingTokens` sets how many tokens recovery may insert. It changes the
  result, so it is grammar semantics, not a resource limit, and keeps its own error, `MissingTokenLimitError`.
  `WithDiagnosticLimit` is a retention policy for stream diagnostics that never fails a parse. Both stay separate
  options.
- **Graceful degradation.** Exhausting a measure aborts the parse. Evicting memo entries to stay under a memory cap is
  a different mechanism and is not proposed. The one exception is a Document's caches, which are dropped before the
  parse fails ([Documents](#documents)).

## Design

### Measures

Each measure is a counter with a cap, and each states what its cap guarantees.

**Compute** (today's `Budget`). Cost units, charged as [record 033](033-cancellable-parses-and-budgets.md) defines:
execution steps, inspected input elements, and value construction in proportion to its size. It is cumulative per
invocation and never refunded. Guarantee: the parse performs at most `Compute` units of work. Because every allocation
is charged, it also bounds allocation by k · `Compute` with the k of [the table above](#why-compute-does-not-suffice).
Its slow path also observes cancellation and, with a finite memory cap, checks memory.

**Memory**. Bytes of parser-owned storage. They are added to the counter when the storage is taken into use, and
subtracted when the parser releases storage at a point it controls:

- *Charged*: every chunk a slab or arena takes into use for the parse (nodes, child lists, field lists, capture frames,
  memo entries, typed-value slabs and `tarena` chunks), allocations that bypass a slab (lists longer than a quarter of
  a chunk), growth of the memo slot array and first-call bit sets, the decoded input in `CodePoints` (rune array and
  offset table), the bytes of strings and lists that actions build, the growth of working stacks (`KidStack`, the
  iterative VM's call stack), and a stream's input buffer.
- *Returned*: a stream's commit, for the input it discards, the memo entries it retires (in practice they are reused
  from the free list without a new charge), and the node chunks of elements already delivered. A Document's history
  reset returns its caches.
- *Not returned*: storage abandoned by backtracking. It is garbage, but only the collector knows when it is released,
  and returning it would put a charge on every failure path, the hottest paths of a PEG parse.

Guarantee: at every point of the parse, the bytes the parser holds for it are at most the counter. The parse fails at
the first poll at which the counter exceeds `Memory`, so the counter passes the cap by at most what the parse
allocates between two polls. That is one chunk per slab plus at most 256 compute units' worth of allocation: tens of
kilobytes, or one list larger than a chunk. The caller's input string is not charged: `Input` bounds it.

Unlike compute, memory is not reserved before the allocation. A reservation needs a call in the refill branch, and a
call there pushes the inlined allocators over Go's inlining budget. The first prototype did that, and its Node parses
took 1 to 2 % longer even with memory unlimited ([Performance and Results](#performance-and-results)). An addition to
a field costs almost nothing in the budget. The check rides on the compute slow path, which a finite cap makes run
at every poll. [Decision 11](#decisions) records why
strict reservation was rejected.

Charging is deterministic because it follows the logical use of chunks, not the physical allocation. A parser reused
from a pool may still hold partially used chunks from an earlier parse, and a memo table keeps its chunks between
parses. A parse with a finite `Memory` therefore starts with empty node, pointer and field slabs, and the memo table
charges each chunk it takes into use whether the chunk is new or kept. With memory unlimited, a reused parser keeps
using its leftover chunks as it does today.

**Input**. The length of the input in the position unit, checked before any work (today's `MaxInput`, unchanged).
Guarantee: no input longer than `Input` is parsed. A stream fails when the parser asks for a unit beyond the cap.

**Depth**. The nesting of recursive rule calls and Operators Rule operands, as the
[specification](../../../spec/runtime/parsing.md#limits) defines it (today's `WithMaxDepth`, unchanged in what it
counts). Guarantee: the call stack holds at most `Depth` counted entries plus a grammar-constant number of others.
Unlike the cumulative measures, it is a level: it rises on entry and falls on return.

### Configuration and Defaults

| Field | Unit | Zero selects | `Unlimited` |
|:--|:--|:--|:--|
| `Compute` | cost units | `max(2^20, 1024 · n)`, or the classifier's `c_G · (n + 1) + k` once that default lands | no compute bound |
| `Memory` | bytes | `Unlimited` | no memory bound |
| `Input` | position units | `Unlimited` | no input bound |
| `Depth` | counted entries | the backend's default (100,000; 10,000,000 on the iterative VM; the generated default) | no depth bound, on the iterative VM only; an option error elsewhere ([decision 6](#decisions)) |

`Memory` has no finite default because compute already bounds memory for every parse with a budget. A second default
cap would add a failure mode without adding a guarantee that callers who do not ask for one need. The zero and
`Unlimited` convention and the single `Unlimited` constant remain: one constant disables every field, and a field
added later takes the same meaning.

A parse runs controlled when compute or memory is finite or a context is given; that is unchanged for compute. A
finite `Memory` alone makes the parse controlled. `Input` alone does not, because it is checked once on entry. `Depth`
is checked on every backend and in every generated form, controlled or not, as today.

At a poll, cancellation wins (as today), then memory, then compute. Compute and memory are checked in the same slow
path, so the order is deterministic. A finite `Memory` without a context makes the control poll every 256 units, the
interval of a cancelable context, but without calling the context.

### Documents

The memory counter of a `Document` parse starts at the bytes the Document already retains: the memo entries,
repetition records and input pieces that [`CacheStats.EstimatedBytes`](035-incremental-cache-controls.md) reports.
A finite `Memory` is therefore a bound on the Document's footprint after the parse, not only on what the parse adds.
If the retained bytes alone exceed the cap when `Parse` begins, the Document drops its history, as `ResetHistory`
does, and parses from scratch under the cap. It fails only if that fresh parse exceeds the cap. Caches are an
optimization, and dropping them before failing keeps the cap from turning a reusable Document into a permanently
failing one. Compute and the other measures keep their per-`Parse` meaning.

### Streams

A stream charges its input buffer and returns it at each commit, and memo entries retired at a commit are reused from
the free list without a new charge. A finite `Memory` therefore bounds the stream's window: the uncommitted input,
the memo entries and nodes since the last commit, and the diagnostics it retains. It does not bound the stream's
length. Compute keeps its adaptive default, which follows the furthest demanded position. `Input` keeps its stream
meaning.

### Go API

The engine (`zp`) and generated Go packages expose the same names:

```go
const Unlimited = math.MaxUint64

type Limits struct{ Compute, Memory, Input, Depth uint64 }

func WithLimits(Limits) ParseOption

var ErrLimit = errors.New("parse limit exceeded")

type LimitKind uint8

const (
    LimitCompute LimitKind = iota
    LimitMemory
    LimitInput
    LimitDepth
)

type LimitError struct{ Kind LimitKind; Limit, Used, Requested uint64 }
```

`WithMaxDepth` is removed: `Limits.Depth` replaces it. For `LimitDepth`, `Limit` is the resolved depth, `Used` the
depth reached and `Requested` one. For `LimitMemory`, `Used` is the counter at the poll that failed, and `Requested`
is zero, because memory is checked at polls rather than reserved. A generated parser without limit controls accepts zero
and `Unlimited` for `Compute` and `Memory` and rejects a finite value, as it rejects a finite budget today. It accepts
any `Input` and `Depth`.

### Generated Code

`zp gen -budget-controls` becomes `-limits`: it generates the controlled form, which charges compute and memory.
Memory charges add no per-site cost (see below), so a separate generation flag for memory would buy nothing.
[Decision 7](#decisions) records why measures are not selected at generation time, as optimization profiles are.

The typed-value slabs are generated per grammar (`tslabs` in `parser.go`). Their refill functions get the same
one-line addition as the runtime's slabs. The runtime allocators are shared by both forms, so a parser generated
without limit controls also executes one addition per chunk refill, about one per 256 objects. No control reads that
counter. This form was not benchmarked, and the argument that the cost is negligible rests on the `off` arm, which
executes the same additions.

### TypeScript

The options mirror the Go configuration in one nested object, because a flat `input` option would read as the parse
input:

```ts
parse(text, { limits: { compute: 0, memory: 64 << 20, input: 0, depth: 0 } });
```

`ParseInterruptionError` keeps its name, because it also reports cancellation. Its kinds become `"canceled"`,
`"compute-limit"`, `"memory-limit"`, `"input-limit"` and `"depth-limit"`, and the depth error becomes a
`ParseInterruptionError`. The TypeScript runtime allocates objects one by one, not from slabs, so memory would be
charged per object at the sites that already charge compute for construction, from estimated object sizes. That is a
hot-path charge, and it is not measured in this record. [Decision 2](#decisions) orders TypeScript memory after
Go.

### Command Line

The commands that run a parse (`parse`, `trace`, `profile`, playground) take `-max-compute`, `-max-memory`,
`-max-input` and `-max-depth`. Each accepts a number, `0` for the default, or `unlimited`. `zp sample -budget` and
`zp sample -max-depth` bound input generation, not a parse. They keep their names and are documented as unrelated.

### Static Bounds and the Classifier Default

The classifier's default ([record 064](064-grammar-complexity.md#budgets)) chooses the compute default. It does not
choose a memory default. A `Linear` grammar also has a static memory bound m_G · (n + 1) + k, because memo entries
are at most one per rule, position and environment, and nodes at most proportional to compute. That bound is
available to a caller but is not applied without being asked for.

Skipping runtime charges where a static bound suffices applies per measure:

- **Compute**: its charges are on hot paths, and that is where elision pays (the budgeted form takes 1.12 to 1.37
  times the uncontrolled form).
- **Memory**: its charges are additions in refill branches, so eliding them saves nothing measurable. Eliding the
  poll interval that a finite cap imposes saves up to 6 %. A finite `Memory` that is at
  least the static bound lets a parse with no other finite measure and no context run the uncontrolled form.
- **Depth**: a grammar with no recursive rule and no Operators Rule has a static depth bound, so its depth checks can be
  removed. That is independent of this record.

A parse runs the uncontrolled form only if every finite measure is statically satisfied and no context is given.

### Rename Plan

Every name changes in one change, with no aliases, because the formats and APIs carry no compatibility promise before
the first release.

| Before | After |
|:--|:--|
| `Limits.Budget` | `Limits.Compute` |
| `Limits.MaxInput` | `Limits.Input` |
| `WithMaxDepth(n)` | `Limits.Depth` |
| (new) | `Limits.Memory`, `LimitMemory` |
| `LimitBudget` | `LimitCompute` |
| (plain error) | `LimitDepth` |
| `zp gen -budget-controls` | `zp gen -limits` |
| `-budget` | `-max-compute` |
| (new) | `-max-memory` |
| TypeScript `budget`, `maxInput`, `maxDepth` | `limits: { compute, input, depth }`, plus `memory` |
| TypeScript `"budget-limit"` | `"compute-limit"`, plus `"memory-limit"` and `"depth-limit"` |

The rename touches `zp.go`, `internal/limits`, `runtime/budget.go`, the generated `api.go` template, the TypeScript
runtime, `cmd/zp`, `cmd/playground`, and the specification pages for parsing, generated parsers, streaming and
Documents. `Limits`, `WithLimits`, `LimitError`, `LimitKind`, `ErrLimit` and `Unlimited` keep their names
([decision 1](#decisions)).

### Implementation Sequence

1. Rename, and move depth into `Limits` with `LimitDepth`. This has no new charges. Every backend reports depth through
   `LimitError`, and the depth tests switch to `errors.As`.
2. Add memory in the engine and generated Go: the charge sites below, the slab reset for a finite cap, Document
   carry-over and stream returns. Tests: threshold tests per site (a cap equal to the final counter succeeds, a smaller
   one fails within one poll interval), inlining checks for the allocators (`-gcflags=-m`), determinism with a pooled
   parser, Document cache drop, stream window bound.
3. Add memory in TypeScript, once its per-object charge is measured.

## Prototype Charge Sites

The measurement prototype is not committed. It charged memory at the following sites, and an implementation follows
the same placement. Each site is one addition to a counter, inside the branch that refills a chunk:

- `runtime/runtime.go`:
  - the four node slab refills (`NewNode`, `NewNodeOrdinary`, `NewTerminalOrdinary`, `NewBranchOrdinary`; 256 × 104
    bytes);
  - `Nodes(n)`: the pointer slab refill (1,024 × 8 bytes) and the large-list path (n × 8);
  - `fields(n)`: the slab refill (256 × 32 bytes) and the large-list path (n × 32);
  - `memoTable.alloc`, on taking a chunk (256 × 128 bytes, charged whether the chunk is new or kept from an earlier
    parse), into a counter of the memo table;
  - `memoTable.put`, on extending the slot array by its logical length, not its capacity ((pos + 1 − len) × 8).
  - The counters are `Parser.memUsed` and `memoTable.memBytes`. Both are fields, not pointers, so the addition has no
    nil check.
- `runtime/budget.go`:
  - `BudgetControl` gains the cap and pointers to the two counters, attached at parse start.
  - `chargeSlow` fails with the memory kind when their sum exceeds the cap.
  - `refill` keeps the 256-unit poll interval when the cap is finite, even without a context.
- Parse start (the batch entry in `runtime.go`, `stream.go`, `document.go`): attach the counters, reset them, and, with a
  finite cap, start with empty node, pointer and field slabs.

Inlining costs, from `go build -gcflags=-m=2`, of the allocators that must stay inlined:

| Function | Before | With the counter addition |
|:--|--:|--:|
| `NewNodeOrdinary` | 43 | 47 |
| `NewTerminalOrdinary` | 58 | 62 |
| `NewBranchOrdinary` | 56 | 60 |
| `Nodes` | 43 | 54 |
| `fields` | 37 | 48 |
| `memoTable.alloc` | 76 | 80 |

Two call forms were tried before the counter form, and both broke inlining. A reservation call in the refill branch
and an out-of-line refill function each put `NewNodeOrdinary`, `Nodes` and `memoTable.alloc` over the limit of 80, at
costs of 87 to 103. A checked addition with a nil test (cost 23 per site) put `Nodes`, `fields`, `NewTerminalOrdinary`
and `NewBranchOrdinary` over it.

`memoTable.alloc` is at the limit of 80. An implementation must keep it inlined, for example by moving the chunk
refill out of line. The test suite checks the inlining.

The engine prototype (`internal/engine/core/alloc.go`: node, pointer and field slab refills and the large-list path)
used the first form, a call to a charge function in the refill branch, and made `NewNode`, `Nodes` and `fields` lose
inlining. Its closure-backend arm still showed no difference beyond noise, because the closure backend's per-node
work dominates. An implementation uses the counter form there too.

The implementation adds the sites the prototype left out:

- capture frame scratch chunks;
- first-call bit sets and pages;
- decoded input;
- action strings and lists, next to the existing `spendValue` charges, in the controlled path only;
- working-stack growth;
- typed `tarena` chunks and the generated `tslabs` refills;
- the engine memo slab;
- stream buffers and their returns.

## Alternatives Considered

- **Compute only, with documented k.** Keep record 064's decision and document the bytes-per-unit ratio, so a caller
  can derive a compute budget from a memory target. It needs no new code, but the bound is 20 times loose and does
  not cover Documents ([above](#why-compute-does-not-suffice)).
- **Weighted single counter.** One counter whose per-site weights depend on a selected purpose: bytes at allocation
  sites and zero at scans for a memory purpose, units everywhere for compute. It keeps one counter, but a parse can
  then be bounded by only one measure. The weights would also have to be loaded at run time at every site, or the
  purpose fixed at generation time, and neither is acceptable.
- **Live memory with refunds on backtracking.** Return the storage of failed alternatives when the parser truncates its
  stacks. It is tighter, but it charges every failure path, and nodes reachable from memo entries stay live after the
  alternative fails, so the refund would need reachability. Rejected for cost and complexity.
- **Process memory sampling.** Poll `runtime.MemStats` or a memory limit of the Go runtime. It is not deterministic,
  not per parse, and not portable to TypeScript.
- **Purpose presets.** Named configurations such as an editor preset or a server preset. They are convenient, but a
  preset fixes numbers that depend on the caller's hardware and load. The fields are already the selection.

## Testing

- Threshold tests per measure and site. For compute, input and depth, a cap equal to the final counter succeeds, one
  less fails with the right kind before the work, and `Used` and `Requested` are exact. For memory, a smaller cap
  fails within one poll interval.
- Determinism: the same parse twice on a pooled parser, and once on a fresh one, charges the same memory.
- Parity: every backend and generated Go fails the same input at the same kind for compute-independent caps (input,
  depth). Memory, like compute, is per backend.
- Documents: retained bytes count against the cap; a cap below the retained bytes drops history and parses; the result
  equals a fresh parse.
- Streams: a long stream with a small window completes under a cap smaller than its total allocation.
- The no-controls form rejects finite compute and memory; its only change is the counter additions in the refills.

## Performance and Results

All runs: Go 1.27.1, Apple M3 Max, darwin/arm64, `GOMAXPROCS=16`, through the shared benchmark lock, at commit
`a601cd9e` with a prototype applied. Three test binaries ran in interleaved rounds, each benchmark once per binary per
round:

- `base`, the commit itself;
- `off`, the prototype with memory unlimited;
- `on`, the prototype with the largest finite memory cap, `2^64 − 2`.

The benchmarks are:

- `BenchmarkParse` for generated Go: Node parsing in both position units, and typed `ParseAST`, which allocates
  through generated slabs that the prototype does not touch, as a control;
- `BenchmarkIncremental` and `BenchmarkIncrementalLong` (generated Documents);
- `BenchmarkStream` (generated stream);
- `BenchmarkParse` on the closure backend for JSON, XML and Minilang (the engine arm).

All of them run with the default compute budget, so the control is active and every memory site is reached. Ratios
are of medians. The paired range is the smallest and largest per-round ratio.

### Counter form (the design)

Six rounds, each benchmark invocation started only once the one-minute load average was below 6. An earlier attempt
of this run, during which other work loaded the machine to a load average above 50, was discarded.

| Benchmark | Base (ms) | `off` / base | `on` / base | `on` / base, paired range | B/op `on` / base |
|:--|--:|--:|--:|--:|--:|
| `Parse/JSON/generated/codepoints` | 6.03 | 0.994 | 1.002 | 0.994–1.056 | 1.001 |
| `Parse/JSON/generated/bytes` | 6.92 | 0.994 | 0.992 | 0.985–1.066 | 1.000 |
| `Parse/JSON/generated_ast` | 4.23 | 1.010 | 1.020 | 1.008–1.030 | 1.003 |
| `Parse/CSV/generated/codepoints` | 2.79 | 0.995 | 1.013 | 0.922–1.028 | 1.000 |
| `Parse/CSV/generated/bytes` | 3.27 | 0.996 | 1.008 | 0.907–1.078 | 1.000 |
| `Parse/CSV/generated_ast` | 2.49 | 1.004 | 1.026 | 0.973–1.056 | 1.000 |
| `Parse/XML/generated/codepoints` | 7.40 | 1.000 | 1.034 | 1.022–1.075 | 1.000 |
| `Parse/XML/generated/bytes` | 8.13 | 0.999 | 1.026 | 1.016–1.063 | 1.000 |
| `Parse/XML/generated_ast` | 5.55 | 1.003 | 1.057 | 1.053–1.092 | 0.991 |
| `Parse/Arith_OperatorsRule/generated/codepoints` | 7.01 | 1.005 | 1.017 | 1.002–1.044 | 1.000 |
| `Parse/Arith_OperatorsRule/generated/bytes` | 8.21 | 1.008 | 1.014 | 1.001–1.021 | 1.000 |
| `Parse/Arith_OperatorsRule/generated_ast` | 6.63 | 1.003 | 1.008 | 1.003–1.018 | 1.004 |
| `Parse/Arith_LeftRec/generated/codepoints` | 14.95 | 1.003 | 0.994 | 0.975–1.006 | 1.000 |
| `Parse/Arith_LeftRec/generated/bytes` | 15.98 | 0.998 | 0.994 | 0.973–1.010 | 1.000 |
| `Parse/Arith_LeftRec/generated_ast` | 11.76 | 1.006 | 1.010 | 1.007–1.017 | 1.002 |
| `Parse/Minilang/generated/codepoints` | 7.93 | 1.004 | 1.006 | 0.995–1.014 | 1.000 |
| `Parse/Minilang/generated/bytes` | 9.03 | 1.006 | 1.005 | 0.998–1.014 | 1.000 |
| `Parse/Minilang/generated_ast` | 7.52 | 0.998 | 1.007 | 0.998–1.010 | 1.000 |
| `Parse/Recovery/generated/codepoints` | 7.80 | 1.006 | 1.008 | 0.999–1.013 | 1.000 |
| `Parse/Recovery/generated/bytes` | 8.89 | 0.999 | 1.000 | 0.982–1.007 | 0.996 |
| `Parse/Recovery/generated_ast` | 7.45 | 0.993 | 1.002 | 0.942–1.011 | 1.000 |
| `Parse/Outline/generated/codepoints` | 2.54 | 0.987 | 0.996 | 0.973–1.016 | 0.998 |
| `Parse/Outline/generated/bytes` | 3.14 | 1.002 | 0.995 | 0.985–1.014 | 1.000 |
| `Parse/Outline/generated_ast` | 1.85 | 1.005 | 1.014 | 0.999–1.018 | 1.001 |
| `Incremental/generated/document` | 9.29 | 1.003 | 1.008 | 0.996–1.060 | 1.000 |
| `Incremental/generated/fresh` | 8.73 | 1.003 | 1.000 | 0.994–1.014 | 1.000 |
| `IncrementalLong/generated` | 44.43 | 0.970 | 0.979 | 0.948–0.997 | 1.000 |
| `Stream/generated` | 72.56 | 1.019 | 1.022 | 0.996–1.034 | 1.000 |

With memory unlimited (`off`), every ratio is within 0.970 to 1.019, and Node parsing within 0.987 to 1.008, while
the untouched typed control is within 0.993 to 1.010. The counter additions cost nothing measurable.

With a finite cap (`on`), Node parsing takes 0.994 to 1.034 times the base, and XML is the only grammar whose whole
paired range lies above 1 (1.016 to 1.075). The typed control slows as much (XML `ParseAST` 1.057), although it charges
no memory. The cost therefore comes from the 256-unit poll interval that a finite cap imposes, not from counting. A
longer interval for memory alone, such as 4,096 units, would cut that cost. The price is a larger overshoot: up to
4,096 units of allocation, about 512 KiB at the worst site. [Decision 11](#decisions) keeps 256 units. Allocated bytes
per operation do not change in any arm.

The engine was not re-measured in the counter form.

### Call form (first prototype)

The first prototype reserved memory by calling a charge function in each refill branch. That made the runtime
allocators and the engine's `NewNode`, `Nodes` and `fields` lose inlining
([Prototype Charge Sites](#prototype-charge-sites)). Six rounds:

| Benchmark | Base (ms) | `off` / base | `on` / base | `on` / base, paired range | B/op `on` / base |
|:--|--:|--:|--:|--:|--:|
| `Parse/JSON/generated/codepoints` | 6.06 | 1.036 | 1.018 | 0.986–1.041 | 1.001 |
| `Parse/JSON/generated/bytes` | 7.08 | 1.022 | 0.997 | 0.977–1.064 | 1.000 |
| `Parse/JSON/generated_ast` | 4.24 | 1.030 | 1.008 | 0.970–1.061 | 0.997 |
| `Parse/CSV/generated/codepoints` | 2.83 | 1.015 | 0.991 | 0.955–1.068 | 1.000 |
| `Parse/CSV/generated/bytes` | 3.28 | 1.021 | 1.013 | 0.991–1.053 | 1.000 |
| `Parse/CSV/generated_ast` | 2.49 | 1.028 | 1.005 | 0.980–1.049 | 1.000 |
| `Parse/XML/generated/codepoints` | 7.44 | 1.025 | 1.009 | 1.002–1.059 | 1.000 |
| `Parse/XML/generated/bytes` | 8.15 | 1.025 | 1.012 | 0.999–1.057 | 1.000 |
| `Parse/XML/generated_ast` | 5.68 | 0.988 | 0.999 | 0.966–1.046 | 0.981 |
| `Parse/Arith_OperatorsRule/generated/codepoints` | 7.06 | 1.042 | 1.019 | 0.989–1.041 | 1.000 |
| `Parse/Arith_OperatorsRule/generated/bytes` | 8.24 | 1.062 | 1.028 | 0.995–1.055 | 1.000 |
| `Parse/Arith_OperatorsRule/generated_ast` | 6.62 | 1.036 | 1.013 | 0.998–1.057 | 0.999 |
| `Parse/Arith_LeftRec/generated/codepoints` | 15.00 | 1.033 | 1.011 | 1.001–1.064 | 1.000 |
| `Parse/Arith_LeftRec/generated/bytes` | 15.97 | 1.029 | 1.018 | 0.977–1.051 | 1.000 |
| `Parse/Arith_LeftRec/generated_ast` | 11.80 | 1.001 | 1.005 | 0.997–1.032 | 1.000 |
| `Parse/Minilang/generated/codepoints` | 7.97 | 1.034 | 1.027 | 0.990–1.093 | 1.000 |
| `Parse/Minilang/generated/bytes` | 9.28 | 1.010 | 1.006 | 0.981–1.062 | 1.000 |
| `Parse/Minilang/generated_ast` | 7.56 | 1.005 | 1.026 | 0.993–1.035 | 1.001 |
| `Parse/Recovery/generated/codepoints` | 7.86 | 1.015 | 1.062 | 1.012–1.114 | 1.001 |
| `Parse/Recovery/generated/bytes` | 8.92 | 1.030 | 1.075 | 0.995–1.209 | 1.001 |
| `Parse/Recovery/generated_ast` | 7.49 | 1.003 | 1.039 | 0.997–1.119 | 1.002 |
| `Parse/Outline/generated/codepoints` | 2.61 | 0.973 | 1.044 | 0.725–1.753 | 1.000 |
| `Parse/Outline/generated/bytes` | 3.24 | 0.984 | 1.033 | 0.739–1.578 | 1.000 |
| `Parse/Outline/generated_ast` | 1.91 | 1.000 | 1.022 | 0.806–1.309 | 0.997 |
| `Incremental/generated/document` | 9.83 | 0.995 | 1.003 | 0.849–1.018 | 1.000 |
| `Incremental/generated/fresh` | 9.00 | 1.004 | 1.014 | 0.897–1.064 | 1.000 |
| `IncrementalLong/generated` | 44.99 | 0.999 | 1.001 | 0.946–1.038 | 1.000 |
| `Stream/generated` | 74.47 | 0.990 | 0.986 | 0.982–1.009 | 1.000 |
| `Parse/JSON/closure/codepoints` | 16.43 | 1.000 | 1.006 | 0.996–1.035 | 1.000 |
| `Parse/JSON/closure/bytes` | 16.10 | 1.000 | 1.003 | 0.968–1.026 | 1.000 |
| `Parse/XML/closure/codepoints` | 17.01 | 1.007 | 1.016 | 0.974–1.034 | 1.000 |
| `Parse/XML/closure/bytes` | 16.72 | 1.004 | 1.015 | 0.965–1.040 | 1.000 |
| `Parse/Minilang/closure/codepoints` | 17.51 | 0.993 | 0.992 | 0.928–1.000 | 1.000 |
| `Parse/Minilang/closure/bytes` | 17.79 | 1.004 | 0.998 | 0.934–1.021 | 1.000 |

The untouched typed control moved by 0.988 to 1.036 (`off`), so differences of a few percent are within this run's
noise. Node parsing moved by 0.973 to 1.062 with memory unlimited. A focused ten-round rerun of the two grammars with
the largest `on` ratios gave:

| Benchmark | Base (ms) | `off` / base | `on` / base | `on` / base, paired range | B/op `on` / base |
|:--|--:|--:|--:|--:|--:|
| `Parse/Arith_OperatorsRule/generated/codepoints` | 6.97 | 1.009 | 1.007 | 0.936–1.017 | 1.000 |
| `Parse/Arith_OperatorsRule/generated/bytes` | 8.14 | 1.018 | 1.019 | 0.926–1.028 | 1.000 |
| `Parse/Arith_OperatorsRule/generated_ast` | 6.56 | 1.000 | 1.000 | 0.971–1.031 | 1.000 |
| `Parse/Recovery/generated/codepoints` | 7.80 | 1.013 | 1.017 | 0.979–1.323 | 1.000 |
| `Parse/Recovery/generated/bytes` | 8.83 | 1.018 | 1.021 | 1.006–1.072 | 1.000 |
| `Parse/Recovery/generated_ast` | 7.44 | 0.999 | 0.999 | 0.989–1.021 | 1.000 |

There, with the control at 0.999 to 1.000, Node parsing took 1.009 to 1.021 times the base, in both `off` and `on`.
The cost was the same whether memory was counted or not, so it came from the lost inlining, not from the charge. That
result led to the counter form. The closure-backend arm stayed within 0.992 to 1.016 even with lost inlining.

### Charged memory and compute

Charged memory against compute spent, for the generated Node parsers on the benchmark inputs, from the counter-form
prototype's counters at the end of a Bytes parse. Charged / allocated compares the charged bytes with the bytes a
first `CodePoints` parse in the process allocated (`runtime.MemStats.TotalAlloc`). The difference is mostly the sites
the prototype leaves out, in particular the decoded input, at 8 bytes per input byte.

| Grammar | Input (bytes) | Charged memory (bytes) | Compute (units) | Bytes per unit | Units per input byte | Charged / allocated |
|:--|--:|--:|--:|--:|--:|--:|
| JSON | 262,220 | 8,693,392 | 1,385,461 | 6.3 | 5.3 | 0.64 |
| CSV | 211,425 | 5,332,040 | 740,368 | 7.2 | 3.5 | 0.70 |
| XML | 262,163 | 11,187,536 | 3,154,851 | 3.5 | 12.0 | 0.49 |
| Arith_OperatorsRule | 133,457 | 10,528,768 | 1,497,904 | 7.0 | 11.2 | 0.84 |
| Arith_LeftRec | 133,457 | 24,023,040 | 3,525,609 | 6.8 | 26.4 | 0.65 |
| Minilang | 88,888 | 9,687,744 | 1,902,506 | 5.1 | 21.4 | 0.60 |
| Recovery | 89,404 | 9,212,608 | 1,925,968 | 4.8 | 21.5 | 0.58 |
| Outline | 83,368 | 3,588,672 | 577,465 | 6.2 | 6.9 | 0.68 |

## Decisions

1. **The configuration keeps the name `Limits`, with one field per measure.** `Limits{Compute, Memory, Input, Depth}`,
   `WithLimits`, `LimitError`, and the kinds `LimitCompute`, `LimitMemory`, `LimitInput` and `LimitDepth`. A limit is
   the general concept and each field is named after what it measures. Input and depth are caps, not budgets, so
   `Budget{...}` with `WithBudget` was rejected. Keeping `Budget` and `MaxInput` and adding fields was rejected
   because `Budget` is ambiguous once there are two budgets.
2. **The work lands in stages.** First the rename and the move of depth into `Limits`. Next, memory in the engine and
   generated Go. TypeScript memory comes after its per-object charge is measured. The API changes once and each stage
   is verified on its own. Shipping every measure on every backend at once would block on the unmeasured TypeScript
   cost. Shipping only the rename would keep the compute-only bound, which is 20 times loose and cannot bound a
   Document.
3. **Memory counts bytes allocated minus bytes released at points the parser controls.** It bounds what the parser
   holds at every point, costs nothing on hot paths, and bounds a stream's window. Counting allocation without returns
   was rejected: it would make a cap also cap a stream's length. Live bytes with refunds on backtracking were rejected:
   they charge every failure path and need reachability for memoized nodes.
4. **`Memory` defaults to `Unlimited`.** Compute already bounds memory for every budgeted parse, and a second default
   would add a failure mode no caller asked for. A multiple of the input was rejected because any factor is a guess
   that fails for grammars that build large values. A default derived from compute was rejected because it adds no
   guarantee.
5. **A Document's memory counter starts at the bytes it retains.** If they already exceed the cap, the Document drops
   its history and parses from scratch, and fails only if that parse exceeds the cap. The cap bounds the footprint,
   which is what an editor needs, and a Document never becomes permanently failing. A per-`Parse` cap on new
   allocation was rejected because it does not bound the footprint. Failing without dropping the history was rejected
   because the Document would stay failing until the caller reset it.
6. **`Depth: Unlimited` is accepted only on the iterative VM.** That is the only backend whose call stack is on the
   heap, where memory bounds it. On other backends it is an option error, because a Go stack overflow cannot be
   recovered. This is the one exception to the rule that `Unlimited` disables every field. Accepting it everywhere
   would let one input crash the process. Mapping it to the backend's largest safe depth would make `Unlimited` mean
   something other than unlimited.
7. **One generation flag, `zp gen -limits`, compiles in every measure.** It replaces `-budget-controls`. Memory has no
   hot-path cost, so selecting measures at generation time, as optimization profiles are selected, would add
   generated forms to test without benefit. That alternative is reconsidered if a future measure has a hot-path cost.
8. **TypeScript takes a nested `limits: { compute, memory, input, depth }` object.** The kinds are
   `"compute-limit"`, `"memory-limit"`, `"input-limit"` and `"depth-limit"`. This mirrors Go and avoids a flat `input`
   option, which would read as the parse input.
9. **The command line uses `-max-compute`, `-max-memory`, `-max-input` and `-max-depth`.** One prefix for all four.
   Mixed names and a single compound `-limit` flag were rejected as harder to read and to document.
10. **There are no presets by purpose.** The fields are the selection. A preset would fix numbers that depend on the
    caller's hardware and load.
11. **Memory is counted by an addition and checked at the compute poll.** A finite cap sets the poll interval to 256
    units. Counting is free with memory unlimited, and the allocators stay inlined. A finite cap costs up to 3 % on
    Node parsing and 6 % on typed parsing because of the poll. The counter can pass the cap by one poll interval's
    allocation. A 4,096-unit interval for memory alone is reconsidered if a workload shows that cost. Reserving before
    each allocation was rejected because the call breaks inlining.

## Limitations and Open Questions

- Memory is an estimate from the sizes of the parser's own types. It excludes the Go allocator's size classes and
  headers and any storage the collector has not yet freed, so process memory can exceed it by the allocator's
  overhead and by garbage awaiting collection.
- Memory charged by chunk can exceed the bytes in use by up to one chunk per slab. The cap is enforced at polls, so the
  counter can pass it by what one poll interval allocates.
- The counts differ between backends and position units, as compute counts do.
- TypeScript memory charging is not measured.
- The engine arm charges node, pointer and field slabs only, in the first (call) form. The counter form, its memo slab
  and its frame chunks are not measured there.
