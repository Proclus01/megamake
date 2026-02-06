package adapters

import "github.com/megamake/megamake/internal/domains/chat/ports"

// FSAdapters bundles the default adapters for the chat domain.
//
// Note: despite the name, this includes in-memory components as well
// (jobs, provider model cache), because those are coordination/cache state
// in the long-running server process.
type FSAdapters struct {
	Store       FSRunStore
	Settings    FSSettingsStore
	RunSettings FSRunSettingsStore
	Env         FSEnvLoader

	Jobs         ports.JobQueue
	TokenCounter ports.TokenCounter

	Providers  ports.ProviderRegistry
	ModelCache ports.ModelCache
}

func NewFSAdapters() FSAdapters {
	return FSAdapters{
		Store:       NewFSRunStore(),
		Settings:    NewFSSettingsStore(),
		RunSettings: NewFSRunSettingsStore(),
		Env:         NewFSEnvLoader(),

		Jobs:         NewMemoryJobQueue(),
		TokenCounter: NewHeuristicTokenCounter(),

		Providers:  NewProviderRegistry(),
		ModelCache: NewMemoryModelCache(),
	}
}
