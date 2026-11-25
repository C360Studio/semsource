package cfgfile

import (
	"path/filepath"
	"slices"
	"time"

	"github.com/c360studio/semengine/message"
	semvocab "github.com/c360studio/semengine/vocabulary"

	"github.com/c360studio/semsource/entityid"
	"github.com/c360studio/semsource/handler"
	source "github.com/c360studio/semsource/source/vocabulary"
)

func controlState(id string, triples []message.Triple, indexedAt time.Time) *handler.EntityState {
	return &handler.EntityState{
		ID:              id,
		Triples:         triples,
		UpdatedAt:       indexedAt,
		IndexingProfile: semvocab.IndexingProfileControl,
	}
}

// --------------------------------------------------------------------------
// Typed entity structs — bypass the normalizer, build triples directly.
// Statements carry no Timestamp: graph-ingest stamps them with the envelope
// time, which is what the #336 reconcile fences its write on (#384).
// Each struct mirrors the fields produced by its corresponding parseFile*
// method, replacing Properties + Edges with canonical vocabulary predicates.
// --------------------------------------------------------------------------

// goModuleEntity is a fully-typed go.mod module entity.
type goModuleEntity struct {
	ID         string
	ModulePath string
	GoVersion  string
	FilePath   string
	System     string
	Org        string
	IndexedAt  time.Time

	// DepIDs holds the entity IDs of dependency entities that this module
	// requires, used to build relationship triples.
	DepIDs []string
}

func newGoModuleEntity(authority entityid.Authority, modulePath, goVersion, filePath, system, rel string, indexedAt time.Time) *goModuleEntity {
	instance := slugify(modulePath)
	if instance == "" {
		instance = "module"
	}
	return &goModuleEntity{
		ID:         authority.Build(system, "config", "gomod", declared(instance, rel)),
		ModulePath: modulePath,
		GoVersion:  goVersion,
		FilePath:   filePath,
		System:     system,
		Org:        authority.Org,
		IndexedAt:  indexedAt,
	}
}

func (e *goModuleEntity) triples() []message.Triple {
	triples := []message.Triple{
		// Title makes the entity NAME_INDEX-visible (search-ranking-and-reach D4).
		{Subject: e.ID, Predicate: source.DcTitle, Object: e.ModulePath, Source: entityid.PlatformSemsource, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.ConfigModulePath, Object: e.ModulePath, Source: entityid.PlatformSemsource, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.ConfigModuleGoVer, Object: e.GoVersion, Source: entityid.PlatformSemsource, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.ConfigFilePath, Object: e.FilePath, Source: entityid.PlatformSemsource, Confidence: 1.0},
	}
	for _, depID := range e.DepIDs {
		triples = append(triples, message.Triple{
			Subject:    e.ID,
			Predicate:  source.ConfigRequires,
			Object:     depID,
			Source:     entityid.PlatformSemsource,
			Confidence: 1.0,
		})
	}
	return triples
}

func (e *goModuleEntity) entityState() *handler.EntityState {
	return controlState(e.ID, e.triples(), e.IndexedAt)
}

// --------------------------------------------------------------------------

// goDependencyEntity is a fully-typed go.mod dependency entity.
type goDependencyEntity struct {
	ID        string
	Name      string
	Version   string
	Indirect  bool
	System    string
	Org       string
	IndexedAt time.Time
}

func newGoDependencyEntity(authority entityid.Authority, depPath, version string, indirect bool, system, rel string, indexedAt time.Time) *goDependencyEntity {
	instance := contentHashShort(depPath + "@" + version)
	return &goDependencyEntity{
		ID:        authority.Build(system, "config", "dependency", declared(instance, rel)),
		Name:      depPath,
		Version:   version,
		Indirect:  indirect,
		System:    system,
		Org:       authority.Org,
		IndexedAt: indexedAt,
	}
}

// fold merges a repeated require of the same module version: it is
// indirect only when every require says so (#375).
func (e *goDependencyEntity) fold(o *goDependencyEntity) { e.Indirect = e.Indirect && o.Indirect }

func (e *goDependencyEntity) entityID() string { return e.ID }

func (e *goDependencyEntity) triples() []message.Triple {
	indirect := "false"
	if e.Indirect {
		indirect = "true"
	}
	return []message.Triple{
		{Subject: e.ID, Predicate: source.DcTitle, Object: e.Name, Source: entityid.PlatformSemsource, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.ConfigDepName, Object: e.Name, Source: entityid.PlatformSemsource, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.ConfigDepVersion, Object: e.Version, Source: entityid.PlatformSemsource, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.ConfigDepIndirect, Object: indirect, Source: entityid.PlatformSemsource, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.ConfigDepKind, Object: "go", Source: entityid.PlatformSemsource, Confidence: 1.0},
	}
}

func (e *goDependencyEntity) entityState() *handler.EntityState {
	return controlState(e.ID, e.triples(), e.IndexedAt)
}

// --------------------------------------------------------------------------

// npmPackageEntity is a fully-typed package.json package entity.
type npmPackageEntity struct {
	ID        string
	Name      string
	Version   string
	FilePath  string
	System    string
	Org       string
	IndexedAt time.Time

	// DepIDs holds the entity IDs of npm dependency entities for relationship triples.
	DepIDs []string
}

func newNPMPackageEntity(authority entityid.Authority, name, version, filePath, system, rel string, indexedAt time.Time) *npmPackageEntity {
	instance := slugify(name)
	if instance == "" {
		instance = "package"
	}
	return &npmPackageEntity{
		ID:        authority.Build(system, "config", "package", declared(instance, rel)),
		Name:      name,
		Version:   version,
		FilePath:  filePath,
		System:    system,
		Org:       authority.Org,
		IndexedAt: indexedAt,
	}
}

func (e *npmPackageEntity) triples() []message.Triple {
	triples := []message.Triple{
		{Subject: e.ID, Predicate: source.DcTitle, Object: e.Name, Source: entityid.PlatformSemsource, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.ConfigPkgName, Object: e.Name, Source: entityid.PlatformSemsource, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.ConfigPkgVersion, Object: e.Version, Source: entityid.PlatformSemsource, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.ConfigFilePath, Object: e.FilePath, Source: entityid.PlatformSemsource, Confidence: 1.0},
	}
	for _, depID := range e.DepIDs {
		triples = append(triples, message.Triple{
			Subject:    e.ID,
			Predicate:  source.ConfigDepends,
			Object:     depID,
			Source:     entityid.PlatformSemsource,
			Confidence: 1.0,
		})
	}
	return triples
}

func (e *npmPackageEntity) entityState() *handler.EntityState {
	return controlState(e.ID, e.triples(), e.IndexedAt)
}

// --------------------------------------------------------------------------

// npmDependencyEntity is a fully-typed package.json dependency entity.
type npmDependencyEntity struct {
	ID        string
	Name      string
	Version   string
	Kind      string // "npm-prod" or "npm-dev"
	System    string
	Org       string
	IndexedAt time.Time
}

func newNPMDependencyEntity(authority entityid.Authority, name, version string, dev bool, system, rel string, indexedAt time.Time) *npmDependencyEntity {
	instance := contentHashShort(name + "@" + version)
	kind := "npm-prod"
	if dev {
		kind = "npm-dev"
	}
	return &npmDependencyEntity{
		ID:        authority.Build(system, "config", "dependency", declared(instance, rel)),
		Name:      name,
		Version:   version,
		Kind:      kind,
		System:    system,
		Org:       authority.Org,
		IndexedAt: indexedAt,
	}
}

// fold merges a dependency listed in both dependencies and devDependencies:
// npm installs it for production, so prod wins (#375).
func (e *npmDependencyEntity) fold(o *npmDependencyEntity) {
	if o.Kind == "npm-prod" {
		e.Kind = "npm-prod"
	}
}

func (e *npmDependencyEntity) entityID() string { return e.ID }

func (e *npmDependencyEntity) triples() []message.Triple {
	return []message.Triple{
		{Subject: e.ID, Predicate: source.DcTitle, Object: e.Name, Source: entityid.PlatformSemsource, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.ConfigDepName, Object: e.Name, Source: entityid.PlatformSemsource, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.ConfigDepVersion, Object: e.Version, Source: entityid.PlatformSemsource, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.ConfigDepKind, Object: e.Kind, Source: entityid.PlatformSemsource, Confidence: 1.0},
	}
}

func (e *npmDependencyEntity) entityState() *handler.EntityState {
	return controlState(e.ID, e.triples(), e.IndexedAt)
}

// --------------------------------------------------------------------------

// dockerImageEntity is a fully-typed Dockerfile image entity.
type dockerImageEntity struct {
	ID           string
	Image        string
	ExposedPorts []string
	FilePath     string
	System       string
	Org          string
	IndexedAt    time.Time
}

func newDockerImageEntity(authority entityid.Authority, image string, exposedPorts []string, filePath, system, rel string, indexedAt time.Time) *dockerImageEntity {
	// The slug alone is not injective (foo/bar:1 and foo-bar:1), so the raw
	// reference's hash keeps distinct images apart (#375).
	instance := slugify(image) + "-" + contentHashShort(image)
	return &dockerImageEntity{
		ID:           authority.Build(system, "config", "image", declared(instance, rel)),
		Image:        image,
		ExposedPorts: exposedPorts,
		FilePath:     filePath,
		System:       system,
		Org:          authority.Org,
		IndexedAt:    indexedAt,
	}
}

func (e *dockerImageEntity) triples() []message.Triple {
	triples := []message.Triple{
		{Subject: e.ID, Predicate: source.DcTitle, Object: e.Image, Source: entityid.PlatformSemsource, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.ConfigImageName, Object: e.Image, Source: entityid.PlatformSemsource, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.ConfigFilePath, Object: e.FilePath, Source: entityid.PlatformSemsource, Confidence: 1.0},
	}
	for _, port := range e.ExposedPorts {
		triples = append(triples, message.Triple{
			Subject:    e.ID,
			Predicate:  source.ConfigImagePorts,
			Object:     port,
			Source:     entityid.PlatformSemsource,
			Confidence: 1.0,
		})
	}
	return triples
}

func (e *dockerImageEntity) entityState() *handler.EntityState {
	return controlState(e.ID, e.triples(), e.IndexedAt)
}

// --------------------------------------------------------------------------

// mavenProjectEntity is a fully-typed pom.xml project entity.
type mavenProjectEntity struct {
	ID         string
	GroupID    string
	ArtifactID string
	Version    string
	Packaging  string
	FilePath   string
	System     string
	Org        string
	IndexedAt  time.Time

	// DepIDs and ModuleIDs hold entity IDs for relationship triples.
	DepIDs    []string
	ModuleIDs []string
}

func newMavenProjectEntity(authority entityid.Authority, groupID, artifactID, version, packaging, filePath, system, rel string, indexedAt time.Time) *mavenProjectEntity {
	instance := slugify(groupID + ":" + artifactID)
	if instance == "" {
		instance = "project"
	}
	return &mavenProjectEntity{
		ID:         authority.Build(system, "config", "project", declared(instance, rel)),
		GroupID:    groupID,
		ArtifactID: artifactID,
		Version:    version,
		Packaging:  packaging,
		FilePath:   filePath,
		System:     system,
		Org:        authority.Org,
		IndexedAt:  indexedAt,
	}
}

func (e *mavenProjectEntity) triples() []message.Triple {
	triples := []message.Triple{
		{Subject: e.ID, Predicate: source.DcTitle, Object: e.GroupID + ":" + e.ArtifactID, Source: entityid.PlatformSemsource, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.ConfigProjectGroup, Object: e.GroupID, Source: entityid.PlatformSemsource, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.ConfigProjectArtifact, Object: e.ArtifactID, Source: entityid.PlatformSemsource, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.ConfigProjectVersion, Object: e.Version, Source: entityid.PlatformSemsource, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.ConfigProjectPackaging, Object: e.Packaging, Source: entityid.PlatformSemsource, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.ConfigFilePath, Object: e.FilePath, Source: entityid.PlatformSemsource, Confidence: 1.0},
	}
	for _, depID := range e.DepIDs {
		triples = append(triples, message.Triple{
			Subject:    e.ID,
			Predicate:  source.ConfigRequires,
			Object:     depID,
			Source:     entityid.PlatformSemsource,
			Confidence: 1.0,
		})
	}
	for _, modID := range e.ModuleIDs {
		triples = append(triples, message.Triple{
			Subject:    e.ID,
			Predicate:  source.ConfigContains,
			Object:     modID,
			Source:     entityid.PlatformSemsource,
			Confidence: 1.0,
		})
	}
	return triples
}

func (e *mavenProjectEntity) entityState() *handler.EntityState {
	return controlState(e.ID, e.triples(), e.IndexedAt)
}

// --------------------------------------------------------------------------

// mavenDependencyEntity is a fully-typed Maven/pom.xml dependency entity.
type mavenDependencyEntity struct {
	ID      string
	Name    string // "groupId:artifactId"
	Version string
	Scopes  []string // every scope the manifest declares it under (#375)
	// Type (when not "jar") and Classifier name a distinct artifact, so they
	// are part of the instance (#375).
	Type       string
	Classifier string
	System     string
	Org        string
	IndexedAt  time.Time
}

func newMavenDependencyEntity(authority entityid.Authority, dep POMDep, system, rel string, indexedAt time.Time) *mavenDependencyEntity {
	typ := dep.Type
	if typ == "jar" {
		typ = ""
	}
	key := dep.GroupID + ":" + dep.ArtifactID
	if typ != "" || dep.Classifier != "" {
		key += ":" + typ + ":" + dep.Classifier
	}
	instance := contentHashShort(key + "@" + dep.Version)
	var scopes []string
	if dep.Scope != "" {
		scopes = []string{dep.Scope}
	}
	return &mavenDependencyEntity{
		ID:         authority.Build(system, "config", "dependency", declared(instance, rel)),
		Name:       dep.GroupID + ":" + dep.ArtifactID,
		Version:    dep.Version,
		Scopes:     scopes,
		Type:       typ,
		Classifier: dep.Classifier,
		System:     system,
		Org:        authority.Org,
		IndexedAt:  indexedAt,
	}
}

func (e *mavenDependencyEntity) triples() []message.Triple {
	triples := []message.Triple{
		{Subject: e.ID, Predicate: source.DcTitle, Object: e.Name, Source: entityid.PlatformSemsource, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.ConfigDepName, Object: e.Name, Source: entityid.PlatformSemsource, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.ConfigDepVersion, Object: e.Version, Source: entityid.PlatformSemsource, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.ConfigDepKind, Object: "maven", Source: entityid.PlatformSemsource, Confidence: 1.0},
	}
	triples = appendValues(triples, e.ID, source.ConfigDepScope, e.Scopes)
	triples = appendValues(triples, e.ID, source.ConfigDepType, []string{e.Type})
	return appendValues(triples, e.ID, source.ConfigDepClassifier, []string{e.Classifier})
}

// fold merges a repeated declaration of the same artifact under another
// scope (#375).
func (e *mavenDependencyEntity) fold(o *mavenDependencyEntity) {
	e.Scopes = append(e.Scopes, o.Scopes...)
}

func (e *mavenDependencyEntity) entityID() string { return e.ID }

func (e *mavenDependencyEntity) entityState() *handler.EntityState {
	return controlState(e.ID, e.triples(), e.IndexedAt)
}

// --------------------------------------------------------------------------

// pomModuleEntity is a fully-typed Maven sub-module entity (from <modules>).
// Uses entity type "gomod" to match the go.mod module entity type since both
// represent a named module within the project graph.
type pomModuleEntity struct {
	ID        string
	Name      string
	FilePath  string
	System    string
	Org       string
	IndexedAt time.Time
}

func newPOMModuleEntity(authority entityid.Authority, name, filePath, system, rel string, indexedAt time.Time) *pomModuleEntity {
	instance := slugify(name)
	if instance == "" {
		instance = contentHashShort(name)
	}
	return &pomModuleEntity{
		ID:        authority.Build(system, "config", "gomod", declared(instance, rel)),
		Name:      name,
		FilePath:  filePath,
		System:    system,
		Org:       authority.Org,
		IndexedAt: indexedAt,
	}
}

func (e *pomModuleEntity) triples() []message.Triple {
	return []message.Triple{
		// Title makes the entity NAME_INDEX-visible and gives graph_search a label (#142).
		{Subject: e.ID, Predicate: source.DcTitle, Object: e.Name, Source: entityid.PlatformSemsource, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.ConfigModulePath, Object: e.Name, Source: entityid.PlatformSemsource, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.ConfigFilePath, Object: e.FilePath, Source: entityid.PlatformSemsource, Confidence: 1.0},
	}
}

func (e *pomModuleEntity) entityState() *handler.EntityState {
	return controlState(e.ID, e.triples(), e.IndexedAt)
}

// --------------------------------------------------------------------------

// gradleProjectEntity is a fully-typed build.gradle project entity.
type gradleProjectEntity struct {
	ID        string
	Name      string
	FilePath  string
	System    string
	Org       string
	IndexedAt time.Time

	// DepIDs holds entity IDs of dependencies for relationship triples.
	DepIDs []string
}

func newGradleProjectEntity(authority entityid.Authority, name, filePath, system, rel string, indexedAt time.Time) *gradleProjectEntity {
	instance := slugify(name)
	if instance == "" {
		instance = "project"
	}
	return &gradleProjectEntity{
		ID:        authority.Build(system, "config", "project", declared(instance, rel)),
		Name:      name,
		FilePath:  filePath,
		System:    system,
		Org:       authority.Org,
		IndexedAt: indexedAt,
	}
}

func (e *gradleProjectEntity) triples() []message.Triple {
	triples := []message.Triple{
		{Subject: e.ID, Predicate: source.DcTitle, Object: e.Name, Source: entityid.PlatformSemsource, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.ConfigProjectArtifact, Object: e.Name, Source: entityid.PlatformSemsource, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.ConfigProjectBuild, Object: "gradle", Source: entityid.PlatformSemsource, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.ConfigFilePath, Object: e.FilePath, Source: entityid.PlatformSemsource, Confidence: 1.0},
	}
	for _, depID := range e.DepIDs {
		triples = append(triples, message.Triple{
			Subject:    e.ID,
			Predicate:  source.ConfigRequires,
			Object:     depID,
			Source:     entityid.PlatformSemsource,
			Confidence: 1.0,
		})
	}
	return triples
}

func (e *gradleProjectEntity) entityState() *handler.EntityState {
	return controlState(e.ID, e.triples(), e.IndexedAt)
}

// --------------------------------------------------------------------------

// gradleDependencyEntity is a fully-typed build.gradle dependency entity.
type gradleDependencyEntity struct {
	ID             string
	Name           string // "group:name"
	Version        string
	Configurations []string // every configuration the build file declares it under (#375)
	System         string
	Org            string
	IndexedAt      time.Time
}

func newGradleDependencyEntity(authority entityid.Authority, group, name, version, configuration, system, rel string, indexedAt time.Time) *gradleDependencyEntity {
	instance := contentHashShort(group + ":" + name + "@" + version)
	return &gradleDependencyEntity{
		ID:             authority.Build(system, "config", "dependency", declared(instance, rel)),
		Name:           group + ":" + name,
		Version:        version,
		Configurations: []string{configuration},
		System:         system,
		Org:            authority.Org,
		IndexedAt:      indexedAt,
	}
}

func (e *gradleDependencyEntity) triples() []message.Triple {
	triples := []message.Triple{
		// Title makes the entity NAME_INDEX-visible and gives graph_search a label (#142).
		{Subject: e.ID, Predicate: source.DcTitle, Object: e.Name, Source: entityid.PlatformSemsource, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.ConfigDepName, Object: e.Name, Source: entityid.PlatformSemsource, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.ConfigDepVersion, Object: e.Version, Source: entityid.PlatformSemsource, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.ConfigDepKind, Object: "gradle", Source: entityid.PlatformSemsource, Confidence: 1.0},
	}
	return appendValues(triples, e.ID, source.ConfigDepConfiguration, e.Configurations)
}

// fold merges a repeated declaration of the same coordinate under another
// configuration (#375).
func (e *gradleDependencyEntity) fold(o *gradleDependencyEntity) {
	e.Configurations = append(e.Configurations, o.Configurations...)
}

func (e *gradleDependencyEntity) entityID() string { return e.ID }

func (e *gradleDependencyEntity) entityState() *handler.EntityState {
	return controlState(e.ID, e.triples(), e.IndexedAt)
}

// --------------------------------------------------------------------------
// parseFileEntityStates dispatches to the right typed-entity builder based on
// the base filename. Mirrors parseFile but returns []*handler.EntityState
// instead of []handler.RawEntity.
// --------------------------------------------------------------------------

func (h *ConfigHandler) parseFileEntityStates(base, path string, content []byte, root string, authority entityid.Authority, now time.Time) []*handler.EntityState {
	system := h.system(root)
	rel := declaringPath(root, path)
	switch base {
	case "go.mod":
		return h.goModEntityStates(content, path, system, rel, authority, now)
	case "package.json":
		return h.npmPackageEntityStates(content, path, system, rel, authority, now)
	case "Dockerfile":
		return h.dockerEntityStates(content, path, system, rel, authority, now)
	case "pom.xml":
		return h.mavenEntityStates(content, path, system, rel, authority, now)
	case "build.gradle":
		return h.gradleEntityStates(content, path, system, rel, authority, now)
	}
	return nil
}

func (h *ConfigHandler) goModEntityStates(content []byte, path, system, rel string, authority entityid.Authority, now time.Time) []*handler.EntityState {
	result, err := ParseGoMod(content)
	if err != nil {
		h.logger.Warn("cfgfile: parse go.mod failed", "path", path, "error", err)
		return nil
	}

	mod := newGoModuleEntity(authority, result.Module, result.GoVersion, path, system, rel, now)

	deps := make([]*goDependencyEntity, 0, len(result.Deps))
	for _, dep := range result.Deps {
		deps = append(deps, newGoDependencyEntity(authority, dep.Path, dep.Version, dep.Indirect, system, rel, now))
	}
	var depStates []*handler.EntityState
	for _, d := range foldByID(deps) {
		mod.DepIDs = append(mod.DepIDs, d.ID)
		depStates = append(depStates, d.entityState())
	}

	// Module entity first so consumers see the root before its leaves.
	return append([]*handler.EntityState{mod.entityState()}, depStates...)
}

func (h *ConfigHandler) npmPackageEntityStates(content []byte, path, system, rel string, authority entityid.Authority, now time.Time) []*handler.EntityState {
	result, err := ParsePackageJSON(content)
	if err != nil {
		h.logger.Warn("cfgfile: parse package.json failed", "path", path, "error", err)
		return nil
	}

	pkg := newNPMPackageEntity(authority, result.Name, result.Version, path, system, rel, now)

	deps := make([]*npmDependencyEntity, 0, len(result.Deps))
	for _, dep := range result.Deps {
		deps = append(deps, newNPMDependencyEntity(authority, dep.Name, dep.Version, dep.Dev, system, rel, now))
	}
	var depStates []*handler.EntityState
	for _, d := range foldByID(deps) {
		pkg.DepIDs = append(pkg.DepIDs, d.ID)
		depStates = append(depStates, d.entityState())
	}

	return append([]*handler.EntityState{pkg.entityState()}, depStates...)
}

func (h *ConfigHandler) dockerEntityStates(content []byte, path, system, rel string, authority entityid.Authority, now time.Time) []*handler.EntityState {
	result, err := ParseDockerfile(content)
	if err != nil {
		h.logger.Warn("cfgfile: parse Dockerfile failed", "path", path, "error", err)
		return nil
	}

	var states []*handler.EntityState
	for _, img := range result.BaseImages {
		e := newDockerImageEntity(authority, img, result.ExposedPorts, path, system, rel, now)
		states = append(states, e.entityState())
	}
	return states
}

func (h *ConfigHandler) mavenEntityStates(content []byte, path, system, rel string, authority entityid.Authority, now time.Time) []*handler.EntityState {
	result, err := ParsePOM(content)
	if err != nil {
		h.logger.Warn("cfgfile: parse pom.xml failed", "path", path, "error", err)
		return nil
	}

	proj := newMavenProjectEntity(authority, result.GroupID, result.ArtifactID, result.Version, result.Packaging, path, system, rel, now)

	deps := make([]*mavenDependencyEntity, 0, len(result.Deps))
	for _, dep := range result.Deps {
		deps = append(deps, newMavenDependencyEntity(authority, dep, system, rel, now))
	}
	var childStates []*handler.EntityState
	for _, d := range foldByID(deps) {
		proj.DepIDs = append(proj.DepIDs, d.ID)
		childStates = append(childStates, d.entityState())
	}
	for _, mod := range result.Modules {
		m := newPOMModuleEntity(authority, mod, path, system, rel, now)
		proj.ModuleIDs = append(proj.ModuleIDs, m.ID)
		childStates = append(childStates, m.entityState())
	}

	return append([]*handler.EntityState{proj.entityState()}, childStates...)
}

func (h *ConfigHandler) gradleEntityStates(content []byte, path, system, rel string, authority entityid.Authority, now time.Time) []*handler.EntityState {
	result, err := ParseGradle(content)
	if err != nil {
		h.logger.Warn("cfgfile: parse build.gradle failed", "path", path, "error", err)
		return nil
	}

	// A root-level build.gradle is named for its source (the project
	// override, else the root's slug), never the checkout folder (#348).
	dirName := filepath.Base(filepath.Dir(path))
	if filepath.Dir(rel) == "." {
		dirName = system
	}
	proj := newGradleProjectEntity(authority, dirName, path, system, rel, now)

	deps := make([]*gradleDependencyEntity, 0, len(result.Deps))
	for _, dep := range result.Deps {
		deps = append(deps, newGradleDependencyEntity(authority, dep.Group, dep.Name, dep.Version, dep.Configuration, system, rel, now))
	}
	var depStates []*handler.EntityState
	for _, d := range foldByID(deps) {
		proj.DepIDs = append(proj.DepIDs, d.ID)
		depStates = append(depStates, d.entityState())
	}

	return append([]*handler.EntityState{proj.entityState()}, depStates...)
}

// declaringPath is the manifest's path relative to the source root,
// slash-separated, so an ID never depends on where the tree is checked out
// (#348). Walk and watch paths are both root-joined, so Rel always succeeds.
func declaringPath(root, path string) string {
	if rel, err := filepath.Rel(root, path); err == nil {
		return filepath.ToSlash(rel)
	}
	// shortcut: unreachable for root-joined paths; falls back to the path as given, which is location-dependent. Upgrade if a caller ever passes a path outside root.
	return filepath.ToSlash(path)
}

// declared scopes an instance to the manifest that declares it, so two
// manifests naming one coordinate, image or directory never write one entity
// ID with different facts (#348).
func declared(instance, rel string) string {
	return instance + "-" + contentHashShort(rel)
}

// foldByID keeps the first declaration of each entity ID in one manifest and
// folds later declarations of that ID into it, so a manifest never emits two
// states under one ID or names one ID twice in its edges (#375).
func foldByID[T interface {
	entityID() string
	fold(T)
}](decls []T) []T {
	seen := make(map[string]T, len(decls))
	out := decls[:0:0]
	for _, d := range decls {
		if prev, ok := seen[d.entityID()]; ok {
			prev.fold(d)
			continue
		}
		seen[d.entityID()] = d
		out = append(out, d)
	}
	return out
}

// appendValues adds one triple per distinct non-empty value, sorted, so a
// folded entity does not depend on declaration order (#375).
func appendValues(triples []message.Triple, id, pred string, values []string) []message.Triple {
	vals := slices.Clone(values)
	slices.Sort(vals)
	for _, v := range slices.Compact(vals) {
		if v == "" {
			continue
		}
		triples = append(triples, message.Triple{
			Subject: id, Predicate: pred, Object: v,
			Source: entityid.PlatformSemsource, Confidence: 1.0,
		})
	}
	return triples
}
