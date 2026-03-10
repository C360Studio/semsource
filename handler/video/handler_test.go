package video_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/c360studio/semsource/handler"
	videohandler "github.com/c360studio/semsource/handler/video"
)

// ---------------------------------------------------------------------------
// Test infrastructure
// ---------------------------------------------------------------------------

// memStore is an in-memory implementation of storage.Store for testing.
type memStore struct {
	mu   sync.Mutex
	data map[string][]byte
}

func newMemStore() *memStore {
	return &memStore{data: make(map[string][]byte)}
}

func (s *memStore) Put(_ context.Context, key string, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = append([]byte(nil), data...)
	return nil
}

func (s *memStore) Get(_ context.Context, key string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.data[key]
	if !ok {
		return nil, fmt.Errorf("key not found: %s", key)
	}
	return d, nil
}

func (s *memStore) List(_ context.Context, prefix string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var keys []string
	for k := range s.data {
		if strings.HasPrefix(k, prefix) {
			keys = append(keys, k)
		}
	}
	return keys, nil
}

func (s *memStore) Delete(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, key)
	return nil
}

// sourceConfig adapts test parameters to handler.SourceConfig.
type sourceConfig struct {
	typ              string
	path             string
	paths            []string
	url              string
	watch            bool
	keyframeMode     string
	keyframeInterval string
	sceneThreshold   float64
}

func (s sourceConfig) GetType() string             { return s.typ }
func (s sourceConfig) GetPath() string             { return s.path }
func (s sourceConfig) GetPaths() []string          { return s.paths }
func (s sourceConfig) GetURL() string              { return s.url }
func (s sourceConfig) GetBranch() string           { return "" }
func (s sourceConfig) IsWatchEnabled() bool        { return s.watch }
func (s sourceConfig) GetKeyframeMode() string     { return s.keyframeMode }
func (s sourceConfig) GetKeyframeInterval() string { return s.keyframeInterval }
func (s sourceConfig) GetSceneThreshold() float64  { return s.sceneThreshold }

// ffmpegAvailable returns true when both ffprobe and ffmpeg are in PATH.
func ffmpegAvailable() bool {
	_, errProbe := exec.LookPath("ffprobe")
	_, errFfmpeg := exec.LookPath("ffmpeg")
	return errProbe == nil && errFfmpeg == nil
}

// ---------------------------------------------------------------------------
// SourceType / Supports
// ---------------------------------------------------------------------------

func TestVideoHandler_SourceType(t *testing.T) {
	h := videohandler.New()
	if got := h.SourceType(); got != "video" {
		t.Errorf("SourceType() = %q, want %q", got, "video")
	}
}

func TestVideoHandler_Supports(t *testing.T) {
	h := videohandler.New()

	tests := []struct {
		typ  string
		want bool
	}{
		{"video", true},
		{"image", false},
		{"doc", false},
		{"git", false},
		{"", false},
	}
	for _, tt := range tests {
		cfg := sourceConfig{typ: tt.typ}
		if got := h.Supports(cfg); got != tt.want {
			t.Errorf("Supports(%q) = %v, want %v", tt.typ, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// Watch — disabled returns nil
// ---------------------------------------------------------------------------

func TestVideoHandler_Watch_WatchDisabledReturnsNil(t *testing.T) {
	h := videohandler.New()
	cfg := sourceConfig{typ: "video", path: t.TempDir(), watch: false}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, err := h.Watch(ctx, cfg)
	if err != nil {
		t.Fatalf("Watch() error: %v", err)
	}
	if ch != nil {
		t.Error("Watch() should return nil channel when watch is disabled")
	}
}

// ---------------------------------------------------------------------------
// formatTimestamp — table-driven
// ---------------------------------------------------------------------------

func TestVideoHandler_FormatTimestamp(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{0, "0s"},
		{15 * time.Second, "15s"},
		{90 * time.Second, "1m30s"},
		{3600 * time.Second, "1h0m0s"},
		// Sub-second durations are truncated to whole seconds.
		{500 * time.Millisecond, "0s"},
		{1*time.Second + 999*time.Millisecond, "1s"},
	}

	for _, tt := range tests {
		got := videohandler.FormatTimestamp(tt.d)
		if got != tt.want {
			t.Errorf("FormatTimestamp(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// mimeForExt — table-driven
// ---------------------------------------------------------------------------

func TestVideoHandler_MimeForExt(t *testing.T) {
	tests := []struct {
		ext  string
		want string
	}{
		{".mp4", "video/mp4"},
		{".webm", "video/webm"},
		{".mov", "video/quicktime"},
		{".avi", "video/x-msvideo"},
		{".mkv", "video/x-matroska"},
		{".MP4", "video/mp4"}, // case-insensitive
		{".unknown", "application/octet-stream"},
		{"", "application/octet-stream"},
	}

	for _, tt := range tests {
		got := videohandler.MimeForExt(tt.ext)
		if got != tt.want {
			t.Errorf("MimeForExt(%q) = %q, want %q", tt.ext, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// SlugifyPath
// ---------------------------------------------------------------------------

func TestVideoHandler_SlugifyPath(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"/home/user/videos", "home-user-videos"},
		{"/tmp/semsource", "tmp-semsource"},
		{"relative/path", "relative-path"},
		{"/", ""},
		{"no-slashes", "no-slashes"},
	}

	for _, tt := range tests {
		got := videohandler.Slugify(tt.input)
		if got != tt.want {
			t.Errorf("Slugify(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// parseProbeOutput — unit test with mock JSON (no ffprobe required)
// ---------------------------------------------------------------------------

func TestVideoHandler_ParseProbeOutput_VideoStream(t *testing.T) {
	// Realistic ffprobe JSON for a short H.264 MP4.
	mockJSON := []byte(`{
		"streams": [
			{
				"codec_type": "video",
				"codec_name": "h264",
				"width": 1920,
				"height": 1080,
				"r_frame_rate": "30/1"
			},
			{
				"codec_type": "audio",
				"codec_name": "aac"
			}
		],
		"format": {
			"duration": "90.5",
			"bit_rate": "4000000"
		}
	}`)

	pr, err := videohandler.ParseProbeOutput(mockJSON)
	if err != nil {
		t.Fatalf("ParseProbeOutput() error: %v", err)
	}

	if pr.Width != 1920 {
		t.Errorf("Width = %d, want 1920", pr.Width)
	}
	if pr.Height != 1080 {
		t.Errorf("Height = %d, want 1080", pr.Height)
	}
	if pr.Codec != "h264" {
		t.Errorf("Codec = %q, want %q", pr.Codec, "h264")
	}
	if pr.FrameRate != 30.0 {
		t.Errorf("FrameRate = %f, want 30.0", pr.FrameRate)
	}
	// Duration: 90.5 seconds.
	wantDur := time.Duration(90.5 * float64(time.Second))
	if pr.Duration != wantDur {
		t.Errorf("Duration = %v, want %v", pr.Duration, wantDur)
	}
	if pr.Bitrate != 4_000_000 {
		t.Errorf("Bitrate = %d, want 4000000", pr.Bitrate)
	}
}

func TestVideoHandler_ParseProbeOutput_FractionalFrameRate(t *testing.T) {
	mockJSON := []byte(`{
		"streams": [
			{
				"codec_type": "video",
				"codec_name": "h265",
				"width": 3840,
				"height": 2160,
				"r_frame_rate": "2997/100"
			}
		],
		"format": {
			"duration": "10.0",
			"bit_rate": "20000000"
		}
	}`)

	pr, err := videohandler.ParseProbeOutput(mockJSON)
	if err != nil {
		t.Fatalf("ParseProbeOutput() error: %v", err)
	}

	const wantFPS = 29.97
	const tolerance = 0.01
	if pr.FrameRate < wantFPS-tolerance || pr.FrameRate > wantFPS+tolerance {
		t.Errorf("FrameRate = %f, want ~%f", pr.FrameRate, wantFPS)
	}
}

func TestVideoHandler_ParseProbeOutput_NoVideoStream(t *testing.T) {
	// Audio-only file — no video stream should yield zero dimensions.
	mockJSON := []byte(`{
		"streams": [
			{
				"codec_type": "audio",
				"codec_name": "mp3"
			}
		],
		"format": {
			"duration": "210.0",
			"bit_rate": "128000"
		}
	}`)

	pr, err := videohandler.ParseProbeOutput(mockJSON)
	if err != nil {
		t.Fatalf("ParseProbeOutput() error: %v", err)
	}
	if pr.Width != 0 || pr.Height != 0 {
		t.Errorf("expected zero dimensions for audio-only, got %dx%d", pr.Width, pr.Height)
	}
	if pr.Codec != "" {
		t.Errorf("expected empty Codec for audio-only, got %q", pr.Codec)
	}
}

func TestVideoHandler_ParseProbeOutput_InvalidJSON(t *testing.T) {
	_, err := videohandler.ParseProbeOutput([]byte(`not json`))
	if err == nil {
		t.Error("expected error for invalid JSON, got nil")
	}
}

// ---------------------------------------------------------------------------
// SourceTypeVideo constant
// ---------------------------------------------------------------------------

func TestSourceTypeVideo_Constant(t *testing.T) {
	if handler.SourceTypeVideo != "video" {
		t.Errorf("SourceTypeVideo = %q, want %q", handler.SourceTypeVideo, "video")
	}
}

// ---------------------------------------------------------------------------
// Config passthrough — keyframeMode, keyframeInterval, sceneThreshold
// ---------------------------------------------------------------------------

func TestVideoHandler_KeyframeMode_FromConfig(t *testing.T) {
	tests := []struct {
		mode string
		want string
	}{
		{"", "interval"}, // default
		{"interval", "interval"},
		{"scene", "scene"},
		{"iframes", "iframes"},
	}
	for _, tt := range tests {
		cfg := sourceConfig{typ: "video", keyframeMode: tt.mode}
		got := videohandler.KeyframeMode(cfg)
		if got != tt.want {
			t.Errorf("KeyframeMode(%q) = %q, want %q", tt.mode, got, tt.want)
		}
	}
}

func TestVideoHandler_KeyframeInterval_FromConfig(t *testing.T) {
	tests := []struct {
		interval string
		want     string
	}{
		{"", "30s"}, // default
		{"10s", "10s"},
		{"1m", "1m"},
		{"90s", "90s"},
	}
	for _, tt := range tests {
		cfg := sourceConfig{typ: "video", keyframeInterval: tt.interval}
		got := videohandler.KeyframeInterval(cfg)
		if got != tt.want {
			t.Errorf("KeyframeInterval(%q) = %q, want %q", tt.interval, got, tt.want)
		}
	}
}

func TestVideoHandler_SceneThreshold_FromConfig(t *testing.T) {
	tests := []struct {
		threshold float64
		want      float64
	}{
		{0, 0.3}, // default
		{0.5, 0.5},
		{0.1, 0.1},
		{0.9, 0.9},
	}
	for _, tt := range tests {
		cfg := sourceConfig{typ: "video", sceneThreshold: tt.threshold}
		got := videohandler.SceneThreshold(cfg)
		if got != tt.want {
			t.Errorf("SceneThreshold(%v) = %v, want %v", tt.threshold, got, tt.want)
		}
	}
}

func TestVideoHandler_IntervalSeconds(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"", 30}, // default
		{"10s", 10},
		{"1m", 60},
		{"90s", 90},
		{"invalid", 30}, // fallback
		{"-5s", 30},     // non-positive fallback
	}
	for _, tt := range tests {
		got := videohandler.IntervalSeconds(tt.input)
		if got != tt.want {
			t.Errorf("IntervalSeconds(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// Integration — requires ffprobe/ffmpeg in PATH
// ---------------------------------------------------------------------------

func TestVideoHandler_Ingest_WithFFmpeg(t *testing.T) {
	if !ffmpegAvailable() {
		t.Skip("ffmpeg/ffprobe not available in PATH")
	}

	// Generate a minimal 1-second test video using ffmpeg.
	dir := t.TempDir()
	videoPath := filepath.Join(dir, "test.mp4")
	cmd := exec.Command("ffmpeg",
		"-f", "lavfi", "-i", "color=c=blue:s=320x240:d=1",
		"-f", "lavfi", "-i", "anullsrc=r=44100:cl=mono",
		"-shortest",
		"-c:v", "libx264", "-preset", "ultrafast",
		"-c:a", "aac",
		"-y", videoPath,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to generate test video: %v\n%s", err, out)
	}

	store := newMemStore()
	h := videohandler.New(videohandler.WithStore(store))

	cfg := sourceConfig{
		typ:              "video",
		path:             dir,
		keyframeMode:     "interval",
		keyframeInterval: "1s",
	}

	ctx := context.Background()
	entities, err := h.Ingest(ctx, cfg)
	if err != nil {
		t.Fatalf("Ingest() error: %v", err)
	}

	if len(entities) == 0 {
		t.Fatal("Ingest() returned no entities")
	}

	// First entity should be the video itself.
	video := entities[0]
	if video.EntityType != "video" {
		t.Errorf("first entity type = %q, want %q", video.EntityType, "video")
	}

	// Verify metadata properties.
	if w, ok := video.Properties["width"].(int); !ok || w != 320 {
		t.Errorf("width = %v, want 320", video.Properties["width"])
	}
	if h, ok := video.Properties["height"].(int); !ok || h != 240 {
		t.Errorf("height = %v, want 240", video.Properties["height"])
	}
	if codec, ok := video.Properties["codec"].(string); !ok || codec != "h264" {
		t.Errorf("codec = %v, want h264", video.Properties["codec"])
	}

	// Video binary should be stored.
	if ref, ok := video.Properties["storage_ref"].(string); !ok || ref == "" {
		t.Error("expected storage_ref property for video with store")
	}

	// Should have at least one keyframe entity (1s video with 1s interval = 1 frame).
	var keyframes int
	for _, e := range entities {
		if e.EntityType == "keyframe" {
			keyframes++
			// Keyframe should have keyframe_of edge.
			if len(e.Edges) == 0 {
				t.Error("keyframe entity has no edges")
			} else if e.Edges[0].EdgeType != "keyframe_of" {
				t.Errorf("keyframe edge type = %q, want %q", e.Edges[0].EdgeType, "keyframe_of")
			}
		}
	}
	if keyframes == 0 {
		t.Error("expected at least one keyframe entity")
	}
}

func TestVideoHandler_Ingest_SceneMode(t *testing.T) {
	if !ffmpegAvailable() {
		t.Skip("ffmpeg/ffprobe not available in PATH")
	}

	// Generate a 2-second video with a color change at 1s for scene detection.
	dir := t.TempDir()
	videoPath := filepath.Join(dir, "scene.mp4")

	// Create two 1s clips of different colors, then concatenate.
	clip1 := filepath.Join(dir, "clip1.mp4")
	clip2 := filepath.Join(dir, "clip2.mp4")

	cmd1 := exec.Command("ffmpeg",
		"-f", "lavfi", "-i", "color=c=red:s=160x120:d=1",
		"-c:v", "libx264", "-preset", "ultrafast",
		"-y", clip1,
	)
	if out, err := cmd1.CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg clip1: %v\n%s", err, out)
	}

	cmd2 := exec.Command("ffmpeg",
		"-f", "lavfi", "-i", "color=c=green:s=160x120:d=1",
		"-c:v", "libx264", "-preset", "ultrafast",
		"-y", clip2,
	)
	if out, err := cmd2.CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg clip2: %v\n%s", err, out)
	}

	// Concatenate using the concat demuxer.
	concatFile := filepath.Join(dir, "concat.txt")
	concatContent := fmt.Sprintf("file '%s'\nfile '%s'\n", clip1, clip2)
	if err := os.WriteFile(concatFile, []byte(concatContent), 0644); err != nil {
		t.Fatalf("write concat file: %v", err)
	}

	cmdConcat := exec.Command("ffmpeg",
		"-f", "concat", "-safe", "0", "-i", concatFile,
		"-c:v", "libx264", "-preset", "ultrafast",
		"-y", videoPath,
	)
	if out, err := cmdConcat.CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg concat: %v\n%s", err, out)
	}

	h := videohandler.New()
	cfg := sourceConfig{
		typ:            "video",
		path:           dir,
		keyframeMode:   "scene",
		sceneThreshold: 0.2,
	}

	ctx := context.Background()
	entities, err := h.Ingest(ctx, cfg)
	if err != nil {
		t.Fatalf("Ingest() error: %v", err)
	}

	// We should get the video entity plus any detected scene keyframes.
	if len(entities) == 0 {
		t.Fatal("Ingest() returned no entities")
	}

	// The concat video should produce at least one keyframe at the scene change.
	video := entities[0]
	if video.EntityType != "video" {
		t.Errorf("first entity type = %q, want %q", video.EntityType, "video")
	}

	t.Logf("scene mode: got %d total entities (1 video + %d keyframes)", len(entities), len(entities)-1)
}

// ---------------------------------------------------------------------------
// resolvePaths — table-driven (no ffmpeg required)
// ---------------------------------------------------------------------------

func TestVideoHandler_ResolvePaths(t *testing.T) {
	tests := []struct {
		name  string
		path  string
		paths []string
		want  []string
	}{
		{
			name:  "GetPaths non-empty takes priority",
			path:  "/single",
			paths: []string{"/a", "/b"},
			want:  []string{"/a", "/b"},
		},
		{
			name:  "falls back to GetPath when GetPaths is nil",
			path:  "/single",
			paths: nil,
			want:  []string{"/single"},
		},
		{
			name:  "falls back to GetPath when GetPaths is empty slice",
			path:  "/only",
			paths: []string{},
			want:  []string{"/only"},
		},
		{
			name:  "returns nil when both are empty",
			path:  "",
			paths: nil,
			want:  nil,
		},
		{
			name:  "three-path config",
			path:  "",
			paths: []string{"/p1", "/p2", "/p3"},
			want:  []string{"/p1", "/p2", "/p3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := sourceConfig{typ: "video", path: tt.path, paths: tt.paths}
			got := videohandler.ResolvePaths(cfg)

			if len(got) != len(tt.want) {
				t.Fatalf("ResolvePaths() = %v, want %v", got, tt.want)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("ResolvePaths()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Ingest multi-path — structural test (no ffmpeg required)
// ---------------------------------------------------------------------------

// TestVideoHandler_Ingest_MultiplePaths verifies that Ingest walks every path
// returned by resolvePaths. Since ffmpeg may not be present, we use empty
// directories (no video files) and non-video files to confirm:
//   - Both directories are visited without error.
//   - Non-video files are silently skipped.
//   - Zero entities are returned (correct — no video files were found).
func TestVideoHandler_Ingest_MultiplePaths(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()

	// Place a non-video file in each directory — they must be silently skipped.
	if err := os.WriteFile(filepath.Join(dir1, "readme.txt"), []byte("hello"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir2, "config.json"), []byte("{}"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	h := videohandler.New()
	cfg := sourceConfig{
		typ:   "video",
		paths: []string{dir1, dir2},
	}

	entities, err := h.Ingest(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Ingest() unexpected error: %v", err)
	}
	if len(entities) != 0 {
		t.Errorf("Ingest() = %d entities, want 0 (no video files present)", len(entities))
	}
}

// ---------------------------------------------------------------------------
// IngestEntityStates — normalizer-free typed entity production
// ---------------------------------------------------------------------------

func TestVideoHandler_IngestEntityStates_EmptyDirReturnsEmpty(t *testing.T) {
	dir := t.TempDir()

	h := videohandler.New()
	cfg := sourceConfig{typ: "video", path: dir}

	states, err := h.IngestEntityStates(context.Background(), cfg, "acme")
	if err != nil {
		t.Fatalf("IngestEntityStates() error: %v", err)
	}
	if len(states) != 0 {
		t.Errorf("entity state count: got %d, want 0 for empty dir", len(states))
	}
}

func TestVideoHandler_IngestEntityStates_WithFFmpeg(t *testing.T) {
	if !ffmpegAvailable() {
		t.Skip("ffmpeg/ffprobe not available in PATH")
	}

	dir := t.TempDir()
	videoPath := filepath.Join(dir, "test.mp4")
	cmd := exec.Command("ffmpeg",
		"-f", "lavfi", "-i", "color=c=red:s=160x120:d=1",
		"-c:v", "libx264", "-preset", "ultrafast",
		"-y", videoPath,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg: %v\n%s", err, out)
	}

	h := videohandler.New()
	cfg := sourceConfig{
		typ:              "video",
		path:             dir,
		keyframeMode:     "interval",
		keyframeInterval: "1s",
	}

	states, err := h.IngestEntityStates(context.Background(), cfg, "acme")
	if err != nil {
		t.Fatalf("IngestEntityStates() error: %v", err)
	}
	if len(states) == 0 {
		t.Fatal("IngestEntityStates() returned no states")
	}

	// First state must be the video entity with a 6-part ID.
	video := states[0]
	parts := strings.Split(video.ID, ".")
	if len(parts) != 6 {
		t.Errorf("video ID %q has %d parts, want 6", video.ID, len(parts))
	}
	if parts[0] != "acme" {
		t.Errorf("video ID org segment = %q, want %q", parts[0], "acme")
	}

	// Verify the video entity carries the required vocabulary predicates.
	predicates := make(map[string]bool)
	for _, tr := range video.Triples {
		predicates[tr.Predicate] = true
	}
	for _, p := range []string{
		"source.media.type",
		"source.media.file_path",
		"source.media.mime_type",
		"source.media.file_hash",
		"source.media.format",
		"source.media.duration",
		"source.media.frame_rate",
		"source.media.codec",
	} {
		if !predicates[p] {
			t.Errorf("missing required predicate %q in video triples", p)
		}
	}

	// Verify the media.type triple is "video".
	for _, tr := range video.Triples {
		if tr.Predicate == "source.media.type" && tr.Object != "video" {
			t.Errorf("video media.type = %v, want %q", tr.Object, "video")
		}
	}

	// Keyframe entities (if any) must have a keyframe_of relationship triple
	// pointing back to the video entity ID.
	for _, state := range states[1:] {
		var hasKeyframeOf bool
		for _, tr := range state.Triples {
			if tr.Predicate == "source.media.keyframe_of" {
				hasKeyframeOf = true
				if tr.Object != video.ID {
					t.Errorf("keyframe_of Object = %q, want video ID %q", tr.Object, video.ID)
				}
			}
		}
		if !hasKeyframeOf {
			t.Errorf("keyframe entity %q missing source.media.keyframe_of triple", state.ID)
		}
	}
}

// TestVideoHandler_Ingest_NoPathsError verifies that Ingest returns an error
// when neither GetPath nor GetPaths is configured.
func TestVideoHandler_Ingest_NoPathsError(t *testing.T) {
	h := videohandler.New()
	cfg := sourceConfig{typ: "video"} // path and paths both zero values

	_, err := h.Ingest(context.Background(), cfg)
	if err == nil {
		t.Fatal("Ingest() expected an error when no paths are configured, got nil")
	}
}

func TestVideoHandler_Ingest_MetadataOnlyWithoutStore(t *testing.T) {
	if !ffmpegAvailable() {
		t.Skip("ffmpeg/ffprobe not available in PATH")
	}

	dir := t.TempDir()
	videoPath := filepath.Join(dir, "meta.mp4")
	cmd := exec.Command("ffmpeg",
		"-f", "lavfi", "-i", "color=c=black:s=160x120:d=1",
		"-c:v", "libx264", "-preset", "ultrafast",
		"-y", videoPath,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg: %v\n%s", err, out)
	}

	// No store — metadata-only mode.
	h := videohandler.New()
	cfg := sourceConfig{typ: "video", path: dir}

	entities, err := h.Ingest(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Ingest() error: %v", err)
	}

	if len(entities) == 0 {
		t.Fatal("Ingest() returned no entities")
	}

	video := entities[0]
	// Should NOT have storage_ref without a store.
	if _, ok := video.Properties["storage_ref"]; ok {
		t.Error("unexpected storage_ref property without store")
	}

	// Should still have metadata.
	if video.Properties["codec"] == "" {
		t.Error("expected codec metadata even without store")
	}
}
