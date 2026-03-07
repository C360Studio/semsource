// export_test.go re-exports unexported package-level helpers for use in the
// external test package (audio_test). This file is compiled only during testing.
package audio

import "github.com/c360studio/semsource/handler"

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

// ResolvePaths exposes resolvePaths for table-driven unit tests.
func ResolvePaths(cfg handler.SourceConfig) []string {
	return resolvePaths(cfg)
}
