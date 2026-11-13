package main

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/lazysnail1024/k8s-crd-toolkit/yamlview"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type retentionKey struct{}
type retainedBlob struct{ Data []byte }

func retentionCases() {
	concurrentCancellationCases()
	foregroundDuringParsingCase()
	for _, mode := range []string{"success", "error", "cancel"} {
		var finalized atomic.Int64
		var before, after runtime.MemStats
		for i := 0; i < 20; i++ {
			yamlview.Parse(context.Background(), snap("warm: up\n"))
		}
		runtime.GC()
		runtime.GC()
		runtime.ReadMemStats(&before)
		valid := true
		for i := 0; i < 96; i++ {
			value := &retainedBlob{Data: make([]byte, 1024*1024)}
			for j := 0; j < len(value.Data); j += 4096 {
				value.Data[j] = byte(i)
			}
			runtime.SetFinalizer(value, func(*retainedBlob) { finalized.Add(1) })
			ctx := context.WithValue(context.Background(), retentionKey{}, value)
			input := "value: released\n"
			if mode == "error" {
				input = strings.Repeat("[", 600) + "0" + strings.Repeat("]", 600)
			}
			cancel := func() {}
			if mode == "cancel" {
				input = strings.Repeat("- [a,b,c,d,e,f,g,h,i,j]\n", 20000)
				ctx, cancel = context.WithTimeout(ctx, time.Millisecond)
			}
			_, e := yamlview.Parse(ctx, snap(input))
			cancel()
			valid = valid && ((mode == "success" && e == nil) || (mode == "error" && e != nil) || (mode == "cancel" && errors.Is(e, context.DeadlineExceeded)))
		}
		runtime.GC()
		time.Sleep(20 * time.Millisecond)
		runtime.GC()
		until := time.Now().Add(2 * time.Second)
		for finalized.Load() < 96 && time.Now().Before(until) {
			runtime.GC()
			time.Sleep(10 * time.Millisecond)
		}
		runtime.ReadMemStats(&after)
		delta := int64(after.HeapAlloc) - int64(before.HeapAlloc)
		check(mode+"-result", valid)
		check(mode+"-context-released", delta < 8*1024*1024)
		check(mode+"-all-contexts-finalized-while-idle", finalized.Load() == 96)
		json.NewEncoder(os.Stderr).Encode(map[string]any{"mode": mode, "parses": 96, "retainedBytes": delta, "finalized": finalized.Load(), "threshold": 8 * 1024 * 1024})
	}
}

func foregroundDuringParsingCase() {
	const comment = "# comment!\n"
	const framing = "head: ok\ntail: end\n"
	input := snap("head: ok\n" + strings.Repeat(comment, (5*1024*1024-len(framing))/len(comment)) + "tail: end\n")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	started := make(chan struct{}, 2)
	completed := make(chan bool, 2)
	for i := 0; i < 2; i++ {
		go func() {
			started <- struct{}{}
			view, err := yamlview.Parse(ctx, input)
			valid := errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
			if err == nil && len(view.Documents()) == 1 && view.Documents()[0].Complete() {
				members := view.Documents()[0].Root().Members()
				valid = len(members) == 2 && members[0].Key.Raw() == "head" && members[0].Value.Raw() == "ok" && members[1].Key.Raw() == "tail" && members[1].Value.Raw() == "end"
			}
			completed <- valid
		}()
	}
	<-started
	<-started
	time.Sleep(200 * time.Millisecond)
	pending := 2 - len(completed)
	foreground, stop := context.WithTimeout(context.Background(), 500*time.Millisecond)
	began := time.Now()
	view, err := yamlview.Parse(foreground, snap("foreground: available\n"))
	elapsed := time.Since(began)
	stop()
	valid := err == nil && len(view.Documents()) == 1 && view.Documents()[0].Complete()
	if valid {
		members := view.Documents()[0].Root().Members()
		valid = len(members) == 1 && members[0].Key.Raw() == "foreground" && members[0].Value.Raw() == "available"
	}
	check("independent-small-parse-remains-available", valid)
	check("independent-small-parse-latency", elapsed < 100*time.Millisecond)
	cancel()
	first, second := <-completed, <-completed
	check("concurrent-background-parses-terminate", first && second)
	json.NewEncoder(os.Stderr).Encode(map[string]any{"backgroundCalls": 2, "inputBytes": len(input.Text()), "pendingAtQuery": pending, "foregroundElapsedMS": elapsed.Milliseconds()})
}

// Real public calls overlap slow syntax work, then cancel while other callers
// still need the parser. This also catches a worker retaining its last context.
func concurrentCancellationCases() {
	const calls = 24
	var finalized atomic.Int64
	input := snap("head: ok\n" + strings.Repeat("# comment!\n", 70000) + "tail: end\n")
	start := make(chan struct{})
	results := make(chan bool, calls)
	var workers sync.WaitGroup
	for i := 0; i < calls; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			value := &retainedBlob{Data: make([]byte, 1024*1024)}
			runtime.SetFinalizer(value, func(*retainedBlob) { finalized.Add(1) })
			<-start
			ctx, cancel := context.WithTimeout(context.WithValue(context.Background(), retentionKey{}, value), 10*time.Millisecond)
			_, err := yamlview.Parse(ctx, input)
			cancel()
			results <- errors.Is(err, context.DeadlineExceeded)
		}()
	}
	began := time.Now()
	close(start)
	workers.Wait()
	elapsed := time.Since(began)
	close(results)
	valid := true
	for result := range results {
		valid = valid && result
	}
	check("concurrent-cancellation-results", valid)
	check("concurrent-cancellation-prompt", elapsed < 3*time.Second)
	until := time.Now().Add(2 * time.Second)
	for finalized.Load() < calls && time.Now().Before(until) {
		runtime.GC()
		time.Sleep(10 * time.Millisecond)
	}
	check("idle-parser-contexts-released", finalized.Load() == calls)
	v, err := yamlview.Parse(context.Background(), snap("ready: recovered\n"))
	healthy := err == nil && len(v.Documents()) == 1 && v.Documents()[0].Complete()
	if healthy {
		members := v.Documents()[0].Root().Members()
		healthy = len(members) == 1 && members[0].Key.Raw() == "ready" && members[0].Value.Raw() == "recovered"
	}
	check("parser-usable-after-concurrent-cancellation", healthy)
	json.NewEncoder(os.Stderr).Encode(map[string]any{"concurrentCalls": calls, "cancelElapsedMS": elapsed.Milliseconds(), "finalized": finalized.Load()})
}
