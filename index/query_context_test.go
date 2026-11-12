package index

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/lazysnail1024/k8s-crd-toolkit/source"
)

type queryWaitContext struct {
	context.Context
	checked chan struct{}
	once    sync.Once
}

func (ctx *queryWaitContext) Err() error {
	err := ctx.Context.Err()
	ctx.once.Do(func() { close(ctx.checked) })
	return err
}

// The public consumer covers outputs, budgets and mutations. This small
// lock-boundary test proves cancellation while the provider lock stays held.
func TestSymbolQueryCanceledWhileWaitingForLock(t *testing.T) {
	i, err := New(Limits{Files: 4, Symbols: 4, References: 4, Bytes: 4096})
	if err != nil {
		t.Fatal(err)
	}
	base, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx := &queryWaitContext{Context: base, checked: make(chan struct{})}
	i.mu.Lock()
	defer i.mu.Unlock()
	done := make(chan error, 1)
	go func() {
		result, err := i.SymbolsWithPrefixContext(ctx, "scope", "", "", 1, 1)
		if result.Complete || result.Items != nil || result.Work != 0 {
			done <- errors.New("waiting query returned completed work")
			return
		}
		done <- err
	}()
	<-ctx.checked
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("waiting query: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancellation waited for the provider lock")
	}
}

func TestSymbolQueryBatchDoesNotDrainWholeScope(t *testing.T) {
	i, err := New(Limits{Files: 1, Symbols: 1024, References: 1, Bytes: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	facts := Facts{}
	for n := 0; n < 1024; n++ {
		facts.Symbols = append(facts.Symbols, Symbol{ID: SymbolID(string(rune(n + 1))), Key: "key", Span: source.Span{Start: n, End: n + 1}, Selection: source.Span{Start: n, End: n + 1}})
	}
	ticket, err := i.Begin("file:///symbols", "scope", 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := i.Commit(ticket, facts); err != nil {
		t.Fatal(err)
	}
	state := i.scopes["scope"]
	cursor := symbolCursor{member: state.first, symbol: -1}
	items, work, err := i.symbolBatch(context.Background(), "scope", state.generation, &cursor, 1025, nil)
	if err != nil || work != 128 || len(items) != 127 || cursor.member == nil {
		t.Fatalf("batch drained too much: work=%d items=%d remaining=%v err=%v", work, len(items), cursor.member != nil, err)
	}
	if !i.mu.TryLock() {
		t.Fatal("query batch kept the provider lock")
	}
	i.mu.Unlock()
}

func TestSymbolQueryGenerationCannotWrap(t *testing.T) {
	i, err := New(Limits{Files: 1, Symbols: 1, References: 1, Bytes: 4096})
	if err != nil {
		t.Fatal(err)
	}
	i.generation = ^uint64(0) - 1
	ticket, err := i.Begin("file:///last", "scope", 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := i.Commit(ticket, Facts{}); err == nil {
		t.Fatal("commit reused a mutation generation after exhaustion")
	}
	if _, err := i.Remove("file:///last"); err == nil {
		t.Fatal("remove reused a mutation generation after exhaustion")
	}
	if _, err := i.Begin("file:///last", "moved", 2); err == nil {
		t.Fatal("scope migration reused a mutation generation after exhaustion")
	}
	result, err := i.SymbolsWithPrefixContext(context.Background(), "scope", "", "", 1, 1)
	if err != nil || !result.Complete || result.State.Generation != ^uint64(0) || result.State.Pending != 1 || result.Work != 1 || len(result.Items) != 0 {
		t.Fatalf("rejected mutations changed the last scope generation: %+v %v", result, err)
	}
}
