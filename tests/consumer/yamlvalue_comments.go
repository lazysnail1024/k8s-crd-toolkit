package main

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"strings"

	"github.com/lazysnail1024/k8s-crd-toolkit/source"
	"github.com/lazysnail1024/k8s-crd-toolkit/yamlvalue"
	"github.com/lazysnail1024/k8s-crd-toolkit/yamlview"
)

func yamlValueCommentCases() {
	ctx := context.Background()
	options := yamlvalue.Options{MaxNodes: 100000}
	for _, test := range []struct{ name, text, want string }{
		{"inline", "a: one # tail\n", `{"a":"one"}`},
		{"suffix", "a: one\n# tail\n  \n# last\n", `{"a":"one"}`},
		{"comment-tab", "a: one\n# tail\t\n", `{"a":"one"}`},
		{"null", "a: # tail\n", `{"a":null}`},
		{"flow-map", "{a: one, # inside\nb: two} # tail\n", `{"a":"one","b":"two"}`},
		{"flow-sequence", "[one, # inside\ntwo] # tail\n", `["one","two"]`},
		{"literal-keep", "a: |+\n  line\n  # literal\n\n# tail\n", `{"a":"line\n# literal\n\n"}`},
		{"literal-indent", "a: |2+\n  line\n\n # tail\n", `{"a":"line\n\n"}`},
		{"literal-first-hash", "a: |+\n # literal\n\n", `{"a":"# literal\n\n"}`},
		{"literal-strip", "a: |-\n  line\n\n# tail\n", `{"a":"line"}`},
		{"folded-keep", "a: >+\n  line\n  # literal\n\n# tail\n", `{"a":"line # literal\n\n"}`},
		{"quoted", "a: \"first\n  # literal\n  last\" # tail\n", `{"a":"first # literal last"}`},
		{"standard-tag", "a: !!str on # tail\n", `{"a":"on"}`},
		{"anchor-merge", "base: &base {a: one}\nuse: {<<: *base, b: two} # tail\n", `{"base":{"a":"one"},"use":{"a":"one","b":"two"}}`},
		{"end-marker", "---\na: one # inside\n... # tail\n", `{"a":"one"}`},
		{"crlf", "a: |+\r\n  line\r\n\r\n# tail\r\n", `{"a":"line\n\n"}`},
		{"unicode-comment", "a: one\n# 注释😀\n", `{"a":"one"}`},
	} {
		snapshot := snap(test.text)
		view, parseErr := yamlview.Parse(ctx, snapshot)
		ok := parseErr == nil && len(view.Documents()) == 1
		if ok {
			doc := view.Documents()[0]
			span, raw := doc.Span(), doc.Root().Raw()
			if test.name == "comment-tab" {
				content, err := doc.ContentSpan(snapshot)
				check("comment-tab-keeps-full-span", err == nil && content == span)
			}
			result, err := yamlvalue.Project(ctx, snapshot, doc, options)
			value, jsonErr := json.Marshal(result.Value)
			ok = err == nil && jsonErr == nil && string(value) == test.want && doc.Span() == span && doc.Root().Raw() == raw
		}
		check("comment-value-"+test.name, ok)
	}

	// These separators are YAML line breaks but can be inside a CST comment.
	// Trimming them would silently discard a real second mapping member.
	for _, separator := range []rune{'\u0085', '\u2028', '\u2029'} {
		snapshot := snap("a: one\n# comment" + string(separator) + "extra: two\n")
		view, err := yamlview.Parse(ctx, snapshot)
		ok := err == nil && len(view.Documents()) == 1
		if ok {
			result, err := yamlvalue.Project(ctx, snapshot, view.Documents()[0], options)
			value, jsonErr := json.Marshal(result.Value)
			ok = err == nil && jsonErr == nil && string(value) == `{"a":"one","extra":"two"}`
		}
		check(fmt.Sprintf("comment-unicode-break-%04X", separator), ok)
	}
	// Syntax completeness does not make control characters valid YAML input.
	for _, control := range []rune{1, 7, 11, 12, 27, 127, 128, 159, '\ufffe', '\uffff'} {
		snapshot := snap("a: one\n# comment" + string(control) + "tail\n")
		view, err := yamlview.Parse(ctx, snapshot)
		ok := err == nil && len(view.Documents()) == 1
		if ok {
			_, err := yamlvalue.Project(ctx, snapshot, view.Documents()[0], options)
			ok = err != nil
		}
		check(fmt.Sprintf("comment-invalid-character-%04X", control), ok)
	}
	for _, test := range []struct{ name, text string }{
		{"blank", "a: one\n# tail\n \t\n# last\n"},
		{"indent", "a: one\n# tail\n\t# last\n"},
		{"blank-indent", "a: one\n# tail\n\n \t# last\n"},
	} {
		snapshot := snap(test.text)
		view, err := yamlview.Parse(ctx, snapshot)
		ok := err == nil && len(view.Documents()) == 1
		if ok {
			_, err := yamlvalue.Project(ctx, snapshot, view.Documents()[0], options)
			ok = err != nil
		}
		check("comment-invalid-tab-gap-"+test.name, ok)
	}

	stream := snap("a: 😀\r\n# first\r\n---\r\nbase: &base one\r\nuse: *base # last\r\n")
	view, err := yamlview.Parse(ctx, stream)
	ok := err == nil && len(view.Documents()) == 2
	if ok {
		first, firstErr := yamlvalue.Project(ctx, stream, view.Documents()[0], options)
		second, secondErr := yamlvalue.Project(ctx, stream, view.Documents()[1], options)
		firstJSON, _ := json.Marshal(first.Value)
		secondJSON, _ := json.Marshal(second.Value)
		ok = firstErr == nil && secondErr == nil && string(firstJSON) == `{"a":"😀"}` && string(secondJSON) == `{"base":"one","use":"one"}`
		if ok {
			origin := second.Origin.At([]string{"use"})
			position, posErr := stream.Position(origin.Node.Span().Start)
			ok = origin.Kind == "derived" && origin.Node.Raw() == "*base" && posErr == nil && position == (source.Position{Line: 4, UTF16Column: 5})
		}
	}
	check("comment-stream-source-positions", ok)

	// The recovery split before --- leaves a preceding directive incomplete.
	// Comment optimization must retain that result and the following document.
	directed := snap("%YAML 1.1\n---\na: one # inside\n... # tail\n")
	view, err = yamlview.Parse(ctx, directed)
	ok = err == nil && len(view.Documents()) == 2
	if ok {
		first, second := view.Documents()[0], view.Documents()[1]
		content, spanErr := first.ContentSpan(directed)
		_, firstErr := yamlvalue.Project(ctx, directed, first, options)
		result, secondErr := yamlvalue.Project(ctx, directed, second, options)
		value, jsonErr := json.Marshal(result.Value)
		ok = spanErr == nil && content == first.Span() && firstErr != nil && secondErr == nil && jsonErr == nil && string(value) == `{"a":"one"}`
	}
	check("comment-directive-recovery-unchanged", ok)

	// A flow root does not cover a later end marker in Root.Raw. A narrower
	// decoder range must still validate the complete original document span.
	original := snap("{a: one} # inside\n... # tail\n")
	view, err = yamlview.Parse(ctx, original)
	ok = err == nil && len(view.Documents()) == 1
	if ok {
		changed := snap("{a: one} # inside\nbad # tail\n")
		_, err := yamlvalue.Project(ctx, changed, view.Documents()[0], options)
		ok = err != nil
	}
	check("comment-source-mismatch-rejected", ok)

	// Exclude snapshot/CST construction from this measurement. The semantic
	// value is tiny; re-decoding megabytes of comments must not scale Project's
	// allocation with those bytes. This bound is deliberately much wider than
	// the small value's normal allocation, and independent of machine speed.
	prefix := "a: one\n"
	large := snap(prefix + "#" + strings.Repeat("x", 5*1024*1024-len(prefix)-1))
	view, err = yamlview.Parse(ctx, large)
	if err != nil || len(view.Documents()) != 1 {
		check("trailing-comments-view", false)
		return
	}
	doc := view.Documents()[0]
	content, err := doc.ContentSpan(large)
	check("trailing-comments-full-source", err == nil && content.End == len(prefix) && doc.Span().End == len(large.Text()) && doc.Root().Raw() == large.Text())
	const repeats = 3
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	ok = true
	for i := 0; i < repeats; i++ {
		result, err := yamlvalue.Project(ctx, large, doc, options)
		value, jsonErr := json.Marshal(result.Value)
		ok = ok && err == nil && jsonErr == nil && string(value) == `{"a":"one"}`
	}
	runtime.ReadMemStats(&after)
	check("trailing-comments-projected-value", ok)
	check("trailing-comments-project-allocation", after.TotalAlloc-before.TotalAlloc < repeats*1024*1024)
	runtime.KeepAlive(large)
}
