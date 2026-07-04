# MatchPoint Agent Prompt

You are the implementation owner for Project MatchPoint. Use the lightweight
agent loop described in `docs/AGENTS.md`.

## Start Or Resume

When asked to continue work:

1. Read `docs/FEATURES.md` for the first unchecked module.
2. Read `docs/SESSIONS.md` for in-progress state and resume notes.
3. Run `git status --short` and inspect active module files.
4. Continue from the current state without overwriting user-created work.

No slash command is required. The expected user prompt is:

```text
Continue on our workload.
```

## Active Loop

- Use a planning subagent for meaningful design work. The planner writes no
  files and returns an implementation plan in chat.
- The main agent implements code and tests.
- Use a review subagent after implementation. The reviewer audits the diff and
  may make narrow fixes.
- Update `docs/SESSIONS.md` whenever a module changes state.
- Update the checklist in `docs/FEATURES.md` only after implementation, review,
  and required checks are complete.

## Artifact Policy

Historical files under `contracts/`, `tasks/`, `reports/`, and module READMEs
are context only. Do not create new per-module contracts, task files, checker
reports, or module READMEs unless the human explicitly asks.

## Checks

Choose checks based on the change:

```bash
go test ./internal/<module>/... -count=1
go test ./internal/<module>/... -race -count=1
go test ./... -count=1
go test ./internal/<module>/... -bench=. -benchmem -run='^$'
go vet ./...
staticcheck ./...
```
