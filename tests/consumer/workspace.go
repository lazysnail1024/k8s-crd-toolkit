package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"

	"github.com/lazysnail1024/k8s-crd-toolkit/source"
	"github.com/lazysnail1024/k8s-crd-toolkit/workspace"
)

var workspaceEvidence = map[string]any{}

func workspaceCases() {
	documentCases()
	documentDiscardCases()
	documentLimitCases()
	queueCases()
	queueFairnessCases()
	queueCoordinationCases()
	scannerCases()
	scannerFailureCases()
	scannerInteractionCases()
	scannerLifecycleCases()
	workspaceEvidence["platform"] = runtime.GOOS + "/" + runtime.GOARCH
	check("workspace-evidence-written", json.NewEncoder(os.Stderr).Encode(workspaceEvidence) == nil)
}

func consumerDocuments(name string, limits workspace.DocumentLimits) *workspace.Documents {
	documents, err := workspace.NewDocuments(limits)
	check(name+"-new", err == nil && documents != nil)
	return documents
}

func documentCases() {
	documents := consumerDocuments("documents", workspace.DocumentLimits{Entries: 16, FileBytes: 4096, OpenBytes: 8192})
	if documents == nil {
		return
	}
	uri := source.URI("file:///document.yaml")
	diskTicket, err := documents.BeginRead(uri)
	check("disk-ticket-created", err == nil)
	opened, err := documents.Open(uri, "name: unsaved\n")
	check("overlay-opened", err == nil)
	_, err = documents.PublishRead(diskTicket, "name: disk\n")
	check("disk-result-cannot-overwrite-overlay", errors.Is(err, workspace.ErrStale))
	_, err = documents.BeginRead(uri)
	check("overlay-prevents-disk-read", errors.Is(err, workspace.ErrOverlay))
	documents.Invalidate(uri)
	current, found := documents.Snapshot(uri)
	check("disk-event-keeps-overlay", found && current.Text() == "name: unsaved\n" && current.Revision() == opened.Revision())
	changed, err := documents.Change(uri, opened.Revision(), []source.Change{{Range: source.Span{Start: 6, End: 13}, Text: "edited"}})
	check("overlay-versioned-change", err == nil && changed.Text() == "name: edited\n" && changed.Revision() > opened.Revision())
	_, err = documents.Change(uri, opened.Revision(), []source.Change{{Range: source.Span{}, Text: "stale"}})
	check("old-overlay-revision-rejected", errors.Is(err, workspace.ErrStale))
	_, err = documents.Change(uri, changed.Revision(), []source.Change{{Range: source.Span{Start: 0, End: 1}, Text: "x"}, {Range: source.Span{Start: 100, End: 101}, Text: "y"}})
	current, found = documents.Snapshot(uri)
	check("invalid-overlay-batch-atomic", err != nil && found && current.Text() == "name: edited\n" && current.Revision() == changed.Revision())
	check("overlay-close", documents.Close(uri))
	_, found = documents.Snapshot(uri)
	check("close-does-not-restore-cached-disk", !found)
	old, err := documents.BeginRead(uri)
	check("closed-document-new-read", err == nil)
	newer, err := documents.BeginRead(uri)
	check("replacement-read-ticket", err == nil)
	_, err = documents.PublishRead(old, "obsolete")
	check("superseded-read-rejected", errors.Is(err, workspace.ErrStale))
	disk, err := documents.PublishRead(newer, "name: current-disk\n")
	check("latest-disk-read-published", err == nil && disk.Text() == "name: current-disk\n" && disk.Revision() > changed.Revision())
	_, found = documents.Snapshot(uri)
	check("closed-disk-text-not-retained", !found)
	_, err = documents.PublishRead(newer, "replayed")
	check("read-ticket-cannot-replay", errors.Is(err, workspace.ErrStale))
	beforeEvent, err := documents.BeginRead(uri)
	check("read-before-event", err == nil)
	documents.Invalidate(uri)
	_, err = documents.PublishRead(beforeEvent, "stale-event-result")
	check("disk-event-invalidates-ticket", errors.Is(err, workspace.ErrStale))
	beforeReopen, err := documents.BeginRead(uri)
	check("read-before-reopen", err == nil)
	_, err = documents.Open(uri, "new lifetime")
	check("overlay-reopen", err == nil)
	documents.Close(uri)
	_, err = documents.PublishRead(beforeReopen, "old lifetime")
	check("reopened-document-rejects-old-ticket", errors.Is(err, workspace.ErrStale))
	other := consumerDocuments("other-documents", workspace.DocumentLimits{Entries: 4, FileBytes: 4096, OpenBytes: 8192})
	if other == nil {
		return
	}
	foreign, err := documents.BeginRead("file:///same.yaml")
	check("foreign-ticket-created", err == nil)
	_, err = other.BeginRead("file:///same.yaml")
	check("foreign-ticket-target-created", err == nil)
	_, err = other.PublishRead(foreign, "wrong store")
	check("cross-document-store-ticket-rejected", errors.Is(err, workspace.ErrStale))
	_, err = documents.Open("file:///invalid-utf8.yaml", "\xff")
	check("invalid-overlay-utf8-rejected", err != nil)
	_, err = documents.Open("", "empty uri")
	check("empty-document-uri-rejected", err != nil)
}

func documentDiscardCases() {
	documents := consumerDocuments("discard-reads", workspace.DocumentLimits{Entries: 1, FileBytes: 4096, OpenBytes: 4096})
	if documents == nil {
		return
	}
	uri := source.URI("file:///discard.yaml")
	old, firstErr := documents.BeginRead(uri)
	newer, secondErr := documents.BeginRead(uri)
	discarded := documents.DiscardRead(old)
	current, err := documents.PublishRead(newer, "newest disk text")
	check("cancelled-old-read-keeps-new-ticket", firstErr == nil && secondErr == nil && !discarded && err == nil && current.Text() == "newest disk text")
	check("consumed-read-discard-is-noop", !documents.DiscardRead(newer))

	own, err := documents.BeginRead(uri)
	check("self-cancel-read-created", err == nil)
	_, err = documents.BeginRead("file:///next.yaml")
	check("outstanding-read-uses-entry", errors.Is(err, workspace.ErrLimit))
	check("own-read-discarded", documents.DiscardRead(own))
	next, err := documents.BeginRead("file:///next.yaml")
	check("self-cancel-releases-entry", err == nil)
	check("self-cancel-followup-cleanup", documents.DiscardRead(next))
	recreated, err := documents.BeginRead(uri)
	check("same-uri-after-discard-created", err == nil)
	discarded = documents.DiscardRead(own)
	current, err = documents.PublishRead(recreated, "after cancellation")
	check("repeated-discard-cannot-cancel-recreated-read", !discarded && err == nil && current.Text() == "after cancellation")

	beforeOpen, err := documents.BeginRead(uri)
	check("read-before-overlay-discard", err == nil)
	opened, err := documents.Open(uri, "unsaved overlay")
	check("overlay-before-discard-created", err == nil)
	discarded = documents.DiscardRead(beforeOpen)
	current, found := documents.Snapshot(uri)
	check("discard-keeps-authoritative-overlay", !discarded && found && current.Text() == opened.Text() && current.Revision() == opened.Revision())
	check("zero-read-discard-is-noop", !documents.DiscardRead(workspace.ReadTicket{}))
	check("discard-overlay-closed", documents.Close(uri))

	first := consumerDocuments("discard-first-store", workspace.DocumentLimits{Entries: 1, FileBytes: 4096, OpenBytes: 4096})
	second := consumerDocuments("discard-second-store", workspace.DocumentLimits{Entries: 1, FileBytes: 4096, OpenBytes: 4096})
	if first == nil || second == nil {
		return
	}
	foreign, firstErr := first.BeginRead(uri)
	local, secondErr := second.BeginRead(uri)
	discarded = second.DiscardRead(foreign)
	current, err = second.PublishRead(local, "local result")
	check("cross-store-discard-cannot-cancel-local-read", firstErr == nil && secondErr == nil && !discarded && err == nil && current.Text() == "local result")
	current, err = first.PublishRead(foreign, "foreign result")
	check("cross-store-discard-keeps-source-read", err == nil && current.Text() == "foreign result")
	documentReadFailureCases(documents)
}

func documentReadFailureCases(documents *workspace.Documents) {
	root := fixtureDirectory("discard-failures")
	if root == "" {
		return
	}
	defer os.RemoveAll(root)
	reads := 0
	_, err := readForConsumer(context.Background(), documents, "file:missing", filepath.Join(root, "missing"), 4096, &reads)
	check("consumer-read-error-reported", os.IsNotExist(err))
	next, err := documents.BeginRead("file:after-error")
	check("consumer-read-error-releases-own-ticket", err == nil && documents.DiscardRead(next))
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = readForConsumer(cancelled, documents, "file:cancelled", filepath.Join(root, "missing"), 4096, &reads)
	check("consumer-read-cancellation-reported", errors.Is(err, context.Canceled))
	next, err = documents.BeginRead("file:after-cancel")
	check("consumer-cancellation-releases-own-ticket", err == nil && documents.DiscardRead(next))
	check("bounded-read-fixture-created", writeFixture(root, "large", strings.Repeat("x", 4097)) == nil)
	_, err = readForConsumer(context.Background(), documents, "file:oversized", filepath.Join(root, "large"), 4096, &reads)
	check("consumer-read-bounded-before-publish", errors.Is(err, workspace.ErrLimit))
	next, err = documents.BeginRead("file:after-limit")
	check("bounded-read-failure-releases-own-ticket", err == nil && documents.DiscardRead(next))
}

func documentLimitCases() {
	_, err := workspace.NewDocuments(workspace.DocumentLimits{})
	check("zero-document-limits-rejected", err != nil)
	documents := consumerDocuments("document-limit", workspace.DocumentLimits{Entries: 4, FileBytes: 16, OpenBytes: 20})
	if documents == nil {
		return
	}
	first, err := documents.Open("file:///first", "abcdefghijklmnop")
	check("document-file-byte-boundary", err == nil)
	_, err = documents.Open("file:///second", "1234")
	check("document-open-byte-boundary", err == nil)
	_, err = documents.Open("file:///third", "x")
	check("document-open-byte-limit", errors.Is(err, workspace.ErrLimit))
	_, err = documents.Change("file:///first", first.Revision(), []source.Change{{Range: source.Span{Start: 16, End: 16}, Text: "q"}})
	current, found := documents.Snapshot("file:///first")
	check("document-file-limit-atomic", errors.Is(err, workspace.ErrLimit) && found && current.Text() == first.Text() && current.Revision() == first.Revision())
	check("document-limit-release", documents.Close("file:///second"))
	_, err = documents.Open("file:///third", "x")
	check("document-limit-recovers", err == nil)
	ticket, err := documents.BeginRead("file:///oversized")
	check("oversized-read-ticket", err == nil)
	_, err = documents.PublishRead(ticket, strings.Repeat("x", 17))
	check("disk-read-byte-limit", errors.Is(err, workspace.ErrLimit))
	entries := consumerDocuments("document-entry-limit", workspace.DocumentLimits{Entries: 1, FileBytes: 16, OpenBytes: 32})
	if entries == nil {
		return
	}
	_, err = entries.Open("file:///one", "a")
	check("document-entry-limit-first", err == nil)
	_, err = entries.Open("file:///two", "b")
	check("document-entry-limit-explicit", errors.Is(err, workspace.ErrLimit))
}

func queueCases() {
	_, err := workspace.NewQueue(0)
	check("zero-queue-capacity-rejected", err != nil)
	queue, err := workspace.NewQueue(2)
	check("queue-created", err == nil)
	if err != nil {
		return
	}
	err = queue.Put(workspace.Job{URI: "file:///current", Priority: workspace.Background})
	check("queue-background-added", err == nil)
	err = queue.Put(workspace.Job{URI: "file:///other", Priority: workspace.Related})
	check("queue-related-added", err == nil)
	err = queue.Put(workspace.Job{URI: "file:///current", Priority: workspace.Current})
	check("queue-promotion-at-capacity", err == nil && queue.Len() == 2)
	err = queue.Put(workspace.Job{URI: "file:///current", Priority: workspace.Background})
	check("queue-coalesces-uri", err == nil && queue.Len() == 2)
	err = queue.Put(workspace.Job{URI: "file:///current", Priority: workspace.Current})
	check("queue-repeated-notification-is-idempotent", err == nil && queue.Len() == 2)
	err = queue.Put(workspace.Job{URI: "file:///overflow", Priority: workspace.Background})
	check("queue-full-explicit", errors.Is(err, workspace.ErrLimit) && queue.Len() == 2)
	job, found := queue.Pop()
	check("queue-current-priority-preserved", found && job.URI == "file:///current" && job.Priority == workspace.Current)
	queue.Remove("file:///other")
	_, found = queue.Pop()
	check("queue-remove-clears-pending", !found && queue.Len() == 0)
	err = queue.Put(workspace.Job{URI: "", Priority: workspace.Current})
	check("queue-rejects-empty-uri", err != nil)
	err = queue.Put(workspace.Job{URI: "file:///invalid", Priority: workspace.Priority(3)})
	check("queue-rejects-invalid-priority", err != nil)
}

func queueFairnessCases() {
	queue, err := workspace.NewQueue(3)
	check("fair-queue-created", err == nil)
	if err != nil {
		return
	}
	jobs := []workspace.Job{
		{URI: "file:///current", Priority: workspace.Current},
		{URI: "file:///related", Priority: workspace.Related},
		{URI: "file:///background", Priority: workspace.Background},
	}
	for _, job := range jobs {
		check("fair-queue-input-"+string(job.URI), queue.Put(job) == nil)
	}
	counts := map[workspace.Priority]int{}
	valid := true
	for n := 0; n < 26; n++ {
		job, found := queue.Pop()
		if !found {
			valid = false
			break
		}
		counts[job.Priority]++
		valid = queue.Put(job) == nil && valid
	}
	check("background-progress-under-continuous-edits", valid && counts[workspace.Background] >= 2)
	check("related-progress-under-continuous-edits", valid && counts[workspace.Related] >= 8)
	check("current-work-retains-majority", valid && counts[workspace.Current] >= 16)
	workspaceEvidence["queueServiceCounts"] = map[string]int{"current": counts[workspace.Current], "related": counts[workspace.Related], "background": counts[workspace.Background]}
}

func fixtureDirectory(name string) string {
	root, err := os.MkdirTemp("", "toolkit-"+name+"-")
	check(name+"-fixture-directory", err == nil)
	return root
}

func writeFixture(root, relative, text string) error {
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(text), 0600)
}

func yamlScanner(root string, maxDirectories int) (*workspace.Scanner, error) {
	return workspace.NewScanner([]string{root}, workspace.ScanOptions{Extensions: []string{".yaml", ".yml"}, ExcludeDirs: []string{"vendor", ".git"}, MaxDirectories: maxDirectories})
}

type observedScan struct {
	files    []string
	issues   int
	visited  int
	maxBatch int
	done     bool
	complete bool
	err      error
}

func drainScanner(scanner *workspace.Scanner, maxWork int) observedScan {
	observed := observedScan{}
	for n := 0; n < 20000; n++ {
		batch, err := scanner.Next(context.Background(), maxWork)
		observed.files = append(observed.files, batch.Files...)
		observed.issues += len(batch.Issues)
		observed.visited += batch.Visited
		observed.maxBatch = max(observed.maxBatch, batch.Visited)
		if err != nil || batch.Done {
			observed.done, observed.complete, observed.err = batch.Done, batch.Complete, err
			return observed
		}
	}
	observed.err = fmt.Errorf("scanner did not finish after 20000 bounded batches")
	return observed
}

func scannerCases() {
	root := fixtureDirectory("scanner")
	if root == "" {
		return
	}
	defer os.RemoveAll(root)
	fixtures := map[string]string{
		"a.yaml": "name: a\n", "repo/one.yml": "name: one\n", "repo/two.yaml": "name: two\n",
		"readme.txt": "ordinary", "repo/binary.bin": "ordinary", "vendor/hidden.yaml": "ignored: true\n", ".git/hidden.yaml": "ignored: true\n",
	}
	for name, text := range fixtures {
		check("scanner-fixture-"+name, writeFixture(root, name, text) == nil)
	}
	scanner, err := yamlScanner(root, 32)
	check("scanner-created-without-traversal", err == nil)
	if err != nil {
		return
	}
	defer scanner.Close()
	check("scanner-late-fixture", writeFixture(root, "late.yaml", "name: late\n") == nil)
	_, err = scanner.Next(context.Background(), 0)
	check("scanner-zero-work-rejected", err != nil)
	_, err = scanner.Next(context.Background(), 4097)
	check("scanner-excess-work-rejected", err != nil)
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	batch, err := scanner.Next(cancelled, 1)
	check("scanner-cancel-not-complete", errors.Is(err, context.Canceled) && !batch.Done && !batch.Complete)
	observed := drainScanner(scanner, 1)
	want := []string{filepath.Join(root, "a.yaml"), filepath.Join(root, "late.yaml"), filepath.Join(root, "repo", "one.yml"), filepath.Join(root, "repo", "two.yaml")}
	sort.Strings(want)
	sort.Strings(observed.files)
	check("scanner-resumes-to-complete", observed.err == nil && observed.done && observed.complete && observed.issues == 0)
	check("scanner-candidates-exact-once", reflect.DeepEqual(observed.files, want))
	check("scanner-bounded-work", observed.maxBatch <= 1 && observed.visited > len(want))
	workspaceEvidence["smallScan"] = map[string]any{"visited": observed.visited, "files": len(observed.files), "maxBatchVisited": observed.maxBatch}
	scannerGenericPolicyCase(root)
	scannerSymlinkCase(root)
}

func scannerGenericPolicyCase(root string) {
	scanner, err := workspace.NewScanner([]string{root}, workspace.ScanOptions{MaxDirectories: 32})
	check("generic-scanner-created", err == nil)
	if err != nil {
		return
	}
	defer scanner.Close()
	observed := drainScanner(scanner, 8)
	check("scanner-no-hidden-yaml-or-directory-policy", observed.err == nil && observed.complete && len(observed.files) == 8)
	_, err = workspace.NewScanner([]string{root}, workspace.ScanOptions{Extensions: []string{"yaml"}, MaxDirectories: 32})
	check("scanner-invalid-extension-rejected", err != nil)
	_, err = workspace.NewScanner([]string{root}, workspace.ScanOptions{ExcludeDirs: []string{"../escape"}, MaxDirectories: 32})
	check("scanner-invalid-exclude-rejected", err != nil)
	_, err = workspace.NewScanner([]string{root}, workspace.ScanOptions{})
	check("scanner-invalid-directory-limit-rejected", err != nil)
	_, err = workspace.NewScanner([]string{""}, workspace.ScanOptions{MaxDirectories: 1})
	check("scanner-empty-root-rejected", err != nil)
}

func scannerSymlinkCase(root string) {
	linkFile := filepath.Join(root, "linked.yaml")
	linkDir := filepath.Join(root, "linked-dir")
	fileErr := os.Symlink(filepath.Join(root, "a.yaml"), linkFile)
	dirErr := os.Symlink(filepath.Join(root, "repo"), linkDir)
	if fileErr != nil || dirErr != nil {
		workspaceEvidence["symlinkCoverage"] = map[string]any{"status": "unavailable", "fileError": fmt.Sprint(fileErr), "directoryError": fmt.Sprint(dirErr)}
		check("symlink-fixture-or-windows-restriction", runtime.GOOS == "windows")
		return
	}
	scanner, err := yamlScanner(root, 32)
	check("symlink-scanner-created", err == nil)
	if err != nil {
		return
	}
	defer scanner.Close()
	observed := drainScanner(scanner, 8)
	linked := false
	for _, file := range observed.files {
		linked = linked || file == linkFile || strings.HasPrefix(file, linkDir+string(filepath.Separator))
	}
	check("symlinks-not-candidates-or-traversed", observed.err == nil && observed.done && observed.complete && !linked && len(observed.files) == 4)
	linkedRoot, err := yamlScanner(linkDir, 32)
	check("symlink-root-created-without-io", err == nil)
	if err != nil {
		return
	}
	defer linkedRoot.Close()
	linkedResult := drainScanner(linkedRoot, 8)
	check("symlink-root-explicitly-incomplete", linkedResult.err == nil && linkedResult.done && !linkedResult.complete && linkedResult.issues > 0 && len(linkedResult.files) == 0)
	workspaceEvidence["symlinkCoverage"] = map[string]any{"status": "executed", "candidates": len(observed.files)}
}

func scannerFailureCases() {
	root := fixtureDirectory("scanner-failures")
	if root == "" {
		return
	}
	defer os.RemoveAll(root)
	check("limited-root-fixture", writeFixture(root, "included.yaml", "name: included\n") == nil)
	for n := 0; n < 4; n++ {
		check(fmt.Sprintf("limited-subdir-%d", n), writeFixture(root, fmt.Sprintf("dir-%d/file.yaml", n), "name: hidden\n") == nil)
	}
	scanner, err := yamlScanner(root, 1)
	check("limited-scanner-created", err == nil)
	if err != nil {
		return
	}
	observed := drainScanner(scanner, 4)
	check("directory-limit-not-false-complete", observed.err == nil && observed.done && !observed.complete && observed.issues > 0)
	check("directory-limit-keeps-independent-files", reflect.DeepEqual(observed.files, []string{filepath.Join(root, "included.yaml")}))
	check("limited-scanner-closed", scanner.Close() == nil)

	missing, err := yamlScanner(filepath.Join(root, "does-not-exist"), 8)
	check("missing-root-constructor-does-not-read", err == nil)
	if err != nil {
		return
	}
	defer missing.Close()
	observed = drainScanner(missing, 2)
	check("missing-root-not-false-complete", observed.err == nil && observed.done && !observed.complete && observed.issues > 0)
	ordinary, err := yamlScanner(root, 16)
	check("priority-boundary-scanner-created", err == nil)
	if err != nil {
		return
	}
	defer ordinary.Close()
	check("priority-cannot-escape-root", ordinary.Prioritize(filepath.Dir(root)) != nil)
	check("priority-cannot-bypass-exclude", ordinary.Prioritize(filepath.Join(root, "vendor")) != nil)
	check("scanner-explicit-close", ordinary.Close() == nil)
	batch, err := ordinary.Next(context.Background(), 1)
	check("closed-scanner-not-complete", err != nil && !batch.Complete)
}

func readForConsumer(ctx context.Context, documents *workspace.Documents, uri source.URI, file string, maxBytes int64, reads *int) (source.Snapshot, error) {
	ticket, err := documents.BeginRead(uri)
	if errors.Is(err, workspace.ErrOverlay) {
		if snapshot, found := documents.Snapshot(uri); found {
			return snapshot, nil
		}
		return source.Snapshot{}, fmt.Errorf("overlay disappeared before the consumer read it")
	}
	if err != nil {
		return source.Snapshot{}, err
	}
	// Cleanup is scoped to this worker's ticket. A superseding read or overlay
	// may appear during I/O; an unconditional URI invalidation would revoke it.
	defer documents.DiscardRead(ticket)
	if err := ctx.Err(); err != nil {
		return source.Snapshot{}, err
	}
	*reads++
	text, err := boundedRead(ctx, file, maxBytes)
	if err != nil {
		return source.Snapshot{}, err
	}
	return documents.PublishRead(ticket, string(text))
}

func boundedRead(ctx context.Context, path string, maxBytes int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	text, readErr := io.ReadAll(io.LimitReader(file, maxBytes+1))
	closeErr := file.Close()
	if err := errors.Join(readErr, closeErr, ctx.Err()); err != nil {
		return nil, err
	}
	if int64(len(text)) > maxBytes {
		return nil, workspace.ErrLimit
	}
	return text, nil
}

func scannerInteractionCases() {
	root := fixtureDirectory("scanner-interaction")
	if root == "" {
		return
	}
	defer os.RemoveAll(root)
	for n := 0; n < 800; n++ {
		if writeFixture(root, fmt.Sprintf("bulk/%04d.yaml", n), "name: background\n") != nil || writeFixture(root, fmt.Sprintf("bulk/%04d.txt", n), "ordinary") != nil {
			check("large-fixture-created", false)
			return
		}
	}
	check("large-fixture-created", true)
	scanner, err := yamlScanner(root, 16)
	check("interaction-scanner-created", err == nil)
	if err != nil {
		return
	}
	defer scanner.Close()
	var prefixFiles []string
	prefixVisited := 0
	prefixMaxBatch := 0
	for n := 0; n < 32 && (prefixVisited < 512 || len(prefixFiles) == 0); n++ {
		batch, nextErr := scanner.Next(context.Background(), 64)
		if nextErr != nil || batch.Done {
			check("background-scan-started", false)
			return
		}
		prefixFiles = append(prefixFiles, batch.Files...)
		prefixVisited += batch.Visited
		prefixMaxBatch = max(prefixMaxBatch, batch.Visited)
	}
	prefixInBulk := true
	for _, file := range prefixFiles {
		prefixInBulk = prefixInBulk && strings.HasPrefix(file, filepath.Join(root, "bulk")+string(filepath.Separator))
	}
	check("background-scan-started", prefixVisited >= 512 && len(prefixFiles) > 0 && prefixInBulk)
	check("priority-fixture-created", writeFixture(root, "active/current.yaml", "name: disk\n") == nil)
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	interrupted, cancelErr := scanner.Next(cancelled, 2)
	check("started-scan-cancel-not-complete", errors.Is(cancelErr, context.Canceled) && !interrupted.Done && !interrupted.Complete)
	check("current-directory-prioritized", scanner.Prioritize(filepath.Join(root, "active")) == nil)
	first, err := scanner.Next(context.Background(), 2)
	activePath := filepath.Join(root, "active", "current.yaml")
	check("current-directory-served-before-bulk", err == nil && reflect.DeepEqual(first.Files, []string{activePath}) && !first.Done)

	queue, err := workspace.NewQueue(8)
	check("interaction-queue-created", err == nil)
	documents := consumerDocuments("interaction-documents", workspace.DocumentLimits{Entries: 4, FileBytes: 4096, OpenBytes: 8192})
	if err != nil || documents == nil {
		return
	}
	currentURI := source.URI("file:current-document")
	_, err = documents.Open(currentURI, "name: unsaved\n")
	check("interaction-overlay-created", err == nil)
	check("interaction-background-queued", queue.Put(workspace.Job{URI: "file:background", Priority: workspace.Background}) == nil)
	check("interaction-current-queued", queue.Put(workspace.Job{URI: currentURI, Priority: workspace.Current}) == nil)
	job, found := queue.Pop()
	reads := 0
	text, err := readForConsumer(context.Background(), documents, currentURI, filepath.Join(root, "not-present.yaml"), 4096, &reads)
	check("current-document-runs-during-discovery", found && job.URI == currentURI && err == nil && text.Text() == "name: unsaved\n" && reads == 0)
	check("interaction-overlay-closed", documents.Close(currentURI))
	text, err = readForConsumer(context.Background(), documents, currentURI, activePath, 4096, &reads)
	check("close-forces-new-disk-read", err == nil && text.Text() == "name: disk\n" && reads == 1)
	check("interaction-disk-edited", writeFixture(root, "active/current.yaml", "name: new-disk\n") == nil)
	text, err = readForConsumer(context.Background(), documents, currentURI, activePath, 4096, &reads)
	check("consumer-cannot-reuse-old-disk-text", err == nil && text.Text() == "name: new-disk\n" && reads == 2)
	remaining := drainScanner(scanner, 8)
	allFiles := append(prefixFiles, first.Files...)
	allFiles = append(allFiles, remaining.files...)
	seen := map[string]bool{}
	onlyCandidates := true
	for _, file := range allFiles {
		onlyCandidates = onlyCandidates && filepath.Ext(file) == ".yaml" && !seen[file]
		seen[file] = true
	}
	totalVisited := prefixVisited + first.Visited + remaining.visited
	check("large-scan-progress-preserved", remaining.err == nil && remaining.done && remaining.complete && len(allFiles) == 801 && onlyCandidates)
	check("ordinary-files-never-become-parse-jobs", totalVisited >= 1600 && len(allFiles) == 801)
	check("large-scan-work-stays-bounded", prefixMaxBatch <= 64 && first.Visited <= 2 && remaining.maxBatch <= 8)
	// About 1,608 units cover all entries and directory overhead. The margin
	// allows discovery of the new directory but excludes replaying the prefix.
	check("large-scan-does-not-restart-processed-prefix", totalVisited <= 1800)
	workspaceEvidence["largeScan"] = map[string]any{"candidates": len(allFiles), "ordinaryFiles": 800, "visited": totalVisited, "maxVisited": 1800, "prefixVisited": prefixVisited, "maxBatchVisited": remaining.maxBatch, "diskReads": reads}
}

type cancelAfterChecks struct {
	context.Context
	cancel    context.CancelFunc
	remaining int
}

func (c *cancelAfterChecks) Err() error {
	c.remaining--
	if c.remaining == 0 {
		c.cancel()
	}
	return c.Context.Err()
}

func scannerLifecycleCases() {
	root := fixtureDirectory("scanner-lifecycle")
	if root == "" {
		return
	}
	defer os.RemoveAll(root)
	valid := true
	for n := 0; n < 1200; n++ {
		valid = writeFixture(root, fmt.Sprintf("noise-%04d.txt", n), "noise") == nil && valid
	}
	valid = writeFixture(root, "current/target.yaml", "kind: Resource\n") == nil && valid
	related := filepath.Join(root, "current")
	target := filepath.Join(related, "target.yaml")
	want := []string{target}
	for n := 0; n < 3; n++ {
		path := fmt.Sprintf("background-%d/value.yaml", n)
		valid = writeFixture(root, path, "kind: Resource\n") == nil && valid
		want = append(want, filepath.Join(root, filepath.FromSlash(path)))
	}
	check("scanner-lifecycle-fixtures", valid)
	scanner, err := workspace.NewScanner([]string{root, related}, workspace.ScanOptions{Extensions: []string{".yaml"}, MaxDirectories: 32})
	check("scanner-lifecycle-created", err == nil)
	if err != nil {
		return
	}
	defer scanner.Close()
	initial, err := scanner.Next(context.Background(), 1)
	check("scanner-background-cursor-started-within-budget", err == nil && initial.Visited == 1 && !initial.Done)
	priorityErr := scanner.Prioritize(related)
	urgent, err := scanner.Next(context.Background(), 4)
	check("scanner-related-directory-within-four-units", priorityErr == nil && err == nil && len(urgent.Files) == 1 && urgent.Files[0] == target)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	partial, err := scanner.Next(&cancelAfterChecks{Context: ctx, cancel: cancel, remaining: 11}, 100)
	check("scanner-mid-scan-cancellation-explicit", errors.Is(err, context.Canceled) && partial.Visited > 0 && !partial.Complete)
	remaining := drainScanner(scanner, 31)
	files := append(append(append(initial.Files, urgent.Files...), partial.Files...), remaining.files...)
	sort.Strings(files)
	sort.Strings(want)
	check("scanner-overlapping-roots-resume-exactly", remaining.err == nil && remaining.done && remaining.complete && remaining.issues == 0 && len(initial.Issues)+len(urgent.Issues)+len(partial.Issues) == 0 && reflect.DeepEqual(files, want))
	check("scanner-non-yaml-work-remains-bounded", remaining.maxBatch <= 31)
	writeErr := writeFixture(root, "after-completion.yaml", "late: yes\n")
	priorityErr = scanner.Prioritize(root)
	again, err := scanner.Next(context.Background(), 10)
	check("scanner-completed-directory-not-requeued", writeErr == nil && priorityErr == nil && err == nil && again.Done && again.Complete && len(again.Files) == 0)

	missing := filepath.Join(root, "initially-missing")
	failed, err := workspace.NewScanner([]string{missing}, workspace.ScanOptions{MaxDirectories: 4})
	check("scanner-lifecycle-missing-created", err == nil)
	if err != nil {
		return
	}
	defer failed.Close()
	bad, err := failed.Next(context.Background(), 10)
	check("scanner-failed-directory-explicitly-incomplete", err == nil && bad.Done && !bad.Complete && len(bad.Issues) == 1)
	writeErr = writeFixture(missing, "new.yaml", "late: yes\n")
	priorityErr = failed.Prioritize(missing)
	repaired, err := failed.Next(context.Background(), 10)
	check("scanner-failed-directory-not-requeued-in-pass", writeErr == nil && priorityErr == nil && err == nil && repaired.Done && !repaired.Complete && len(repaired.Files) == 0)
}
