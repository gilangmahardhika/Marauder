import { fireEvent, render, screen, within } from '@testing-library/svelte';
import { expect, test } from 'vitest';
import { blankFamily, blankOptions, type FamilyDraft, type OptionsDraft } from '$lib/llm';
import FamilyForm from './FamilyForm.svelte';

function family(protocol: string): FamilyDraft {
  const draft = $state({ ...blankFamily(), id: 'f', protocol, models: ['model-a'] });
  return draft;
}

function blankOptionsWith(over: Partial<OptionsDraft>): OptionsDraft {
  return { ...blankOptions(), ...over };
}

function optionLabels(select: HTMLElement) {
  return within(select)
    .getAllByRole('option')
    .map((option) => option.textContent?.trim());
}

test('Adaptive is offered on anthropic only, and the effort list follows the protocol', () => {
  const anthropic = render(FamilyForm, { family: family('anthropic') });
  const group = screen.getByRole('group', { name: 'Settings for model-a' });
  expect(optionLabels(within(group).getByLabelText('Thinking'))).toEqual(['Provider default', 'On', 'Off', 'Adaptive']);
  expect(optionLabels(within(group).getByLabelText('Effort'))).toEqual(['Provider default', 'low', 'medium', 'high', 'xhigh', 'max']);
  anthropic.unmount();

  render(FamilyForm, { family: family('openai') });
  const openai = screen.getByRole('group', { name: 'Settings for model-a' });
  expect(optionLabels(within(openai).getByLabelText('Thinking'))).toEqual(['Provider default', 'On', 'Off']);
  expect(optionLabels(within(openai).getByLabelText('Effort'))).toEqual([
    'Provider default',
    'none',
    'minimal',
    'low',
    'medium',
    'high',
    'xhigh'
  ]);
});

test('the budget shows only for On on anthropic', async () => {
  const anthropic = render(FamilyForm, { family: family('anthropic') });
  expect(screen.queryByLabelText('Budget')).toBeNull();
  await fireEvent.change(screen.getByLabelText('Thinking'), { target: { value: 'enabled' } });
  expect(screen.getByLabelText('Budget')).toBeTruthy();
  await fireEvent.change(screen.getByLabelText('Thinking'), { target: { value: 'adaptive' } });
  expect(screen.queryByLabelText('Budget')).toBeNull();
  anthropic.unmount();

  render(FamilyForm, { family: family('openai') });
  await fireEvent.change(screen.getByLabelText('Thinking'), { target: { value: 'enabled' } });
  expect(screen.queryByLabelText('Budget')).toBeNull();
});

test('every model row has a history budget on either protocol', async () => {
  const draft = family('openai');
  render(FamilyForm, { family: draft });
  const group = screen.getByRole('group', { name: 'Settings for model-a' });
  const history = within(group).getByLabelText('History budget (KiB)');
  await fireEvent.input(history, { target: { value: '64' } });
  expect(draft.options[0].historyBudgetKiB).toBe(64);
});

test('model errors show under the model row they are about', () => {
  const draft = family('anthropic');
  draft.models = ['model-a', 'model-b', 'model-a'];
  draft.options = [blankOptionsWith({}), blankOptionsWith({ historyBudgetKiB: 2 }), blankOptionsWith({})];
  render(FamilyForm, {
    family: draft,
    fields: { models: 'model ids must be unique', modelOptions: 'the history budget must be from 4 to 512 KiB' }
  });
  expect(screen.getByLabelText('Model 3').getAttribute('aria-invalid')).toBe('true');
  expect(screen.getByLabelText('Model 1').getAttribute('aria-invalid')).toBeNull();
  const history = within(screen.getByRole('group', { name: 'Settings for model-b' })).getByLabelText('History budget (KiB)');
  expect(history.getAttribute('aria-invalid')).toBe('true');
  expect(document.getElementById('model-1-history-error')?.textContent).toContain('the history budget must be from 4 to 512 KiB');
});

test('Add model adds a settings row, and removing a model removes its settings', async () => {
  render(FamilyForm, { family: family('anthropic') });
  await fireEvent.click(screen.getByRole('button', { name: 'Add model' }));
  expect(screen.getAllByLabelText('Effort')).toHaveLength(2);
  await fireEvent.click(screen.getByRole('button', { name: 'Remove model-a' }));
  expect(screen.getAllByLabelText('Effort')).toHaveLength(1);
  expect(screen.queryByRole('group', { name: 'Settings for model-a' })).toBeNull();
});
