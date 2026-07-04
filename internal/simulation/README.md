# Simulation Module

This package implements Module E: deterministic helpers for the macro-simulation
player state machine.

## Invariants

- `SimPlayerState` embeds `contracts.Ticket` and stays under the 200-byte target.
- Player ticks are deterministic and take caller-provided rolls, timestamps, and output storage.
- Result delivery uses a capacity-one mailbox and non-blocking `select`; full mailboxes drop the newest result and increment metrics.
- Trophy losses respect the highest tier floor reached by the player.

## Hot Path Notes

- `SimPlayerTick`, `DeliverResult`, and `CheckConvergence` allocate `0 B/op`.
- Population seeding writes into caller-owned slices and uses a small deterministic mixer instead of global random state.
- Deck mutation shifts one vector dimension and immediately re-normalizes in place.
