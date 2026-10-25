# egglog 0.4.0 extraction check

Tested with `egglog 0.4.0_2026-10-21_` (official CLI binary) on Windows PowerShell. Input:

```lisp
(datatype Plan (Scan String) (Join Plan Plan))
(let p (Join (Scan "big") (Scan "small")))
(rewrite (Join a b) (Join b a))
(run 2)
(extract p)
```

`egglog.exe extract.egg` printed exactly:

```text
(Join (Scan "big") (Scan "small"))
```

Changing the last command to `(extract p :variants 4)` produced **no stdout** with this binary, so we do not use that CLI output as a candidate protocol. `egglog.exe --to-json extract.egg` wrote `extract.json`; its `nodes` contained `function-0-Join` with children `big,small` and `function-2-Join` with children `small,big`, both in e-class `Plan-2`. Those are the two alternatives observed in the real integration test. The JSON node `cost` was `1.0` for both join constructors; it is an egglog extraction weight, not a database execution estimate.

The planner therefore wraps the query term in a `Root` constructor, runs bounded rewrites, reads the e-graph JSON, enumerates terms in the root plan's e-class, validates their relational semantics in Go, and evaluates each using supplied row counts and selectivities. It never treats default AST size as the database cost. This approach is bounded by process time, graph bytes/nodes, tree depth, and candidate count. Very large e-graphs fail with an explicit limit error rather than claiming exhaustive optimization.

A second real-CLI integration probe starts from `Filter("f", Join("j", Scan("big"), Scan("small")))` and applies the filter-pushdown rewrite once. The exported root e-class contains both a `Filter` root (late filter) and a `Join` root (pushed filter). With `big.rows=1000`, `small.rows=100`, filter selectivity `0.01`, and join selectivity `0.01`, Go computes estimated work **103,100** for the late filter and **3,110** for the pushed filter. The default `(extract p)` output alone does not encode this database-specific comparison.

The [egglog 0.4.0 changelog](https://github.com/egraphs-good/egglog/blob/v0.4.0/CHANGELOG.md) describes the CLI and extraction support of this release.
