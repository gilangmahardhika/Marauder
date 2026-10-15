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
  expect(statementButton.className).toContain('min-h-11');
  expect(statementButton.textContent?.trim()).toBe('Refine with AI');
  expect(statementButton.className).not.toMatch(/(^|\s)(aria-expanded:)?bg-accent(\s|$)/);
  const label = statementButton.querySelector('span') as HTMLElement;
  expect(label.className).not.toMatch(/(^|\s)(ring-1|bg-surface)(\s|$)/);

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
  expect(refine.mock.calls[0][0]).toEqual({
    field: 'statement',
    statement: 'Export the board.',
    acceptance: ['Works well.'],
    instructions: '',
    model: '',
    thinking: '',
    effort: ''
  });
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

test('each refine box says what else it reads', async () => {
  render(RequirementForm, { statement: 'Export the board.', acceptance: '- Works well.', refine: replying('x') });
  await fireEvent.click(screen.getByRole('button', { name: 'Refine statement with AI' }));
  await fireEvent.click(screen.getByRole('button', { name: 'Refine acceptance with AI' }));
  const statementPanel = document.getElementById('refine-statement-panel') as HTMLElement;
  const acceptancePanel = document.getElementById('refine-acceptance-panel') as HTMLElement;
  expect(within(statementPanel).getByText('Reads your acceptance: 1 line')).toBeTruthy();
  expect(within(acceptancePanel).getByText('Reads your statement: 3 words')).toBeTruthy();

  await fireEvent.input(screen.getByLabelText('Acceptance'), { target: { value: '' } });
  expect(within(statementPanel).getByText('No acceptance yet')).toBeTruthy();
});

test('each refine box keeps its own message', async () => {
  const refine = replying('x');
  render(RequirementForm, { statement: 'Export the board.', acceptance: '', refine });
  await fireEvent.click(screen.getByRole('button', { name: 'Refine statement with AI' }));
  await fireEvent.click(screen.getByRole('button', { name: 'Refine acceptance with AI' }));
  const [statementNote, acceptanceNote] = screen.getAllByLabelText('What to improve (optional)') as HTMLTextAreaElement[];
  await fireEvent.input(statementNote, { target: { value: 'Say who needs it' } });
  expect(acceptanceNote.value).toBe('');

  const acceptancePanel = document.getElementById('refine-acceptance-panel') as HTMLElement;
  await fireEvent.click(within(acceptancePanel).getByRole('button', { name: 'Refine' }));
  expect(refine.mock.calls[0][0].instructions).toBe('');
  expect(statementNote.value).toBe('Say who needs it');
});

test('Close refine collapses the box and the corner button', async () => {
  render(RequirementForm, { statement: 'Export the board.', acceptance: '', refine: replying('x') });
  const button = screen.getByRole('button', { name: 'Refine statement with AI' });
  await fireEvent.click(button);
  const panel = document.getElementById('refine-statement-panel') as HTMLElement;
  await fireEvent.click(within(panel).getByRole('button', { name: 'Close refine' }));
  expect(document.getElementById('refine-statement-panel')).toBeNull();
  expect(button.getAttribute('aria-expanded')).toBe('false');
});

test('applying a statement offers to refine the acceptance against it', async () => {
  const refine = replying('Members export the board as CSV.');
  render(RequirementForm, { statement: 'Export the board.', acceptance: '- Works well.', refine });
  await fireEvent.click(screen.getByRole('button', { name: 'Refine statement with AI' }));
  await fireEvent.click(screen.getByRole('button', { name: 'Refine' }));
  await screen.findByRole('region', { name: 'Refined statement' });
  await fireEvent.click(screen.getByRole('button', { name: 'Apply' }));
  expect(screen.getByText('Statement updated.')).toBeTruthy();

  await fireEvent.click(screen.getByRole('button', { name: 'Refine acceptance to match' }));
  expect(screen.queryByText('Statement updated.')).toBeNull();
  expect(screen.getByRole('button', { name: 'Refine acceptance with AI' }).getAttribute('aria-expanded')).toBe('true');
  expect(refine).toHaveBeenCalledTimes(2);
  expect(refine.mock.calls[1][0]).toEqual({
    field: 'acceptance',
    statement: 'Members export the board as CSV.',
    acceptance: ['Works well.'],
    instructions: '',
    model: '',
    thinking: '',
    effort: ''
  });
});

const models = [
  { id: 'claude-a', family: 'anthropic', protocol: 'anthropic' },
  { id: 'gpt-b', family: 'openai', protocol: 'openai' }
];

test('without models the refine box shows no model choices', async () => {
  render(RequirementForm, { statement: 'Export the board.', acceptance: '', refine: replying('x') });
  await fireEvent.click(screen.getByRole('button', { name: 'Refine statement with AI' }));
  expect(screen.queryByRole('combobox', { name: 'Model' })).toBeNull();
  expect(screen.queryByRole('combobox', { name: 'Effort' })).toBeNull();
});

const text = (node: HTMLElement) => node.textContent?.replace(/\s+/g, ' ').trim();

async function choose(name: string, option: string) {
  await fireEvent.click(screen.getByRole('combobox', { name }));
  await fireEvent.click(within(screen.getByRole('listbox', { name })).getByRole('option', { name: option }));
}

test('the refine box starts on the implement model and sends the chosen model and effort', async () => {
  const refine = replying('x');
  render(RequirementForm, { statement: 'Export the board.', acceptance: '', refine, models, implementModel: 'claude-a' });
  await fireEvent.click(screen.getByRole('button', { name: 'Refine statement with AI' }));
  const model = screen.getByRole('combobox', { name: 'Model' });
  const thinking = screen.getByRole('combobox', { name: 'Thinking' });
  expect(text(model)).toBe('claude-a');
  expect(text(thinking)).toBe('Thinking Default');

  await fireEvent.click(model);
  const current = screen.getByRole('option', { name: /claude-a/ });
  expect(current.getAttribute('aria-selected')).toBe('true');
  expect(text(current)).toBe('claude-a Implement');
  await fireEvent.click(model);

  await fireEvent.click(thinking);
  expect(within(screen.getByRole('listbox', { name: 'Thinking' })).getAllByRole('option').map(text)).toEqual([
    'Model default',
    'On',
    'Off',
    'Adaptive'
  ]);
  await fireEvent.click(screen.getByRole('option', { name: 'Adaptive' }));
  expect(text(thinking)).toBe('Thinking Adaptive');

  await choose('Model', 'gpt-b');
  expect(text(model)).toBe('gpt-b');
  expect(text(thinking)).toBe('Thinking Default');
  await fireEvent.click(thinking);
  expect(screen.queryByRole('option', { name: 'Adaptive' })).toBeNull();
  await fireEvent.click(thinking);
  await choose('Effort', 'Minimal');
  expect(text(screen.getByRole('combobox', { name: 'Effort' }))).toBe('Effort Minimal');

  await fireEvent.click(screen.getByRole('button', { name: 'Refine' }));
  expect(refine.mock.calls[0][0]).toMatchObject({ model: 'gpt-b', thinking: '', effort: 'minimal' });
});

test('the model choices are shared by both refine boxes', async () => {
  render(RequirementForm, { statement: 'Export the board.', acceptance: '', refine: replying('x'), models, implementModel: 'claude-a' });
  await fireEvent.click(screen.getByRole('button', { name: 'Refine statement with AI' }));
  await choose('Model', 'gpt-b');
  await fireEvent.click(screen.getByRole('button', { name: 'Refine acceptance with AI' }));
  const [statementModel, acceptanceModel] = screen.getAllByRole('combobox', { name: 'Model' });
  expect(text(statementModel)).toBe('gpt-b');
  expect(text(acceptanceModel)).toBe('gpt-b');
});

test('the acceptance offer drafts when there are no lines, and goes away on dismiss or an edit', async () => {
  render(RequirementForm, { statement: 'Export the board.', acceptance: '', refine: replying('Members export the board.') });
  const refineStatement = async () => {
    await fireEvent.click(screen.getByRole('button', { name: 'Refine statement with AI' }));
    await fireEvent.click(screen.getByRole('button', { name: 'Refine' }));
    await screen.findByRole('region', { name: 'Refined statement' });
    await fireEvent.click(screen.getByRole('button', { name: 'Apply' }));
  };

  await refineStatement();
  expect(screen.getByRole('button', { name: 'Draft acceptance from it' })).toBeTruthy();
  await fireEvent.click(screen.getByRole('button', { name: 'Dismiss' }));
  expect(screen.queryByText('Statement updated.')).toBeNull();

  await refineStatement();
  expect(screen.getByText('Statement updated.')).toBeTruthy();
  await fireEvent.input(screen.getByLabelText('Statement'), { target: { value: 'Members export the board today.' } });
  expect(screen.queryByText('Statement updated.')).toBeNull();
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

const mcpServers = [
  { slug: 'tickets', enabled: true, toolCount: 2 },
  { slug: 'docs', enabled: false, toolCount: 1 }
];

async function typeAt(area: HTMLTextAreaElement, text: string) {
  area.value = text;
  area.setSelectionRange(text.length, text.length);
  await fireEvent.input(area);
}

test('typing @ opens the server list, arrows move, Enter inserts, and Escape closes', async () => {
  render(RequirementForm, { statement: '', acceptance: '', servers: mcpServers });
  const statement = screen.getByLabelText('Statement') as HTMLTextAreaElement;
  expect(screen.getAllByText(/Type @ to name an MCP server\./)).toHaveLength(2);
  expect(statement.getAttribute('aria-expanded')).toBe('false');

  await typeAt(statement, 'Use @');
  const list = screen.getByRole('listbox', { name: 'MCP servers' });
  expect(within(list).getAllByRole('option')).toHaveLength(2);
  expect(statement.getAttribute('aria-expanded')).toBe('true');
  expect(statement.getAttribute('aria-autocomplete')).toBe('list');
  expect(statement.getAttribute('aria-activedescendant')).toBe('requirement-statement-mention-0');
  await fireEvent.keyDown(statement, { key: 'ArrowDown' });
  expect(statement.getAttribute('aria-activedescendant')).toBe('requirement-statement-mention-1');

  await typeAt(statement, 'Use @ti');
  expect(within(screen.getByRole('listbox')).getAllByRole('option').map((option) => option.textContent)).toEqual([
    expect.stringContaining('tickets')
  ]);
  await fireEvent.keyDown(statement, { key: 'Enter' });
  expect(statement.value).toBe('Use @tickets ');
  expect(screen.queryByRole('listbox')).toBeNull();

  await typeAt(statement, 'Use @tickets and @');
  expect(screen.getByRole('listbox')).toBeTruthy();
  await fireEvent.keyDown(statement, { key: 'Escape' });
  expect(screen.queryByRole('listbox')).toBeNull();
});

test('with no server the list says so', async () => {
  render(RequirementForm, { statement: '', acceptance: '' });
  await typeAt(screen.getByLabelText('Statement') as HTMLTextAreaElement, '@');
  expect(within(screen.getByRole('listbox')).getByText('No MCP servers in this tribe.')).toBeTruthy();
});

test('chips show a known server, a disabled one, and an unknown slug under their field', () => {
  render(RequirementForm, { statement: 'Read @tickets and @docs.', acceptance: '- Check @nothere', servers: mcpServers });
  const [statementList, acceptanceList] = screen.getAllByRole('list', { name: 'MCP servers mentioned' });
  expect(statementList.textContent).toContain('@tickets');
  expect(within(statementList).getByText('disabled: jobs will not call it')).toBeTruthy();
  expect(within(acceptanceList).getByText('no server with this slug')).toBeTruthy();
  expect(within(statementList).queryByText('no server with this slug')).toBeNull();
});
