package wiring

import (
	repoadapters "github.com/megamake/megamake/internal/domains/repo/adapters"
	repoapi "github.com/megamake/megamake/internal/domains/repo/api"

	promptadapters "github.com/megamake/megamake/internal/domains/prompt/adapters"
	promptapi "github.com/megamake/megamake/internal/domains/prompt/api"

	docadapters "github.com/megamake/megamake/internal/domains/doc/adapters"
	docapi "github.com/megamake/megamake/internal/domains/doc/api"

	diagadapters "github.com/megamake/megamake/internal/domains/diagnose/adapters"
	diagapi "github.com/megamake/megamake/internal/domains/diagnose/api"

	tpadapters "github.com/megamake/megamake/internal/domains/testplan/adapters"
	tpapi "github.com/megamake/megamake/internal/domains/testplan/api"

	chatadapters "github.com/megamake/megamake/internal/domains/chat/adapters"
	chatapi "github.com/megamake/megamake/internal/domains/chat/api"

	artifactwriter "github.com/megamake/megamake/internal/platform/artifact"
	"github.com/megamake/megamake/internal/platform/clock"
)

// Container is the in-process DI container for patch0..patch6 (+ chat).
type Container struct {
	Clock          clock.Clock
	ArtifactWriter artifactwriter.Writer

	Repo     repoapi.API
	Prompt   promptapi.API
	Doc      docapi.API
	Diagnose diagapi.API
	TestPlan tpapi.API

	Chat chatapi.API
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

	// Chat
	chatFS := chatadapters.NewFSAdapters()
	chat := chatapi.New(chatapi.Dependencies{
		Clock:        clk,
		Store:        chatFS.Store,
		Settings:     chatFS.Settings,
		Env:          chatFS.Env,
		Jobs:         chatFS.Jobs,
		TokenCounter: chatFS.TokenCounter,
		Providers:    chatFS.Providers,
		ModelCache:   chatFS.ModelCache,
		RunSettings:  chatFS.RunSettings,
	})

	return Container{
		Clock:          clk,
		ArtifactWriter: aw,
		Repo:           repo,
		Prompt:         prompt,
		Doc:            doc,
		Diagnose:       diagnose,
		TestPlan:       testPlan,
		Chat:           chat,
	}
}
