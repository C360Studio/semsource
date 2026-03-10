// Package audio implements the Handler for audio file sources.
// It extracts metadata from audio files using ffprobe.
package audio

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/c360studio/semsource/handler"
	"github.com/c360studio/semstreams/storage"
)

// sourceTypeKey is the config source type key for audio sources.
const sourceTypeKey = "audio"

// defaultExtensions lists the file extensions Handler will process.
var defaultExtensions = map[string]bool{
	".mp3":  true,
	".wav":  true,
	".flac": true,
	".aac":  true,
	".ogg":  true,
	".m4a":  true,
	".wma":  true,
}

// Handler handles audio file sources.
// It implements handler.SourceHandler.
type Handler struct {
	store  storage.Store // nil = no binary storage (metadata only)
	logger *slog.Logger
	// org is the organisation namespace used when building EntityState values
	// via IngestEntityStates and enrichEvent. Empty disables the typed path.
	org string
}

// Option is a functional option for configuring an Handler.
type Option func(*Handler)

// WithStore sets the binary storage backend. When nil (the default), the
// handler records metadata triples only and skips binary storage.
func WithStore(s storage.Store) Option {
	return func(h *Handler) { h.store = s }
}

// WithLogger sets a custom structured logger on the handler.
func WithLogger(l *slog.Logger) Option {
	return func(h *Handler) { h.logger = l }
}

// WithOrg sets the organisation namespace used when building typed EntityState
// values via IngestEntityStates and Watch enrichment.
func WithOrg(org string) Option {
	return func(h *Handler) { h.org = org }
}

// New returns a ready-to-use Handler configured by the provided options.
// Calling New() with no options produces a metadata-only handler using the
// default slog logger.
func New(opts ...Option) *Handler {
	h := &Handler{}
	for _, opt := range opts {
		opt(h)
	}
	if h.logger == nil {
		h.logger = slog.Default()
	}
	return h
}

// SourceType returns the handler type identifier as used in semsource.json.
func (h *Handler) SourceType() string {
	return sourceTypeKey
}

// Supports returns true when cfg describes an "audio" source.
func (h *Handler) Supports(cfg handler.SourceConfig) bool {
	return cfg.GetType() == sourceTypeKey
}

// Ingest walks all paths in cfg, reads each supported audio file, extracts
// metadata, and returns a RawEntity per audio file. It respects ctx cancellation.
//
// Path resolution: GetPaths() is used when non-empty; otherwise GetPath() is
// used as a single-element list. An error is returned when both are empty.
func (h *Handler) Ingest(ctx context.Context, cfg handler.SourceConfig) ([]handler.RawEntity, error) {
	roots := resolvePaths(cfg)
	if len(roots) == 0 {
		return nil, fmt.Errorf("audio handler: no paths configured (set path or paths in source config)")
	}

	var entities []handler.RawEntity

	for _, root := range roots {
		err := filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}

			ext := strings.ToLower(filepath.Ext(path))
			if !defaultExtensions[ext] {
				return nil
			}

			audioEntity, err := h.ingestFile(ctx, path, root)
			if err != nil {
				// Non-fatal: skip unreadable or unsupported files and continue.
				h.logger.Warn("audio handler: skipping file", "path", path, "error", err)
				return nil
			}
			entities = append(entities, audioEntity)
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("audio handler: walk %q: %w", root, err)
		}
	}

	return entities, nil
}

// resolvePaths returns the list of root directories to process for cfg.
// It prefers GetPaths() when non-empty, falling back to a single-element
// slice containing GetPath(). Returns nil when both are empty.
func resolvePaths(cfg handler.SourceConfig) []string {
	if paths := cfg.GetPaths(); len(paths) > 0 {
		return paths
	}
	if p := cfg.GetPath(); p != "" {
		return []string{p}
	}
	return nil
}

// ingestFile streams a single audio file to compute its hash, probes metadata,
// and returns the audio RawEntity.
func (h *Handler) ingestFile(ctx context.Context, path, root string) (handler.RawEntity, error) {
	// Stream hash computation — avoid loading the entire audio file into memory.
	f, err := os.Open(path)
	if err != nil {
		return handler.RawEntity{}, fmt.Errorf("open %q: %w", path, err)
	}
	hasher := sha256.New()
	fileSize, err := io.Copy(hasher, f)
	f.Close()
	if err != nil {
		return handler.RawEntity{}, fmt.Errorf("hash %q: %w", path, err)
	}
	hash := hex.EncodeToString(hasher.Sum(nil))
	instance := hash[:6]

	relPath, _ := filepath.Rel(root, path)
	ext := strings.ToLower(filepath.Ext(path))
	mimeType := mimeForExt(ext)
	format := formatForExt(ext)
	system := slugify(root)

	// Extract audio metadata via ffprobe. Non-fatal on failure.
	pr, probeErr := probe(ctx, path)
	if probeErr != nil {
		h.logger.Warn("audio handler: ffprobe failed, metadata will be partial",
			"path", path, "error", probeErr)
		pr = &ProbeResult{}
	}

	audioEntity := handler.RawEntity{
		SourceType: handler.SourceTypeAudio,
		Domain:     handler.DomainMedia,
		System:     system,
		EntityType: "audio",
		Instance:   instance,
		Properties: map[string]any{
			"media_type":  "audio",
			"file_path":   relPath,
			"mime_type":   mimeType,
			"file_hash":   hash,
			"file_size":   fileSize,
			"format":      format,
			"duration":    pr.Duration.Seconds(),
			"codec":       pr.Codec,
			"bitrate":     pr.Bitrate,
			"sample_rate": pr.SampleRate,
			"channels":    pr.Channels,
			"bit_depth":   pr.BitDepth,
		},
	}

	// Only read full file content when a store is configured for binary persistence.
	if h.store != nil {
		content, err := os.ReadFile(path)
		if err != nil {
			h.logger.Warn("audio handler: failed to read file for storage", "path", path, "error", err)
		} else {
			storageKey := fmt.Sprintf("audio/%s/%s/original", system, instance)
			if err := h.store.Put(ctx, storageKey, content); err != nil {
				h.logger.Warn("audio handler: failed to store audio binary",
					"path", path, "error", err)
			} else {
				audioEntity.Properties["storage_ref"] = storageKey
			}
		}
	}

	return audioEntity, nil
}

// ProbeResult holds the metadata extracted from an audio file via ffprobe.
// Exported so that export_test.go can surface it to the external test package.
type ProbeResult struct {
	Duration   time.Duration
	Codec      string
	Bitrate    int
	SampleRate int
	Channels   int
	BitDepth   int
}

// ffprobeOutput is the JSON structure returned by ffprobe.
type ffprobeOutput struct {
	Streams []struct {
		CodecName        string `json:"codec_name"`
		CodecType        string `json:"codec_type"`
		SampleRate       string `json:"sample_rate"`
		Channels         int    `json:"channels"`
		BitsPerRawSample string `json:"bits_per_raw_sample"`
	} `json:"streams"`
	Format struct {
		Duration string `json:"duration"`
		BitRate  string `json:"bit_rate"`
	} `json:"format"`
}

// probe runs ffprobe on path and returns the extracted audio metadata.
// Returns an error when ffprobe is not available or the file is unreadable.
func probe(ctx context.Context, path string) (*ProbeResult, error) {
	cmd := exec.CommandContext(ctx, "ffprobe",
		"-v", "quiet",
		"-print_format", "json",
		"-show_streams",
		"-show_format",
		path,
	)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe: %w", err)
	}
	return parseProbeOutput(out)
}

// parseProbeOutput decodes raw ffprobe JSON into a ProbeResult.
// Finds the first audio stream for codec, sample_rate, channels, and bit_depth.
// Exported for testability — tests can call this directly with mock JSON.
func parseProbeOutput(data []byte) (*ProbeResult, error) {
	var raw ffprobeOutput
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse ffprobe output: %w", err)
	}

	pr := &ProbeResult{}

	// Find the first audio stream.
	for _, s := range raw.Streams {
		if s.CodecType != "audio" {
			continue
		}
		pr.Codec = s.CodecName
		pr.Channels = s.Channels

		if s.SampleRate != "" {
			if sr, err := strconv.Atoi(s.SampleRate); err == nil {
				pr.SampleRate = sr
			}
		}
		if s.BitsPerRawSample != "" {
			if bd, err := strconv.Atoi(s.BitsPerRawSample); err == nil {
				pr.BitDepth = bd
			}
		}
		break
	}

	// Parse duration from the format section (more reliable than per-stream).
	if raw.Format.Duration != "" {
		secs, err := strconv.ParseFloat(raw.Format.Duration, 64)
		if err == nil {
			pr.Duration = time.Duration(secs * float64(time.Second))
		}
	}

	// Parse bitrate (bits per second — store as-is).
	if raw.Format.BitRate != "" {
		bps, err := strconv.Atoi(raw.Format.BitRate)
		if err == nil {
			pr.Bitrate = bps
		}
	}

	return pr, nil
}

// mimeForExt returns the MIME type for known audio extensions.
func mimeForExt(ext string) string {
	switch strings.ToLower(ext) {
	case ".mp3":
		return "audio/mpeg"
	case ".wav":
		return "audio/wav"
	case ".flac":
		return "audio/flac"
	case ".aac":
		return "audio/aac"
	case ".ogg":
		return "audio/ogg"
	case ".m4a":
		return "audio/mp4"
	case ".wma":
		return "audio/x-ms-wma"
	default:
		return "application/octet-stream"
	}
}

// formatForExt returns the container format name for known audio extensions.
func formatForExt(ext string) string {
	switch strings.ToLower(ext) {
	case ".mp3":
		return "mp3"
	case ".wav":
		return "wav"
	case ".flac":
		return "flac"
	case ".aac":
		return "aac"
	case ".ogg":
		return "ogg"
	case ".m4a":
		return "m4a"
	case ".wma":
		return "wma"
	default:
		return strings.TrimPrefix(strings.ToLower(ext), ".")
	}
}

// slugify converts a filesystem path into a slug safe for use in entity IDs.
// Slashes become hyphens and the leading slash is stripped.
func slugify(path string) string {
	s := filepath.ToSlash(path)
	s = strings.TrimPrefix(s, "/")
	return strings.ReplaceAll(s, "/", "-")
}
