import type { Action } from 'svelte/action';

type Grow = { on: boolean; value: unknown };

/** Sizes a textarea to its text. The height is set through the CSSOM, which the CSP allows; a style attribute in markup is not. */
export const autogrow: Action<HTMLTextAreaElement, Grow> = (node, initial) => {
  let on = initial?.on ?? false;
  const fit = () => {
    if (!on) return;
    node.style.height = 'auto';
    const height = node.scrollHeight + node.offsetHeight - node.clientHeight;
    node.style.height = node.scrollHeight > 0 ? `${height}px` : '';
  };
  fit();
  node.addEventListener('input', fit);
  return {
    update(next) {
      on = next?.on ?? false;
      fit();
    },
    destroy() {
      node.removeEventListener('input', fit);
    }
  };
};
