package audio_test

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
	audiohandler "github.com/c360studio/semsource/handler/audio"
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
	typ   string
	path  string
	paths []string
	url   string
	watch bool
}

func (s sourceConfig) GetType() string             { return s.typ }
func (s sourceConfig) GetPath() string             { return s.path }
func (s sourceConfig) GetPaths() []string          { return s.paths }
func (s sourceConfig) GetURL() string              { return s.url }
func (s sourceConfig) IsWatchEnabled() bool        { return s.watch }
func (s sourceConfig) GetKeyframeMode() string     { return "" }
func (s sourceConfig) GetKeyframeInterval() string { return "" }
func (s sourceConfig) GetSceneThreshold() float64  { return 0 }

// ffprobeAvailable returns true when ffprobe is in PATH.
func ffprobeAvailable() bool {
	_, err := exec.LookPath("ffprobe")
	return err == nil
}

// ffmpegAvailable returns true when both ffprobe and ffmpeg are in PATH.
func ffmpegAvailable() bool {
	_, errProbe := exec.LookPath("ffprobe")
	_, errFfmpeg := exec.LookPath("ffmpeg")
	return errProbe == nil && errFfmpeg == nil
}

// ---------------------------------------------------------------------------
// SourceType / Supports
// ---------------------------------------------------------------------------

func TestAudioHandler_SourceType(t *testing.T) {
	h := audiohandler.New()
	if got := h.SourceType(); got != "audio" {
		t.Errorf("SourceType() = %q, want %q", got, "audio")
	}
}

func TestAudioHandler_Supports(t *testing.T) {
	h := audiohandler.New()

	tests := []struct {
		typ  string
		want bool
	}{
		{"audio", true},
		{"video", false},
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

func TestAudioHandler_Watch_WatchDisabledReturnsNil(t *testing.T) {
	h := audiohandler.New()
	cfg := sourceConfig{typ: "audio", path: t.TempDir(), watch: false}

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
// mimeForExt — table-driven
// ---------------------------------------------------------------------------

func TestAudioHandler_MimeForExt(t *testing.T) {
	tests := []struct {
		ext  string
		want string
	}{
		{".mp3", "audio/mpeg"},
		{".wav", "audio/wav"},
		{".flac", "audio/flac"},
		{".aac", "audio/aac"},
		{".ogg", "audio/ogg"},
		{".m4a", "audio/mp4"},
		{".wma", "audio/x-ms-wma"},
		{".MP3", "audio/mpeg"},   // case-insensitive
		{".unknown", "application/octet-stream"},
		{"", "application/octet-stream"},
	}

	for _, tt := range tests {
		got := audiohandler.MimeForExt(tt.ext)
		if got != tt.want {
			t.Errorf("MimeForExt(%q) = %q, want %q", tt.ext, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// SlugifyPath
// ---------------------------------------------------------------------------

func TestAudioHandler_SlugifyPath(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"/home/user/audio", "home-user-audio"},
		{"/tmp/semsource", "tmp-semsource"},
		{"relative/path", "relative-path"},
		{"/", ""},
		{"no-slashes", "no-slashes"},
	}

	for _, tt := range tests {
		got := audiohandler.Slugify(tt.input)
		if got != tt.want {
			t.Errorf("Slugify(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// parseProbeOutput — unit tests with mock JSON (no ffprobe required)
// ---------------------------------------------------------------------------

func TestAudioHandler_ParseProbeOutput_AudioStream(t *testing.T) {
	// Realistic ffprobe JSON for a short MP3 file.
	mockJSON := []byte(`{
		"streams": [
			{
				"codec_type": "audio",
				"codec_name": "mp3",
				"sample_rate": "44100",
				"channels": 2,
				"bits_per_raw_sample": "16"
			}
		],
		"format": {
			"duration": "210.5",
			"bit_rate": "128000"
		}
	}`)

	pr, err := audiohandler.ParseProbeOutput(mockJSON)
	if err != nil {
		t.Fatalf("ParseProbeOutput() error: %v", err)
	}

	if pr.Codec != "mp3" {
		t.Errorf("Codec = %q, want %q", pr.Codec, "mp3")
	}
	if pr.SampleRate != 44100 {
		t.Errorf("SampleRate = %d, want 44100", pr.SampleRate)
	}
	if pr.Channels != 2 {
		t.Errorf("Channels = %d, want 2", pr.Channels)
	}
	if pr.BitDepth != 16 {
		t.Errorf("BitDepth = %d, want 16", pr.BitDepth)
	}

	wantDur := time.Duration(210.5 * float64(time.Second))
	if pr.Duration != wantDur {
		t.Errorf("Duration = %v, want %v", pr.Duration, wantDur)
	}
	if pr.Bitrate != 128000 {
		t.Errorf("Bitrate = %d, want 128000", pr.Bitrate)
	}
}

func TestAudioHandler_ParseProbeOutput_NoAudioStream(t *testing.T) {
	// Video-only file — no audio stream should yield zero audio fields.
	mockJSON := []byte(`{
		"streams": [
			{
				"codec_type": "video",
				"codec_name": "h264",
				"width": 1920,
				"height": 1080,
				"r_frame_rate": "30/1"
			}
		],
		"format": {
			"duration": "90.0",
			"bit_rate": "4000000"
		}
	}`)

	pr, err := audiohandler.ParseProbeOutput(mockJSON)
	if err != nil {
		t.Fatalf("ParseProbeOutput() error: %v", err)
	}
	if pr.Codec != "" {
		t.Errorf("expected empty Codec for video-only file, got %q", pr.Codec)
	}
	if pr.SampleRate != 0 {
		t.Errorf("expected zero SampleRate for video-only file, got %d", pr.SampleRate)
	}
	if pr.Channels != 0 {
		t.Errorf("expected zero Channels for video-only file, got %d", pr.Channels)
	}
}

func TestAudioHandler_ParseProbeOutput_InvalidJSON(t *testing.T) {
	_, err := audiohandler.ParseProbeOutput([]byte(`not json`))
	if err == nil {
		t.Error("expected error for invalid JSON, got nil")
	}
}

// ---------------------------------------------------------------------------
// SourceTypeAudio constant
// ---------------------------------------------------------------------------

func TestSourceTypeAudio_Constant(t *testing.T) {
	if handler.SourceTypeAudio != "audio" {
		t.Errorf("SourceTypeAudio = %q, want %q", handler.SourceTypeAudio, "audio")
	}
}

// ---------------------------------------------------------------------------
// resolvePaths — table-driven (no ffprobe required)
// ---------------------------------------------------------------------------

func TestAudioHandler_ResolvePaths(t *testing.T) {
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
			cfg := sourceConfig{typ: "audio", path: tt.path, paths: tt.paths}
			got := audiohandler.ResolvePaths(cfg)

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
// Integration — requires ffprobe/ffmpeg in PATH
// ---------------------------------------------------------------------------

func TestAudioHandler_Ingest_WithFFprobe(t *testing.T) {
	if !ffmpegAvailable() {
		t.Skip("ffmpeg/ffprobe not available in PATH")
	}

	// Generate a minimal 1-second MP3 using ffmpeg.
	dir := t.TempDir()
	audioPath := filepath.Join(dir, "test.mp3")
	cmd := exec.Command("ffmpeg",
		"-f", "lavfi",
		"-i", "sine=frequency=440:duration=1",
		"-c:a", "libmp3lame",
		"-y", audioPath,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to generate test audio: %v\n%s", err, out)
	}

	store := newMemStore()
	h := audiohandler.New(audiohandler.WithStore(store))

	cfg := sourceConfig{
		typ:  "audio",
		path: dir,
	}

	ctx := context.Background()
	entities, err := h.Ingest(ctx, cfg)
	if err != nil {
		t.Fatalf("Ingest() error: %v", err)
	}

	if len(entities) == 0 {
		t.Fatal("Ingest() returned no entities")
	}

	audio := entities[0]
	if audio.EntityType != "audio" {
		t.Errorf("entity type = %q, want %q", audio.EntityType, "audio")
	}
	if audio.SourceType != handler.SourceTypeAudio {
		t.Errorf("SourceType = %q, want %q", audio.SourceType, handler.SourceTypeAudio)
	}

	// Verify codec is set.
	codec, _ := audio.Properties["codec"].(string)
	if codec == "" {
		t.Error("expected codec property to be set")
	}

	// Audio binary should be stored.
	if ref, ok := audio.Properties["storage_ref"].(string); !ok || ref == "" {
		t.Error("expected storage_ref property for audio with store")
	}

	// Verify storage key prefix.
	if ref, ok := audio.Properties["storage_ref"].(string); ok {
		if !strings.HasPrefix(ref, "audio/") {
			t.Errorf("storage_ref %q should start with audio/", ref)
		}
	}
}

// ---------------------------------------------------------------------------
// Ingest multi-path — structural test (no ffmpeg required)
// ---------------------------------------------------------------------------

// TestAudioHandler_Ingest_MultiplePaths verifies that Ingest walks every path
// returned by resolvePaths. Since ffprobe may not be present, we use empty
// directories (no audio files) and non-audio files to confirm:
//   - Both directories are visited without error.
//   - Non-audio files are silently skipped.
//   - Zero entities are returned (correct — no audio files were found).
func TestAudioHandler_Ingest_MultiplePaths(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()

	// Place a non-audio file in each directory — they must be silently skipped.
	if err := os.WriteFile(filepath.Join(dir1, "readme.txt"), []byte("hello"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir2, "config.json"), []byte("{}"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	h := audiohandler.New()
	cfg := sourceConfig{
		typ:   "audio",
		paths: []string{dir1, dir2},
	}

	entities, err := h.Ingest(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Ingest() unexpected error: %v", err)
	}
	if len(entities) != 0 {
		t.Errorf("Ingest() = %d entities, want 0 (no audio files present)", len(entities))
	}
}

// TestAudioHandler_Ingest_NoPathsError verifies that Ingest returns an error
// when neither GetPath nor GetPaths is configured.
func TestAudioHandler_Ingest_NoPathsError(t *testing.T) {
	h := audiohandler.New()
	cfg := sourceConfig{typ: "audio"} // path and paths both zero values

	_, err := h.Ingest(context.Background(), cfg)
	if err == nil {
		t.Fatal("Ingest() expected an error when no paths are configured, got nil")
	}
}

// ---------------------------------------------------------------------------
// Unused import guard for time (used in ParseProbeOutput test)
// ---------------------------------------------------------------------------

var _ = time.Second
