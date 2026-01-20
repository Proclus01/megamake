package app

import (
	"path/filepath"

	"github.com/megamake/megamake/internal/contracts/v1/project"
	"github.com/megamake/megamake/internal/domains/repo/ports"
)

type Service struct {
	det ports.Detector
	scn ports.Scanner
	rdr ports.Reader
}

func NewService(det ports.Detector, scn ports.Scanner, rdr ports.Reader) *Service {
	return &Service{
		det: det,
		scn: scn,
		rdr: rdr,
	}
}

func (s *Service) Detect(rootPath string) (project.ProjectProfileV1, error) {
	return s.det.Detect(rootPath)
}

type ScanOptions struct {
	MaxFileBytes int64
	IgnoreNames  []string
	IgnoreGlobs  []string
}

func (s *Service) Scan(rootPath string, profile project.ProjectProfileV1, opts ScanOptions) ([]project.FileRefV1, error) {
	return s.scn.Scan(ports.ScanRequest{
		RootPath:     rootPath,
		Profile:      profile,
		MaxFileBytes: opts.MaxFileBytes,
		IgnoreNames:  opts.IgnoreNames,
		IgnoreGlobs:  opts.IgnoreGlobs,
	})
}

func (s *Service) ReadFileRel(rootPath string, relPath string, maxBytes int64) ([]byte, error) {
	// relPath is POSIX-style in contracts; convert to OS path at the edge.
	abs := filepath.Join(rootPath, filepath.FromSlash(relPath))
	return s.rdr.ReadFile(abs, maxBytes)
}
