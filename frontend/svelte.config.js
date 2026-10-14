import adapter from '@sveltejs/adapter-static';

export default {
  kit: {
    adapter: adapter({ fallback: 'index.html' }),
    serviceWorker: { register: false },
    csp: {
      mode: 'hash',
      directives: {
        'default-src': ['self'],
        'script-src': ['self'],
        'style-src': ['self'],
        'img-src': ['self'],
        'connect-src': ['self', 'http://127.0.0.1:8787'],
        'object-src': ['none'],
        'base-uri': ['self'],
        'form-action': ['self']
      }
    }
  }
};
