package index

import (
	"fmt"
	"sort"
	"strings"

	"github.com/lazysnail1024/k8s-crd-toolkit/source"
)

func (i *Index) state(scope Scope) State {
	if s := i.scopes[scope]; s != nil {
		return State{s.generation, len(s.pending)}
	}
	return State{Generation: i.generation}
}

// GetFile returns fresh facts or pending metadata without old locations. The
// returned State belongs to the file's desired scope. Unknown URIs have no scope.
func (i *Index) GetFile(uri source.URI) (File, bool, State) {
	i.mu.RLock()
	defer i.mu.RUnlock()
	e := i.files[uri]
	if e == nil {
		return File{}, false, State{Generation: i.generation}
	}
	result := File{URI: e.uri, Scope: e.scope, Revision: e.revision, Fresh: e.fresh}
	if e.fresh {
		result.Facts = copyFacts(e.data.facts)
	}
	return result, true, i.state(e.scope)
}

func copyFacts(f Facts) Facts {
	f.Symbols = append([]Symbol(nil), f.Symbols...)
	f.References = append([]Reference(nil), f.References...)
	f.Dependencies = append([]Key(nil), f.Dependencies...)
	return f
}

// Definitions returns known fresh candidates, never a uniqueness proof.
func (i *Index) Definitions(scope Scope, key Key) ([]Definition, State) {
	i.mu.RLock()
	defer i.mu.RUnlock()
	result := []Definition{}
	for uri := range i.definitions[lookup{scope, key}] {
		e := i.files[uri]
		if !e.fresh {
			continue
		}
		for _, s := range e.data.facts.Symbols {
			if s.Key == key {
				result = append(result, Definition{uri, e.revision, s})
			}
		}
	}
	sortDefinitions(result)
	return result, i.state(scope)
}

// References returns only fresh navigable uses. A missing target does not
// discard its references. Explicit Dependencies have no navigation location.
func (i *Index) References(scope Scope, key Key) ([]Use, State) {
	i.mu.RLock()
	defer i.mu.RUnlock()
	result := []Use{}
	for uri := range i.dependencies[lookup{scope, key}] {
		e := i.files[uri]
		if !e.fresh {
			continue
		}
		for _, r := range e.data.facts.References {
			if r.Target == key {
				result = append(result, Use{uri, e.revision, r})
			}
		}
	}
	sort.Slice(result, func(a, b int) bool {
		x, y := result[a], result[b]
		if x.URI != y.URI {
			return x.URI < y.URI
		}
		if x.Reference.Span != y.Reference.Span {
			return spanLess(x.Reference.Span, y.Reference.Span)
		}
		return x.Reference.From < y.Reference.From
	})
	return result, i.state(scope)
}

// Dependents is a scheduling query, so it includes retained pending summaries.
// It includes explicit dependencies and references to targets not yet defined.
func (i *Index) Dependents(scope Scope, key Key) ([]source.URI, State) {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return sortedURIs(i.dependencies[lookup{scope, key}]), i.state(scope)
}

// Symbols searches known fresh names with a case-sensitive substring match.
// limit must be positive. Items are the first matches ordered by URI, span, and
// symbol ID. Truncated explicitly distinguishes a bounded result from all known
// matches. The query retains at most limit result items while scanning.
func (i *Index) Symbols(scope Scope, query string, limit int) (SymbolResult, error) {
	if limit <= 0 {
		return SymbolResult{}, fmt.Errorf("index: symbol result limit must be positive")
	}
	i.mu.RLock()
	defer i.mu.RUnlock()
	result := SymbolResult{Items: []Definition{}, State: i.state(scope)}
	s := i.scopes[scope]
	if s == nil {
		return result, nil
	}
	for uri := range s.files {
		e := i.files[uri]
		if e.fresh {
			i.appendSymbols(&result, e, query, limit)
		}
	}
	sortDefinitions(result.Items)
	return result, nil
}

func (i *Index) appendSymbols(result *SymbolResult, e *entry, query string, limit int) {
	for _, s := range e.data.facts.Symbols {
		if !strings.Contains(s.Name, query) {
			continue
		}
		keepSymbol(result, Definition{e.uri, e.revision, s}, limit)
	}
}

// Keep a max heap so map iteration order cannot change a truncated response.
func keepSymbol(result *SymbolResult, item Definition, limit int) {
	if len(result.Items) < limit {
		result.Items = append(result.Items, item)
		for child := len(result.Items) - 1; child > 0; {
			parent := (child - 1) / 2
			if !definitionLess(result.Items[parent], result.Items[child]) {
				break
			}
			result.Items[parent], result.Items[child] = result.Items[child], result.Items[parent]
			child = parent
		}
		return
	}
	result.Truncated = true
	if !definitionLess(item, result.Items[0]) {
		return
	}
	result.Items[0] = item
	for parent := 0; ; {
		child := parent*2 + 1
		if child >= len(result.Items) {
			return
		}
		if right := child + 1; right < len(result.Items) && definitionLess(result.Items[child], result.Items[right]) {
			child = right
		}
		if !definitionLess(result.Items[parent], result.Items[child]) {
			return
		}
		result.Items[parent], result.Items[child] = result.Items[child], result.Items[parent]
		parent = child
	}
}

func spanLess(a, b source.Span) bool {
	if a.Start != b.Start {
		return a.Start < b.Start
	}
	return a.End < b.End
}

func sortDefinitions(items []Definition) {
	sort.Slice(items, func(a, b int) bool {
		return definitionLess(items[a], items[b])
	})
}

func definitionLess(a, b Definition) bool {
	if a.URI != b.URI {
		return a.URI < b.URI
	}
	if a.Symbol.Span != b.Symbol.Span {
		return spanLess(a.Symbol.Span, b.Symbol.Span)
	}
	return a.Symbol.ID < b.Symbol.ID
}
