package cli

import (
	"os"

	"github.com/c360studio/semsource/config"
)

func init() {
	RegisterSourceWizard(&imageWizard{})
}

type imageWizard struct{}

func (w *imageWizard) Name() string              { return "Images" }
func (w *imageWizard) TypeKey() string           { return "image" }
func (w *imageWizard) Description() string       { return "screenshots, diagrams, visual assets" }
func (w *imageWizard) Available() (bool, string) { return true, "" }

func (w *imageWizard) Prompts(term *Term) (*config.SourceEntry, error) {
	term.Header("Image source")
	term.Info("Enter paths to image files or directories (supported: png, jpg, gif, webp, svg).")

	// Show detected image directories as hints.
	defaults := detectImagePaths()
	if len(defaults) > 0 {
		term.Info("  (detected: " + joinPaths(defaults) + ")")
		term.Info("  Press Enter on empty line to accept detected paths, or enter your own.")
	}

	paths := term.MultiLine("Paths")
	if len(paths) == 0 && len(defaults) > 0 {
		paths = defaults
	}
	for len(paths) == 0 {
		term.Info("  At least one path is required.")
		paths = term.MultiLine("Paths")
	}

	maxFileSize := term.Prompt("Max file size", "50MB")
	generateThumbnails := term.Confirm("Generate thumbnails?", true)

	return &config.SourceEntry{
		Type:               "image",
		Paths:              paths,
		Watch:              true,
		MaxFileSize:        maxFileSize,
		GenerateThumbnails: boolPtr(generateThumbnails),
	}, nil
}

// detectImagePaths checks for common image directories in the working directory.
func detectImagePaths() []string {
	candidates := []string{
		"assets/",
		"images/",
		"static/images/",
		"docs/images/",
		"screenshots/",
	}
	var found []string
	for _, d := range candidates {
		// Trim trailing slash for Stat, restore for display.
		name := d
		if len(name) > 0 && name[len(name)-1] == '/' {
			name = name[:len(name)-1]
		}
		if info, err := os.Stat(name); err == nil && info.IsDir() {
			found = append(found, d)
		}
	}
	return found
}

// boolPtr returns a pointer to the given bool value.
func boolPtr(b bool) *bool {
	return &b
}
