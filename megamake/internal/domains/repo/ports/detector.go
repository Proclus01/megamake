package ports

import project "github.com/megamake/megamake/internal/contracts/v1/project"

type Detector interface {
	Detect(rootPath string) (project.ProjectProfileV1, error)
}
