import { fireEvent, render, screen, within } from '@testing-library/svelte';
import { expect, test, vi } from 'vitest';
import RefineCompare from './RefineCompare.svelte';

const statement = { field: 'statement' as const, before: 'Export the board quickly.', after: 'Export the whole board as CSV.', modelId: 'model-a' };
const acceptance = {
  field: 'acceptance' as const,
  before: 'One row per card.\nWorks well.',
  after: 'One row per card.\nThe file opens in a spreadsheet.',
  modelId: 'model-a'
};

test('a statement shows inserted and deleted words', () => {
  const { container } = render(RefineCompare, { ...statement, onapply: vi.fn(), ondiscard: vi.fn() });
  expect(screen.getByRole('region', { name: 'Refined statement' })).toBeTruthy();
  expect(screen.getByRole('heading', { name: 'Refined by model-a' })).toBeTruthy();
  expect(Array.from(container.querySelectorAll('ins')).map((node) => node.textContent)).toEqual(
    expect.arrayContaining([expect.stringContaining('whole')])
  );
  expect(Array.from(container.querySelectorAll('del')).map((node) => node.textContent)).toEqual(
    expect.arrayContaining([expect.stringContaining('quickly.')])
  );
  expect(screen.queryByRole('list', { name: 'Acceptance changes' })).toBeNull();
  expect(screen.queryByRole('status')).toBeNull();
});

test('acceptance shows added and removed lines', () => {
  render(RefineCompare, { ...acceptance, onapply: vi.fn(), ondiscard: vi.fn() });
  const lines = within(screen.getByRole('list', { name: 'Acceptance changes' })).getAllByRole('listitem');
  expect(lines.map((line) => line.textContent?.replace(/\s+/g, ' ').trim())).toEqual([
    'Unchanged: One row per card.',
    '− Removed: Works well.',
    '+ Added: The file opens in a spreadsheet.'
  ]);
  expect(screen.getByText(/1 added, 1 removed/)).toBeTruthy();
  expect(screen.queryByLabelText('Statement changes')).toBeNull();
});

test('Before and After render each version', async () => {
  render(RefineCompare, { ...statement, onapply: vi.fn(), ondiscard: vi.fn() });
  await fireEvent.click(screen.getByRole('button', { name: 'Before' }));
  expect(within(screen.getByRole('region', { name: 'Statement before' })).getByText('Export the board quickly.')).toBeTruthy();
  await fireEvent.click(screen.getByRole('button', { name: 'After' }));
  expect(within(screen.getByRole('region', { name: 'Statement after' })).getByText('Export the whole board as CSV.')).toBeTruthy();
});

test('Before shows the acceptance lines as a checklist', async () => {
  render(RefineCompare, { ...acceptance, onapply: vi.fn(), ondiscard: vi.fn() });
  await fireEvent.click(screen.getByRole('button', { name: 'Before' }));
  expect(within(screen.getByRole('list', { name: 'Acceptance before' })).getAllByRole('listitem')).toHaveLength(2);
});

test('Apply and Discard call back, and a stale field is flagged', async () => {
  const onapply = vi.fn();
  const ondiscard = vi.fn();
  const { rerender } = render(RefineCompare, { ...statement, onapply, ondiscard });
  expect(screen.getByText('Nothing is saved until you press Save.')).toBeTruthy();
  await rerender({ ...statement, stale: true, onapply, ondiscard });
  expect(screen.getByRole('status').textContent).toContain('Apply replaces those edits');
  expect(screen.queryByText('Nothing is saved until you press Save.')).toBeNull();
  await fireEvent.click(screen.getByRole('button', { name: 'Apply' }));
  await fireEvent.click(screen.getByRole('button', { name: 'Discard' }));
  expect(onapply).toHaveBeenCalledTimes(1);
  expect(ondiscard).toHaveBeenCalledTimes(1);
});

test('unchanged text says so', () => {
  render(RefineCompare, { ...acceptance, after: acceptance.before, onapply: vi.fn(), ondiscard: vi.fn() });
  expect(screen.getByText('The model kept the text as it is.')).toBeTruthy();
});
