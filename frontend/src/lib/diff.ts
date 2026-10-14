import type { DiffFile, DiffLine } from '$lib/types';

export type HunkHeader = { oldStart: number; oldCount: number; newStart: number; newCount: number; context: string };

export type NumberedLine = DiffLine & { old: number | null; new: number | null };

const headerPattern = /^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@ ?(.*)$/;

/** Reads `@@ -a,b +c,d @@ context`. A count left out is 1, as in git. */
export function parseHunkHeader(header: string): HunkHeader | null {
  const match = headerPattern.exec(header);
  if (!match) return null;
  return {
    oldStart: Number(match[1]),
    oldCount: match[2] === undefined ? 1 : Number(match[2]),
    newStart: Number(match[3]),
    newCount: match[4] === undefined ? 1 : Number(match[4]),
    context: match[5].trim()
  };
}

/** Gives each line its old and new line number. A deleted line has no new number, an added line no old one. */
export function numberLines(hunk: { header: string; lines: DiffLine[] }): NumberedLine[] {
  const parsed = parseHunkHeader(hunk.header);
  let old = parsed?.oldStart ?? 0;
  let next = parsed?.newStart ?? 0;
  return hunk.lines.map((line) => {
    if (!parsed || line.kind === 'meta') return { ...line, old: null, new: null };
    if (line.kind === 'add') return { ...line, old: null, new: next++ };
    if (line.kind === 'del') return { ...line, old: old++, new: null };
    return { ...line, old: old++, new: next++ };
  });
}

/** "Lines 67 to 73" for the new side of a hunk, or the raw header when it does not parse. */
export function hunkLabel(header: string): string {
  const parsed = parseHunkHeader(header);
  if (!parsed) return header;
  const start = parsed.newCount === 0 ? parsed.oldStart : parsed.newStart;
  const count = parsed.newCount === 0 ? parsed.oldCount : parsed.newCount;
  if (count <= 1) return `Line ${start}`;
  return `Lines ${start} to ${start + count - 1}`;
}

/** The folder and the file name of a path. A file at the root has the folder "". */
export function splitPath(path: string): { folder: string; name: string } {
  const cut = path.lastIndexOf('/');
  return cut < 0 ? { folder: '', name: path } : { folder: path.slice(0, cut), name: path.slice(cut + 1) };
}

export type FolderGroup = { folder: string; files: { file: DiffFile; index: number }[] };

/** Groups files by folder in the order each folder first appears, keeping each file's index in the list. */
export function groupByFolder(files: DiffFile[]): FolderGroup[] {
  const groups: FolderGroup[] = [];
  const byFolder = new Map<string, FolderGroup>();
  files.forEach((file, index) => {
    const { folder } = splitPath(file.path);
    let group = byFolder.get(folder);
    if (!group) {
      group = { folder, files: [] };
      byFolder.set(folder, group);
      groups.push(group);
    }
    group.files.push({ file, index });
  });
  return groups;
}

/** Changed lines of a file, 0 for a binary file. */
export function changedLines(file: DiffFile): number {
  return file.binary ? 0 : Math.max(0, file.added) + Math.max(0, file.deleted);
}
