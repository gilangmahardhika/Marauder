import { fireEvent, render, screen, within } from '@testing-library/svelte';
import { expect, test } from 'vitest';
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
