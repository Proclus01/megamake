package ports

type Reader interface {
	// ReadFile reads a file by absolute path. The domain uses this behind a RepoAPI.
	ReadFile(absPath string, maxBytes int64) ([]byte, error)
}
