package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/lazysnail1024/k8s-crd-toolkit/yamlvalue"
	"github.com/lazysnail1024/k8s-crd-toolkit/yamlview"
)

func yamlValueKeyCases() {
	ctx := context.Background()
	options := yamlvalue.Options{MaxNodes: 100000}
	for _, test := range []struct{ name, text, want string }{
		{"ordinary", "apiVersion: value\nname: value\nfalsePositive: value\nKey_2-tail: value\n", `{"Key_2-tail":"value","apiVersion":"value","falsePositive":"value","name":"value"}`},
		{"mixed-case-strings", "oN: value\nyEs: value\nnUlL: value\n", `{"nUlL":"value","oN":"value","yEs":"value"}`},
		{"date", "2001-12-14: value\n", `{"2001-12-14":"value"}`},
		{"punctuation", "path/to.key: value\n_under: value\n? ! plain\n: value\n", `{"_under":"value","path/to.key":"value","plain":"value"}`},
		{"unicode", "caf\u00e9: value\n\U0001f600: value\n", "{\"caf\u00e9\":\"value\",\"\U0001f600\":\"value\"}"},
		{"quoted", "'yes': value\n\"on\": value\n'42': value\n", `{"42":"value","on":"value","yes":"value"}`},
		{"explicit-string", "!!str yes: value\n", `{"yes":"value"}`},
		{"explicit-binary", "!!binary a2V5: value\n", `{"key":"value"}`},
		{"alias-key", "? &k actual-key\n: first\nother:\n  ? *k\n  : second\n", `{"actual-key":"first","other":{"actual-key":"second"}}`},
	} {
		snapshot := snap(test.text)
		view, err := yamlview.Parse(ctx, snapshot)
		ok := err == nil && len(view.Documents()) == 1 && view.Documents()[0].Complete()
		if ok {
			result, err := yamlvalue.Project(ctx, snapshot, view.Documents()[0], options)
			actual, jsonErr := json.Marshal(result.Value)
			ok = err == nil && jsonErr == nil && string(actual) == test.want
			if test.name == "alias-key" && ok {
				origin := result.Origin.At([]string{"other", "actual-key"})
				ok = origin.Kind == "derived" && origin.Node.Raw() == "*k" && origin.Key.Raw() == "*k"
			}
		}
		check("mapping-key-value-"+test.name, ok)
	}
	for _, key := range []string{
		"y", "Y", "yes", "Yes", "YES", "n", "N", "no", "No", "NO",
		"on", "On", "ON", "off", "Off", "OFF", "true", "True", "TRUE",
		"false", "False", "FALSE", "null", "Null", "NULL", "~", "42", "0x20", ".nan",
		"! on", "!!bool true", "!custom actual-key",
	} {
		snapshot := snap(key + ": value\n")
		view, err := yamlview.Parse(ctx, snapshot)
		ok := err == nil && len(view.Documents()) == 1 && view.Documents()[0].Complete()
		if ok {
			result, err := yamlvalue.Project(ctx, snapshot, view.Documents()[0], options)
			failure, typed := err.(*yamlvalue.Error)
			ok = typed && failure.Code == "YAML_PROJECTION_INCOMPLETE" && result.Value == nil
		}
		check("mapping-key-reject-"+key, ok)
	}
	for _, test := range []struct{ name, text string }{
		{"mixed-quote", "actual-key: first\n\"actual-key\": second\n"},
		{"binary", "actual-key: first\n!!binary YWN0dWFsLWtleQ==: second\n"},
		{"alias", "? &k actual-key\n: first\n? *k\n: second\n"},
	} {
		snapshot := snap(test.text)
		view, err := yamlview.Parse(ctx, snapshot)
		ok := err == nil && len(view.Documents()) == 1 && view.Documents()[0].Complete()
		if ok {
			result, err := yamlvalue.Project(ctx, snapshot, view.Documents()[0], options)
			failure, typed := err.(*yamlvalue.Error)
			ok = typed && failure.Code == "YAML_PROJECTION_INCOMPLETE" && result.Value == nil
		}
		check("mapping-key-duplicate-"+test.name, ok)
	}

	// Exclude CST construction and compare projection of many small records.
	// Each record has distinct ordinary names and fixed independently known values.
	const records = 500
	const row = "- {name: example, source: input, metadata: label, enabled: true, count: 2}\n"
	snapshot := snap(strings.Repeat(row, records))
	view, err := yamlview.Parse(ctx, snapshot)
	ok := err == nil && len(view.Documents()) == 1 && view.Documents()[0].Complete()
	check("mapping-key-allocation-input", ok)
	if !ok {
		return
	}
	doc := view.Documents()[0]
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	result, err := yamlvalue.Project(ctx, snapshot, doc, options)
	runtime.ReadMemStats(&after)
	rows, ok := result.Value.([]any)
	ok = ok && err == nil && len(rows) == records && result.Origin == nil
	for _, row := range rows {
		m, isMap := row.(map[string]any)
		ok = ok && isMap && len(m) == 5 && m["name"] == "example" && m["source"] == "input" && m["metadata"] == "label" && m["enabled"] == true && m["count"] == json.Number("2")
	}
	check("mapping-key-allocation-values", ok)
	allocated := after.TotalAlloc - before.TotalAlloc
	check("mapping-key-allocation-budget", allocated < 12*1024*1024)
	fmt.Fprintf(os.Stderr, "ordinary mapping projection: %d allocated bytes for %d records\n", allocated, records)
}
