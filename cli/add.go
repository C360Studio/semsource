package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/c360studio/semsource/config"
	"github.com/c360studio/semsource/storage/s3store"
)

// Add either runs the interactive wizard to add a source (no extra args),
// or parses non-interactive flags for the given typeKey.
//
// args are the arguments after "semsource add", e.g. ["ast", "--path", "./src"].
func Add(term *Term, configPath string, args []string) error {
	if configPath == "" {
		configPath = defaultConfigPath
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	var entry *config.SourceEntry

	if len(args) > 0 {
		// Non-interactive: semsource add <type> [flags]
		typeKey := args[0]
		entry, err = addNonInteractive(typeKey, args[1:])
		if err != nil {
			return err
		}
	} else {
		// Interactive: show source type menu (single select).
		wizards := Wizards()
		var available []SourceWizard
		var labels []string
		for _, w := range wizards {
			ok, reason := w.Available()
			if !ok {
				labels = append(labels, fmt.Sprintf("%s — %s (%s)", w.Name(), w.Description(), reason))
			} else {
				labels = append(labels, fmt.Sprintf("%s — %s", w.Name(), w.Description()))
			}
			available = append(available, w)
		}

		idx := term.Select("Choose source type to add", labels)
		chosen := available[idx]

		ok, reason := chosen.Available()
		if !ok {
			return fmt.Errorf("%s is %s", chosen.Name(), reason)
		}

		entry, err = chosen.Prompts(term)
		if err != nil {
			return err
		}
	}

	// Validate before writing. A source entry that cannot load is worse than a
	// rejected one: `semsource run` would refuse to start, and the operator
	// would be debugging a file this command told them was fine.
	if err := entry.Validate(); err != nil {
		return err
	}
	if err := verifySource(entry); err != nil {
		return err
	}

	cfg.Sources = append(cfg.Sources, *entry)

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, append(data, '\n'), 0644); err != nil {
		return fmt.Errorf("write %s: %w", configPath, err)
	}

	term.Success(fmt.Sprintf("Added %s source to %s", entry.Type, configPath))
	return nil
}

// verifySource checks that a new source entry describes something reachable,
// before anything is written.
//
// Indirected through a package variable so tests can drive both outcomes
// without a bucket, a network, or credentials.
var verifySource = func(entry *config.SourceEntry) error {
	if entry.Type != "s3" {
		// Every other source type is a path or a URL whose reachability is the
		// ingest loop's business — a directory that does not exist yet is a
		// normal thing to register.
		return nil
	}
	return verifyObjectStore(entry)
}

// verifyObjectStore probes the configured bucket.
//
// A bucket is worth probing where a directory is not: the failure modes are a
// wrong endpoint, a credential that is not in the environment, and a bucket
// that does not exist — all of which are invisible until the first ingest, and
// all of which the operator can fix in the second it takes to read the error.
// The listing is the probe because it exercises all three in one request.
func verifyObjectStore(entry *config.SourceEntry) error {
	store, err := s3store.New(s3store.Config{
		Bucket:    entry.Bucket,
		Endpoint:  entry.Endpoint,
		Region:    entry.Region,
		PathStyle: entry.PathStyle,
	})
	if err != nil {
		return fmt.Errorf("object store %s bucket %q is not usable: %w",
			endpointLabel(entry.Endpoint), entry.Bucket, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), objectStoreProbeTimeout)
	defer cancel()

	if _, err := store.Objects(ctx, entry.Prefix); err != nil {
		// Endpoint, bucket, and cause: without all three the operator cannot
		// tell a typo in the bucket from a credential that is not exported.
		return fmt.Errorf("cannot reach bucket %q at %s: %w\n"+
			"  check the endpoint, that the bucket exists, and that %s and %s are set in the environment",
			entry.Bucket, endpointLabel(entry.Endpoint), err,
			s3store.EnvAccessKeyID, s3store.EnvSecretAccessKey)
	}
	return nil
}

// objectStoreProbeTimeout bounds the reachability check. Short: this runs in
// front of a human at a terminal, and a store that cannot answer a listing in
// this long is not one to register without noticing.
const objectStoreProbeTimeout = 15 * time.Second

// endpointLabel names an endpoint for an error message, including the case
// where none was configured.
func endpointLabel(endpoint string) string {
	if endpoint == "" {
		return s3store.DefaultEndpoint + " (the default)"
	}
	return endpoint
}

// addNonInteractive dispatches to a type-specific flag parser.
func addNonInteractive(typeKey string, args []string) (*config.SourceEntry, error) {
	switch typeKey {
	case "ast":
		return parseASTFlags(args)
	case "git":
		return parseGitFlags(args)
	case "repo":
		return parseRepoFlags(args)
	case "docs":
		return parseDocsFlags(args)
	case "config":
		return parseConfigFlags(args)
	case "url":
		return parseURLFlags(args)
	case "s3":
		return parseS3Flags(args)
	case "image":
		return parseImageFlags(args)
	case "video":
		return parseVideoFlags(args)
	case "audio":
		return parseAudioFlags(args)
	default:
		return nil, fmt.Errorf("unknown source type %q (valid: %s)", typeKey, strings.Join(config.SourceTypes(), ", "))
	}
}

// parseS3Flags parses the flags for an S3-compatible object-store source.
//
// There is no credential flag, and there must never be one: this writes to
// semsource.json, which is watched and replicated through KV, so a key placed
// here would be distributed well beyond the process that needs it. The
// credentials come from the process environment at run time.
func parseS3Flags(args []string) (*config.SourceEntry, error) {
	fs := flag.NewFlagSet("add s3", flag.ContinueOnError)
	bucket := fs.String("bucket", "", "bucket holding the document artifacts")
	prefix := fs.String("prefix", "", "key prefix to scope ingestion to (empty means the whole bucket)")
	endpoint := fs.String("endpoint", "", "S3-compatible endpoint URL, scheme included (empty means AWS)")
	region := fs.String("region", "", "region forwarded for request signing")
	pathStyle := fs.Bool("path-style", false, "use path-style bucket addressing (needed by most self-hosted stores)")
	watch := fs.Bool("watch", true, "re-list the prefix on an interval to pick up changes")
	// Explicit identity, the same pair every other source takes: project is
	// what supersession corresponds entities by, version is what lets two
	// snapshots of one corpus coexist. Omitting both derives identity from the
	// bucket, byte-for-byte.
	project := fs.String("project", "", "explicit project identity (overrides the bucket-derived slug)")
	version := fs.String("version", "", "explicit version for this registration")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if *bucket == "" {
		return nil, fmt.Errorf("s3 source requires --bucket")
	}

	return &config.SourceEntry{
		Type:      "s3",
		Bucket:    *bucket,
		Prefix:    *prefix,
		Endpoint:  *endpoint,
		Region:    *region,
		PathStyle: *pathStyle,
		Watch:     *watch,
		Project:   *project,
		Version:   *version,
	}, nil
}

func parseASTFlags(args []string) (*config.SourceEntry, error) {
	fs := flag.NewFlagSet("add ast", flag.ContinueOnError)
	path := fs.String("path", ".", "root path to scan")
	language := fs.String("language", "", "language (go, typescript, python, java, svelte)")
	watch := fs.Bool("watch", true, "watch for changes")
	// Explicit identity (the config file's project/version pair): same
	// project at two versions is what code_changes diffs; omission keeps
	// path-derived identity.
	project := fs.String("project", "", "explicit project identity (overrides path-derived)")
	version := fs.String("version", "", "explicit version for this registration")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	return &config.SourceEntry{
		Type:     "ast",
		Path:     *path,
		Language: *language,
		Watch:    *watch,
		Project:  *project,
		Version:  *version,
	}, nil
}

func parseGitFlags(args []string) (*config.SourceEntry, error) {
	fs := flag.NewFlagSet("add git", flag.ContinueOnError)
	url := fs.String("url", "", "repository path or URL")
	branch := fs.String("branch", "main", "branch to track")
	watch := fs.Bool("watch", true, "watch for new commits")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if *url == "" {
		return nil, fmt.Errorf("git source requires --url")
	}
	return &config.SourceEntry{
		Type:   "git",
		URL:    *url,
		Branch: *branch,
		Watch:  *watch,
	}, nil
}

func parseRepoFlags(args []string) (*config.SourceEntry, error) {
	fs := flag.NewFlagSet("add repo", flag.ContinueOnError)
	url := fs.String("url", "", "remote repository URL to clone and analyze")
	branch := fs.String("branch", "", "branch (default: remote default)")
	language := fs.String("language", "", "primary language (go, java, python, typescript, or leave blank to auto-detect)")
	watch := fs.Bool("watch", true, "watch for changes")
	// Explicit identity, applied to the expanded code entries exactly as a
	// config-file declaration would be; omission keeps URL-derived identity.
	project := fs.String("project", "", "explicit project identity (overrides URL-derived)")
	version := fs.String("version", "", "explicit version for this registration")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if *url == "" {
		return nil, fmt.Errorf("repo source requires --url")
	}
	return &config.SourceEntry{
		Type:     "repo",
		URL:      *url,
		Branch:   *branch,
		Language: *language,
		Watch:    *watch,
		Project:  *project,
		Version:  *version,
	}, nil
}

func parseDocsFlags(args []string) (*config.SourceEntry, error) {
	fs := flag.NewFlagSet("add docs", flag.ContinueOnError)
	paths := fs.String("paths", "", "comma-separated list of paths")
	watch := fs.Bool("watch", true, "watch for changes")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if *paths == "" {
		return nil, fmt.Errorf("docs source requires --paths")
	}
	return &config.SourceEntry{
		Type:  "docs",
		Paths: splitComma(*paths),
		Watch: *watch,
	}, nil
}

func parseConfigFlags(args []string) (*config.SourceEntry, error) {
	fs := flag.NewFlagSet("add config", flag.ContinueOnError)
	paths := fs.String("paths", "", "comma-separated list of config file paths")
	watch := fs.Bool("watch", true, "watch for changes")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if *paths == "" {
		return nil, fmt.Errorf("config source requires --paths")
	}
	return &config.SourceEntry{
		Type:  "config",
		Paths: splitComma(*paths),
		Watch: *watch,
	}, nil
}

func parseURLFlags(args []string) (*config.SourceEntry, error) {
	fs := flag.NewFlagSet("add url", flag.ContinueOnError)
	urls := fs.String("urls", "", "comma-separated list of URLs")
	poll := fs.String("poll-interval", "5m", "poll interval")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if *urls == "" {
		return nil, fmt.Errorf("url source requires --urls")
	}
	return &config.SourceEntry{
		Type:         "url",
		URLs:         splitComma(*urls),
		PollInterval: *poll,
	}, nil
}

func parseImageFlags(args []string) (*config.SourceEntry, error) {
	fs := flag.NewFlagSet("add image", flag.ContinueOnError)
	paths := fs.String("paths", "", "comma-separated list of paths to scan")
	watch := fs.Bool("watch", true, "watch for changes")
	maxSize := fs.String("max-file-size", "50MB", "maximum file size")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if *paths == "" {
		return nil, fmt.Errorf("image source requires --paths")
	}
	return &config.SourceEntry{
		Type:        "image",
		Paths:       splitComma(*paths),
		Watch:       *watch,
		MaxFileSize: *maxSize,
	}, nil
}

func parseVideoFlags(args []string) (*config.SourceEntry, error) {
	fs := flag.NewFlagSet("add video", flag.ContinueOnError)
	paths := fs.String("paths", "", "comma-separated list of paths to scan")
	watch := fs.Bool("watch", true, "watch for changes")
	keyframeMode := fs.String("keyframe-mode", "interval", "keyframe extraction mode: interval, scene, or iframes")
	keyframeInterval := fs.String("keyframe-interval", "30s", "interval between keyframe extractions (e.g. 30s)")
	maxSize := fs.String("max-file-size", "2GB", "maximum file size")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if *paths == "" {
		return nil, fmt.Errorf("video source requires --paths")
	}
	return &config.SourceEntry{
		Type:             "video",
		Paths:            splitComma(*paths),
		Watch:            *watch,
		KeyframeMode:     *keyframeMode,
		KeyframeInterval: *keyframeInterval,
		MaxFileSize:      *maxSize,
	}, nil
}

func parseAudioFlags(args []string) (*config.SourceEntry, error) {
	fs := flag.NewFlagSet("add audio", flag.ContinueOnError)
	paths := fs.String("paths", "", "comma-separated list of paths to scan")
	watch := fs.Bool("watch", true, "watch for changes")
	maxSize := fs.String("max-file-size", "500MB", "maximum file size")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if *paths == "" {
		return nil, fmt.Errorf("audio source requires --paths")
	}
	return &config.SourceEntry{
		Type:        "audio",
		Paths:       splitComma(*paths),
		Watch:       *watch,
		MaxFileSize: *maxSize,
	}, nil
}

func splitComma(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
