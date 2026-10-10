import { session } from '$lib/session';
import type {
  Board,
  Card,
  DeploySettings,
  GitHubSettings,
  Intake,
  LLMSettings,
  RouterSettings,
  TraceEvent
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
  const payload = text ? (JSON.parse(text) as { error?: { code?: string; message?: string; fields?: Record<string, string> } }) : {};
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
    throw new ApiError(response.status, error.code ?? 'error', error.message ?? response.statusText, error.fields ?? {});
  }
  return payload as T;
}

export function openSession(token: string) {
  return request<{ bearer: string; expiresAt: string }>('/api/session', {
    method: 'POST',
    body: JSON.stringify({ token })
  });
}

export function getCards() {
  return request<Board>('/api/cards');
}

export function getCard(id: string) {
  return request<Card>(`/api/cards/${id}`);
}

export function getTrace(id: string) {
  return request<{ events: TraceEvent[] }>(`/api/cards/${id}/trace`);
}

export function getIntake() {
  return request<Intake>('/api/intake');
}

export function ingest() {
  return request<Intake>('/api/ingest', { method: 'POST', body: '{}' });
}

export function acceptRequirements(requirementIds: string[]) {
  return request<Intake>('/api/accept', { method: 'POST', body: JSON.stringify({ requirementIds }) });
}

export function startRun() {
  return request<{ runId: string; canRun: boolean; reason?: string }>('/api/runs', { method: 'POST', body: '{}' });
}

export function approveCard(id: string, sha: string) {
  return request<Card>(`/api/cards/${id}/approve`, { method: 'POST', body: JSON.stringify({ sha }) });
}

export function requestChanges(id: string) {
  return request<Card>(`/api/cards/${id}/request-changes`, { method: 'POST', body: '{}' });
}

export function rejectCard(id: string) {
  return request<Card>(`/api/cards/${id}/reject`, { method: 'POST', body: '{}' });
}

export function unblockCard(id: string, resetAttempts: boolean) {
  return request<Card>(`/api/cards/${id}/unblock`, {
    method: 'POST',
    body: JSON.stringify({ resetAttempts })
  });
}

export function getGitHub() {
  return request<GitHubSettings>('/api/settings/github');
}

export function putGitHub(body: Record<string, unknown>) {
  return request<GitHubSettings>('/api/settings/github', { method: 'PUT', body: JSON.stringify(body) });
}

export function testGitHub() {
  return request<{ ok: boolean; login?: string; message?: string }>('/api/settings/github/test', {
    method: 'POST',
    body: '{}'
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
  return request<DeploySettings>('/api/settings/deploy');
}

export function putDeploy(body: Record<string, unknown>) {
  return request<DeploySettings>('/api/settings/deploy', { method: 'PUT', body: JSON.stringify(body) });
}

export function unreachable() {
  return 'API unreachable at http://127.0.0.1:8787';
}
