# RFC-0057: Refine as a chat

Status: Accepted for implementation. Date: 2026-10-15.

This RFC adds decision `RFC-125` under REQ-083. It supersedes the statements it names below. The request, the stream, the save rules, and the comparison of RFC-087, and the model choices of RFC-124, are unchanged.

## Decision index

| Id | Decision | Supersedes |
| --- | --- | --- |
| RFC-125 | Each refine box is a chat: a thread of the member's messages and the model's streamed replies, and a composer with the model choices and a send button. A follow-up message refines the previous answer, and Apply takes the latest one | RFC-087 "Each opens a box under its field with an optional 'What to improve' input and a Refine button"; RFC-123 "The statement box and the acceptance box edit one 'What to improve' note" |

## RFC-125 Refine as a chat

### Decision

- The refine box under a field is a chat card. Its header names the field, says what else the model reads (RFC-123), and has "New chat" once there is a thread and a close button.
- The member's messages show on the right, and the model's replies on the left with the model id. A reply streams into the card as it is written (RFC-087).
- The composer at the bottom has a message box, labelled "What to improve (optional)" for assistive technology, the Model, Thinking, and Effort choices of RFC-124 as compact selects, and a round send button named "Refine". Enter sends and Shift+Enter starts a new line. While a reply streams, the button is "Stop".
- The first message sends the request of RFC-087 with the message as `instructions`; an empty message refines the field as it is.
- A follow-up message sends the same request with the latest finished reply in place of the refined field's text, and the new message as `instructions`. The other field is the form's text, as before. When no reply has finished, it sends the form's text.
- The latest finished reply shows the comparison from the field as it was when the thread began to that reply, with Apply and Discard. Apply changes only that field in the form and closes the box. Discard and New chat empty the thread. Earlier replies stay in the thread as plain text.
- Each box keeps its own message draft. The two boxes still share the model choices (RFC-124).
- The API does not change. Each message is one `POST /api/projects/{projectId}/requirements/refine`.

### Why

A member usually wants a second pass: shorter, another word, one more check. Asking again from the form's text loses the first answer. Sending the previous answer back with the new message lets the member steer the text in a few short turns, and a chat shows that history in the order it happened. Keeping each turn a separate request means no conversation is stored and the API stays as it is.

### Set aside

- Storing threads or showing them after the page reloads.
- Sending earlier messages of the thread to the model. Each turn sends only the latest answer and the new message.
