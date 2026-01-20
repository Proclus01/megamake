package api

import (
	tpapp "github.com/megamake/megamake/internal/domains/testplan/app"
	tpports "github.com/megamake/megamake/internal/domains/testplan/ports"
	repoapi "github.com/megamake/megamake/internal/domains/repo/api"
	"github.com/megamake/megamake/internal/platform/clock"
)

type API interface {
	Build(req tpapp.BuildRequest) (tpapp.BuildResult, error)
}

type Dependencies struct {
	Clock          clock.Clock
	Repo           repoapi.API
	ArtifactWriter tpports.ArtifactWriter
	Git            tpports.Git
}

func New(deps Dependencies) API {
	return &testPlanAPI{
		svc: &tpapp.Service{
			Clock:          deps.Clock,
			Repo:           deps.Repo,
			ArtifactWriter: deps.ArtifactWriter,
			Git:            deps.Git,
		},
	}
}

type testPlanAPI struct {
	svc *tpapp.Service
}

func (t *testPlanAPI) Build(req tpapp.BuildRequest) (tpapp.BuildResult, error) {
	return t.svc.Build(req)
}
