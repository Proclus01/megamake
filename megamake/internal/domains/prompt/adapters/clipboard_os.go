package adapters

import (
	"bytes"
	"os/exec"
	"runtime"
)

// OSClipboard is a best-effort, cross-platform clipboard implementation.
// It mirrors the intent of the Swift implementation (pbcopy/wl-copy/xclip/xsel/clip).
type OSClipboard struct{}

func NewOSClipboard() OSClipboard {
	return OSClipboard{}
}

func (c OSClipboard) Copy(text string) (bool, error) {
	type candidate struct {
		cmd  string
		args []string
	}

	candidates := []candidate{
		{cmd: "pbcopy", args: nil},                                // macOS
		{cmd: "wl-copy", args: nil},                               // Wayland
		{cmd: "xclip", args: []string{"-selection", "clipboard"}}, // X11
		{cmd: "xsel", args: []string{"--clipboard", "--input"}},   // X11
	}

	if runtime.GOOS == "windows" {
		candidates = append([]candidate{{cmd: "clip", args: nil}}, candidates...)
	}

	for _, cand := range candidates {
		path, err := exec.LookPath(cand.cmd)
		if err != nil {
			continue
		}

		cmd := exec.Command(path, cand.args...)
		cmd.Stdin = bytes.NewBufferString(text)
		cmd.Stdout = nil
		cmd.Stderr = nil

		if err := cmd.Run(); err != nil {
			// best effort: try next candidate
			continue
		}
		return true, nil
	}

	return false, nil
}
