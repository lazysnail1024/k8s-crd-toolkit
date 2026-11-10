package main

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sort"
	"strings"

	"github.com/lazysnail1024/k8s-crd-toolkit/schemaquery"
	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
)

// This consumer uses a synthetic, non-Argo CRD. Its expectations describe the
// editor contract, independently of either plugin's release catalog or helpers.
func schemaQueryCases() {
	const fixture = `{
      "$schema":"https://json-schema.org/draft/2020-12/schema",
      "type":"object",
      "properties":{
        "apiVersion":{"const":"delivery.example.test/v1"},
        "kind":{"const":"DeliveryPolicy"},
        "spec":{"$ref":"#/$defs/policy"},
        "payload":{"type":"object","x-kubernetes-preserve-unknown-fields":true,"properties":{"enabled":{"type":"boolean"}}},
        "tuple":{"type":"array","prefixItems":[{"enum":["header"]}],"items":{"enum":["tail"]}},
        "channel":{"allOf":[{"enum":["stable","unsafe","alpha"]},{"pattern":"^(stable|alpha)$"}]},
        "count":{"allOf":[{"type":"number"},{"type":"integer"}]}
      },
      "$defs":{"policy":{"oneOf":[
        {"type":"object","properties":{"mode":{"const":"http"},"endpoint":{"type":"string"},"scheme":{"enum":["http","https"]}},"required":["mode","endpoint"],"additionalProperties":false},
        {"type":"object","properties":{"mode":{"const":"queue"},"queueName":{"type":"string"},"scheme":{"enum":["amqp"]}},"required":["mode","queueName"],"additionalProperties":false}
      ]}}
    }`
	compile := func(raw string) *jsonschema.Schema {
		var value any
		if err := json.Unmarshal([]byte(raw), &value); err != nil {
			panic(err)
		}
		c := jsonschema.NewCompiler()
		if err := c.AddResource("urn:toolkit:delivery-policy", value); err != nil {
			panic(err)
		}
		s, err := c.Compile("urn:toolkit:delivery-policy")
		if err != nil {
			panic(err)
		}
		return s
	}
	root := compile(fixture)
	ctx := context.Background()
	value := map[string]any{"apiVersion": "delivery.example.test/v1", "kind": "DeliveryPolicy", "spec": map[string]any{"mode": "http", "endpoint": 12}}
	path := []string{"spec"}
	query, err := schemaquery.Lookup(ctx, root, path, value, schemaquery.Limits{})
	names := map[string]bool{}
	for _, s := range query.Nodes {
		for name := range s.Properties {
			names[name] = true
		}
	}
	check("schemaquery-non-argo-ref-and-finite-sibling", err == nil && query.Complete && names["endpoint"] && names["scheme"] && !names["queueName"])
	check("schemaquery-ordinary-invalid-sibling-keeps-candidates", err == nil && len(names) == 3)
	allowed, err := schemaquery.AllowsKey(ctx, root, []string{"spec", "queueName"}, value, schemaquery.Limits{})
	check("schemaquery-enclosing-closure", err == nil && !allowed)
	value["spec"].(map[string]any)["scheme"] = "unfinished"
	values, err := schemaquery.Values(ctx, root, []string{"spec", "scheme"}, value, schemaquery.Limits{})
	check("schemaquery-active-value-excluded", err == nil && reflect.DeepEqual(values, []any{"http", "https"}))

	values, err = schemaquery.Values(ctx, root, []string{"channel"}, value, schemaquery.Limits{})
	check("schemaquery-leaf-validation", err == nil && reflect.DeepEqual(values, []any{"alpha", "stable"}))
	kind, err := schemaquery.Type(ctx, root, []string{"count"}, value, schemaquery.Limits{})
	check("schemaquery-type-intersection", err == nil && kind == "integer")
	allowed, err = schemaquery.AllowsKey(ctx, root, []string{"payload", "unknown"}, value, schemaquery.Limits{})
	check("schemaquery-preserved-open-object", err == nil && allowed)
	tupleValue := map[string]any{"tuple": []any{"bad", "bad"}}
	first, e1 := schemaquery.Values(ctx, root, []string{"tuple", "0"}, tupleValue, schemaquery.Limits{})
	rest, e2 := schemaquery.Values(ctx, root, []string{"tuple", "1"}, tupleValue, schemaquery.Limits{})
	check("schemaquery-prefix-and-tail-items", e1 == nil && e2 == nil && reflect.DeepEqual(first, []any{"header"}) && reflect.DeepEqual(rest, []any{"tail"}))
	duplicate := compile(`{"type":"object","oneOf":[{"properties":{"value":{"enum":["same"]}}},{"properties":{"value":{"enum":["same"]}}}]}`)
	values, err = schemaquery.Values(ctx, duplicate, []string{"value"}, map[string]any{"value": "broken"}, schemaquery.Limits{})
	check("schemaquery-oneof-cardinality", err == nil && len(values) == 0)
	query, err = schemaquery.Lookup(ctx, root, []string{"spec"}, map[string]any{"spec": map[string]any{}}, schemaquery.Limits{})
	check("schemaquery-incomplete-input-ambiguous", err == nil && query.Complete && query.Ambiguous)
	query, err = schemaquery.Lookup(ctx, root, []string{"spec", "scheme"}, value, schemaquery.Limits{Work: 1})
	check("schemaquery-budget-not-empty-success", errors.Is(err, schemaquery.ErrBudget) && !query.Complete && len(query.Nodes) == 0)
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	query, err = schemaquery.Lookup(cancelled, root, nil, value, schemaquery.Limits{})
	check("schemaquery-cancellation-not-empty-success", errors.Is(err, context.Canceled) && !query.Complete && len(query.Nodes) == 0)
	query, err = schemaquery.Lookup(ctx, root, []string{"spec", "scheme"}, value, schemaquery.Limits{Depth: 1})
	check("schemaquery-depth-not-empty-success", errors.Is(err, schemaquery.ErrBudget) && !query.Complete)
	branches := compile(`{"anyOf":[{"type":"object","properties":{"left":{"type":"string"}},"additionalProperties":false},{"type":"object","properties":{"right":{"type":"string"}},"additionalProperties":false}]}`)
	before := append([]*jsonschema.Schema(nil), branches.AnyOf...)
	selected, err := schemaquery.SelectShapes(ctx, branches.AnyOf, map[string]any{"left": nil}, []string{"unknown"}, schemaquery.Limits{})
	check("schemaquery-shape-selection-is-immutable", err == nil && len(selected) == 1 && selected[0] == before[0] && reflect.DeepEqual(branches.AnyOf, before))
	selected, err = schemaquery.SelectShapes(ctx, branches.AnyOf, map[string]any{"left": nil}, []string{"left"}, schemaquery.Limits{})
	check("schemaquery-shape-excludes-active-field", err == nil && len(selected) == 2)
	selected, err = schemaquery.SelectShapes(ctx, branches.AnyOf, map[string]any{"neither": nil}, nil, schemaquery.Limits{})
	check("schemaquery-empty-selection-is-caller-policy", err == nil && len(selected) == 0)
	conditional := compile(`{"type":"object","properties":{"mode":{"enum":["on","off"]}},"if":{"properties":{"mode":{"const":"on"}}},"then":{"properties":{"extra":{"type":"string"}}}}`)
	query, err = schemaquery.Lookup(ctx, conditional, nil, map[string]any{}, schemaquery.Limits{})
	check("schemaquery-unsupported-is-explicit", errors.Is(err, schemaquery.ErrUnsupported) && !query.Complete && len(query.Nodes) == 0)
	legacy := compile(`{"$schema":"http://json-schema.org/draft-07/schema#","type":"array","items":[{"enum":["first"]}],"additionalItems":{"enum":["rest"]}}`)
	first, e1 = schemaquery.Values(ctx, legacy, []string{"0"}, []any{"bad", "bad"}, schemaquery.Limits{})
	rest, e2 = schemaquery.Values(ctx, legacy, []string{"1"}, []any{"bad", "bad"}, schemaquery.Limits{})
	check("schemaquery-legacy-tuple", e1 == nil && e2 == nil && reflect.DeepEqual(first, []any{"first"}) && reflect.DeepEqual(rest, []any{"rest"}))
	// Candidate ordering is stable across map iteration and query calls.
	values, err = schemaquery.Values(ctx, root, []string{"channel"}, nil, schemaquery.Limits{})
	ordered := []string{}
	for _, v := range values {
		ordered = append(ordered, v.(string))
	}
	check("schemaquery-stable-order", err == nil && sort.StringsAreSorted(ordered) && strings.Join(ordered, ",") == "alpha,stable")
}
