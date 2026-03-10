package cli

import "github.com/c360studio/semsource/config"

func init() {
	RegisterSourceWizard(&urlWizard{})
}

type urlWizard struct{}

func (w *urlWizard) Name() string              { return "URLs" }
func (w *urlWizard) TypeKey() string           { return "url" }
func (w *urlWizard) Description() string       { return "web pages, API docs" }
func (w *urlWizard) Available() (bool, string) { return true, "" }

func (w *urlWizard) Prompts(term *Term) (*config.SourceEntry, error) {
	term.Header("URL source")
	term.Info("Enter HTTP/S URLs to ingest (e.g. https://example.com/api-docs).")

	var urls []string
	for len(urls) == 0 {
		urls = term.MultiLine("URLs")
		if len(urls) == 0 {
			term.Info("  At least one URL is required.")
		}
	}
	pollInterval := term.Prompt("Poll interval", "5m")

	return &config.SourceEntry{
		Type:         "url",
		URLs:         urls,
		PollInterval: pollInterval,
	}, nil
}
