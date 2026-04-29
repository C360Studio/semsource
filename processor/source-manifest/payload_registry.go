package sourcemanifest

import (
	"errors"

	"github.com/c360studio/semstreams/payloadregistry"
)

// RegisterPayloads registers the source-manifest payload types with the
// supplied registry. Called from cmd/semsource/run.go during bootstrap.
func RegisterPayloads(reg *payloadregistry.Registry) error {
	return errors.Join(
		reg.Register(&payloadregistry.Registration{
			Domain:      "semsource",
			Category:    "manifest",
			Version:     "v1",
			Description: "Source manifest listing all configured ingestion sources",
			Factory:     func() any { return &ManifestPayload{} },
		}),
		reg.Register(&payloadregistry.Registration{
			Domain:      "semsource",
			Category:    "status",
			Version:     "v1",
			Description: "Ingestion status with per-source phase, entity counts, and aggregate lifecycle",
			Factory:     func() any { return &StatusPayload{} },
		}),
		reg.Register(&payloadregistry.Registration{
			Domain:      "semsource",
			Category:    "predicates",
			Version:     "v1",
			Description: "Predicate schema advertising predicates emitted per source type with semantic roles",
			Factory:     func() any { return &PredicateSchemaPayload{} },
		}),
		reg.Register(&payloadregistry.Registration{
			Domain:      "semsource",
			Category:    "ingest.add.request",
			Version:     "v1",
			Description: "Programmatic source-add request (graph.ingest.add.{namespace})",
			Factory:     func() any { return &AddRequest{} },
		}),
		reg.Register(&payloadregistry.Registration{
			Domain:      "semsource",
			Category:    "ingest.add.reply",
			Version:     "v1",
			Description: "Reply to a programmatic source-add request",
			Factory:     func() any { return &AddReply{} },
		}),
		reg.Register(&payloadregistry.Registration{
			Domain:      "semsource",
			Category:    "ingest.remove.request",
			Version:     "v1",
			Description: "Programmatic source-remove request (graph.ingest.remove.{namespace})",
			Factory:     func() any { return &RemoveRequest{} },
		}),
		reg.Register(&payloadregistry.Registration{
			Domain:      "semsource",
			Category:    "ingest.remove.reply",
			Version:     "v1",
			Description: "Reply to a programmatic source-remove request",
			Factory:     func() any { return &RemoveReply{} },
		}),
	)
}
