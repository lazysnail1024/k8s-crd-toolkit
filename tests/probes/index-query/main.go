package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"runtime"
	"strconv"
	"time"

	"github.com/lazysnail1024/k8s-crd-toolkit/index"
	"github.com/lazysnail1024/k8s-crd-toolkit/source"
)

func main() {
	n, _ := strconv.Atoi(os.Args[1])
	limit, _ := strconv.Atoi(os.Args[2])
	queries, _ := strconv.Atoi(os.Args[3])
	i, err := index.New(index.Limits{Files: n + 1, Symbols: n + 1, References: 1, Bytes: 128 * 1024 * 1024})
	if err != nil {
		panic(err)
	}
	order := rand.New(rand.NewSource(19)).Perm(n)
	expected := make([]source.URI, limit)
	for j := range expected {
		expected[j] = source.URI(fmt.Sprintf("file:///fixture/%06d.yaml", j))
	}
	for _, j := range order {
		uri := source.URI(fmt.Sprintf("file:///fixture/%06d.yaml", j))
		ticket, beginErr := i.Begin(uri, "one-scope", 1)
		if beginErr != nil {
			panic(beginErr)
		}
		_, commitErr := i.Commit(ticket, index.Facts{Symbols: []index.Symbol{{ID: "resource", Key: index.Key(fmt.Sprintf("key-%d", j)), Name: "match", Kind: "resource", Span: source.Span{Start: 0, End: 5}, Selection: source.Span{Start: 0, End: 5}}}})
		if commitErr != nil {
			panic(commitErr)
		}
	}
	for j := 0; j < 2; j++ {
		_, _ = i.Symbols("one-scope", "match", limit)
	}
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	var elapsed time.Duration
	correct := true
	for j := 0; j < queries; j++ {
		started := time.Now()
		result, queryErr := i.Symbols("one-scope", "match", limit)
		elapsed += time.Since(started)
		correct = correct && queryErr == nil && result.Truncated == (n > limit) && len(result.Items) == len(expected)
		if len(result.Items) == len(expected) {
			for k, item := range result.Items {
				correct = correct && item.URI == expected[k]
			}
		}
		runtime.KeepAlive(result)
	}
	runtime.ReadMemStats(&after)
	runtime.KeepAlive(i)
	json.NewEncoder(os.Stdout).Encode(map[string]any{"n": n, "limit": limit, "queries": queries, "seed": 19, "correct": correct, "queryNanoseconds": int64(elapsed), "totalAllocBytes": after.TotalAlloc - before.TotalAlloc, "mallocs": after.Mallocs - before.Mallocs, "gcCycles": after.NumGC - before.NumGC, "GOMAXPROCS": runtime.GOMAXPROCS(0), "go": runtime.Version()})
	if !correct {
		os.Exit(1)
	}
}
