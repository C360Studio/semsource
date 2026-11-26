package cfgfile_test

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	source "github.com/c360studio/semsource/source/vocabulary"
)

// A version bump re-ingests the same manifest with a new version. The
// dependency keeps its ID and carries only the new version (#393).
// spec: entity-identity-safety / Config-file dependency and image identity excludes the version
func TestIngestEntityStates_VersionBumpKeepsTheDependencyID(t *testing.T) {
	for _, tc := range []struct {
		name, file, before, after, title, oldV, newV string
		// instance is the pinned ID tail: hash(coordinate)-hash(declaring file).
		instance string
	}{
		{"go", "go.mod",
			"module example.com/a\n\ngo 1.23\n\nrequire github.com/a/b v1.2.0\n",
			"module example.com/a\n\ngo 1.23\n\nrequire github.com/a/b v1.3.0\n",
			"github.com/a/b", "v1.2.0", "v1.3.0", "b703f2af-33ef32bf"},
		{"npm", "package.json",
			`{"name":"app","version":"1.0.0","dependencies":{"lodash":"^4.17.20"}}`,
			`{"name":"app","version":"1.0.0","dependencies":{"lodash":"^4.17.21"}}`,
			"lodash", "^4.17.20", "^4.17.21", "94a75553-7ae45ad1"},
		{"maven", "pom.xml",
			`<project><groupId>g</groupId><artifactId>a</artifactId><version>1</version><dependencies><dependency><groupId>junit</groupId><artifactId>junit</artifactId><version>4.13.1</version></dependency></dependencies></project>`,
			`<project><groupId>g</groupId><artifactId>a</artifactId><version>1</version><dependencies><dependency><groupId>junit</groupId><artifactId>junit</artifactId><version>4.13.2</version></dependency></dependencies></project>`,
			"junit:junit", "4.13.1", "4.13.2", "5a975c72-9c5fb3d1"},
		{"gradle", "build.gradle",
			"dependencies {\n    implementation 'org.slf4j:slf4j-api:2.0.8'\n}\n",
			"dependencies {\n    implementation 'org.slf4j:slf4j-api:2.0.9'\n}\n",
			"org.slf4j:slf4j-api", "2.0.8", "2.0.9", "b70f653c-49a96e7e"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeTree(t, root, map[string]string{tc.file: tc.before})
			before := oneFolded(t, ingestTree(t, root), "dependency", tc.title)

			writeTree(t, root, map[string]string{tc.file: tc.after})
			after := oneFolded(t, ingestTree(t, root), "dependency", tc.title)

			if before.ID != after.ID {
				t.Errorf("version bump changed the ID: %s -> %s", before.ID, after.ID)
			}
			if !strings.HasSuffix(after.ID, ".config.dependency."+tc.instance) {
				t.Errorf("ID = %s, want one ending .config.dependency.%s", after.ID, tc.instance)
			}
			if got := tripleValues(after, source.ConfigDepVersion); !slices.Equal(got, []any{tc.newV}) {
				t.Errorf("versions after the bump = %v, want [%s] (not %s)", got, tc.newV, tc.oldV)
			}
		})
	}
}

// One manifest declaring one coordinate at two versions is one entity carrying
// both versions, sorted (#393).
// spec: entity-identity-safety / Config-file dependency and image identity excludes the version
func TestIngestEntityStates_TwoVersionsOfOneCoordinateFold(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{"build.gradle": "dependencies {\n" +
		"    testImplementation 'org.slf4j:slf4j-api:2.0.9'\n" +
		"    implementation 'org.slf4j:slf4j-api:1.7.36'\n}\n"})
	dep := oneFolded(t, ingestTree(t, root), "dependency", "org.slf4j:slf4j-api")
	if got, want := tripleValues(dep, source.ConfigDepVersion), []any{"1.7.36", "2.0.9"}; !slices.Equal(got, want) {
		t.Errorf("versions = %v, want %v", got, want)
	}
	if got, want := tripleValues(dep, source.ConfigDepConfiguration), []any{"implementation", "testImplementation"}; !slices.Equal(got, want) {
		t.Errorf("configurations = %v, want %v", got, want)
	}
}

// spec: entity-identity-safety / Config-file dependency and image identity excludes the version
func TestIngestEntityStates_ImageTagsDoNotSplitAnImage(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{"Dockerfile": "FROM golang:1.22 AS build\nFROM golang:1.23\n"})
	images := titledStates(ingestTree(t, root), "image", "golang")
	if len(images) != 1 {
		t.Fatalf("want one image titled golang, got %d", len(images))
	}
	if want := ".config.image.golang-d754ed9f-dd2c0eb6"; !strings.HasSuffix(images[0].ID, want) {
		t.Errorf("ID = %s, want one ending %s", images[0].ID, want)
	}
	if got, want := tripleValues(images[0], source.ConfigImageName), []any{"golang:1.22", "golang:1.23"}; !slices.Equal(got, want) {
		t.Errorf("image references = %v, want %v", got, want)
	}
}

// spec: entity-identity-safety / Config-file dependency and image identity excludes the version
func TestIngestEntityStates_ImageNameKeepsRegistryPortAndDropsDigest(t *testing.T) {
	for _, tc := range []struct{ name, ref, title, idTail string }{
		// 79d2ca39 = hash("registry:5000/team/app"), dd2c0eb6 = hash("Dockerfile").
		{"registry port is not a tag", "registry:5000/team/app:1.0", "registry:5000/team/app", ".registry-5000-team-app-79d2ca39-dd2c0eb6"},
		{"registry port without a tag", "registry:5000/team/app", "registry:5000/team/app", ".registry-5000-team-app-79d2ca39-dd2c0eb6"},
		{"digest", "alpine@sha256:0123abcd", "alpine", ""},
		{"tag and digest", "alpine:3.19@sha256:0123abcd", "alpine", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeTree(t, root, map[string]string{"Dockerfile": "FROM " + tc.ref + "\n"})
			images := titledStates(ingestTree(t, root), "image", tc.title)
			if len(images) != 1 {
				t.Fatalf("want one image titled %q, got %d", tc.title, len(images))
			}
			if got := tripleValues(images[0], source.ConfigImageName); !slices.Equal(got, []any{tc.ref}) {
				t.Errorf("image references = %v, want [%s]", got, tc.ref)
			}
			if tc.idTail != "" && !strings.HasSuffix(images[0].ID, tc.idTail) {
				t.Errorf("ID = %s, want one ending %s", images[0].ID, tc.idTail)
			}
		})
	}
}

// Every cfgfile entity carries its declaring manifest's absolute path, so the
// staleness pass can judge dependencies and images by it (#393).
// spec: entity-staleness / Config-file entities go stale with their manifest
func TestIngestEntityStates_EveryEntityCarriesItsDeclaringFilePath(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"svc/go.mod":       "module example.com/svc\n\ngo 1.23\n\nrequire github.com/a/b v1.2.0\n",
		"web/package.json": `{"name":"web","version":"1.0.0","dependencies":{"lodash":"^4.17.21"}}`,
		"img/Dockerfile":   "FROM golang:1.23\n",
		"jvm/pom.xml":      `<project><groupId>g</groupId><artifactId>a</artifactId><version>1</version><dependencies><dependency><groupId>junit</groupId><artifactId>junit</artifactId><version>4.13.2</version></dependency></dependencies><modules><module>core</module></modules></project>`,
		"gr/build.gradle":  "dependencies {\n    implementation 'org.slf4j:slf4j-api:2.0.9'\n}\n",
	})
	states := ingestTree(t, root)
	if len(states) < 10 {
		t.Fatalf("got %d states; the fixture is vacuous", len(states))
	}
	for _, s := range states {
		got := tripleValues(s, source.ConfigFilePath)
		if len(got) != 1 {
			t.Errorf("%s carries %d file paths, want 1", s.ID, len(got))
			continue
		}
		p, _ := got[0].(string)
		if !filepath.IsAbs(p) || !strings.HasPrefix(p, root) {
			t.Errorf("%s file path = %q, want an absolute path under %s", s.ID, p, root)
		}
		if s.Root != root {
			t.Errorf("%s root = %q, want %q", s.ID, s.Root, root)
		}
	}
}

// A Gradle classifier or extension names a distinct artifact, so it is part of
// the identity, never folded into the plain jar's version (#393).
// spec: entity-identity-safety / Config-file dependency and image identity excludes the version
func TestIngestEntityStates_GradleClassifierIsItsOwnEntity(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{"build.gradle": "dependencies {\n" +
		"    implementation 'org.lwjgl:lwjgl:3.3.1'\n" +
		"    runtimeOnly 'org.lwjgl:lwjgl:3.3.1:natives-linux'\n" +
		"    runtimeOnly 'org.lwjgl:lwjgl:3.3.1@zip'\n}\n"})
	deps := titledStates(ingestTree(t, root), "dependency", "org.lwjgl:lwjgl")
	if len(deps) != 3 {
		t.Fatalf("want plain, natives-linux and zip as three entities, got %d", len(deps))
	}
	var classified, typed int
	for _, d := range deps {
		if got := tripleValues(d, source.ConfigDepVersion); !slices.Equal(got, []any{"3.3.1"}) {
			t.Errorf("%s versions = %v, want [3.3.1]", d.ID, got)
		}
		if slices.Equal(tripleValues(d, source.ConfigDepClassifier), []any{"natives-linux"}) {
			classified++
		}
		if slices.Equal(tripleValues(d, source.ConfigDepType), []any{"zip"}) {
			typed++
		}
	}
	if classified != 1 || typed != 1 {
		t.Errorf("classifier natives-linux on %d states, type zip on %d; want 1 each", classified, typed)
	}
}
