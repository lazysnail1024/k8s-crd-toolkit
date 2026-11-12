package main

import (
	"context"
	"fmt"

	"github.com/lazysnail1024/k8s-crd-toolkit/index"
	"github.com/lazysnail1024/k8s-crd-toolkit/source"
)

func indexPrefixCases() {
	i := consumerIndex("prefix", index.Limits{Files: 4, Symbols: 2048, References: 4, Bytes: 1 << 20})
	if i == nil {
		return
	}
	foreign := index.Facts{}
	for n := 0; n < 1001; n++ {
		foreign.Symbols = append(foreign.Symbols, index.Symbol{ID: index.SymbolID(fmt.Sprint(n)), Key: index.Key(fmt.Sprintf("foreign/%d", n)), Name: "candidate", Kind: "declaration", Span: source.Span{Start: n * 2, End: n*2 + 1}, Selection: source.Span{Start: n * 2, End: n*2 + 1}})
	}
	_, err := commitIndexFile(i, "file:///a.yaml", "scope", 1, foreign)
	check("prefix-foreign-population", err == nil)
	facts := definitionFacts("wanted/item", "target", 0)
	facts.Symbols[0].Name = "candidate"
	_, err = commitIndexFile(i, "file:///z.yaml", "scope", 1, facts)
	check("prefix-target-population", err == nil)
	all, err := i.Symbols("scope", "candidate", 1)
	check("prefix-unfiltered-bound", err == nil && all.Truncated && len(all.Items) == 1 && all.Items[0].URI == "file:///a.yaml")
	selected, err := i.SymbolsWithPrefix("scope", "wanted/", "candidate", 1)
	check("prefix-before-limit", err == nil && !selected.Truncated && len(selected.Items) == 1 && selected.Items[0].URI == "file:///z.yaml")
	bounded, err := i.SymbolsWithPrefixContext(context.Background(), "scope", "wanted/", "candidate", 1, 1004)
	check("query-context-prefix-before-limit", err == nil && bounded.Complete && !bounded.Truncated && bounded.Work == 1004 && len(bounded.Items) == 1 && bounded.Items[0].URI == "file:///z.yaml")
	missing, err := i.SymbolsWithPrefix("scope", "wanted/", "absent", 1)
	check("prefix-and-name", err == nil && !missing.Truncated && len(missing.Items) == 0)
	_, err = commitIndexFile(i, "file:///zz.yaml", "scope", 1, facts)
	selected, queryErr := i.SymbolsWithPrefix("scope", "wanted/", "", 1)
	check("prefix-truncation-preserved", err == nil && queryErr == nil && selected.Truncated && len(selected.Items) == 1 && selected.Items[0].URI == "file:///z.yaml")
	_, err = i.Begin("file:///z.yaml", "scope", 2)
	selected, queryErr = i.SymbolsWithPrefix("scope", "wanted/", "", 1)
	check("prefix-pending-hidden", err == nil && queryErr == nil && !selected.Truncated && len(selected.Items) == 1 && selected.Items[0].URI == "file:///zz.yaml" && selected.State.Pending == 1)
}
