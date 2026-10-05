package service

import "testing"

func TestWellKnownDocumentKeysAreConventions(t *testing.T) {
	want := map[string]bool{
		"plan": true, "spec": true, "spike": true, "notes": true, "review": true, "handoff": true,
	}
	if len(WELL_KNOWN_DOCUMENT_KEYS) != len(want) {
		t.Fatalf("WELL_KNOWN_DOCUMENT_KEYS = %v", WELL_KNOWN_DOCUMENT_KEYS)
	}
	for _, key := range WELL_KNOWN_DOCUMENT_KEYS {
		if !want[key] {
			t.Fatalf("unexpected well-known key %q", key)
		}
	}
}
