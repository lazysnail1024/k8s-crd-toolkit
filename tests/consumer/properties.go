package main

import (
	"fmt"
	"github.com/lazysnail1024/k8s-crd-toolkit/source"
	"github.com/lazysnail1024/k8s-crd-toolkit/workspace"
	"math/rand"
	"reflect"
)

// Each sequence uses an independent string oracle; revisions are checked but
// excluded from equality because a fresh load has a different input lifetime.
func propertyCases() {
	for seed := int64(0); seed < 16; seed++ {
		rng := rand.New(rand.NewSource(seed))
		uri := source.URI("file:///property.yaml")
		docs, _ := workspace.NewDocuments(workspace.DocumentLimits{Entries: 4, FileBytes: 4096, OpenBytes: 8192})
		old, _ := docs.BeginRead(uri)
		snapshot, _ := docs.Open(uri, "key: initial\n")
		expected := "key: initial\n"
		for n := 0; n < 12; n++ {
			replacement := fmt.Sprintf("key: value%d\n", rng.Intn(10000))
			next, err := docs.Change(uri, snapshot.Revision(), []source.Change{{Range: source.Span{Start: 0, End: len(expected)}, Text: replacement}})
			check(fmt.Sprintf("property-%d-change-%d", seed, n), err == nil && next.Text() == replacement && snapshot.Text() == expected)
			snapshot = next
			expected = replacement
		}
		_, stale := docs.PublishRead(old, "key: obsolete\n")
		check(fmt.Sprintf("property-%d-overlay-fence", seed), stale != nil)
		fresh, _ := source.New(uri, snapshot.Revision(), expected)
		check(fmt.Sprintf("property-%d-final-load", seed), reflect.DeepEqual(snapshot, fresh))
		docs.Invalidate(uri)
		kept, ok := docs.Snapshot(uri)
		check(fmt.Sprintf("property-%d-delete-overlay", seed), ok && kept.Text() == expected)
		docs.Close(uri)
		ticket, _ := docs.BeginRead(uri)
		disk, err := docs.PublishRead(ticket, "key: disk\n")
		check(fmt.Sprintf("property-%d-close-fresh-disk", seed), err == nil && disk.Text() == "key: disk\n")
		i := generousIndex(fmt.Sprintf("property-%d", seed))
		_, err = commitIndexFile(i, uri, "a", 1, definitionFacts("target", "payload", 0))
		check(fmt.Sprintf("property-%d-index-commit", seed), err == nil)
		ticket2, err := i.Begin(uri, "a", 2)
		_, reuseErr := i.Reuse(ticket2)
		j := generousIndex(fmt.Sprintf("property-fresh-%d", seed))
		_, freshErr := commitIndexFile(j, uri, "a", 2, definitionFacts("target", "payload", 0))
		x, xs := i.Definitions("a", "target")
		y, ys := j.Definitions("a", "target")
		check(fmt.Sprintf("property-%d-warm-cold", seed), err == nil && reuseErr == nil && freshErr == nil && reflect.DeepEqual(x, y) && xs.Pending == ys.Pending)
		_, err = i.Remove(uri)
		x, _ = i.Definitions("a", "target")
		check(fmt.Sprintf("property-%d-delete-index", seed), err == nil && len(x) == 0)
		_, err = commitIndexFile(i, uri, "b", 3, definitionFacts("target", "different", 0))
		x, _ = i.Definitions("a", "target")
		y, _ = i.Definitions("b", "target")
		check(fmt.Sprintf("property-%d-scope-isolation", seed), err == nil && len(x) == 0 && len(y) == 1)
	}
}
