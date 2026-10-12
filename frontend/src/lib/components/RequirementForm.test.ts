import { fireEvent, render, screen, within } from '@testing-library/svelte';
import { expect, test, vi } from 'vitest';
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

const proposal = {
  statement: 'Members export the board as CSV.',
  acceptance: ['The export has one row per card.', 'Each row lists the card id.'],
  notes: 'Split the vague line.',
  modelId: 'model-a'
};

test('without refine there is no refine button', () => {
  render(RequirementForm, { statement: 'Export', acceptance: '' });
  expect(screen.queryByRole('button', { name: 'Refine with AI' })).toBeNull();
});

test('Refine shows the comparison and Apply fills the fields', async () => {
  const refine = vi.fn().mockResolvedValue(proposal);
  render(RequirementForm, { statement: 'Export the board.', acceptance: '- Works well.', refine });
  await fireEvent.input(screen.getByLabelText('What to improve (optional)'), { target: { value: ' Make it testable ' } });
  await fireEvent.click(screen.getByRole('button', { name: 'Refine with AI' }));
  expect(refine).toHaveBeenCalledWith({ statement: 'Export the board.', acceptance: ['Works well.'], instructions: 'Make it testable' });
  expect(await screen.findByRole('region', { name: 'Refined requirement' })).toBeTruthy();
  expect((screen.getByLabelText('Statement') as HTMLTextAreaElement).value).toBe('Export the board.');

  await fireEvent.click(screen.getByRole('button', { name: 'Apply to form' }));
  expect((screen.getByLabelText('Statement') as HTMLTextAreaElement).value).toBe('Members export the board as CSV.');
  expect((screen.getByLabelText('Acceptance') as HTMLTextAreaElement).value).toBe(
    '- The export has one row per card.\n- Each row lists the card id.'
  );
  expect(screen.queryByRole('region', { name: 'Refined requirement' })).toBeNull();
});

test('Discard keeps the original and an edit after asking is flagged', async () => {
  const refine = vi.fn().mockResolvedValue(proposal);
  render(RequirementForm, { statement: 'Export the board.', acceptance: '', refine });
  await fireEvent.click(screen.getByRole('button', { name: 'Refine with AI' }));
  await screen.findByRole('region', { name: 'Refined requirement' });
  const statement = screen.getByLabelText('Statement') as HTMLTextAreaElement;
  await fireEvent.input(statement, { target: { value: 'Export the board today.' } });
  expect(screen.getByText(/Apply replaces those edits/)).toBeTruthy();
  await fireEvent.click(screen.getByRole('button', { name: 'Discard' }));
  expect(screen.queryByRole('region', { name: 'Refined requirement' })).toBeNull();
  expect(statement.value).toBe('Export the board today.');
});

test('a failed refine shows its message and an empty statement sends nothing', async () => {
  const refine = vi.fn().mockRejectedValue(new Error('set a model for the implement route in Router settings to refine requirements'));
  render(RequirementForm, { statement: '  ', acceptance: '', refine });
  await fireEvent.click(screen.getByRole('button', { name: 'Refine with AI' }));
  expect(screen.getByRole('alert').textContent).toBe('Write a statement first, then refine it.');
  expect(refine).not.toHaveBeenCalled();

  await fireEvent.input(screen.getByLabelText('Statement'), { target: { value: 'Export' } });
  await fireEvent.click(screen.getByRole('button', { name: 'Refine with AI' }));
  expect((await screen.findByRole('alert')).textContent).toContain('implement route');
});
