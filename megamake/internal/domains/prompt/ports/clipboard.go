package ports

// Clipboard supports best-effort copy-to-clipboard.
// Implementations must not fail the command if clipboard is unavailable.
type Clipboard interface {
	Copy(text string) (copied bool, err error)
}
