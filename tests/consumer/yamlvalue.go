package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/lazysnail1024/k8s-crd-toolkit/source"
	"github.com/lazysnail1024/k8s-crd-toolkit/yamlvalue"
	"github.com/lazysnail1024/k8s-crd-toolkit/yamlview"
)

func yamlValueCases() {
	yamlValueCommentCases()
	ctx := context.Background()
	options := yamlvalue.Options{MaxNodes: 100000, Fields: []string{"apiVersion", "kind"}}
	project := func(text string, options yamlvalue.Options) (source.Snapshot, yamlvalue.Result, error) {
		snapshot := snap(text)
		view, err := yamlview.Parse(ctx, snapshot)
		if err != nil || len(view.Documents()) != 1 {
			return snapshot, yamlvalue.Result{}, errors.New("fixture must contain one complete document")
		}
		value, err := yamlvalue.Project(ctx, snapshot, view.Documents()[0], options)
		return snapshot, value, err
	}
	_, plain, err := project("apiVersion: example.io/v1\nkind: Widget\ncount: 2\n", options)
	check("ordinary-value", err == nil && plain.Value.(map[string]any)["count"] == json.Number("2"))
	check("ordinary-retention", plain.Origin == nil && plain.Nodes == 0 && plain.LogicalNodes == 0 && len(plain.Shared) == 0)
	check("caller-selected-fields", plain.Fields["apiVersion"] == "example.io/v1" && plain.Fields["kind"] == "Widget")

	text := "label: 😀\r\nfirst: &first {name: first, selected: 1}\r\nsecond: &second {name: second, extra: 2}\r\nmerged: {<<: [*first, *second], selected: 3}\r\nuse: *first\r\nindependent: yes\r\n"
	snapshot, graph, err := project(text, options)
	check("graph-projects", err == nil)
	if err == nil {
		root := graph.Value.(map[string]any)
		merged := root["merged"].(map[string]any)
		check("merge-precedence", merged["name"] == "first" && merged["selected"] == json.Number("3") && merged["extra"] == json.Number("2"))
		check("pinned-yaml11-scalar", root["independent"] == true)
		derived := graph.Origin.At([]string{"use", "name"})
		position, posErr := snapshot.Position(derived.Node.Span().Start)
		check("derived-use-location", derived.Kind == "derived" && derived.Node.Raw() == "*first" && posErr == nil && position.Line == 4 && position.UTF16Column == 5)
		shared := graph.Origin.At([]string{"first", "name"})
		check("shared-anchor", shared.Kind == "ambiguous" && yamlvalue.Shared(graph.Shared, shared.Node.Span()))
		exact := graph.Origin.At([]string{"merged", "selected"})
		check("independent-exact", exact.Kind == "exact" && exact.Node.Raw() == "3" && !yamlvalue.Shared(graph.Shared, exact.Node.Span()))
		check("bounded-cost", graph.Nodes > graph.LogicalNodes && graph.LogicalNodes > 0)
		_, _, exhausted := project(text, yamlvalue.Options{MaxNodes: graph.Nodes - 1})
		check("expansion-budget", exhausted != nil)
		_, _, admitted := project(text, yamlvalue.Options{MaxNodes: graph.Nodes})
		check("expansion-boundary", admitted == nil)
	}
	_, _, err = project("x: &x one\ncopy: *x\n", yamlvalue.Options{MaxNodes: 100, MaxVisits: 5})
	check("alias-visit-budget", err != nil && strings.Contains(err.Error(), "alias expansion exceeds 5 nodes"))
	_, _, err = project("x: &x one\ncopy: *x\n", yamlvalue.Options{MaxNodes: 100, MaxVisits: 6})
	check("alias-visit-boundary", err == nil)

	for _, text := range []string{"key: one\nkey: two\n", "'key': one\n\"key\": two\n", "item: *missing\n", "item: &loop [*loop]\n", "item: {<<: 2}\n"} {
		_, _, err := project(text, options)
		check("reject-invalid-"+text, err != nil)
	}
	_, partial, err := project("api: &api example.io/v1\napiVersion: *api\nkind: Widget\nvalue: *missing\n", options)
	check("partial-fields-not-values", err != nil && partial.Value == nil && partial.Fields["apiVersion"] == "example.io/v1" && partial.Fields["kind"] == "Widget")
	_, unknown, err := project("apiVersion: !custom example.io/v1\nkind: Widget\n", options)
	check("custom-tag-not-known", err != nil && unknown.Fields["apiVersion"] == "")
	_, tagged, err := project("value: &value !!str on\nuse: *value\n", options)
	check("standard-tag-preserved", err == nil && tagged.Value.(map[string]any)["use"] == "on")
	_, _, err = project("value: !!str on\n", yamlvalue.Options{MaxNodes: 100000, RejectExplicitTags: true})
	check("explicit-tag-policy", err != nil && strings.Contains(err.Error(), "explicit YAML tag"))

	stream := snap("a: &a one\n---\nb: *a\n")
	view, err := yamlview.Parse(ctx, stream)
	check("stream-view", err == nil && len(view.Documents()) == 2)
	if err == nil && len(view.Documents()) == 2 {
		_, firstErr := yamlvalue.Project(ctx, stream, view.Documents()[0], options)
		_, secondErr := yamlvalue.Project(ctx, stream, view.Documents()[1], options)
		check("document-anchor-isolation", firstErr == nil && secondErr != nil)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	_, err = yamlvalue.Project(cancelled, stream, view.Documents()[0], options)
	check("projection-cancel", errors.Is(err, context.Canceled))
}
