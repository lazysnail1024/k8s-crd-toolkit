package main

import (
	"errors"
	"fmt"

	"github.com/lazysnail1024/k8s-crd-toolkit/index"
	"github.com/lazysnail1024/k8s-crd-toolkit/source"
	"github.com/lazysnail1024/k8s-crd-toolkit/workspace"
)

type acceptedInput struct {
	snapshot source.Snapshot
	ticket   index.Ticket
}

// The consumer already needs the latest input payload while work is pending
// or running. URI notifications add no generation map or replacement clock.
// Calls here are serialized by the consumer's event loop.
type coordinator struct {
	docs   *workspace.Documents
	idx    *index.Index
	queue  *workspace.Queue
	latest map[source.URI]*acceptedInput
}

func newCoordinator() *coordinator {
	docs := consumerDocuments("coordinator-documents", workspace.DocumentLimits{Entries: 8, FileBytes: 1024, OpenBytes: 8192})
	idx := consumerIndex("coordinator-index", index.Limits{Files: 8, Symbols: 16, References: 16, Bytes: 8192})
	queue, err := workspace.NewQueue(4)
	check("coordinator-queue-created", err == nil)
	if docs == nil || idx == nil || err != nil {
		return nil
	}
	return &coordinator{docs: docs, idx: idx, queue: queue, latest: map[source.URI]*acceptedInput{}}
}

func (c *coordinator) accept(snapshot source.Snapshot) (*acceptedInput, error) {
	ticket, err := c.idx.Begin(snapshot.URI(), "scope", snapshot.Revision())
	if err != nil {
		return nil, err
	}
	input := &acceptedInput{snapshot, ticket}
	c.latest[snapshot.URI()] = input
	return input, c.queue.Put(workspace.Job{URI: snapshot.URI(), Priority: workspace.Current})
}

func (c *coordinator) open(uri source.URI, text string) (*acceptedInput, error) {
	snapshot, err := c.docs.Open(uri, text)
	if err != nil {
		return nil, err
	}
	return c.accept(snapshot)
}

func (c *coordinator) change(uri source.URI, text string) (*acceptedInput, error) {
	previous, found := c.docs.Snapshot(uri)
	if !found {
		return nil, fmt.Errorf("consumer: missing open snapshot")
	}
	snapshot, err := c.docs.Change(uri, previous.Revision(), []source.Change{{Range: source.Span{Start: 0, End: len(previous.Text())}, Text: text}})
	if err != nil {
		return nil, err
	}
	return c.accept(snapshot)
}

func (c *coordinator) take() (*acceptedInput, bool) {
	for {
		job, ok := c.queue.Pop()
		if !ok {
			return nil, false
		}
		if input, present := c.latest[job.URI]; present {
			return input, true
		}
		// A late notification cannot create input or trigger a disk reread.
	}
}

func (c *coordinator) complete(input *acceptedInput) error {
	if input == nil {
		return fmt.Errorf("consumer: no accepted input")
	}
	text := input.snapshot.Text()
	facts := index.Facts{Symbols: []index.Symbol{{ID: "resource", Key: "key", Name: text, Span: source.Span{Start: 0, End: len(text)}, Selection: source.Span{Start: 0, End: len(text)}}}}
	_, err := c.idx.Commit(input.ticket, facts)
	if err == nil {
		if c.latest[input.snapshot.URI()] == input {
			delete(c.latest, input.snapshot.URI())
		}
	}
	return err
}

func (c *coordinator) remove(uri source.URI) error {
	c.docs.Close(uri)
	_, err := c.idx.Remove(uri)
	delete(c.latest, uri)
	c.queue.Remove(uri)
	return err
}

func (c *coordinator) notifyOld(input *acceptedInput) error {
	if input == nil {
		return fmt.Errorf("consumer: no notification source")
	}
	return c.queue.Put(workspace.Job{URI: input.snapshot.URI(), Priority: workspace.Current})
}

func freshName(c *coordinator, uri source.URI, name string) bool {
	file, ok, _ := c.idx.GetFile(uri)
	return ok && file.Fresh && len(file.Facts.Symbols) == 1 && file.Facts.Symbols[0].Name == name
}

func queueCoordinationCases() {
	c := newCoordinator()
	if c == nil {
		return
	}
	uri := source.URI("file:///active.yaml")
	first, firstErr := c.open(uri, "first")
	_, middleErr := c.change(uri, "second")
	last, lastErr := c.change(uri, "third")
	check("coordinator-pending-changes-coalesce", firstErr == nil && middleErr == nil && lastErr == nil && c.queue.Len() == 1)
	taken, ok := c.take()
	check("coordinator-pending-pop-uses-latest-input", ok && taken == last && taken.snapshot.Text() == "third")
	check("coordinator-pending-latest-committed", c.complete(taken) == nil && freshName(c, uri, "third"))
	check("coordinator-pending-obsolete-ticket-rejected", errors.Is(c.complete(first), index.ErrStaleTicket))
	check("coordinator-committed-input-released", len(c.latest) == 0)

	running, err := c.change(uri, "running-old")
	lease, ok := c.take()
	check("coordinator-running-input-taken", err == nil && ok && lease == running)
	_, middleErr = c.change(uri, "running-middle")
	latest, lastErr := c.change(uri, "running-new")
	check("coordinator-old-notice-while-pending-safe", c.notifyOld(lease) == nil)
	now, ok := c.take()
	check("coordinator-running-changes-use-latest-input", middleErr == nil && lastErr == nil && ok && now == latest)
	err = c.complete(now)
	check("coordinator-late-worker-cannot-overwrite", err == nil && errors.Is(c.complete(lease), index.ErrStaleTicket) && freshName(c, uri, "running-new"))
	check("coordinator-old-notice-after-commit-safe", c.notifyOld(lease) == nil)
	_, ok = c.take()
	check("coordinator-completed-input-not-recreated", !ok && len(c.latest) == 0 && freshName(c, uri, "running-new"))

	deleting, err := c.change(uri, "will-delete")
	_, ok = c.take()
	check("coordinator-deletion-had-running-work", err == nil && ok)
	err = c.remove(uri)
	check("coordinator-deleted-worker-cannot-commit", err == nil && errors.Is(c.complete(deleting), index.ErrStaleTicket))
	check("coordinator-old-notice-after-delete-safe", c.notifyOld(deleting) == nil)
	_, ok = c.take()
	_, exists, _ := c.idx.GetFile(uri)
	check("coordinator-deleted-uri-cannot-resurrect", !ok && !exists && len(c.latest) == 0)

	reopened, err := c.open(uri, "reopened-new")
	check("coordinator-old-notice-after-reopen-safe", err == nil && c.notifyOld(deleting) == nil)
	reopenedInput, ok := c.take()
	check("coordinator-reopen-uses-new-lifetime", err == nil && ok && reopenedInput == reopened && deleting != nil && reopenedInput.snapshot.Revision() > deleting.snapshot.Revision())
	err = c.complete(reopenedInput)
	check("coordinator-reopen-rejects-old-fact-ticket", err == nil && errors.Is(c.complete(deleting), index.ErrStaleTicket) && freshName(c, uri, "reopened-new"))

	bounded, err := workspace.NewQueue(1)
	check("coordinator-bounded-queue-created", err == nil)
	if err != nil {
		return
	}
	firstErr = bounded.Put(workspace.Job{URI: "file:///a", Priority: workspace.Background})
	lastErr = bounded.Put(workspace.Job{URI: "file:///a", Priority: workspace.Current})
	check("coordinator-coalescing-at-capacity", firstErr == nil && lastErr == nil && bounded.Len() == 1)
	check("coordinator-explicit-backpressure", errors.Is(bounded.Put(workspace.Job{URI: "file:///b", Priority: workspace.Background}), workspace.ErrLimit))
	selected, ok := bounded.Pop()
	check("coordinator-priority-promotion-preserved", ok && selected.URI == "file:///a" && selected.Priority == workspace.Current)

	fair, err := workspace.NewQueue(3)
	check("coordinator-fair-queue-created", err == nil)
	if err != nil {
		return
	}
	jobs := []workspace.Job{{URI: "current", Priority: workspace.Current}, {URI: "related", Priority: workspace.Related}, {URI: "background", Priority: workspace.Background}}
	valid := true
	for _, job := range jobs {
		valid = fair.Put(job) == nil && valid
	}
	counts := map[source.URI]int{}
	for n := 0; n < 13; n++ {
		job, found := fair.Pop()
		valid = valid && found
		counts[job.URI]++
		valid = fair.Put(job) == nil && valid
	}
	check("coordinator-same-fair-service", valid && counts["current"] == 8 && counts["related"] == 4 && counts["background"] == 1)
}
