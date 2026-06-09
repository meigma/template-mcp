package cli

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/spf13/cobra"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/meigma/template-mcp/internal/mcpserver"
)

const (
	// httpCommandName is the name of the http subcommand, also used by its tests.
	httpCommandName = "http"
	// httpShutdownTimeout bounds how long graceful shutdown waits for in-flight
	// requests to finish before the server is forced closed.
	httpShutdownTimeout = 10 * time.Second
	// httpReadHeaderTimeout bounds how long the server waits to read request
	// headers, mitigating Slowloris-style attacks.
	httpReadHeaderTimeout = 10 * time.Second
	// demoAuthScope is the single scope advertised and required by the
	// DEMO-ONLY bearer-token seam. A real deployment derives scopes from the
	// validated token instead.
	demoAuthScope = "mcp"
	// demoTokenLifetime is the synthetic expiry reported for the demo token.
	// The middleware requires a non-zero expiration; a real verifier would read
	// the token's own exp claim instead.
	demoTokenLifetime = time.Hour
)

// newHTTPCommand builds the "http" subcommand, which serves the MCP server over
// the Streamable HTTP transport for networked clients.
//
// To produce a stdio-only repository, delete this file and its AddCommand call
// in root.go.
func newHTTPCommand(options Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   httpCommandName,
		Short: "Serve the MCP server over Streamable HTTP (networked transport)",
		Long: "Serve the MCP server over the Streamable HTTP transport.\n\n" +
			"Binds to a loopback address by default; exposing it to the network " +
			"is an explicit, security-relevant opt-in.",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			addr := options.Viper.GetString("addr")
			authToken := options.Viper.GetString("auth-token")
			insecure := options.Viper.GetBool("insecure")
			return runHTTP(cmd.Context(), options.Build, addr, authToken, insecure)
		},
	}

	// --addr defaults to loopback, not 0.0.0.0: binding to all interfaces
	// exposes the server to the local network and is a deliberate opt-in.
	cmd.Flags().String("addr", "localhost:8080", "address to listen on (env TEMPLATE_MCP_ADDR)")
	// --auth-token is empty by default, which disables auth. See requireBearerToken
	// for the heavy caveats: this is a DEMO-ONLY seam, not production auth.
	cmd.Flags().String(
		"auth-token",
		"",
		"DEMO-ONLY shared bearer token; empty disables auth (env TEMPLATE_MCP_AUTH_TOKEN)",
	)
	// --insecure is the explicit opt-in to bind a non-loopback address without
	// authentication. Without it, runHTTP refuses such a configuration so a
	// published port or container cannot silently expose every tool.
	cmd.Flags().Bool(
		"insecure",
		false,
		"allow binding a non-loopback address without authentication (UNSAFE; env TEMPLATE_MCP_INSECURE)",
	)

	return cmd
}

func runHTTP(ctx context.Context, build BuildInfo, addr, authToken string, insecure bool) error {
	if err := checkBindSecurity(addr, authToken, insecure); err != nil {
		return err
	}

	handler := mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server {
			return mcpserver.New(mcpserver.BuildInfo{Version: build.Version})
		},
		nil,
	)

	// MUST: the SDK does NOT enable Origin verification by default. Wrapping the
	// handler with the Go 1.25+ stdlib CrossOriginProtection rejects
	// cross-origin browser requests, mitigating CSRF and DNS-rebinding attacks.
	// CrossOriginProtection.Handler returns an http.Handler, so rootHandler is
	// already the interface type and can be reassigned with the auth wrapper below.
	rootHandler := http.NewCrossOriginProtection().Handler(handler)

	// When a token is configured, gate the server behind the DEMO-ONLY bearer
	// middleware. The middleware runs outside CrossOriginProtection so that
	// unauthenticated requests are rejected as early as possible.
	if authToken != "" {
		rootHandler = requireBearerToken(authToken, addr)(rootHandler)
	}

	srv := &http.Server{
		Addr:              addr,
		Handler:           rootHandler,
		ReadHeaderTimeout: httpReadHeaderTimeout,
	}

	// Graceful shutdown: when the context is cancelled (e.g. SIGINT/SIGTERM),
	// stop accepting connections and let in-flight requests drain. The
	// goroutine exits either way (ctx cancelled, or serveDone closed when
	// ListenAndServe returns for another reason such as a bind failure), so it
	// never leaks.
	serveDone := make(chan struct{})
	shutdownErr := make(chan error, 1)
	// G118 (gosec): the goroutine derives Shutdown's context from
	// context.Background rather than ctx on purpose. ctx is already cancelled by
	// the time this case fires (that cancellation is what triggers shutdown), so
	// it cannot bound the drain; a fresh context gives in-flight requests time to
	// finish.
	//nolint:gosec // G118: ctx is cancelled by design; see comment above.
	go func() {
		select {
		case <-ctx.Done():
			shutdownCtx, cancel := context.WithTimeout(context.Background(), httpShutdownTimeout)
			defer cancel()
			shutdownErr <- srv.Shutdown(shutdownCtx)
		case <-serveDone:
			shutdownErr <- nil
		}
	}()

	err := srv.ListenAndServe()
	close(serveDone)
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve http: %w", err)
	}
	if err := <-shutdownErr; err != nil {
		return fmt.Errorf("shutdown http server: %w", err)
	}

	return nil
}

// checkBindSecurity fails closed against a network-exposed, unauthenticated
// server. A non-loopback bind with no authentication would expose every tool to
// anyone who can reach the port — CrossOriginProtection only stops browser
// cross-origin requests, not direct clients such as curl. Such a configuration
// is allowed only when the operator explicitly opts in with --insecure.
func checkBindSecurity(addr, authToken string, insecure bool) error {
	if isLoopbackHost(addr) || authToken != "" || insecure {
		return nil
	}

	return fmt.Errorf(
		"refusing to bind non-loopback address %q without authentication: "+
			"set --auth-token (env TEMPLATE_MCP_AUTH_TOKEN) to require a bearer token, "+
			"or pass --insecure to expose all tools unauthenticated (UNSAFE)",
		addr,
	)
}

// isLoopbackHost reports whether addr binds only the loopback interface.
//
// It treats "localhost" and any loopback IP literal as loopback. An empty host
// (for example ":8080") binds all interfaces and is not loopback, and a
// non-localhost hostname cannot be proven loopback without resolving it, so it
// is treated as non-loopback. The conservative direction is deliberate: when in
// doubt, the caller's fail-closed guard applies.
func isLoopbackHost(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		// No port present; treat the whole string as the host.
		host = addr
	}
	switch host {
	case "":
		return false
	case "localhost":
		return true
	default:
		if ip := net.ParseIP(host); ip != nil {
			return ip.IsLoopback()
		}
		return false
	}
}

// requireBearerToken builds DEMO-ONLY bearer-token middleware.
//
// WARNING: This is a placeholder, NOT production authorization. It compares the
// presented bearer token to a single shared secret. It does not validate a JWT
// signature, issuer, audience, or expiry, and a shared static token cannot be
// revoked or scoped per client.
//
// To make this production-grade, replace the TokenVerifier below with a real
// OAuth 2.1 resource-server verifier that:
//   - validates the access token's signature against the authorization
//     server's JWKS;
//   - checks the issuer (iss) and audience (aud) claims, binding the token to
//     this resource server per RFC 8707;
//   - checks expiry (exp/nbf) and the required scopes;
//   - publishes /.well-known/oauth-protected-resource (RFC 9728) so clients can
//     discover the authorization server.
//
// See the README for the full upgrade path.
func requireBearerToken(token, addr string) func(http.Handler) http.Handler {
	secret := []byte(token)

	verifier := func(_ context.Context, presented string, _ *http.Request) (*auth.TokenInfo, error) {
		// Constant-time comparison avoids leaking the secret length/contents
		// through timing. ConstantTimeCompare reports inequality for differing
		// lengths, so no separate length check is needed.
		if subtle.ConstantTimeCompare([]byte(presented), secret) != 1 {
			return nil, fmt.Errorf("%w", auth.ErrInvalidToken)
		}

		// The middleware requires a non-zero expiration and the configured
		// scopes. A real verifier would read these from the validated token.
		return &auth.TokenInfo{
			Scopes:     []string{demoAuthScope},
			Expiration: time.Now().Add(demoTokenLifetime),
		}, nil
	}

	return auth.RequireBearerToken(verifier, &auth.RequireBearerTokenOptions{
		// Advertised to unauthenticated clients via the WWW-Authenticate header
		// so they can discover the (here, hypothetical) authorization server.
		ResourceMetadataURL: fmt.Sprintf("http://%s/.well-known/oauth-protected-resource", addr),
		Scopes:              []string{demoAuthScope},
	})
}
