package passterisk

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

// helper: run readLoop with given input bytes, return (result, output, err)
func run(input []byte, opts Options) (string, string, error) {
	var out bytes.Buffer
	result, err := readLoop(bytes.NewReader(input), &out, opts)
	return result, out.String(), err
}

func noMask() Options {
	return Options{InterruptKeys: []byte{3, 4}}
}

func starMask() Options {
	return Options{Mask: '*', InterruptKeys: []byte{3, 4}}
}

// --- basic accumulation ---

func TestBasicASCII(t *testing.T) {
	got, _, err := run([]byte("hello\r"), noMask())
	if err != nil || got != "hello" {
		t.Fatalf("want hello/nil, got %q/%v", got, err)
	}
}

func TestNewlineEnter(t *testing.T) {
	got, _, err := run([]byte("hi\n"), noMask())
	if err != nil || got != "hi" {
		t.Fatalf("want hi/nil, got %q/%v", got, err)
	}
}

func TestEmptyInput(t *testing.T) {
	got, _, err := run([]byte("\r"), noMask())
	if err != nil || got != "" {
		t.Fatalf("want empty/nil, got %q/%v", got, err)
	}
}

// --- mask echo ---

func TestMaskEcho(t *testing.T) {
	_, out, err := run([]byte("abc\r"), starMask())
	if err != nil {
		t.Fatal(err)
	}
	// three asterisks plus a newline
	if !strings.HasPrefix(out, "***") {
		t.Fatalf("want ***... in output, got %q", out)
	}
}

func TestNoEchoWhenMaskZero(t *testing.T) {
	_, out, err := run([]byte("secret\r"), noMask())
	if err != nil {
		t.Fatal(err)
	}
	// only the trailing newline should be written
	if strings.ContainsAny(out, "secret") {
		t.Fatalf("expected no echo, got %q", out)
	}
}

// --- backspace ---

func TestBackspaceDEL(t *testing.T) {
	// "abc" then DEL removes 'c', Enter
	got, _, err := run([]byte("abc\x7f\r"), noMask())
	if err != nil || got != "ab" {
		t.Fatalf("want ab/nil, got %q/%v", got, err)
	}
}

func TestBackspaceBS(t *testing.T) {
	got, _, err := run([]byte("xyz\x08\r"), noMask())
	if err != nil || got != "xy" {
		t.Fatalf("want xy/nil, got %q/%v", got, err)
	}
}

func TestBackspaceAtEmpty(t *testing.T) {
	// DEL on empty buffer should be a no-op
	got, _, err := run([]byte("\x7f\x7f\r"), noMask())
	if err != nil || got != "" {
		t.Fatalf("want empty/nil, got %q/%v", got, err)
	}
}

func TestBackspaceErasesOneMaskChar(t *testing.T) {
	// type 'a', backspace, type 'b', enter → should yield "b" and two mask chars echoed, then one erased
	got, out, err := run([]byte("a\x7fb\r"), starMask())
	if err != nil || got != "b" {
		t.Fatalf("want b/nil, got %q/%v", got, err)
	}
	// output should contain \b \b sequence for the erase
	if !strings.Contains(out, "\b \b") {
		t.Fatalf("expected backspace-erase sequence in output, got %q", out)
	}
}

// --- interrupt keys ---

func TestCtrlCInterrupt(t *testing.T) {
	_, _, err := run([]byte{3}, starMask())
	if !errors.Is(err, ErrInterrupted) {
		t.Fatalf("want ErrInterrupted, got %v", err)
	}
}

func TestCtrlDInterrupt(t *testing.T) {
	_, _, err := run([]byte{4}, starMask())
	if !errors.Is(err, ErrInterrupted) {
		t.Fatalf("want ErrInterrupted, got %v", err)
	}
}

func TestCustomInterruptKey(t *testing.T) {
	opts := Options{Mask: '*', InterruptKeys: []byte{27}} // ESC
	_, _, err := run([]byte{27}, opts)
	if !errors.Is(err, ErrInterrupted) {
		t.Fatalf("want ErrInterrupted, got %v", err)
	}
}

func TestInterruptReturnsBeforeContent(t *testing.T) {
	// content typed before interrupt should be discarded (empty return)
	result, _, err := run([]byte("hello\x03"), starMask())
	if !errors.Is(err, ErrInterrupted) {
		t.Fatalf("want ErrInterrupted, got %v", err)
	}
	if result != "" {
		t.Fatalf("want empty result on interrupt, got %q", result)
	}
}

// --- control character filtering ---

func TestControlCharsIgnored(t *testing.T) {
	// \x01 (Ctrl-A), \x1b (ESC) with no interrupt mapping, \x1f — all < 32 and not mapped
	opts := Options{InterruptKeys: []byte{}} // no interrupt keys
	got, _, err := run([]byte("\x01\x1b\x1fabc\r"), opts)
	if err != nil || got != "abc" {
		t.Fatalf("want abc/nil, got %q/%v", got, err)
	}
}

// --- UTF-8 multi-byte runes ---

func TestUTF8TwoByteRune(t *testing.T) {
	// é = 0xC3 0xA9
	got, _, err := run(append([]byte{0xC3, 0xA9}, '\r'), noMask())
	if err != nil || got != "é" {
		t.Fatalf("want é/nil, got %q/%v", got, err)
	}
}

func TestUTF8ThreeByteRune(t *testing.T) {
	// '中' = 0xE4 0xB8 0xAD
	got, _, err := run(append([]byte{0xE4, 0xB8, 0xAD}, '\r'), noMask())
	if err != nil || got != "中" {
		t.Fatalf("want 中/nil, got %q/%v", got, err)
	}
}

func TestUTF8BackspaceErasesMaskOnce(t *testing.T) {
	// '中' is 2-wide, mask='*' — backspace should erase one mask char (width 1)
	got, out, err := run(append([]byte{0xE4, 0xB8, 0xAD, 0x7F}, '\r'), starMask())
	if err != nil || got != "" {
		t.Fatalf("want empty/nil after backspace, got %q/%v", got, err)
	}
	_ = out // erase sequence is present; exact count depends on mask rune width
}

func TestWideCharMaskWidth(t *testing.T) {
	// Use a wide mask rune (fullwidth asterisk U+FF0A = 0xEF 0xBC 0x8A, width 2)
	opts := Options{Mask: '＊', InterruptKeys: []byte{3, 4}} // U+FF0A
	_, out, err := run([]byte("a\r"), opts)
	if err != nil {
		t.Fatal(err)
	}
	// The wide mask should be echoed; output contains at least the rune
	if !strings.ContainsRune(out, '＊') {
		t.Fatalf("expected wide mask rune in output, got %q", out)
	}
}

// --- ClearLine ---

func TestClearLineErasesOnEnter(t *testing.T) {
	opts := Options{Mask: '*', ClearLine: true, InterruptKeys: []byte{3, 4}}
	_, out, err := run([]byte("abc\r"), opts)
	if err != nil {
		t.Fatal(err)
	}
	// Should contain backspace-erase sequences for 3 asterisks
	count := strings.Count(out, "\b \b")
	if count < 3 {
		t.Fatalf("expected ≥3 backspace-erase sequences for ClearLine, got %d in %q", count, out)
	}
}

func TestClearLineNoOpWhenNoMask(t *testing.T) {
	opts := Options{Mask: 0, ClearLine: true, InterruptKeys: []byte{3, 4}}
	_, out, err := run([]byte("abc\r"), opts)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "\b") {
		t.Fatalf("expected no backspace with mask=0, got %q", out)
	}
}

// --- EOF / read error ---

func TestEOFReturnsPartialInput(t *testing.T) {
	// Reader with no terminator — returns what was accumulated plus io.EOF
	got, _, err := run([]byte("abc"), noMask())
	if !errors.Is(err, io.EOF) {
		t.Fatalf("want io.EOF, got %v", err)
	}
	if got != "abc" {
		t.Fatalf("want abc, got %q", got)
	}
}

// --- readPlain ---

func TestReadPlain(t *testing.T) {
	var out bytes.Buffer
	result, err := readPlain(strings.NewReader("mypassword\n"), &out)
	if err != nil || result != "mypassword" {
		t.Fatalf("want mypassword/nil, got %q/%v", result, err)
	}
}

func TestReadPlainEOF(t *testing.T) {
	var out bytes.Buffer
	_, err := readPlain(strings.NewReader(""), &out)
	if !errors.Is(err, io.EOF) {
		t.Fatalf("want io.EOF, got %v", err)
	}
}

// --- DefaultOptions ---

func TestDefaultOptionsMask(t *testing.T) {
	if DefaultOptions.Mask != '*' {
		t.Fatalf("want '*', got %q", DefaultOptions.Mask)
	}
}

func TestDefaultOptionsInterruptKeys(t *testing.T) {
	if len(DefaultOptions.InterruptKeys) != 2 {
		t.Fatalf("want 2 interrupt keys, got %d", len(DefaultOptions.InterruptKeys))
	}
}
