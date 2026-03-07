package cli

import (
	"errors"

	"github.com/c360studio/semsource/config"
)

func init() {
	RegisterSourceWizard(&videoWizard{})
}

type videoWizard struct{}

func (w *videoWizard) Name() string        { return "Video stream" }
func (w *videoWizard) TypeKey() string     { return "video" }
func (w *videoWizard) Description() string { return "video stream ingestion" }
func (w *videoWizard) Available() (bool, string) { return false, "coming soon" }

func (w *videoWizard) Prompts(_ *Term) (*config.SourceEntry, error) {
	return nil, errors.New("video source type is not yet available")
}
