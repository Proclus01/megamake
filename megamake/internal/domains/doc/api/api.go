package api

import (
	docapp "github.com/megamake/megamake/internal/domains/doc/app"
	docports "github.com/megamake/megamake/internal/domains/doc/ports"
	repoapi "github.com/megamake/megamake/internal/domains/repo/api"
	"github.com/megamake/megamake/internal/platform/clock"
)

type API interface {
	Create(req docapp.CreateRequest) (docapp.CreateResult, error)
	Get(req docapp.GetRequest) (docapp.GetResult, error)
}

type Dependencies struct {
	Clock          clock.Clock
	Repo           repoapi.API
	ArtifactWriter docports.ArtifactWriter
}

func New(deps Dependencies) API {
	return &docAPI{
		svc: &docapp.Service{
			Clock:          deps.Clock,
			Repo:           deps.Repo,
			ArtifactWriter: deps.ArtifactWriter,
		},
	}
}

type docAPI struct {
	svc *docapp.Service
}

func (d *docAPI) Create(req docapp.CreateRequest) (docapp.CreateResult, error) {
	return d.svc.Create(req)
}

func (d *docAPI) Get(req docapp.GetRequest) (docapp.GetResult, error) {
	return d.svc.Get(req)
}
