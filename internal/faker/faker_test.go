package faker

import (
	"strings"
	"testing"
)

func TestGenerateUUID(t *testing.T) {
	t.Parallel()
	result := Generate("id={{gen.uuid}}")
	if strings.Contains(result, "{{gen.uuid}}") {
		t.Error("placeholder not replaced")
	}
	if !strings.Contains(result, "-") {
		t.Error("UUID should contain dashes")
	}
	// UUID format: 8-4-4-4-12
	parts := strings.Split(strings.TrimPrefix(result, "id="), "-")
	if len(parts) != 5 {
		t.Errorf("expected 5 UUID parts, got %d", len(parts))
	}
}

func TestGenerateEmail(t *testing.T) {
	t.Parallel()
	result := Generate("{{gen.email}}")
	if !strings.Contains(result, "@") {
		t.Error("email should contain @")
	}
}

func TestGenerateName(t *testing.T) {
	t.Parallel()
	result := Generate("{{gen.name}}")
	if strings.Contains(result, "{{") {
		t.Error("placeholder not replaced")
	}
	if !strings.Contains(result, " ") {
		t.Error("name should have first and last")
	}
}

func TestGenerateInt(t *testing.T) {
	t.Parallel()
	result := Generate("num={{gen.int}}")
	if strings.Contains(result, "{{") {
		t.Error("not replaced")
	}
	if !strings.HasPrefix(result, "num=") {
		t.Error("prefix lost")
	}
}

func TestGenerateMultiple(t *testing.T) {
	t.Parallel()
	result := Generate("{{gen.name}} <{{gen.email}}> id:{{gen.uuid}}")
	if strings.Contains(result, "{{gen.") {
		t.Error("some placeholders not replaced")
	}
	if !strings.Contains(result, "@") {
		t.Error("email not generated")
	}
	if !strings.Contains(result, "-") {
		t.Error("UUID not generated")
	}
}

func TestGenerateNoPlaceholder(t *testing.T) {
	t.Parallel()
	input := "just a normal string"
	result := Generate(input)
	if result != input {
		t.Error("should not modify string without placeholders")
	}
}

func TestGenerateTimestamp(t *testing.T) {
	t.Parallel()
	result := Generate("{{gen.timestamp}}")
	if strings.Contains(result, "{{") {
		t.Error("not replaced")
	}
	if !strings.Contains(result, "T") {
		t.Error("RFC3339 should contain T")
	}
}

func TestMultipleSamePlaceholder(t *testing.T) {
	t.Parallel()
	result := Generate("{{gen.uuid}}-{{gen.uuid}}")
	if strings.Contains(result, "{{") {
		t.Error("not replaced")
	}
}
