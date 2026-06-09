package templateinfo

import "testing"

func TestEnvPrefix(t *testing.T) {
	t.Parallel()

	if got, want := EnvPrefix(), "TEMPLATE_MCP"; got != want {
		t.Fatalf("EnvPrefix() = %q, want %q", got, want)
	}
}

func TestIdentityConstantsSet(t *testing.T) {
	t.Parallel()

	if Name == "" {
		t.Fatal("Name is empty")
	}
	if Title == "" {
		t.Fatal("Title is empty")
	}
}
