export type IconName =
  | 'board'
  | 'intake'
  | 'projects'
  | 'git'
  | 'spark'
  | 'router'
  | 'terminal'
  | 'users'
  | 'shield'
  | 'chevron'
  | 'menu'
  | 'close'
  | 'check'
  | 'plus'
  | 'link'
  | 'building'
  | 'eye'
  | 'eye-off'
  | 'rules'
  | 'skill'
  | 'alert'
  | 'chart'
  | 'mcp'
  | 'server'
  | 'copy';

export const iconPaths: Record<IconName, string[]> = {
  copy: ['M9 9h11v11H9z', 'M15 9V4H4v11h5'],
  alert: ['M12 3.5L21.5 20h-19z', 'M12 10v4.5', 'M12 17.5h.01'],
  server: ['M4 4h16v6H4z', 'M4 14h16v6H4z', 'M7.5 7h.01', 'M7.5 17h.01'],
  mcp: ['M9 3v5', 'M15 3v5', 'M7 8h10v3a5 5 0 01-10 0z', 'M12 16v5'],
  chart: ['M4 20h16', 'M6.5 20v-7', 'M11 20V6', 'M15.5 20v-10', 'M20 20V3.5'],
  rules: ['M9 6h11', 'M9 12h11', 'M9 18h11', 'M4.5 6h.01', 'M4.5 12h.01', 'M4.5 18h.01'],
  skill: ['M14.7 6.3a4 4 0 00-5.4 5.4L4 17v3h3l5.3-5.3a4 4 0 005.4-5.4l-2.5 2.5-2.4-.6-.6-2.4z'],
  board: ['M4 4h5v16H4z', 'M10.5 4h4v10h-4z', 'M16 4h4v14h-4z'],
  intake: ['M12 3l8 4v10l-8 4-8-4V7z', 'M4 7l8 4 8-4', 'M12 11v10'],
  projects: ['M4 4h7v7H4z', 'M13 4h7v7h-7z', 'M4 13h7v7H4z', 'M13 13h7v7h-7z'],
  git: ['M7 5a2 2 0 110-4 2 2 0 010 4', 'M7 5v14', 'M7 12h7a3 3 0 013 3v1', 'M17 16a2 2 0 110 4 2 2 0 010-4'],
  spark: ['M12 3l1.4 5.2L19 10l-5.6 1.8L12 17l-1.4-5.2L5 10l5.6-1.8z'],
  router: ['M5 7h5', 'M14 7h5', 'M12 7v10', 'M7 17h10'],
  terminal: ['M5 7l5 5-5 5', 'M12 17h7'],
  users: ['M9 11a3 3 0 100-6 3 3 0 000 6', 'M3.5 19a5.5 5.5 0 0111 0', 'M17 11.5a2.5 2.5 0 100-5', 'M16.5 19a4.2 4.2 0 014-3.5'],
  shield: ['M12 3l7 3v6c0 4.2-2.8 7.2-7 8.8C7.8 19.2 5 16.2 5 12V6z'],
  chevron: ['M6 9l6 6 6-6'],
  menu: ['M4 7h16', 'M4 12h16', 'M4 17h16'],
  close: ['M6 6l12 12', 'M18 6L6 18'],
  check: ['M5 12l5 5 9-9'],
  plus: ['M12 5v14', 'M5 12h14'],
  link: ['M10 14a4 4 0 005.7 0l3-3a4 4 0 00-5.7-5.7l-1 1', 'M14 10a4 4 0 00-5.7 0l-3 3a4 4 0 005.7 5.7l1-1'],
  building: ['M5 21V5l7-2v18', 'M12 8l7 2v11', 'M3 21h18', 'M8 9h1', 'M8 13h1', 'M8 17h1', 'M15 13h1', 'M15 17h1'],
  eye: ['M2.5 12S6 5.5 12 5.5 21.5 12 21.5 12 18 18.5 12 18.5 2.5 12 2.5 12z', 'M12 15a3 3 0 100-6 3 3 0 000 6'],
  'eye-off': [
    'M4 4l16 16',
    'M10.6 5.6A9.6 9.6 0 0112 5.5c6 0 9.5 6.5 9.5 6.5a16.8 16.8 0 01-3 3.8',
    'M6.6 6.9C4 8.6 2.5 12 2.5 12S6 18.5 12 18.5a9 9 0 004.6-1.3',
    'M9.9 9.9a3 3 0 004.2 4.2'
  ]
};
