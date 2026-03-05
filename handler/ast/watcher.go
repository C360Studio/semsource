package ast

import (
	"time"

	semspecast "github.com/c360studio/semspec/processor/ast"
	"github.com/c360studio/semsource/handler"
)

// translateWatchEvent converts a semspec WatchEvent into a handler.ChangeEvent.
func translateWatchEvent(ev semspecast.WatchEvent, lang, system string) handler.ChangeEvent {
	ce := handler.ChangeEvent{
		Path:      ev.Path,
		Timestamp: time.Now(),
	}

	switch ev.Operation {
	case semspecast.OpCreate:
		ce.Operation = handler.OperationCreate
	case semspecast.OpModify:
		ce.Operation = handler.OperationModify
	case semspecast.OpDelete:
		ce.Operation = handler.OperationDelete
		// Delete events carry no entities — the engine uses the Path to issue RETRACT.
		return ce
	default:
		ce.Operation = handler.OperationModify
	}

	if ev.Result != nil {
		domain := langToDomain(lang)
		ce.Entities = mapParseResult(ev.Result, lang, system)
		// Backfill domain on entities in case mapParseResult used a default.
		for i := range ce.Entities {
			if ce.Entities[i].Domain == "" {
				ce.Entities[i].Domain = domain
			}
		}
	}

	return ce
}
