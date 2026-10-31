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
