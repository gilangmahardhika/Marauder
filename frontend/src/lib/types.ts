export type Card = {
  id: string;
  title: string;
  status: string;
  group: string;
  actions: string[];
  unblockTo: string | null;
  requirementIds: string[];
  decisionIds: string[];
  dependsOn: string[];
  risk: string;
  service: string;
  manual: boolean;
  localCheck: string | null;
  devCheck: string | null;
  attemptsUsed: number;
  maxAttempts: number;
  branch: string | null;
  prUrl: string | null;
  headSha: string | null;
  reviewedSha: string | null;
  reviewIndependent: boolean | null;
  approvedSha: string | null;
  deployedSha: string | null;
  environment: string;
  failureReason: string | null;
  blockedReason: string | null;
  updatedAt: string;
};

export type Board = {
  canRun: boolean;
  running: boolean;
  cards: Card[];
};

export type TraceEvent = {
  id: string;
  ts: string;
  attemptId?: string;
  kind: string;
  payload: unknown;
};

export type Requirement = {
  id: string;
  sourceKind: string;
  sourcePath: string;
  heading: string;
  statement: string;
  acceptance: string[];
  accepted: boolean;
  linkedCards: string[];
};

export type Decision = {
  id: string;
  sourcePath: string;
  statement: string;
};

export type Intake = {
  draftPending: boolean;
  planError: string | null;
  requirements: Requirement[];
  decisions: Decision[];
};

export type SessionUser = {
  id: string;
  email: string;
  role: string;
  admin: boolean;
  disabled?: boolean;
};

export type TribeRecord = {
  id: string;
  name: string;
};

export type ProjectRecord = {
  id: string;
  tribeId: string;
  name: string;
  repo: string;
  baseBranch: string;
};

export type AccountGitHub = {
  host: string;
  login: string;
  tokenSet: boolean;
};

export type CompanyGitHub = {
  host: string;
  org: string;
  botLogin: string;
  tokenSet: boolean;
};

export type CompanyRepo = {
  name: string;
  defaultBranch: string;
  archived: boolean;
};

export type Repository = {
  host: string;
  org: string;
  repo: string;
  baseBranch: string;
  cloned: boolean;
  headSha: string | null;
};

export type ProjectLink = {
  projectId: string;
  name: string;
  repo: string;
  sharedPaths: string[];
};

export type Family = {
  id: string;
  protocol: string;
  baseUrl: string;
  models: string[];
  keySet: boolean;
};

export type LLMSettings = {
  families: Family[];
};

export type RouteChoice = {
  model: string;
  fallback: string;
};

export type RouterSettings = {
  routes: Record<string, RouteChoice>;
  budgets: {
    maxAttempts: number;
    maxModelTurnsPerAttempt: number;
    maxTokensPerCard: number;
    maxWallMinutesPerAttempt: number;
  };
  reviewIndependentPossible: boolean;
};

export type DeploySettings = {
  environment: string;
  devUrl: string;
  devHosts: string[];
  devCommand: string;
  rollbackCommand: string;
  healthUrl: string;
  deployTokenSet: boolean;
  localCheck: string;
  devCheck: string;
  allowExecutables: string[];
  sandboxEngine: string;
  sandboxImage: string;
  implementationCap: number;
  prdPath: string;
  rfcPath: string;
};
