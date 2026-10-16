import { fireEvent, render, screen } from '@testing-library/svelte';
import { afterEach, expect, test, vi } from 'vitest';
import CardQuestions from './CardQuestions.svelte';
import { session } from '$lib/session';
import { toast } from '$lib/toast.svelte';
import type { Card, CardQuestion } from '$lib/types';

afterEach(() => {
  vi.unstubAllGlobals();
  toast.clear();
});

function question(id: string, extra: Partial<CardQuestion>): CardQuestion {
  return {
    id,
    attemptN: 1,
    question: 'Which format?',
    kind: 'single',
    options: ['CSV', 'Excel'],
    allowOther: false,
    recommended: { selected: ['CSV'], text: '' },
    why: 'The requirement names CSV.',
    answer: null,
    answerSource: null,
    answeredBy: null,
    answeredAt: null,
    createdAt: '2026-10-14T05:00:00Z',
    ...extra
  };
}

const open = [
  question('qst_000000000001', {}),
  question('qst_000000000002', {
    question: 'Which columns?',
    kind: 'multiple',
    options: ['Id', 'Total'],
    allowOther: true,
    recommended: { selected: ['Id'], text: 'Customer' },
    why: ''
  }),
  question('qst_000000000003', {
    question: 'File name?',
    kind: 'text',
    options: [],
    recommended: { selected: [], text: 'orders.csv' },
    why: ''
  })
];

const earlier = [
  question('qst_000000000010', {
    question: 'Keep the header row?',
    answer: { selected: ['CSV'], text: '' },
    answerSource: 'person',
    answeredBy: { id: 'usr_ada', name: 'Ada Lovelace', email: 'ada@example.com' }
  }),
  question('qst_000000000011', {
    question: 'Sort by date?',
    answer: { selected: ['Excel'], text: '' },
    answerSource: 'suggestion'
  }),
  question('qst_000000000012', { question: 'Gzip it?', answer: { selected: ['CSV'], text: '' }, answerSource: 'auto' })
];

function card(questions: CardQuestion[], actions = ['answer', 'question_mode']): Card {
  return {
    id: 'card_0123456789ab',
    title: 'Export orders',
    status: 'waiting_answer',
    group: 'question',
    actions,
    unblockTo: null,
    owner: null,
    reviewers: [],
    skills: [],
    requirementIds: [],
    decisionIds: [],
    dependsOn: [],
    queued: false,
    waitingOn: [],
    risk: 'low',
    service: 'default',
    manual: false,
    localCheck: null,
    devCheck: null,
    attemptsUsed: 0,
    maxAttempts: 3,
    branch: null,
    prUrl: null,
    headSha: null,
    reviewedSha: null,
    reviewIndependent: null,
    reviewVerdict: null,
    selfReview: false,
    aiReviewMode: 'continue',
    questionMode: 'manual',
    planMode: 'off',
    approvedSha: null,
    deployedSha: null,
    environment: 'dev',
    failureReason: null,
    blockedReason: null,
    updatedAt: '2026-10-14T05:00:00Z',
    questions
  };
}

function toastTexts() {
  return toast.items.map((item) => item.text);
}

function stubFetch(reply: (body: Record<string, unknown>) => Response) {
  session.set('stored-bearer');
  session.setProject('proj_widget');
  const calls: { url: string; method: string; body: Record<string, unknown> }[] = [];
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string, init: RequestInit) => {
      const body = JSON.parse(String(init.body ?? '{}'));
      calls.push({ url, method: String(init.method ?? 'GET'), body });
      return reply(body);
    })
  );
  return calls;
}

test('each kind renders its control, with the suggestion and nothing preselected', () => {
  render(CardQuestions, { card: card(open) });
  expect(screen.getByText('The agent has 3 questions')).toBeTruthy();
  const single = screen.getByRole('radiogroup', { name: 'Which format?' });
  const radios = single.querySelectorAll('input[type="radio"]');
  expect(radios).toHaveLength(2);
  expect([...radios].some((radio) => (radio as HTMLInputElement).checked)).toBe(false);
  expect(screen.getAllByRole('checkbox')).toHaveLength(3);
  expect(screen.getAllByText('Other')).toHaveLength(1);
  expect(screen.queryByRole('textbox', { name: 'Your answer' })).not.toBeNull();
  expect(screen.getAllByRole('textbox')).toHaveLength(1);
  expect(screen.getByText('Agent suggests: CSV')).toBeTruthy();
  expect(screen.getByText('Why: The requirement names CSV.')).toBeTruthy();
  expect(screen.getByText('Agent suggests: Id; Customer')).toBeTruthy();
});

test('Other shows its text input only after it is picked', async () => {
  render(CardQuestions, { card: card([open[1]]) });
  expect(screen.queryByRole('textbox')).toBeNull();
  await fireEvent.click(screen.getByRole('checkbox', { name: 'Other' }));
  expect(screen.getByRole('textbox', { name: 'Your answer' })).toBeTruthy();
});

test('an unanswered question is flagged and nothing is sent', async () => {
  const calls = stubFetch(() => new Response('{}', { status: 200 }));
  render(CardQuestions, { card: card(open) });
  await fireEvent.click(screen.getByRole('radio', { name: 'CSV' }));
  await fireEvent.click(screen.getByRole('button', { name: 'Submit answers' }));
  expect(screen.getAllByText('Answer this question.')).toHaveLength(2);
  expect(calls).toHaveLength(0);
});

test('a complete submit posts the answers and a 422 shows under its question', async () => {
  let fail = true;
  const calls = stubFetch((body) => {
    if (fail) {
      fail = false;
      return new Response(
        JSON.stringify({ error: { code: 'validation', message: 'bad', fields: { 'answers.0': 'Choose one option.' } } }),
        { status: 422 }
      );
    }
    expect(body.answers).toBeTruthy();
    return new Response(JSON.stringify(card(earlier, ['question_mode'])), { status: 200 });
  });
  render(CardQuestions, { card: card(open) });
  await fireEvent.click(screen.getByRole('radio', { name: 'Excel' }));
  await fireEvent.click(screen.getByRole('checkbox', { name: 'Total' }));
  await fireEvent.click(screen.getByRole('checkbox', { name: 'Other' }));
  const other = screen.getAllByRole('textbox', { name: 'Your answer' })[0];
  await fireEvent.input(other, { target: { value: 'Customer' } });
  const textarea = screen.getAllByRole('textbox', { name: 'Your answer' })[1];
  await fireEvent.input(textarea, { target: { value: 'orders.csv' } });
  await fireEvent.click(screen.getByRole('button', { name: 'Submit answers' }));
  expect(await screen.findByText('Choose one option.')).toBeTruthy();
  expect(calls[0].url).toContain('/api/projects/proj_widget/cards/card_0123456789ab/answers');
  expect(calls[0].method).toBe('POST');
  expect(calls[0].body).toEqual({
    answers: [
      { questionId: 'qst_000000000001', selected: ['Excel'], text: '' },
      { questionId: 'qst_000000000002', selected: ['Total'], text: 'Customer' },
      { questionId: 'qst_000000000003', selected: [], text: 'orders.csv' }
    ]
  });
  await fireEvent.click(screen.getByRole('button', { name: 'Submit answers' }));
  await vi.waitFor(() => expect(toastTexts()).toEqual(['Answers sent. The next attempt is queued.']));
  expect(calls).toHaveLength(2);
});

test('Let the agent decide posts useSuggestions', async () => {
  const calls = stubFetch(() => new Response(JSON.stringify(card(earlier, ['question_mode'])), { status: 200 }));
  render(CardQuestions, { card: card(open) });
  await fireEvent.click(screen.getByRole('button', { name: 'Let the agent decide' }));
  await vi.waitFor(() => expect(toastTexts()).toEqual(['Answers sent. The next attempt is queued.']));
  expect(calls[0].body).toEqual({ useSuggestions: true });
});

test('earlier questions wait behind a button and name their source', async () => {
  render(CardQuestions, { card: card(earlier, ['question_mode']) });
  expect(screen.queryByText('Keep the header row?')).toBeNull();
  await fireEvent.click(screen.getByRole('button', { name: 'Show 3 earlier questions' }));
  expect(screen.getByText('Keep the header row?')).toBeTruthy();
  expect(screen.getByText('You')).toBeTruthy();
  expect(screen.getByText('Ada Lovelace')).toBeTruthy();
  expect(screen.getByText("Agent's suggestion")).toBeTruthy();
  expect(screen.getByText('Auto')).toBeTruthy();
});
