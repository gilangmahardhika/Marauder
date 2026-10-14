# RFC-0051: Agent questions

Status: Accepted for implementation. Date: 2026-10-14.

This RFC adds decisions `RFC-114` and `RFC-115` for REQ-097. The implementer may ask the team questions with an `ask_user` tool. A card's answer mode decides whether a person answers, while the card waits, or the agent's own suggestion stands. It supersedes no statement. NG-008 still holds: an answered question reaches the next attempt through its pack, not by continuing the conversation.

Sprints 88 and 89 in [../plan/sprints.md](../plan/sprints.md) implement this file.

## Decision index

| Id | Decision | Supersedes |
| --- | --- | --- |
| RFC-114 | The implementer tool `ask_user` stores up to five questions on the card. On a Manual card the attempt ends with outcome `question`, which does not use an attempt, and the card moves `in_progress -> waiting_answer`. Answering every open question moves it to `ready` and queues a run. Every later attempt's pack carries the answered questions | none |
| RFC-115 | `cards.question_mode` is `manual` (default) or `auto`. On an Auto card `ask_user` records the agent's suggested answers, marked `auto`, and returns them so the attempt goes on. A person who may run the card changes the mode, and an attempt reads it once when it starts | none |

## RFC-114 The agent asks

### Problem

A requirement often leaves a choice open: which of two endpoints to extend, whether a field is optional, what an empty state says. The implementer guessed, and a wrong guess cost a full attempt and a review round before a person saw it.

### Decision

The tool:

- `ask_user` joins the implementer tools. Its argument is `{ "questions": [ { "question", "kind", "options", "allowOther", "recommended": { "selected", "text" }, "why" } ] }`.
- `kind` is `single` (exactly one option), `multiple` (one or more options), or `text` (free text, no options). `single` and `multiple` carry 2 to 8 distinct options of at most 120 characters. `allowOther` lets the person write their own answer beside or instead of an option.
- One call carries 1 to 5 questions. A question is at most 500 characters, and `why` at most 500.
- `recommended` is the agent's own best answer and must be a valid answer to the question. `why` says why in one or two sentences.
- A bad argument returns an `error: ` result to the model, as for any tool (RFC-080).
- `ask_user` runs alone in its turn, after any concurrent read-only calls of the same turn.
- It is refused with `error: ` after a `commit` in the attempt, because a later run resumes an unchecked commit without the model.
- On a Manual card it is refused once the card has three attempts with outcome `question`. The result tells the model to decide, and to state its assumption in the commit message.
- The tool description tells the model to ask before it edits files: a parked attempt's uncommitted edits are discarded when the next attempt starts.

Storage:

- `card_questions` keeps one row per question: `project_id`, `card_id`, `attempt_id`, `position`, `question`, `kind`, `options`, `allow_other`, `recommended`, `why`, and, once answered, `answer` (`{ "selected": [...], "text": "..." }`), `answer_source` (`person`, `suggestion`, or `auto`), `answered_by`, and `answered_at`.
- The question text, options, and suggestion are redacted with the attempt's secrets before they are stored. They are untrusted model output: pages render them as text.
- A `tool_call` event records the call with its questions and, on an Auto card, its answers.

Parking a Manual card:

- The loop stops after the call. The attempt finishes with outcome `question`, and `attempts_used` does not change.
- The job moves the card `in_progress -> waiting_answer` with the attempt id as evidence. No check runs. The card lock is released, so the worker slot is free.
- A remote runner reports the outcome `question`, and the control plane makes the same move. The runner writes the questions through its lease and never writes a status (RFC-108).
- `waiting_answer` is in the board group `waiting`. A requirement change moves it to `blocked` with `source_changed`, like a `ready` card.

Answering:

- `POST /api/projects/{projectId}/cards/{id}/answers` with `{ "answers": [ { "questionId", "selected", "text" } ] }` answers every open question in one request, marked `person`. `{ "useSuggestions": true }` takes every open question's `recommended` answer, marked `suggestion`.
- A `single` answer is exactly one listed option, or only text when `allowOther` is on. A `multiple` answer is one or more listed options, and text when `allowOther` is on. A `text` answer is non-empty text. Text is trimmed and at most 2,000 characters. Each failure is `422` on `answers.{n}`, and nothing changes.
- A card that is not `waiting_answer` is `409`. The caller needs the card's `answer` action: a role that may run cards, or a tribe admin of the project's tribe.
- One transaction stores the answers, moves `waiting_answer -> ready` with the caller as actor, and inserts a card-scoped run request (RFC-061), so the card shows as queued. A `questions_answered` event records the question ids and the source.

The pack:

- Every later attempt's implementer pack has an "Answers from the team" section after the instructions. It lists each answered question of the card, oldest first, with its answer and source. The section is data; the model follows it as the person's decision for this card.

Notification:

- `-> waiting_answer` notifies the owner with kind `question`, title "Agent has a question", and a browser push, per the RFC-113 table. The link opens the card.

### Why

Ending the attempt keeps the rule that an attempt is one conversation that never resumes (RFC-001, NG-008), frees the worker slot and the lock while a person thinks, and needs no new job state. Not counting the attempt keeps asking cheaper than guessing. The cap and the commit rule stop an agent from asking forever or asking after its work is fixed.

### Set aside

- Holding the attempt open while it waits. It keeps a worker slot, the lock, and a runner lease for as long as a person takes, and the wall-clock cap ends it anyway.
- Asking from the reviewer, triage, or planner. Their output already goes to a person.
- Answering by chat with the agent. Answers are part of the card's record and reach every later attempt the same way.

## RFC-115 Answer mode per card

### Decision

- `cards.question_mode` is `manual` or `auto`, default `manual`. The card JSON carries `questionMode`.
- `PUT /api/projects/{projectId}/cards/{id}/question-mode` with `{ "questionMode": "auto" | "manual" }`. Another value is `422`. The caller needs the card's `question_mode` action: a role that may run cards, or a tribe admin. A `done` or `wont_fix` card is `409`. A `question_mode_changed` event records the old and new mode.
- An attempt reads the mode once when it starts. A change applies from the next attempt.
- On an Auto card `ask_user` stores each question with `answer` equal to `recommended`, `answer_source` `auto`, and no `answered_by`, and returns "No person answers on this card. Proceed with these answers:" and each answer. One attempt may call it three times; a fourth call is refused with `error: `.
- Switching a `waiting_answer` card to Auto does not answer it. The card page offers "Let the agent decide", which takes the suggestions.

### Why

Some cards are routine and a person wants the agent to keep going; others carry product choices a person must make. The setting belongs to the card because the same project holds both. Manual is the default because a guessed product choice costs more than a short wait.

### Tests

- `TestAskUserParksCard`: a Manual attempt that asks finishes with outcome `question`, stores the open questions, and the card is `waiting_answer` with its attempt count unchanged.
- `TestAskUserAutoAnswers`: an Auto attempt stores the questions answered `auto`, the tool result carries the answers, and the attempt goes on to commit.
- `TestAskUserValidation`: too many questions, a `single` question with one option, and a suggestion that is not an option each return `error: `.
- `TestAskUserAfterCommitRejected` and `TestQuestionRoundCap`.
- `TestQuestionAttemptNotCounted`.
- `TestAnswerQueuesRun`, `TestAnswerValidation`, `TestAnswerSuggestions`, `TestQuestionModeRights`.
- `TestPackIncludesAnswers`, `TestQuestionNotifiesOwner`, `TestRunnerQuestionOutcome`, `TestWaitingAnswerEdges`.
- Vitest: `CardQuestions.test.ts`, `QuestionModeField.test.ts`, and the card page shows the questions only with the `answer` action.
