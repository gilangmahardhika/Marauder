import { redirect } from '@sveltejs/kit';
import { getProjects } from '$lib/api';
import { needsTribe, session } from '$lib/session';

export const ssr = false;
export const prerender = false;

/** A card link from a notification names its project; this tab switches to that project and its tribe. */
async function switchProject(id: string) {
  try {
    const project = (await getProjects()).projects.find((item) => item.id === id);
    if (!project) return;
    session.setTribe(project.tribeId);
    session.setProject(project.id);
  } catch {
    /* The card page reports a project it cannot reach. */
  }
}

export async function load({ url }) {
  if (url.pathname !== '/login' && session.bearer() === '') {
    redirect(307, '/login');
  }
  const project = url.searchParams.get('project');
  if (project && url.pathname.startsWith('/cards/') && project !== session.project()) {
    await switchProject(project);
  }
  if (needsTribe(url.pathname) && session.tribe() === '') {
    redirect(307, '/choose-tribe');
  }
}
