# passterisk

Masked password input for Go terminal programs.

## Why this exists

The Go standard library has no masked password reader. The closest option is `golang.org/x/term.ReadPassword`, which reads a line silently — you type, nothing appears, and you have no signal that your keystrokes are landing. That works, but it's a bad UX:

- Users can't tell if the program is hung, if their keyboard is connected, or if they've already typed three characters or thirteen.
- Backspace works at the byte level, not the rune level — paste a Unicode password and a single backspace can leave a half-formed UTF-8 sequence.
- There's no way to opt into the familiar `*****` echo style most CLIs use.

Most projects end up reinventing this: raw-mode toggling, byte-by-byte UTF-8 reassembly, backspace handling that respects rune boundaries (and rune *widths* — `中` is two columns wide), Ctrl-C / Ctrl-D handling, and a non-TTY fallback for piped input. Get any one of those wrong and you ship a password prompt that mangles non-ASCII input or swallows interrupts.

`passterisk` is that code, written once, tested, and packaged.

## What it does

- Echoes a configurable mask rune (`*` by default; supports any Unicode rune, including wide chars and Nerd Font glyphs) for each character typed.
- Correctly handles multi-byte UTF-8 input — accumulates bytes until a full rune is available before echoing or appending.
- Backspace erases one *rune* and the right number of terminal columns, regardless of mask width.
- Interrupt keys (Ctrl-C, Ctrl-D by default; configurable) abort cleanly with `ErrInterrupted`.
- `ClearLine` option wipes the echoed mask row after Enter, so the prompt doesn't leave `********` on screen.
- Output defaults to `os.Stderr` so it never collides with stdout pipelines.
- Falls back to a plain line read when stdin isn't a TTY (piped input, CI, etc.) instead of blowing up on raw-mode failure.
- Cross-platform: handles both DEL (127, Unix) and BS (8, Windows) for backspace.

## Install

```sh
go get github.com/kenk667/passterisk
```

## Usage

### Quick start (defaults: `*` mask, Ctrl-C/Ctrl-D abort, stderr prompt)

```go
package main

import (
    "errors"
    "fmt"
    "log"
    "os"

    "github.com/kenk667/passterisk"
)

func main() {
    pw, err := passterisk.Read("Password: ", passterisk.DefaultOptions)
    if errors.Is(err, passterisk.ErrInterrupted) {
        fmt.Fprintln(os.Stderr, "aborted")
        os.Exit(130)
    }
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println("got password of length", len(pw))
}
```

### Silent input (no echo at all)

Matches `golang.org/x/term.ReadPassword` behavior — useful when you don't want to leak password length:

```go
opts := passterisk.Options{Mask: 0, InterruptKeys: []byte{3, 4}}
pw, err := passterisk.Read("Password: ", opts)
```

### Custom mask rune

Any Unicode rune works, including wide characters and Nerd Font glyphs:

```go
opts := passterisk.DefaultOptions
opts.Mask = '•'
pw, err := passterisk.Read("Password: ", opts)
```

### Clear the masked row after Enter

Wipes `********` off the screen once the user submits, so the terminal stays tidy:

```go
opts := passterisk.DefaultOptions
opts.ClearLine = true
pw, err := passterisk.Read("Password: ", opts)
```

### Add ESC as an extra interrupt key

```go
opts := passterisk.DefaultOptions
opts.InterruptKeys = []byte{3, 4, 27} // Ctrl-C, Ctrl-D, ESC
pw, err := passterisk.Read("Password: ", opts)
```

### Change the project-wide defaults

`DefaultOptions` is a package-level `var` — mutate it once at startup and every call site picks up the change:

```go
func init() {
    passterisk.DefaultOptions.Mask = '•'
    passterisk.DefaultOptions.ClearLine = true
}
```

### Behavior reference

| Situation | What happens |
|---|---|
| User presses Enter | Returns the accumulated string, `nil` error |
| User presses Backspace | Removes one rune from the buffer and erases the corresponding mask cells |
| User presses Ctrl-C or Ctrl-D | Returns `""`, `ErrInterrupted` |
| Stdin is not a TTY (piped input) | Falls back to a plain line read with no echo manipulation |
| Stdin closes mid-input (EOF) | Returns whatever was typed so far plus `io.EOF` |
| User pastes Unicode | Multi-byte UTF-8 sequences are reassembled before echo; backspace erases by rune |

### Options

| Field | Type | Default | Effect |
|---|---|---|---|
| `Mask` | `rune` | `'*'` | Echoed for each character. `0` = silent. |
| `ClearLine` | `bool` | `false` | Erase the mask row from the terminal on Enter. |
| `Output` | `io.Writer` | `os.Stderr` | Where the prompt and mask are written. Stderr keeps stdout clean for pipelines. |
| `InterruptKeys` | `[]byte` | `{3, 4}` | Single-byte values that abort with `ErrInterrupted`. |

## License

MIT.
