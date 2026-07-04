# GIT_POLICY.md — MatchPoint Version Control Policy

> All agent commits must follow this policy exactly. Deviations must be amended
> before the module is considered complete.

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

| Type     | When to use                                             |
|----------|---------------------------------------------------------|
| feat     | User-visible or module behavior implementation          |
| test     | Test, benchmark, fuzz, or integration coverage          |
| fix      | Correctness, review, race, allocation, or regression fix |
| docs     | Canonical docs, progress checklist, or session ledger    |
| chore    | go.mod, go.sum, tooling config, CI changes              |

**Examples:**

```
feat(ringbuffer): implement SPSC lock-free ring buffer

Head and tail use atomic.Uint64. Slot array pre-allocated at
construction. Write path verified 0 B/op under -benchmem.

Co-authored-by: Owen Hochwald <owenhochwald@gmail.com>
Co-authored-by: Codex <codex@openai.com>
```

```
fix(vectorarch): cover zero-vector similarity guard

Rejects normalization of empty archetype vectors and adds regression
coverage for NaN prevention.

Co-authored-by: Owen Hochwald <owenhochwald@gmail.com>
Co-authored-by: Codex <codex@openai.com>
```

---

## Branch Strategy

```
main
└── agent/<module>   — one branch per active module
```

The agent may create a module branch before implementation work begins and
merges to main only after implementation, review, and required checks are
complete.

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

**Merge strategy:** squash merge to main. The squash commit message summarizes
the completed module and preserves both co-author trailers.

---

## Commit Granularity

Prefer cohesive commits that leave the repository buildable:

- implementation and tests for a focused module capability,
- review fixes,
- canonical docs or session-state updates.

Do not bundle multiple modules into one commit. Do not commit partial
implementations as complete work. Each completion commit should leave the
repository in a buildable, `go build ./...`-clean state when feasible.

---

## What the Agent Must Never Do

- Force-push to main
- Commit with `--no-verify` to bypass hooks
- Omit either co-author trailer
- Commit generated files (binaries, cpu.prof, *.test) — these belong
  in .gitignore
- Mark a module complete before implementation, review, and required checks pass
