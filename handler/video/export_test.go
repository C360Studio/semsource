// export_test.go re-exports unexported package-level helpers for use in the
// external test package (video_test). This file is compiled only during testing.
package video

import "time"

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
