package plan

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
)

type Column struct {
	Name string `json:"name"`
	Type string `json:"type"`
}
type Relation struct {
	Name    string   `json:"name"`
	Rows    float64  `json:"rows"`
	Columns []Column `json:"columns"`
}
type Predicate struct {
	Kind        string  `json:"kind"`
	Left        string  `json:"left"`
	Right       string  `json:"right,omitempty"`
	Value       *string `json:"value,omitempty"`
	ValueType   string  `json:"value_type,omitempty"`
	Selectivity float64 `json:"selectivity"`
}
type Node struct {
	Op        string     `json:"op"`
	Relation  string     `json:"relation,omitempty"`
	Predicate *Predicate `json:"predicate,omitempty"`
	Columns   []string   `json:"columns,omitempty"`
	Input     *Node      `json:"input,omitempty"`
	Left      *Node      `json:"left,omitempty"`
	Right     *Node      `json:"right,omitempty"`
}
type Input struct {
	Version int        `json:"version"`
	Catalog []Relation `json:"catalog"`
	Plan    *Node      `json:"plan"`
}

func Parse(data []byte) (Input, error) {
	var in Input
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		return in, fmt.Errorf("input: %w", err)
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		return in, fmt.Errorf("input: trailing data")
	}
	if _, err := Validate(in); err != nil {
		return in, err
	}
	return in, nil
}
func Marshal(in Input) ([]byte, error) {
	if _, err := Validate(in); err != nil {
		return nil, err
	}
	return json.MarshalIndent(in, "", "  ")
}

func Validate(in Input) ([]string, error) {
	if in.Version != 1 {
		return nil, fmt.Errorf("input: unsupported version %d", in.Version)
	}
	if len(in.Catalog) == 0 {
		return nil, fmt.Errorf("input: empty catalog")
	}
	catalog := make(map[string]Relation, len(in.Catalog))
	for _, r := range in.Catalog {
		if r.Name == "" || strings.Contains(r.Name, ".") {
			return nil, fmt.Errorf("catalog: invalid relation name %q", r.Name)
		}
		if _, ok := catalog[r.Name]; ok {
			return nil, fmt.Errorf("catalog: duplicate relation %q", r.Name)
		}
		if !positive(r.Rows) || len(r.Columns) == 0 {
			return nil, fmt.Errorf("catalog: missing or invalid rows/columns for %q", r.Name)
		}
		names := map[string]bool{}
		for _, c := range r.Columns {
			if c.Name == "" || strings.Contains(c.Name, ".") || names[c.Name] || (c.Type != "int" && c.Type != "string") {
				return nil, fmt.Errorf("catalog: invalid column %q.%q", r.Name, c.Name)
			}
			names[c.Name] = true
		}
		catalog[r.Name] = r
	}
	return ValidateNode(in.Plan, catalog)
}
func positive(v float64) bool { return v > 0 && !math.IsNaN(v) && !math.IsInf(v, 0) }
func validSel(v float64) bool { return positive(v) && v <= 1 }

// ValidateNode returns root columns in order. The catalog is indexed by relation name.
func ValidateNode(root *Node, catalog map[string]Relation) ([]string, error) {
	seen := map[string]bool{}
	cols, err := validate(root, catalog, seen, 0)
	if err != nil {
		return nil, err
	}
	out := make([]string, len(cols))
	for i, c := range cols {
		out[i] = c.name
	}
	return out, nil
}

type typedColumn struct{ name, typ string }

func validate(n *Node, catalog map[string]Relation, seen map[string]bool, depth int) ([]typedColumn, error) {
	if n == nil || depth > 64 {
		return nil, fmt.Errorf("plan: missing or too-deep node")
	}
	switch n.Op {
	case "Scan":
		if n.Input != nil || n.Left != nil || n.Right != nil || n.Predicate != nil || len(n.Columns) > 0 {
			return nil, fmt.Errorf("Scan: unexpected fields")
		}
		r, ok := catalog[n.Relation]
		if !ok {
			return nil, fmt.Errorf("Scan: unknown relation %q", n.Relation)
		}
		if seen[r.Name] {
			return nil, fmt.Errorf("Scan: duplicate base relation %q", r.Name)
		}
		seen[r.Name] = true
		cols := make([]typedColumn, len(r.Columns))
		for i, c := range r.Columns {
			cols[i] = typedColumn{r.Name + "." + c.Name, c.Type}
		}
		return cols, nil
	case "Filter":
		if n.Relation != "" || n.Left != nil || n.Right != nil || len(n.Columns) > 0 || n.Predicate == nil {
			return nil, fmt.Errorf("Filter: invalid fields")
		}
		cols, err := validate(n.Input, catalog, seen, depth+1)
		if err != nil {
			return nil, err
		}
		p := n.Predicate
		if p.Kind != "eq_const" || p.Right != "" || p.Value == nil || !validSel(p.Selectivity) {
			return nil, fmt.Errorf("Filter: invalid equality or selectivity")
		}
		typ, ok := columnType(cols, p.Left)
		if !ok {
			return nil, fmt.Errorf("Filter: unknown column %q", p.Left)
		}
		if typ != p.ValueType {
			return nil, fmt.Errorf("Filter: incompatible equality type %q", p.Left)
		}
		if typ == "int" {
			if _, err := strconv.ParseInt(*p.Value, 10, 64); err != nil {
				return nil, fmt.Errorf("Filter: invalid signed 64-bit integer literal %q", *p.Value)
			}
		}
		return cols, nil
	case "Project":
		if n.Relation != "" || n.Left != nil || n.Right != nil || n.Predicate != nil || len(n.Columns) == 0 {
			return nil, fmt.Errorf("Project: invalid fields")
		}
		cols, err := validate(n.Input, catalog, seen, depth+1)
		if err != nil {
			return nil, err
		}
		out := make([]typedColumn, 0, len(n.Columns))
		used := map[string]bool{}
		for _, name := range n.Columns {
			typ, ok := columnType(cols, name)
			if !ok || used[name] {
				return nil, fmt.Errorf("Project: unknown or duplicate column %q", name)
			}
			used[name] = true
			out = append(out, typedColumn{name, typ})
		}
		return out, nil
	case "Join":
		if n.Relation != "" || n.Input != nil || len(n.Columns) > 0 || n.Predicate == nil {
			return nil, fmt.Errorf("Join: invalid fields")
		}
		left, err := validate(n.Left, catalog, seen, depth+1)
		if err != nil {
			return nil, err
		}
		right, err := validate(n.Right, catalog, seen, depth+1)
		if err != nil {
			return nil, err
		}
		p := n.Predicate
		if p.Kind != "eq_cols" || p.Value != nil || p.ValueType != "" || !validSel(p.Selectivity) {
			return nil, fmt.Errorf("Join: invalid equality or selectivity")
		}
		lType, lOK := columnType(left, p.Left)
		rType, rOK := columnType(right, p.Right)
		if !lOK || !rOK {
			lType, lOK = columnType(left, p.Right)
			rType, rOK = columnType(right, p.Left)
		}
		if !lOK || !rOK || lType != rType {
			return nil, fmt.Errorf("Join: condition must connect compatible columns on opposite sides")
		}
		return append(left, right...), nil
	default:
		return nil, fmt.Errorf("plan: unsupported operator %q", n.Op)
	}
}
func columnType(cols []typedColumn, name string) (string, bool) {
	for _, c := range cols {
		if c.name == name {
			return c.typ, true
		}
	}
	return "", false
}
func CatalogMap(in Input) map[string]Relation {
	m := make(map[string]Relation, len(in.Catalog))
	for _, r := range in.Catalog {
		m[r.Name] = r
	}
	return m
}
