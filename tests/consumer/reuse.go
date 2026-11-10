package main

import (
	"errors"
	"fmt"
	"reflect"
	"runtime"

	"github.com/lazysnail1024/k8s-crd-toolkit/index"
	"github.com/lazysnail1024/k8s-crd-toolkit/source"
)

func indexReuseCases() {
	i := generousIndex("reuse")
	if i == nil {
		return
	}
	uri := source.URI("file:///reuse.yaml")
	caller := source.URI("file:///reuse-caller.yaml")
	facts := definitionFacts("target", "unchanged", 4)
	_, err := commitIndexFile(i, uri, "scope", 1, facts)
	check("reuse-original-commit", err == nil)
	_, err = commitIndexFile(i, caller, "scope", 1, referenceFacts("target", 0))
	check("reuse-dependent-commit", err == nil)
	stale, err := i.Begin(uri, "scope", 2)
	check("reuse-first-ticket", err == nil)
	ticket, err := i.Begin(uri, "scope", 3)
	check("reuse-new-ticket", err == nil)
	_, err = i.Reuse(stale)
	check("reuse-stale-ticket-rejected", errors.Is(err, index.ErrStaleTicket))
	definitions, pending := i.Definitions("scope", "target")
	file, found, _ := i.GetFile(uri)
	check("reuse-pending-locations-hidden", len(definitions) == 0 && pending.Pending == 1 && found && !file.Fresh && len(file.Facts.Symbols) == 0)
	change, err := i.Reuse(ticket)
	definitions, ready := i.Definitions("scope", "target")
	file, found, _ = i.GetFile(uri)
	check("reuse-restores-new-revision", err == nil && found && file.Fresh && file.Revision == 3 && reflect.DeepEqual(file.Facts, facts) && len(definitions) == 1 && definitions[0].Revision == 3)
	check("reuse-advances-generation-without-caller-reparse", !change.SemanticChanged && exactAffected(change, uri) && ready.Pending == 0 && ready.Generation > pending.Generation)
	dependents, _ := i.Dependents("scope", "target")
	check("reuse-preserves-reverse-dependencies", reflect.DeepEqual(dependents, []source.URI{caller}))
	_, err = i.Reuse(ticket)
	check("reuse-ticket-cannot-replay", errors.Is(err, index.ErrStaleTicket))

	moved, err := i.Begin(uri, "other-scope", 4)
	check("reuse-scope-change-begins", err == nil)
	_, err = i.Reuse(moved)
	check("reuse-rejects-different-scope", errors.Is(err, index.ErrNoPrevious))
	_, err = i.Commit(moved, definitionFacts("other-target", "changed-scope", 10))
	check("reuse-fallback-commit-keeps-ticket", err == nil)
	definitions, _ = i.Definitions("scope", "target")
	check("reuse-fallback-releases-old-scope", len(definitions) == 0)

	deleted, err := i.Begin(uri, "other-scope", 5)
	check("reuse-ticket-before-remove", err == nil)
	_, err = i.Remove(uri)
	check("reuse-remove-file", err == nil)
	recreated, err := i.Begin(uri, "other-scope", 5)
	check("reuse-recreated-ticket", err == nil)
	_, err = i.Reuse(deleted)
	check("reuse-deleted-ticket-rejected", errors.Is(err, index.ErrStaleTicket))
	_, err = i.Reuse(recreated)
	check("reuse-cannot-resurrect-removed-facts", errors.Is(err, index.ErrNoPrevious))
	_, err = i.Commit(recreated, index.Facts{})
	check("reuse-new-empty-summary-commit", err == nil)
	empty, err := i.Begin(uri, "other-scope", 6)
	check("reuse-empty-summary-ticket", err == nil)
	_, err = i.Reuse(empty)
	check("reuse-empty-summary-is-a-commit", err == nil)

	other := generousIndex("reuse-other-instance")
	if other == nil {
		return
	}
	_, err = other.Begin(uri, "other-scope", 6)
	check("reuse-other-instance-ticket", err == nil)
	_, err = other.Reuse(empty)
	check("reuse-cross-instance-rejected", errors.Is(err, index.ErrStaleTicket))
	indexReuseAllocationCase()
}

func indexReuseAllocationCase() {
	i := consumerIndex("reuse-large", index.Limits{Files: 1, Symbols: 20000, References: 1, Bytes: 4 << 20})
	if i == nil {
		return
	}
	facts := index.Facts{}
	for n := range 20000 {
		id := fmt.Sprintf("symbol-%05d", n)
		facts.Symbols = append(facts.Symbols, index.Symbol{ID: index.SymbolID(id), Key: index.Key(id), Name: id, Kind: "resource"})
	}
	_, err := commitIndexFile(i, "file:///large.yaml", "scope", 1, facts)
	check("reuse-large-commit", err == nil)
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	ok := true
	for revision := source.Revision(2); revision <= 5; revision++ {
		ticket, err := i.Begin("file:///large.yaml", "scope", revision)
		if err != nil {
			ok = false
			break
		}
		if _, err := i.Reuse(ticket); err != nil {
			ok = false
			break
		}
	}
	runtime.ReadMemStats(&after)
	check("reuse-does-not-copy-large-summary", ok && after.TotalAlloc-before.TotalAlloc < 1<<20)
}
