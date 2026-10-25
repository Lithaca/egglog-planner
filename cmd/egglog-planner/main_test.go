package main

import (
	"bytes"
	"egglog-planner/internal/plan"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIJSONAndFailures(t *testing.T) {
	root := filepath.Join("..", "..")
	exe := testEgglogPath(root)
	if _, err := os.Stat(exe); err != nil {
		t.Skip("egglog 0.4.0 unavailable")
	}
	var out, errs bytes.Buffer
	code := run([]string{"optimize", filepath.Join(root, "examples", "two-tables.json"), "--json"}, &out, &errs)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errs.String())
	}
	if !strings.Contains(out.String(), `"chosen_index"`) || !strings.Contains(out.String(), `"assumptions"`) {
		t.Fatalf("missing report: %s", out.String())
	}
	var two report
	if err := json.Unmarshal(out.Bytes(), &two); err != nil {
		t.Fatal(err)
	}
	if two.Chosen.Input.Op != "Join" || two.Chosen.Input.Left.Op != "Filter" || two.Chosen.Input.Left.Predicate.Left != "orders.status" {
		t.Fatalf("filter was not pushed to orders: %+v", two.Chosen)
	}
	out.Reset()
	errs.Reset()
	code = run([]string{"optimize", filepath.Join(root, "examples", "three-tables.json"), "--json"}, &out, &errs)
	if code != 0 {
		t.Fatalf("three-table exit %d: %s", code, errs.String())
	}
	var three report
	if err := json.Unmarshal(out.Bytes(), &three); err != nil {
		t.Fatal(err)
	}
	if three.Chosen.Input.Op != "Join" || three.Chosen.Input.Predicate.Left != "a.bid" || !containsBCJoin(three.Chosen.Input) {
		t.Fatalf("b/c were not joined first: %+v", three.Chosen)
	}
	out.Reset()
	errs.Reset()
	code = run([]string{"optimize", "missing.json"}, &out, &errs)
	if code == 0 || errs.Len() == 0 {
		t.Fatal("missing input did not fail")
	}
}

func TestCLITextShowsOriginalAndSteps(t *testing.T) {
	root := filepath.Join("..", "..")
	if _, err := os.Stat(testEgglogPath(root)); err != nil {
		t.Skip("egglog 0.4.0 unavailable")
	}
	var out, errs bytes.Buffer
	code := run([]string{"optimize", filepath.Join(root, "examples", "two-tables.json")}, &out, &errs)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errs.String())
	}
	for _, want := range []string{"Original plan:", "Candidate 0 plan:", "Step 0:", "Estimated work:"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("text report missing %q", want)
		}
	}
}

func testEgglogPath(root string) string {
	if path := os.Getenv("EGGLOG_BIN"); path != "" {
		return path
	}
	return filepath.Join(root, ".tools", "bin", "egglog.exe")
}

func containsBCJoin(n *plan.Node) bool {
	if n == nil {
		return false
	}
	if n.Op == "Join" && n.Predicate.Left == "b.cid" && n.Predicate.Right == "c.id" {
		return true
	}
	return containsBCJoin(n.Left) || containsBCJoin(n.Right) || containsBCJoin(n.Input)
}
