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
  | 'building';

export const iconPaths: Record<IconName, string[]> = {
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
  building: ['M5 21V5l7-2v18', 'M12 8l7 2v11', 'M3 21h18', 'M8 9h1', 'M8 13h1', 'M8 17h1', 'M15 13h1', 'M15 17h1']
};
