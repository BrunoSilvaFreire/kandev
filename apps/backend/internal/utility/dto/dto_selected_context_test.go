package dto

import (
	"strings"
	"testing"
)

func TestValidateSelectedContext(t *testing.T) {
	valid := []SelectedContextItem{{Kind: "document", Label: "Spec", Text: "details"}}
	if err := ValidateSelectedContext(valid); err != nil {
		t.Fatalf("ValidateSelectedContext(valid): %v", err)
	}
	if err := ValidateSelectedContext(make([]SelectedContextItem, maxSelectedContextItems+1)); err == nil {
		t.Fatal("expected item-limit error")
	}
	if err := ValidateSelectedContext([]SelectedContextItem{{Kind: "file", Label: "x", Text: "x"}}); err == nil {
		t.Fatal("expected kind error")
	}
	if err := ValidateSelectedContext([]SelectedContextItem{{Kind: "session", Label: "x", Text: strings.Repeat("x", maxSelectedContextBytes+1)}}); err == nil {
		t.Fatal("expected byte-limit error")
	}
}
