package yamlview_test

import (
	"context"
	"github.com/lazysnail1024/k8s-crd-toolkit/source"
	"github.com/lazysnail1024/k8s-crd-toolkit/yamlview"
	"testing"
	"unicode/utf8"
)

func FuzzParseRecovery(f *testing.F) {
	for _, text := range []string{"a: b\n", "---\na: [", "a: &x {b: c}\nz: *x\n"} {
		f.Add(text)
	}
	f.Fuzz(func(t *testing.T, text string) {
		if len(text) > 8192 || !utf8.ValidString(text) {
			t.Skip()
		}
		s, _ := source.New("file:///fuzz", 1, text)
		_, _ = yamlview.Parse(context.Background(), s)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := yamlview.Parse(ctx, s); err == nil {
			t.Fatal("cancelled parse succeeded")
		}
		known, _ := source.New("file:///known", 2, "key: value\n")
		v, err := yamlview.Parse(context.Background(), known)
		if err != nil || len(v.Documents()) != 1 || !v.Documents()[0].Complete() || len(v.Issues()) != 0 {
			t.Fatal("parser failed to recover")
		}
		members := v.Documents()[0].Root().Members()
		if len(members) != 1 || members[0].Key.Raw() != "key" || members[0].Value.Raw() != "value" {
			t.Fatal("known mapping changed")
		}
	})
}
