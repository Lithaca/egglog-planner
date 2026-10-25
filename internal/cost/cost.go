package cost

import (
	"egglog-planner/internal/plan"
	"fmt"
	"math"
)

const Assumptions = "Estimated rows: Scan=base rows; Filter=input rows × selectivity; Join=left rows × right rows × selectivity. Estimated work: Scan=rows; Filter/Project=input rows; Join=left rows × right rows + output rows. No indexes, memory or physical operators are modeled."

type Step struct {
	Operator       string  `json:"operator"`
	Rows           float64 `json:"rows"`
	LocalCost      float64 `json:"local_cost"`
	CumulativeCost float64 `json:"cumulative_cost"`
}
type EstimateResult struct {
	Rows  float64 `json:"rows"`
	Total float64 `json:"total"`
	Steps []Step  `json:"steps"`
}

func Estimate(n *plan.Node, catalog map[string]plan.Relation) (EstimateResult, error) {
	if _, err := plan.ValidateNode(n, catalog); err != nil {
		return EstimateResult{}, err
	}
	r, err := estimate(n, catalog)
	if err != nil {
		return EstimateResult{}, err
	}
	if math.IsInf(r.Total, 0) || math.IsNaN(r.Total) || math.IsInf(r.Rows, 0) || math.IsNaN(r.Rows) {
		return r, fmt.Errorf("cost: estimate overflow")
	}
	return r, nil
}
func estimate(n *plan.Node, catalog map[string]plan.Relation) (EstimateResult, error) {
	var r EstimateResult
	var local float64
	switch n.Op {
	case "Scan":
		r.Rows = catalog[n.Relation].Rows
		local = r.Rows
	case "Filter", "Project":
		child, err := estimate(n.Input, catalog)
		if err != nil {
			return r, err
		}
		r = child
		local = child.Rows
		if n.Op == "Filter" {
			r.Rows = child.Rows * n.Predicate.Selectivity
		}
	case "Join":
		left, err := estimate(n.Left, catalog)
		if err != nil {
			return r, err
		}
		right, err := estimate(n.Right, catalog)
		if err != nil {
			return r, err
		}
		r.Rows = left.Rows * right.Rows * n.Predicate.Selectivity
		local = left.Rows*right.Rows + r.Rows
		r.Total = left.Total + right.Total
		r.Steps = append(left.Steps, right.Steps...)
	}
	r.Total += local
	r.Steps = append(r.Steps, Step{Operator: n.Op, Rows: r.Rows, LocalCost: local, CumulativeCost: r.Total})
	return r, nil
}
func Select(candidates []*plan.Node, catalog map[string]plan.Relation) (int, []EstimateResult, error) {
	if len(candidates) == 0 {
		return -1, nil, fmt.Errorf("cost: no candidates")
	}
	results := make([]EstimateResult, len(candidates))
	best := -1
	for i, n := range candidates {
		r, err := Estimate(n, catalog)
		if err != nil {
			return -1, nil, fmt.Errorf("candidate %d: %w", i, err)
		}
		results[i] = r
		if best < 0 || r.Total < results[best].Total {
			best = i
		}
	}
	return best, results, nil
}
