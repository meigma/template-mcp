package reloader

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		options        func(t *testing.T) Options
		wantErr        string
		assertReloader func(t *testing.T, opts Options, got *Reloader)
	}{
		{
			name: "errors when Watcher is missing",
			options: func(t *testing.T) Options {
				t.Helper()
				opts := validOptions(t)
				opts.Watcher = nil
				return opts
			},
			wantErr: "watcher is required",
		},
		{
			name: "errors when Builder is missing",
			options: func(t *testing.T) Options {
				t.Helper()
				opts := validOptions(t)
				opts.Builder = nil
				return opts
			},
			wantErr: "builder is required",
		},
		{
			name: "errors when Upstream is missing",
			options: func(t *testing.T) Options {
				t.Helper()
				opts := validOptions(t)
				opts.Upstream = nil
				return opts
			},
			wantErr: "upstream is required",
		},
		{
			name:    "defaults nil Logger to a no-op logger and nil Clock to real time",
			options: validOptions,
			assertReloader: func(t *testing.T, _ Options, got *Reloader) {
				t.Helper()
				assert.NotNil(t, got.logger, "expected a no-op logger default for nil Options.Logger")
				assert.IsType(t, systemClock{}, got.clock, "expected the real-time clock default for nil Options.Clock")
			},
		},
		{
			name: "keeps a caller-provided Logger and Clock",
			options: func(t *testing.T) Options {
				t.Helper()
				opts := validOptions(t)
				opts.Logger = slog.New(slog.DiscardHandler)
				opts.Clock = NewMockClock(t)
				return opts
			},
			assertReloader: func(t *testing.T, opts Options, got *Reloader) {
				t.Helper()
				assert.Same(t, opts.Logger, got.logger, "expected the caller-provided logger to be kept")
				assert.Same(t, opts.Clock, got.clock, "expected the caller-provided clock to be kept")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			opts := tt.options(t)

			got, err := New(opts)

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				assert.Nil(t, got, "expected no Reloader alongside a construction error")
				return
			}
			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Same(t, opts.Watcher, got.watcher, "expected the provided Watcher port to be wired")
			assert.Same(t, opts.Builder, got.builder, "expected the provided Builder port to be wired")
			assert.Same(t, opts.Upstream, got.upstream, "expected the provided Upstream port to be wired")
			tt.assertReloader(t, opts, got)
		})
	}
}

func validOptions(t *testing.T) Options {
	t.Helper()

	return Options{
		Watcher:  NewMockWatcher(t),
		Builder:  NewMockBuilder(t),
		Upstream: NewMockUpstream(t),
	}
}
