package cli

import (
	"bytes"
	"context"
	"testing"

	"github.com/spf13/cobra"
)

func TestVersionFlagPrintsBuildMetadata(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	root := NewRootCommand(Options{
		Out: &stdout,
		Err: &stderr,
		Build: BuildInfo{
			Version: "0.1.0",
			Commit:  "abc1234",
			Date:    "2026-05-08T10:00:00Z",
		},
	})
	root.SetArgs([]string{"--version"})

	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("ExecuteContext returned an error: %v", err)
	}
	if got, want := stdout.String(), "template-mcp 0.1.0 (abc1234) built 2026-05-08T10:00:00Z\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
	if got := stderr.String(); got != "" {
		t.Fatalf("stderr = %q, want empty", got)
	}
}

func TestRootCommandRegistersTransportSubcommands(t *testing.T) {
	t.Parallel()

	root := NewRootCommand(Options{})

	got := make(map[string]bool)
	for _, cmd := range root.Commands() {
		got[cmd.Name()] = true
	}

	for _, name := range []string{stdioCommandName, httpCommandName} {
		if !got[name] {
			t.Errorf("root command is missing the %q subcommand", name)
		}
	}
}

func TestHTTPCommandDefaultsAddrToLoopback(t *testing.T) {
	t.Parallel()

	root := NewRootCommand(Options{})

	var httpCmd *cobra.Command
	for _, cmd := range root.Commands() {
		if cmd.Name() == httpCommandName {
			httpCmd = cmd
			break
		}
	}
	if httpCmd == nil {
		t.Fatal("root command is missing the http subcommand")
	}

	addr, err := httpCmd.Flags().GetString("addr")
	if err != nil {
		t.Fatalf("get addr flag: %v", err)
	}
	if want := "localhost:8080"; addr != want {
		t.Fatalf("addr default = %q, want %q", addr, want)
	}
}
