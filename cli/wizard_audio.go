package cli

import (
	"github.com/c360studio/semsource/config"
)

func init() {
	RegisterSourceWizard(&audioWizard{})
}

type audioWizard struct{}

func (w *audioWizard) Name() string        { return "Audio files" }
func (w *audioWizard) TypeKey() string     { return "audio" }
func (w *audioWizard) Description() string { return "mp3, wav, flac, aac, ogg, m4a, wma" }

func (w *audioWizard) Available() (bool, string) {
	if !ffmpegAvailable() {
		return false, "ffmpeg not found in PATH"
	}
	return true, ""
}

func (w *audioWizard) Prompts(term *Term) (*config.SourceEntry, error) {
	term.Header("Audio source")
	term.Info("Enter paths to audio files or directories (supported: mp3, wav, flac, aac, ogg, m4a, wma).")

	paths := term.MultiLine("Paths")
	if len(paths) == 0 {
		paths = []string{"."}
	}

	maxSize := term.Prompt("Max file size", "500MB")
	watch := term.Confirm("Watch for changes?", true)

	return &config.SourceEntry{
		Type:        "audio",
		Paths:       paths,
		MaxFileSize: maxSize,
		Watch:       watch,
	}, nil
}
