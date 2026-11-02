package event

import (
	"fmt"
	"sort"
	"sync/atomic"
	"time"
)

// jobRun records when the running run of a background job started (Unix
// nanoseconds, 0 while the job is idle).
type jobRun struct {
	started atomic.Int64
	limit   time.Duration
}

// The outbox jobs of this process and how long one run may take. A run of
// publishing is at most 200 NATS publishes, one of anchoring at most 100
// events with an IPFS write and a timestamp each. A run that lasts longer is
// stuck on a call that does not return (DCS-NFR-SF-01, DCS-NFR-PER-03).
var jobRuns = map[string]*jobRun{
	"outbox-publishing":       {limit: 5 * time.Minute},
	"outbox-anchoring":        {limit: 60 * time.Minute},
	"checkpoint-timestamping": {limit: 30 * time.Minute},
}

// trackRun runs fn as a run of the named job, so StuckJobs sees it while it
// runs.
func trackRun(name string, fn func() error) error {
	run := jobRuns[name]
	run.started.Store(time.Now().UnixNano())
	defer run.started.Store(0)
	return fn()
}

// StuckJobs returns the outbox jobs whose current run has lasted longer than
// its limit. The liveness endpoint reports them, so Kubernetes restarts the
// process instead of leaving the outbox undelivered behind a hung run.
func StuckJobs(now time.Time) []string {
	var stuck []string
	for name, run := range jobRuns {
		started := run.started.Load()
		if started != 0 && now.Sub(time.Unix(0, started)) > run.limit {
			stuck = append(stuck, fmt.Sprintf("%s (running since %s)", name, time.Unix(0, started).UTC().Format(time.RFC3339)))
		}
	}
	sort.Strings(stuck)
	return stuck
}
