package configyaml

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}
	return path
}

func TestLoadAndExpand_ResolvesEnvPlaceholder(t *testing.T) {
	t.Setenv("CONFIGYAML_TEST_SECRET", "s3cr3t")
	path := writeFile(t, "password: \"{{env:CONFIGYAML_TEST_SECRET}}\"\n")

	var out struct {
		Password string `yaml:"password"`
	}
	if err := LoadAndExpand(path, &out); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if out.Password != "s3cr3t" {
		t.Fatalf("expected resolved password %q, got %q", "s3cr3t", out.Password)
	}
}

func TestLoadAndExpand_ResolvesFilePlaceholder(t *testing.T) {
	secretFile := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(secretFile, []byte("from-a-file\n"), 0o600); err != nil {
		t.Fatalf("writing secret file: %v", err)
	}
	path := writeFile(t, "token: \"{{file:"+secretFile+"}}\"\n")

	var out struct {
		Token string `yaml:"token"`
	}
	if err := LoadAndExpand(path, &out); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if out.Token != "from-a-file" {
		t.Fatalf("expected resolved token %q, got %q", "from-a-file", out.Token)
	}
}

// Placeholder resolution isn't limited to string fields — a non-string
// (int) field templated the same way must still decode to the right type,
// proving the resolved node's tag/style reset works.
func TestLoadAndExpand_ResolvesPlaceholderOnNonStringField(t *testing.T) {
	t.Setenv("CONFIGYAML_TEST_PORT", "9999")
	path := writeFile(t, "port: \"{{env:CONFIGYAML_TEST_PORT}}\"\n")

	var out struct {
		Port int `yaml:"port"`
	}
	if err := LoadAndExpand(path, &out); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if out.Port != 9999 {
		t.Fatalf("expected port 9999, got %d", out.Port)
	}
}

// A value that only partly looks like a placeholder (extra text alongside
// the braces) is left as a literal, not resolved — the whole scalar must be
// the placeholder.
func TestLoadAndExpand_PartialPlaceholderIsLiteral(t *testing.T) {
	path := writeFile(t, "value: \"prefix-{{env:SOME_VAR}}-suffix\"\n")

	var out struct {
		Value string `yaml:"value"`
	}
	if err := LoadAndExpand(path, &out); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if out.Value != "prefix-{{env:SOME_VAR}}-suffix" {
		t.Fatalf("expected literal passthrough, got %q", out.Value)
	}
}

func TestLoadAndExpand_UnsetEnvFailsClosed(t *testing.T) {
	path := writeFile(t, "nested:\n  password: \"{{env:CONFIGYAML_TEST_DOES_NOT_EXIST}}\"\n")

	var out struct {
		Nested struct {
			Password string `yaml:"password"`
		} `yaml:"nested"`
	}
	err := LoadAndExpand(path, &out)
	if err == nil {
		t.Fatal("expected an error for an unset placeholder env var")
	}
	if !strings.Contains(err.Error(), "nested.password") {
		t.Fatalf("error = %q, want it to name the failing field path", err)
	}
}

func TestLoadAndExpand_MissingFile(t *testing.T) {
	err := LoadAndExpand(filepath.Join(t.TempDir(), "does-not-exist.yaml"), &struct{}{})
	if err == nil {
		t.Fatal("expected an error for a missing file")
	}
	if !strings.Contains(err.Error(), "reading config file") {
		t.Fatalf("expected a file-read error, got: %v", err)
	}
}

func TestLoadAndExpand_MalformedYAML(t *testing.T) {
	path := writeFile(t, "not: valid: yaml: [")

	err := LoadAndExpand(path, &struct{}{})
	if err == nil {
		t.Fatal("expected an error for malformed YAML")
	}
	if !strings.Contains(err.Error(), "parsing config file") {
		t.Fatalf("expected a parse error, got: %v", err)
	}
}
