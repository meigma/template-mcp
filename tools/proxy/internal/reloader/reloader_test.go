package reloader

import (
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Caller-provided knob values for TestNew, distinct from the defaults.
const (
	customBufferLimit    = 7
	customBufferTimeout  = 3 * time.Second
	customDebounce       = 100 * time.Millisecond
	customQuiesceGrace   = 2 * time.Second
	customBackoffFloor   = 50 * time.Millisecond
	customBackoffCeiling = time.Second
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
			name: "errors when BufferLimit is negative",
			options: func(t *testing.T) Options {
				t.Helper()
				opts := validOptions(t)
				opts.BufferLimit = -1
				return opts
			},
			wantErr: "buffer limit must not be negative",
		},
		{
			name: "errors when BufferTimeout is negative",
			options: func(t *testing.T) Options {
				t.Helper()
				opts := validOptions(t)
				opts.BufferTimeout = -time.Second
				return opts
			},
			wantErr: "buffer timeout must not be negative",
		},
		{
			name: "errors when Debounce is negative",
			options: func(t *testing.T) Options {
				t.Helper()
				opts := validOptions(t)
				opts.Debounce = -time.Second
				return opts
			},
			wantErr: "debounce must not be negative",
		},
		{
			name: "errors when QuiesceGrace is negative",
			options: func(t *testing.T) Options {
				t.Helper()
				opts := validOptions(t)
				opts.QuiesceGrace = -time.Second
				return opts
			},
			wantErr: "quiesce grace must not be negative",
		},
		{
			name: "errors when BackoffFloor is negative",
			options: func(t *testing.T) Options {
				t.Helper()
				opts := validOptions(t)
				opts.BackoffFloor = -time.Second
				return opts
			},
			wantErr: "backoff floor must not be negative",
		},
		{
			name: "errors when BackoffCeiling is negative",
			options: func(t *testing.T) Options {
				t.Helper()
				opts := validOptions(t)
				opts.BackoffCeiling = -time.Second
				return opts
			},
			wantErr: "backoff ceiling must not be negative",
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
			name:    "defaults zero timing and sizing knobs",
			options: validOptions,
			assertReloader: func(t *testing.T, _ Options, got *Reloader) {
				t.Helper()
				require.NotNil(t, got.router, "expected the call router wired at construction")
				assert.Equal(t, defaultBufferLimit, got.router.bufferLimit, "expected the default buffer limit")
				assert.Equal(t, defaultBufferTimeout, got.router.bufferTimeout, "expected the default buffer timeout")
				assert.Equal(t, defaultDebounce, got.debounce, "expected the default debounce")
				assert.Equal(t, defaultQuiesceGrace, got.quiesceGrace, "expected the default quiesce grace")
				assert.Equal(t, defaultBackoffFloor, got.backoffFloor, "expected the default backoff floor")
				assert.Equal(t, defaultBackoffCeiling, got.backoffCeiling, "expected the default backoff ceiling")
			},
		},
		{
			name: "keeps caller-provided timing and sizing knobs",
			options: func(t *testing.T) Options {
				t.Helper()
				opts := validOptions(t)
				opts.BufferLimit = customBufferLimit
				opts.BufferTimeout = customBufferTimeout
				opts.Debounce = customDebounce
				opts.QuiesceGrace = customQuiesceGrace
				opts.BackoffFloor = customBackoffFloor
				opts.BackoffCeiling = customBackoffCeiling
				return opts
			},
			assertReloader: func(t *testing.T, _ Options, got *Reloader) {
				t.Helper()
				assert.Equal(
					t,
					customBufferLimit,
					got.router.bufferLimit,
					"expected the caller-provided buffer limit kept",
				)
				assert.Equal(
					t,
					customBufferTimeout,
					got.router.bufferTimeout,
					"expected the caller-provided buffer timeout kept",
				)
				assert.Equal(t, customDebounce, got.debounce,
					"expected the caller-provided debounce kept")
				assert.Equal(t, customQuiesceGrace, got.quiesceGrace,
					"expected the caller-provided quiesce grace kept")
				assert.Equal(t, customBackoffFloor, got.backoffFloor,
					"expected the caller-provided backoff floor kept")
				assert.Equal(t, customBackoffCeiling, got.backoffCeiling,
					"expected the caller-provided backoff ceiling kept")
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
