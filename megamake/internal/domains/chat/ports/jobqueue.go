package ports

import "time"

// JobStatus is the lifecycle state of an async chat job.
type JobStatus string

const (
	JobQueued   JobStatus = "queued"
	JobRunning  JobStatus = "running"
	JobDone     JobStatus = "done"
	JobError    JobStatus = "error"
	JobCanceled JobStatus = "canceled"
)

// JobState is the current state of an async job.
//
// This is an in-memory coordination structure (not a long-term persisted artifact).
// The canonical output for tailing is written to the run folder
// (assistant_turn_###.partial.txt / assistant_turn_###.txt).
type JobState struct {
	JobID   string    `json:"job_id"`
	Status  JobStatus `json:"status"`
	Percent int       `json:"percent"` // 0..100 best-effort
	Message string    `json:"message"` // short human-readable status

	// Populated for correlation:
	RunName string `json:"run_name"`
	Turn    int    `json:"turn"`

	Error string `json:"error,omitempty"` // filled when Status==JobError

	CreatedAt time.Time `json:"created_ts"`
	UpdatedAt time.Time `json:"updated_ts"`
}

// JobQueue manages async jobs (in memory for v1).
// Implementations must be concurrency-safe.
type JobQueue interface {
	Create(req CreateJobRequest) (jobID string, state JobState, err error)
	Get(jobID string) (state JobState, ok bool)
	Update(req UpdateJobRequest) (state JobState, ok bool)

	// Cancel requests cancellation of a job.
	// If the job is already done/error/canceled, it returns the existing state.
	Cancel(jobID string) (state JobState, ok bool)
}

type CreateJobRequest struct {
	RunName string
	Turn    int

	// Optional initial message; if empty, implementation may set a default.
	Message string
}

type UpdateJobRequest struct {
	JobID string

	// Any field may be left at its zero value to mean "no change"
	// (except JobID which is required).
	Status  JobStatus
	Percent *int
	Message *string
	Error   *string

	UpdatedAt time.Time // if zero, implementation uses time.Now().UTC()
}
