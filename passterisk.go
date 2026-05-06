// Package passterisk provides masked password input for terminals.
// It handles raw mode, UTF-8 multi-byte sequences, wide characters,
// backspace, interrupt keys, and non-TTY fallback — cross-platform.
package passterisk

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"
	"golang.org/x/term"
)

// Read prints prompt to opts.Output then reads a masked line from stdin.
// In non-TTY contexts (piped input) it falls back to a plain line read with no echo.
func Read(prompt string, opts Options) (string, error) {
	out := opts.Output
	if out == nil {
		out = os.Stderr
	}

	fmt.Fprint(out, prompt)

	fd := int(os.Stdin.Fd())

	// Non-TTY fallback: piped input — plain line read, no echo manipulation.
	if !term.IsTerminal(fd) {
		return readPlain(os.Stdin, out)
	}

	oldState, err := term.MakeRaw(fd)
	if err != nil {
		// Raw mode unavailable — fall back to plain read.
		return readPlain(os.Stdin, out)
	}
	defer term.Restore(fd, oldState)

	return readLoop(os.Stdin, out, opts)
}

// readPlain reads a single line without any masking. Used for non-TTY fallback.
func readPlain(in io.Reader, out io.Writer) (string, error) {
	sc := bufio.NewScanner(in)
	if sc.Scan() {
		fmt.Fprintln(out)
		return sc.Text(), nil
	}
	if err := sc.Err(); err != nil {
		return "", err
	}
	return "", io.EOF
}

// readLoop is the core character-by-character input loop. Separated from Read
// so it can be exercised in tests without a real TTY.
func readLoop(in io.Reader, out io.Writer, opts Options) (string, error) {
	interruptSet := make(map[byte]bool, len(opts.InterruptKeys))
	for _, b := range opts.InterruptKeys {
		interruptSet[b] = true
	}

	maskWidth := 0
	if opts.Mask != 0 {
		maskWidth = runewidth.RuneWidth(opts.Mask)
	}

	var (
		buf       []rune // accumulated input runes
		byteBuf   []byte // accumulator for multi-byte UTF-8 sequences
		maskCount int    // number of mask runes echoed (used by ClearLine)
	)

	for {
		raw := make([]byte, 1)
		if _, err := in.Read(raw); err != nil {
			fmt.Fprintln(out)
			return string(buf), err
		}
		ch := raw[0]

		// Interrupt keys are always single bytes.
		if interruptSet[ch] {
			fmt.Fprintln(out)
			return "", ErrInterrupted
		}

		// Enter — raw mode delivers \r; also accept \n for non-raw readers in tests.
		if ch == '\r' || ch == '\n' {
			if opts.ClearLine && opts.Mask != 0 && maskCount > 0 {
				for i := 0; i < maskCount*maskWidth; i++ {
					fmt.Fprint(out, "\b \b")
				}
			}
			fmt.Fprintln(out)
			return string(buf), nil
		}

		// Backspace: DEL (127) on most Unix terminals, BS (8) on Windows/some others.
		if ch == 127 || ch == 8 {
			if len(buf) > 0 {
				if opts.Mask != 0 {
					for i := 0; i < maskWidth; i++ {
						fmt.Fprint(out, "\b \b")
					}
					maskCount--
				}
				buf = buf[:len(buf)-1]
			}
			continue
		}

		// Ignore other control characters.
		if ch < 32 {
			continue
		}

		// Accumulate bytes until we have a complete UTF-8 rune.
		byteBuf = append(byteBuf, ch)
		r, size := utf8.DecodeRune(byteBuf)
		if r == utf8.RuneError && size == 1 && len(byteBuf) < utf8.UTFMax {
			// Incomplete multi-byte sequence — wait for more bytes.
			continue
		}
		byteBuf = byteBuf[:0]

		if r == utf8.RuneError {
			// Invalid encoding — discard.
			continue
		}

		buf = append(buf, r)
		if opts.Mask != 0 {
			fmt.Fprintf(out, "%c", opts.Mask)
			maskCount++
		}
	}
}
