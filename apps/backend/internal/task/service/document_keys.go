package service

// WELL_KNOWN_DOCUMENT_KEYS are conventions for task-document names. Any other
// safe key is valid; only plan has special catalog behavior.
var WELL_KNOWN_DOCUMENT_KEYS = []string{"plan", "spec", "spike", "notes", "review", "handoff"}
