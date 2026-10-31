# Documents

This proof shows that a parse of a [Document](../runtime/documents.md) after
any sequence of edits returns what a parse of the current text from scratch
returns ([Reuse](../runtime/documents.md#reuse)). A Document reuses two kinds
of results of earlier parses: memo entries, which it keeps, shifts or drops at
each edit, and the recorded runs of repetitions, which it resumes. The proof
reduces both to the [memoization proof](memoization.md): the memo entries are
sound for the current text, and a resumed run records what evaluating its
iterations records.

## Setting

A Document has a text $t_k$ after $k$ edits. Edit $k$ replaces the positions
$[a_k, b_k)$ of $t_{k-1}$ with text of length $b_k - a_k + \delta_k$, so a
position $i \ge b_k$ of $t_{k-1}$ becomes $i + \delta_k$ in $t_k$. A batch is
one edit in this sense for the text, and it clears all reuse state
([Edits](../runtime/documents.md#edits)). Keys, lookahead states, outcomes,
growths, the reference parse $\mathcal{R}$ and soundness are as in the
[memoization proof](memoization.md#setting), on the text at hand.

**Slots.** Slot $i$ of a text $t$ of length $n$ is the character at position
$i$ when $i < n$, and the end of the text when $i = n$. A terminal, a character
class, `.`, an anchor or the decoding of a character **examines** a slot when
its result depends on what the slot holds: matching a character examines its
slot, rejecting one examines it too, and testing for the end of the input at
$i$ examines slot $i$. Decoding the character at position $i$ in bytes examines
every byte that it reads, and reads no byte before $i$. Testing for the
beginning of a line or of the input at $i$ examines slot $i - 1$, or the
absence of one at $i = 0$, written slot $-1$.

**Examined range.** The examined range of an evaluation is the interval
$[l, h)$ of the least and one past the greatest slot that it, or a call within
it, examined. For a call at $p$, $l \le p$. A Document may keep a larger
interval in place of $[l, h)$, such as the conservative `Examined` frontier of
[Generated Go Document Tracing and Profiling](../runtime/observability.md#generated-go-document-tracing-and-profiling):
a larger interval only makes the conditions below drop more entries and resume
fewer iterations, and the proofs hold for it unchanged. Every failure that the
evaluation records is at an examined slot, so its farthest failure $f$
satisfies $l \le f < h$ whenever it recorded one.

**Position-free outcomes.** An outcome is **position-free** when its value
holds no position value (the `.Start` and `.End` members and what is computed
from them in actions) and it has no recovered errors, whose lines and columns
depend on positions. Positions still occur in a position-free outcome as its
end, the ranges of its nodes and its farthest failure. **Shifting** an outcome
by $\delta$ adds $\delta$ to each of these.

**Reuse state.** The proof assumes that the reuse state of a Document has the
following properties.

- **(E) Edits.** At each edit $[a, b)$ with $\delta$, a memo entry with
  examined range $[l, h)$ is kept unchanged if $h \le a$; it is shifted by
  $\delta$, together with its key position and its range, if $l \ge b$ and
  its outcome is position-free; otherwise it is dropped. Edits are applied to
  an entry in order, either at once or when it is next looked up.
- **(P) Parses.** An entry is stored only under condition (S) of the
  memoization proof, so it read no growth open when its call started. A parse
  that is interrupted clears the memo entries and the runs, and publishes no
  partial table ([Interruption](../runtime/documents.md#interruption)).
- **(R) Runs.** A run is recorded for a repetition that matched outside a
  lookahead and a `skip`, in a parse in which it recovered no error and read no
  growth open when it started. Its element has no predicate, calls no rule
  that reads variables, and writes no capture to an enclosing scope. The record
  holds, for each iteration that matched, its start, end, examined range,
  value, and farthest failure with its expected items. A run whose values can
  hold position values is resumed only after an edit with $\delta = 0$.

## Lemma 1 (Locality)

*Let an evaluation in $\mathcal{R}$ of an expression at $p$ on a text $t$, in a
state that reads no growth open at its start, have the outcome $\omega$ and
the examined range $[l, h)$. Let $t'$ be a text and $\delta$ an offset such
that slot $i + \delta$ of $t'$ holds what slot $i$ of $t$ holds for every
$i \in [l, h)$, and let $\delta = 0$ or $\omega$ be position-free. Then the
evaluation of the same expression at $p + \delta$ on $t'$, in the same
variables, captures, level and lookahead state, has the outcome $\omega$
shifted by $\delta$ and the examined range $[l + \delta, h + \delta)$.*

*Proof.* By induction on the steps of the evaluation on $t$, matched one to
one with those on $t'$. Each step that reads the input examines a slot $i$ in
$[l, h)$, and reads at $i + \delta$ on $t'$ the same character, the same end
of the text, or the same absence of a preceding character, so it decides the
same way and moves the position by the same amount. The other steps depend on
the input only through earlier steps, and on positions only as follows: as
the ends and ranges that the outcome holds, which shift with the evaluation;
as position values, which the outcome holds only when $\delta = 0$; and as
the lines and columns of recovered errors, which it holds only when
$\delta = 0$. Calls within the evaluation have the keys shifted by $\delta$
and, by the induction hypothesis, their outcomes shifted by $\delta$. By
Lemma 1 of the [memoization proof](memoization.md#lemma-1-context) the
calls depend on nothing else. $\square$

## Lemma 2 (Carried Entries)

*At the start of each parse of a Document, every memo entry that it holds,
with the edits since it was stored applied by (E), is sound for the current
text.*

*Proof.* By induction on parses and edits. An entry stored in a parse of
$t_j$ is sound for $t_j$ by the [memoization theorem](memoization.md#theorem-memoization-never-changes-the-result),
since by (P) it satisfies (S); an interrupted parse leaves no entry. Let an
entry be sound for $t_{k-1}$, with key position $p$, outcome $\omega$ and
examined range $[l, h)$, and let edit $k$ be $[a, b)$ with $\delta$.

- If $h \le a$, every slot in $[l, h)$ is before the replaced positions, so
  $t_k$ holds what $t_{k-1}$ holds there: for $i < n_{k-1}$ the character is
  unchanged, and slot $n_{k-1}$ is not in the range, since
  $h \le a \le n_{k-1}$. By Lemma 1 with $\delta = 0$ the entry is sound for
  $t_k$.
- If $l \ge b$ and $\omega$ is position-free, slot $i$ of $t_{k-1}$ is slot
  $i + \delta$ of $t_k$ for every $i \ge b$, the end of the text included. By
  Lemma 1 the shifted entry is sound for $t_k$ at $p + \delta$.
- Otherwise (E) drops the entry.

Applying the edits when an entry is looked up applies the same rule in the
same order, so it gives the same entry. $\square$

## Lemma 3 (Resumed Runs)

*In a parse of $t_k$, let a repetition at $p$ resume the run that (R) recorded
in the parse of $t_{k-1}$ at $p$, or at $p - \delta_k$ when $p \ge b_k + \delta_k$.
Then it ends where evaluating its iterations ends, with the same value, and
records the same outcome and examined range into the enclosing evaluation.*

*Proof.* Let the recorded iterations start at $r_0 < r_1 < \dots < r_m$, each
ending where the next starts. By (R), an iteration is an evaluation of the
element at its start that reads no variables, writes no capture outside
itself and reads no open growth, so by Lemma 1 of the memoization proof its
outcome is a function of its start and the text. The resumed run is made of
four parts.

1. *Iterations before the edit.* The run reuses recorded iterations
   $0, 1, \dots$ while their examined range ends at or before $a_k$. By
   Lemma 1 with $\delta = 0$, each has on $t_k$ the outcome it had, and it
   starts where the one before ended, as in the evaluation.
2. *New iterations.* From the first iteration it cannot reuse on, the run
   evaluates iterations, which have their outcomes in $\mathcal{R}$ by the
   memoization theorem.
3. *Iterations after the edit.* When an evaluated iteration ends at
   $r_j + \delta_k$ for a recorded iteration $j$ such that every recorded
   iteration from $j$ on examined only slots at or after $b_k$, the run reuses
   iterations $j, j+1, \dots$ shifted by $\delta_k$. By Lemma 1 each is the
   outcome of evaluating the element at its shifted start, and each starts
   where the one before ended. A run that the edit moved as a whole is the
   case $j = 0$.
4. *The end.* After the reused iterations the run goes on as a repetition
   does. It stops after an iteration that consumed nothing or at its upper
   bound, counting the iterations of all four parts, or evaluates the next
   iteration, as the evaluation would at that position.

In each part the run visits the iterations that the evaluation visits, in the
same order and with the same outcomes. A reused iteration records its farthest
failure and expected items, and its examined range, into the enclosing
evaluation, as evaluating it does, and the operation $\cdot$ and the hull of
[Skip Runs](skip-runs.md#setting) are associative, so the run leaves the
enclosing evaluation with the same record. An iteration that failed was not
recorded, so it is evaluated in either case. $\square$

## Theorem (Document Parses Equal Fresh Parses)

*A parse of a Document after any sequence of edits, batches, parses and
interrupted parses returns the value, the recovered errors and the syntax
error that a parse of its current text from scratch returns.*

*Proof.* By Lemma 2 the parse starts with a table of sound entries. A resumed
run is, by Lemma 3, a step whose outcome is the one that evaluating it has,
so it can be added to the cases of the
[memoization theorem](memoization.md#theorem-memoization-never-changes-the-result)
as a combinator. With those, the theorem applies to the Document parse and
gives the outcomes of the reference parse of the current text. A parse from
scratch is also a memoized parse, from an empty table, so it gives the same
outcomes. A batch and an interruption clear the reuse state, which leaves the
table empty and the conclusion unchanged. $\square$

The statistics of a parse, the identity of nodes and whether nodes returned
earlier are moved in place are not part of the outcome
([Parsing](../runtime/documents.md#parsing)).
