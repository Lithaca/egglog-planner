package cost

import (
	"egglog-planner/internal/plan"
	"testing"
)

func TestPushdownLowersEstimatedJoinCost(t *testing.T) {
	v := "open"
	f := &plan.Predicate{Kind: "eq_const", Left: "a.status", Value: &v, ValueType: "string", Selectivity: .01}
	j := &plan.Predicate{Kind: "eq_cols", Left: "a.id", Right: "b.id", Selectivity: .001}
	a := func() *plan.Node { return &plan.Node{Op: "Scan", Relation: "a"} }
	b := func() *plan.Node { return &plan.Node{Op: "Scan", Relation: "b"} }
	late := &plan.Node{Op: "Filter", Predicate: f, Input: &plan.Node{Op: "Join", Predicate: j, Left: a(), Right: b()}}
	early := &plan.Node{Op: "Join", Predicate: j, Left: &plan.Node{Op: "Filter", Predicate: f, Input: a()}, Right: b()}
	catalog := map[string]plan.Relation{"a": {Name: "a", Rows: 1000, Columns: []plan.Column{{Name: "id", Type: "int"}, {Name: "status", Type: "string"}}}, "b": {Name: "b", Rows: 100, Columns: []plan.Column{{Name: "id", Type: "int"}}}}
	selected, estimates, err := Select([]*plan.Node{late, early}, catalog)
	if err != nil {
		t.Fatal(err)
	}
	if selected != 1 || estimates[1].Total >= estimates[0].Total {
		t.Fatalf("selected %d costs %v", selected, estimates)
	}
	if estimates[1].Rows != estimates[0].Rows {
		t.Fatalf("bag result row estimates differ: %v", estimates)
	}
}

func TestMissingStatisticsFails(t *testing.T) {
	_, err := Estimate(&plan.Node{Op: "Scan", Relation: "a"}, map[string]plan.Relation{})
	if err == nil {
		t.Fatal("missing relation statistics accepted")
	}
}

func TestLargerTreeCanHaveLowerEstimatedWork(t *testing.T) {
	v := "open"
	f := &plan.Predicate{Kind: "eq_const", Left: "a.status", Value: &v, ValueType: "string", Selectivity: .01}
	j := &plan.Predicate{Kind: "eq_cols", Left: "a.id", Right: "b.id", Selectivity: .001}
	scan := func(s string) *plan.Node { return &plan.Node{Op: "Scan", Relation: s} }
	late := &plan.Node{Op: "Filter", Predicate: f, Input: &plan.Node{Op: "Join", Predicate: j, Left: scan("a"), Right: scan("b")}}
	early := &plan.Node{Op: "Project", Columns: []string{"a.id", "a.status", "b.id"}, Input: &plan.Node{Op: "Join", Predicate: j, Left: &plan.Node{Op: "Filter", Predicate: f, Input: scan("a")}, Right: scan("b")}}
	catalog := map[string]plan.Relation{"a": {Name: "a", Rows: 1000, Columns: []plan.Column{{Name: "id", Type: "int"}, {Name: "status", Type: "string"}}}, "b": {Name: "b", Rows: 100, Columns: []plan.Column{{Name: "id", Type: "int"}}}}
	index, estimates, err := Select([]*plan.Node{late, early}, catalog)
	if err != nil {
		t.Fatal(err)
	}
	if index != 1 || estimates[1].Total >= estimates[0].Total {
		t.Fatalf("larger tree did not win: %+v", estimates)
	}
}
