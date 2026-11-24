package sourcespawn

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	semconfig "github.com/c360studio/semengine/config"
	"github.com/c360studio/semengine/types"
	"github.com/c360studio/semsource/config"
	"github.com/c360studio/semsource/entityid"
)

// PlannedComponent is one component a deployment would spawn. Pin carries the
// one fact the component config does not: boot discovery made it as a
// submodule pin (config.SourceEntry.InstanceSuffix is never serialized).
type PlannedComponent struct {
	Instance string
	Config   types.ComponentConfig
	Pin      bool

	pending bool // an add_source spec, not yet registered
}

// systemKey is what two roots must share to mint the same entity IDs (#349):
// the ID space (ast code IDs and web document IDs differ in their domain
// segment; doc-source and objectstore-source both mint web document and chunk
// IDs, so they share one), the org and the system segment.
type systemKey struct{ kind, org, system string }

// idSpace is the ID space each component kind in the rule mints into.
var idSpace = map[string]string{
	"ast-source":         "ast",
	"doc-source":         "doc",
	"objectstore-source": "doc",
}

type systemRoot struct {
	key          systemKey
	instance     string
	root         string
	pin, pending bool
	// languages is what an ast root indexes, sorted; sha is a pin's gitlink
	// SHA as its identity records it (shortened).
	languages []string
	sha       string
}

// collisionConfig reads only the identity inputs of ast-source, doc-source and
// objectstore-source configs, as cmd/semsource sourceScopeSystems does.
type collisionConfig struct {
	WatchPaths []struct {
		Path      string   `json:"path"`
		Org       string   `json:"org"`
		Project   string   `json:"project"`
		Version   string   `json:"version"`
		Languages []string `json:"languages"`
	} `json:"watch_paths"`
	Org     string   `json:"org"`
	Paths   []string `json:"paths"`
	Project string   `json:"project"`
	// objectstore-source
	Version  string `json:"version"`
	Bucket   string `json:"bucket"`
	Prefix   string `json:"prefix"`
	Endpoint string `json:"endpoint"`
}

// systemRoots returns the roots of c with the system each mints IDs under,
// computed as the component does: ast-source ScopedSystemSlug(project,
// version) per watch path (processor/ast-source initializeWatchers), doc-source
// SystemSlug(project) when set, else SystemSlug(root) (handler/doc system),
// objectstore-source ScopedSystemSlug(project or bucket, version) for its one
// bucket (handler/objectstore System). Other kinds are outside the rule and
// yield none.
func systemRoots(c PlannedComponent) ([]systemRoot, error) {
	space, ok := idSpace[c.Config.Name]
	if !ok {
		return nil, nil
	}
	var cfg collisionConfig
	if err := json.Unmarshal(c.Config.Config, &cfg); err != nil {
		return nil, fmt.Errorf("read identity of %s (%s): %w", c.Instance, c.Config.Name, err)
	}
	var roots []systemRoot
	add := func(org, system, root string) *systemRoot {
		roots = append(roots, systemRoot{
			key:      systemKey{kind: space, org: org, system: system},
			instance: c.Instance, root: root, pin: c.Pin, pending: c.pending,
		})
		return &roots[len(roots)-1]
	}
	switch c.Config.Name {
	case "ast-source":
		for _, wp := range cfg.WatchPaths {
			r := add(wp.Org, entityid.ScopedSystemSlug(wp.Project, wp.Version), wp.Path)
			r.languages = indexedLanguages(wp.Languages)
			if c.Pin {
				r.sha = wp.Version
			}
		}
		return roots, nil
	case "objectstore-source":
		project := cfg.Bucket
		if cfg.Project != "" {
			project = cfg.Project
		}
		add(cfg.Org, entityid.ScopedSystemSlug(project, cfg.Version), bucketRoot(cfg))
		return roots, nil
	}
	for _, path := range cfg.Paths {
		system := entityid.SystemSlug(path)
		if cfg.Project != "" {
			system = entityid.SystemSlug(cfg.Project)
		}
		r := add(cfg.Org, system, path)
		if c.Pin {
			// A docs pin carries its SHA as the project suffix
			// (internal/subwatch pinEntries: project + "-" + sha12).
			// shortcut: parsed back from the project, since docs pins have
			// no version field; carry the SHA explicitly if pinEntries'
			// project shape ever changes.
			r.sha = cfg.Project[strings.LastIndex(cfg.Project, "-")+1:]
		}
	}
	return roots, nil
}

// indexedLanguages is the language set an ast watch path indexes: none
// configured means go, as the ast-source component and astComponentConfig
// default it.
func indexedLanguages(langs []string) []string {
	if len(langs) == 0 {
		return []string{"go"}
	}
	return slices.Sorted(slices.Values(langs))
}

// bucketRoot labels an object-store source's root: its endpoint, bucket and
// prefix, those that are set.
func bucketRoot(cfg collisionConfig) string {
	var parts []string
	for _, p := range []string{strings.TrimSuffix(cfg.Endpoint, "/"), cfg.Bucket, cfg.Prefix} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, "/")
}

// collision is one system that more than one root would mint IDs under.
type collision struct {
	key   systemKey
	roots []systemRoot
	// differs says how the non-pin roots of a group with a pin differ from
	// the pin; empty for a group without one.
	differs []string
}

func (c collision) involvesPending() bool {
	return slices.ContainsFunc(c.roots, func(r systemRoot) bool { return r.pending })
}

func (c collision) String() string {
	members := make([]string, 0, len(c.roots))
	for _, r := range c.roots {
		members = append(members, fmt.Sprintf("%s (root %s)", r.instance, r.root))
	}
	msg := fmt.Sprintf("%s roots %s would share entity-ID system %q in org %q",
		c.key.kind, strings.Join(members, " and "), c.key.system, c.key.org)
	if len(c.differs) > 0 {
		msg += " but differ from the submodule pin (" + strings.Join(c.differs, ", ") + ")"
	}
	return msg
}

// collisions groups every enabled root by its system key and returns each
// group with more than one root, except a group that includes a submodule pin
// and whose other roots hold the pin's content (D2, narrowed by the #349
// review ruling). Discovery expands one submodule at one gitlink SHA into
// byte-identical sources, and an equal system with a pin means every other
// root declared the pin's identity: for ast the same project and version (the
// gitlink SHA), for docs a project that carries the SHA. The declaration is
// checked against the checkout: each other root's HEAD must be the pinned SHA,
// and an ast root must index the pin's languages, or the roots would write
// different content under the pin's IDs.
func collisions(ctx context.Context, components []PlannedComponent) ([]collision, error) {
	roots, err := enabledRoots(components)
	if err != nil {
		return nil, err
	}
	return groupCollisions(ctx, roots), nil
}

// enabledRoots is every root of the enabled components, ordered by instance.
func enabledRoots(components []PlannedComponent) ([]systemRoot, error) {
	sorted := slices.Clone(components)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Instance < sorted[j].Instance })
	var out []systemRoot
	for _, c := range sorted {
		if !c.Config.Enabled {
			continue // a disabled envelope is removal intent; it does not spawn
		}
		roots, err := systemRoots(c)
		if err != nil {
			return nil, err
		}
		out = append(out, roots...)
	}
	return out, nil
}

func groupCollisions(ctx context.Context, roots []systemRoot) []collision {
	groups := map[systemKey][]systemRoot{}
	var order []systemKey
	for _, r := range roots {
		if _, seen := groups[r.key]; !seen {
			order = append(order, r.key)
		}
		groups[r.key] = append(groups[r.key], r)
	}
	var out []collision
	for _, key := range order {
		members := groups[key]
		if len(members) < 2 {
			continue
		}
		pin := slices.IndexFunc(members, func(r systemRoot) bool { return r.pin })
		if pin < 0 {
			out = append(out, collision{key: key, roots: members})
			continue
		}
		if differs := pinDifferences(ctx, members[pin], members); len(differs) > 0 {
			out = append(out, collision{key: key, roots: members, differs: differs})
		}
	}
	return out
}

// pinDifferences says how each non-pin member differs from pin: a checkout
// HEAD that is not the pinned SHA (or cannot be read), or, for ast, another
// language set. Pins of one system are one gitlink SHA by construction.
func pinDifferences(ctx context.Context, pin systemRoot, members []systemRoot) []string {
	var out []string
	for _, r := range members {
		if r.pin {
			continue
		}
		head, err := checkoutHead(ctx, r.root)
		switch {
		case err != nil:
			out = append(out, fmt.Sprintf("%s HEAD unreadable, so it cannot be shown to hold pinned SHA %s: %v", r.root, pin.sha, err))
		case pin.sha == "" || !strings.HasPrefix(head, strings.ToLower(pin.sha)):
			out = append(out, fmt.Sprintf("%s HEAD is %s, not pinned SHA %q of %s", r.root, head, pin.sha, pin.root))
		}
		if !slices.Equal(r.languages, pin.languages) {
			out = append(out, fmt.Sprintf("%s indexes %v, pin %s indexes %v", r.root, r.languages, pin.root, pin.languages))
		}
	}
	return out
}

// checkoutHead is the full SHA of the commit checked out at root, as
// handler/git headCommitSHA reads it.
func checkoutHead(ctx context.Context, root string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--verify", "HEAD")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) && len(exit.Stderr) > 0 {
			return "", fmt.Errorf("git rev-parse HEAD: %s", strings.TrimSpace(string(exit.Stderr)))
		}
		return "", fmt.Errorf("git rev-parse HEAD: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func collisionError(found []collision) error {
	if len(found) == 0 {
		return nil
	}
	msgs := make([]string, 0, len(found))
	for _, c := range found {
		msgs = append(msgs, c.String())
	}
	return &Error{
		Code: CodeValidationFailed,
		Message: strings.Join(msgs, "; ") +
			"; each would replace the other's entities, so give each source a distinct project (or version) (#349)",
	}
}

// CheckSystemCollisions refuses a component set in which two ast-source roots,
// or two document roots (doc-source paths, objectstore-source buckets), would
// mint entity IDs under one org and system, unless the collision includes a
// submodule pin and every other root's checkout is at the pinned commit and
// indexes the pin's languages (D2 as the #349 review narrowed it; reading a
// HEAD runs git under ctx). The error names each colliding instance and root,
// the shared system and any difference from the pin, with CodeValidationFailed.
func CheckSystemCollisions(ctx context.Context, components []PlannedComponent) error {
	found, err := collisions(ctx, components)
	if err != nil {
		return &Error{Code: CodeValidationFailed, Message: "check entity-ID systems", Cause: err}
	}
	return collisionError(found)
}

// Planned returns the components a component snapshot would spawn, marking
// as pins the instances in pins.
func Planned(components semconfig.ComponentConfigs, pins map[string]bool) []PlannedComponent {
	out := make([]PlannedComponent, 0, len(components))
	for name, cc := range components {
		out = append(out, PlannedComponent{Instance: name, Config: cc, Pin: pins[name]})
	}
	return out
}

// CheckSourceCollisions builds the ast, doc and object-store components of sources and runs
// CheckSystemCollisions over them, for a file's sources before boot. Two
// entries that build one instance over the same roots are one source; two that
// build one instance over different roots collide (the second would replace
// the first's component). Multi-branch repos are skipped: their branches are
// listed from a checkout, which is boot's job, and boot checks them. Nothing
// here clones or contacts a remote: single-branch repo expansion derives its
// paths from the URL, and only the git leaf, which is outside the rule,
// resolves a remote branch.
func CheckSourceCollisions(ctx context.Context, sources []config.SourceEntry, opts Options) error {
	var planned []PlannedComponent
	seen := map[string][]string{}
	for _, src := range sources {
		entries := []config.SourceEntry{src}
		if src.Type == "repo" {
			if len(src.Branches) > 0 {
				continue
			}
			expanded, err := config.ExpandRepoSources(ctx, entries, opts.WorkspaceDir)
			if err != nil {
				return &Error{Code: CodeValidationFailed, Message: "expand repo source", Cause: err}
			}
			entries = expanded.Sources
		}
		for _, entry := range entries {
			if entry.Type != "ast" && entry.Type != "docs" && entry.Type != "s3" {
				continue
			}
			specs, err := buildSpecs(ctx, entry, opts)
			if err != nil {
				return err
			}
			for _, spec := range specs {
				pc, err := plannedFromSpec(spec, entry.InstanceSuffix != "")
				if err != nil {
					return err
				}
				roots, err := rootList(pc)
				if err != nil {
					return err
				}
				if prev, dup := seen[pc.Instance]; dup && slices.Equal(prev, roots) {
					continue
				}
				seen[pc.Instance] = roots
				planned = append(planned, pc)
			}
		}
	}
	return CheckSystemCollisions(ctx, planned)
}

// checkAddCollisions refuses an add whose specs would share a system with a
// registered source or with each other. A registered root the add rebuilds
// under the same instance and system is the same writer (a re-add or refresh)
// and leaves the comparison. A registered root the add's instance would drop
// stays: the add would move that instance onto another root of one system,
// which is a second writer, not an update. Collisions among registered sources
// alone predate the request and are boot's to refuse.
func checkAddCollisions(ctx context.Context, existing semconfig.ComponentConfigs, specs []componentSpec) error {
	pending := make([]PlannedComponent, 0, len(specs))
	for _, spec := range specs {
		pc, err := plannedFromSpec(spec, false)
		if err != nil {
			return err
		}
		pc.pending = true
		pending = append(pending, pc)
	}
	pins, err := PinInstances(existing)
	if err != nil {
		return &Error{Code: CodeValidationFailed, Message: "check entity-ID systems", Cause: err}
	}
	registered := Planned(existing, pins)
	pendingRoots, err := enabledRoots(pending)
	if err != nil {
		return &Error{Code: CodeValidationFailed, Message: "check entity-ID systems", Cause: err}
	}
	registeredRoots, err := enabledRoots(registered)
	if err != nil {
		return &Error{Code: CodeValidationFailed, Message: "check entity-ID systems", Cause: err}
	}
	found := groupCollisions(ctx, oneWriterEach(pendingRoots, registeredRoots))
	if err := collisionError(slices.DeleteFunc(found, func(c collision) bool { return !c.involvesPending() })); err != nil {
		return err
	}
	return discoveredTakeover(existing, pending)
}

// discoveredTakeover refuses a pending component whose instance the
// boot-discovery record owns unless it builds that instance from the roots
// discovery's own component holds (#349 B1). Boot discovery rewrites a name it
// owns as its own refresh (a moved worktree still boots), so a runtime source
// admitted under that name from another root would be overwritten at the next
// boot. The record carries names, not roots: the discovered root is read from
// the component kept under the name, enabled or disabled by a runtime removal.
// A recorded name with no component kept cannot be shown to be the same
// writer and is refused too.
func discoveredTakeover(existing semconfig.ComponentConfigs, pending []PlannedComponent) error {
	owned, err := discoveredNames(existing)
	if err != nil {
		return &Error{Code: CodeValidationFailed, Message: "check entity-ID systems", Cause: err}
	}
	var msgs []string
	for _, pc := range pending {
		if _, ok := owned[pc.Instance]; !ok {
			continue
		}
		want, err := rootPaths(pc)
		if err != nil {
			return err
		}
		have, kept := existing[pc.Instance]
		if !kept {
			msgs = append(msgs, fmt.Sprintf("%s (root %s) is a name boot discovery owns, and no discovered root is kept to show it is the same source",
				pc.Instance, strings.Join(want, ", ")))
			continue
		}
		held, err := rootPaths(PlannedComponent{Instance: pc.Instance, Config: have})
		if err != nil {
			return err
		}
		if !slices.Equal(held, want) {
			msgs = append(msgs, fmt.Sprintf("%s (root %s) would take over a name boot discovery owns from discovered root %s",
				pc.Instance, strings.Join(want, ", "), strings.Join(held, ", ")))
		}
	}
	if len(msgs) == 0 {
		return nil
	}
	return &Error{
		Code: CodeValidationFailed,
		Message: strings.Join(msgs, "; ") +
			"; the next boot's discovery would replace it, so give the source a distinct project (or version) (#349)",
	}
}

// rootPaths is a component's roots, cleaned and sorted.
func rootPaths(c PlannedComponent) ([]string, error) {
	roots, err := systemRoots(c)
	if err != nil {
		return nil, &Error{Code: CodeValidationFailed, Message: "check entity-ID systems", Cause: err}
	}
	out := make([]string, 0, len(roots))
	for _, r := range roots {
		out = append(out, filepath.Clean(r.root))
	}
	slices.Sort(out)
	return out, nil
}

// oneWriterEach is incoming plus each held root that is not one of incoming's
// writers: a held root that incoming rebuilds under the same instance and
// system from the same directory is the same writer (a refresh), so it is
// counted once; any other held root stays and can collide.
func oneWriterEach(incoming, held []systemRoot) []systemRoot {
	same := map[string]bool{}
	for _, r := range incoming {
		same[r.identity()] = true
	}
	roots := slices.Clone(incoming)
	for _, r := range held {
		if !same[r.identity()] {
			roots = append(roots, r)
		}
	}
	return roots
}

// CheckDiscoveredCollisions refuses boot discovery's writes when the
// components it is about to write (discovered) and the components in effect
// that stay (retained) would mint IDs under one system, before anything is
// written: a discovered component that builds a retained component's instance
// from another root would otherwise replace it, and a later check would see
// only the survivor. A retained root discovery rebuilds under the same
// instance and system from the same directory is the same writer, and the pin
// exception applies (D2). The set is checked whole: retained collisions that
// predate discovery are refused here too, as boot would refuse them anyway.
func CheckDiscoveredCollisions(ctx context.Context, retained, discovered []PlannedComponent) error {
	discoveredRoots, err := enabledRoots(discovered)
	if err != nil {
		return &Error{Code: CodeValidationFailed, Message: "check entity-ID systems", Cause: err}
	}
	retainedRoots, err := enabledRoots(retained)
	if err != nil {
		return &Error{Code: CodeValidationFailed, Message: "check entity-ID systems", Cause: err}
	}
	return collisionError(groupCollisions(ctx, oneWriterEach(discoveredRoots, retainedRoots)))
}

// identity is what makes two roots one writer: one instance minting one
// system from one directory.
func (r systemRoot) identity() string {
	return r.instance + "\x00" + r.key.kind + "\x00" + r.key.org + "\x00" + r.key.system + "\x00" + filepath.Clean(r.root)
}

func plannedFromSpec(spec componentSpec, pin bool) (PlannedComponent, error) {
	raw, err := json.Marshal(spec.compCfg)
	if err != nil {
		return PlannedComponent{}, &Error{
			Code:    CodeValidationFailed,
			Message: fmt.Sprintf("marshal component config for %q", spec.instanceName),
			Cause:   err,
		}
	}
	return PlannedComponent{
		Instance: spec.instanceName,
		Config:   types.ComponentConfig{Name: spec.factoryName, Type: types.ComponentTypeProcessor, Enabled: true, Config: raw},
		Pin:      pin,
	}, nil
}

// rootList is a component's root identities in config order: what a second
// listing of one source must match to be the same source.
func rootList(c PlannedComponent) ([]string, error) {
	roots, err := systemRoots(c)
	if err != nil {
		return nil, &Error{Code: CodeValidationFailed, Message: "check entity-ID systems", Cause: err}
	}
	out := make([]string, 0, len(roots))
	for _, r := range roots {
		out = append(out, r.identity())
	}
	return out, nil
}

// PinInstances returns the instances boot-time discovery recorded as submodule
// pins: the "discovered" record (kind "submodule") in the source-manifest
// envelope of components. Component configs do not carry the InstanceSuffix
// that marks a pin, and the record is what discovery owns.
func PinInstances(components semconfig.ComponentConfigs) (map[string]bool, error) {
	names, err := discoveredNames(components)
	if err != nil {
		return nil, err
	}
	pins := map[string]bool{}
	for name, kind := range names {
		if kind == "submodule" {
			pins[name] = true
		}
	}
	return pins, nil
}

// discoveredNames returns every instance the "discovered" record in the
// source-manifest envelope of components files, with the kind it is filed
// under. The record is decoded here rather than through the source-manifest
// package, which imports this one.
func discoveredNames(components semconfig.ComponentConfigs) (map[string]string, error) {
	names := map[string]string{}
	envelope, ok := components["source-manifest"]
	if !ok {
		return names, nil
	}
	var m struct {
		Discovered []struct {
			Kind       string   `json:"kind"`
			Components []string `json:"components"`
		} `json:"discovered"`
	}
	if err := json.Unmarshal(envelope.Config, &m); err != nil {
		return nil, fmt.Errorf("decode source-manifest discovery record: %w", err)
	}
	for _, r := range m.Discovered {
		for _, name := range r.Components {
			names[name] = r.Kind
		}
	}
	return names, nil
}
