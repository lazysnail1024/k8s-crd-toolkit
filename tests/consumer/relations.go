package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"github.com/lazysnail1024/k8s-crd-toolkit/index"
	"github.com/lazysnail1024/k8s-crd-toolkit/source"
	"github.com/lazysnail1024/k8s-crd-toolkit/yamlview"
)

const applicationRelationYAML = `apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: portal
  namespace: argocd
spec:
  project: platform
  source:
    repoURL: https://example.invalid/config.git
    path: apps/portal
  destination:
    server: https://kubernetes.default.svc
    namespace: portal
`

const projectRelationYAML = `apiVersion: argoproj.io/v1alpha1
kind: AppProject
metadata:
  name: platform
  namespace: argocd
spec:
  sourceRepos: ["https://example.invalid/config.git"]
  destinations:
    - server: https://kubernetes.default.svc
      namespace: portal
`

const workflowRelationYAML = `apiVersion: argoproj.io/v1alpha1
kind: Workflow
metadata:
  name: build
  namespace: ci
spec:
  entrypoint: main
  templates:
    - name: main
      steps:
        - - name: invoke
            templateRef:
              name: shared
              template: run
            arguments:
              parameters:
                - name: image
                  value: alpine
`

const templateRelationYAML = `apiVersion: argoproj.io/v1alpha1
kind: WorkflowTemplate
metadata:
  name: shared
  namespace: ci
spec:
  templates:
    - name: run
      inputs:
        parameters:
          - name: image
      outputs:
        parameters:
          - name: result
            value: completed
      container:
        image: "{{inputs.parameters.image}}"
`

const applicationReferenceOnlyYAML = `apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  namespace: argocd
spec:
  project: platform
`

const workflowReferenceOnlyYAML = `apiVersion: argoproj.io/v1alpha1
kind: Workflow
metadata:
  namespace: ci
spec:
  workflowTemplateRef:
    name: shared
`

type relationInput struct {
	snapshot source.Snapshot
	facts    index.Facts
}

func relationKey(kind, namespace, name string) index.Key {
	return index.Key(kind + "\x00" + namespace + "\x00" + name)
}

func relationNode(root yamlview.Node, path ...string) yamlview.Node {
	for _, field := range path {
		var next yamlview.Node
		for _, member := range root.Members() {
			if member.Key.Raw() == field {
				next = member.Value
				break
			}
		}
		root = next
	}
	return root
}

// This deliberately extracts only plain, fixed fields used by the four fixtures
// above. It is a consumer of public syntax locations, not an Argo validator or
// a namespace, alias, inheritance, or dynamic-reference implementation.
func parseRelation(uri source.URI, text string, revision source.Revision) (relationInput, error) {
	snapshot, err := source.New(uri, revision, text)
	if err != nil {
		return relationInput{}, err
	}
	view, err := yamlview.Parse(context.Background(), snapshot)
	if err != nil {
		return relationInput{}, err
	}
	documents := view.Documents()
	if len(documents) != 1 || !documents[0].Complete() {
		return relationInput{}, fmt.Errorf("fixture must contain one complete document")
	}
	root := documents[0].Root()
	kind := relationNode(root, "kind").Raw()
	name := relationNode(root, "metadata", "name")
	namespace := relationNode(root, "metadata", "namespace").Raw()
	if relationNode(root, "apiVersion").Raw() != "argoproj.io/v1alpha1" || name.Raw() == "" || namespace == "" {
		return relationInput{}, fmt.Errorf("fixture identity is missing")
	}
	facts := index.Facts{Symbols: []index.Symbol{{ID: "resource", Key: relationKey(kind, namespace, name.Raw()), Name: name.Raw(), Kind: kind, Span: root.Span(), Selection: name.Span()}}}
	semantic := kind + "\x00" + namespace + "\x00" + name.Raw()
	switch kind {
	case "Application":
		project := relationNode(root, "spec", "project")
		if project.Raw() == "" {
			return relationInput{}, fmt.Errorf("fixture project is missing")
		}
		facts.References = []index.Reference{{Target: relationKey("AppProject", namespace, project.Raw()), Span: project.Span()}}
		semantic += "\x00" + project.Raw()
	case "AppProject":
	case "Workflow", "WorkflowTemplate":
		part, err := relationTemplate(root, kind, namespace, name.Raw(), &facts)
		if err != nil {
			return relationInput{}, err
		}
		semantic += "\x00" + part
	default:
		return relationInput{}, fmt.Errorf("unsupported fixture kind %q", kind)
	}
	facts.SemanticDigest = sha256.Sum256([]byte(semantic))
	return relationInput{snapshot, facts}, nil
}

func relationTemplate(root yamlview.Node, kind, namespace, resource string, facts *index.Facts) (string, error) {
	templates := relationNode(root, "spec", "templates").Elements()
	if len(templates) != 1 {
		return "", fmt.Errorf("fixture requires one template")
	}
	template := templates[0]
	name := relationNode(template, "name")
	if name.Raw() == "" {
		return "", fmt.Errorf("fixture template name is missing")
	}
	key := relationKey(kind+"/template", namespace, resource+"/"+name.Raw())
	facts.Symbols = append(facts.Symbols, index.Symbol{ID: "template", Key: key, Name: name.Raw(), Kind: "template", Span: template.Span(), Selection: name.Span()})
	if kind == "WorkflowTemplate" {
		return name.Raw() + "\x00" + relationParameterNames(template, "inputs") + "\x00" + relationParameterNames(template, "outputs"), nil
	}
	groups := relationNode(template, "steps").Elements()
	if len(groups) != 1 || len(groups[0].Elements()) != 1 {
		return "", fmt.Errorf("fixture requires one step")
	}
	ref := relationNode(groups[0].Elements()[0], "templateRef")
	resourceName, templateName := relationNode(ref, "name"), relationNode(ref, "template")
	if resourceName.Raw() == "" || templateName.Raw() == "" {
		return "", fmt.Errorf("fixture template reference is missing")
	}
	facts.References = []index.Reference{
		{Target: relationKey("WorkflowTemplate", namespace, resourceName.Raw()), Span: resourceName.Span()},
		{Target: relationKey("WorkflowTemplate/template", namespace, resourceName.Raw()+"/"+templateName.Raw()), Span: templateName.Span()},
	}
	return name.Raw() + "\x00" + resourceName.Raw() + "\x00" + templateName.Raw(), nil
}

func relationParameterNames(template yamlview.Node, direction string) string {
	names := []string{}
	for _, parameter := range relationNode(template, direction, "parameters").Elements() {
		names = append(names, relationNode(parameter, "name").Raw())
	}
	return strings.Join(names, "\x00")
}

func putRelation(i *index.Index, input relationInput) (index.Change, error) {
	return commitIndexFile(i, input.snapshot.URI(), "fixture-partition", input.snapshot.Revision(), input.facts)
}

func relationCases() {
	for _, fixture := range []struct {
		name, caller, definition, expected string
		key                                index.Key
	}{
		{"cd", applicationRelationYAML, projectRelationYAML, "platform", relationKey("AppProject", "argocd", "platform")},
		{"wf", workflowRelationYAML, templateRelationYAML, "run", relationKey("WorkflowTemplate/template", "ci", "shared/run")},
	} {
		caller, firstErr := parseRelation(source.URI("file:///"+fixture.name+"-caller.yaml"), fixture.caller, 1)
		definition, secondErr := parseRelation(source.URI("file:///"+fixture.name+"-definition.yaml"), fixture.definition, 1)
		check(fixture.name+"-real-yaml-parsed", firstErr == nil && secondErr == nil)
		if firstErr != nil || secondErr != nil {
			continue
		}
		i := generousIndex(fixture.name + "-relation")
		if i == nil {
			continue
		}
		relationLifecycle(i, fixture.name, caller, definition, fixture.key, fixture.expected)
		if fixture.name == "wf" {
			relationTemplateChanges(i, caller, definition)
		}
		change, err := i.Remove(definition.snapshot.URI())
		definitions, _ := i.Definitions("fixture-partition", fixture.key)
		dependents, _ := i.Dependents("fixture-partition", fixture.key)
		check(fixture.name+"-target-delete-restores-unresolved", err == nil && len(definitions) == 0 && exactAffected(change, caller.snapshot.URI(), definition.snapshot.URI()) && reflect.DeepEqual(dependents, []source.URI{caller.snapshot.URI()}))
	}
}

// These editor drafts have an exact navigable use but no declaration name.
// No owner symbol is invented merely to store the reference.
func parseReferenceOnlyFixture(uri source.URI, text string, revision source.Revision) (relationInput, error) {
	snapshot, err := source.New(uri, revision, text)
	if err != nil {
		return relationInput{}, err
	}
	view, err := yamlview.Parse(context.Background(), snapshot)
	if err != nil {
		return relationInput{}, err
	}
	documents := view.Documents()
	if len(documents) != 1 || !documents[0].Complete() {
		return relationInput{}, fmt.Errorf("reference fixture must be syntactically complete")
	}
	root := documents[0].Root()
	namespace := relationNode(root, "metadata", "namespace").Raw()
	if namespace == "" || relationNode(root, "metadata", "name").Raw() != "" || len(relationNode(root, "spec", "templates").Elements()) != 0 {
		return relationInput{}, fmt.Errorf("reference fixture must have no named declarations")
	}
	var targetKind string
	var use yamlview.Node
	switch relationNode(root, "kind").Raw() {
	case "Application":
		targetKind, use = "AppProject", relationNode(root, "spec", "project")
	case "Workflow":
		targetKind, use = "WorkflowTemplate", relationNode(root, "spec", "workflowTemplateRef", "name")
	default:
		return relationInput{}, fmt.Errorf("unsupported reference-only fixture")
	}
	if use.Raw() == "" {
		return relationInput{}, fmt.Errorf("reference fixture must contain a use")
	}
	target := relationKey(targetKind, namespace, use.Raw())
	facts := index.Facts{SemanticDigest: sha256.Sum256([]byte(target)), References: []index.Reference{{Target: target, Span: use.Span()}}}
	return relationInput{snapshot, facts}, nil
}

func referenceOnlyCases() {
	for _, fixture := range []struct {
		name, text, definition, expected string
		key                              index.Key
	}{
		{"cd-reference-only", applicationReferenceOnlyYAML, projectRelationYAML, "platform", relationKey("AppProject", "argocd", "platform")},
		{"wf-reference-only", workflowReferenceOnlyYAML, templateRelationYAML, "shared", relationKey("WorkflowTemplate", "ci", "shared")},
	} {
		caller, callerErr := parseReferenceOnlyFixture(source.URI("file:///"+fixture.name+"-caller.yaml"), fixture.text, 1)
		definition, definitionErr := parseRelation(source.URI("file:///"+fixture.name+"-definition.yaml"), fixture.definition, 1)
		check(fixture.name+"-fixture-parsed", callerErr == nil && definitionErr == nil && len(caller.facts.Symbols) == 0 && len(caller.facts.References) == 1)
		if callerErr != nil || definitionErr != nil {
			continue
		}
		i := generousIndex(fixture.name)
		if i == nil {
			continue
		}
		relationLifecycle(i, fixture.name, caller, definition, fixture.key, fixture.expected)
		file, found, _ := i.GetFile(caller.snapshot.URI())
		symbols, err := i.Symbols("fixture-partition", "", 1)
		check(fixture.name+"-no-synthetic-owner-in-symbol-search", found && file.Fresh && len(file.Facts.Symbols) == 0 && err == nil && len(symbols.Items) == 1 && symbols.Items[0].URI == definition.snapshot.URI() && symbols.Items[0].Symbol.Name == fixture.expected && symbols.Truncated == (len(definition.facts.Symbols) > 1))
		shifted, err := parseReferenceOnlyFixture(caller.snapshot.URI(), "# editor comment\n"+fixture.text, 2)
		check(fixture.name+"-comment-fixture-parsed", err == nil)
		if err != nil {
			continue
		}
		change, err := putRelation(i, shifted)
		uses, _ := i.References("fixture-partition", fixture.key)
		check(fixture.name+"-position-refresh-remains-local", err == nil && !change.SemanticChanged && exactAffected(change, caller.snapshot.URI()) && len(uses) == 1 && uses[0].Revision == 2 && relationSlice(shifted.snapshot, uses[0].Reference.Span, fixture.expected))
		change, err = i.Remove(definition.snapshot.URI())
		definitions, _ := i.Definitions("fixture-partition", fixture.key)
		dependents, _ := i.Dependents("fixture-partition", fixture.key)
		check(fixture.name+"-target-delete-keeps-unresolved-use", err == nil && len(definitions) == 0 && exactAffected(change, caller.snapshot.URI(), definition.snapshot.URI()) && reflect.DeepEqual(dependents, []source.URI{caller.snapshot.URI()}))
		_, err = i.Remove(caller.snapshot.URI())
		uses, _ = i.References("fixture-partition", fixture.key)
		dependents, _ = i.Dependents("fixture-partition", fixture.key)
		check(fixture.name+"-caller-delete-removes-use", err == nil && len(uses) == 0 && len(dependents) == 0)
	}
}

func relationLifecycle(i *index.Index, name string, caller, definition relationInput, key index.Key, expected string) {
	_, err := putRelation(i, caller)
	definitions, _ := i.Definitions("fixture-partition", key)
	uses, _ := i.References("fixture-partition", key)
	check(name+"-real-yaml-unresolved-use", err == nil && len(definitions) == 0 && len(uses) == 1 && relationSlice(caller.snapshot, uses[0].Reference.Span, expected))
	change, err := putRelation(i, definition)
	definitions, _ = i.Definitions("fixture-partition", key)
	check(name+"-real-yaml-target-arrives", err == nil && change.SemanticChanged && exactAffected(change, caller.snapshot.URI(), definition.snapshot.URI()) && len(definitions) == 1 && relationSlice(definition.snapshot, definitions[0].Symbol.Selection, expected))
	duplicate, parseErr := parseRelation(source.URI("file:///"+name+"-duplicate.yaml"), definition.snapshot.Text(), 1)
	check(name+"-duplicate-fixture-parsed", parseErr == nil)
	if parseErr != nil {
		return
	}
	change, err = putRelation(i, duplicate)
	definitions, _ = i.Definitions("fixture-partition", key)
	check(name+"-real-yaml-ambiguity-retained", err == nil && len(definitions) == 2 && exactAffected(change, caller.snapshot.URI(), duplicate.snapshot.URI()))
	change, err = i.Remove(duplicate.snapshot.URI())
	definitions, _ = i.Definitions("fixture-partition", key)
	check(name+"-real-yaml-ambiguity-removed", err == nil && len(definitions) == 1 && definitions[0].URI == definition.snapshot.URI() && exactAffected(change, caller.snapshot.URI(), duplicate.snapshot.URI()))
}

func relationSlice(snapshot source.Snapshot, span source.Span, expected string) bool {
	text, err := snapshot.Slice(span)
	position, positionErr := snapshot.Position(span.Start)
	offset, offsetErr := snapshot.Offset(position)
	return err == nil && text == expected && positionErr == nil && offsetErr == nil && offset == span.Start
}

func relationTemplateChanges(i *index.Index, caller, original relationInput) {
	text := strings.Replace(original.snapshot.Text(), "name: image", "name: image2", 1)
	for n, item := range []struct{ name, text string }{
		{"input", text},
		{"output", strings.Replace(text, "name: result", "name: report", 1)},
	} {
		updated, parseErr := parseRelation(original.snapshot.URI(), item.text, source.Revision(n+2))
		check("wf-"+item.name+"-fixture-parsed", parseErr == nil)
		if parseErr != nil {
			return
		}
		change, err := putRelation(i, updated)
		check("wf-"+item.name+"-signature-wakes-caller", err == nil && change.SemanticChanged && exactAffected(change, caller.snapshot.URI(), original.snapshot.URI()))
		text = item.text
	}
	key := relationKey("WorkflowTemplate/template", "ci", "shared/run")
	before, _ := i.Definitions("fixture-partition", key)
	shifted, parseErr := parseRelation(original.snapshot.URI(), "# comment one\n# comment two\n"+text, 4)
	check("wf-comment-fixture-parsed", parseErr == nil)
	if parseErr != nil {
		return
	}
	change, err := putRelation(i, shifted)
	after, _ := i.Definitions("fixture-partition", key)
	check("wf-comment-keeps-caller-semantic-cache", err == nil && !change.SemanticChanged && exactAffected(change, original.snapshot.URI()))
	check("wf-comment-refreshes-real-source-location", len(before) == 1 && len(after) == 1 && after[0].Revision == 4 && after[0].Symbol.Selection.Start > before[0].Symbol.Selection.Start && relationSlice(shifted.snapshot, after[0].Symbol.Selection, "run"))
}

func concurrentFacts(revision source.Revision) index.Facts {
	label := strconv.FormatUint(uint64(revision), 10)
	facts := definitionFacts("concurrent", label, int(revision))
	facts.Symbols[0].Name = label
	facts.References = []index.Reference{{Target: index.Key("target-" + label), Span: source.Span{Start: int(revision) + 10, End: int(revision) + 14}}}
	facts.Dependencies = []index.Key{index.Key("dependency-" + label)}
	return facts
}

func consistentFile(i *index.Index, uri source.URI) bool {
	file, found, _ := i.GetFile(uri)
	if !found {
		return false
	}
	if !file.Fresh {
		return len(file.Facts.Symbols) == 0 && len(file.Facts.References) == 0 && len(file.Facts.Dependencies) == 0
	}
	return reflect.DeepEqual(file.Facts, concurrentFacts(file.Revision))
}

func indexConcurrencyCases() {
	i := generousIndex("concurrent-index")
	if i == nil {
		return
	}
	uris := []source.URI{"file:///concurrent-a.yaml", "file:///concurrent-b.yaml"}
	first, firstErr := i.Begin(uris[0], "concurrent", 1)
	second, secondErr := i.Begin(uris[1], "concurrent", 1)
	_, commitFirst := i.Commit(first, concurrentFacts(1))
	_, commitSecond := i.Commit(second, concurrentFacts(1))
	check("different-uris-do-not-invalidate-tickets", firstErr == nil && secondErr == nil && commitFirst == nil && commitSecond == nil)
	if firstErr != nil || secondErr != nil || commitFirst != nil || commitSecond != nil {
		return
	}
	start, done := make(chan struct{}), make(chan struct{})
	ready := make(chan struct{}, 4)
	outcomes := make(chan bool, 6)
	var writers sync.WaitGroup
	for _, uri := range uris {
		writers.Add(1)
		go func() {
			defer writers.Done()
			<-start
			ok := true
			for revision := source.Revision(2); revision <= 61; revision++ {
				_, err := commitIndexFile(i, uri, "concurrent", revision, concurrentFacts(revision))
				ok = ok && err == nil
				runtime.Gosched()
			}
			outcomes <- ok
		}()
	}
	for n := 0; n < 4; n++ {
		uri := uris[n%len(uris)]
		go func() {
			ok := consistentFile(i, uri)
			ready <- struct{}{}
			<-start
			for {
				select {
				case <-done:
					outcomes <- ok && consistentFile(i, uri)
					return
				default:
					ok = consistentFile(i, uri) && ok
					runtime.Gosched()
				}
			}
		}()
	}
	for n := 0; n < 4; n++ {
		<-ready
	}
	close(start)
	writers.Wait()
	close(done)
	ok := true
	for n := 0; n < 6; n++ {
		ok = <-outcomes && ok
	}
	check("concurrent-fresh-facts-are-one-atomic-batch", ok)
	for n, uri := range uris {
		file, found, state := i.GetFile(uri)
		check(fmt.Sprintf("concurrent-final-file-%d", n), found && file.Fresh && file.Revision == 61 && state.Pending == 0 && consistentFile(i, uri))
	}
}
