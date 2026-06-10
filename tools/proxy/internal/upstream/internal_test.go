package upstream

import (
	"bytes"
	"os"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCommandTransportFactory inspects the default factory's output without
// spawning anything; the real process spawn is proven by the M4 E2E tests.
func TestCommandTransportFactory(t *testing.T) {
	t.Parallel()

	t.Run("substitutes the artifact and wires stderr with the short default terminate", func(t *testing.T) {
		t.Parallel()

		var stderr bytes.Buffer
		up, err := New(Options{
			Argv:   []string{"{{artifact}}", "stdio", "--bin={{artifact}}"},
			Stderr: &stderr,
		})
		require.NoError(t, err, "construct upstream")

		transport, err := up.factory("/tmp/child-7")
		require.NoError(t, err, "build the default transport")

		command, ok := transport.(*mcp.CommandTransport)
		require.True(t, ok, "expected the default factory to produce a CommandTransport")
		assert.Equal(t, []string{"/tmp/child-7", "stdio", "--bin=/tmp/child-7"}, command.Command.Args,
			"expected {{artifact}} substituted in every argv element")
		assert.Same(t, &stderr, command.Command.Stderr,
			"expected the child's stderr wired to the configured writer — the transport does not do it")
		assert.Equal(t, time.Second, command.TerminateDuration,
			"expected the dev-loop-short terminate duration default, not the SDK's 5s")
	})

	t.Run("honors a configured terminate duration and defaults stderr to the proxy's", func(t *testing.T) {
		t.Parallel()

		up, err := New(Options{Argv: []string{"{{artifact}}"}, TerminateDuration: 250 * time.Millisecond})
		require.NoError(t, err, "construct upstream")

		transport, err := up.factory("/tmp/child-8")
		require.NoError(t, err, "build the default transport")

		command, ok := transport.(*mcp.CommandTransport)
		require.True(t, ok, "expected the default factory to produce a CommandTransport")
		assert.Equal(t, 250*time.Millisecond, command.TerminateDuration,
			"expected the configured terminate duration")
		assert.Same(t, os.Stderr, command.Command.Stderr,
			"expected the child's stderr to default to the proxy's stderr")
	})
}
