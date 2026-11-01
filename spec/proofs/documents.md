# Documents

Arata Furukawa · Last revised 2026-11-01

This proof shows that a parse of a [Document](../runtime/documents.md) after
any sequence of edits returns what a parse of the current text from scratch
returns ([Reuse](../runtime/documents.md#reuse)). A Document reuses two kinds
of results: memo entries, which it keeps, shifts or drops at each edit, and the
recorded runs of repetitions, which it resumes. The proof extends the induction
of the [memoization proof](memoization.md) to both: the memo entries are sound
for the current text, and a resumed run records what evaluating its iterations
records.

## Setting

A Document has a text $t_k$ after $k$ edits. Edit $k$ replaces the positions
$[a_k, b_k)$ of $t_{k-1}$ with text of length $b_k - a_k + \delta_k$, so a
position $i \ge b_k$ of $t_{k-1}$ becomes $i + \delta_k$ in $t_k$. A batch is
one edit in this sense for the text, and it clears all reuse state
([Edits](../runtime/documents.md#edits)). Keys, lookahead states, outcomes,
growths, the parses $\mathcal{R}$ and $\mathcal{M}$, soundness and the
scope and assumptions are as in the [memoization proof](memoization.md#setting),
on the text at hand. A Document does not support `#missing`
([Construction and Ownership](../runtime/documents.md#construction-and-ownership)).

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
it, examined. For an evaluation at $p$, $l \le p$. Every failure that the
evaluation records is at an examined slot, so its farthest failure $f$
satisfies $l \le f < h$ whenever it recorded one. A Document may keep a larger
interval in place of $[l, h)$, such as the conservative `Examined` frontier of
[Generated Go Document Tracing and Profiling](../runtime/observability.md#generated-go-document-tracing-and-profiling).
A larger interval only makes the conditions below drop more entries and resume
fewer iterations, and the proofs hold for it unchanged, since they use the
recorded range only through the invariant (I2) of the memoization proof, that
it contains the range in $\mathcal{R}$.

**Position-free evaluations.** An evaluation is **position-free** when no
action or predicate in it, or in a call within it, reads a position (the
`.Start` and `.End` members), and its outcome has no recovered errors, whose
lines and columns depend on positions. Positions still occur in the outcome of
a position-free evaluation as its end, the ranges of its nodes and its farthest
failure. **Shifting** an outcome by $\delta$ adds $\delta$ to each of these.

**Reuse state.** The proof assumes that the reuse state of a Document has the
following properties. The [Incremental Documents](../runtime/documents.md)
chapter states the guarantee and not this mechanism.

- **(E) Edits.** At each edit $[a, b)$ with $\delta$, a memo entry with recorded
  range $[l, h)$ is kept unchanged if $h \le a$; it is shifted by $\delta$,
  together with its key position and its range, if $l \ge b$ and its call was
  position-free; otherwise it is dropped. Edits are applied to an entry in
  order, either at once or when it is next looked up.
- **(P) Parses.** Entries are stored under the conditions of $\mathcal{M}$. A
  parse that does not complete, because it is interrupted
  ([Interruption](../runtime/documents.md#interruption)) or aborted by a
  run-time error of an action, the nesting limit or another fatal error,
  clears the memo entries and the runs, and publishes no partial table.
- **(R) Runs.** A **run record** of a repetition at $p$ holds, for each
  iteration that matched, its start, end, recorded range, value, and farthest
  failure with its expected items.
  - *Recording.* A run is recorded for a repetition that matched outside a
    lookahead and a `skip`. Its element has no predicate, calls no rule that
    reads variables, and writes no capture to an enclosing scope. No
    recovered error remains at the end of the run. During the run no call
    read the current result of a growth, directly or through an entry stored
    by a call that did, even of a growth that started and ended within the
    run. This condition is taken from the intended mechanism: the reference
    implementation refuses a run whose calls read the current result of a
    growth open at its start, but does not yet refuse one that read the
    current result of a growth that started and ended within the run.
  - *Resuming.* A repetition resumes a record only outside a lookahead and a
    `skip`. It resumes in one of two ways. **Whole**: the record of the same
    repetition at the same position, stored earlier in the same parse, or by
    an earlier parse of the same text with no edit since; it reuses every
    recorded iteration. **Across one
    edit**: the record of the previous parse when exactly one edit $[a, b)$
    with $\delta$ came since, stored at $p$, or at $p - \delta$ when
    $p \ge b + \delta$. It reuses the recorded iterations from the first on
    while their range ends at or before $a$, whatever $\delta$ is. It then
    evaluates iterations, possibly none. When the current position is
    $r_j + \delta$ for a recorded iteration $j$ such that every recorded
    iteration from $j$ on examined only slots at or after $b$, it reuses
    those iterations shifted by $\delta$, which requires $\delta = 0$ or
    position-free iterations. After the reused iterations it goes on as a
    repetition does.

## Lemma 1 (Locality)

Let an evaluation in $\mathcal{R}$ of an expression at $p$ on a text $t$, in a
state that reads no growth open at its start, have the outcome $\omega$ and
the examined range $[l, h)$. Let $t'$ be a text and $\delta$ an offset such
that slot $i + \delta$ of $t'$ holds what slot $i$ of $t$ holds for every
$i \in [l, h)$, and let either $\delta = 0$ or the evaluation be
position-free. When $\omega$ has recovered errors, let also $\delta = 0$ and
$t'$ agree with $t$ on $[0, h)$. Then the evaluation of the same expression at
$p + \delta$ on $t'$, in the same variables, captures, level and lookahead
state, has the outcome $\omega$ shifted by $\delta$ and the examined range
$[l + \delta, h + \delta)$.

*Proof.* By induction on the steps of the evaluation on $t$, matched one to
one with those on $t'$. Each step that reads the input examines a slot $i$ in
$[l, h)$, and reads at $i + \delta$ on $t'$ the same character, the same end
of the text, or the same absence of a preceding character, so it decides the
same way and moves the position by the same amount. The other steps depend on
the input only through earlier steps, and on positions only in these ways:

- as the ends and ranges that the outcome holds, which shift with the
  evaluation;
- as positions that actions and predicates read, which a position-free
  evaluation does not have, and which are equal when $\delta = 0$;
- as the lines and columns of recovered errors, which depend on the text before
  the error. An error is at a recorded failure, before $h$, and $t'$ agrees
  with $t$ on $[0, h)$ when there are such errors.

Calls within the evaluation have their keys shifted by $\delta$ and, by the
induction hypothesis, their outcomes shifted by $\delta$. By
[Lemma 1 of the memoization proof](memoization.md#lemma-1-context) the
calls depend on nothing else. $\square$

The condition on $[0, h)$ is needed. Let $t$ be the four characters `x`, a
line feed, `a` and `b`. Replacing the line feed, $[1, 2)$, with `y` leaves the
slots $[2, 4)$ in place, but an error recovered at position $3$ moves from
line 2, column 2, to line 1, column 4.

## Lemma 2 (Carried Entries)

Let every entry be sound for $t_{k-1}$. After edit $k$ applied by (E), every
entry that remains is sound for $t_k$.

*Proof.* Let an entry have key position $p$, outcome $\omega$ and recorded
range $[l', h')$, which by soundness contains the range $[l, h)$ of the call
in $\mathcal{R}$ on $t_{k-1}$. Let edit $k$ be $[a, b)$ with $\delta$.

- If $h' \le a$, then $h \le a$, and every position before $a$ holds in $t_k$
  what it holds in $t_{k-1}$: the characters before $a$ are unchanged, and
  slot $n_{k-1}$ is not in $[0, h)$, since $h \le a \le n_{k-1}$. By Lemma 1
  with $\delta = 0$, which allows recovered errors here, the entry is sound
  for $t_k$.
- If $l' \ge b$ and the call was position-free, then $l \ge b$, and slot $i$
  of $t_{k-1}$ is slot $i + \delta$ of $t_k$ for every $i \ge b$, the end of
  the text included. By Lemma 1 the shifted entry is sound for $t_k$ at
  $p + \delta$, and its shifted range contains the shifted range of the call.
- Otherwise (E) drops the entry.

Applying the edits when an entry is looked up applies the same rule in the
same order, so it gives the same entry. $\square$

## Lemma 3 (Resumed Runs)

In a parse of $t_k$, let a repetition at $p$ resume a record, and let every
evaluation that completed before it in this parse satisfy the invariant
(I1) to (I3) of the
[memoization theorem](memoization.md#theorem-memoization-never-changes-the-result).
Let the record have been stored in this parse, or in an earlier parse whose
completed evaluations satisfied the invariant on its text. Then the repetition
ends where it ends in $\mathcal{R}$, with the same value, and records into the
enclosing evaluation the outcome that the iterations record in $\mathcal{R}$
and a range that contains theirs.

*Proof.* When the record was stored, its iterations were evaluations of
$\mathcal{M}$ that completed and recovered no error, and by (R) none of them
read a current result, directly or through an entry. By (I1) and (I2) of the
parse that stored it, each recorded outcome is the outcome in $\mathcal{R}$ on
that text, each recorded range contains the range in $\mathcal{R}$, and the
evaluation in $\mathcal{R}$ reads no current result either, since a call that
$\mathcal{M}$ answered from an entry is in $\mathcal{R}$ the evaluation that
stored it. By Lemma 4 below, each reused iteration reads no growth open at its
start in $\mathcal{R}$ when the run resumes. The element reads
no variable and no capture from outside, its cut decides only between its own
alternatives and whether the repetition fails, and its level does not change
within the repetition, so by Lemma 1 of the memoization proof its outcome
$G(r)$ in $\mathcal{R}$ is a function of its start $r$ and the text.

So each recorded outcome is $G(r_i)$ on the text on which it was recorded.
Let the recorded iterations start at $r_0 < r_1 < \dots < r_m$, each ending
where the next starts.

- *Whole.* The text is the one the record was stored on, so the recorded
  outcomes are $G(r_i)$ on $t_k$. The run reuses all of them in order, each
  starting where the one before ended, as $\mathcal{R}$ evaluates them.
- *Across one edit.* The run is made of four parts.
  1. *Iterations before the edit.* The recorded iterations from the first on
     whose range ends at or before $a$. By Lemma 1 with $\delta = 0$, each has
     on $t_k$ its recorded outcome, and it starts where the one before ended.
     $\delta$ does not enter: these iterations are not moved.
  2. *New iterations.* Evaluations of $\mathcal{M}$, which satisfy (I1) and
     (I2) because they complete before the repetition does. There may be none,
     as after a deletion that ends where an iteration began.
  3. *Iterations after the edit.* When the current position is
     $r_j + \delta$ and every recorded iteration from $j$ on examined only
     slots at or after $b$, the run reuses iterations $j, j + 1, \dots$
     shifted by $\delta$. By Lemma 1, with $\delta = 0$ or position-free
     iterations, each is $G$ at its shifted start on $t_k$, and each starts
     where the one before ended. A record that the edit moved as a whole is
     the case $j = 0$ with no iterations in the first part.
  4. *The end.* After the reused iterations the run goes on as a repetition
     does. It stops after an iteration that consumed nothing or at its upper
     bound, counting the iterations of all parts, or evaluates the next
     iteration, as $\mathcal{R}$ does at that position.

In each case the run visits the iterations that $\mathcal{R}$ visits, in the
same order and with the same outcomes. An iteration that fails is not
recorded, so the iteration that ends the repetition in $\mathcal{R}$ is
evaluated in either case. A reused iteration records its farthest failure,
expected items and range into the enclosing evaluation as evaluating it does,
and the operation $\cdot$ and the hull of
[Skip Runs](skip-runs.md#setting) are associative, so the run leaves the
enclosing evaluation with the same record. $\square$

## Lemma 4 (Recorded Iterations Read No Open Growth)

Let an iteration $I$ of a record that (R) allows to be reused be evaluated in
$\mathcal{R}$ at its start $r$ when the run resumes. Then it reads no growth
open at its start.

*Proof.* Call $A$ the evaluation of $I$ in $\mathcal{R}$ on the text on which
it was recorded, which reads no current result at all, as the proof of Lemma 3
shows, and $B$ its evaluation when the run resumes. Every slot that $A$
examines lies in the recorded range of $I$, where the two texts agree, shifted
for an iteration after the edit, as in the cases of Lemma 3. Two segments of
evaluation that start with the same expression at corresponding positions in
the same state therefore take the same steps, examining corresponding slots,
until one of them is answered with a current result.

Suppose that $B$ reads a growth open at its start. As in
[Lemma 2 of the memoization proof](memoization.md#lemma-2-reading-an-open-growth),
that growth is at $r$. Let $G_1, \dots, G_m$ be the growths open at $r$ when
$I$ starts, of leaders $L_1, \dots, L_m$, in the order in which they started.

1. Compare $B$ with $A$ from their start. $A$ is never answered with a current
   result, so the first difference is a call $x$ of $B$ that is. A growth that
   started within $I$ before $x$ started in $A$ too, where reading it would be
   a difference in $A$, so $x$ reads some $G_j$, and is a call of $L_j$ at
   $r$. In $A$, the corresponding call of $L_j$ at $r$ finds no growth of
   $L_j$ open, so $A$ grows $L_j$ there, and its first step evaluates the body
   of $L_j$ at $r$ with a failing seed.
2. In $B$, $I$ and $x$ lie within the current step of $G_j$, which evaluated
   the body of $L_j$ at $r$ and reached $x$ through calls that consume no
   input. Compare that segment of $B$ with the first step of $L_j$ in $A$. The
   variables do not enter, since no rule that $I$ calls reads variables. If $B$
   is not answered with a current result before $x$, then $A$ reaches the
   corresponding call of $L_j$ at $r$ while it grows $L_j$ there, and reads
   its current result, which $A$ never does. Otherwise let $z$ be the first
   call of the segment of $B$ that is. It cannot read a growth that started
   within the segment, nor $G_j$, because $A$ would read the corresponding
   growth, which is open in $A$ too. So $z$ reads some $G_i$ with $i < j$, and
   in $A$ the corresponding call of $L_i$ at $r$ grows $L_i$.
3. Repeat step 2 with $i$ and $z$ in place of $j$ and $x$. The index decreases
   each time, and no growth at $r$ started before $G_1$, so the repetition
   ends in the contradiction of step 2.

So $B$ reads no growth open at its start. $\square$

The argument uses the recording condition of (R) for the growths that start
and end within the run: a run that grew a leader within its first iteration
and read that leader's current result is not recorded, and that is the case
that reusing it at the position of an open growth would get wrong.

## Theorem (Document Parses Equal Fresh Parses)

*A parse of a Document after any sequence of edits, batches, parses,
interrupted parses and aborted parses returns the value, the recovered errors
and the syntax error that a parse of its current text from scratch returns,
when no limit stops it.*

*Proof.* By induction on the parses of the Document. Every entry and record
that a parse can use was left by earlier completed parses, with the edits since
applied, or stored earlier in the same parse: a batch, an interrupted parse
and an aborted parse clear them by (P). By the induction hypothesis, every
completed parse satisfied the invariant of the
[memoization theorem](memoization.md#theorem-memoization-never-changes-the-result),
so the entries it left were sound for its text, and by Lemma 2, applied to each
edit in turn, including consecutive edits with no parse between them, the
entries are sound for the current text when the parse starts.

The parse then follows the induction of the memoization theorem, with one more
kind of evaluation: a repetition that resumes a record, and so skips the
evaluation of the iterations that it reuses. By Lemma 3, whose assumptions are
the induction hypothesis, it satisfies (I1) and (I2), and it stores no entry.
So the invariant holds for every evaluation of the parse, and the parse
returns what $\mathcal{R}$ returns on the current text. A parse from scratch is
a memoized parse from an empty table, so it returns the same. $\square$

The statistics of a parse, the identity of nodes and whether nodes returned
earlier are moved in place are not part of the outcome
([Parsing](../runtime/documents.md#parsing)).
