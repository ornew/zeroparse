# R180: Skip-Run Records

The walks of unbounded repetitions in `#recover` skip expressions share the rest of their walks, so skips that
start at many positions of one input walk each position a bounded number of times.

## Mechanism and Invariants

A skip-run site is an unbounded repetition in a skip expression whose rest of the walk from a position depends
only on the position and the input: no predicate that reads or binds a variable, no `#cut` that applies to the
repetition, and no called rule that reads a variable, contains `#recover` or belongs to a left-recursion cycle.
Per site and lookahead state, each position has a slot that is fresh, visited or recorded: a bit marks the slots
that are not fresh, and a map holds the records. A walk marks the position of each iteration it moves past until it
reaches one that is not fresh, and records from there on, so a walk that fails at once touches no slot. When a recording walk ends,
it stores the rest of the walk (end, farthest failure and expectations, examined range) for each position it
visited while recording, backward from the last. A recording walk that has matched its minimum reuses a recorded
slot as a memo hit. Records are stored only when a walk ends, so a stopped parse leaves none. Each reused record
equals the rest of the walk it replaces, and each position is evaluated at most twice, plus the minimum and one
more per walk
([proof](../../../spec/proofs/skip-runs.md)).

## Scope and State

All six backends, Documents and streams. Slots last for one parse; streams drop those before a commit. A parse
with missing-token insertion keeps no slots. Repetitions in rules that skips call are not sites.

## Reference Control

None maintained. Tests compare against parses with records disabled (`core.SkipRecordsOff`) and mutate the
runtimes to never record or to store records early.

## Evidence and Implementation

Evidence record: [132](../132-records-of-recovery-skip-walks.md).

Source owners: [the site analysis](../../../internal/compile/skip_sites.go), [the engine
operations](../../../internal/engine/core/skip_runs.go), [the generated Go runtime](../../../runtime/skip_runs.go),
[the Flow IR operations](../../../internal/ir/flow/flow.go) and [their lowering](../../../internal/ir/flow/lower/lower.go),
[the TypeScript runtime](../../../internal/gen/tsgen/tsrt/runtime.ts).

Related mechanisms: [R170](R170-left-recursion-member-memo.md).

[Optimization Catalog](../README.md)
