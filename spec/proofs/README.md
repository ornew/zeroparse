# ZeroParse Proofs

The runtimes reuse results to bound the cost of a parse. Each mechanism of
reuse is stated in the [Parser and Runtime Specification](../runtime/README.md),
and a reuse is correct only if it gives the result that evaluating again would
give. The proofs here show that, and the cost bounds that the mechanisms claim,
as lemmas over the definitions of the specifications that they cite.

## Contents

| Proof | Shows |
|:--|:--|
| [Skip Runs](skip-runs.md) | A reused record of a [skip run](../runtime/parsing.md#skip-runs) gives what the rest of the walk would, and the walks of a site evaluate its element a linear number of times |
| [Memoization](memoization.md) | Answering a call from a memo entry, including the members of a left-recursion cycle and results that depend on the order of evaluation, gives the outcome that evaluating it again would give |
| [Documents](documents.md) | A parse of a [Document](../runtime/documents.md) after edits, reusing carried memo entries and recorded runs, gives the result of a fresh parse of its text |
| [Linear Cost](linear-cost.md) | An outline of how the [Conditions for Linear Cost](../runtime/parsing.md#conditions-for-linear-cost) bound the cost of a parse, with its lemmas and the steps still open |
| [Constant-Cost Text](constant-cost-text.md) | Taking the [text](../language/actions.md#text) of a node is a step of constant cost given the guarantee of [Cost](../runtime/parsing.md#cost), so the conditions need not count it as a copy, and a comparison of strings may count only one operand |
