package rewrite

import (
	"context"
	"egglog-planner/internal/eggbridge"
	"egglog-planner/internal/plan"
	"egglog-planner/rules"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

type encoding struct {
	predicates map[string]*plan.Predicate
	projects   map[string][]string
	next       int
}

func (e *encoding) id(prefix string) string { e.next++; return fmt.Sprintf("%s%d", prefix, e.next) }
func (e *encoding) term(n *plan.Node) string {
	switch n.Op {
	case "Scan":
		return "(Scan " + strconv.Quote(n.Relation) + ")"
	case "Filter":
		id := e.id("f")
		e.predicates[id] = n.Predicate
		return "(Filter " + strconv.Quote(id) + " " + e.term(n.Input) + ")"
	case "Project":
		id := e.id("p")
		e.projects[id] = n.Columns
		return "(Project " + strconv.Quote(id) + " " + e.term(n.Input) + ")"
	case "Join":
		id := e.id("j")
		e.predicates[id] = n.Predicate
		return "(Join " + strconv.Quote(id) + " " + e.term(n.Left) + " " + e.term(n.Right) + ")"
	}
	panic("validated plan has unsupported operator")
}

// Explore uses egglog's e-graph as the source of alternatives. Every term is
// checked again because the broad algebraic rules can produce ill-typed terms.
func Explore(ctx context.Context, in plan.Input, engine eggbridge.Options, maxCandidates int) ([]*plan.Node, error) {
	originalCols, err := plan.Validate(in)
	if err != nil {
		return nil, err
	}
	if maxCandidates <= 0 {
		maxCandidates = 200
	}
	e := encoding{predicates: map[string]*plan.Predicate{}, projects: map[string][]string{}}
	program := rules.Source + "\n(let target (Root " + e.term(in.Plan) + "))\n(run 2)\n(extract target)\n"
	graph, err := eggbridge.Run(ctx, engine, program)
	if err != nil {
		return nil, err
	}
	groups := map[string][]string{}
	rootClass := ""
	for id, n := range graph.Nodes {
		if n.Subsumed {
			continue
		}
		groups[n.EClass] = append(groups[n.EClass], id)
		if n.Op == "Root" {
			if len(n.Children) != 1 {
				return nil, fmt.Errorf("egglog: malformed Root")
			}
			c := graph.Nodes[n.Children[0]].EClass
			if rootClass != "" && rootClass != c {
				return nil, fmt.Errorf("egglog: ambiguous Root")
			}
			rootClass = c
		}
	}
	if rootClass == "" {
		return nil, fmt.Errorf("egglog: Root missing from graph")
	}
	for _, ids := range groups {
		sort.Strings(ids)
	}
	d := decoder{graph: graph, groups: groups, meta: e, limit: maxCandidates * 20}
	candidates, err := d.class(rootClass, map[string]bool{}, 0)
	if err != nil {
		return nil, err
	}
	result := []*plan.Node{}
	unique := map[string]bool{}
	for _, n := range candidates {
		cols, err := plan.ValidateNode(n, plan.CatalogMap(in))
		if err != nil || strings.Join(cols, "\x00") != strings.Join(originalCols, "\x00") || !sameSemantics(in.Plan, n) {
			continue
		}
		data, _ := json.Marshal(n)
		key := string(data)
		if !unique[key] {
			unique[key] = true
			result = append(result, n)
		}
		if len(result) > maxCandidates {
			return nil, fmt.Errorf("egglog: candidate limit %d exceeded", maxCandidates)
		}
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("egglog: no valid candidates")
	}
	return result, nil
}

type decoder struct {
	graph  eggbridge.Graph
	groups map[string][]string
	meta   encoding
	limit  int
	built  int
}

func (d *decoder) stringNode(id string) (string, error) {
	n, ok := d.graph.Nodes[id]
	if !ok || len(n.Children) != 0 {
		return "", fmt.Errorf("egglog: malformed string node")
	}
	s, err := strconv.Unquote(n.Op)
	if err != nil {
		return "", fmt.Errorf("egglog: malformed string %q", n.Op)
	}
	return s, nil
}
func (d *decoder) class(class string, stack map[string]bool, depth int) ([]*plan.Node, error) {
	if depth > 12 {
		return nil, fmt.Errorf("egglog: plan depth limit exceeded")
	}
	if stack[class] {
		return nil, nil
	}
	stack[class] = true
	defer delete(stack, class)
	out := []*plan.Node{}
	for _, id := range d.groups[class] {
		n := d.graph.Nodes[id]
		var built []*plan.Node
		switch n.Op {
		case "Scan":
			if len(n.Children) != 1 {
				return nil, fmt.Errorf("egglog: malformed Scan")
			}
			s, err := d.stringNode(n.Children[0])
			if err != nil {
				return nil, err
			}
			built = []*plan.Node{{Op: "Scan", Relation: s}}
		case "Filter", "Project":
			if len(n.Children) != 2 {
				return nil, fmt.Errorf("egglog: malformed %s", n.Op)
			}
			key, err := d.stringNode(n.Children[0])
			if err != nil {
				return nil, err
			}
			children, err := d.class(d.graph.Nodes[n.Children[1]].EClass, stack, depth+1)
			if err != nil {
				return nil, err
			}
			for _, child := range children {
				if n.Op == "Filter" {
					p, ok := d.meta.predicates[key]
					if !ok {
						return nil, fmt.Errorf("egglog: unknown filter %q", key)
					}
					built = append(built, &plan.Node{Op: "Filter", Predicate: p, Input: child})
				} else {
					cols, ok := d.meta.projects[key]
					if !ok {
						return nil, fmt.Errorf("egglog: unknown project %q", key)
					}
					built = append(built, &plan.Node{Op: "Project", Columns: cols, Input: child})
				}
			}
		case "Join":
			if len(n.Children) != 3 {
				return nil, fmt.Errorf("egglog: malformed Join")
			}
			key, err := d.stringNode(n.Children[0])
			if err != nil {
				return nil, err
			}
			p, ok := d.meta.predicates[key]
			if !ok {
				return nil, fmt.Errorf("egglog: unknown join %q", key)
			}
			left, err := d.class(d.graph.Nodes[n.Children[1]].EClass, stack, depth+1)
			if err != nil {
				return nil, err
			}
			right, err := d.class(d.graph.Nodes[n.Children[2]].EClass, stack, depth+1)
			if err != nil {
				return nil, err
			}
			for _, l := range left {
				for _, r := range right {
					built = append(built, &plan.Node{Op: "Join", Predicate: p, Left: l, Right: r})
					if len(built) > d.limit {
						return nil, fmt.Errorf("egglog: enumeration limit exceeded")
					}
				}
			}
		default:
			continue
		}
		d.built += len(built)
		if d.built > d.limit*20 {
			return nil, fmt.Errorf("egglog: enumeration limit exceeded")
		}
		out = append(out, built...)
		if len(out) > d.limit {
			return nil, fmt.Errorf("egglog: enumeration limit exceeded")
		}
	}
	return out, nil
}

func sameSemantics(a, b *plan.Node) bool {
	count := func(n *plan.Node) map[string]int {
		m := map[string]int{}
		var walk func(*plan.Node)
		walk = func(x *plan.Node) {
			if x == nil {
				return
			}
			switch x.Op {
			case "Scan":
				m["scan:"+x.Relation]++
			case "Filter", "Join":
				raw, _ := json.Marshal(x.Predicate)
				m[x.Op+":"+string(raw)]++
			case "Project":
				raw, _ := json.Marshal(x.Columns)
				m["project:"+string(raw)]++
			}
			walk(x.Input)
			walk(x.Left)
			walk(x.Right)
		}
		walk(n)
		return m
	}
	x, y := count(a), count(b)
	if len(x) != len(y) {
		return false
	}
	for k, v := range x {
		if y[k] != v {
			return false
		}
	}
	return true
}
