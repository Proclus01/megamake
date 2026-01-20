package project

// FileRefV1 is a stable reference to a repo file using a normalized POSIX relative path.
type FileRefV1 struct {
	RelPath   string `json:"relPath"`   // POSIX style (e.g., "src/main.go")
	SizeBytes int64  `json:"sizeBytes"` // best-effort size from filesystem
	IsTest    bool   `json:"isTest"`
}
