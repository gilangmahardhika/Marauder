package machine

var edges = map[[2]string]struct{}{
	{"backlog", "ready"}:                  {},
	{"ready", "in_progress"}:              {},
	{"in_progress", "self_check"}:         {},
	{"in_progress", "blocked"}:            {},
	{"self_check", "in_progress"}:         {},
	{"self_check", "pr_open"}:             {},
	{"self_check", "blocked"}:             {},
	{"pr_open", "agent_review"}:           {},
	{"agent_review", "changes_requested"}: {},
	{"agent_review", "human_review"}:      {},
	{"agent_review", "blocked"}:           {},
	{"agent_review", "pr_open"}:           {},
	{"changes_requested", "in_progress"}:  {},
	{"changes_requested", "blocked"}:      {},
	{"human_review", "changes_requested"}: {},
	{"human_review", "approved"}:          {},
	{"human_review", "blocked"}:           {},
	{"approved", "deploying_dev"}:         {},
	{"approved", "human_review"}:          {},
	{"approved", "changes_requested"}:     {},
	{"approved", "blocked"}:               {},
	{"deploying_dev", "verifying"}:        {},
	{"deploying_dev", "blocked"}:          {},
	{"verifying", "done"}:                 {},
	{"verifying", "changes_requested"}:    {},
	{"verifying", "blocked"}:              {},
	{"blocked", "ready"}:                  {},
	{"blocked", "human_review"}:           {},
}

func Edges() map[[2]string]struct{} {
	out := make(map[[2]string]struct{}, len(edges))
	for pair := range edges {
		out[pair] = struct{}{}
	}
	return out
}
