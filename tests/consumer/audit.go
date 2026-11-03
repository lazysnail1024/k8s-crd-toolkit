package main

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/lazysnail1024/k8s-crd-toolkit/yamlview"
	"os"
	"runtime"
	"strings"
	"time"
)

func auditCases() {
	ctx := context.Background()
	v, e := yamlview.Parse(ctx, snap(strings.Repeat("[", 256)+"0"+strings.Repeat("]", 256)))
	check("depth-boundary", e == nil && len(v.Documents()) == 1 && v.Documents()[0].Complete())
	v, e = yamlview.Parse(ctx, snap(strings.Repeat("---\n", 2000)))
	check("document-boundary", e == nil && len(v.Documents()) == 2000)
	text := "---\nok: 1\n---\nbad: [\n---\nok: 2\n"
	v, e = yamlview.Parse(ctx, snap(text))
	ds := v.Documents()
	check("recovery-three", e == nil && len(ds) == 3 && ds[0].Complete() && !ds[1].Complete() && ds[2].Complete())
	if len(ds) == 3 {
		m := ds[2].Root().Members()
		check("recovered-global-span", len(m) == 1 && m[0].Value.Raw() == "2" && m[0].Value.Span().Start == 29)
	}
	issues := v.Issues()
	if len(issues) > 0 {
		issues[0].Message = "changed"
		check("issues-copy", v.Issues()[0].Message != "changed")
	} else {
		check("issues-copy", false)
	}
	v, e = yamlview.Parse(ctx, snap("base: &x {a: 1}\ncopy: *x\nmerge: {<<: *x, a: 2, a: 3}\nquoted: \"x\\ny\"\n"))
	ds = v.Documents()
	check("raw-structure-parse", e == nil && len(ds) == 1 && ds[0].Complete())
	if len(ds) == 1 {
		m := ds[0].Root().Members()
		if len(m) == 4 {
			mm := m[2].Value.Members()
			check("alias-unexpanded", m[1].Value.Kind() == yamlview.Alias && m[1].Value.Raw() == "*x" && len(m[1].Value.Members()) == 0)
			check("merge-duplicate-preserved", len(mm) == 3 && mm[0].Key.Raw() == "<<" && mm[0].Value.Kind() == yamlview.Alias && mm[1].Value.Raw() == "2" && mm[2].Value.Raw() == "3")
			check("quoted-not-decoded", m[3].Value.Raw() == "\"x\\ny\"")
		} else {
			check("structure-members", false)
		}
	}
	large := snap(strings.Repeat("item: [1, 2, 3, 4]\n", 40000))
	c, cancel := context.WithTimeout(ctx, time.Millisecond)
	start := time.Now()
	_, e = yamlview.Parse(c, large)
	cancel()
	check("in-flight-deadline", errors.Is(e, context.DeadlineExceeded))
	check("cancel-return-bounded", time.Since(start) < 5*time.Second)
}
func resourceCases() {
	payload := snap(strings.Repeat("a: [1, 2, 3]\n", 256))
	parse := func() bool {
		v, e := yamlview.Parse(context.Background(), payload)
		return e == nil && len(v.Documents()) == 1 && len(v.Documents()[0].Root().Members()) == 256
	}
	for i := 0; i < 100; i++ {
		if !parse() {
			check("resource-input", false)
			return
		}
	}
	runtime.GC()
	before := rss()
	samples := []int64{before}
	ok := true
	for i := 0; i < 2000; i++ {
		ok = parse() && ok
		if (i+1)%250 == 0 {
			runtime.GC()
			samples = append(samples, rss())
		}
	}
	after := rss()
	check("repeated-structure", ok)
	samples = append(samples, after)
	validSamples := true
	for _, sample := range samples {
		validSamples = validSamples && sample > 0
	}
	check("resident-growth", validSamples && after-before < 64*1024*1024)
	json.NewEncoder(os.Stderr).Encode(map[string]any{"warmup": 100, "iterations": 2000, "rssBytes": samples, "growthLimitBytes": 64 * 1024 * 1024, "growthBytes": after - before, "platform": runtime.GOOS + "/" + runtime.GOARCH})
}
