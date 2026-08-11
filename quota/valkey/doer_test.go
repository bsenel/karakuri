package valkey

import (
	"context"
	"errors"
	"testing"
)

// The Doer contract promises to tolerate whichever numeric shape a client hands
// back, because they genuinely disagree: some return int64 for a RESP integer,
// some a bulk string for everything, some []byte. An adapter author should not
// have to guess which one this package wanted.
func TestToInt64AcceptsEveryShapeAClientMightReturn(t *testing.T) {
	tests := []struct {
		name string
		in   any
		want int64
	}{
		{"int64", int64(42), 42},
		{"int", 42, 42},
		{"bulk string", "42", 42},
		{"bytes", []byte("42"), 42},
		{"negative", int64(-1), -1},
		// A nil element is how a client reports a missing value; zero is the
		// right reading and an error would be noise.
		{"nil", nil, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := toInt64(tc.in)
			if err != nil {
				t.Fatalf("toInt64(%#v) = %v", tc.in, err)
			}
			if got != tc.want {
				t.Errorf("toInt64(%#v) = %d, want %d", tc.in, got, tc.want)
			}
		})
	}
}

func TestToInt64RejectsWhatItCannotRead(t *testing.T) {
	for _, in := range []any{"not a number", []byte("x"), 1.5, struct{}{}} {
		if _, err := toInt64(in); !errors.Is(err, ErrUnexpectedReply) {
			t.Errorf("toInt64(%#v) error = %v, want ErrUnexpectedReply", in, err)
		}
	}
}

func TestDoerFuncSatisfiesDoer(t *testing.T) {
	// The adapter shape the package doc promises is four lines; this is the
	// compile-time proof that a plain function is one of them.
	var d Doer = DoerFunc(func(_ context.Context, args ...string) (any, error) {
		return int64(len(args)), nil
	})
	got, err := d.Do(context.Background(), "PING", "x")
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if got != int64(2) {
		t.Errorf("Do() = %v, want 2", got)
	}
}
