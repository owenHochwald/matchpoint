# simulation Behaviour Specification

## Scope

`simulation` provides deterministic player state transitions, population
seeding, non-blocking result delivery, and convergence checks. Default tests use
small in-memory fakes; no Redis, Docker, network, or real timers are required.

## Behaviours

B-SIMULATION-1: Given zero config, when an engine is created, then defaults from `FEATURES.md` are applied.

B-SIMULATION-2: Given invalid config values, when an engine is created, then `SimStatusInvalidConfig` is returned.

B-SIMULATION-3: Given caller-owned state storage, when `SeedPopulation` runs, then every player gets a non-zero ID, valid phase, trophy floor, and normalized deck vector.

B-SIMULATION-4: Given a queued player, when `SimPlayerTick` runs, then the player moves to waiting and publishes a ticket.

B-SIMULATION-5: Given a waiting player with a result, when `SimPlayerTick` runs, then assignment metadata is copied and phase becomes matched.

B-SIMULATION-6: Given a matched player, when `SimPlayerTick` runs, then phase becomes playing and match end time is deterministic and non-negative.

B-SIMULATION-7: Given a playing player whose match has ended, when `SimPlayerTick` runs, then phase becomes post-match and outcome is resolved from predicted win probability.

B-SIMULATION-8: Given a post-match loss or win, when `SimPlayerTick` runs, then tilt, churn risk, streaks, and trophies are updated in place.

B-SIMULATION-9: Given a loss below tier floor, when trophies are updated, then the result is clamped to the reached floor.

B-SIMULATION-10: Given mutation probability succeeds, when `SimPlayerTick` runs in post-match, then one deck dimension shifts by `0.1` and the vector is re-normalized.

B-SIMULATION-11: Given session losses exceed config or churn roll succeeds, when post-match completes, then phase becomes quit.

B-SIMULATION-12: Given an empty result mailbox, when `DeliverResult` runs, then it sends without blocking.

B-SIMULATION-13: Given a full result mailbox, when `DeliverResult` runs, then it returns `SimStatusDropped` and records a drop.

B-SIMULATION-14: Given all warm-up gates met, when `CheckConvergence` runs, then `Converged` is true.

B-SIMULATION-15: Given any failed warm-up gate, when `CheckConvergence` runs, then `SimStatusConvergenceFail` and the first failed gate are returned.

B-SIMULATION-16: Given hot-loop player ticks, when benchmarked with caller-owned inputs/outputs, then `SimPlayerTick` performs `0 B/op`.

## Allocation Budget

| Function | Budget |
| --- | ---: |
| `SimPlayerTick` | `0 B/op` |
| `DeliverResult` | `0 B/op` |
| `CheckConvergence` | `0 B/op` |
| `SeedPopulation` | `0 B/op` excluding caller-owned slice allocation |

## Edge Cases

| Case | Expected handling |
| --- | --- |
| Nil state or output | Return `SimStatusInvalidState`. |
| Full result channel | Drop newest assignment and increment metrics. |
| Negative match duration sample | Clamp to zero duration. |
| Deck mutation creates near-zero vector | Restore a one-hot fallback dimension. |
| Warm-up not elapsed | Fail convergence gate 1. |
