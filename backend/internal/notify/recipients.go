package notify

import (
	"context"
	"database/sql"
	"encoding/json"

	"orchestrator/backend/internal/app"
	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/settings"
)

// Note is one person to notify of an event. Kind and the event id make it
// unique; Push says whether it is also sent as a browser notification.
type Note struct {
	UserID string
	Kind   string
	Title  string
	Push   bool
}

// Roles that may act on a card, as the API allows them.
var (
	approveRoles = map[string]bool{"lead_developer": true, "qa_lead": true, "product_owner": true}
	retryRoles   = map[string]bool{"lead_developer": true, "senior_developer": true, "developer": true, "qa": true}
	shipRoles    = map[string]bool{"lead_developer": true}
	// deployReasons block a card a Lead Developer unblocks back to review.
	deployReasons = map[string]bool{"deploy_failed": true, "health": true, "deploy_config": true}
)

// reasonLabels name the block reasons a title may show. Any other reason is
// left out, since a free-text reason may carry details.
var reasonLabels = map[string]string{
	"source_changed":   "Requirement changed",
	"attempt_cap":      "Out of attempts",
	"rejected":         "Rejected",
	"deploy_failed":    "Deploy failed",
	"deploy_config":    "Deploy not set up",
	"github_config":    "GitHub not set up",
	"no_commit":        "No commit",
	"assertion_failed": "Dev check failed",
	"ci":               "CI failed",
	"health":           "Dev health check failed",
}

type transitionPayload struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// Recipients lists who to notify of ev on card, per the RFC-113 table. The
// actor, people outside the project's tribe, and disabled users are left out,
// and each person appears once.
func Recipients(ctx context.Context, db *sql.DB, data string, ev Event, card board.Card) ([]Note, error) {
	members, err := projectMembers(ctx, db, card.ProjectID)
	if err != nil {
		return nil, err
	}
	var notes []Note
	add := func(userID, kind, title string, push bool) {
		notes = append(notes, Note{UserID: userID, Kind: kind, Title: title, Push: push})
	}
	withRole := func(roles map[string]bool, kind, title string, push bool) {
		for id, role := range members {
			if roles[role] {
				add(id, kind, title, push)
			}
		}
	}
	owner := card.OwnerID
	solo := soloApproval(data, card.ProjectID)

	switch ev.Kind {
	case "review_failed":
		add(owner, "review_failed", "AI review gave no verdict", true)
		withRole(retryRoles, "review_failed", "AI review gave no verdict", true)
	case "transition":
		var t transitionPayload
		if err := json.Unmarshal([]byte(ev.Payload), &t); err != nil {
			return nil, nil
		}
		switch t.To {
		case "human_review":
			reviewers, err := app.CardReviewerUsers(ctx, db, card.ProjectID, card.ID)
			if err != nil {
				return nil, err
			}
			for _, r := range reviewers[card.ID] {
				add(r.ID, "review", "Review needed", true)
			}
			if len(reviewers[card.ID]) == 0 {
				for id, role := range members {
					if approveRoles[role] && id != owner {
						add(id, "review", "Review needed", true)
					}
				}
			}
			self, err := app.CardSelfReview(ctx, db, card)
			if err != nil {
				return nil, err
			}
			if (solo || self) && approveRoles[members[owner]] {
				add(owner, "review", "Review needed", true)
			}
		case "approved":
			title := "Ready to deploy"
			if !app.DeployEnabled(data, card.ProjectID) {
				title = "Ready to merge"
				if solo {
					add(owner, "ship", title, true)
				}
			}
			withRole(shipRoles, "ship", title, true)
			add(owner, "approved", "Approved", false)
		case "changes_requested":
			add(owner, "changes_requested", "Changes requested", true)
		case "blocked":
			if t.From == "wont_fix" {
				add(owner, "reopened", "Reopened", false)
				break
			}
			reason := ""
			if card.Status == "blocked" {
				reason = card.BlockedReason
			}
			if reason == "owner_github" {
				add(owner, "owner_github", "Connect your GitHub account", true)
				break
			}
			title := "Blocked"
			if label := reasonLabels[reason]; label != "" {
				title += ": " + label
			}
			add(owner, "blocked", title, true)
			if deployReasons[reason] {
				withRole(shipRoles, "blocked", title, true)
			}
		case "done":
			add(owner, "done", "Done", false)
		case "wont_fix":
			add(owner, "wont_fix", "Closed as won't fix", false)
		}
	}

	out := notes[:0]
	seen := map[string]bool{}
	for _, n := range notes {
		if n.UserID == "" || n.UserID == ev.ActorID || seen[n.UserID] {
			continue
		}
		if _, ok := members[n.UserID]; !ok {
			continue
		}
		seen[n.UserID] = true
		out = append(out, n)
	}
	return out, nil
}

// projectMembers maps each enabled member of the project's tribe to their role.
func projectMembers(ctx context.Context, db *sql.DB, projectID string) (map[string]string, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT u.id, u.role FROM projects p
		JOIN tribe_members m ON m.tribe_id = p.tribe_id
		JOIN users u ON u.id = m.user_id AND u.disabled = false
		WHERE p.id = $1`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var id, role string
		if err := rows.Scan(&id, &role); err != nil {
			return nil, err
		}
		out[id] = role
	}
	return out, rows.Err()
}

func soloApproval(data, projectID string) bool {
	if data == "" {
		return false
	}
	cfg, _, err := settings.LoadProject(data, projectID)
	return err == nil && cfg.Approval.Solo
}
