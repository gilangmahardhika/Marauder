/** Splits a textarea into acceptance lines, dropping blank ones. */
export function toLines(text: string): string[] {
  return text
    .split('\n')
    .map((line) => line.trim())
    .filter((line) => line !== '');
}

export function fromLines(lines: string[]): string {
  return lines.join('\n');
}
