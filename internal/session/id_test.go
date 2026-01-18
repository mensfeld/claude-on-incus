package session

import (
	"testing"
)

func TestGenerateSessionID(t *testing.T) {
	// Generate an ID
	id, err := GenerateSessionID()
	if err != nil {
		t.Fatalf("GenerateSessionID() failed: %v", err)
	}

	// Check length (UUID format: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx = 36 chars)
	if len(id) != 36 {
		t.Errorf("GenerateSessionID() length = %d, want 36", len(id))
	}

	// Check UUID v4 format: 8-4-4-4-12 with dashes at positions 8, 13, 18, 23
	if id[8] != '-' || id[13] != '-' || id[18] != '-' || id[23] != '-' {
		t.Errorf("GenerateSessionID() = %s, not in UUID format (expected dashes at positions 8,13,18,23)", id)
	}

	// Check that non-dash characters are hex
	for i, c := range id {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			continue // Skip dashes
		}
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			t.Errorf("GenerateSessionID() char at %d is not hex: %c", i, c)
		}
	}
}

func TestGenerateSessionIDUnique(t *testing.T) {
	// Generate multiple IDs and check they're unique
	ids := make(map[string]bool)
	count := 100

	for i := 0; i < count; i++ {
		id, err := GenerateSessionID()
		if err != nil {
			t.Fatalf("GenerateSessionID() iteration %d failed: %v", i, err)
		}

		if ids[id] {
			t.Errorf("GenerateSessionID() produced duplicate ID: %s", id)
		}
		ids[id] = true
	}

	if len(ids) != count {
		t.Errorf("Expected %d unique IDs, got %d", count, len(ids))
	}
}

func TestGenerateSessionIDFormat(t *testing.T) {
	id, err := GenerateSessionID()
	if err != nil {
		t.Fatalf("GenerateSessionID() failed: %v", err)
	}

	// Should be UUID v4 format: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx (36 chars)
	if len(id) != 36 {
		t.Errorf("ID length = %d, want 36", len(id))
	}

	// Check dashes at correct positions
	if id[8] != '-' || id[13] != '-' || id[18] != '-' || id[23] != '-' {
		t.Errorf("ID format incorrect: %s (expected UUID format with dashes)", id)
	}

	// All non-dash characters should be lowercase hex
	for i, c := range id {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			continue // Skip dashes
		}
		valid := (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')
		if !valid {
			t.Errorf("Invalid character in session ID at position %d: %c", i, c)
		}
	}

	// Check UUID v4 version bits (13th hex digit should be '4')
	if id[14] != '4' {
		t.Errorf("UUID version incorrect: expected '4' at position 14, got '%c'", id[14])
	}

	// Check UUID variant bits (17th hex digit should be 8, 9, a, or b)
	variant := id[19]
	if variant != '8' && variant != '9' && variant != 'a' && variant != 'b' {
		t.Errorf("UUID variant incorrect: expected 8/9/a/b at position 19, got '%c'", variant)
	}
}
