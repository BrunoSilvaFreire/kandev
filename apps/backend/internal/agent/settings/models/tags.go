package models

import "github.com/kandev/kandev/internal/common/tags"

// Profile tag bounds, re-exported from the shared canonicalizer so existing
// callers keep their package-local names.
const (
	MaxProfileTags    = tags.MaxCount
	MaxProfileTagSize = tags.MaxTagSize
)

// CanonicalTags trims, lowercases, rejects empty values, deduplicates, sorts,
// and bounds a profile tag list.
func CanonicalTags(in []string) ([]string, error) {
	return tags.Canonical(in)
}
