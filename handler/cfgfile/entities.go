package cfgfile

import (
	"path/filepath"
	"time"

	"github.com/c360studio/semstreams/message"

	"github.com/c360studio/semsource/entityid"
	"github.com/c360studio/semsource/handler"
	source "github.com/c360studio/semsource/source/vocabulary"
)

// --------------------------------------------------------------------------
// Typed entity structs — bypass the normalizer, build triples directly.
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

func newGoModuleEntity(org, modulePath, goVersion, filePath, system string, indexedAt time.Time) *goModuleEntity {
	instance := slugify(modulePath)
	return &goModuleEntity{
		ID:         entityid.Build(org, entityid.PlatformSemsource, "config", system, "gomod", instance),
		ModulePath: modulePath,
		GoVersion:  goVersion,
		FilePath:   filePath,
		System:     system,
		Org:        org,
		IndexedAt:  indexedAt,
	}
}

func (e *goModuleEntity) triples() []message.Triple {
	now := e.IndexedAt
	triples := []message.Triple{
		{Subject: e.ID, Predicate: source.ConfigModulePath, Object: e.ModulePath, Source: entityid.PlatformSemsource, Timestamp: now, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.ConfigModuleGoVer, Object: e.GoVersion, Source: entityid.PlatformSemsource, Timestamp: now, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.ConfigFilePath, Object: e.FilePath, Source: entityid.PlatformSemsource, Timestamp: now, Confidence: 1.0},
	}
	for _, depID := range e.DepIDs {
		triples = append(triples, message.Triple{
			Subject:    e.ID,
			Predicate:  source.ConfigRequires,
			Object:     depID,
			Source:     entityid.PlatformSemsource,
			Timestamp:  now,
			Confidence: 1.0,
		})
	}
	return triples
}

func (e *goModuleEntity) entityState() *handler.EntityState {
	return &handler.EntityState{ID: e.ID, Triples: e.triples(), UpdatedAt: e.IndexedAt}
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

func newGoDependencyEntity(org, depPath, version string, indirect bool, system string, indexedAt time.Time) *goDependencyEntity {
	instance := contentHashShort(depPath + "@" + version)
	return &goDependencyEntity{
		ID:        entityid.Build(org, entityid.PlatformSemsource, "config", system, "dependency", instance),
		Name:      depPath,
		Version:   version,
		Indirect:  indirect,
		System:    system,
		Org:       org,
		IndexedAt: indexedAt,
	}
}

func (e *goDependencyEntity) triples() []message.Triple {
	now := e.IndexedAt
	indirect := "false"
	if e.Indirect {
		indirect = "true"
	}
	return []message.Triple{
		{Subject: e.ID, Predicate: source.ConfigDepName, Object: e.Name, Source: entityid.PlatformSemsource, Timestamp: now, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.ConfigDepVersion, Object: e.Version, Source: entityid.PlatformSemsource, Timestamp: now, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.ConfigDepIndirect, Object: indirect, Source: entityid.PlatformSemsource, Timestamp: now, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.ConfigDepKind, Object: "go", Source: entityid.PlatformSemsource, Timestamp: now, Confidence: 1.0},
	}
}

func (e *goDependencyEntity) entityState() *handler.EntityState {
	return &handler.EntityState{ID: e.ID, Triples: e.triples(), UpdatedAt: e.IndexedAt}
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

func newNPMPackageEntity(org, name, version, filePath, system string, indexedAt time.Time) *npmPackageEntity {
	instance := slugify(name)
	if instance == "" {
		instance = contentHashShort(filePath)
	}
	return &npmPackageEntity{
		ID:        entityid.Build(org, entityid.PlatformSemsource, "config", system, "package", instance),
		Name:      name,
		Version:   version,
		FilePath:  filePath,
		System:    system,
		Org:       org,
		IndexedAt: indexedAt,
	}
}

func (e *npmPackageEntity) triples() []message.Triple {
	now := e.IndexedAt
	triples := []message.Triple{
		{Subject: e.ID, Predicate: source.ConfigPkgName, Object: e.Name, Source: entityid.PlatformSemsource, Timestamp: now, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.ConfigPkgVersion, Object: e.Version, Source: entityid.PlatformSemsource, Timestamp: now, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.ConfigFilePath, Object: e.FilePath, Source: entityid.PlatformSemsource, Timestamp: now, Confidence: 1.0},
	}
	for _, depID := range e.DepIDs {
		triples = append(triples, message.Triple{
			Subject:    e.ID,
			Predicate:  source.ConfigDepends,
			Object:     depID,
			Source:     entityid.PlatformSemsource,
			Timestamp:  now,
			Confidence: 1.0,
		})
	}
	return triples
}

func (e *npmPackageEntity) entityState() *handler.EntityState {
	return &handler.EntityState{ID: e.ID, Triples: e.triples(), UpdatedAt: e.IndexedAt}
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

func newNPMDependencyEntity(org, name, version string, dev bool, system string, indexedAt time.Time) *npmDependencyEntity {
	instance := contentHashShort(name + "@" + version)
	kind := "npm-prod"
	if dev {
		kind = "npm-dev"
	}
	return &npmDependencyEntity{
		ID:        entityid.Build(org, entityid.PlatformSemsource, "config", system, "dependency", instance),
		Name:      name,
		Version:   version,
		Kind:      kind,
		System:    system,
		Org:       org,
		IndexedAt: indexedAt,
	}
}

func (e *npmDependencyEntity) triples() []message.Triple {
	now := e.IndexedAt
	return []message.Triple{
		{Subject: e.ID, Predicate: source.ConfigDepName, Object: e.Name, Source: entityid.PlatformSemsource, Timestamp: now, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.ConfigDepVersion, Object: e.Version, Source: entityid.PlatformSemsource, Timestamp: now, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.ConfigDepKind, Object: e.Kind, Source: entityid.PlatformSemsource, Timestamp: now, Confidence: 1.0},
	}
}

func (e *npmDependencyEntity) entityState() *handler.EntityState {
	return &handler.EntityState{ID: e.ID, Triples: e.triples(), UpdatedAt: e.IndexedAt}
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

func newDockerImageEntity(org, image string, exposedPorts []string, filePath, system string, indexedAt time.Time) *dockerImageEntity {
	instance := slugify(image)
	return &dockerImageEntity{
		ID:           entityid.Build(org, entityid.PlatformSemsource, "config", system, "image", instance),
		Image:        image,
		ExposedPorts: exposedPorts,
		FilePath:     filePath,
		System:       system,
		Org:          org,
		IndexedAt:    indexedAt,
	}
}

func (e *dockerImageEntity) triples() []message.Triple {
	now := e.IndexedAt
	triples := []message.Triple{
		{Subject: e.ID, Predicate: source.ConfigImageName, Object: e.Image, Source: entityid.PlatformSemsource, Timestamp: now, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.ConfigFilePath, Object: e.FilePath, Source: entityid.PlatformSemsource, Timestamp: now, Confidence: 1.0},
	}
	for _, port := range e.ExposedPorts {
		triples = append(triples, message.Triple{
			Subject:    e.ID,
			Predicate:  source.ConfigImagePorts,
			Object:     port,
			Source:     entityid.PlatformSemsource,
			Timestamp:  now,
			Confidence: 1.0,
		})
	}
	return triples
}

func (e *dockerImageEntity) entityState() *handler.EntityState {
	return &handler.EntityState{ID: e.ID, Triples: e.triples(), UpdatedAt: e.IndexedAt}
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

func newMavenProjectEntity(org, groupID, artifactID, version, packaging, filePath, system string, indexedAt time.Time) *mavenProjectEntity {
	instance := slugify(groupID + ":" + artifactID)
	if instance == "" {
		instance = contentHashShort(filePath)
	}
	return &mavenProjectEntity{
		ID:         entityid.Build(org, entityid.PlatformSemsource, "config", system, "project", instance),
		GroupID:    groupID,
		ArtifactID: artifactID,
		Version:    version,
		Packaging:  packaging,
		FilePath:   filePath,
		System:     system,
		Org:        org,
		IndexedAt:  indexedAt,
	}
}

func (e *mavenProjectEntity) triples() []message.Triple {
	now := e.IndexedAt
	triples := []message.Triple{
		{Subject: e.ID, Predicate: source.ConfigProjectGroup, Object: e.GroupID, Source: entityid.PlatformSemsource, Timestamp: now, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.ConfigProjectArtifact, Object: e.ArtifactID, Source: entityid.PlatformSemsource, Timestamp: now, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.ConfigProjectVersion, Object: e.Version, Source: entityid.PlatformSemsource, Timestamp: now, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.ConfigProjectPackaging, Object: e.Packaging, Source: entityid.PlatformSemsource, Timestamp: now, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.ConfigFilePath, Object: e.FilePath, Source: entityid.PlatformSemsource, Timestamp: now, Confidence: 1.0},
	}
	for _, depID := range e.DepIDs {
		triples = append(triples, message.Triple{
			Subject:    e.ID,
			Predicate:  source.ConfigRequires,
			Object:     depID,
			Source:     entityid.PlatformSemsource,
			Timestamp:  now,
			Confidence: 1.0,
		})
	}
	for _, modID := range e.ModuleIDs {
		triples = append(triples, message.Triple{
			Subject:    e.ID,
			Predicate:  source.ConfigContains,
			Object:     modID,
			Source:     entityid.PlatformSemsource,
			Timestamp:  now,
			Confidence: 1.0,
		})
	}
	return triples
}

func (e *mavenProjectEntity) entityState() *handler.EntityState {
	return &handler.EntityState{ID: e.ID, Triples: e.triples(), UpdatedAt: e.IndexedAt}
}

// --------------------------------------------------------------------------

// mavenDependencyEntity is a fully-typed Maven/pom.xml dependency entity.
type mavenDependencyEntity struct {
	ID        string
	Name      string // "groupId:artifactId"
	Version   string
	Scope     string
	System    string
	Org       string
	IndexedAt time.Time
}

func newMavenDependencyEntity(org, groupID, artifactID, version, scope, system string, indexedAt time.Time) *mavenDependencyEntity {
	instance := contentHashShort(groupID + ":" + artifactID + "@" + version)
	return &mavenDependencyEntity{
		ID:        entityid.Build(org, entityid.PlatformSemsource, "config", system, "dependency", instance),
		Name:      groupID + ":" + artifactID,
		Version:   version,
		Scope:     scope,
		System:    system,
		Org:       org,
		IndexedAt: indexedAt,
	}
}

func (e *mavenDependencyEntity) triples() []message.Triple {
	now := e.IndexedAt
	triples := []message.Triple{
		{Subject: e.ID, Predicate: source.ConfigDepName, Object: e.Name, Source: entityid.PlatformSemsource, Timestamp: now, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.ConfigDepVersion, Object: e.Version, Source: entityid.PlatformSemsource, Timestamp: now, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.ConfigDepKind, Object: "maven", Source: entityid.PlatformSemsource, Timestamp: now, Confidence: 1.0},
	}
	if e.Scope != "" {
		triples = append(triples, message.Triple{
			Subject:    e.ID,
			Predicate:  source.ConfigDepScope,
			Object:     e.Scope,
			Source:     entityid.PlatformSemsource,
			Timestamp:  now,
			Confidence: 1.0,
		})
	}
	return triples
}

func (e *mavenDependencyEntity) entityState() *handler.EntityState {
	return &handler.EntityState{ID: e.ID, Triples: e.triples(), UpdatedAt: e.IndexedAt}
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

func newPOMModuleEntity(org, name, filePath, system string, indexedAt time.Time) *pomModuleEntity {
	instance := slugify(name)
	if instance == "" {
		instance = contentHashShort(name)
	}
	return &pomModuleEntity{
		ID:        entityid.Build(org, entityid.PlatformSemsource, "config", system, "gomod", instance),
		Name:      name,
		FilePath:  filePath,
		System:    system,
		Org:       org,
		IndexedAt: indexedAt,
	}
}

func (e *pomModuleEntity) triples() []message.Triple {
	now := e.IndexedAt
	return []message.Triple{
		{Subject: e.ID, Predicate: source.ConfigModulePath, Object: e.Name, Source: entityid.PlatformSemsource, Timestamp: now, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.ConfigFilePath, Object: e.FilePath, Source: entityid.PlatformSemsource, Timestamp: now, Confidence: 1.0},
	}
}

func (e *pomModuleEntity) entityState() *handler.EntityState {
	return &handler.EntityState{ID: e.ID, Triples: e.triples(), UpdatedAt: e.IndexedAt}
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

func newGradleProjectEntity(org, name, filePath, system string, indexedAt time.Time) *gradleProjectEntity {
	instance := slugify(name)
	if instance == "" {
		instance = contentHashShort(filePath)
	}
	return &gradleProjectEntity{
		ID:        entityid.Build(org, entityid.PlatformSemsource, "config", system, "project", instance),
		Name:      name,
		FilePath:  filePath,
		System:    system,
		Org:       org,
		IndexedAt: indexedAt,
	}
}

func (e *gradleProjectEntity) triples() []message.Triple {
	now := e.IndexedAt
	triples := []message.Triple{
		{Subject: e.ID, Predicate: source.ConfigProjectArtifact, Object: e.Name, Source: entityid.PlatformSemsource, Timestamp: now, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.ConfigProjectBuild, Object: "gradle", Source: entityid.PlatformSemsource, Timestamp: now, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.ConfigFilePath, Object: e.FilePath, Source: entityid.PlatformSemsource, Timestamp: now, Confidence: 1.0},
	}
	for _, depID := range e.DepIDs {
		triples = append(triples, message.Triple{
			Subject:    e.ID,
			Predicate:  source.ConfigRequires,
			Object:     depID,
			Source:     entityid.PlatformSemsource,
			Timestamp:  now,
			Confidence: 1.0,
		})
	}
	return triples
}

func (e *gradleProjectEntity) entityState() *handler.EntityState {
	return &handler.EntityState{ID: e.ID, Triples: e.triples(), UpdatedAt: e.IndexedAt}
}

// --------------------------------------------------------------------------

// gradleDependencyEntity is a fully-typed build.gradle dependency entity.
type gradleDependencyEntity struct {
	ID            string
	Name          string // "group:name"
	Version       string
	Configuration string
	System        string
	Org           string
	IndexedAt     time.Time
}

func newGradleDependencyEntity(org, group, name, version, configuration, system string, indexedAt time.Time) *gradleDependencyEntity {
	instance := contentHashShort(group + ":" + name + "@" + version)
	return &gradleDependencyEntity{
		ID:            entityid.Build(org, entityid.PlatformSemsource, "config", system, "dependency", instance),
		Name:          group + ":" + name,
		Version:       version,
		Configuration: configuration,
		System:        system,
		Org:           org,
		IndexedAt:     indexedAt,
	}
}

func (e *gradleDependencyEntity) triples() []message.Triple {
	now := e.IndexedAt
	triples := []message.Triple{
		{Subject: e.ID, Predicate: source.ConfigDepName, Object: e.Name, Source: entityid.PlatformSemsource, Timestamp: now, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.ConfigDepVersion, Object: e.Version, Source: entityid.PlatformSemsource, Timestamp: now, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.ConfigDepKind, Object: "gradle", Source: entityid.PlatformSemsource, Timestamp: now, Confidence: 1.0},
	}
	if e.Configuration != "" {
		triples = append(triples, message.Triple{
			Subject:    e.ID,
			Predicate:  source.ConfigDepConfiguration,
			Object:     e.Configuration,
			Source:     entityid.PlatformSemsource,
			Timestamp:  now,
			Confidence: 1.0,
		})
	}
	return triples
}

func (e *gradleDependencyEntity) entityState() *handler.EntityState {
	return &handler.EntityState{ID: e.ID, Triples: e.triples(), UpdatedAt: e.IndexedAt}
}

// --------------------------------------------------------------------------
// parseFileEntityStates dispatches to the right typed-entity builder based on
// the base filename. Mirrors parseFile but returns []*handler.EntityState
// instead of []handler.RawEntity.
// --------------------------------------------------------------------------

func (h *ConfigHandler) parseFileEntityStates(base, path string, content []byte, root, org string, now time.Time) []*handler.EntityState {
	system := systemSlug(root)
	switch base {
	case "go.mod":
		return h.goModEntityStates(content, path, system, org, now)
	case "package.json":
		return h.npmPackageEntityStates(content, path, system, org, now)
	case "Dockerfile":
		return h.dockerEntityStates(content, path, system, org, now)
	case "pom.xml":
		return h.mavenEntityStates(content, path, system, org, now)
	case "build.gradle":
		return h.gradleEntityStates(content, path, system, org, now)
	}
	return nil
}

func (h *ConfigHandler) goModEntityStates(content []byte, path, system, org string, now time.Time) []*handler.EntityState {
	result, err := ParseGoMod(content)
	if err != nil {
		h.logger.Warn("cfgfile: parse go.mod failed", "path", path, "error", err)
		return nil
	}

	mod := newGoModuleEntity(org, result.Module, result.GoVersion, path, system, now)

	var depStates []*handler.EntityState
	for _, dep := range result.Deps {
		d := newGoDependencyEntity(org, dep.Path, dep.Version, dep.Indirect, system, now)
		mod.DepIDs = append(mod.DepIDs, d.ID)
		depStates = append(depStates, d.entityState())
	}

	// Module entity first so consumers see the root before its leaves.
	return append([]*handler.EntityState{mod.entityState()}, depStates...)
}

func (h *ConfigHandler) npmPackageEntityStates(content []byte, path, system, org string, now time.Time) []*handler.EntityState {
	result, err := ParsePackageJSON(content)
	if err != nil {
		h.logger.Warn("cfgfile: parse package.json failed", "path", path, "error", err)
		return nil
	}

	pkg := newNPMPackageEntity(org, result.Name, result.Version, path, system, now)

	var depStates []*handler.EntityState
	for _, dep := range result.Deps {
		d := newNPMDependencyEntity(org, dep.Name, dep.Version, dep.Dev, system, now)
		pkg.DepIDs = append(pkg.DepIDs, d.ID)
		depStates = append(depStates, d.entityState())
	}

	return append([]*handler.EntityState{pkg.entityState()}, depStates...)
}

func (h *ConfigHandler) dockerEntityStates(content []byte, path, system, org string, now time.Time) []*handler.EntityState {
	result, err := ParseDockerfile(content)
	if err != nil {
		h.logger.Warn("cfgfile: parse Dockerfile failed", "path", path, "error", err)
		return nil
	}

	var states []*handler.EntityState
	for _, img := range result.BaseImages {
		e := newDockerImageEntity(org, img, result.ExposedPorts, path, system, now)
		states = append(states, e.entityState())
	}
	return states
}

func (h *ConfigHandler) mavenEntityStates(content []byte, path, system, org string, now time.Time) []*handler.EntityState {
	result, err := ParsePOM(content)
	if err != nil {
		h.logger.Warn("cfgfile: parse pom.xml failed", "path", path, "error", err)
		return nil
	}

	proj := newMavenProjectEntity(org, result.GroupID, result.ArtifactID, result.Version, result.Packaging, path, system, now)

	var childStates []*handler.EntityState
	for _, dep := range result.Deps {
		d := newMavenDependencyEntity(org, dep.GroupID, dep.ArtifactID, dep.Version, dep.Scope, system, now)
		proj.DepIDs = append(proj.DepIDs, d.ID)
		childStates = append(childStates, d.entityState())
	}
	for _, mod := range result.Modules {
		m := newPOMModuleEntity(org, mod, path, system, now)
		proj.ModuleIDs = append(proj.ModuleIDs, m.ID)
		childStates = append(childStates, m.entityState())
	}

	return append([]*handler.EntityState{proj.entityState()}, childStates...)
}

func (h *ConfigHandler) gradleEntityStates(content []byte, path, system, org string, now time.Time) []*handler.EntityState {
	result, err := ParseGradle(content)
	if err != nil {
		h.logger.Warn("cfgfile: parse build.gradle failed", "path", path, "error", err)
		return nil
	}

	dirName := filepath.Base(filepath.Dir(path))
	proj := newGradleProjectEntity(org, dirName, path, system, now)

	var depStates []*handler.EntityState
	for _, dep := range result.Deps {
		d := newGradleDependencyEntity(org, dep.Group, dep.Name, dep.Version, dep.Configuration, system, now)
		proj.DepIDs = append(proj.DepIDs, d.ID)
		depStates = append(depStates, d.entityState())
	}

	return append([]*handler.EntityState{proj.entityState()}, depStates...)
}
