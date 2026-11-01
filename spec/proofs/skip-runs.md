# Skip Runs

This proof covers the records of [skip runs](../runtime/parsing.md#skip-runs):
that a reused record gives what the rest of the walk would have given
(Lemma 1), and that the walks of a site in a parse evaluate its element a
number of times linear in the input (Lemma 2). Sites, walks, slots and records
are as defined there.

## Setting

Fix a parse, a site $\sigma$ and a lookahead state $\lambda$ (inside or outside
a lookahead), and let $n$ be the length of the input. A walk of $\sigma$ in
state $\lambda$ evaluates the element $e$ of $\sigma$ at positions
$r_0 < r_1 < \dots$, one iteration each.

**Iterations.** The evaluation of $e$ at $r$, written $E(r)$, either fails or
matches up to a position $r' \ge r$. Either way it has a farthest failure
$f(r)$ with expectations $X(r)$, and it examines an input range $[l(r), h(r)]$.
By the conditions on sites, $E(r)$ depends only on $r$, $\lambda$ and the
input:

- $e$ has no predicate that reads or binds a variable, and no rule it calls
  reads a variable, so no environment enters;
- a skip expression has no captures, and $e$ builds no value;
- no rule $e$ calls contains `#recover`, so $E(r)$ appends no recovered error;
- no rule $e$ calls belongs to a left-recursion cycle, so no growth in progress
  outside $e$ changes what a call inside it returns;
- $e$ has no `#cut` that applies to the repetition, so a `#cut` inside $e$
  only decides between alternatives of $e$, and the failure of $E(r)$ ends the
  walk as any failure does.

Memoized results that $E(r)$ reads or stores change its cost, never its
outcome.

**Outcomes.** Let $O$ be the set of pairs $(f, X)$ of a farthest failure and a
sequence of expectations without duplicates, and define on $O$

$$(f, X) \cdot (g, Y) = \begin{cases} (f, X) & f > g \\ (g, Y) & f < g \\ (f, X \mathbin{\Vert} Y) & f = g \end{cases}$$

where $X \mathbin{\Vert} Y$ is $X$ followed by the items of $Y$ not in $X$.
The operation is associative, with $(0, \varnothing)$ as identity on the
positions at or after $0$: when the three farthest failures differ, both
groupings select the pair with the largest; when the largest is shared, both
groupings give the shared items in the order of their first occurrence. The
range of an outcome combines by the hull,
$[l, h] \sqcup [l', h'] = [\min(l, l'), \max(h, h')]$, which is associative and
commutative.

Recording an outcome $o$ into the result that encloses it, as a parse does with
the expectations of every failed terminal, replaces the outcome $c$ of the
enclosing result by $c \cdot o$ and its range $\rho$ by $\rho \sqcup \rho_o$.
With expectations not recorded, $X$ is empty and only $f$ takes part.

**The rest of a walk.** For a position $r$ define $\mathrm{Rest}(r)$, the end,
outcome and range of the walk from $r$ on, by

- if $E(r)$ fails or matches up to $r$:
  $\mathrm{Rest}(r) = (r,\ (f(r), X(r)),\ [l(r), h(r)])$;
- if $E(r)$ matches up to $r' > r$: $\mathrm{Rest}(r) = (\mathrm{end}', (f(r), X(r)) \cdot o',\ [l(r), h(r)] \sqcup \rho')$
  where $(\mathrm{end}', o', \rho') = \mathrm{Rest}(r')$.

The definition terminates because $r' > r$ and positions are bounded by $n$.
Unfolding it, $\mathrm{Rest}(r_i)$ ends where the walk from $r_i$ ends, and its
outcome and range are the products of those of the iterations at
$r_i, r_{i+1}, \dots$ in order.

## Lemma 1 (Exactness)

Every record stored at a position $r$ in the slots of $(\sigma, \lambda)$
equals $\mathrm{Rest}(r)$. A walk that reuses the record at $r$ ends at the
same position, leaves the enclosing result with the same outcome and range,
and succeeds or fails exactly as the walk that evaluates the iterations from
$r$ on.

*Proof.* By induction on the order in which records are stored.

A recording walk visits $r_j < \dots < r_k$ while recording, each visit an
iteration that the parser isolates: it evaluates $E(r_m)$ with the outcome and
range of the enclosing result set aside, takes what $E(r_m)$ recorded as
$(f(r_m), X(r_m))$ and $[l(r_m), h(r_m)]$, and then records them into the
enclosing result. The walk ends in one of two ways.

- At an iteration that fails or matches up to its own start. That iteration is
  the last visit $r_k$, and the record stored at $r_k$ is
  $(r_k, (f(r_k), X(r_k)), [l(r_k), h(r_k)]) = \mathrm{Rest}(r_k)$.
- At a recorded position $r_{k+1}$, whose record $R$ was stored earlier, so
  $R = \mathrm{Rest}(r_{k+1})$ by the induction hypothesis.

The records of the other visits are stored from $r_k$ back to $r_j$, each as
the product of the iteration at $r_m$ with the record just stored for
$r_{m+1}$, which is $\mathrm{Rest}(r_{m+1})$. Since $E(r_m)$ matched up to
$r_{m+1} > r_m$, the product is $\mathrm{Rest}(r_m)$ by the second case of the
definition. This proves the first claim.

A walk looks up a slot only after it has matched the minimum number of
iterations $c_{\min}$. The walk that evaluates on from $r$ ends where
$\mathrm{Rest}(r)$ ends, with at least $c_{\min}$ iterations, so it succeeds;
so does the walk that reuses the record. The evaluating walk records the
outcome and range of each iteration into the enclosing result in order, which
by associativity leaves it with $c \cdot o_{\mathrm{Rest}}$ and
$\rho \sqcup \rho_{\mathrm{Rest}}$, exactly what the reuse records at once.
$\square$

Whether a walk is plain or recording decides only which positions get records,
and Lemma 1 holds for every record stored. So the marks of visited positions,
and the order of the walks, never change the result of a parse.

## Lemma 2 (Cost)

In a parse with $W$ walks of $(\sigma, \lambda)$, the element of $\sigma$ is
evaluated at most $2(n + 1) + W (c_{\min} + 1)$ times in those walks, and they
look up at most $n + 1 + W (c_{\min} + 1)$ slots and store at most
$n + 1 + W c_{\min}$ records.

*Proof.* A slot moves only forward through the states fresh, visited and
recorded within a parse: a plain walk marks only fresh slots visited, and
records replace any state by recorded. A plain walk marks the position of an
iteration when it moves past it, at the next iteration or when it starts
recording there, so it marks every position it visits except the last one of
a walk that ends, or stops, while plain. A stream drops the slots of positions
before what it commits, and the parse never comes back to those positions.
Walks do not nest, because neither the element nor a rule it calls contains
`#recover`, so
each walk ends before the next one starts, or the parse stops.

Sort the evaluations of the element in the walks of $(\sigma, \lambda)$ by the
state of the slot of their position when they start.

1. *A plain walk at a fresh slot.* A plain walk visits only fresh slots, since
   it starts recording at the first slot that is not fresh. It marks each slot
   it visits but the last one of a walk that ends, or stops, while plain. A slot is marked once,
   so there are at most $n + 1$ such evaluations at slots that the walk marks,
   and at most $W$ at slots that it does not.
2. *A recording walk at a slot that is not recorded.* The walk ends, since the
   parse would otherwise stop, and stores a record at every position it visited
   while recording. Within a walk the positions increase, and the next walk
   starts after the records are stored, so each slot is evaluated this way at
   most once before it is recorded: at most $n + 1$ evaluations.
3. *A recording walk at a recorded slot.* A recording walk looks the slot up
   before each iteration from the minimum on and ends at a recorded slot, so it
   evaluates at a recorded slot only in its first $c_{\min}$ iterations: at most
   $W c_{\min}$ evaluations.

A walk that the parse stops, by a limit or a cancellation, is in one of the
cases above for each evaluation it started, and it stores no record, so it
changes no state that later evaluations count. No evaluation follows it, since
the parse has stopped.

Lookups are made only by recording walks, and each precedes an evaluation of
the second or third case or ends a walk, which gives the bound on lookups. A
record is stored for each visit of a recording walk, that is, for
each evaluation of the second and third cases. $\square$

**Orders of walks.** The bound holds whatever order the walks come in, since
it counts the states of slots and not which walk left them. For walks from each
position of one region of the input in turn, alternating with walks from one
fixed position of a second region, the second region is marked by its first
walk, recorded by its second and reused from the third on, while each walk of
the first region from the second on records from its first position and
reuses a record once it has matched its minimum. In a nested recovery, where
every walk starts one position before the one before it and runs to the end,
each walk marks its first position, records the second, and reuses the record
of the third.

**Memory.** The slots of $(\sigma, \lambda)$ that are not fresh are marked
by a bit for each position between the least and the greatest position walked,
at most $n + 1$ bits. Making room before the bits adds as much room as they
hold, down to position $0$, so walks that start further and further back copy
them $O(\log n)$ times, at a total cost of $O(n)$. The records are kept by
position, at most $n + 1$ of them, one for each visit of a recording walk. A
walk that ends at its first iteration touches no slot. A stream drops the bits
before what it commits, and removes the records there once they could make up
half of those kept, so the removals cost time in proportion to the records
stored.

The cost of the walks therefore follows: the element is evaluated at each
position at most twice, plus $c_{\min} + 1$ times per walk, and each walk is
started by an evaluation of the skip expression that the parse pays for in any
case.
