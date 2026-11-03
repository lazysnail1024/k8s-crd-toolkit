package main

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/lazysnail1024/k8s-crd-toolkit/yamlview"
	"os"
	"runtime"
	"strings"
	"sync/atomic"
	"time"
)

type retentionKey struct{}
type retainedBlob struct{ Data []byte }

func retentionCases() {
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
		runtime.ReadMemStats(&after)
		delta := int64(after.HeapAlloc) - int64(before.HeapAlloc)
		check(mode+"-result", valid)
		check(mode+"-context-released", delta < 8*1024*1024)
		json.NewEncoder(os.Stderr).Encode(map[string]any{"mode": mode, "parses": 96, "retainedBytes": delta, "finalized": finalized.Load(), "threshold": 8 * 1024 * 1024})
	}
}
