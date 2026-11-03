package main

import (
	"context"
	_ "embed"
	"github.com/lazysnail1024/k8s-crd-toolkit/yamlview"
	"strconv"
	"strings"
	"sync"
)

//go:embed deep.yaml
var reportedDeepInput string

func nested(n int, sequence bool, tail bool) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		b.WriteString(strings.Repeat("  ", i))
		if sequence {
			b.WriteString("-\n")
		} else {
			b.WriteString("x:\n")
		}
	}
	if tail {
		b.WriteString(strings.Repeat("  ", n))
		b.WriteString("ok\n")
	}
	return b.String()
}
func budgetCases() {
	ctx := context.Background()
	_, reportedErr := yamlview.Parse(ctx, snap(reportedDeepInput))
	check("reported-cd-input-rejected", reportedErr != nil)
	var workers sync.WaitGroup
	answers := make(chan bool, 16)
	for i := 0; i < 16; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			_, e := yamlview.Parse(ctx, snap(nested(1000, false, true)))
			v, f := yamlview.Parse(ctx, snap("ok: true\n"))
			answers <- e != nil && f == nil && len(v.Documents()) == 1 && v.Documents()[0].Complete()
		}()
	}
	workers.Wait()
	close(answers)
	parallel := true
	for ok := range answers {
		parallel = parallel && ok
	}
	check("concurrent-budget-isolation", parallel)

	for _, n := range []int{1000, 300, 257, 256} {
		for _, seq := range []bool{false, true} {
			v, e := yamlview.Parse(ctx, snap(nested(n, seq, true)))
			name := "mapping"
			if seq {
				name = "sequence"
			}
			if n > 256 {
				check(name+strconv.Itoa(n)+"-rejected", e != nil)
			} else {
				check(name+"-256-complete", e == nil && len(v.Documents()) == 1 && v.Documents()[0].Complete())
			}
		}
	}
	_, e := yamlview.Parse(ctx, snap(nested(300, false, false)))
	check("deep-missing-value-rejected", e != nil)
	var wide strings.Builder
	for i := 0; i < 256; i++ {
		wide.WriteString(strings.Repeat(" ", i*128))
		wide.WriteString("x:\n")
	}
	wide.WriteString(strings.Repeat(" ", 256*128))
	wide.WriteString("ok\n")
	v, err := yamlview.Parse(ctx, snap(wide.String()))
	check("wide-depth-256", err == nil && len(v.Documents()) == 1 && v.Documents()[0].Complete())
	for _, input := range []struct{ name, text string }{{"wide-shallow", "x:\n" + strings.Repeat(" ", 40000) + "y: ok\n"}, {"quoted-lines", "x: \"start\n" + strings.Repeat("  literal [ { : text\n", 300) + "  end\"\n"}, {"block-lines", "x: |\n" + strings.Repeat("  literal [ { : text\n", 300)}, {"flow-256", strings.Repeat("[", 256) + "0" + strings.Repeat("]", 256)}} {
		v, e := yamlview.Parse(ctx, snap(input.text))
		check(input.name, e == nil && len(v.Documents()) == 1 && v.Documents()[0].Complete())
	}
	for i := 0; i < 10; i++ {
		_, e = yamlview.Parse(ctx, snap(nested(300, false, true)))
		if e == nil {
			check("repeat-budget", false)
			return
		}
		v, e := yamlview.Parse(ctx, snap("x: [1, 2]\n"))
		if e != nil || len(v.Documents()) != 1 || !v.Documents()[0].Complete() {
			check("recovery-after-budget", false)
			return
		}
	}
	check("repeat-budget", true)
	check("recovery-after-budget", true)
}
