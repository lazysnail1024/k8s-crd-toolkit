package main

import (
	"context"
	"errors"
	"github.com/lazysnail1024/k8s-crd-toolkit/yamlview"
	"strings"
)

func yamlCases() {
	ctx := context.Background()
	input := "---\na: 1\na: 2\nblock: |\n  ---\n  hi\nref: *x\n---\n- \u4e2d\n- 😀\n---\n"
	v, e := yamlview.Parse(ctx, snap(input))
	check("parse", e == nil)
	ds := v.Documents()
	check("documents", len(ds) == 3)
	if len(ds) == 3 {
		r := ds[0].Root()
		ms := r.Members()
		check("duplicate", len(ms) == 4 && ms[0].Key.Raw() == "a" && ms[1].Value.Raw() == "2")
		check("mapping", r.Kind() == yamlview.Mapping && ds[0].Complete())
		if len(ms) == 4 {
			check("block", ms[2].Value.Kind() == yamlview.Scalar && strings.Contains(ms[2].Value.Raw(), "---"))
			check("alias", ms[3].Value.Kind() == yamlview.Alias)
			sp := ms[1].Value.Span()
			check("exact-span", sp.Start == 12 && sp.End == 13)
			ms[0] = yamlview.Member{}
			check("members-copy", r.Members()[0].Key.Raw() == "a")
		}
		els := ds[1].Root().Elements()
		check("sequence", len(els) == 2 && els[0].Raw() == "\u4e2d" && els[1].Raw() == "😀")
		if len(els) > 0 {
			els[0] = yamlview.Node{}
			check("elements-copy", ds[1].Root().Elements()[0].Raw() == "\u4e2d")
		}
		check("empty-document", ds[2].Root().Kind() == yamlview.Empty && ds[2].Complete())
		ds[0] = yamlview.Document{}
		check("documents-copy", v.Documents()[0].Root().Kind() == yamlview.Mapping)
	}
	bad, e := yamlview.Parse(ctx, snap("---\na: [broken\n---\ngood: yes\n"))
	check("damaged", e == nil && len(bad.Issues()) > 0)
	found := false
	for _, d := range bad.Documents() {
		if d.Complete() && strings.Contains(d.Root().Raw(), "good: yes") {
			found = true
		}
	}
	check("recovery", found)
	for i, input := range []string{"a: [1, 2\n", "a: {b: c\n", "a: \"open\n"} {
		broken, err := yamlview.Parse(ctx, snap(input))
		damaged := false
		for _, doc := range broken.Documents() {
			damaged = damaged || !doc.Complete()
		}
		check("missing-delimiter-"+string(rune('A'+i)), err == nil && damaged && len(broken.Issues()) > 0)
	}
	c, cancel := context.WithCancel(ctx)
	cancel()
	_, e = yamlview.Parse(c, snap("a: 1"))
	check("cancel", errors.Is(e, context.Canceled))
	_, e = yamlview.Parse(ctx, snap(strings.Repeat("x", 5*1024*1024+1)))
	check("bytes-limit", e != nil)
	_, e = yamlview.Parse(ctx, snap(strings.Repeat("---\n", 2001)))
	check("document-limit", e != nil)
	_, e = yamlview.Parse(ctx, snap(strings.Repeat("[", 257)+"0"+strings.Repeat("]", 257)))
	check("depth-limit", e != nil)
	for i := 0; i < 300; i++ {
		v, e = yamlview.Parse(ctx, snap("x: [1, 2]\n"))
		if e != nil || len(v.Documents()) != 1 {
			check("lifecycle", false)
			return
		}
	}
	check("lifecycle", true)
}
