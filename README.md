# egglog-planner

A small, independent Go planner. It uses **egglog 0.4.0** to explore equivalent restricted relational plans, then uses supplied statistics in Go to choose the lowest *estimated* work among validated candidates. It does not execute queries or parse SQL.

## Run on Windows PowerShell

Requires Go 1.26.2 and the official egglog CLI 0.4.0. Put `egglog.exe` in `.tools/bin/` or set `EGGLOG_BIN` to its path.

```powershell
go test ./...
go vet ./...
./scripts/build.ps1
./bin/egglog-planner.exe optimize examples/two-tables.json
./bin/egglog-planner.exe optimize examples/three-tables.json --json
```

The build script puts Go cache in `.cache/go-build` and the executable in `bin/`; both paths are ignored by Git. The CLI also puts temporary egglog programs and graph output in `.cache/engine` and removes each temporary directory when finished. Engine runs have a ten second timeout, an 8 MiB graph limit, a 5,000 node limit, and a 200 valid candidate limit. Limit errors are explicit.

## Version 1 JSON format

See [the two-table example](examples/two-tables.json) and [the three-table example](examples/three-tables.json). Top-level fields are `version: 1`, `catalog`, and `plan`. The catalog is an array of unique relations with `name`, positive `rows`, and `columns` (`name`, `type`). Types are `int` or `string`. Each base relation may appear once. All references use `relation.column`; aliases and unqualified columns are unsupported.

The `plan` is a tree of:

| `op` | Required fields | Meaning |
| --- | --- | --- |
| `Scan` | `relation` | Read a base relation. |
| `Filter` | `predicate`, `input` | Keep rows satisfying `eq_const`. |
| `Project` | `columns`, `input` | Return those columns in that order, preserving duplicate rows. |
| `Join` | `predicate`, `left`, `right` | Inner equijoin. |

`Filter` predicates use `{"kind":"eq_const","left":"orders.status","value":"open","value_type":"string","selectivity":0.01}`. `Join` predicates use `{"kind":"eq_cols","left":"orders.customer_id","right":"customers.id","selectivity":0.01}`. Selectivities are supplied estimates in `(0,1]`; omitted or invalid values are errors. `value_type` must match the referenced column, and join columns must have the same type and occur on opposite sides. Integer constants are JSON strings containing signed base-10 64-bit integers. Unknown fields and unsupported operators are errors.

The CLI prints the original plan, all accepted candidates and their per-node estimates in `--json` mode. Text mode prints the original and every candidate plan, each estimation step, totals, the chosen plan, and the cost assumptions. It reports engine failures, missing statistics, malformed output, and timeout as errors with nonzero exit status.

## Cost model and example

Estimated rows: `Scan = catalog.rows`; `Filter = input.rows × selectivity`; `Join = left.rows × right.rows × selectivity`; `Project = input.rows`. Estimated work: `Scan = rows`; `Filter` and `Project` each add their input rows; `Join` adds `left.rows × right.rows + output.rows`. Total work is the sum over the tree. This is a simple nested-loop-style *estimate*, not measured runtime. It omits indexes, memory, physical join algorithms, correlations, and data distribution. Statistics are treated as user-supplied assumptions.

With the included statistics, `two-tables.json` gives four candidates. Pushing `orders.status = "open"` below the join reduces estimated work from **1,030,200** to **30,300** while estimated output stays at 100 rows. `three-tables.json` gives eight candidates. Joining `b` with `c` before joining `a` reduces estimated work from **10,123,010** to **1,023,110** while estimated output stays at 1,000 rows. These are reproducible estimates under this model, not database execution measurements.

## Correctness boundary

egglog produces algebraic alternatives from [the embedded rules](rules/planner.egg). Go accepts a candidate only when its scans, predicates, and projections match the original multiset; all column references and types remain valid; and root output columns retain the original order. The broad egglog rules may produce invalid intermediate terms; those are discarded. Tests also compare duplicate-row results for small deterministic fixtures. This is a limited relational-algebra check, not a proof for arbitrary SQL.

Only inner joins and qualified, typed equality predicates are supported. NULL, outer joins, aggregation, sorting, LIMIT, self-joins/aliases, other operators, and query execution are outside version 1. See [extraction notes](docs/egglog-extraction-notes.md) for why egglog's default extraction cost is not used as the query cost.
