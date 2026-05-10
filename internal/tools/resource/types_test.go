package resource

import (
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// TestTruncateRunes pins the rune-aware truncation contract: byte-length
// based slicing would happily cut a multi-byte rune in half and produce
// invalid UTF-8 in the JSON response. truncateRunes must always return
// well-formed UTF-8 plus a flag indicating whether truncation happened.
func TestTruncateRunes(t *testing.T) {
	t.Run("ascii under cap is unchanged", func(t *testing.T) {
		got, truncated := truncateRunes("hello", 10)
		assert.Equal(t, "hello", got)
		assert.False(t, truncated)
	})

	t.Run("ascii at cap is unchanged", func(t *testing.T) {
		got, truncated := truncateRunes("hello", 5)
		assert.Equal(t, "hello", got)
		assert.False(t, truncated)
	})

	t.Run("ascii over cap is truncated with ellipsis", func(t *testing.T) {
		got, truncated := truncateRunes("hello world", 5)
		assert.Equal(t, "hello…", got)
		assert.True(t, truncated)
	})

	t.Run("multi-byte runes produce valid utf8", func(t *testing.T) {
		// Each "→" is 3 bytes, but one rune.
		s := "policy → fail → validation"
		got, truncated := truncateRunes(s, 8)
		assert.True(t, truncated)
		assert.True(t, utf8.ValidString(got), "truncated output must be valid UTF-8")
		// 8 runes from "policy → fail → validation" = "policy →"
		assert.Equal(t, "policy →…", got)
	})

	t.Run("zero cap on non-empty string truncates to ellipsis", func(t *testing.T) {
		got, truncated := truncateRunes("anything", 0)
		assert.Equal(t, "…", got)
		assert.True(t, truncated)
	})
}

// TestExtractEventInfo_MessageTruncation verifies that extractEventInfo
// honours the rune-aware cap on event.message and only sets
// messageTruncated when the message actually got cut. The kyverno-style
// arrow characters in the input ensure a byte-based truncation would be
// observable as invalid UTF-8.
func TestExtractEventInfo_MessageTruncation(t *testing.T) {
	long := ""
	for utf8.RuneCountInString(long) <= eventMessageMaxRunes {
		long += "policy → fail "
	}

	obj := &unstructured.Unstructured{Object: map[string]any{
		"type":    "Warning",
		"reason":  "PolicyViolation",
		"message": long,
	}}
	summary := ResourceSummary{Extra: map[string]interface{}{}}

	extractEventInfo(obj, &summary)

	msg, ok := summary.Extra["message"].(string)
	require.True(t, ok, "message must be a string")
	assert.True(t, utf8.ValidString(msg), "truncated message must be valid UTF-8")
	assert.LessOrEqual(t, utf8.RuneCountInString(msg), eventMessageMaxRunes+1, "expected at most maxRunes + ellipsis")
	assert.Equal(t, true, summary.Extra["messageTruncated"], "messageTruncated must be set when truncated")

	short := "Pulled image"
	obj2 := &unstructured.Unstructured{Object: map[string]any{
		"type":    "Normal",
		"message": short,
	}}
	summary2 := ResourceSummary{Extra: map[string]interface{}{}}
	extractEventInfo(obj2, &summary2)
	assert.Equal(t, short, summary2.Extra["message"])
	_, hasTruncatedFlag := summary2.Extra["messageTruncated"]
	assert.False(t, hasTruncatedFlag, "messageTruncated must be absent when message fits")
}
