package cli

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

func TestIsLoopbackHost(t *testing.T) {
	t.Parallel()

	tests := []struct {
		addr string
		want bool
	}{
		{addr: "localhost:8080", want: true},
		{addr: "127.0.0.1:8080", want: true},
		{addr: "[::1]:8080", want: true},
		{addr: "localhost", want: true},
		{addr: "127.0.0.1", want: true},
		{addr: "0.0.0.0:8080", want: false},
		{addr: ":8080", want: false},
		{addr: "192.168.1.10:8080", want: false},
		{addr: "example.com:8080", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.addr, func(t *testing.T) {
			t.Parallel()

			if got := isLoopbackHost(tt.addr); got != tt.want {
				t.Fatalf("isLoopbackHost(%q) = %v, want %v", tt.addr, got, tt.want)
			}
		})
	}
}

func TestCheckBindSecurity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		addr      string
		authToken string
		insecure  bool
		wantErr   bool
	}{
		{name: "loopback without auth is allowed", addr: "localhost:8080"},
		{name: "loopback ip without auth is allowed", addr: "127.0.0.1:8080"},
		{name: "non-loopback with auth is allowed", addr: "0.0.0.0:8080", authToken: "secret"},
		{name: "non-loopback with insecure is allowed", addr: "0.0.0.0:8080", insecure: true},
		{name: "non-loopback without auth is refused", addr: "0.0.0.0:8080", wantErr: true},
		{name: "all interfaces without auth is refused", addr: ":8080", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := checkBindSecurity(tt.addr, tt.authToken, tt.insecure)
			if tt.wantErr && err == nil {
				t.Fatalf("checkBindSecurity(%q, %q, %v) = nil, want error", tt.addr, tt.authToken, tt.insecure)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("checkBindSecurity(%q, %q, %v) = %v, want nil", tt.addr, tt.authToken, tt.insecure, err)
			}
		})
	}
}

func TestRequireBearerToken(t *testing.T) {
	t.Parallel()

	const token = "s3cret-token"
	middleware := requireBearerToken(token, "localhost:8080")

	tests := []struct {
		name          string
		authHeader    string
		wantStatus    int
		wantReached   bool
		wantChallenge bool
	}{
		{name: "missing token is rejected", authHeader: "", wantStatus: http.StatusUnauthorized, wantChallenge: true},
		{
			name:          "wrong token is rejected",
			authHeader:    "Bearer wrong",
			wantStatus:    http.StatusUnauthorized,
			wantChallenge: true,
		},
		{name: "correct token passes", authHeader: "Bearer " + token, wantStatus: http.StatusOK, wantReached: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var reached bool
			handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				reached = true
				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest(http.MethodGet, "http://localhost:8080/", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if reached != tt.wantReached {
				t.Fatalf("handler reached = %v, want %v", reached, tt.wantReached)
			}
			if tt.wantChallenge && rec.Header().Get("WWW-Authenticate") == "" {
				t.Fatal("missing WWW-Authenticate challenge header on rejection")
			}
		})
	}
}

// TestHTTPCommandReadsAddrFromEnvironment exercises the TEMPLATE_MCP_ADDR -> addr
// binding (the wiring most likely to break silently after the rename step). The
// fail-closed guard refuses the non-loopback address before any socket is bound,
// so the refusal error mentioning that address proves the env value reached the
// command.
func TestHTTPCommandReadsAddrFromEnvironment(t *testing.T) {
	t.Setenv("TEMPLATE_MCP_ADDR", "0.0.0.0:65535")

	root := NewRootCommand(Options{Viper: viper.New()})
	root.SetArgs([]string{httpCommandName})
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)

	err := root.ExecuteContext(context.Background())
	if err == nil {
		t.Fatal("expected the http command to refuse the non-loopback bind, got nil")
	}
	if !strings.Contains(err.Error(), "0.0.0.0:65535") {
		t.Fatalf("error = %v, want it to mention the env-provided address", err)
	}
}

// TestEnvBindingResolvesHyphenatedFlag covers the SetEnvKeyReplacer hop that the
// addr test does not: the "auth-token" flag binds to TEMPLATE_MCP_AUTH_TOKEN
// (hyphen -> underscore). A regression dropping the replacer would break this
// while the hyphen-free addr key kept working, so it is tested explicitly. It
// binds flags directly rather than serving, keeping the test deterministic.
func TestEnvBindingResolvesHyphenatedFlag(t *testing.T) {
	t.Setenv("TEMPLATE_MCP_AUTH_TOKEN", "from-env")

	vp := viper.New()
	httpCmd := newHTTPCommand(Options{Viper: vp})
	if err := initializeConfig(httpCmd, vp); err != nil {
		t.Fatalf("initializeConfig: %v", err)
	}

	if got, want := vp.GetString(authTokenFlag), "from-env"; got != want {
		t.Fatalf("auth-token from env = %q, want %q", got, want)
	}
}
