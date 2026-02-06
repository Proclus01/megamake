package adapters

import (
	"crypto/rand"
	"fmt"
	"sync"
	"time"

	"github.com/megamake/megamake/internal/domains/chat/ports"
)

// MemoryJobQueue is a simple concurrency-safe in-memory job registry.
// It is intentionally minimal for v1, but supports cancel so the server/UI can stop work.
type MemoryJobQueue struct {
	mu   sync.Mutex
	jobs map[string]ports.JobState
}

func NewMemoryJobQueue() *MemoryJobQueue {
	return &MemoryJobQueue{
		jobs: map[string]ports.JobState{},
	}
}

func (q *MemoryJobQueue) Create(req ports.CreateJobRequest) (jobID string, state ports.JobState, err error) {
	if q == nil {
		return "", ports.JobState{}, fmt.Errorf("jobqueue: nil receiver")
	}

	now := time.Now().UTC()
	id := newJobID(now)

	msg := req.Message
	if msg == "" {
		msg = "queued"
	}

	st := ports.JobState{
		JobID:     id,
		Status:    ports.JobQueued,
		Percent:   0,
		Message:   msg,
		RunName:   req.RunName,
		Turn:      req.Turn,
		Error:     "",
		CreatedAt: now,
		UpdatedAt: now,
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	// Extremely unlikely, but avoid collision.
	if _, exists := q.jobs[id]; exists {
		id = newJobID(now.Add(1 * time.Nanosecond))
		st.JobID = id
	}

	q.jobs[id] = st
	return id, st, nil
}

func (q *MemoryJobQueue) Get(jobID string) (state ports.JobState, ok bool) {
	if q == nil {
		return ports.JobState{}, false
	}
	jobID = stringsTrim(jobID)
	if jobID == "" {
		return ports.JobState{}, false
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	st, ok := q.jobs[jobID]
	return st, ok
}

func (q *MemoryJobQueue) Update(req ports.UpdateJobRequest) (state ports.JobState, ok bool) {
	if q == nil {
		return ports.JobState{}, false
	}
	id := stringsTrim(req.JobID)
	if id == "" {
		return ports.JobState{}, false
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	st, ok := q.jobs[id]
	if !ok {
		return ports.JobState{}, false
	}

	// Do not mutate terminal states unless explicitly changing to JobCanceled.
	if st.Status == ports.JobDone || st.Status == ports.JobError || st.Status == ports.JobCanceled {
		// Allow a no-op update to return current state.
		q.jobs[id] = st
		return st, true
	}

	if req.Status != "" {
		st.Status = req.Status
	}
	if req.Percent != nil {
		p := *req.Percent
		if p < 0 {
			p = 0
		}
		if p > 100 {
			p = 100
		}
		st.Percent = p
	}
	if req.Message != nil {
		st.Message = *req.Message
	}
	if req.Error != nil {
		st.Error = *req.Error
	}
	if !req.UpdatedAt.IsZero() {
		st.UpdatedAt = req.UpdatedAt.UTC()
	} else {
		st.UpdatedAt = time.Now().UTC()
	}

	q.jobs[id] = st
	return st, true
}

func (q *MemoryJobQueue) Cancel(jobID string) (state ports.JobState, ok bool) {
	if q == nil {
		return ports.JobState{}, false
	}
	id := stringsTrim(jobID)
	if id == "" {
		return ports.JobState{}, false
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	st, ok := q.jobs[id]
	if !ok {
		return ports.JobState{}, false
	}

	// If already terminal, return as-is.
	if st.Status == ports.JobDone || st.Status == ports.JobError || st.Status == ports.JobCanceled {
		return st, true
	}

	now := time.Now().UTC()
	st.Status = ports.JobCanceled
	st.Message = "canceled"
	st.Error = ""
	st.Percent = 100
	st.UpdatedAt = now

	q.jobs[id] = st
	return st, true
}

func newJobID(now time.Time) string {
	// Example: job-20260206_153012Z-8e3af90d
	ts := now.UTC().Format("20060102_150405Z")

	var b [4]byte
	_, _ = rand.Read(b[:])

	const hexdigits = "0123456789abcdef"
	hex := make([]byte, 8)
	hex[0] = hexdigits[(b[0]>>4)&0xF]
	hex[1] = hexdigits[b[0]&0xF]
	hex[2] = hexdigits[(b[1]>>4)&0xF]
	hex[3] = hexdigits[b[1]&0xF]
	hex[4] = hexdigits[(b[2]>>4)&0xF]
	hex[5] = hexdigits[b[2]&0xF]
	hex[6] = hexdigits[(b[3]>>4)&0xF]
	hex[7] = hexdigits[b[3]&0xF]

	return "job-" + ts + "-" + string(hex)
}

func stringsTrim(s string) string {
	start := 0
	end := len(s)
	for start < end {
		c := s[start]
		if c != ' ' && c != '\n' && c != '\r' && c != '\t' {
			break
		}
		start++
	}
	for end > start {
		c := s[end-1]
		if c != ' ' && c != '\n' && c != '\r' && c != '\t' {
			break
		}
		end--
	}
	return s[start:end]
}
