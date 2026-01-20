package ports

import project "github.com/megamake/megamake/internal/contracts/v1/project"

type ScanRequest struct {
	RootPath     string
	Profile      project.ProjectProfileV1
	MaxFileBytes int64
	IgnoreNames  []string
	IgnoreGlobs  []string
}

type Scanner interface {
	Scan(req ScanRequest) ([]project.FileRefV1, error)
}
