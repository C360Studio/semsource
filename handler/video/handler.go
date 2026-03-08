// Package video implements the VideoHandler for video file sources.
// It extracts metadata and keyframes from video files using ffprobe and ffmpeg.
package video

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

// sourceTypeKey is the config source type key for video sources.
const sourceTypeKey = "video"

// defaultExtensions lists the file extensions VideoHandler will process.
var defaultExtensions = map[string]bool{
	".mp4":  true,
	".webm": true,
	".mov":  true,
	".avi":  true,
	".mkv":  true,
}

// VideoHandler handles video file sources.
// It implements handler.SourceHandler.
type VideoHandler struct {
	store  storage.Store // nil = no binary storage (metadata only)
	logger *slog.Logger
	// org is the organisation namespace used when building EntityState values
	// via IngestEntityStates and enrichEvent. Empty disables the typed path.
	org string
}

// Option is a functional option for configuring a VideoHandler.
type Option func(*VideoHandler)

// WithStore sets the binary storage backend. When nil (the default), the
// handler records metadata triples only and skips binary storage.
func WithStore(s storage.Store) Option {
	return func(h *VideoHandler) { h.store = s }
}

// WithLogger sets a custom structured logger on the handler.
func WithLogger(l *slog.Logger) Option {
	return func(h *VideoHandler) { h.logger = l }
}

// WithOrg sets the organisation namespace used when building typed EntityState
// values via IngestEntityStates and Watch enrichment.
func WithOrg(org string) Option {
	return func(h *VideoHandler) { h.org = org }
}

// New returns a ready-to-use VideoHandler configured by the provided options.
// Calling New() with no options produces a metadata-only handler using the
// default slog logger.
func New(opts ...Option) *VideoHandler {
	h := &VideoHandler{}
	for _, opt := range opts {
		opt(h)
	}
	if h.logger == nil {
		h.logger = slog.Default()
	}
	return h
}

// SourceType returns the handler type identifier as used in semsource.json.
func (h *VideoHandler) SourceType() string {
	return sourceTypeKey
}

// Supports returns true when cfg describes a "video" source.
func (h *VideoHandler) Supports(cfg handler.SourceConfig) bool {
	return cfg.GetType() == sourceTypeKey
}

// Ingest walks all paths in cfg, reads each supported video file, extracts
// metadata and keyframes, and returns a RawEntity per video plus one per
// keyframe. It respects ctx cancellation.
//
// Path resolution: GetPaths() is used when non-empty; otherwise GetPath() is
// used as a single-element list. An error is returned when both are empty.
func (h *VideoHandler) Ingest(ctx context.Context, cfg handler.SourceConfig) ([]handler.RawEntity, error) {
	roots := resolvePaths(cfg)
	if len(roots) == 0 {
		return nil, fmt.Errorf("video handler: no paths configured (set path or paths in source config)")
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

			videoEntity, keyframeEntities, err := h.ingestFile(ctx, path, root, cfg)
			if err != nil {
				// Non-fatal: skip unreadable or unsupported files and continue.
				h.logger.Warn("video handler: skipping file", "path", path, "error", err)
				return nil
			}
			entities = append(entities, videoEntity)
			entities = append(entities, keyframeEntities...)
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("video handler: walk %q: %w", root, err)
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

// ingestFile streams a single video file to compute its hash, probes metadata,
// extracts keyframes, and returns the video RawEntity plus one RawEntity per keyframe.
func (h *VideoHandler) ingestFile(ctx context.Context, path, root string, cfg handler.SourceConfig) (handler.RawEntity, []handler.RawEntity, error) {
	// Stream hash computation — avoid loading the entire video file into memory.
	f, err := os.Open(path)
	if err != nil {
		return handler.RawEntity{}, nil, fmt.Errorf("open %q: %w", path, err)
	}
	hasher := sha256.New()
	fileSize, err := io.Copy(hasher, f)
	f.Close()
	if err != nil {
		return handler.RawEntity{}, nil, fmt.Errorf("hash %q: %w", path, err)
	}
	hash := hex.EncodeToString(hasher.Sum(nil))
	instance := hash[:6]

	relPath, _ := filepath.Rel(root, path)
	ext := strings.ToLower(filepath.Ext(path))
	mimeType := mimeForExt(ext)
	format := formatForExt(ext)
	system := slugify(root)

	// Extract video metadata via ffprobe. Non-fatal on failure.
	pr, probeErr := probe(ctx, path)
	if probeErr != nil {
		h.logger.Warn("video handler: ffprobe failed, metadata will be partial",
			"path", path, "error", probeErr)
		pr = &ProbeResult{}
	}

	videoEntity := handler.RawEntity{
		SourceType: handler.SourceTypeVideo,
		Domain:     handler.DomainMedia,
		System:     system,
		EntityType: "video",
		Instance:   instance,
		Properties: map[string]any{
			"media_type": "video",
			"file_path":  relPath,
			"mime_type":  mimeType,
			"file_hash":  hash,
			"file_size":  fileSize,
			"format":     format,
			"duration":   pr.Duration.Seconds(),
			"frame_rate": pr.FrameRate,
			"width":      pr.Width,
			"height":     pr.Height,
			"codec":      pr.Codec,
			"bitrate":    pr.Bitrate,
		},
	}

	// Only read full file content when a store is configured for binary persistence.
	if h.store != nil {
		content, err := os.ReadFile(path)
		if err != nil {
			h.logger.Warn("video handler: failed to read file for storage", "path", path, "error", err)
		} else {
			storageKey := fmt.Sprintf("videos/%s/%s/original", system, instance)
			if err := h.store.Put(ctx, storageKey, content); err != nil {
				h.logger.Warn("video handler: failed to store video binary",
					"path", path, "error", err)
			} else {
				videoEntity.Properties["storage_ref"] = storageKey
			}
		}
	}

	// Extract keyframes. Non-fatal: log and continue with zero keyframes.
	mode := keyframeMode(cfg)
	interval := keyframeInterval(cfg)
	threshold := sceneThreshold(cfg)
	keyframes, kfErr := h.extractKeyframes(ctx, path, mode, interval, threshold)
	if kfErr != nil {
		h.logger.Warn("video handler: keyframe extraction failed",
			"path", path, "error", kfErr)
	}

	// Record the keyframe count now that we know it.
	videoEntity.Properties["keyframe_count"] = len(keyframes)

	// Build a RawEntity per keyframe.
	var keyframeEntities []handler.RawEntity
	for _, kf := range keyframes {
		kfInstance := fmt.Sprintf("%s-%s", hash[:6], formatTimestamp(kf.Timestamp))

		kfProps := map[string]any{
			"media_type":  "keyframe",
			"timestamp":   formatTimestamp(kf.Timestamp),
			"frame_index": kf.Index,
			"width":       kf.Width,
			"height":      kf.Height,
		}

		// Store keyframe binary when a store is configured. Non-fatal.
		if h.store != nil && len(kf.Data) > 0 {
			kfKey := fmt.Sprintf("videos/%s/%s/keyframe-%04d", system, instance, kf.Index)
			if err := h.store.Put(ctx, kfKey, kf.Data); err != nil {
				h.logger.Warn("video handler: failed to store keyframe",
					"path", path, "frame", kf.Index, "error", err)
			} else {
				kfProps["storage_ref"] = kfKey
			}
		}

		kfEntity := handler.RawEntity{
			SourceType: handler.SourceTypeVideo,
			Domain:     handler.DomainMedia,
			System:     system,
			EntityType: "keyframe",
			Instance:   kfInstance,
			Properties: kfProps,
			Edges: []handler.RawEdge{
				{FromHint: kfInstance, ToHint: instance, EdgeType: "keyframe_of", ToType: "video"},
			},
		}
		keyframeEntities = append(keyframeEntities, kfEntity)
	}

	return videoEntity, keyframeEntities, nil
}

// ProbeResult holds the metadata extracted from a video file via ffprobe.
// Exported so that export_test.go can surface it to the external test package.
type ProbeResult struct {
	Width     int
	Height    int
	Duration  time.Duration
	Codec     string
	FrameRate float64
	Bitrate   int
}

// ffprobeOutput is the JSON structure returned by ffprobe.
type ffprobeOutput struct {
	Streams []struct {
		CodecName   string `json:"codec_name"`
		Width       int    `json:"width"`
		Height      int    `json:"height"`
		RFrameRate  string `json:"r_frame_rate"`
		CodecType   string `json:"codec_type"`
	} `json:"streams"`
	Format struct {
		Duration string `json:"duration"`
		BitRate  string `json:"bit_rate"`
	} `json:"format"`
}

// probe runs ffprobe on path and returns the extracted metadata.
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
// Exported for testability — tests can call this directly with mock JSON.
func parseProbeOutput(data []byte) (*ProbeResult, error) {
	var raw ffprobeOutput
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse ffprobe output: %w", err)
	}

	pr := &ProbeResult{}

	// Find the first video stream.
	for _, s := range raw.Streams {
		if s.CodecType != "video" {
			continue
		}
		pr.Width = s.Width
		pr.Height = s.Height
		pr.Codec = s.CodecName
		pr.FrameRate = parseFrameRate(s.RFrameRate)
		break
	}

	// Parse duration from the format section (more reliable than per-stream).
	if raw.Format.Duration != "" {
		secs, err := strconv.ParseFloat(raw.Format.Duration, 64)
		if err == nil {
			pr.Duration = time.Duration(secs * float64(time.Second))
		}
	}

	// Parse bitrate (bits per second → store as-is).
	if raw.Format.BitRate != "" {
		bps, err := strconv.Atoi(raw.Format.BitRate)
		if err == nil {
			pr.Bitrate = bps
		}
	}

	return pr, nil
}

// parseFrameRate converts a fractional frame rate string like "30/1" or
// "2997/100" into a float64. Returns 0 on any parse failure.
func parseFrameRate(s string) float64 {
	parts := strings.SplitN(s, "/", 2)
	if len(parts) != 2 {
		return 0
	}
	num, err1 := strconv.ParseFloat(parts[0], 64)
	den, err2 := strconv.ParseFloat(parts[1], 64)
	if err1 != nil || err2 != nil || den == 0 {
		return 0
	}
	return num / den
}

// keyframeResult holds the data for a single extracted keyframe.
type keyframeResult struct {
	Index     int
	Timestamp time.Duration // derived from frame index × interval
	Data      []byte        // JPEG image data
	Width     int
	Height    int
}

// extractKeyframes runs ffmpeg to extract keyframes from path into a temp
// directory, then reads the resulting JPEG files. The mode selects the
// extraction strategy: "interval" (default), "scene", or "iframes".
func (h *VideoHandler) extractKeyframes(ctx context.Context, path, mode, interval string, threshold float64) ([]keyframeResult, error) {
	// ffprobe / ffmpeg must be available; skip gracefully when they are not.
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return nil, fmt.Errorf("ffmpeg not found in PATH: %w", err)
	}

	tmpDir, err := os.MkdirTemp("", "semsource-video-*")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	outPattern := filepath.Join(tmpDir, "frame_%04d.jpg")

	var vf string
	switch mode {
	case "scene":
		// Select frames where the scene change score exceeds the threshold.
		vf = fmt.Sprintf("select='gt(scene,%g)',showinfo", threshold)
	case "iframes":
		// Select only I-frames (intra-coded frames).
		vf = "select='eq(pict_type,I)'"
	default:
		// Interval mode: extract one frame every N seconds (default 30s).
		secs := intervalSeconds(interval)
		vf = fmt.Sprintf("fps=1/%d", secs)
	}

	args := []string{"-i", path, "-vf", vf}

	// Use variable frame rate to avoid duplicate frames for scene/iframe modes.
	if mode == "scene" || mode == "iframes" {
		args = append(args, "-fps_mode", "vfr")
	}

	// Force full-range pixel format for MJPEG compatibility.
	args = append(args, "-pix_fmt", "yuvj420p", "-q:v", "2", outPattern)

	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("ffmpeg extract: %w\noutput: %s", err, string(out))
	}

	// Read the generated JPEG files and build keyframeResult values.
	secs := intervalSeconds(interval)
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		return nil, fmt.Errorf("read temp dir: %w", err)
	}

	var results []keyframeResult
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(strings.ToLower(name), ".jpg") {
			continue
		}

		// Parse the 1-based frame index from the filename (frame_NNNN.jpg).
		idx := parseFrameIndex(name)
		data, readErr := os.ReadFile(filepath.Join(tmpDir, name))
		if readErr != nil {
			h.logger.Warn("video handler: failed to read keyframe file",
				"name", name, "error", readErr)
			continue
		}

		// Derive timestamp from frame index and interval for interval mode.
		// For scene/iframes modes, index-based approximation is acceptable at MVP.
		ts := time.Duration(idx*secs) * time.Second

		results = append(results, keyframeResult{
			Index:     idx,
			Timestamp: ts,
			Data:      data,
		})
	}

	return results, nil
}

// parseFrameIndex extracts the numeric index from a filename like "frame_0003.jpg".
// Returns 0 if the filename does not match the expected pattern.
func parseFrameIndex(name string) int {
	// Strip extension and "frame_" prefix.
	base := strings.TrimSuffix(name, filepath.Ext(name))
	base = strings.TrimPrefix(base, "frame_")
	n, err := strconv.Atoi(base)
	if err != nil {
		return 0
	}
	// ffmpeg frame filenames are 1-based; convert to 0-based.
	if n > 0 {
		return n - 1
	}
	return 0
}

// intervalSeconds parses an interval string like "30s", "1m", "90s" and
// returns the equivalent number of whole seconds. Falls back to 30 on error.
func intervalSeconds(s string) int {
	if s == "" {
		return 30
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 30
	}
	secs := int(d.Seconds())
	if secs <= 0 {
		return 30
	}
	return secs
}

// keyframeMode returns the keyframe extraction mode from the source config.
// Falls back to "interval" when not configured.
func keyframeMode(cfg handler.SourceConfig) string {
	if m := cfg.GetKeyframeMode(); m != "" {
		return m
	}
	return "interval"
}

// keyframeInterval returns the keyframe extraction interval from the source config.
// Falls back to "30s" when not configured.
func keyframeInterval(cfg handler.SourceConfig) string {
	if iv := cfg.GetKeyframeInterval(); iv != "" {
		return iv
	}
	return "30s"
}

// sceneThreshold returns the scene-change sensitivity from the source config.
// Falls back to 0.3 when not configured or when the value is out of the
// valid range (0, 1].
func sceneThreshold(cfg handler.SourceConfig) float64 {
	t := cfg.GetSceneThreshold()
	if t <= 0 || t > 1 {
		return 0.3
	}
	return t
}

// mimeForExt returns the MIME type for known video extensions.
func mimeForExt(ext string) string {
	switch strings.ToLower(ext) {
	case ".mp4":
		return "video/mp4"
	case ".webm":
		return "video/webm"
	case ".mov":
		return "video/quicktime"
	case ".avi":
		return "video/x-msvideo"
	case ".mkv":
		return "video/x-matroska"
	default:
		return "application/octet-stream"
	}
}

// formatForExt returns the container format name for known video extensions.
func formatForExt(ext string) string {
	switch strings.ToLower(ext) {
	case ".mp4":
		return "mp4"
	case ".webm":
		return "webm"
	case ".mov":
		return "mov"
	case ".avi":
		return "avi"
	case ".mkv":
		return "mkv"
	default:
		return strings.TrimPrefix(strings.ToLower(ext), ".")
	}
}

// formatTimestamp converts a duration to a compact human-readable string
// truncated to whole seconds: "0s", "15s", "1m30s", etc.
func formatTimestamp(d time.Duration) string {
	return d.Truncate(time.Second).String()
}

// slugify converts a filesystem path into a slug safe for use in entity IDs.
// Slashes become hyphens and the leading slash is stripped.
func slugify(path string) string {
	s := filepath.ToSlash(path)
	s = strings.TrimPrefix(s, "/")
	return strings.ReplaceAll(s, "/", "-")
}
