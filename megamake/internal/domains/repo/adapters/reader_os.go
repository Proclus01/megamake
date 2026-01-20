package adapters

import (
	"io"
	"os"

	"github.com/megamake/megamake/internal/platform/errors"
)

type OSReader struct{}

func NewOSReader() OSReader {
	return OSReader{}
}

// ReadFile reads up to maxBytes bytes from a file path.
// If maxBytes <= 0, it reads the full file.
func (r OSReader) ReadFile(absPath string, maxBytes int64) ([]byte, error) {
	f, err := os.Open(absPath)
	if err != nil {
		return nil, errors.New(errors.KindIO, "failed to open file", err)
	}
	defer f.Close()

	var reader io.Reader = f
	if maxBytes > 0 {
		// Read maxBytes+1 so we can detect truncation cleanly.
		reader = io.LimitReader(f, maxBytes+1)
	}

	b, err := io.ReadAll(reader)
	if err != nil {
		return nil, errors.New(errors.KindIO, "failed to read file", err)
	}
	if maxBytes > 0 && int64(len(b)) > maxBytes {
		return nil, errors.New(errors.KindIO, "file exceeds maxBytes limit", nil)
	}
	return b, nil
}
