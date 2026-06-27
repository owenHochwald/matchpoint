# GIT_POLICY.md — MatchPoint Version Control Policy

> All agent commits must follow this policy exactly. Deviation is a
> pipeline violation and the commit must be amended before the module
> is considered complete.

---

## Co-Authorship

Every commit made by the agent must include both co-authors in the
commit message trailer, exactly as shown:

```
Co-authored-by: Owen Hochwald <owenhochwald@gmail.com>
Co-authored-by: Codex <codex@openai.com>
```

Both lines are required on every commit, no exceptions. The agent's
own authorship (set via git config) identifies the agent as the
technical committer; the co-author trailers attribute ownership to
Owen and Codex as the directing principals.

---

## Commit Message Format

```
<type>(<module>): <short imperative description>

<body — optional, explain why not what, wrap at 72 chars>

Co-authored-by: Owen Hochwald <owenhochwald@gmail.com>
Co-authored-by: Codex <codex@openai.com>
```

**Type prefixes:**

| Type     | When to use                                          |
|----------|------------------------------------------------------|
| contract | Planner phase output (contract.go + spec.md)        |
| feat     | Implementor phase — new implementation file         |
| test     | Implementor phase — test or benchmark file          |
| fix      | Checker-driven regression fix (after PLAN re-cycle) |
| audit    | Checker phase — checker report committed            |
| docs     | README or spec updates not tied to a code change    |
| chore    | go.mod, go.sum, tooling config, CI changes          |

**Examples:**

```
contract(ticket): define Ticket struct and pool interface

Establishes the zero-allocation contract for ticket ingestion.
Pool interface separates Acquire/Release lifecycle from parsing.

Co-authored-by: Owen Hochwald <owenhochwald@gmail.com>
Co-authored-by: Codex <codex@openai.com>
```

```
feat(ringbuffer): implement SPSC lock-free ring buffer

Head and tail use atomic.Uint64. Slot array pre-allocated at
construction. Write path verified 0 B/op under -benchmem.

Co-authored-by: Owen Hochwald <owenhochwald@gmail.com>
Co-authored-by: Codex <codex@openai.com>
```

```
audit(ringbuffer): checker report — PASS

All four audits clean. Zero allocs on Write/Read hot paths.
Race stress 50x clean. No GC frames in top-5 CPU profile.

Co-authored-by: Owen Hochwald <owenhochwald@gmail.com>
Co-authored-by: Codex <codex@openai.com>
```

---

## Branch Strategy

```
main
└── agent/<module>   — one branch per module, created at PLAN phase start
```

The agent creates a new branch at the start of each module's Planner
phase and merges to main only after the Checker issues a PASS verdict.

Branch naming:
```
agent/ticket
agent/ringbuffer
agent/redisqueue
agent/matchcore
agent/eomm
agent/vectorarch
agent/simulation
agent/telemetry
```

**Merge strategy:** squash merge to main. The squash commit message
uses the module's final audit commit message as its body, with both
co-author trailers preserved.

---

## Commit Granularity

Each phase produces exactly one commit on the module branch:

| Phase       | Commit contents                                      |
|-------------|------------------------------------------------------|
| PLAN        | contracts/<module>_contract.go + contracts/<module>_spec.md |
| IMPLEMENTOR | internal/<module>/<module>.go + _test.go + README.md |
| CHECKER     | reports/<module>_checker_report.md                  |

If a Checker FAIL triggers a PLAN re-cycle, the amended contract is
committed as a new `fix(contract/<module>)` commit on the same branch
before the Implementor retouches any code.

Do not bundle multiple modules into one commit. Do not commit
partial implementations. Each commit must leave the repository in
a buildable, `go build ./...`-clean state.

---

## What the Agent Must Never Do

- Force-push to main
- Commit with `--no-verify` to bypass hooks
- Omit either co-author trailer
- Commit generated files (binaries, cpu.prof, *.test) — these belong
  in .gitignore
- Commit a module that has not passed the Checker phase