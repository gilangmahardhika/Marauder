package loop

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"orchestrator/backend/internal/providers"
)

func readTurn(n int, size int) providers.Turn {
	return providers.Turn{
		Calls:   []providers.ToolCall{{ID: fmt.Sprintf("c%d", n), Name: "read_file", Args: map[string]any{"path": fmt.Sprintf("f%d.go", n)}}},
		Results: []string{strings.Repeat(string(rune('a'+n%26)), size)},
	}
}

func sentBytes(turns []providers.Turn) (kept int, omittedN int) {
	for _, turn := range turns {
		for _, r := range turn.Results {
			if r == omitted {
				omittedN++
				continue
			}
			kept += len(r)
		}
	}
	return kept, omittedN
}

func TestHistoryBudgetPerModel(t *testing.T) {
	var turns []providers.Turn
	for i := 0; i < 20; i++ {
		turns = append(turns, readTurn(i, 8<<10))
	}
	small := &session{turns: turns}
	large := &session{turns: turns, in: Input{HistoryBudget: 256 << 10}}
	smallKept, smallOmitted := sentBytes(small.history())
	largeKept, largeOmitted := sentBytes(large.history())
	if smallKept > defaultHistory || smallOmitted == 0 {
		t.Fatalf("16 KiB budget sent %d bytes, omitted %d", smallKept, smallOmitted)
	}
	if largeOmitted != 0 || largeKept != 20*(8<<10) {
		t.Fatalf("256 KiB budget sent %d bytes, omitted %d", largeKept, largeOmitted)
	}
	if resultCap(16<<10) != 12<<10 || resultCap(128<<10) != 32<<10 || resultCap(512<<10) != 64<<10 {
		t.Fatal("result cap does not follow the budget")
	}
}

func TestHistoryStableEviction(t *testing.T) {
	s := &session{in: Input{HistoryBudget: 32 << 10}}
	var previous []providers.Turn
	evictions := 0
	for i := 0; i < 30; i++ {
		s.turns = append(s.turns, readTurn(i, 4<<10))
		got := s.history()
		for t0 := range previous {
			if t0 >= len(got)-1 {
				break
			}
			if previous[t0].Results[0] == omitted && got[t0].Results[0] != omitted {
				t.Fatalf("turn %d came back after it was omitted", t0)
			}
		}
		changed := 0
		for t0 := 0; t0 < len(previous); t0++ {
			if previous[t0].Results[0] != got[t0].Results[0] {
				changed++
			}
		}
		if changed > 0 {
			evictions++
		}
		if kept, _ := sentBytes(got); kept > 32<<10 {
			t.Fatalf("request %d sent %d bytes", i, kept)
		}
		previous = got
	}
	if evictions == 0 || evictions > 10 {
		t.Fatalf("history changed on %d of 30 requests, want a few chunks", evictions)
	}
	for i := 0; i < len(previous)-1; i++ {
		if previous[i].Results[0] != omitted && previous[i+1].Results[0] == omitted && i+1 < len(previous)-1 {
			t.Fatalf("turn %d kept while the newer turn %d was omitted", i, i+1)
		}
	}
}

func TestCheckOutputKeepsTail(t *testing.T) {
	out := "exit 1\n" + strings.Repeat("noise line\n", 3000) + "FAIL: TestLogin expected 200 got 500\n"
	s := &session{turns: []providers.Turn{{
		Calls:   []providers.ToolCall{{ID: "c1", Name: "run_local_check"}, {ID: "c2", Name: "read_file", Args: map[string]any{"path": "big.txt"}}},
		Results: []string{out, "HEAD" + strings.Repeat("x", 20<<10)},
	}}}
	got := s.history()[0].Results
	if !strings.HasPrefix(got[0], "truncated: head cut\n") || !strings.HasSuffix(got[0], "FAIL: TestLogin expected 200 got 500\n") {
		t.Fatalf("check output = %.60q ... %.60q", got[0], got[0][len(got[0])-60:])
	}
	if !strings.HasPrefix(got[1], "HEAD") || !strings.HasSuffix(got[1], "truncated: true") {
		t.Fatalf("read output = %.20q ... %q", got[1], got[1][len(got[1])-20:])
	}
}

// TestHistoryPrefixStable checks that two requests in a row send the same
// earlier turns, so a cache breakpoint on the newest results keeps hitting.
// Only the newest turn's note moves, and it sits after the breakpoint. The
// prefix changes only when a chunk of old results is omitted.
func TestHistoryPrefixStable(t *testing.T) {
	s := &session{in: Input{HistoryBudget: 16 << 10, MaxTurns: 40}}
	s.todos = []todoItem{{Text: "read", Status: "in_progress"}}
	var prev []providers.Turn
	changes := 0
	for n := 0; n < 40; n++ {
		s.turns = append(s.turns, readTurn(n, 2000))
		cur := withNote(withTodos(s.history(), s.todos), 40-n, 40)
		if prev != nil {
			last := len(prev) - 1
			if !strings.Contains(prev[last].Note, "Your todo list") || strings.Contains(cur[last].Note, "Your todo list") {
				t.Fatalf("request %d: the todo list is not only on the newest turn", n+1)
			}
			earlier := make([]providers.Turn, len(prev))
			newlyOmitted := 0
			for i, turn := range prev {
				turn.Results = append([]string(nil), turn.Results...)
				for r := range turn.Results {
					if turn.Results[r] != omitted && cur[i].Results[r] == omitted {
						turn.Results[r] = omitted
						newlyOmitted++
					}
				}
				earlier[i] = turn
			}
			earlier[last].Note = ""
			if !reflect.DeepEqual(earlier, cur[:len(prev)]) {
				t.Fatalf("request %d changed an earlier turn other than by omitting results", n+1)
			}
			if newlyOmitted > 0 {
				changes++
				if newlyOmitted < 2 {
					t.Fatalf("request %d omitted %d result; eviction must go in chunks", n+1, newlyOmitted)
				}
			}
		}
		prev = cur
	}
	if changes == 0 || changes > 39/2 {
		t.Fatalf("prefix changed on %d of 39 requests", changes)
	}
}
