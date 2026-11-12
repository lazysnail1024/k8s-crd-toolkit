package main

import (
	"context"
	"errors"
	"fmt"
	"runtime"

	"github.com/lazysnail1024/k8s-crd-toolkit/index"
	"github.com/lazysnail1024/k8s-crd-toolkit/source"
)

func indexQueryContextCases() {
	indexQueryBudgetCases()
	indexQueryMutationCases()
	indexQueryConcurrentCases()
	indexQueryMembershipCases()
	indexQueryAllocationCase()
}

func indexQueryConcurrentCases() {
	i := consumerIndex("query-concurrent", index.Limits{Files: 1, Symbols: 260, References: 1, Bytes: 1 << 20})
	if i == nil {
		return
	}
	_, err := commitIndexFile(i, "file:///changing.yaml", "scope", 1, querySymbols(260))
	if err != nil {
		check("query-concurrent-populated", false)
		return
	}
	done := make(chan error, 1)
	go func() {
		for revision := 2; revision <= 100; revision++ {
			facts := querySymbols(260)
			for n := range facts.Symbols {
				facts.Symbols[n].Name = fmt.Sprintf("revision-%d", revision)
			}
			_, err := commitIndexFile(i, "file:///changing.yaml", "scope", source.Revision(revision), facts)
			if err != nil {
				done <- err
				return
			}
		}
		done <- nil
	}()
	valid := true
	for attempt := 0; attempt < 100; attempt++ {
		result, err := i.SymbolsWithPrefixContext(context.Background(), "scope", "", "", 260, 261)
		if errors.Is(err, index.ErrQueryChanged) {
			valid = valid && !result.Complete && result.Items == nil
			continue
		}
		valid = valid && err == nil && result.Complete
		for _, item := range result.Items {
			valid = valid && item.Revision == result.Items[0].Revision && item.Symbol.Name == result.Items[0].Symbol.Name
		}
	}
	writerErr := <-done
	check("query-concurrent-no-mixed-generation", valid && writerErr == nil)
	result, err := i.SymbolsWithPrefixContext(context.Background(), "scope", "", "", 260, 261)
	check("query-concurrent-final-generation", err == nil && result.Complete && result.Work == 261 && len(result.Items) == 260 && result.Items[0].Revision == 100 && result.Items[0].Symbol.Name == "revision-100")
}

func querySymbols(count int) index.Facts {
	facts := index.Facts{}
	for n := 0; n < count; n++ {
		facts.Symbols = append(facts.Symbols, index.Symbol{ID: index.SymbolID(fmt.Sprint(n)), Key: "wanted/item", Name: "candidate", Kind: "resource", Span: source.Span{Start: n, End: n + 1}, Selection: source.Span{Start: n, End: n + 1}})
	}
	return facts
}

func indexQueryBudgetCases() {
	i := consumerIndex("query-budget", index.Limits{Files: 64, Symbols: 1024, References: 4, Bytes: 1 << 20})
	if i == nil {
		return
	}
	_, err := commitIndexFile(i, "file:///query.yaml", "scope", 1, querySymbols(260))
	check("query-budget-populated", err == nil)
	got, err := i.SymbolsWithPrefixContext(context.Background(), "scope", "wanted/", "candidate", 2, 261)
	check("query-result-cap-independent-of-work", err == nil && got.Complete && got.Truncated && got.Work == 261 && len(got.Items) == 2 && got.Items[0].Symbol.Span.Start == 0 && got.Items[1].Symbol.Span.Start == 1)
	got, err = i.SymbolsWithPrefixContext(context.Background(), "scope", "wanted/", "absent", 2, 261)
	check("query-complete-no-match-costs-work", err == nil && got.Complete && !got.Truncated && got.Work == 261 && len(got.Items) == 0)
	got, err = i.SymbolsWithPrefixContext(context.Background(), "scope", "unmatched/", "absent", 2, 7)
	check("query-no-match-budget", errors.Is(err, index.ErrQueryBudget) && !got.Complete && got.Items == nil && got.Work == 7)
	got, err = i.SymbolsWithPrefixContext(context.Background(), "scope", "wanted/", "candidate", 2, 7)
	check("query-budget-discards-partial-items", errors.Is(err, index.ErrQueryBudget) && !got.Complete && got.Items == nil && got.Work == 7)
	for n := 0; n < 20; n++ {
		_, e := i.Begin(source.URI(fmt.Sprintf("file:///empty-%d.yaml", n)), "empty", 1)
		check(fmt.Sprintf("query-empty-begin-%d", n), e == nil)
	}
	got, err = i.SymbolsWithPrefixContext(context.Background(), "empty", "", "", 1, 5)
	check("query-pending-files-cost-work", errors.Is(err, index.ErrQueryBudget) && !got.Complete && got.Items == nil && got.Work == 5)
	got, err = i.SymbolsWithPrefixContext(context.Background(), "empty", "", "", 1, 20)
	check("query-pending-scan-not-discovery-proof", err == nil && got.Complete && len(got.Items) == 0 && got.State.Pending == 20 && got.Work == 20)
	got, err = i.SymbolsWithPrefixContext(context.Background(), "unknown", "", "", 1, 1)
	check("query-empty-scope-stable-generation", err == nil && got.Complete && got.State.Generation == 0 && got.Work == 0 && len(got.Items) == 0)
	legacy, err := i.Symbols("unknown", "", 1)
	check("query-legacy-empty-scope-generation-preserved", err == nil && legacy.State.Generation > 0)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got, err = i.SymbolsWithPrefixContext(ctx, "scope", "", "", 1, 1000)
	check("query-already-canceled", errors.Is(err, context.Canceled) && !got.Complete && got.Items == nil && got.Work == 0)
	_, err = i.SymbolsWithPrefixContext(nil, "scope", "", "", 1, 1)
	check("query-reject-nil-context", err != nil)
	_, err = i.SymbolsWithPrefixContext(context.Background(), "scope", "", "", 0, 1)
	check("query-reject-result-limit", err != nil)
	_, err = i.SymbolsWithPrefixContext(context.Background(), "scope", "", "", 1, 0)
	check("query-reject-work-limit", err != nil)
}

// A real context implementation can deterministically change or cancel a
// public consumer between query batches, without sleeps or provider internals.
type queryActionContext struct {
	context.Context
	calls  int
	action func()
}

func (ctx *queryActionContext) Err() error {
	ctx.calls++
	if ctx.calls == 3 && ctx.action != nil {
		ctx.action()
	}
	return ctx.Context.Err()
}

func indexQueryMutationCases() {
	for _, mode := range []string{"same", "final", "other", "recreate", "cancel"} {
		i := consumerIndex("query-mutation-"+mode, index.Limits{Files: 4, Symbols: 1024, References: 4, Bytes: 1 << 20})
		if i == nil {
			continue
		}
		count := 260
		if mode == "final" {
			count = 1 // Mutation after the last batch still needs a final fence.
		}
		_, err := commitIndexFile(i, "file:///query.yaml", "scope", 1, querySymbols(count))
		check("query-mutation-populated-"+mode, err == nil)
		mutationOK := true
		base, cancel := context.WithCancel(context.Background())
		ctx := &queryActionContext{Context: base}
		ctx.action = func() {
			if mode == "cancel" {
				cancel()
				return
			}
			target := index.Scope("scope")
			if mode == "other" {
				target = "unrelated"
			}
			if mode == "recreate" {
				_, e := i.Remove("file:///query.yaml")
				mutationOK = mutationOK && e == nil
			}
			if mode != "cancel" {
				_, e := commitIndexFile(i, "file:///changed.yaml", target, 1, querySymbols(1))
				mutationOK = mutationOK && e == nil
			}
		}
		got, err := i.SymbolsWithPrefixContext(ctx, "scope", "wanted/", "candidate", 2, 1000)
		cancel()
		switch mode {
		case "same":
			check("query-same-scope-change-rejected", mutationOK && errors.Is(err, index.ErrQueryChanged) && !got.Complete && got.Items == nil && got.Work > 0)
		case "final":
			check("query-final-scope-change-rejected", mutationOK && errors.Is(err, index.ErrQueryChanged) && !got.Complete && got.Items == nil && got.Work == 2)
		case "recreate":
			check("query-scope-recreation-rejected", mutationOK && errors.Is(err, index.ErrQueryChanged) && !got.Complete && got.Items == nil && got.Work > 0)
		case "other":
			check("query-unrelated-scope-change-accepted", mutationOK && err == nil && got.Complete && got.Truncated && got.Work == 261 && len(got.Items) == 2)
		case "cancel":
			check("query-mid-traversal-cancel-discards-items", errors.Is(err, context.Canceled) && !got.Complete && got.Items == nil && got.Work > 0 && got.Work < 261)
		}
	}
}

func indexQueryMembershipCases() {
	i := generousIndex("query-membership")
	if i == nil {
		return
	}
	for _, uri := range []source.URI{"file:///c", "file:///a", "file:///b"} {
		_, err := commitIndexFile(i, uri, "old", 1, querySymbols(1))
		check("query-membership-populate-"+string(uri), err == nil)
	}
	ticket, err := i.Begin("file:///a", "new", 2)
	check("query-membership-move-begin", err == nil)
	old, oldErr := i.SymbolsWithPrefixContext(context.Background(), "old", "", "", 8, 20)
	newScope, newErr := i.SymbolsWithPrefixContext(context.Background(), "new", "", "", 8, 20)
	check("query-membership-both-scopes-pending", oldErr == nil && newErr == nil && old.State.Pending == 1 && newScope.State.Pending == 1 && old.Work == 5 && newScope.Work == 1 && len(old.Items) == 2 && len(newScope.Items) == 0)
	invalid := querySymbols(1)
	invalid.Symbols[0].Span.End = -1
	_, err = i.Commit(ticket, invalid)
	check("query-membership-invalid-commit-rejected", err != nil)
	old, oldErr = i.SymbolsWithPrefixContext(context.Background(), "old", "", "", 8, 20)
	newScope, newErr = i.SymbolsWithPrefixContext(context.Background(), "new", "", "", 8, 20)
	check("query-membership-invalid-commit-stays-pending", oldErr == nil && newErr == nil && old.State.Pending == 1 && newScope.State.Pending == 1 && len(old.Items) == 2 && len(newScope.Items) == 0)
	_, err = i.Commit(ticket, querySymbols(1))
	check("query-membership-move-commit", err == nil)
	_, err = i.Remove("file:///c")
	check("query-membership-remove-first", err == nil)
	old, err = i.SymbolsWithPrefixContext(context.Background(), "old", "", "", 8, 20)
	check("query-membership-links-after-detach", err == nil && old.Complete && old.Work == 2 && old.State.Pending == 0 && len(old.Items) == 1 && old.Items[0].URI == "file:///b")
	_, err = commitIndexFile(i, "file:///c", "old", 1, querySymbols(1))
	check("query-membership-recreate", err == nil)
	old, err = i.SymbolsWithPrefixContext(context.Background(), "old", "", "", 8, 20)
	check("query-membership-order-independent-of-insertion", err == nil && old.Work == 4 && len(old.Items) == 2 && old.Items[0].URI == "file:///b" && old.Items[1].URI == "file:///c")
}

func indexQueryAllocationCase() {
	i := consumerIndex("query-allocation", index.Limits{Files: 50000, Symbols: 1, References: 1, Bytes: 8 << 20})
	if i == nil {
		return
	}
	for n := 0; n < 50000; n++ {
		_, err := commitIndexFile(i, source.URI(fmt.Sprintf("file:///empty-%05d.yaml", n)), "scope", 1, index.Facts{})
		if err != nil {
			check("query-allocation-populated", false)
			return
		}
	}
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	valid := true
	for n := 0; n < 3; n++ {
		got, err := i.SymbolsWithPrefixContext(context.Background(), "scope", "", "absent", 1, 1)
		valid = valid && errors.Is(err, index.ErrQueryBudget) && !got.Complete && got.Items == nil && got.Work == 1
	}
	runtime.ReadMemStats(&after)
	check("query-small-budget-no-all-uri-snapshot", valid && after.TotalAlloc-before.TotalAlloc < 512*1024)
	runtime.KeepAlive(i)
}
