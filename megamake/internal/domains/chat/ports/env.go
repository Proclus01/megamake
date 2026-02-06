package ports

// EnvLoader loads environment variables from a dotenv-style file.
//
// This is used for provider API keys and other local configuration.
// It must NOT write secrets into artifacts.
//
// The chat app/service will decide the default path (typically):
//
//	<artifactDir>/MEGACHAT/.env
//
// and allow override via --env-file.
type EnvLoader interface {
	Load(req LoadEnvRequest) (LoadEnvResult, error)
}

type LoadEnvRequest struct {
	// Path is the dotenv file to load. If empty, the implementation may return Loaded=false.
	Path string

	// Overwrite controls whether loaded keys overwrite already-present env vars.
	// Recommended: false by default (respect existing env).
	Overwrite bool
}

type LoadEnvResult struct {
	Loaded   bool
	KeysSet  []string
	Warnings []string
}
