package source_test

import (
	"github.com/lazysnail1024/k8s-crd-toolkit/source"
	"strings"
	"testing"
	"unicode/utf16"
	"unicode/utf8"
)

func FuzzCoordinates(f *testing.F) {
	for _, s := range []string{"", "a\r\nb", "\ufeff\u4e2d\U0001f600\n", "\xff"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, text string) {
		if len(text) > 4096 {
			t.Skip()
		}
		s, err := source.New("file:///fuzz", 1, text)
		if !utf8.ValidString(text) {
			if err == nil {
				t.Fatal("invalid UTF-8 accepted")
			}
			return
		}
		if err != nil {
			t.Fatal(err)
		}
		for offset := 0; offset <= len(text); offset++ {
			// UTF-8 prefix validity independently identifies rune boundaries;
			// the LF byte of CRLF is excluded by the public coordinate contract.
			valid := utf8.ValidString(text[:offset]) && !(offset > 0 && offset < len(text) && text[offset-1] == '\r' && text[offset] == '\n')
			p, e := s.Position(offset)
			if !valid {
				if e == nil {
					t.Fatalf("invalid boundary %d accepted", offset)
				}
				continue
			}
			if e != nil {
				t.Fatalf("valid boundary %d rejected: %v", offset, e)
			}
			// Split the prefix into logical lines and count encoded UTF-16 units.
			// This oracle does not call either coordinate conversion function.
			lines := strings.Split(text[:offset], "\n")
			want := source.Position{Line: uint32(len(lines) - 1), UTF16Column: uint32(len(utf16.Encode([]rune(lines[len(lines)-1]))))}
			if p != want {
				t.Fatalf("position %d: got %v, want %v", offset, p, want)
			}
			got, e := s.Offset(p)
			if e != nil || got != offset {
				t.Fatalf("round trip %d -> %v -> %d: %v", offset, p, got, e)
			}
		}
	})
}
