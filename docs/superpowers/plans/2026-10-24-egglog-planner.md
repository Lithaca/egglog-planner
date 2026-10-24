# egglog-planner Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build an independently testable Go CLI that uses egglog to explore equivalent, restricted relational plans and chooses one by an explicit estimated cost.

**Architecture:** A typed Go plan/catalog layer validates JSON and column references. Separate egglog rules explore equivalent logical plans. A Go cost/extraction layer chooses a valid candidate, and the CLI prints the chosen plan with estimates.

**Tech Stack:** Go 1.26.2, official egglog CLI 0.4.0, Windows PowerShell build script.

**Spec:** `docs/superpowers/specs/2026-10-24-egglog-planner-design.md`

## Global Constraints

- Do not depend on `egglog-helper/internal`; this is a separate Go module.
- JSON input only; no full SQL parser or query executor in v1.
- Inner joins, qualified columns, unique relation names, restricted equality predicates only.
- Preserve bag semantics, root output column order and predicate attachment.
- Cost is an estimate based on supplied statistics, not measured runtime.
- Fail visibly on missing/invalid statistics, malformed egglog output, process failure or timeout.

## Review Focus

- A filter referencing both join inputs must never be pushed to one input.
- Join reassociation must preserve every equality predicate and its referenced columns.
- Joining the same base relation twice must be rejected until aliases are modeled.
- Projection must not drop columns still needed by a later filter or join.
- A plan with lower AST size but higher estimated cost must not be called optimal.

---

### Task 1: Establish the extraction path

**Files:** `internal/eggbridge/bridge.go`, `internal/eggbridge/bridge_test.go`, `docs/egglog-extraction-notes.md`.

- [ ] Write an integration test using egglog 0.4.0 with two equivalent plan-shaped terms of different estimated costs.
- [ ] Run it against the real CLI and record how candidates or custom costs can be obtained, including exact CLI output.
- [ ] Select an extraction approach supported by the pinned version; document its limitations before implementing cost selection.
- [ ] Implement bounded process invocation, strict output parsing and errors for missing engine, malformed output and timeout.
- [ ] Run `go test ./internal/eggbridge -v` with the real egglog binary.

### Task 2: Typed plan and catalog

**Files:** `internal/plan/plan.go`, `internal/plan/plan_test.go`, `internal/plan/json.go`, `examples/two-tables.json`.

- [ ] Write tests for valid Scan/Filter/Project/InnerJoin trees and JSON round trip.
- [ ] Write rejection tests for unknown columns, ambiguous names, invalid equality types, duplicate base relations and unsupported operators.
- [ ] Define stable versioned JSON fields, typed predicate/column references and catalog statistics; document them in README.
- [ ] Implement parsing, schema propagation and validation; run `go test ./internal/plan -v`.

### Task 3: Logical rewrites and equivalence guards

**Files:** `rules/planner.egg`, `internal/rewrite/rewrite.go`, `internal/rewrite/rewrite_test.go`.

- [ ] Write real-engine tests for single-side filter pushdown and three-table inner-join reorder.
- [ ] Write negative tests for a two-side predicate, lost join condition and projection of a needed column.
- [ ] Implement rule encoding/decoding and guards so invalid candidates are discarded or rejected.
- [ ] Use small deterministic data fixtures to compare original and rewritten bag results; run `go test ./internal/rewrite -v`.

### Task 4: Cost selection

**Files:** `internal/cost/cost.go`, `internal/cost/cost_test.go`.

- [ ] Write tests where a smaller printed tree costs more, and where early filter pushdown reduces estimated rows.
- [ ] Implement a documented row-count and join-cost estimate using supplied statistics.
- [ ] Select the cheapest valid candidate using the Task 1 extraction path; report intermediate estimates and assumptions.
- [ ] Run `go test ./internal/cost -v`.

### Task 5: CLI, build and complete example

**Files:** `cmd/egglog-planner/main.go`, `cmd/egglog-planner/main_test.go`, `scripts/build.ps1`, `README.md`.

- [ ] Write CLI tests for input, text/JSON output, exit codes and engine/statistics errors.
- [ ] Implement `egglog-planner optimize <input.json>` and a reproducible Windows build with pinned egglog.
- [ ] Run `go test ./...`, `go vet ./...`, the build script, and both two-table and three-table end-to-end examples.
- [ ] Update README with commands, exact input schema, cost assumptions, limitations and an honest example result.
