package api

import (
	repoapi "github.com/megamake/megamake/internal/domains/repo/api"
	promptapp "github.com/megamake/megamake/internal/domains/prompt/app"
	promptports "github.com/megamake/megamake/internal/domains/prompt/ports"
	"github.com/megamake/megamake/internal/platform/clock"
)

type API interface {
	Generate(req promptapp.GenerateRequest) (promptapp.GenerateResult, error)
}

type Dependencies struct {
	Clock          clock.Clock
	Repo           repoapi.API
	ArtifactWriter promptports.ArtifactWriter
	Clipboard      promptports.Clipboard
}

func New(deps Dependencies) API {
	return &promptAPI{
		svc: &promptapp.Service{
			Clock:          deps.Clock,
			Repo:           deps.Repo,
			ArtifactWriter: deps.ArtifactWriter,
			Clipboard:      deps.Clipboard,
		},
	}
}

type promptAPI struct {
	svc *promptapp.Service
}

func (p *promptAPI) Generate(req promptapp.GenerateRequest) (promptapp.GenerateResult, error) {
	return p.svc.Generate(req)
}
