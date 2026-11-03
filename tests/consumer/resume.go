package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/lazysnail1024/k8s-crd-toolkit/yamlview"
	"os"
	"strings"
	"time"
)

func resumeCases() {
	var text strings.Builder
	for i := 0; i < 8000; i++ {
		fmt.Fprintf(&text, "key%d: value%d\n", i, i)
	}
	v, e := yamlview.Parse(context.Background(), snap(text.String()))
	valid := e == nil && len(v.Documents()) == 1 && v.Documents()[0].Complete()
	if valid {
		m := v.Documents()[0].Root().Members()
		valid = len(m) == 8000
		for i, x := range m {
			valid = valid && x.Key.Raw() == fmt.Sprint("key", i) && x.Value.Raw() == fmt.Sprint("value", i)
			sp := x.Value.Span()
			valid = valid && text.String()[sp.Start:sp.End] == x.Value.Raw()
		}
	}
	check("resumed-exact-structure", valid)
	input := snap("head: ok\n" + strings.Repeat("# comment!\n", 70000) + "tail: end\n")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	start := time.Now()
	_, e = yamlview.Parse(ctx, input)
	cancel()
	elapsed := time.Since(start)
	check("comments-deadline", errors.Is(e, context.DeadlineExceeded))
	check("comments-cancel-prompt", elapsed < 3*time.Second)
	json.NewEncoder(os.Stderr).Encode(map[string]any{"commentLines": 70000, "cancelElapsedMS": elapsed.Milliseconds()})
}
