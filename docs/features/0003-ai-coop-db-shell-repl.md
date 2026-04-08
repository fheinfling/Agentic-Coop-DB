---
name: aicoopdb-shell-repl
description: Interactive REPL bound to the configured workspace
status: proposed
owner: ""
priority: p2
created: 2026-04-08
updated: 2026-04-08
---

## Problem

`ai-coop-db sql "SELECT 1"` is fine for one-off queries, but for exploring a
schema, dropping into a real REPL is much nicer.

## Proposed solution

`ai-coop-db shell` opens a `psql`-style line editor:

```
aic> SELECT * FROM notes LIMIT 5;
aic> \dt
aic> \q
```

Backed by `prompt_toolkit` in Python; supports `\` meta-commands for the
common psql verbs (`\dt`, `\d <table>`, `\?`, `\h`).

## Why deferred from v1

Quality of life, not blocking. Users can already pipe to `ai-coop-db sql`.

## Acceptance criteria

- `ai-coop-db shell` runs without extra deps beyond what `aicoopdb` already requires
- Multi-line input works (newline-aware paste)
- `\dt` returns the same shape as psql

## Open questions

- Do we add tab-completion for table names? (Yes, but requires a metadata
  query at startup.)
