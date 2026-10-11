import { describe, expect, it } from 'vitest';
import { reasonLabel, statusLabel } from './status';

const statuses = [
  'backlog',
  'ready',
  'in_progress',
  'self_check',
  'pr_open',
  'agent_review',
  'changes_requested',
  'human_review',
  'approved',
  'deploying_dev',
  'verifying',
  'done',
  'blocked'
];

describe('status labels', () => {
  it('names every card status in words', () => {
    for (const status of statuses) {
      expect(statusLabel(status)).not.toContain('_');
      expect(statusLabel(status)).toMatch(/^[A-Z]/);
    }
    expect(statusLabel('human_review')).toBe('Waiting for review');
    expect(statusLabel('in_progress')).toBe('In progress');
  });

  it('turns an unknown code into a sentence', () => {
    expect(statusLabel('waiting_on_ci')).toBe('Waiting on ci');
  });

  it('names blocked and failure reasons', () => {
    expect(reasonLabel('owner_github')).toBe("Owner's GitHub not verified");
    expect(reasonLabel('attempt_cap')).toBe('Out of attempts');
    expect(reasonLabel('lock_holder')).toBe('Lock holder');
    expect(reasonLabel('The dev server did not answer.')).toBe('The dev server did not answer.');
  });
});
