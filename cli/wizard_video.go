package cli

import (
	"os/exec"

	"github.com/c360studio/semsource/config"
)

func init() {
	RegisterSourceWizard(&videoWizard{})
}

type videoWizard struct{}

func (w *videoWizard) Name() string        { return "Video stream" }
func (w *videoWizard) TypeKey() string     { return "video" }
func (w *videoWizard) Description() string { return "Video files with keyframe extraction" }

func (w *videoWizard) Available() (bool, string) {
	if !ffmpegAvailable() {
		return false, "ffmpeg not found in PATH"
	}
	return true, ""
}

func (w *videoWizard) Prompts(term *Term) (*config.SourceEntry, error) {
	term.Header("Video source")
	term.Info("Enter paths to video files or directories (supported: mp4, mov, mkv, avi, webm).")

	paths := term.MultiLine("Paths")
	if len(paths) == 0 {
		paths = []string{"."}
	}

	// Keyframe extraction mode selection.
	term.Info("  Keyframe extraction mode:")
	term.Info("    1. interval — extract at fixed time intervals (default)")
	term.Info("    2. scene    — extract on scene changes")
	term.Info("    3. iframes  — extract all I-frames")
	modeChoice := term.Prompt("Mode", "1")

	mode := "interval"
	switch modeChoice {
	case "2":
		mode = "scene"
	case "3":
		mode = "iframes"
	}

	var interval string
	var threshold float64
	switch mode {
	case "interval":
		interval = term.Prompt("Keyframe interval", "30s")
	case "scene":
		// Default threshold set programmatically; not prompted to keep UX simple.
		threshold = 0.3
	}

	maxSize := term.Prompt("Max file size", "2GB")
	watch := term.Confirm("Watch for changes?", true)

	entry := &config.SourceEntry{
		Type:             "video",
		Paths:            paths,
		KeyframeMode:     mode,
		KeyframeInterval: interval,
		MaxFileSize:      maxSize,
		Watch:            watch,
	}
	if threshold > 0 {
		entry.SceneThreshold = threshold
	}

	return entry, nil
}

// ffmpegAvailable reports whether ffmpeg is present in PATH.
func ffmpegAvailable() bool {
	_, err := exec.LookPath("ffmpeg")
	return err == nil
}
