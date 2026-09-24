package models

import (
	"strings"
	"testing"
)

func TestCanonicalTagsNormalizes(t *testing.T) {
	got, err := CanonicalTags([]string{"  Review ", "SECURITY", "review"})
	if err != nil {
		t.Fatalf("canonical tags: %v", err)
	}
	want := []string{"review", "security"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestCanonicalTagsRejectsEmptyValue(t *testing.T) {
	if _, err := CanonicalTags([]string{"ok", "   "}); err == nil {
		t.Fatal("expected error for empty tag")
	}
}

func TestCanonicalTagsRejectsOversizedTag(t *testing.T) {
	if _, err := CanonicalTags([]string{strings.Repeat("a", MaxProfileTagSize+1)}); err == nil {
		t.Fatal("expected error for oversized tag")
	}
}

func TestCanonicalTagsRejectsTooMany(t *testing.T) {
	in := make([]string, MaxProfileTags+1)
	for i := range in {
		in[i] = string(rune('a'+i%26)) + string(rune('0'+i/26))
	}
	if _, err := CanonicalTags(in); err == nil {
		t.Fatal("expected error for too many tags")
	}
}

func TestCanonicalTagsEmptyInputIsNonNil(t *testing.T) {
	got, err := CanonicalTags(nil)
	if err != nil {
		t.Fatalf("canonical tags: %v", err)
	}
	if got == nil || len(got) != 0 {
		t.Fatalf("got %#v, want empty non-nil slice", got)
	}
}
