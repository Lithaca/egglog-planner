package eggbridge

import (
	"context"
	"egglog-planner/internal/cost"
	"egglog-planner/internal/plan"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRealEgglogEnumeratesEquivalentPlans(t *testing.T) {
	exe := os.Getenv("EGGLOG_BIN")
	if exe == "" {
		exe = filepath.Join("..", "..", ".tools", "bin", "egglog.exe")
	}
	if _, err := os.Stat(exe); err != nil {
		t.Skip("set EGGLOG_BIN to egglog 0.4.0")
	}
	program := `(datatype Plan (Scan String) (Join Plan Plan))
(constructor Root (Plan) Plan)
(let p (Root (Join (Scan "big") (Scan "small"))))
(rewrite (Join a b) (Join b a))
(run 2)
(extract p)
`
	graph, err := Run(context.Background(), Options{Executable: exe, WorkDir: t.TempDir(), Timeout: 5 * time.Second, MaxBytes: 1 << 20}, program)
	if err != nil {
		t.Fatal(err)
	}
	if len(graph.Nodes) < 7 {
		t.Fatalf("expected e-graph alternatives, got %d nodes", len(graph.Nodes))
	}
	var joined int
	for _, n := range graph.Nodes {
		if n.Op == "Join" {
			joined++
		}
	}
	if joined != 2 {
		t.Fatalf("expected both join orders, got %d", joined)
	}
	_, err = Run(context.Background(), Options{Executable: exe, WorkDir: t.TempDir(), Timeout: time.Nanosecond}, program)
	if err == nil || !strings.Contains(err.Error(), "deadline exceeded") {
		t.Fatalf("timeout: %v", err)
	}
}

func TestRealEgglogKeepsPlansWithDifferentExecutionEstimates(t *testing.T) {
	exe := os.Getenv("EGGLOG_BIN")
	if exe == "" {
		exe = filepath.Join("..", "..", ".tools", "bin", "egglog.exe")
	}
	if _, err := os.Stat(exe); err != nil {
		t.Skip("set EGGLOG_BIN to egglog 0.4.0")
	}
	program := `(datatype Plan (Scan String) (Filter String Plan) (Join String Plan Plan))
(constructor Root (Plan) Plan)
(let p (Root (Filter "f" (Join "j" (Scan "big") (Scan "small")))))
(rewrite (Filter f (Join j a b)) (Join j (Filter f a) b))
(run 1)
(extract p)
`
	graph, err := Run(context.Background(), Options{Executable: exe, WorkDir: t.TempDir(), Timeout: 5 * time.Second}, program)
	if err != nil {
		t.Fatal(err)
	}
	var rootClass string
	for _, n := range graph.Nodes {
		if n.Op == "Root" {
			rootClass = graph.Nodes[n.Children[0]].EClass
		}
	}
	var hasFilter, hasJoin bool
	for _, n := range graph.Nodes {
		if n.EClass == rootClass && n.Op == "Filter" {
			hasFilter = true
		}
		if n.EClass == rootClass && n.Op == "Join" {
			hasJoin = true
		}
	}
	if !hasFilter || !hasJoin {
		t.Fatalf("same e-class lacks late and pushed plans: filter=%v join=%v", hasFilter, hasJoin)
	}
	v := "open"
	f := &plan.Predicate{Kind: "eq_const", Left: "big.status", Value: &v, ValueType: "string", Selectivity: .01}
	j := &plan.Predicate{Kind: "eq_cols", Left: "big.id", Right: "small.id", Selectivity: .01}
	scan := func(name string) *plan.Node { return &plan.Node{Op: "Scan", Relation: name} }
	late := &plan.Node{Op: "Filter", Predicate: f, Input: &plan.Node{Op: "Join", Predicate: j, Left: scan("big"), Right: scan("small")}}
	early := &plan.Node{Op: "Join", Predicate: j, Left: &plan.Node{Op: "Filter", Predicate: f, Input: scan("big")}, Right: scan("small")}
	catalog := map[string]plan.Relation{"big": {Name: "big", Rows: 1000, Columns: []plan.Column{{Name: "id", Type: "int"}, {Name: "status", Type: "string"}}}, "small": {Name: "small", Rows: 100, Columns: []plan.Column{{Name: "id", Type: "int"}}}}
	a, err := cost.Estimate(late, catalog)
	if err != nil {
		t.Fatal(err)
	}
	b, err := cost.Estimate(early, catalog)
	if err != nil {
		t.Fatal(err)
	}
	if a.Total != 103100 || b.Total != 3110 {
		t.Fatalf("unexpected work estimates: late=%v early=%v", a.Total, b.Total)
	}
}

func TestEngineFailures(t *testing.T) {
	_, err := Run(context.Background(), Options{Executable: "does-not-exist", WorkDir: t.TempDir(), Timeout: time.Second}, "")
	if err == nil || !strings.Contains(err.Error(), "egglog") {
		t.Fatalf("missing engine: %v", err)
	}
	_, err = ParseGraph([]byte(`{"nodes":{}}`), 100)
	if err == nil {
		t.Fatal("accepted empty graph")
	}
	_, err = ParseGraph([]byte(`{"nodes":{"x":{"op":"Scan","eclass":"p","children":[]}}} garbage`), 100)
	if err == nil {
		t.Fatal("accepted malformed trailing graph data")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = Run(ctx, Options{Executable: "does-not-exist", WorkDir: t.TempDir(), Timeout: time.Second}, "")
	if err == nil || !strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("canceled: %v", err)
	}
}

func TestCappedProcessOutput(t *testing.T) {
	b := newCappedOutput(4)
	if _, err := b.Write([]byte("123")); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Write([]byte("456789")); err != nil {
		t.Fatal(err)
	}
	if b.String() != "1234" || !b.Exceeded() {
		t.Fatalf("buffer = %q, exceeded = %v", b.String(), b.Exceeded())
	}
}
