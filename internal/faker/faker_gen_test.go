package faker

import (
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// TestAllPlaceholdersReplaced ensures every documented generator actually
// substitutes and leaves no template markers behind.
func TestAllPlaceholdersReplaced(t *testing.T) {
	placeholders := []string{
		"uuid", "timestamp", "unix", "unix_ms", "date", "email", "name",
		"first_name", "last_name", "phone", "int", "int100", "int1000",
		"float", "bool", "hex16", "hex32", "alpha8", "alpha16", "alnum12",
		"ip", "useragent", "paragraph", "word", "color", "country", "city",
	}
	for _, p := range placeholders {
		in := "x={{gen." + p + "}}"
		out := Generate(in)
		if strings.Contains(out, "{{") {
			t.Errorf("%s: placeholder not replaced: %q", p, out)
		}
		if !strings.HasPrefix(out, "x=") {
			t.Errorf("%s: surrounding text lost: %q", p, out)
		}
		if out == in {
			t.Errorf("%s: value unchanged", p)
		}
	}
}

func TestGeneratorFormats(t *testing.T) {
	val := func(p string) string {
		return strings.TrimPrefix(Generate("v="+"{{gen."+p+"}}"), "v=")
	}

	if m, _ := regexp.MatchString(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`, val("uuid")); !m {
		t.Errorf("uuid not a valid v4 UUID: %s", val("uuid"))
	}
	if h := val("hex16"); len(h) != 16 {
		t.Errorf("hex16 length = %d, want 16", len(h))
	}
	if h := val("hex32"); len(h) != 32 {
		t.Errorf("hex32 length = %d, want 32", len(h))
	}
	if a := val("alpha8"); len(a) != 8 || strings.ToLower(a) != a {
		t.Errorf("alpha8 invalid: %q", a)
	}
	if a := val("alnum12"); len(a) != 12 {
		t.Errorf("alnum12 length = %d, want 12", len(a))
	}
	// int ranges
	for p, max := range map[string]int{"int": 10000, "int100": 100, "int1000": 1000} {
		n, err := strconv.Atoi(val(p))
		if err != nil || n < 0 || n >= max {
			t.Errorf("%s out of range or unparseable: %q (err %v)", p, val(p), err)
		}
	}
	if b := val("bool"); b != "true" && b != "false" {
		t.Errorf("bool = %q, want true/false", b)
	}
	// ip: four octets 0-255
	octets := strings.Split(val("ip"), ".")
	if len(octets) != 4 {
		t.Fatalf("ip not 4 octets: %s", val("ip"))
	}
	for _, o := range octets {
		if n, err := strconv.Atoi(o); err != nil || n < 0 || n > 255 {
			t.Errorf("ip octet invalid: %s", o)
		}
	}
	if e := val("email"); !strings.Contains(e, "@") || !strings.Contains(e, ".") {
		t.Errorf("email invalid: %s", e)
	}
	if p := val("phone"); !strings.HasPrefix(p, "+1") {
		t.Errorf("phone should start with +1: %s", p)
	}
	if d := val("date"); !regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`).MatchString(d) {
		t.Errorf("date not YYYY-MM-DD: %s", d)
	}
	if _, err := strconv.ParseInt(val("unix"), 10, 64); err != nil {
		t.Errorf("unix not an integer: %s", val("unix"))
	}
	if !strings.HasSuffix(val("paragraph"), ".") {
		t.Errorf("paragraph should end with a period: %s", val("paragraph"))
	}
}

func TestGenerateEmptyString(t *testing.T) {
	if Generate("") != "" {
		t.Error("empty input should return empty")
	}
}
