package api

import (
	"github.com/megamake/megamake/internal/contracts/v1/project"
	"github.com/megamake/megamake/internal/domains/repo/app"
	"github.com/megamake/megamake/internal/domains/repo/ports"
)

// API is the stable boundary for repo detection/scanning/reading.
type API interface {
	Detect(rootPath string) (project.ProjectProfileV1, error)
	Scan(rootPath string, profile project.ProjectProfileV1, opts ScanOptions) ([]project.FileRefV1, error)
	ReadFileRel(rootPath string, relPath string, maxBytes int64) ([]byte, error)
}

// Dependencies are the OS adapters (or mocks) injected by the composition root.
type Dependencies struct {
	Detector ports.Detector
	Scanner  ports.Scanner
	Reader   ports.Reader
}

// ScanOptions are CLI-facing scan options.
type ScanOptions struct {
	MaxFileBytes int64
	IgnoreNames  []string
	IgnoreGlobs  []string
}

func New(deps Dependencies) API {
	svc := app.NewService(deps.Detector, deps.Scanner, deps.Reader)
	return &repoAPI{
		svc: svc,
	}
}

type repoAPI struct {
	svc *app.Service
}

func (r *repoAPI) Detect(rootPath string) (project.ProjectProfileV1, error) {
	return r.svc.Detect(rootPath)
}

func (r *repoAPI) Scan(rootPath string, profile project.ProjectProfileV1, opts ScanOptions) ([]project.FileRefV1, error) {
	return r.svc.Scan(rootPath, profile, app.ScanOptions{
		MaxFileBytes: opts.MaxFileBytes,
		IgnoreNames:  opts.IgnoreNames,
		IgnoreGlobs:  opts.IgnoreGlobs,
	})
}

func (r *repoAPI) ReadFileRel(rootPath string, relPath string, maxBytes int64) ([]byte, error) {
	return r.svc.ReadFileRel(rootPath, relPath, maxBytes)
}
