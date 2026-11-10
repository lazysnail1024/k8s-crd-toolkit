package main

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lazysnail1024/k8s-crd-toolkit/source"
	"github.com/lazysnail1024/k8s-crd-toolkit/workspace"
)

func consumerWorkQueue(ready, outstanding int) *workspace.WorkQueue {
	queue, err := workspace.NewWorkQueue(ready, outstanding)
	check(fmt.Sprintf("workqueue-new-%d-%d", ready, outstanding), err == nil && queue != nil)
	return queue
}

func workQueueCases() {
	for _, limits := range [][2]int{{0, 1}, {1, 0}, {2, 1}} {
		_, err := workspace.NewWorkQueue(limits[0], limits[1])
		check(fmt.Sprintf("workqueue-invalid-limits-%v", limits), err != nil)
	}
	workQueueOverflowCases()
	workQueueFollowupCases()
	workQueueRemovalCases()
	workQueueRetiredCapacityCase()
	workQueueFairnessCases()
	workQueueWakeCase()
	workQueueConcurrentCases()
}

func workQueueOverflowCases() {
	queue := consumerWorkQueue(1, 4)
	if queue == nil {
		return
	}
	for _, job := range []workspace.Job{{"file:background-a", workspace.Background}, {"file:related", workspace.Related}, {"file:background-b", workspace.Background}, {"file:foreground", workspace.Current}} {
		check("workqueue-admitted-"+string(job.URI), queue.Put(job) == nil)
	}
	check("workqueue-bounds-ready-not-notifications", queue.Len() == 1 && queue.Backlog() == 3)
	check("workqueue-overflow-retains-highest-priority", queue.Put(workspace.Job{URI: "file:related", Priority: workspace.Background}) == nil)
	err := queue.Put(workspace.Job{URI: "file:over-limit", Priority: workspace.Current})
	check("workqueue-outstanding-limit-explicit", errors.Is(err, workspace.ErrLimit))
	check("workqueue-invalid-uri", queue.Put(workspace.Job{}) != nil)
	check("workqueue-invalid-priority", queue.Put(workspace.Job{URI: "file:bad-priority", Priority: 3}) != nil)
	var order []source.URI
	for n := 0; n < 4; n++ {
		ticket, found := queue.Pop()
		if !found {
			break
		}
		order = append(order, ticket.URI)
		check("workqueue-completed-"+string(ticket.URI), queue.Complete(ticket))
	}
	check("workqueue-overflow-priority-and-fifo", reflect.DeepEqual(order, []source.URI{"file:foreground", "file:related", "file:background-a", "file:background-b"}))
	check("workqueue-complete-releases-capacity", queue.Len() == 0 && queue.Backlog() == 0 && queue.Put(workspace.Job{URI: "file:next"}) == nil)
}

func workQueueFollowupCases() {
	queue := consumerWorkQueue(1, 2)
	if queue == nil {
		return
	}
	uri := source.URI("file:edited-during-analysis")
	check("workqueue-initial-input", queue.Put(workspace.Job{URI: uri, Priority: workspace.Background}) == nil)
	first, found := queue.Pop()
	check("workqueue-dispatch-reserves-uri", found && queue.Pending(uri) && queue.Running(uri) && !queue.Waiting(uri))
	for _, priority := range []workspace.Priority{workspace.Related, workspace.Current, workspace.Background} {
		check(fmt.Sprintf("workqueue-inflight-notification-%d", priority), queue.Put(workspace.Job{URI: uri, Priority: priority}) == nil)
	}
	_, duplicate := queue.Pop()
	check("workqueue-running-not-dispatched-twice", !duplicate && queue.Waiting(uri) && queue.Backlog() == 1)
	check("workqueue-first-completion-accepted", queue.Complete(first))
	next, found := queue.Pop()
	check("workqueue-running-update-survives-completion", found && next.URI == uri && next.Priority == workspace.Current)
	check("workqueue-old-completion-cannot-release-next-run", !queue.Complete(first) && queue.Running(uri))
	other := consumerWorkQueue(1, 1)
	if other != nil {
		check("workqueue-cross-owner-completion-rejected", !other.Complete(next) && queue.Running(uri))
	}
	check("workqueue-followup-completes", queue.Complete(next) && !queue.Pending(uri))
	check("workqueue-zero-completion-rejected", !queue.Complete(workspace.WorkTicket{}))
}

func workQueueRemovalCases() {
	queue := consumerWorkQueue(1, 1)
	if queue == nil {
		return
	}
	uri := source.URI("file:removed-recreated")
	check("workqueue-before-removal", queue.Put(workspace.Job{URI: uri, Priority: workspace.Background}) == nil)
	old, found := queue.Pop()
	check("workqueue-removal-run-started", found)
	queue.Remove(uri)
	check("workqueue-retired-run-keeps-bounded-reservation", queue.Pending(uri) && queue.Running(uri) && !queue.Waiting(uri) && errors.Is(queue.Put(workspace.Job{URI: "file:unrelated"}), workspace.ErrLimit))
	check("workqueue-recreation-admitted", queue.Put(workspace.Job{URI: uri, Priority: workspace.Current}) == nil)
	_, overlap := queue.Pop()
	check("workqueue-recreated-uri-waits-for-retired-worker", !overlap)
	check("workqueue-retired-completion-only-releases-reservation", !queue.Complete(old) && queue.Waiting(uri) && !queue.Running(uri))
	recreated, found := queue.Pop()
	check("workqueue-recreated-notification-preserved", found && recreated.URI == uri && recreated.Priority == workspace.Current)
	check("workqueue-retired-replay-cannot-complete-recreation", !queue.Complete(old) && queue.Running(uri))
	check("workqueue-recreated-completes", queue.Complete(recreated))
	check("workqueue-retired-capacity-released", queue.Put(workspace.Job{URI: "file:unrelated"}) == nil)
	queue.Remove("file:unrelated")
	check("workqueue-remove-waiting-clears-obligation", !queue.Pending("file:unrelated") && queue.Len() == 0 && queue.Backlog() == 0)
}

func workQueueFairnessCases() {
	for _, ready := range []int{1, 2} {
		queue := consumerWorkQueue(ready, 3)
		if queue == nil {
			continue
		}
		jobs := []workspace.Job{{"file:continuous-current", workspace.Current}, {"file:continuous-related", workspace.Related}, {"file:continuous-background", workspace.Background}}
		valid := true
		for _, job := range jobs {
			valid = queue.Put(job) == nil && valid
		}
		counts := [3]int{}
		lastBackground := -1
		for n := 0; n < 52; n++ {
			ticket, found := queue.Pop()
			if !found {
				valid = false
				break
			}
			counts[ticket.Priority]++
			if ticket.Priority == workspace.Background {
				valid = n-lastBackground <= 13 && valid
				lastBackground = n
			}
			valid = queue.Put(workspace.Job{URI: ticket.URI, Priority: ticket.Priority}) == nil && valid
			valid = queue.Complete(ticket) && queue.Len() <= ready && valid
		}
		check(fmt.Sprintf("workqueue-small-ready-fairness-%d", ready), valid && counts == [3]int{32, 16, 4})
		workspaceEvidence[fmt.Sprintf("workQueueServiceReady%d", ready)] = counts
	}
}

func workQueueRetiredCapacityCase() {
	queue := consumerWorkQueue(1, 2) // One registered file plus one worker slot.
	if queue == nil {
		return
	}
	check("workqueue-retired-extra-initial", queue.Put(workspace.Job{URI: "file:old"}) == nil)
	old, found := queue.Pop()
	check("workqueue-retired-extra-running", found)
	queue.Remove("file:old")
	check("workqueue-retired-extra-allows-replacement-file", queue.Put(workspace.Job{URI: "file:new"}) == nil)
	err := queue.Put(workspace.Job{URI: "file:overflow"})
	check("workqueue-true-limit-keeps-accepted-obligations", errors.Is(err, workspace.ErrLimit) && queue.Running("file:old") && queue.Waiting("file:new") && !queue.Pending("file:overflow"))
	check("workqueue-retired-extra-released", !queue.Complete(old))
	next, found := queue.Pop()
	check("workqueue-accepted-file-survives-limit", found && next.URI == "file:new" && queue.Complete(next))
	check("workqueue-rejected-event-can-retry", queue.Put(workspace.Job{URI: "file:overflow"}) == nil)
}

func workQueueConcurrentCases() {
	queue := consumerWorkQueue(2, 32)
	if queue == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var mu sync.Mutex
	accepted, completed, active := map[source.URI]int{}, map[source.URI]int{}, map[source.URI]int{}
	var invalid atomic.Bool
	var workers sync.WaitGroup
	for range 4 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for ctx.Err() == nil {
				ticket, found := queue.Pop()
				if !found {
					select {
					case <-ctx.Done():
					case <-queue.Wake():
					}
					continue
				}
				mu.Lock()
				active[ticket.URI]++
				if active[ticket.URI] != 1 {
					invalid.Store(true)
				}
				version := accepted[ticket.URI]
				mu.Unlock()
				runtime.Gosched()
				mu.Lock()
				active[ticket.URI]--
				completed[ticket.URI] = max(completed[ticket.URI], version)
				mu.Unlock()
				if !queue.Complete(ticket) {
					invalid.Store(true)
				}
			}
		}()
	}
	var producers sync.WaitGroup
	for producer := range 4 {
		producers.Add(1)
		go func() {
			defer producers.Done()
			for n := 0; n < 256; n++ {
				uri := source.URI(fmt.Sprintf("file:concurrent-%02d", (n*7+producer*3)%32))
				mu.Lock()
				accepted[uri]++
				if queue.Put(workspace.Job{URI: uri, Priority: workspace.Priority((n + producer) % 3)}) != nil {
					invalid.Store(true)
				}
				mu.Unlock()
				runtime.Gosched()
			}
		}()
	}
	producers.Wait()
	drained := false
	for ctx.Err() == nil {
		mu.Lock()
		drained = len(completed) == len(accepted)
		for uri, version := range accepted {
			drained = completed[uri] == version && !queue.Pending(uri) && drained
		}
		mu.Unlock()
		if drained {
			break
		}
		time.Sleep(time.Millisecond)
	}
	cancel()
	workers.Wait()
	check("workqueue-concurrent-inputs-all-observed", drained && !invalid.Load())
	check("workqueue-concurrent-drain-releases-all-work", queue.Len() == 0 && queue.Backlog() == 0)
	workspaceEvidence["workQueueConcurrent"] = map[string]int{"workers": 4, "producers": 4, "uris": len(accepted), "notifications": 1024}
}

func workQueueWakeCase() {
	queue := consumerWorkQueue(2, 2)
	if queue == nil {
		return
	}
	check("workqueue-wake-two-inputs", queue.Put(workspace.Job{URI: "file:wake-a"}) == nil && queue.Put(workspace.Job{URI: "file:wake-b"}) == nil)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	started := make(chan workspace.WorkTicket, 2)
	release := make(chan struct{})
	var workers sync.WaitGroup
	for range 2 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			select {
			case <-ctx.Done():
				return
			case <-queue.Wake():
			}
			ticket, found := queue.Pop()
			if found {
				defer queue.Complete(ticket)
				started <- ticket
				<-release
			}
		}()
	}
	seen := map[source.URI]bool{}
	for len(seen) < 2 && ctx.Err() == nil {
		select {
		case ticket := <-started:
			seen[ticket.URI] = true
		case <-ctx.Done():
		}
	}
	check("workqueue-coalesced-wake-serves-two-workers", len(seen) == 2 && queue.Running("file:wake-a") && queue.Running("file:wake-b"))
	close(release)
	cancel()
	workers.Wait()
}

func membershipCases() {
	var membership workspace.Membership
	initial := membership.Observe()
	check("membership-before-first-pass-safe", !membership.Seen(initial) && !(workspace.ScanPass{}).Missing(initial))
	old := membership.Start()
	known := map[string]workspace.Observation{"removed": membership.Observe(), "retained": membership.Observe()}
	check("membership-first-pass-finished", membership.Finish(old, true))
	failed := membership.Start()
	known["retained"] = membership.Observe()
	check("membership-active-pass-cannot-delete", !failed.Missing(known["removed"]))
	check("membership-failed-pass-cannot-delete", !membership.Finish(failed, false) && !failed.Missing(known["removed"]) && !membership.Finish(failed, true))
	pass := membership.Start()
	known["retained"] = membership.Observe()
	// The directory containing this new file has already been enumerated. Only
	// the watcher reports it; sweep must not require a second scan observation.
	known["watcher-created"] = membership.Observe()
	check("membership-watcher-is-current-observation", membership.Seen(known["watcher-created"]))
	check("membership-complete-pass-finishes", membership.Finish(pass, true))
	var removed []string
	for name, observed := range known {
		if pass.Missing(observed) {
			removed = append(removed, name)
		}
	}
	check("membership-watcher-created-file-survives-sweep", reflect.DeepEqual(removed, []string{"removed"}))
	newer := membership.Start()
	check("membership-old-pass-cannot-delete-newer-input", !pass.Missing(known["removed"]) && !membership.Finish(pass, true))
	var other workspace.Membership
	foreign := other.Start()
	check("membership-foreign-pass-rejected", !membership.Finish(foreign, true) && !other.Seen(known["retained"]))
	check("membership-replacement-pass-can-finish", membership.Finish(newer, true))
}
