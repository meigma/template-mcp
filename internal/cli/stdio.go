package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync/atomic"

	"github.com/spf13/cobra"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/meigma/template-mcp/internal/mcpserver"
)

// stdioCommandName is the name of the stdio subcommand, also used by its tests.
const stdioCommandName = "stdio"

// newStdioCommand builds the "stdio" subcommand, which serves the MCP server
// over the stdio transport for local clients that spawn the process.
//
// To produce an HTTP-only repository, delete this file and its AddCommand call
// in root.go.
func newStdioCommand(options Options) *cobra.Command {
	return &cobra.Command{
		Use:   stdioCommandName,
		Short: "Serve the MCP server over stdio (local transport)",
		Long: "Serve the MCP server over the stdio transport.\n\n" +
			"The client launches this process and exchanges newline-delimited " +
			"JSON-RPC messages over stdin/stdout.",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			// CRITICAL: stdout is the JSON-RPC channel for this transport.
			// Nothing in this code path may write to os.Stdout — a stray
			// fmt.Println would corrupt the protocol. The server logs to stderr
			// (see mcpserver.New); keep all diagnostics on stderr too.
			srv := mcpserver.New(mcpserver.BuildInfo{Version: options.Build.Version})

			// Drive the protocol over the command's streams rather than
			// os.Stdin/os.Stdout directly, so the injectable Options.In/Out seam
			// is real and the stdio path is testable. In production these resolve
			// to os.Stdin/os.Stdout (see main.go); the closers are no-ops because
			// the process, not this transport, owns those file descriptors.
			input := &eofReader{reader: cmd.InOrStdin()}
			transport := &mcp.IOTransport{
				Reader: io.NopCloser(input),
				Writer: nopWriteCloser{Writer: cmd.OutOrStdout()},
			}

			err := srv.Run(cmd.Context(), transport)
			// Treat both normal stdio shutdowns as a clean (zero-status) exit;
			// otherwise every routine disconnect would look like a crash.
			//   - SIGINT/SIGTERM: the signal-derived context is cancelled and
			//     Server.Run returns ctx.Err() (context.Canceled).
			//   - The client closes the input stream (the MCP spec's primary way
			//     to shut a stdio server down). When the client is idle at that
			//     point, the SDK filters io.EOF and Run returns nil; but when the
			//     client disconnects while a request is still in flight, Run
			//     instead returns an internal "server is closing: EOF" error that
			//     does NOT unwrap to io.EOF. Detecting the input EOF directly
			//     covers both timings.
			// Anything else is a real failure.
			switch {
			case err == nil, errors.Is(err, context.Canceled), input.sawEOF():
				return nil
			default:
				return fmt.Errorf("run stdio server: %w", err)
			}
		},
	}
}

// eofReader wraps an [io.Reader] and records whether it has reached [io.EOF], so
// the caller can recognize the client closing the input stream as a clean
// shutdown even when the SDK surfaces it as a non-EOF "server is closing" error.
//
// The flag is atomic so the Read (on the SDK's read goroutine) and the sawEOF
// check (after Server.Run returns) are race-free; Run returns only after that
// goroutine has finished, which establishes the happens-before that makes the
// store visible to sawEOF.
type eofReader struct {
	reader io.Reader
	eof    atomic.Bool
}

func (r *eofReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	if errors.Is(err, io.EOF) {
		r.eof.Store(true)
	}
	return n, err
}

func (r *eofReader) sawEOF() bool { return r.eof.Load() }

// nopWriteCloser adapts an [io.Writer] to an [io.WriteCloser] with a no-op
// Close. It is the write-side counterpart to [io.NopCloser].
type nopWriteCloser struct{ io.Writer }

func (nopWriteCloser) Close() error { return nil }
