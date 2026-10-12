import { session } from '$lib/session';
import type {
  AccountGitHub,
  Board,
  Card,
  CardDiff,
  CompanyGitHub,
  CompanyAccess,
  CompanyRepo,
  ConventionCatalog,
  ConventionRule,
  DeploySettings,
  Intake,
  LLMSettings,
  ProjectConventions,
  ProjectLink,
  ProjectLinks,
  ProjectRecord,
  Repository,
  RequirementDraft,
  RouterSettings,
  RuleInput,
  SessionUser,
  Skill,
  SkillInput,
  Stack,
  StackInput,
  TraceResponse,
  TribeCandidate,
  TribeMember,
  TribeRecord
} from '$lib/types';

const base = 'http://127.0.0.1:8787';

export class ApiError extends Error {
  status: number;
  code: string;
  fields: Record<string, string>;

  constructor(status: number, code: string, message: string, fields: Record<string, string> = {}) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
    this.code = code;
    this.fields = fields;
  }
}

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers);
  const bearer = session.bearer();
  if (bearer) headers.set('Authorization', `Bearer ${bearer}`);
  if (init.body != null) headers.set('Content-Type', 'application/json');
  let response: Response;
  try {
    response = await fetch(base + path, { ...init, headers });
  } catch {
    throw new ApiError(0, 'network', 'API unreachable at http://127.0.0.1:8787');
  }
  const text = await response.text();
  const payload = text
    ? (JSON.parse(text) as { error?: { code?: string; message?: string; fields?: Record<string, string> } })
    : {};
  if (response.status === 401 && path !== '/api/session') {
    session.clear();
    try {
      window.location.assign('/login');
    } catch {
      /* The test runner has no navigation. */
    }
  }
  if (!response.ok) {
    const error = payload.error ?? {};
    throw new ApiError(
      response.status,
      error.code ?? 'error',
      error.message ?? response.statusText,
      error.fields ?? {}
    );
  }
  return payload as T;
}

export function openSession(email: string, password: string) {
  return request<{ bearer: string; expiresAt: string; user: SessionUser }>('/api/session', {
    method: 'POST',
    body: JSON.stringify({ email, password })
  });
}

export function closeSession() {
  return request<void>('/api/session', { method: 'DELETE' });
}

type Me = SessionUser & {
  githubLogin: string;
  githubTokenSet: boolean;
  githubVerified: boolean;
  tribeAdminOf: string[];
};

export function getMe() {
  return request<Me>('/api/me');
}

export function putMe(body: { name: string }) {
  return request<Me>('/api/me', { method: 'PUT', body: JSON.stringify(body) });
}

export function changePassword(currentPassword: string, newPassword: string) {
  return request<void>('/api/me/password', {
    method: 'PUT',
    body: JSON.stringify({ currentPassword, newPassword })
  });
}

export function getMyGitHub() {
  return request<AccountGitHub>('/api/me/github');
}

export function putMyGitHub(body: Record<string, unknown>) {
  return request<AccountGitHub>('/api/me/github', { method: 'PUT', body: JSON.stringify(body) });
}

export function testMyGitHub() {
  return request<{ ok: boolean; login?: string; message?: string }>('/api/me/github/test', {
    method: 'POST',
    body: '{}'
  });
}

function projectPrefix() {
  return `/api/projects/${session.project()}`;
}

export function getProjects() {
  return request<{ projects: ProjectRecord[] }>('/api/projects');
}

export function createProject(body: {
  tribeId: string;
  name: string;
  repo: string;
  baseBranch: string;
  links: { projectId: string; sharedPaths: string[] }[];
}) {
  return request<ProjectRecord & { links: ProjectLink[] }>('/api/projects', {
    method: 'POST',
    body: JSON.stringify(body)
  });
}

export function getTribes() {
  return request<{ tribes: TribeRecord[] }>('/api/tribes');
}

export function createTribe(name: string) {
  return request<TribeRecord>('/api/tribes', { method: 'POST', body: JSON.stringify({ name }) });
}

export function addMember(tribeId: string, userId: string) {
  return request<void>(`/api/tribes/${tribeId}/members`, { method: 'POST', body: JSON.stringify({ userId }) });
}

export function getMembers(tribeId: string) {
  return request<{ members: TribeMember[] }>(`/api/tribes/${tribeId}/members`);
}

export function setTribeAdmin(tribeId: string, userId: string, tribeAdmin: boolean) {
  return request<void>(`/api/tribes/${tribeId}/members/${userId}`, {
    method: 'PUT',
    body: JSON.stringify({ tribeAdmin })
  });
}

export function getTribeCandidates(tribeId: string) {
  return request<{ users: TribeCandidate[] }>(`/api/tribes/${tribeId}/candidates`);
}

export function removeMember(tribeId: string, userId: string) {
  return request<void>(`/api/tribes/${tribeId}/members/${userId}`, { method: 'DELETE' });
}

export function getUsers() {
  return request<{ users: SessionUser[] }>('/api/users');
}

export function createUser(body: { name: string; email: string; password: string; role: string; admin: boolean }) {
  return request<SessionUser>('/api/users', { method: 'POST', body: JSON.stringify(body) });
}

export function updateUser(
  id: string,
  body: { name?: string; role: string; admin: boolean; disabled: boolean; password?: string }
) {
  return request<SessionUser>(`/api/users/${id}`, { method: 'PUT', body: JSON.stringify(body) });
}

export function getCards() {
  return request<Board>(`${projectPrefix()}/cards`);
}

export function getCard(id: string) {
  return request<Card>(`${projectPrefix()}/cards/${id}`);
}

export function getCardDiff(id: string) {
  return request<CardDiff>(`${projectPrefix()}/cards/${id}/diff`);
}

export function getTrace(id: string) {
  return request<TraceResponse>(`${projectPrefix()}/cards/${id}/trace`);
}

export function getIntake() {
  return request<Intake>(`${projectPrefix()}/intake`);
}

export function ingest() {
  return request<Intake>(`${projectPrefix()}/ingest`, { method: 'POST', body: '{}' });
}

export function acceptRequirements(requirementIds: string[]) {
  return request<Intake>(`${projectPrefix()}/accept`, { method: 'POST', body: JSON.stringify({ requirementIds }) });
}

export function addRequirement(body: RequirementDraft & { id: string; selfReview?: boolean }) {
  return request<Intake>(`${projectPrefix()}/requirements`, { method: 'POST', body: JSON.stringify(body) });
}

export function updateRequirement(id: string, body: RequirementDraft) {
  return request<Intake>(`${projectPrefix()}/requirements/${encodeURIComponent(id)}`, {
    method: 'PUT',
    body: JSON.stringify(body)
  });
}

export function setReviewers(id: string, userIds: string[]) {
  return request<Intake>(`${projectPrefix()}/requirements/${encodeURIComponent(id)}/reviewers`, {
    method: 'PUT',
    body: JSON.stringify({ userIds })
  });
}

export function setSelfReview(id: string, selfReview: boolean) {
  return request<Intake>(`${projectPrefix()}/requirements/${encodeURIComponent(id)}/self-review`, {
    method: 'PUT',
    body: JSON.stringify({ selfReview })
  });
}

export function deleteRequirement(id: string) {
  return request<Intake>(`${projectPrefix()}/requirements/${encodeURIComponent(id)}`, { method: 'DELETE' });
}

export function startRun() {
  return request<{ queued: number; readyCount: number }>(`${projectPrefix()}/runs`, {
    method: 'POST',
    body: '{}'
  });
}

export function runCard(id: string) {
  return request<Card>(`${projectPrefix()}/cards/${id}/run`, { method: 'POST', body: '{}' });
}

export function deployCard(id: string) {
  return request<Card>(`${projectPrefix()}/cards/${id}/deploy`, { method: 'POST', body: '{}' });
}

export function approveCard(id: string, sha: string) {
  return request<Card>(`${projectPrefix()}/cards/${id}/approve`, { method: 'POST', body: JSON.stringify({ sha }) });
}

export function requestChanges(id: string) {
  return request<Card>(`${projectPrefix()}/cards/${id}/request-changes`, { method: 'POST', body: '{}' });
}

export function rejectCard(id: string) {
  return request<Card>(`${projectPrefix()}/cards/${id}/reject`, { method: 'POST', body: '{}' });
}

export function unblockCard(id: string, resetAttempts: boolean) {
  return request<Card>(`${projectPrefix()}/cards/${id}/unblock`, {
    method: 'POST',
    body: JSON.stringify({ resetAttempts })
  });
}

export function getCompanyGitHub() {
  return request<CompanyGitHub>('/api/settings/github');
}

export function putCompanyGitHub(body: Record<string, unknown>) {
  return request<CompanyGitHub>('/api/settings/github', { method: 'PUT', body: JSON.stringify(body) });
}

export function testCompanyGitHub() {
  return request<CompanyAccess>('/api/settings/github/test', {
    method: 'POST',
    body: '{}'
  });
}

export function getCompanyRepos() {
  return request<{ repos: CompanyRepo[] }>('/api/settings/github/repos');
}

export function getRepository() {
  return request<Repository>(`${projectPrefix()}/settings/repository`);
}

export function putRepository(body: { repo: string; baseBranch: string }) {
  return request<Repository>(`${projectPrefix()}/settings/repository`, { method: 'PUT', body: JSON.stringify(body) });
}

export function syncRepository() {
  return request<Repository>(`${projectPrefix()}/sync`, { method: 'POST', body: '{}' });
}

export function getLinks() {
  return request<ProjectLinks>(`${projectPrefix()}/settings/links`);
}

export function putLinks(links: { projectId: string; sharedPaths: string[] }[]) {
  return request<ProjectLinks>(`${projectPrefix()}/settings/links`, {
    method: 'PUT',
    body: JSON.stringify({ links })
  });
}

export function getLLM() {
  return request<LLMSettings>('/api/settings/llm');
}

export function putLLM(body: Record<string, unknown>) {
  return request<LLMSettings>('/api/settings/llm', { method: 'PUT', body: JSON.stringify(body) });
}

export function testLLM(family: string) {
  return request<{ ok: boolean; message?: string }>(`/api/settings/llm/${family}/test`, {
    method: 'POST',
    body: '{}'
  });
}

export function getRouter() {
  return request<RouterSettings>('/api/settings/router');
}

export function putRouter(body: Record<string, unknown>) {
  return request<RouterSettings>('/api/settings/router', { method: 'PUT', body: JSON.stringify(body) });
}

export function getDeploy() {
  return request<DeploySettings>(`${projectPrefix()}/settings/deploy`);
}

export function putDeploy(body: Record<string, unknown>) {
  return request<DeploySettings>(`${projectPrefix()}/settings/deploy`, { method: 'PUT', body: JSON.stringify(body) });
}

export function getConventions() {
  return request<ConventionCatalog>('/api/conventions');
}

export type ConventionKind = 'rules' | 'stacks' | 'skills';

function saveConvention<T>(kind: ConventionKind, id: string, body: unknown) {
  const path = id ? `/api/conventions/${kind}/${encodeURIComponent(id)}` : `/api/conventions/${kind}`;
  return request<T>(path, { method: id ? 'PUT' : 'POST', body: JSON.stringify(body) });
}

export function saveRule(id: string, body: RuleInput) {
  return saveConvention<ConventionRule>('rules', id, body);
}

export function saveStack(id: string, body: StackInput) {
  return saveConvention<Stack>('stacks', id, body);
}

export function saveSkill(id: string, body: SkillInput) {
  return saveConvention<Skill>('skills', id, body);
}

export function deleteConvention(kind: ConventionKind, id: string) {
  return request<void>(`/api/conventions/${kind}/${encodeURIComponent(id)}`, { method: 'DELETE' });
}

export function getProjectConventions() {
  return request<ProjectConventions>(`${projectPrefix()}/settings/conventions`);
}

export function putProjectConventions(body: { manualStackIds: string[]; skillIds: string[] }) {
  return request<ProjectConventions>(`${projectPrefix()}/settings/conventions`, {
    method: 'PUT',
    body: JSON.stringify(body)
  });
}

export function unreachable() {
  return 'API unreachable at http://127.0.0.1:8787';
}
