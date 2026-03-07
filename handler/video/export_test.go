// export_test.go re-exports unexported package-level helpers for use in the
// external test package (video_test). This file is compiled only during testing.
package video

import (
	"time"

	"github.com/c360studio/semsource/handler"
)

// FormatTimestamp exposes formatTimestamp for table-driven unit tests.
func FormatTimestamp(d time.Duration) string {
	return formatTimestamp(d)
}

// MimeForExt exposes mimeForExt for table-driven unit tests.
func MimeForExt(ext string) string {
	return mimeForExt(ext)
}

// Slugify exposes slugify for unit tests.
func Slugify(path string) string {
	return slugify(path)
}

// ParseProbeOutput exposes parseProbeOutput so tests can feed mock ffprobe
// JSON without needing ffprobe installed.
func ParseProbeOutput(data []byte) (*ProbeResult, error) {
	return parseProbeOutput(data)
}

// KeyframeMode exposes keyframeMode for testing config passthrough.
func KeyframeMode(cfg handler.SourceConfig) string {
	return keyframeMode(cfg)
}

// KeyframeInterval exposes keyframeInterval for testing config passthrough.
func KeyframeInterval(cfg handler.SourceConfig) string {
	return keyframeInterval(cfg)
}

// SceneThreshold exposes sceneThreshold for testing config passthrough.
func SceneThreshold(cfg handler.SourceConfig) float64 {
	return sceneThreshold(cfg)
}

// IntervalSeconds exposes intervalSeconds for testing.
func IntervalSeconds(s string) int {
	return intervalSeconds(s)
}
