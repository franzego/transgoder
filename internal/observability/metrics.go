package observability

import (
	"sync/atomic"
	"time"
)

type Metrics struct {
	requestsTotal    atomic.Int64
	requestsErrored  atomic.Int64
	activeRequests   atomic.Int64
	transcodesTotal  atomic.Int64
	transcodeFailed  atomic.Int64
	transcodeMsTotal atomic.Int64
}

type Snapshot struct {
	RequestsTotal    int64 `json:"requests_total"`
	RequestsErrored  int64 `json:"requests_errored"`
	ActiveRequests   int64 `json:"active_requests"`
	TranscodesTotal  int64 `json:"transcodes_total"`
	TranscodesFailed int64 `json:"transcodes_failed"`
	AvgTranscodeMs   int64 `json:"avg_transcode_ms"`
}

func New() *Metrics {
	return &Metrics{}
}

func (m *Metrics) RequestStarted() {
	m.requestsTotal.Add(1)
	m.activeRequests.Add(1)
}

func (m *Metrics) RequestFinished(statusCode int) {
	m.activeRequests.Add(-1)
	if statusCode >= 500 {
		m.requestsErrored.Add(1)
	}
}

func (m *Metrics) TranscodeDone(duration time.Duration, failed bool) {
	m.transcodesTotal.Add(1)
	m.transcodeMsTotal.Add(duration.Milliseconds())
	if failed {
		m.transcodeFailed.Add(1)
	}
}

func (m *Metrics) Snapshot() Snapshot {
	total := m.transcodesTotal.Load()
	avg := int64(0)
	if total > 0 {
		avg = m.transcodeMsTotal.Load() / total
	}
	return Snapshot{
		RequestsTotal:    m.requestsTotal.Load(),
		RequestsErrored:  m.requestsErrored.Load(),
		ActiveRequests:   m.activeRequests.Load(),
		TranscodesTotal:  total,
		TranscodesFailed: m.transcodeFailed.Load(),
		AvgTranscodeMs:   avg,
	}
}
