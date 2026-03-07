package cli

import (
	"testing"
)

func TestRegistryContainsExpectedWizards(t *testing.T) {
	wizards := Wizards()
	if len(wizards) == 0 {
		t.Fatal("expected at least one registered wizard")
	}

	// Verify well-known types are registered.
	wantTypes := map[string]bool{
		"ast":    false,
		"git":    false,
		"docs":   false,
		"config": false,
		"url":    false,
		"image":  false,
		"video":  false,
		"audio":  false,
	}
	for _, w := range wizards {
		wantTypes[w.TypeKey()] = true
	}
	for typ, found := range wantTypes {
		if !found {
			t.Errorf("expected wizard for type %q to be registered", typ)
		}
	}
}

func TestAvailableFiltering(t *testing.T) {
	var available int
	for _, w := range Wizards() {
		ok, reason := w.Available()
		if ok {
			available++
		} else {
			// Unavailable wizards must provide a reason.
			if reason == "" {
				t.Errorf("wizard %q: Available() returned false with empty reason", w.TypeKey())
			}
		}
	}

	if available == 0 {
		t.Fatal("expected at least one available wizard")
	}
}

func TestWizardMetadata(t *testing.T) {
	for _, w := range Wizards() {
		if w.Name() == "" {
			t.Errorf("wizard %T: Name() must not be empty", w)
		}
		if w.TypeKey() == "" {
			t.Errorf("wizard %T: TypeKey() must not be empty", w)
		}
		if w.Description() == "" {
			t.Errorf("wizard %T: Description() must not be empty", w)
		}
	}
}
