package main

import (
	"encoding/json"
	"github.com/lazysnail1024/k8s-crd-toolkit/edit"
	"github.com/lazysnail1024/k8s-crd-toolkit/source"
	"os"
	"reflect"
)

var results = map[string]bool{}

func check(name string, ok bool) { results[name] = ok }
func main() {
	stage := os.Args[1]
	if stage == "source" {
		sourceCases()
	} else if stage == "edit" {
		editCases()
	} else if stage == "resume" {
		resumeCases()
	} else if stage == "retention" {
		retentionCases()
	} else if stage == "budget" {
		budgetCases()
	} else if stage == "audit" {
		auditCases()
	} else if stage == "resources" {
		resourceCases()
	} else if stage == "yaml" {
		yamlCases()
	} else if stage == "index" {
		indexCases()
	} else if stage == "workspace" {
		workspaceCases()
	} else {
		check("known-stage", false)
	}
	json.NewEncoder(os.Stdout).Encode(results)
	for _, ok := range results {
		if !ok {
			os.Exit(1)
		}
	}
	if len(results) == 0 {
		os.Exit(2)
	}
}
func snap(t string) source.Snapshot {
	s, e := source.New("file:demo", 7, t)
	if e != nil {
		panic(e)
	}
	return s
}
func sourceCases() {
	s := snap("\ufeff\u4e2d😀\r\nx\ny")
	for _, c := range []struct {
		b int
		p source.Position
	}{{0, source.Position{0, 0}}, {3, source.Position{0, 1}}, {6, source.Position{0, 2}}, {10, source.Position{0, 4}}, {12, source.Position{1, 0}}, {14, source.Position{2, 0}}, {15, source.Position{2, 1}}} {
		p, e := s.Position(c.b)
		b, f := s.Offset(c.p)
		check("coordinate-"+string(rune('A'+c.b)), e == nil && f == nil && p == c.p && b == c.b)
	}
	for _, b := range []int{-1, 1, 4, 7, 11, 16} {
		_, e := s.Position(b)
		check("reject-byte-"+string(rune('A'+b)), e != nil)
	}
	_, e := s.Offset(source.Position{0, 3})
	check("reject-surrogate", e != nil)
	_, e = s.Offset(source.Position{5, 0})
	check("reject-line", e != nil)
	_, e = source.New("", 0, "\xff")
	check("invalid-utf8", e != nil)
	v, e := s.Slice(source.Span{10, 11})
	check("slice-crlf", e == nil && v == "\r")
	a := snap("abc")
	n, e := a.Apply(8, []source.Change{{source.Span{0, 1}, "XY"}, {source.Span{2, 3}, "!"}})
	check("sequential", e == nil && n.Text() == "XY!c" && a.Text() == "abc" && n.Revision() == 8)
	_, e = a.Apply(7, nil)
	check("revision", e != nil)
	_, e = a.Apply(8, []source.Change{{source.Span{0, 1}, "z"}, {source.Span{9, 10}, "!"}})
	check("atomic", e != nil && a.Text() == "abc")
	var z source.Snapshot
	p, e := z.Position(0)
	check("zero", e == nil && p == (source.Position{}))
}
func editCases() {
	s := snap("abcdef")
	rs := []edit.Replacement{{source.Span{4, 6}, "ef", "Z"}, {source.Span{0, 2}, "ab", "X"}, {source.Span{2, 2}, "", "!"}, {source.Span{2, 4}, "cd", "Y"}}
	before := append([]edit.Replacement(nil), rs...)
	n, e := edit.Apply(s, 8, edit.Plan{7, rs})
	check("same-snapshot-boundaries", e == nil && n.Text() == "X!YZ" && reflect.DeepEqual(rs, before))
	cases := map[string]edit.Plan{"revision": {6, nil}, "expected": {7, []edit.Replacement{{source.Span{0, 1}, "q", "z"}}}, "overlap": {7, []edit.Replacement{{source.Span{0, 3}, "abc", ""}, {source.Span{2, 4}, "cd", ""}}}, "same-point": {7, []edit.Replacement{{source.Span{2, 2}, "", "x"}, {source.Span{2, 2}, "", "y"}}}, "interior": {7, []edit.Replacement{{source.Span{0, 3}, "abc", ""}, {source.Span{2, 2}, "", "x"}}}, "utf8": {7, []edit.Replacement{{source.Span{0, 1}, "a", "\xff"}}}}
	for k, p := range cases {
		_, e := edit.Apply(s, 8, p)
		check(k, e != nil && s.Text() == "abcdef")
	}
	_, e = edit.Apply(s, 7, edit.Plan{7, nil})
	check("next-revision", e != nil)
}
