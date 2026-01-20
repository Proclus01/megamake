package wiring

import (
	repoapi "github.com/megamake/megamake/internal/domains/repo/api"
	repoadapters "github.com/megamake/megamake/internal/domains/repo/adapters"

	promptapi "github.com/megamake/megamake/internal/domains/prompt/api"
	promptadapters "github.com/megamake/megamake/internal/domains/prompt/adapters"

	docapi "github.com/megamake/megamake/internal/domains/doc/api"
	docadapters "github.com/megamake/megamake/internal/domains/doc/adapters"

	diagapi "github.com/megamake/megamake/internal/domains/diagnose/api"
	diagadapters "github.com/megamake/megamake/internal/domains/diagnose/adapters"

	tpapi "github.com/megamake/megamake/internal/domains/testplan/api"
	tpadapters "github.com/megamake/megamake/internal/domains/testplan/adapters"

	artifactwriter "github.com/megamake/megamake/internal/platform/artifact"
	"github.com/megamake/megamake/internal/platform/clock"
)

// Container is the in-process DI container for patch0..patch6.
type Container struct {
	Clock          clock.Clock
	ArtifactWriter artifactwriter.Writer

	Repo     repoapi.API
	Prompt   promptapi.API
	Doc      docapi.API
	Diagnose diagapi.API
	TestPlan tpapi.API
}

func New() Container {
	clk := clock.SystemUTC{}

	// Repo domain adapters (OS-backed)
	det := repoadapters.NewOSDetector()
	scn := repoadapters.NewOSScanner()
	rdr := repoadapters.NewOSReader()

	repo := repoapi.New(repoapi.Dependencies{
		Detector: det,
		Scanner:  scn,
		Reader:   rdr,
	})

	// Shared platform artifact writer
	aw := artifactwriter.Writer{Clock: clk}

	// Prompt
	promptArtifact := promptadapters.NewPlatformArtifactWriter(aw)
	clipboard := promptadapters.NewOSClipboard()
	prompt := promptapi.New(promptapi.Dependencies{
		Clock:          clk,
		Repo:           repo,
		ArtifactWriter: promptArtifact,
		Clipboard:      clipboard,
	})

	// Doc
	docArtifact := docadapters.NewPlatformArtifactWriter(aw)
	doc := docapi.New(docapi.Dependencies{
		Clock:          clk,
		Repo:           repo,
		ArtifactWriter: docArtifact,
	})

	// Diagnose
	diagArtifact := diagadapters.NewPlatformArtifactWriter(aw)
	execPort := diagadapters.NewPlatformExec()
	diagnose := diagapi.New(diagapi.Dependencies{
		Clock:          clk,
		Repo:           repo,
		ArtifactWriter: diagArtifact,
		Exec:           execPort,
	})

	// Test plan
	tpArtifact := tpadapters.NewPlatformArtifactWriter(aw)
	tpGit := tpadapters.NewPlatformGit()
	testPlan := tpapi.New(tpapi.Dependencies{
		Clock:          clk,
		Repo:           repo,
		ArtifactWriter: tpArtifact,
		Git:            tpGit,
	})

	return Container{
		Clock:          clk,
		ArtifactWriter: aw,
		Repo:           repo,
		Prompt:         prompt,
		Doc:            doc,
		Diagnose:       diagnose,
		TestPlan:       testPlan,
	}
}
