package plan

import (
	"strings"
	"testing"
)

const validJSON = `{"version":1,"catalog":[{"name":"orders","rows":1000,"columns":[{"name":"id","type":"int"},{"name":"customer_id","type":"int"},{"name":"status","type":"string"}]},{"name":"customers","rows":100,"columns":[{"name":"id","type":"int"},{"name":"name","type":"string"}]}],"plan":{"op":"Project","columns":["orders.id","customers.name"],"input":{"op":"Join","predicate":{"kind":"eq_cols","left":"orders.customer_id","right":"customers.id","selectivity":0.01},"left":{"op":"Filter","predicate":{"kind":"eq_const","left":"orders.status","value":"open","value_type":"string","selectivity":0.1},"input":{"op":"Scan","relation":"orders"}},"right":{"op":"Scan","relation":"customers"}}}}`

func TestParseAndRoundTrip(t *testing.T) {
	in, err := Parse([]byte(validJSON))
	if err != nil {
		t.Fatal(err)
	}
	cols, err := Validate(in)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(cols, ",") != "orders.id,customers.name" {
		t.Fatalf("columns %v", cols)
	}
	out, err := Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Parse(out); err != nil {
		t.Fatal(err)
	}
}

func TestRejectInvalidPlans(t *testing.T) {
	cases := map[string]string{
		"unknown column":          strings.Replace(validJSON, "orders.customer_id", "orders.missing", 1),
		"duplicate relation":      strings.Replace(validJSON, `"relation":"customers"`, `"relation":"orders"`, 1),
		"bad equality types":      strings.Replace(validJSON, "customers.id", "customers.name", 1),
		"unsupported operator":    strings.Replace(validJSON, `"op":"Join"`, `"op":"LeftJoin"`, 1),
		"missing statistics":      strings.Replace(validJSON, `"selectivity":0.1`, `"selectivity":0`, 1),
		"unknown field":           strings.Replace(validJSON, `"version":1`, `"version":1,"sql":"select *"`, 1),
		"invalid integer literal": strings.Replace(validJSON, `"left":"orders.status","value":"open","value_type":"string"`, `"left":"orders.id","value":"not-an-integer","value_type":"int"`, 1),
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse([]byte(raw)); err == nil {
				t.Fatal("accepted invalid plan")
			}
		})
	}
}
