package cli

import "testing"

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
