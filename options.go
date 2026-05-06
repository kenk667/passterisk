package passterisk

import (
	"errors"
	"io"
	"os"
)

// ErrInterrupted is returned when the user presses an interrupt key (Ctrl-C, Ctrl-D).
var ErrInterrupted = errors.New("interrupted")

// Options controls the behavior of Read.
// All project-wide defaults live here — change DefaultOptions to affect every call site.
type Options struct {
	// Mask is the rune echoed for each character typed.
	// 0 = no echo (blank, current terminal default behavior).
	// Supports any Unicode rune including Nerd Font glyphs (e.g. '' for ).
	Mask rune

	// ClearLine erases the echoed mask characters from the terminal after Enter.
	// Useful when you don't want the row of asterisks left on screen.
	ClearLine bool

	// Output is where the prompt and mask echo are written.
	// Defaults to os.Stderr so it never interferes with stdout pipelines.
	Output io.Writer

	// InterruptKeys are single-byte values that abort input and return ErrInterrupted.
	// Defaults to Ctrl-C (3) and Ctrl-D (4).
	InterruptKeys []byte
}

// DefaultOptions is the project-wide default. Override individual fields at the
// call site, or change this var to affect all calls.
var DefaultOptions = Options{
	Mask:          '*',
	ClearLine:     false,
	Output:        os.Stderr,
	InterruptKeys: []byte{3, 4},
}
