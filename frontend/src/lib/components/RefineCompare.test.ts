import { fireEvent, render, screen, within } from '@testing-library/svelte';
import { expect, test, vi } from 'vitest';
import RefineCompare from './RefineCompare.svelte';

const before = { statement: 'Export the board quickly.', acceptance: ['One row per card.', 'Works well.'] };
const after = {
  statement: 'Export the whole board as CSV.',
  acceptance: ['One row per card.', 'The file opens in a spreadsheet.'],
  notes: 'Replaced the vague line with a check.',
  modelId: 'model-a'
};

test('changes show inserted and deleted words and acceptance lines', () => {
  const { container } = render(RefineCompare, { before, after, onapply: vi.fn(), ondiscard: vi.fn() });
  expect(screen.getByRole('heading', { name: 'Refined by model-a' })).toBeTruthy();
  expect(Array.from(container.querySelectorAll('ins')).map((node) => node.textContent)).toEqual(
    expect.arrayContaining([expect.stringContaining('whole')])
  );
  expect(Array.from(container.querySelectorAll('del')).map((node) => node.textContent)).toEqual(
    expect.arrayContaining([expect.stringContaining('quickly.')])
  );
  const lines = within(screen.getByRole('list', { name: 'Acceptance changes' })).getAllByRole('listitem');
  expect(lines.map((line) => line.textContent?.replace(/\s+/g, ' ').trim())).toEqual([
    'Unchanged: One row per card.',
    '− Removed: Works well.',
    '+ Added: The file opens in a spreadsheet.'
  ]);
  expect(screen.getByText(/1 added, 1 removed/)).toBeTruthy();
  expect(screen.getByText('Replaced the vague line with a check.')).toBeTruthy();
  expect(screen.queryByRole('status')).toBeNull();
});

test('Before and After render each version', async () => {
  render(RefineCompare, { before, after, onapply: vi.fn(), ondiscard: vi.fn() });
  await fireEvent.click(screen.getByRole('button', { name: 'Before' }));
  expect(within(screen.getByRole('region', { name: 'Statement before' })).getByText('Export the board quickly.')).toBeTruthy();
  expect(within(screen.getByRole('list', { name: 'Acceptance before' })).getAllByRole('listitem')).toHaveLength(2);
  await fireEvent.click(screen.getByRole('button', { name: 'After' }));
  expect(within(screen.getByRole('region', { name: 'Statement after' })).getByText('Export the whole board as CSV.')).toBeTruthy();
});

test('Apply and Discard call back, and a stale form is flagged', async () => {
  const onapply = vi.fn();
  const ondiscard = vi.fn();
  render(RefineCompare, { before, after, stale: true, onapply, ondiscard });
  expect(screen.getByRole('status').textContent).toContain('Apply replaces those edits');
  await fireEvent.click(screen.getByRole('button', { name: 'Apply to form' }));
  await fireEvent.click(screen.getByRole('button', { name: 'Discard' }));
  expect(onapply).toHaveBeenCalledTimes(1);
  expect(ondiscard).toHaveBeenCalledTimes(1);
});

test('an unchanged proposal says so', () => {
  render(RefineCompare, {
    before,
    after: { ...after, statement: before.statement, acceptance: before.acceptance, notes: '' },
    onapply: vi.fn(),
    ondiscard: vi.fn()
  });
  expect(screen.getByText('The model kept the text as it is.')).toBeTruthy();
});
