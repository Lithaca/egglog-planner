package main

import (
	"context"
	"egglog-planner/internal/cost"
	"egglog-planner/internal/eggbridge"
	"egglog-planner/internal/plan"
	"egglog-planner/internal/rewrite"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

type candidate struct {
	Plan     *plan.Node          `json:"plan"`
	Estimate cost.EstimateResult `json:"estimate"`
}
type report struct {
	Original    *plan.Node  `json:"original"`
	Candidates  []candidate `json:"candidates"`
	ChosenIndex int         `json:"chosen_index"`
	Chosen      *plan.Node  `json:"chosen"`
	Assumptions string      `json:"assumptions"`
}

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }
func run(args []string, out, errs io.Writer) int {
	if len(args) < 2 || len(args) > 3 || args[0] != "optimize" || (len(args) == 3 && args[2] != "--json") {
		fmt.Fprintln(errs, "usage: egglog-planner optimize <input.json> [--json]")
		return 2
	}
	root, err := projectRoot()
	if err != nil {
		fmt.Fprintln(errs, err)
		return 1
	}
	data, err := os.ReadFile(args[1])
	if err != nil {
		fmt.Fprintln(errs, "input:", err)
		return 1
	}
	in, err := plan.Parse(data)
	if err != nil {
		fmt.Fprintln(errs, err)
		return 1
	}
	exe := os.Getenv("EGGLOG_BIN")
	if exe == "" {
		exe = filepath.Join(root, ".tools", "bin", "egglog.exe")
	}
	work := filepath.Join(root, ".cache", "engine")
	if err := os.MkdirAll(work, 0700); err != nil {
		fmt.Fprintln(errs, err)
		return 1
	}
	plans, err := rewrite.Explore(context.Background(), in, eggbridge.Options{Executable: exe, WorkDir: work, Timeout: 10 * time.Second, MaxBytes: 8 << 20, MaxNodes: 5000}, 200)
	if err != nil {
		fmt.Fprintln(errs, err)
		return 1
	}
	selected, estimates, err := cost.Select(plans, plan.CatalogMap(in))
	if err != nil {
		fmt.Fprintln(errs, err)
		return 1
	}
	r := report{Original: in.Plan, ChosenIndex: selected, Chosen: plans[selected], Assumptions: cost.Assumptions}
	for i, p := range plans {
		r.Candidates = append(r.Candidates, candidate{Plan: p, Estimate: estimates[i]})
	}
	if len(args) == 3 {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		if err := enc.Encode(r); err != nil {
			fmt.Fprintln(errs, err)
			return 1
		}
		return 0
	}
	fmt.Fprintf(out, "Candidates: %d\nChosen: %d\nEstimated work: %.4f\nEstimated rows: %.4f\nAssumptions: %s\n", len(plans), selected, estimates[selected].Total, estimates[selected].Rows, r.Assumptions)
	original, _ := json.MarshalIndent(r.Original, "", "  ")
	fmt.Fprintf(out, "Original plan:\n%s\n", original)
	for i, c := range r.Candidates {
		fmt.Fprintf(out, "Candidate %d: estimated work %.4f, rows %.4f\n", i, c.Estimate.Total, c.Estimate.Rows)
		encoded, _ := json.MarshalIndent(c.Plan, "", "  ")
		fmt.Fprintf(out, "Candidate %d plan:\n%s\n", i, encoded)
		for j, step := range c.Estimate.Steps {
			fmt.Fprintf(out, "Step %d: %s rows %.4f, local work %.4f, cumulative work %.4f\n", j, step.Operator, step.Rows, step.LocalCost, step.CumulativeCost)
		}
	}
	b, _ := json.MarshalIndent(r.Chosen, "", "  ")
	fmt.Fprintf(out, "Chosen plan:\n%s\n", b)
	return 0
}
func projectRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		next := filepath.Dir(dir)
		if next == dir {
			return "", fmt.Errorf("project root (go.mod) not found")
		}
		dir = next
	}
}
