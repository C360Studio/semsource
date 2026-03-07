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
	var available, unavailable []SourceWizard
	for _, w := range Wizards() {
		ok, _ := w.Available()
		if ok {
			available = append(available, w)
		} else {
			unavailable = append(unavailable, w)
		}
	}

	if len(available) == 0 {
		t.Fatal("expected at least one available wizard")
	}
	// Video should be unavailable.
	foundVideo := false
	for _, w := range unavailable {
		if w.TypeKey() == "video" {
			foundVideo = true
		}
	}
	if !foundVideo {
		t.Error("expected video wizard to be unavailable")
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
