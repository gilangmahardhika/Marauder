package httpapi

import (
	"testing"
	"time"
)

// SetTraceStreamTiming shortens the trace stream's read, ping, and lifetime
// for one test.
func SetTraceStreamTiming(t testing.TB, poll, ping, limit time.Duration) {
	oldPoll, oldPing, oldMax := traceStreamPoll, traceStreamPing, traceStreamMax
	traceStreamPoll, traceStreamPing, traceStreamMax = poll, ping, limit
	t.Cleanup(func() { traceStreamPoll, traceStreamPing, traceStreamMax = oldPoll, oldPing, oldMax })
}
