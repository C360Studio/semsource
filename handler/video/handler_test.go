package video_test

import (
	"context"
	"fmt"
	"os/exec"
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
	typ   string
	path  string
	url   string
	watch bool
}

func (s sourceConfig) GetType() string      { return s.typ }
func (s sourceConfig) GetPath() string      { return s.path }
func (s sourceConfig) GetURL() string       { return s.url }
func (s sourceConfig) IsWatchEnabled() bool { return s.watch }

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
		{".MP4", "video/mp4"},   // case-insensitive
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
// Integration — requires ffprobe/ffmpeg in PATH
// ---------------------------------------------------------------------------

func TestVideoHandler_Ingest_RequiresFFmpeg(t *testing.T) {
	if !ffmpegAvailable() {
		t.Skip("ffmpeg/ffprobe not available in PATH")
	}
	// This test is a placeholder for environments where ffmpeg is installed.
	// Full integration tests belong in a file tagged with //go:build integration.
	t.Log("ffmpeg available — integration tests can be added here")
}
