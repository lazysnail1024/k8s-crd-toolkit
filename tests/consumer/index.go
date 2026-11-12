package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"runtime"
	"strings"

	"github.com/lazysnail1024/k8s-crd-toolkit/index"
	"github.com/lazysnail1024/k8s-crd-toolkit/source"
)

func indexCases() {
	indexTicketCases()
	indexReuseCases()
	indexDependencyCases()
	indexIdentityCases()
	indexSymbolOrderCases()
	indexPrefixCases()
	indexQueryContextCases()
	indexSymbolAllocationCase()
	indexScopeMoveCases()
	indexCopyCases()
	indexInputCases()
	indexLimitCases()
	indexRetentionCase()
	relationCases()
	referenceOnlyCases()
	indexConcurrencyCases()
}

func consumerIndex(name string, limits index.Limits) *index.Index {
	i, err := index.New(limits)
	check(name+"-new", err == nil && i != nil)
	return i
}

func generousIndex(name string) *index.Index {
	return consumerIndex(name, index.Limits{Files: 32, Symbols: 64, References: 128, Bytes: 1 << 20})
}

func definitionFacts(key index.Key, semantic string, offset int) index.Facts {
	return index.Facts{
		SemanticDigest: sha256.Sum256([]byte(semantic)),
		Symbols: []index.Symbol{{
			ID: "root", Key: key, Name: string(key), Kind: "resource",
			Span:      source.Span{Start: offset, End: offset + 20},
			Selection: source.Span{Start: offset + 3, End: offset + 8},
		}},
	}
}

func referenceFacts(key index.Key, offset int) index.Facts {
	f := definitionFacts("caller", "caller-signature", offset)
	f.References = []index.Reference{{Target: key, Span: source.Span{Start: offset + 10, End: offset + 14}}}
	return f
}

func commitIndexFile(i *index.Index, uri source.URI, scope index.Scope, revision source.Revision, facts index.Facts) (index.Change, error) {
	ticket, err := i.Begin(uri, scope, revision)
	if err != nil {
		return index.Change{}, err
	}
	return i.Commit(ticket, facts)
}

func exactAffected(change index.Change, uris ...source.URI) bool {
	if len(uris) == 0 {
		return len(change.Affected) == 0
	}
	return reflect.DeepEqual(change.Affected, uris)
}

func indexTicketCases() {
	i := generousIndex("tickets")
	if i == nil {
		return
	}
	uri := source.URI("file:///tickets.yaml")
	old, err := i.Begin(uri, "scope", 1)
	check("ticket-first-begin", err == nil)
	newer, err := i.Begin(uri, "scope", 2)
	check("ticket-newer-begin", err == nil)
	_, err = i.Commit(old, definitionFacts("obsolete", "old", 0))
	check("stale-ticket-cannot-commit", errors.Is(err, index.ErrStaleTicket))
	_, err = i.Commit(newer, definitionFacts("current", "new", 0))
	check("newest-ticket-commits", err == nil)
	file, found, state := i.GetFile(uri)
	check("newest-file-fresh", found && file.Fresh && file.Revision == 2 && state.Pending == 0)
	_, err = i.Commit(newer, definitionFacts("replayed", "replay", 0))
	check("ticket-cannot-replay", errors.Is(err, index.ErrStaleTicket))
	_, err = i.Begin(uri, "scope", 1)
	check("lower-revision-rejected", err != nil)

	beforeDelete, err := i.Begin(uri, "scope", 3)
	check("ticket-before-delete", err == nil)
	_, err = i.Remove(uri)
	check("remove-pending-file", err == nil)
	recreated, err := i.Begin(uri, "scope", 3)
	check("same-revision-recreation", err == nil)
	_, err = i.Commit(recreated, definitionFacts("recreated", "replacement", 0))
	check("recreated-file-commits", err == nil)
	_, err = i.Commit(beforeDelete, definitionFacts("resurrected", "obsolete", 0))
	check("deleted-ticket-cannot-resurrect", errors.Is(err, index.ErrStaleTicket))
	definitions, _ := i.Definitions("scope", "recreated")
	check("recreation-survives-old-result", len(definitions) == 1 && definitions[0].URI == uri)
	definitions, _ = i.Definitions("scope", "resurrected")
	check("deleted-definition-not-restored", len(definitions) == 0)

	other := generousIndex("other-instance")
	if other == nil {
		return
	}
	first, err := i.Begin("file:///shared.yaml", "scope", 1)
	check("cross-instance-source-ticket", err == nil)
	_, err = other.Begin("file:///shared.yaml", "scope", 1)
	check("cross-instance-destination-ticket", err == nil)
	_, err = other.Commit(first, definitionFacts("wrong-instance", "same-counter", 0))
	check("cross-instance-ticket-rejected", errors.Is(err, index.ErrStaleTicket))
	_, _, before := i.GetFile(uri)
	change, err := i.Remove("file:///not-known.yaml")
	_, _, after := i.GetFile(uri)
	check("remove-unknown-is-noop", err == nil && exactAffected(change) && before.Generation == after.Generation)
}

func indexDependencyCases() {
	i := generousIndex("dependencies")
	if i == nil {
		return
	}
	caller := source.URI("file:///a-caller.yaml")
	otherCaller := source.URI("file:///b-caller.yaml")
	definition := source.URI("file:///definition.yaml")
	_, err := commitIndexFile(i, caller, "root", 1, referenceFacts("missing", 0))
	check("unresolved-use-committed", err == nil)
	uses, _ := i.References("root", "missing")
	dependents, _ := i.Dependents("root", "missing")
	check("unresolved-reference-retained", len(uses) == 1 && uses[0].URI == caller && uses[0].Reference.Target == "missing")
	check("missing-target-has-dependent", reflect.DeepEqual(dependents, []source.URI{caller}))
	change, err := commitIndexFile(i, definition, "root", 1, definitionFacts("missing", "signature-a", 0))
	check("new-target-wakes-missing-caller", err == nil && change.SemanticChanged && exactAffected(change, caller, definition))

	change, err = commitIndexFile(i, definition, "root", 2, definitionFacts("missing", "signature-a", 30))
	definitions, _ := i.Definitions("root", "missing")
	check("location-only-not-semantic", err == nil && !change.SemanticChanged && exactAffected(change, definition))
	check("location-only-updates-navigation", len(definitions) == 1 && definitions[0].Revision == 2 && definitions[0].Symbol.Selection == (source.Span{Start: 33, End: 38}))

	change, err = commitIndexFile(i, definition, "root", 3, definitionFacts("missing", "signature-b", 30))
	check("signature-change-wakes-caller", err == nil && change.SemanticChanged && exactAffected(change, caller, definition))
	_, err = commitIndexFile(i, otherCaller, "root", 1, referenceFacts("next", 0))
	check("next-key-caller-created", err == nil)
	change, err = commitIndexFile(i, definition, "root", 4, definitionFacts("next", "signature-b", 30))
	check("shape-change-cannot-hide-behind-digest", err == nil && change.SemanticChanged && exactAffected(change, caller, otherCaller, definition))
	definitions, _ = i.Definitions("root", "missing")
	check("old-key-removed", len(definitions) == 0)

	dependencyOnly := definitionFacts("dependency-user", "opaque-dependency", 0)
	dependencyOnly.Dependencies = []index.Key{"configuration"}
	_, err = commitIndexFile(i, "file:///dependency-user.yaml", "root", 1, dependencyOnly)
	check("opaque-dependency-committed", err == nil)
	dependents, _ = i.Dependents("root", "configuration")
	uses, _ = i.References("root", "configuration")
	check("dependency-without-location", reflect.DeepEqual(dependents, []source.URI{"file:///dependency-user.yaml"}) && len(uses) == 0)

	pending, err := i.Begin(otherCaller, "root", 2)
	check("pending-reference-begin", err == nil)
	uses, state := i.References("root", "next")
	dependents, _ = i.Dependents("root", "next")
	file, found, _ := i.GetFile(otherCaller)
	check("pending-location-not-published", len(uses) == 0 && state.Pending == 1 && found && !file.Fresh && len(file.Facts.References) == 0)
	check("pending-reference-still-invalidatable", reflect.DeepEqual(dependents, []source.URI{otherCaller}))
	_, err = i.Commit(pending, referenceFacts("other", 40))
	check("pending-reference-replaced", err == nil)
	dependents, _ = i.Dependents("root", "next")
	check("old-reverse-dependency-removed", len(dependents) == 0)
}

func indexIdentityCases() {
	i := generousIndex("identities")
	if i == nil {
		return
	}
	_, err := commitIndexFile(i, "file:///first.yaml", "scope-a", 1, definitionFacts("shared", "first", 0))
	check("first-scoped-definition", err == nil)
	_, err = commitIndexFile(i, "file:///second.yaml", "scope-b", 1, definitionFacts("shared", "second", 0))
	check("second-scoped-definition", err == nil)
	definitions, stateA := i.Definitions("scope-a", "shared")
	check("scope-isolates-same-key", len(definitions) == 1 && definitions[0].URI == "file:///first.yaml")
	_, err = i.Begin("file:///pending-b.yaml", "scope-b", 1)
	check("scope-b-pending-created", err == nil)
	_, currentA := i.Definitions("scope-a", "shared")
	_, currentB := i.Definitions("scope-b", "shared")
	check("scope-state-isolated", currentA.Generation == stateA.Generation && currentA.Pending == 0 && currentB.Pending == 1)
	_, err = commitIndexFile(i, "file:///duplicate.yaml", "scope-a", 1, definitionFacts("shared", "duplicate", 0))
	check("duplicate-definition-accepted", err == nil)
	definitions, _ = i.Definitions("scope-a", "shared")
	check("duplicate-targets-not-collapsed", len(definitions) == 2 && definitions[0].URI != definitions[1].URI)
	change, err := i.Remove("file:///first.yaml")
	definitions, _ = i.Definitions("scope-a", "shared")
	check("delete-removes-only-one-definition", err == nil && change.SemanticChanged && len(definitions) == 1 && definitions[0].URI == "file:///duplicate.yaml")

	_, err = commitIndexFile(i, "file:///opaque-a.yaml", "left\x00right", 1, definitionFacts("tail", "opaque-a", 0))
	check("opaque-composite-a", err == nil)
	_, err = commitIndexFile(i, "file:///opaque-b.yaml", "left", 1, definitionFacts("right\x00tail", "opaque-b", 0))
	check("opaque-composite-b", err == nil)
	definitions, _ = i.Definitions("left\x00right", "tail")
	check("opaque-key-does-not-collide", len(definitions) == 1 && definitions[0].URI == "file:///opaque-a.yaml")

	search, err := i.Symbols("scope-a", "shared", 1)
	check("workspace-symbol-filter", err == nil && len(search.Items) == 1 && search.Items[0].URI == "file:///duplicate.yaml" && !search.Truncated)
	_, err = commitIndexFile(i, "file:///another.yaml", "scope-a", 1, definitionFacts("shared-extra", "more", 0))
	check("symbol-search-extra-input", err == nil)
	search, err = i.Symbols("scope-a", "shared", 1)
	check("workspace-symbol-explicit-truncation", err == nil && len(search.Items) == 1 && search.Truncated && search.Items[0].URI == "file:///another.yaml")
	_, err = i.Symbols("scope-a", "", 0)
	check("workspace-symbol-zero-limit-rejected", err != nil)
}

func indexSymbolOrderCases() {
	i := generousIndex("symbol-order")
	if i == nil {
		return
	}
	symbol := func(id string, start, end int) index.Symbol {
		return index.Symbol{ID: index.SymbolID(id), Key: "shared-key", Name: "match-" + id, Kind: "resource", Span: source.Span{Start: start, End: end}, Selection: source.Span{Start: start, End: end}}
	}
	fixtures := []struct {
		uri     source.URI
		scope   index.Scope
		symbols []index.Symbol
	}{
		{"file:///b.yaml", "search", []index.Symbol{symbol("z", 5, 8), symbol("b", 1, 4), symbol("a", 1, 4), symbol("early", 0, 8), symbol("short", 1, 3)}},
		{"file:///c.yaml", "search", []index.Symbol{symbol("only", 0, 3)}},
		{"file:///a.yaml", "search", []index.Symbol{symbol("late", 8, 9), symbol("early", 1, 3)}},
		{"file:///0-pending.yaml", "search", []index.Symbol{symbol("hidden", 0, 3)}},
		{"file:///0-foreign.yaml", "elsewhere", []index.Symbol{symbol("hidden", 0, 3)}},
	}
	valid := true
	for _, fixture := range fixtures {
		_, err := commitIndexFile(i, fixture.uri, fixture.scope, 1, index.Facts{Symbols: fixture.symbols})
		valid = valid && err == nil
	}
	_, err := i.Begin("file:///0-pending.yaml", "search", 2)
	check("symbol-order-fixture", valid && err == nil)
	// This expected order is literal: URI first, then both span coordinates,
	// then ID. Pending locations and another scope must not occupy a slot.
	want := []string{"file:///a.yaml#early", "file:///a.yaml#late", "file:///b.yaml#early", "file:///b.yaml#short", "file:///b.yaml#a", "file:///b.yaml#b", "file:///b.yaml#z", "file:///c.yaml#only"}
	for _, limit := range []int{1, 2, 3, 5, 8, 9} {
		valid := true
		for repeat := 0; repeat < 16; repeat++ {
			result, err := i.Symbols("search", "match-", limit)
			valid = valid && err == nil && len(result.Items) == min(limit, len(want)) && result.Truncated == (len(want) > limit) && result.State.Pending == 1
			for n, item := range result.Items {
				valid = valid && n < len(want) && string(item.URI)+"#"+string(item.Symbol.ID) == want[n]
			}
		}
		check(fmt.Sprintf("workspace-symbol-stable-top-%d", limit), valid)
		bounded, boundedErr := i.SymbolsWithPrefixContext(context.Background(), "search", "", "match-", limit, 100)
		valid = boundedErr == nil && bounded.Complete && len(bounded.Items) == min(limit, len(want)) && bounded.Truncated == (len(want) > limit) && bounded.State.Pending == 1
		for n, item := range bounded.Items {
			valid = valid && n < len(want) && string(item.URI)+"#"+string(item.Symbol.ID) == want[n]
		}
		check(fmt.Sprintf("query-context-stable-top-%d", limit), valid)
	}
	result, err := i.Symbols("search", "MATCH-", 5)
	check("workspace-symbol-no-match-not-truncated", err == nil && len(result.Items) == 0 && !result.Truncated)
}

func indexSymbolAllocationCase() {
	const files, perFile, limit, queries = 20, 1000, 10, 4
	const allocationLimit = 1024 * 1024
	i := consumerIndex("symbol-query-allocation", index.Limits{Files: files, Symbols: files * perFile, References: 1, Bytes: 8 * 1024 * 1024})
	if i == nil {
		return
	}
	valid := true
	for file := 0; file < files; file++ {
		symbols := make([]index.Symbol, perFile)
		for n := range symbols {
			symbols[n] = index.Symbol{ID: index.SymbolID(fmt.Sprintf("symbol-%04d", n)), Key: "shared-key", Name: "match", Kind: "resource", Span: source.Span{Start: n * 2, End: n*2 + 1}, Selection: source.Span{Start: n * 2, End: n*2 + 1}}
		}
		_, err := commitIndexFile(i, source.URI(fmt.Sprintf("file:///query-%02d.yaml", file)), "query", 1, index.Facts{Symbols: symbols})
		valid = valid && err == nil
	}
	check("symbol-query-large-input", valid)
	_, err := i.Symbols("query", "match", limit)
	check("symbol-query-warmup", err == nil)
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	for n := 0; n < queries; n++ {
		result, err := i.Symbols("query", "match", limit)
		valid = valid && err == nil && len(result.Items) == limit && result.Truncated
		runtime.KeepAlive(result)
	}
	runtime.ReadMemStats(&after)
	runtime.KeepAlive(i)
	allocated := after.TotalAlloc - before.TotalAlloc
	check("symbol-query-large-input-correct", valid)
	// The generous bound catches collecting all 20,000 matches before slicing
	// to ten. It does not impose a query latency or process RSS requirement.
	check("symbol-query-temporary-allocation-bounded", allocated < allocationLimit)
	check("symbol-query-allocation-evidence-written", json.NewEncoder(os.Stderr).Encode(map[string]any{"symbolQuery": map[string]any{"symbols": files * perFile, "limit": limit, "queries": queries, "totalAllocBytes": allocated, "allocationLimit": allocationLimit}}) == nil)
}

func indexScopeMoveCases() {
	i := generousIndex("scope-moves")
	if i == nil {
		return
	}
	uri := source.URI("file:///moving.yaml")
	_, err := commitIndexFile(i, "file:///a-use.yaml", "a", 1, referenceFacts("old-key", 0))
	check("scope-move-old-caller", err == nil)
	_, err = commitIndexFile(i, "file:///c-use.yaml", "c", 1, referenceFacts("new-key", 0))
	check("scope-move-new-caller", err == nil)
	_, err = commitIndexFile(i, uri, "a", 1, definitionFacts("old-key", "old", 0))
	check("scope-move-definition", err == nil)
	_, err = i.Begin(uri, "a", 2)
	check("scope-move-first-invalidation", err == nil)
	_, err = i.Begin(uri, "a", 2)
	_, state := i.Definitions("a", "old-key")
	check("repeated-begin-not-double-pending", err == nil && state.Pending == 1)
	_, err = i.Begin(uri, "b", 2)
	check("scope-move-intermediate", err == nil)
	ticket, err := i.Begin(uri, "c", 2)
	check("scope-move-final-ticket", err == nil)
	definitions, oldState := i.Definitions("a", "old-key")
	_, intermediate := i.Definitions("b", "old-key")
	_, finalState := i.Definitions("c", "new-key")
	check("pending-definitions-not-published", len(definitions) == 0 && oldState.Pending == 1)
	check("scope-move-clears-intermediate", intermediate.Pending == 0 && finalState.Pending == 1)
	change, err := i.Commit(ticket, definitionFacts("new-key", "new", 0))
	check("scope-move-wakes-both-sides", err == nil && change.SemanticChanged && exactAffected(change, "file:///a-use.yaml", "file:///c-use.yaml", uri))
	old, oldState := i.Definitions("a", "old-key")
	current, finalState := i.Definitions("c", "new-key")
	check("scope-move-commits-only-final-scope", len(old) == 0 && len(current) == 1 && current[0].URI == uri && oldState.Pending == 0 && finalState.Pending == 0)
}

func indexCopyCases() {
	i := generousIndex("copies")
	if i == nil {
		return
	}
	facts := referenceFacts("target", 0)
	facts.Dependencies = []index.Key{"extra"}
	_, err := commitIndexFile(i, "file:///copy.yaml", "scope", 1, facts)
	check("copy-input-commit", err == nil)
	facts.Symbols[0].Name = "changed-input"
	facts.References[0].Target = "changed-input"
	facts.Dependencies[0] = "changed-input"
	file, found, _ := i.GetFile("file:///copy.yaml")
	if !found || len(file.Facts.Symbols) != 1 || len(file.Facts.References) != 1 || len(file.Facts.Dependencies) != 1 {
		check("copy-input-observable", false)
		return
	}
	check("caller-cannot-mutate-facts", found && file.Facts.Symbols[0].Name == "caller" && file.Facts.References[0].Target == "target" && file.Facts.Dependencies[0] == "extra")
	file.Facts.Symbols[0].Name = "changed-output"
	file.Facts.References[0].Target = "changed-output"
	file.Facts.Dependencies[0] = "changed-output"
	again, found, _ := i.GetFile("file:///copy.yaml")
	check("file-query-copies-facts", found && len(again.Facts.Symbols) == 1 && len(again.Facts.References) == 1 && len(again.Facts.Dependencies) == 1 && again.Facts.Symbols[0].Name == "caller" && again.Facts.References[0].Target == "target" && again.Facts.Dependencies[0] == "extra")
	definitions, _ := i.Definitions("scope", "caller")
	uses, _ := i.References("scope", "target")
	if len(definitions) != 1 || len(uses) != 1 {
		check("copy-query-observable", false)
		return
	}
	definitions[0].Symbol.Name = "changed-definition"
	uses[0].Reference.Target = "changed-reference"
	definitions, _ = i.Definitions("scope", "caller")
	uses, _ = i.References("scope", "target")
	check("definition-query-isolated", len(definitions) == 1 && definitions[0].Symbol.Name == "caller")
	check("reference-query-isolated", len(uses) == 1 && uses[0].Reference.Target == "target")

	facts = referenceFacts("target", 0)
	facts.Symbols = append(facts.Symbols, index.Symbol{ID: "child", Key: "child", Name: "child", Span: source.Span{Start: 4, End: 8}, Selection: source.Span{Start: 4, End: 8}})
	facts.References = append(facts.References, index.Reference{Target: "another", Span: source.Span{Start: 5, End: 6}})
	facts.Dependencies = []index.Key{"alpha", "beta"}
	_, err = commitIndexFile(i, "file:///order.yaml", "scope", 1, facts)
	check("order-baseline", err == nil)
	facts.Symbols[0], facts.Symbols[1] = facts.Symbols[1], facts.Symbols[0]
	facts.References[0], facts.References[1] = facts.References[1], facts.References[0]
	facts.Dependencies[0], facts.Dependencies[1] = facts.Dependencies[1], facts.Dependencies[0]
	change, err := commitIndexFile(i, "file:///order.yaml", "scope", 2, facts)
	check("array-order-does-not-change-semantics", err == nil && !change.SemanticChanged && exactAffected(change, "file:///order.yaml"))
}

func indexInputCases() {
	i := generousIndex("invalid-input")
	if i == nil {
		return
	}
	_, err := i.Begin("", "scope", 1)
	check("empty-uri-rejected", err != nil)
	cases := []struct {
		name   string
		mutate func(*index.Facts)
	}{
		{"empty-symbol-id", func(f *index.Facts) { f.Symbols[0].ID = "" }},
		{"empty-symbol-key", func(f *index.Facts) { f.Symbols[0].Key = "" }},
		{"duplicate-symbol-id", func(f *index.Facts) { f.Symbols = append(f.Symbols, f.Symbols[0]) }},
		{"negative-span", func(f *index.Facts) { f.Symbols[0].Span.Start = -1 }},
		{"reversed-span", func(f *index.Facts) { f.Symbols[0].Span.End = -1 }},
		{"selection-outside-span", func(f *index.Facts) { f.Symbols[0].Selection.End = 21 }},
		{"empty-reference-target", func(f *index.Facts) { f.References[0].Target = "" }},
		{"negative-reference-span", func(f *index.Facts) { f.References[0].Span.Start = -1 }},
		{"empty-dependency-key", func(f *index.Facts) { f.Dependencies = []index.Key{""} }},
	}
	for n, c := range cases {
		facts := referenceFacts("target", 0)
		c.mutate(&facts)
		ticket, beginErr := i.Begin("file:///invalid.yaml", "scope", source.Revision(n+1))
		_, err = i.Commit(ticket, facts)
		file, found, _ := i.GetFile("file:///invalid.yaml")
		check("reject-"+c.name, beginErr == nil && err != nil && found && !file.Fresh && len(file.Facts.Symbols) == 0)
		_, err = i.Commit(ticket, referenceFacts("target", 0))
		check("recover-"+c.name, err == nil)
	}
}

func indexLimitCases() {
	_, err := index.New(index.Limits{})
	check("zero-index-limits-rejected", err != nil)
	_, err = index.New(index.Limits{Files: -1, Symbols: 1, References: 1, Bytes: 100})
	check("negative-index-limits-rejected", err != nil)
	i := consumerIndex("file-limit", index.Limits{Files: 1, Symbols: 8, References: 8, Bytes: 4096})
	if i == nil {
		return
	}
	_, err = commitIndexFile(i, "file:///one.yaml", "scope", 1, definitionFacts("first", "first", 0))
	check("file-limit-first-file", err == nil)
	_, err = i.Begin("file:///two.yaml", "scope", 1)
	check("file-limit-explicit", errors.Is(err, index.ErrLimit))
	_, err = i.Remove("file:///one.yaml")
	check("file-limit-removal", err == nil)
	_, err = commitIndexFile(i, "file:///two.yaml", "scope", 1, definitionFacts("second", "second", 0))
	check("file-limit-capacity-recovers", err == nil)
	for _, limit := range []string{"symbols", "references", "bytes"} {
		indexBoundCase(limit)
	}
}

func indexBoundCase(kind string) {
	limits := index.Limits{Files: 4, Symbols: 4, References: 4, Bytes: 4096}
	if kind == "symbols" {
		limits.Symbols = 1
	} else if kind == "references" {
		limits.References = 1
	} else {
		limits.Bytes = 256
	}
	i := consumerIndex("limit-"+kind, limits)
	if i == nil {
		return
	}
	facts := definitionFacts("first", "first", 0)
	if kind == "symbols" {
		facts.Symbols = append(facts.Symbols, index.Symbol{ID: "second", Key: "second", Span: source.Span{}, Selection: source.Span{}})
	} else if kind == "references" {
		facts.Dependencies = []index.Key{"one", "two"}
	} else {
		facts.Symbols[0].Name = fmt.Sprintf("%0300d", 1)
	}
	ticket, beginErr := i.Begin("file:///bound.yaml", "scope", 1)
	_, err := i.Commit(ticket, facts)
	check(kind+"-limit-explicit", beginErr == nil && errors.Is(err, index.ErrLimit))
	_, err = i.Commit(ticket, definitionFacts("small", "small", 0))
	file, found, state := i.GetFile("file:///bound.yaml")
	check(kind+"-limit-recoverable", err == nil && found && file.Fresh && state.Pending == 0)
}

func indexRetentionCase() {
	i := generousIndex("summary-retention")
	if i == nil {
		return
	}
	runtime.GC()
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	valid := true
	for n := 0; n < 16; n++ {
		text := fmt.Sprintf("resource-%03d", n) + strings.Repeat("x", 1024*1024)
		facts := definitionFacts(index.Key(fmt.Sprintf("resource-%03d", n)), "summary", 0)
		facts.Symbols[0].Name = text[:12]
		_, err := commitIndexFile(i, source.URI(fmt.Sprintf("file:///resource-%03d.yaml", n)), "scope", 1, facts)
		valid = valid && err == nil
	}
	runtime.GC()
	runtime.GC()
	runtime.ReadMemStats(&after)
	result, err := i.Symbols("scope", "resource-", 32)
	check("retained-summaries-usable", valid && err == nil && len(result.Items) == 16)
	growth := int64(after.HeapAlloc) - int64(before.HeapAlloc)
	check("summaries-do-not-retain-source-storage", growth < 8*1024*1024)
	check("index-evidence-written", json.NewEncoder(os.Stderr).Encode(map[string]any{"summaryCount": 16, "sourceBytesPerSummary": 1024 * 1024, "retainedHeapGrowth": growth, "retentionLimit": 8 * 1024 * 1024}) == nil)
	runtime.KeepAlive(i)
}
