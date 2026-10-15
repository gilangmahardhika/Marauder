# RFC-0055: Refine from inside the field

Status: Accepted for implementation. Date: 2026-10-15.

This RFC adds decision `RFC-123` under REQ-083. It supersedes one statement of RFC-087, the place and look of the refine buttons. The request, the stream, and the save rules of RFC-087 are unchanged.

## Decision index

| Id | Decision | Supersedes |
| --- | --- | --- |
| RFC-123 | Each refine trigger is a labelled "Refine with AI" button at the bottom right inside its text box. Each refine box says which other field the model reads, the two boxes share one "What to improve" note, and applying a refined statement offers to refine the acceptance against it | RFC-087: "show a spark icon button beside the Statement label and beside the Acceptance label" |

## RFC-123 Refine from inside the field

### Decision

- In Write, the Statement and Acceptance text boxes each show a "Refine with AI" button with a sparkles icon at their bottom right, inside the box. The box keeps room below its text so typing never runs under the button. Its accessible name stays "Refine statement with AI" or "Refine acceptance with AI", and it opens the same refine box under its field.
- Each refine box names what the model reads besides the field it refines: the statement box says how many acceptance lines it reads, or that there are none yet; the acceptance box says how many words of the statement it reads. This is the context RFC-087 already sends.
- The statement box and the acceptance box edit one "What to improve" note. Text typed in one shows in the other.
- When a refined statement is applied, the form offers "Refine acceptance to match", or "Draft acceptance from it" when there are no acceptance lines. Choosing it opens the acceptance box and starts an acceptance refine at once. The offer goes away when it is used, dismissed, when the acceptance box is opened by hand, or when the statement is edited.
- The offer sends the same acceptance request as RFC-087. The API does not change.

### Why

An icon beside a label is easy to miss and does not say what it does. A labelled button where the member is typing does. The two fields already inform each other in the request; saying so, and offering the next step after a statement changes, keeps the acceptance in step with the statement without a combined request.

### Set aside

- Refining the statement and acceptance in one request (RFC-087 still sets this aside).
- Drafting a statement from acceptance lines when the statement is empty.
