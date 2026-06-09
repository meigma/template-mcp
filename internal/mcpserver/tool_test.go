package mcpserver

import (
	"context"
	"math"
	"testing"
)

func TestRandomInt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		in      randomIntInput
		wantErr bool
	}{
		{name: "in range", in: randomIntInput{Min: 1, Max: 10}},
		{name: "equal bounds", in: randomIntInput{Min: 5, Max: 5}},
		{name: "negative range", in: randomIntInput{Min: -10, Max: -1}},
		{name: "spans zero", in: randomIntInput{Min: -3, Max: 3}},
		// Extreme ranges must not panic: a naive int64 span computation
		// (max - min + 1) overflows here and would make crypto/rand.Int panic.
		{name: "full int range", in: randomIntInput{Min: math.MinInt, Max: math.MaxInt}},
		{name: "wide range from min", in: randomIntInput{Min: math.MinInt, Max: 0}},
		{name: "wide range to max", in: randomIntInput{Min: 0, Max: math.MaxInt}},
		{name: "min greater than max", in: randomIntInput{Min: 10, Max: 1}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Repeat to exercise the random draw across the range.
			for range 100 {
				assertRandomInt(t, tt.in, tt.wantErr)
			}
		})
	}
}

// assertRandomInt runs randomInt once and checks the error and range
// expectations for the given input.
func assertRandomInt(t *testing.T, in randomIntInput, wantErr bool) {
	t.Helper()

	_, out, err := randomInt(context.Background(), nil, in)
	if wantErr {
		if err == nil {
			t.Fatalf("randomInt(%+v) error = nil, want error", in)
		}
		return
	}
	if err != nil {
		t.Fatalf("randomInt(%+v) error = %v, want nil", in, err)
	}
	if out.Value < in.Min || out.Value > in.Max {
		t.Fatalf("randomInt(%+v) = %d, want within [%d, %d]", in, out.Value, in.Min, in.Max)
	}
}
