package cli

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

// TestStdioCommandExitsCleanlyOnInputClose guards the two behaviors that the
// stdio path must get right: the protocol is driven over the injected
// Options.In stream (not hardcoded [os.Stdin]), and the client closing that
// stream is a clean shutdown (exit 0), not an error. A finite reader reaches EOF
// after the message, so ExecuteContext must return nil regardless of whether the
// disconnect races with in-flight request handling.
func TestStdioCommandExitsCleanlyOnInputClose(t *testing.T) {
	t.Parallel()

	const initialize = `{"jsonrpc":"2.0","id":1,"method":"initialize",` +
		`"params":{"protocolVersion":"2025-11-25","capabilities":{},` +
		`"clientInfo":{"name":"test","version":"0"}}}` + "\n"

	root := NewRootCommand(Options{
		In:    strings.NewReader(initialize),
		Out:   io.Discard,
		Err:   io.Discard,
		Viper: viper.New(),
	})
	root.SetArgs([]string{stdioCommandName})

	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("stdio command exited with error on input close: %v", err)
	}
}
