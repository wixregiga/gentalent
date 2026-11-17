export function lineEnd(source: string, from: number): number {
  const end = source.indexOf("\n", from);
  return end < 0 ? source.length : end;
}

export function blockCommentEnd(source: string, from: number): number {
  const end = source.indexOf("*/", from + 2);
  return end < 0 ? source.length : end + 2;
}

export function nextOf(pattern: RegExp, source: string, from: number): number {
  pattern.lastIndex = from;
  const match = pattern.exec(source);
  return match ? match.index : -1;
}

export function afterNext(text: string, source: string, from: number): number {
  const at = source.indexOf(text, from);
  return at < 0 ? source.length : at + text.length;
}

export function quotedEnd(source: string, from: number, stops: RegExp): number {
  let i = from;
  for (;;) {
    const at = nextOf(stops, source, i);
    if (at < 0) return source.length;
    if (source[at] !== "\\") return at + 1;
    i = at + 2;
  }
}

export class LineIndex {
  private newlines: number[] | undefined;

  constructor(private readonly source: string) {}

  lineAt(offset: number): number {
    const newlines = this.newlines ?? (this.newlines = this.index());
    let low = 0;
    let high = newlines.length;
    while (low < high) {
      const middle = (low + high) >> 1;
      if (newlines[middle] < offset) low = middle + 1;
      else high = middle;
    }
    return low + 1;
  }

  private index(): number[] {
    const newlines: number[] = [];
    for (let at = this.source.indexOf("\n"); at >= 0; at = this.source.indexOf("\n", at + 1)) newlines.push(at);
    return newlines;
  }
}
