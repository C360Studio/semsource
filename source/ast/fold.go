package ast

// FoldDeclarations keeps one entity per ID, for languages where one symbol is
// legitimately declared more than once in a file: a C prototype and its
// definition, the two branches of an #if, a Python name bound twice (#338).
//
// The survivor sits at the position of the ID's first occurrence. A later
// same-ID entity takes the slot only when replaces(candidate, kept) says so;
// that policy is the language's (definition over prototype, implementation over
// stub). The dropped entity's Contains IDs are appended to the survivor's,
// since its children are still emitted and still name the shared ID as parent.
// No other fact is merged: combining calls or doc comments across #if branches
// would describe a symbol no build ever compiles.
//
// Every survivor's Contains is deduplicated, because a file or class lists a
// child ID once per declaration of it.
func FoldDeclarations(entities []*CodeEntity, replaces func(candidate, kept *CodeEntity) bool) []*CodeEntity {
	at := make(map[string]int, len(entities))
	out := entities[:0:0]
	for _, e := range entities {
		i, seen := at[e.ID]
		if !seen {
			at[e.ID] = len(out)
			out = append(out, e)
			continue
		}
		kept := out[i]
		if replaces(e, kept) {
			e.Contains = append(append([]string(nil), kept.Contains...), e.Contains...)
			out[i] = e
		} else {
			kept.Contains = append(kept.Contains, e.Contains...)
		}
	}
	for _, e := range out {
		e.Contains = dedupe(e.Contains)
	}
	return out
}

func dedupe(ids []string) []string {
	if len(ids) < 2 {
		return ids
	}
	seen := make(map[string]bool, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}
