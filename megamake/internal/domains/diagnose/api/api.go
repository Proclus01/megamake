package api

import (
	diagapp "github.com/megamake/megamake/internal/domains/diagnose/app"
	diagports "github.com/megamake/megamake/internal/domains/diagnose/ports"
	repoapi "github.com/megamake/megamake/internal/domains/repo/api"
	"github.com/megamake/megamake/internal/platform/clock"
)

type API interface {
	Diagnose(req diagapp.DiagnoseRequest) (diagapp.DiagnoseResult, error)
}

type Dependencies struct {
	Clock          clock.Clock
	Repo           repoapi.API
	ArtifactWriter diagports.ArtifactWriter
	Exec           diagports.Exec
}

func New(deps Dependencies) API {
	return &diagnoseAPI{
		svc: &diagapp.Service{
			Clock:          deps.Clock,
			Repo:           deps.Repo,
			ArtifactWriter: deps.ArtifactWriter,
			Exec:           deps.Exec,
		},
	}
}

type diagnoseAPI struct {
	svc *diagapp.Service
}

func (d *diagnoseAPI) Diagnose(req diagapp.DiagnoseRequest) (diagapp.DiagnoseResult, error) {
	return d.svc.Diagnose(req)
}
