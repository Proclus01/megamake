package ports

import (
	contractchat "github.com/megamake/megamake/internal/contracts/v1/chat"
)

// RunStore is the persistence port for the chat domain.
//
// It is responsible for storing chat runs under:
//
//	<artifactDir>/MEGACHAT/runs/<run_name>/...
//
// The app service owns orchestration and business rules; the store owns
// filesystem layout, atomic writes, and append-only transcript behavior.
type RunStore interface {
	CreateRun(req CreateRunRequest) error
	ListRuns(req ListRunsRequest) ([]contractchat.MetaV1, error)

	ReadMeta(req ReadMetaRequest) (contractchat.MetaV1, error)
	WriteMeta(req WriteMetaRequest) error

	AppendTranscriptEvent(req AppendTranscriptEventRequest) error
	ReadTranscriptTail(req ReadTranscriptTailRequest) ([]contractchat.TranscriptEventV1, error)

	// Turn helpers (used by send/stream/job flows).
	NextTurnNumber(req NextTurnNumberRequest) (int, error)
	WriteUserTurnText(req WriteUserTurnTextRequest) error
	WriteAssistantPartialText(req WriteAssistantPartialTextRequest) error
	WriteAssistantFinalText(req WriteAssistantFinalTextRequest) error
	WriteTurnMetrics(req WriteTurnMetricsRequest) error
}

type CreateRunRequest struct {
	ArtifactDir string
	RunName     string

	Args             contractchat.ArgsV1
	Meta             contractchat.MetaV1
	SettingsSnapshot *contractchat.SettingsV1 // optional; if nil, store may omit settings.json
}

type ListRunsRequest struct {
	ArtifactDir string
	Limit       int // if <= 0, store returns a reasonable default (e.g., 200)
}

type ReadMetaRequest struct {
	ArtifactDir string
	RunName     string
}

type WriteMetaRequest struct {
	ArtifactDir string
	RunName     string
	Meta        contractchat.MetaV1
}

type AppendTranscriptEventRequest struct {
	ArtifactDir string
	RunName     string
	Event       contractchat.TranscriptEventV1
}

type ReadTranscriptTailRequest struct {
	ArtifactDir string
	RunName     string
	Limit       int // number of events; if <= 0, store chooses a default (e.g., 200)
}

type NextTurnNumberRequest struct {
	ArtifactDir string
	RunName     string
}

type WriteUserTurnTextRequest struct {
	ArtifactDir string
	RunName     string
	Turn        int
	Text        string
}

type WriteAssistantPartialTextRequest struct {
	ArtifactDir string
	RunName     string
	Turn        int
	Text        string
}

type WriteAssistantFinalTextRequest struct {
	ArtifactDir string
	RunName     string
	Turn        int
	Text        string
}

type WriteTurnMetricsRequest struct {
	ArtifactDir string
	RunName     string
	Metrics     contractchat.TurnMetricsV1
}
