import { fireEvent, render, screen, within } from '@testing-library/svelte';
import { expect, test, vi } from 'vitest';
import type { RefineEvent, RefineRequest } from '$lib/types';
import RequirementForm from './RequirementForm.svelte';

test('statement and acceptance start tall and grow', () => {
  render(RequirementForm, { statement: '', acceptance: '' });
  const statement = screen.getByLabelText('Statement') as HTMLTextAreaElement;
  const acceptance = screen.getByLabelText('Acceptance') as HTMLTextAreaElement;
  expect(statement.rows).toBe(8);
  expect(acceptance.rows).toBe(6);
  expect(statement.className).toContain('resize-none');
  expect(screen.getByText(/Markdown works:/)).toBeTruthy();
  expect(screen.getByText(/Markdown works inside a line/)).toBeTruthy();
});

test('Preview formats the statement and Write brings the box back', async () => {
  render(RequirementForm, { statement: '# Export\n\nThe **CSV** has one row per card.', acceptance: '' });
  await fireEvent.click(screen.getByRole('button', { name: 'Preview' }));
  expect(screen.queryByLabelText('Statement')).toBeNull();
  expect(screen.getByRole('heading', { level: 3, name: 'Export' })).toBeTruthy();
  expect(screen.getByText('CSV').tagName).toBe('STRONG');

  await fireEvent.click(screen.getByRole('button', { name: 'Write' }));
  expect((screen.getByLabelText('Statement') as HTMLTextAreaElement).value).toContain('# Export');
});

test('Preview shows acceptance as a checklist of formatted lines', async () => {
  render(RequirementForm, {
    statement: 'Export',
    acceptance: '- [ ] Uses `text/csv`\n\n2. One **row** per card',
    fields: { acceptance: 'Keep each acceptance line to 1000 characters or fewer.' }
  });
  await fireEvent.click(screen.getByRole('button', { name: 'Preview' }));
  expect(screen.queryByLabelText('Acceptance')).toBeNull();
  const items = within(screen.getByRole('list', { name: 'Acceptance lines' })).getAllByRole('listitem');
  expect(items.map((item) => item.textContent?.trim())).toEqual(['Uses text/csv', 'One row per card']);
  expect(screen.getByText('text/csv').tagName).toBe('CODE');
  expect(screen.getByText('row').tagName).toBe('STRONG');
  expect(screen.getByText(/1000 characters or fewer/)).toBeTruthy();
});

test('a pasted document previews only its list items and says how many lines are left out', async () => {
  render(RequirementForm, {
    statement: 'Lint workflow',
    acceptance:
      'Here are the checks.\n\n## Definition of Done\n\n**PHP checks**\n- [ ] Pint runs with `--test`\n- [ ] Composer is cached'
  });
  await fireEvent.click(screen.getByRole('button', { name: 'Preview' }));
  const items = within(screen.getByRole('list', { name: 'Acceptance lines' })).getAllByRole('listitem');
  expect(items.map((item) => item.textContent?.trim())).toEqual(['Pint runs with --test', 'Composer is cached']);
  expect(screen.getByText('3 lines are not list items, so they are not saved.')).toBeTruthy();
});

test('an empty statement and acceptance have nothing to preview', async () => {
  render(RequirementForm, { statement: '  ', acceptance: '\n - \n' });
  await fireEvent.click(screen.getByRole('button', { name: 'Preview' }));
  for (const name of ['Statement preview', 'Acceptance preview']) {
    expect(within(screen.getByRole('region', { name })).getByText('Nothing to preview')).toBeTruthy();
  }
});

/** A stream that replies with text in two deltas and then done. */
function replying(text: string, acceptance?: string[]) {
  return vi.fn(async (_body: RefineRequest, onEvent: (event: RefineEvent) => void) => {
    onEvent({ kind: 'start', modelId: 'model-a' });
    const cut = Math.floor(text.length / 2);
    onEvent({ kind: 'delta', text: text.slice(0, cut) });
    onEvent({ kind: 'delta', text: text.slice(cut) });
    onEvent({ kind: 'done', text, acceptance, modelId: 'model-a' });
  });
}

test('without refine there are no refine buttons', () => {
  render(RequirementForm, { statement: 'Export', acceptance: '' });
  expect(screen.queryByRole('button', { name: /with AI/ })).toBeNull();
});

test('each field has its own refine icon, and opening one does not open the other', async () => {
  render(RequirementForm, { statement: 'Export the board.', acceptance: '- Works well.', refine: replying('x') });
  const statementButton = screen.getByRole('button', { name: 'Refine statement with AI' });
  const acceptanceButton = screen.getByRole('button', { name: 'Refine acceptance with AI' });
  expect(statementButton.getAttribute('aria-expanded')).toBe('false');
  expect(statementButton.className).toContain('size-11');

  await fireEvent.click(statementButton);
  expect(statementButton.getAttribute('aria-expanded')).toBe('true');
  expect(acceptanceButton.getAttribute('aria-expanded')).toBe('false');
  expect(screen.getAllByLabelText('What to improve (optional)')).toHaveLength(1);
  expect(document.getElementById('refine-statement-panel')).toBeTruthy();
  expect(document.getElementById('refine-acceptance-panel')).toBeNull();

  await fireEvent.click(acceptanceButton);
  expect(screen.getAllByLabelText('What to improve (optional)')).toHaveLength(2);
  await fireEvent.click(statementButton);
  expect(document.getElementById('refine-statement-panel')).toBeNull();
  expect(document.getElementById('refine-acceptance-panel')).toBeTruthy();
});

test('Apply on the statement changes only the statement', async () => {
  const refine = replying('Members export the board as CSV.');
  render(RequirementForm, { statement: 'Export the board.', acceptance: '- Works well.', refine });
  await fireEvent.click(screen.getByRole('button', { name: 'Refine statement with AI' }));
  await fireEvent.click(screen.getByRole('button', { name: 'Refine' }));
  expect(refine.mock.calls[0][0]).toEqual({ field: 'statement', statement: 'Export the board.', acceptance: ['Works well.'], instructions: '' });
  await screen.findByRole('region', { name: 'Refined statement' });
  expect((screen.getByLabelText('Statement') as HTMLTextAreaElement).value).toBe('Export the board.');

  await fireEvent.click(screen.getByRole('button', { name: 'Apply' }));
  expect((screen.getByLabelText('Statement') as HTMLTextAreaElement).value).toBe('Members export the board as CSV.');
  expect((screen.getByLabelText('Acceptance') as HTMLTextAreaElement).value).toBe('- Works well.');
  expect(document.getElementById('refine-statement-panel')).toBeNull();
});

test('Apply on acceptance changes only the acceptance lines', async () => {
  const lines = ['The export has one row per card.', 'Each row lists the card id.'];
  const refine = replying(lines.join('\n'), lines);
  render(RequirementForm, { statement: 'Export the board.', acceptance: '- Works well.', refine });
  await fireEvent.click(screen.getByRole('button', { name: 'Refine acceptance with AI' }));
  await fireEvent.click(screen.getByRole('button', { name: 'Refine' }));
  expect(refine.mock.calls[0][0].field).toBe('acceptance');
  await screen.findByRole('region', { name: 'Refined acceptance' });

  await fireEvent.click(screen.getByRole('button', { name: 'Apply' }));
  expect((screen.getByLabelText('Acceptance') as HTMLTextAreaElement).value).toBe(
    '- The export has one row per card.\n- Each row lists the card id.'
  );
  expect((screen.getByLabelText('Statement') as HTMLTextAreaElement).value).toBe('Export the board.');
});

test('an edit after asking is flagged, and Preview hides the refine icons', async () => {
  render(RequirementForm, { statement: 'Export the board.', acceptance: '', refine: replying('Members export the board.') });
  await fireEvent.click(screen.getByRole('button', { name: 'Refine statement with AI' }));
  await fireEvent.click(screen.getByRole('button', { name: 'Refine' }));
  await screen.findByRole('region', { name: 'Refined statement' });
  await fireEvent.input(screen.getByLabelText('Statement'), { target: { value: 'Export the board today.' } });
  expect(screen.getByText(/Apply replaces those edits/)).toBeTruthy();
  await fireEvent.click(screen.getByRole('button', { name: 'Discard' }));
  expect(screen.queryByRole('region', { name: 'Refined statement' })).toBeNull();
  expect((screen.getByLabelText('Statement') as HTMLTextAreaElement).value).toBe('Export the board today.');

  await fireEvent.click(screen.getByRole('button', { name: 'Preview' }));
  expect(screen.queryByRole('button', { name: /with AI/ })).toBeNull();
});
