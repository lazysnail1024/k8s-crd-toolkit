package edit_test

import (
	"github.com/lazysnail1024/k8s-crd-toolkit/edit"
	"github.com/lazysnail1024/k8s-crd-toolkit/source"
	"testing"
	"unicode/utf8"
)

func FuzzAtomicEdit(f *testing.F) {
	f.Add("before", "after")
	f.Add("\u4e2d\r\n", "\U0001f600")
	f.Fuzz(func(t *testing.T, before, after string) {
		if len(before)+len(after) > 4096 || !utf8.ValidString(before) || !utf8.ValidString(after) {
			t.Skip()
		}
		s, _ := source.New("file:///fuzz", 1, before)
		plan := edit.Plan{BaseRevision: 1, Replacements: []edit.Replacement{{Span: source.Span{Start: 0, End: len(before)}, ExpectedText: before, NewText: after}}}
		got, err := edit.Apply(s, 2, plan)
		if err != nil || got.Text() != after || s.Text() != before {
			t.Fatal("replacement or immutability")
		}
		plan.BaseRevision = 0
		if _, err = edit.Apply(s, 2, plan); err == nil {
			t.Fatal("stale revision accepted")
		}
		plan.BaseRevision = 1
		plan.Replacements[0].ExpectedText = before + "!"
		if _, err = edit.Apply(s, 2, plan); err == nil || s.Text() != before {
			t.Fatal("wrong source accepted")
		}
	})
}
