# Vectorarch Module

This package implements Module D: fixed-width deck archetype vector math.

## Invariants

- Vectors are always `[8]float32`; no slice allocation is needed on the hot path.
- `Normalize` rejects near-zero magnitudes before division and writes into caller-owned output.
- `CosineSimilarity` assumes normalized inputs, so match-time scoring is only an eight-lane dot product.
- Pair classification uses the same counter and similar thresholds referenced by EOMM.

## Hot Path Notes

- The baseline is pure Go. SIMD remains an optimization escape hatch, but the current code is easier to audit and benchmark.
- All benchmarked vector functions report `0 B/op` with caller-owned output storage.
