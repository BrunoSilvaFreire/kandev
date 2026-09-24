// Package tags canonicalizes free-form tag lists shared by agent profiles and
// workflow steps.
package tags

import (
	"fmt"
	"sort"
	"strings"
)

// Bounds keep one entity's tag list small enough to scan and compare without
// an index while leaving generous room for human labels.
const (
	MaxCount   = 32
	MaxTagSize = 64
)

// Canonical trims, lowercases, rejects empty values, deduplicates, sorts, and
// bounds a tag list. A nil or empty input returns an empty, non-nil slice so a
// JSON-array column always persists "[]".
func Canonical(in []string) ([]string, error) {
	if len(in) == 0 {
		return []string{}, nil
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for i, raw := range in {
		tag := strings.ToLower(strings.TrimSpace(raw))
		if tag == "" {
			return nil, fmt.Errorf("tag[%d] must not be empty", i)
		}
		if len(tag) > MaxTagSize {
			return nil, fmt.Errorf("tag[%d] exceeds %d bytes", i, MaxTagSize)
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		out = append(out, tag)
	}
	if len(out) > MaxCount {
		return nil, fmt.Errorf("at most %d tags allowed", MaxCount)
	}
	sort.Strings(out)
	return out, nil
}
