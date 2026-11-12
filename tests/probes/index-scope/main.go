package main

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"time"

	"github.com/lazysnail1024/k8s-crd-toolkit/index"
	"github.com/lazysnail1024/k8s-crd-toolkit/source"
)

func heap() uint64 {
	runtime.GC()
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	return stats.HeapAlloc
}

func main() {
	if len(os.Args) != 2 {
		panic("usage: index-scope <positive file count>")
	}
	count, err := strconv.Atoi(os.Args[1])
	if err != nil || count < 1 {
		panic("positive file count required")
	}
	baseline := heap()
	i, err := index.New(index.Limits{Files: count, Symbols: count, References: 1, Bytes: 1 << 30})
	if err != nil {
		panic(err)
	}
	started := time.Now()
	for n := 0; n < count; n++ {
		name := fmt.Sprintf("file:///item-%08d.yaml", n)
		ticket, beginErr := i.Begin(source.URI(name), "partition", 1)
		if beginErr != nil {
			panic(beginErr)
		}
		_, err = i.Commit(ticket, index.Facts{Symbols: []index.Symbol{{ID: "root", Key: index.Key(name), Name: "item", Kind: "symbol", Span: source.Span{Start: 0, End: 4}, Selection: source.Span{Start: 0, End: 4}}}})
		if err != nil {
			panic(err)
		}
	}
	buildNs := time.Since(started).Nanoseconds()
	committed := heap()
	started = time.Now()
	for n := 0; n < count; n++ {
		_, err := i.Begin(source.URI(fmt.Sprintf("file:///item-%08d.yaml", n)), "partition", 2)
		if err != nil {
			panic(err)
		}
	}
	pendingNs := time.Since(started).Nanoseconds()
	pending := heap()
	started = time.Now()
	valid := true
	for n := 0; n < 100000; n++ {
		_, state := i.Definitions("partition", "unknown")
		valid = valid && state.Pending == count
	}
	queryNs := time.Since(started).Nanoseconds()
	runtime.KeepAlive(i)
	if !valid {
		panic("pending count inconsistent")
	}
	result := map[string]any{"files": count, "committedHeap": int64(committed) - int64(baseline), "pendingHeap": int64(pending) - int64(baseline), "buildNs": buildNs, "pendingNs": pendingNs, "stateQueries": 100000, "stateQueryNs": queryNs}
	if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
		panic(err)
	}
}
