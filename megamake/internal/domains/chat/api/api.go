package api

import (
	chatapp "github.com/megamake/megamake/internal/domains/chat/app"
	"github.com/megamake/megamake/internal/domains/chat/ports"
	"github.com/megamake/megamake/internal/platform/clock"
)

// API is the stable boundary for the chat domain.
// CLI and server handlers should call into this interface.
type API interface {
	NewRun(req chatapp.NewRunRequest) (chatapp.NewRunResult, error)
	ListRuns(req chatapp.ListRunsRequest) (chatapp.ListRunsResult, error)
	GetRun(req chatapp.GetRunRequest) (chatapp.GetRunResult, error)

	ConfigGet(req chatapp.ConfigGetRequest) (chatapp.ConfigGetResult, error)
	ConfigSet(req chatapp.ConfigSetRequest) (chatapp.ConfigSetResult, error)

	// Jobs (async + tail polling)
	RunAsync(req chatapp.RunAsyncRequest) (chatapp.RunAsyncResult, error)
	JobStatus(req chatapp.JobStatusRequest) (chatapp.JobStatusResult, error)
	JobTail(req chatapp.JobTailRequest) (chatapp.JobTailResult, error)
	CancelJob(req chatapp.CancelJobRequest) (chatapp.CancelJobResult, error)

	// Providers
	VerifyProvider(req chatapp.VerifyProviderRequest) (chatapp.VerifyProviderResult, error)
	ListModels(req chatapp.ListModelsRequest) (chatapp.ListModelsResult, error)

	// Per-run settings
	GetRunSettings(req chatapp.GetRunSettingsRequest) (chatapp.GetRunSettingsResult, error)
	SetRunSettings(req chatapp.SetRunSettingsRequest) (chatapp.SetRunSettingsResult, error)
}

// Dependencies are injected by wiring.Container.
type Dependencies struct {
	Clock    clock.Clock
	Store    ports.RunStore
	Settings ports.SettingsStore
	Env      ports.EnvLoader

	Jobs         ports.JobQueue
	TokenCounter ports.TokenCounter

	Providers  ports.ProviderRegistry
	ModelCache ports.ModelCache

	RunSettings ports.RunSettingsStore
}

func New(deps Dependencies) API {
	return &chatAPI{
		svc: &chatapp.Service{
			Clock:        deps.Clock,
			Store:        deps.Store,
			Settings:     deps.Settings,
			Env:          deps.Env,
			Jobs:         deps.Jobs,
			TokenCounter: deps.TokenCounter,
			Providers:    deps.Providers,
			ModelCache:   deps.ModelCache,
			RunSettings:  deps.RunSettings,
		},
	}
}

type chatAPI struct {
	svc *chatapp.Service
}

func (c *chatAPI) NewRun(req chatapp.NewRunRequest) (chatapp.NewRunResult, error) {
	return c.svc.NewRun(req)
}

func (c *chatAPI) ListRuns(req chatapp.ListRunsRequest) (chatapp.ListRunsResult, error) {
	return c.svc.ListRuns(req)
}

func (c *chatAPI) GetRun(req chatapp.GetRunRequest) (chatapp.GetRunResult, error) {
	return c.svc.GetRun(req)
}

func (c *chatAPI) ConfigGet(req chatapp.ConfigGetRequest) (chatapp.ConfigGetResult, error) {
	return c.svc.ConfigGet(req)
}

func (c *chatAPI) ConfigSet(req chatapp.ConfigSetRequest) (chatapp.ConfigSetResult, error) {
	return c.svc.ConfigSet(req)
}

func (c *chatAPI) RunAsync(req chatapp.RunAsyncRequest) (chatapp.RunAsyncResult, error) {
	return c.svc.RunAsync(req)
}

func (c *chatAPI) JobStatus(req chatapp.JobStatusRequest) (chatapp.JobStatusResult, error) {
	return c.svc.JobStatus(req)
}

func (c *chatAPI) JobTail(req chatapp.JobTailRequest) (chatapp.JobTailResult, error) {
	return c.svc.JobTail(req)
}

func (c *chatAPI) CancelJob(req chatapp.CancelJobRequest) (chatapp.CancelJobResult, error) {
	return c.svc.CancelJob(req)
}

func (c *chatAPI) VerifyProvider(req chatapp.VerifyProviderRequest) (chatapp.VerifyProviderResult, error) {
	return c.svc.VerifyProvider(req)
}

func (c *chatAPI) ListModels(req chatapp.ListModelsRequest) (chatapp.ListModelsResult, error) {
	return c.svc.ListModels(req)
}

func (c *chatAPI) GetRunSettings(req chatapp.GetRunSettingsRequest) (chatapp.GetRunSettingsResult, error) {
	return c.svc.GetRunSettings(req)
}

func (c *chatAPI) SetRunSettings(req chatapp.SetRunSettingsRequest) (chatapp.SetRunSettingsResult, error) {
	return c.svc.SetRunSettings(req)
}
