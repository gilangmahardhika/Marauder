import type { Card } from './types';

const statuses: Record<string, string> = {
  backlog: 'Backlog',
  ready: 'Ready',
  in_progress: 'In progress',
  waiting_answer: 'Needs answer',
  self_check: 'Running checks',
  pr_open: 'Pull request open',
  agent_review: 'AI review',
  changes_requested: 'Changes requested',
  human_review: 'Waiting for review',
  approved: 'Approved',
  deploying_dev: 'Deploying to dev',
  verifying: 'Verifying on dev',
  done: 'Done',
  blocked: 'Blocked',
  wont_fix: "Won't fix"
};

const reasons: Record<string, string> = {
  owner_github: "Owner's GitHub not verified",
  source_changed: 'Requirement changed',
  attempt_cap: 'Out of attempts',
  rejected: 'Rejected',
  deploy_failed: 'Deploy failed',
  deploy_config: 'Deploy not set up',
  github_config: 'GitHub not set up',
  no_commit: 'No commit',
  assertion_failed: 'Dev check failed',
  ci: 'CI failed'
};

/** "some_code" reads as "Some code". Text that is not a code is returned as is. */
export function sentence(value: string) {
  if (!/^[a-z0-9_]+$/.test(value)) return value;
  const words = value.replaceAll('_', ' ').trim();
  return words.charAt(0).toUpperCase() + words.slice(1);
}

export function statusLabel(status: string) {
  return statuses[status] ?? sentence(status);
}

export function reasonLabel(reason: string) {
  return reasons[reason] ?? sentence(reason);
}

/** Badge tone for a card's status. */
export function cardTone(card: Pick<Card, 'status' | 'group'>) {
  if (card.status === 'blocked' || card.group === 'stopped') return 'red';
  if (card.status === 'waiting_answer') return 'amber';
  if (card.group === 'finished') return 'green';
  if (card.group === 'review') return 'amber';
  if (card.group === 'running' || card.group === 'shipping') return 'sky';
  return 'default';
}
