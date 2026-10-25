package rewrite

import (
	"context"
	"egglog-planner/internal/eggbridge"
	"egglog-planner/internal/plan"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

func TestRealEnginePushdownAndReorder(t *testing.T) {
	exe := os.Getenv("EGGLOG_BIN")
	if exe == "" {
		exe = filepath.Join("..", "..", ".tools", "bin", "egglog.exe")
	}
	if _, err := os.Stat(exe); err != nil {
		t.Skip("egglog 0.4.0 unavailable")
	}
	value := "open"
	scan := func(s string) *plan.Node { return &plan.Node{Op: "Scan", Relation: s} }
	f := &plan.Predicate{Kind: "eq_const", Left: "a.status", Value: &value, ValueType: "string", Selectivity: .1}
	ab := &plan.Predicate{Kind: "eq_cols", Left: "a.bid", Right: "b.id", Selectivity: .01}
	bc := &plan.Predicate{Kind: "eq_cols", Left: "b.cid", Right: "c.id", Selectivity: .01}
	in := plan.Input{Version: 1, Catalog: []plan.Relation{
		{Name: "a", Rows: 1000, Columns: []plan.Column{{Name: "id", Type: "int"}, {Name: "bid", Type: "int"}, {Name: "status", Type: "string"}}},
		{Name: "b", Rows: 100, Columns: []plan.Column{{Name: "id", Type: "int"}, {Name: "cid", Type: "int"}}},
		{Name: "c", Rows: 10, Columns: []plan.Column{{Name: "id", Type: "int"}}},
	}, Plan: &plan.Node{Op: "Project", Columns: []string{"a.id", "b.id", "c.id"}, Input: &plan.Node{Op: "Join", Predicate: bc, Left: &plan.Node{Op: "Join", Predicate: ab, Left: &plan.Node{Op: "Filter", Predicate: f, Input: scan("a")}, Right: scan("b")}, Right: scan("c")}}}
	if _, err := plan.Validate(in); err != nil {
		t.Fatal(err)
	}
	candidates, err := Explore(context.Background(), in, eggbridge.Options{Executable: exe, WorkDir: t.TempDir(), MaxNodes: 3000}, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) < 2 {
		t.Fatalf("expected reordered plans, got %d", len(candidates))
	}
	for _, c := range candidates {
		if _, err := plan.ValidateNode(c, plan.CatalogMap(in)); err != nil {
			t.Fatalf("invalid candidate: %v", err)
		}
	}
	fixture := map[string][]row{
		"a": {{"a.id": "1", "a.bid": "10", "a.status": "open"}, {"a.id": "1", "a.bid": "10", "a.status": "open"}, {"a.id": "2", "a.bid": "20", "a.status": "closed"}},
		"b": {{"b.id": "10", "b.cid": "100"}, {"b.id": "20", "b.cid": "200"}},
		"c": {{"c.id": "100"}, {"c.id": "200"}},
	}
	want := bag(eval(in.Plan, fixture))
	for i, c := range candidates {
		if got := bag(eval(c, fixture)); !reflect.DeepEqual(got, want) {
			t.Fatalf("candidate %d changed bag: %v vs %v", i, got, want)
		}
	}
}

func TestGuardRejectsLostPredicate(t *testing.T) {
	p := &plan.Predicate{Kind: "eq_cols", Left: "a.id", Right: "b.id", Selectivity: .1}
	original := &plan.Node{Op: "Join", Predicate: p, Left: &plan.Node{Op: "Scan", Relation: "a"}, Right: &plan.Node{Op: "Scan", Relation: "b"}}
	candidate := &plan.Node{Op: "Join", Predicate: &plan.Predicate{Kind: "eq_cols", Left: "a.id", Right: "b.id", Selectivity: .2}, Left: &plan.Node{Op: "Scan", Relation: "a"}, Right: &plan.Node{Op: "Scan", Relation: "b"}}
	if sameSemantics(original, candidate) {
		t.Fatal("accepted modified join predicate")
	}
}

func TestReferenceAndProjectionGuards(t *testing.T) {
	catalog := map[string]plan.Relation{"a": {Name: "a", Rows: 10, Columns: []plan.Column{{Name: "id", Type: "int"}}}, "b": {Name: "b", Rows: 10, Columns: []plan.Column{{Name: "id", Type: "int"}, {Name: "other", Type: "int"}}}}
	scan := func(s string) *plan.Node { return &plan.Node{Op: "Scan", Relation: s} }
	cross := &plan.Node{Op: "Filter", Predicate: &plan.Predicate{Kind: "eq_cols", Left: "a.id", Right: "b.id", Selectivity: .1}, Input: &plan.Node{Op: "Join", Predicate: &plan.Predicate{Kind: "eq_cols", Left: "a.id", Right: "b.id", Selectivity: .1}, Left: scan("a"), Right: scan("b")}}
	if _, err := plan.ValidateNode(cross, catalog); err == nil {
		t.Fatal("accepted two-sided filter")
	}
	dropped := &plan.Node{Op: "Join", Predicate: &plan.Predicate{Kind: "eq_cols", Left: "a.id", Right: "b.id", Selectivity: .1}, Left: &plan.Node{Op: "Project", Columns: []string{"a.id"}, Input: scan("a")}, Right: &plan.Node{Op: "Project", Columns: []string{"b.id"}, Input: scan("b")}}
	dropped.Right.Columns = []string{"b.other"}
	if _, err := plan.ValidateNode(dropped, catalog); err == nil {
		t.Fatal("accepted projection that drops needed join column")
	}
}

type row map[string]string

func eval(n *plan.Node, data map[string][]row) []row {
	switch n.Op {
	case "Scan":
		return data[n.Relation]
	case "Filter":
		var out []row
		for _, r := range eval(n.Input, data) {
			if r[n.Predicate.Left] == *n.Predicate.Value {
				out = append(out, r)
			}
		}
		return out
	case "Project":
		var out []row
		for _, r := range eval(n.Input, data) {
			p := row{}
			for _, c := range n.Columns {
				p[c] = r[c]
			}
			out = append(out, p)
		}
		return out
	case "Join":
		var out []row
		for _, l := range eval(n.Left, data) {
			for _, r := range eval(n.Right, data) {
				m := row{}
				for k, v := range l {
					m[k] = v
				}
				for k, v := range r {
					m[k] = v
				}
				if m[n.Predicate.Left] == m[n.Predicate.Right] {
					out = append(out, m)
				}
			}
		}
		return out
	}
	panic("unsupported test operator")
}
func bag(rows []row) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		b, _ := json.Marshal(r)
		out[i] = string(b)
	}
	sort.Strings(out)
	return out
}
