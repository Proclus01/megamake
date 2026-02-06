package app

import (
	"fmt"
	"strings"

	"github.com/megamake/megamake/internal/domains/chat/ports"
)

type CancelJobRequest struct {
	JobID string
}

type CancelJobResult struct {
	Job ports.JobState `json:"job"`
}

// CancelJob requests cancellation of an async job.
//
// v1 semantics:
//   - This is best-effort cancellation. It marks the job canceled in the JobQueue.
//   - The running worker must cooperatively check job state and stop early.
//     (We'll implement the cooperative checks in the next step.)
func (s *Service) CancelJob(req CancelJobRequest) (CancelJobResult, error) {
	if s.Jobs == nil {
		return CancelJobResult{}, fmt.Errorf("internal error: chat Jobs is nil (job queue not wired)")
	}

	id := strings.TrimSpace(req.JobID)
	if id == "" {
		return CancelJobResult{}, fmt.Errorf("chat jobs cancel: job_id is required")
	}

	st, ok := s.Jobs.Cancel(id)
	if !ok {
		return CancelJobResult{}, fmt.Errorf("chat jobs cancel: unknown job_id: %s", id)
	}

	return CancelJobResult{Job: st}, nil
}
