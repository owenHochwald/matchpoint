# SESSIONS.md - MatchPoint Work Ledger

Use this file for lightweight handoff state only. It answers: where should the
next agent resume, what has been planned, what has been implemented, what is
under review, and what is blocked.

`docs/FEATURES.md` remains the source of truth for final completion. A module is
checked there only after implementation, review, and required checks are done.

## How To Resume

When starting a new agent session, say:

```text
Continue on our workload.
```

The agent should:

1. Read `docs/FEATURES.md` for the first unchecked module.
2. Read this file for any active session state.
3. Inspect `git status --short` and the active module files.
4. Continue from the latest state instead of restarting from scratch.

No slash command is required. If a future Codex setup adds project slash
commands, the command should do the same four steps and no more.

## State Values

- `not_started`: no current work beyond historical artifacts.
- `planning`: planning subagent is being used or the plan is being refined.
- `planned`: implementation plan exists in the conversation or summary.
- `implementing`: code/tests are being edited.
- `implemented`: code/tests exist but review has not completed.
- `reviewing`: review subagent is auditing or fixes are being applied.
- `blocked`: human input or external dependency is required.
- `complete`: module is checked off in `docs/FEATURES.md`.

## Current Work

| Module | State | Notes |
| --- | --- | --- |
| vectorarch | complete | Reviewed, patched for downstream construction and `MP_COUNTER_THRESHOLD`, and checked in `docs/FEATURES.md`. |
| simulation | complete | Reviewed, patched quit output and env-configurable defaults, and checked in `docs/FEATURES.md`. |
| telemetry | complete | Async metrics ring, WebSocket visualizer bridge, tests, race run, benchmarks, full tests, and vet are clean. |

## Last Resume Notes

- Historical contract/spec/report/task files are context only.
- Do not create new planning reports, checker reports, task handoff files, or
  module READMEs unless explicitly requested.
- On resume, all listed modules are complete; choose the next human-directed
  follow-up.
