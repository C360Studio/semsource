package cfgfile_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/c360studio/semsource/handler"
	source "github.com/c360studio/semsource/source/vocabulary"
)

// titledStates returns every state of entityType titled title, one entry per
// state: two states under one ID stay two entries, which is the #375 defect.
func titledStates(states []*handler.EntityState, entityType, title string) []*handler.EntityState {
	var out []*handler.EntityState
	for _, s := range collectByType(states, entityType) {
		if tripleObject(s, source.DcTitle) == title {
			out = append(out, s)
		}
	}
	return out
}

// tripleValues returns every value of pred on state, in triple order.
func tripleValues(state *handler.EntityState, pred string) []any {
	var out []any
	for _, tr := range state.Triples {
		if tr.Predicate == pred {
			out = append(out, tr.Object)
		}
	}
	return out
}

// edgeCount counts the triples in states whose object is id under pred.
func edgeCount(states []*handler.EntityState, pred, id string) int {
	n := 0
	for _, s := range states {
		for _, tr := range s.Triples {
			if tr.Predicate == pred && tr.Object == id {
				n++
			}
		}
	}
	return n
}

// oneFolded asserts exactly one state of entityType titled title, named once
// by its manifest's dependency edges, and returns it.
func oneFolded(t *testing.T, states []*handler.EntityState, entityType, title string) *handler.EntityState {
	t.Helper()
	got := titledStates(states, entityType, title)
	if len(got) != 1 {
		t.Fatalf("want one %s state titled %q, got %d", entityType, title, len(got))
	}
	// npm packages point at deps with depends, every other manifest with requires.
	if n := edgeCount(states, source.ConfigRequires, got[0].ID) + edgeCount(states, source.ConfigDepends, got[0].ID); n != 1 {
		t.Errorf("dependency edges to %s = %d, want 1", got[0].ID, n)
	}
	return got[0]
}

// spec: openspec/specs/entity-identity-safety/spec.md#Requirement: Declarations of one artifact within a manifest fold into one entity
func TestIngestEntityStates_GradleConfigurationsFold(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{"build.gradle": "dependencies {\n" +
		"    testImplementation 'org.slf4j:slf4j-api:2.0.9'\n" +
		"    implementation 'org.slf4j:slf4j-api:2.0.9'\n}\n"})
	dep := oneFolded(t, ingestTree(t, root), "dependency", "org.slf4j:slf4j-api")
	got := tripleValues(dep, source.ConfigDepConfiguration)
	if want := []any{"implementation", "testImplementation"}; !slices.Equal(got, want) {
		t.Errorf("configurations = %v, want %v", got, want)
	}
}

// spec: openspec/specs/entity-identity-safety/spec.md#Requirement: Declarations of one artifact within a manifest fold into one entity
func TestIngestEntityStates_MavenScopesFold(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{"pom.xml": `<project><groupId>com.acme</groupId><artifactId>a</artifactId><version>1</version><dependencies>
<dependency><groupId>junit</groupId><artifactId>junit</artifactId><version>4.13.2</version><scope>test</scope></dependency>
<dependency><groupId>junit</groupId><artifactId>junit</artifactId><version>4.13.2</version></dependency>
</dependencies></project>`})
	dep := oneFolded(t, ingestTree(t, root), "dependency", "junit:junit")
	got := tripleValues(dep, source.ConfigDepScope)
	if want := []any{"compile", "test"}; !slices.Equal(got, want) {
		t.Errorf("scopes = %v, want %v", got, want)
	}
}

// spec: openspec/specs/entity-identity-safety/spec.md#Requirement: Declarations of one artifact within a manifest fold into one entity
func TestIngestEntityStates_MavenTestJarIsItsOwnEntity(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{"pom.xml": `<project><groupId>com.acme</groupId><artifactId>a</artifactId><version>1</version><dependencies>
<dependency><groupId>org.acme</groupId><artifactId>core</artifactId><version>1.0</version></dependency>
<dependency><groupId>org.acme</groupId><artifactId>core</artifactId><version>1.0</version><type>test-jar</type><scope>test</scope></dependency>
</dependencies></project>`})
	deps := titledStates(ingestTree(t, root), "dependency", "org.acme:core")
	if len(deps) != 2 || deps[0].ID == deps[1].ID {
		t.Fatalf("want two distinct org.acme:core IDs, got %d states", len(deps))
	}
	// The plain jar's instance as minted before <type> was parsed (pinned at
	// main 61edfd7); the system segment is the temp root's, so compare the tail.
	const plain = ".config.dependency.a4999690-9c5fb3d1"
	if !slices.ContainsFunc(deps, func(s *handler.EntityState) bool { return strings.HasSuffix(s.ID, plain) }) {
		t.Errorf("plain jar ID changed: got %s and %s, want one ending %s", deps[0].ID, deps[1].ID, plain)
	}
	if !slices.ContainsFunc(deps, func(s *handler.EntityState) bool {
		return slices.Equal(tripleValues(s, source.ConfigDepType), []any{"test-jar"})
	}) {
		t.Error("no org.acme:core state carries dependency-type test-jar")
	}
}

// spec: openspec/specs/entity-identity-safety/spec.md#Requirement: Declarations of one artifact within a manifest fold into one entity
func TestIngestEntityStates_MavenClassifierIsItsOwnEntityAndJarIsPlain(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{"pom.xml": `<project><groupId>com.acme</groupId><artifactId>a</artifactId><version>1</version><dependencies>
<dependency><groupId>org.acme</groupId><artifactId>core</artifactId><version>1.0</version></dependency>
<dependency><groupId>org.acme</groupId><artifactId>core</artifactId><version>1.0</version><type>jar</type></dependency>
<dependency><groupId>org.acme</groupId><artifactId>core</artifactId><version>1.0</version><classifier>tests</classifier></dependency>
</dependencies></project>`})
	deps := titledStates(ingestTree(t, root), "dependency", "org.acme:core")
	if len(deps) != 2 || deps[0].ID == deps[1].ID {
		t.Fatalf("want an explicit jar folded into the plain one and the classifier apart: got %d states", len(deps))
	}
	var classified int
	for _, s := range deps {
		if slices.Equal(tripleValues(s, source.ConfigDepClassifier), []any{"tests"}) {
			classified++
		}
		if len(tripleValues(s, source.ConfigDepType)) != 0 {
			t.Errorf("%s carries a dependency-type; jar is the default and is not emitted", s.ID)
		}
	}
	if classified != 1 {
		t.Errorf("states carrying classifier tests = %d, want 1", classified)
	}
}

// spec: openspec/specs/entity-identity-safety/spec.md#Requirement: Declarations of one artifact within a manifest fold into one entity
func TestIngestEntityStates_NPMProdAndDevFoldToProd(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{"package.json": `{"name":"app","version":"1.0.0",
"dependencies":{"lodash":"^4.17.21"},"devDependencies":{"lodash":"^4.17.21"}}`})
	dep := oneFolded(t, ingestTree(t, root), "dependency", "lodash")
	if got := tripleValues(dep, source.ConfigDepKind); !slices.Equal(got, []any{"npm-prod"}) {
		t.Errorf("kind = %v, want [npm-prod]", got)
	}
}

// spec: openspec/specs/entity-identity-safety/spec.md#Requirement: Declarations of one artifact within a manifest fold into one entity
func TestIngestEntityStates_GoIndirectOnlyWhenEveryRequireIs(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{"go.mod": "module example.com/a\n\ngo 1.23\n\n" +
		"require golang.org/x/text v0.14.0 // indirect\nrequire golang.org/x/text v0.14.0\n"})
	dep := oneFolded(t, ingestTree(t, root), "dependency", "golang.org/x/text")
	if got := tripleValues(dep, source.ConfigDepIndirect); !slices.Equal(got, []any{"false"}) {
		t.Errorf("indirect = %v, want [false]", got)
	}
}

// spec: openspec/specs/entity-identity-safety/spec.md#Requirement: Declarations of one artifact within a manifest fold into one entity
func TestIngestEntityStates_ImagesThatSlugifyAlikeStayDistinct(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{"Dockerfile": "FROM foo/bar:1 AS a\nFROM foo-bar:1\n"})
	states := ingestTree(t, root)
	a, b := titledStates(states, "image", "foo/bar:1"), titledStates(states, "image", "foo-bar:1")
	if len(a) != 1 || len(b) != 1 {
		t.Fatalf("want one state per image, got %d and %d", len(a), len(b))
	}
	if a[0].ID == b[0].ID {
		t.Errorf("foo/bar:1 and foo-bar:1 share ID %s", a[0].ID)
	}
}

// spec: openspec/specs/entity-identity-safety/spec.md#Requirement: Declarations of one artifact within a manifest fold into one entity
func TestIngestEntityStates_POMModulesThatSlugifyAlikeStayDistinct(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{"pom.xml": `<project><groupId>g</groupId><artifactId>app</artifactId><version>1</version>
<modules><module>my_module</module><module>my-module</module><module>my-module</module></modules></project>`})
	states := ingestTree(t, root)
	a, b := titledStates(states, "gomod", "my_module"), titledStates(states, "gomod", "my-module")
	if len(a) != 1 || len(b) != 1 {
		t.Fatalf("want one state per module, got %d and %d", len(a), len(b))
	}
	if a[0].ID == b[0].ID {
		t.Errorf("my_module and my-module share ID %s", a[0].ID)
	}
	if n := edgeCount(states, source.ConfigContains, b[0].ID); n != 1 {
		t.Errorf("module edges to the repeated my-module = %d, want 1", n)
	}
}
