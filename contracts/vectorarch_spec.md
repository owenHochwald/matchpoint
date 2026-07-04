# vectorarch Behaviour Specification

## Scope

`vectorarch` provides pure Go `[8]float32` normalization, cosine similarity,
and counter/similar classification. It has no Redis, timer, or downstream
package dependency.

## Behaviours

B-VECTORARCH-1: Given a zero-value config, when an engine is created, then it uses the default zero, counter, and similar thresholds.

B-VECTORARCH-2: Given invalid thresholds, when an engine is created, then creation returns `VectorStatusInvalidConfig`.

B-VECTORARCH-3: Given finite non-zero raw dimensions, when `Normalize` runs, then the output is L2-normalized.

B-VECTORARCH-4: Given magnitude below `VectorZeroThreshold`, when `Normalize` runs, then it returns `VectorStatusZeroVector`.

B-VECTORARCH-5: Given NaN or infinite input, when `Normalize` or `AnalyzePair` runs, then it returns `VectorStatusInvalidInput`.

B-VECTORARCH-6: Given normalized vectors, when `CosineSimilarity` runs, then it returns their dot product clamped to `[-1, 1]`.

B-VECTORARCH-7: Given similarity below the counter threshold, when `AnalyzePair` runs, then `Class` is `VectorPairCounter`.

B-VECTORARCH-8: Given similarity above the similar threshold, when `AnalyzePair` runs, then `Class` is `VectorPairSimilar`.

B-VECTORARCH-9: Given similarity between thresholds, when `AnalyzePair` runs, then `Class` is `VectorPairDefault`.

B-VECTORARCH-10: Given a caller-owned output pointer, when vector functions run in a hot loop, then they perform `0 B/op`.

## Allocation Budget

| Function | Budget |
| --- | ---: |
| `Normalize` | `0 B/op` |
| `CosineSimilarity` | `0 B/op` |
| `AnalyzePair` | `0 B/op` |

## Edge Cases

| Case | Expected handling |
| --- | --- |
| Zero or near-zero magnitude | Return `VectorStatusZeroVector`; do not divide. |
| NaN/Inf component | Return `VectorStatusInvalidInput`. |
| Floating-point overshoot | Clamp similarity to `[-1, 1]`. |
| Counter threshold >= similar threshold | Reject config. |
