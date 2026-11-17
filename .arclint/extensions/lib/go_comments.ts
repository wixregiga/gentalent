import type { Comment, CommentKind } from "./comment";
import { afterNext, blockCommentEnd, LineIndex, lineEnd, nextOf, quotedEnd } from "./source_span";

interface ImportToken {
  line: number;
  before: Comment[];
}

const directive = /^\/\/(line |extern |export |[a-z0-9]+:[a-z0-9])|^\/\*line /;
const suppression = /^\/\/(nolint\b|lint:(file-)?ignore\b)/;
const generatedMarker = /^\/\/ Code generated .* DO NOT EDIT\.$/;
const stops = /[/"'`]/g;
const interpretedStringStops = /["\\\n]/g;
const runeStops = /['\\\n]/g;
const code = /[^ \t\r\n]/;
const endsWithImport = /(^|[^A-Za-z0-9_\u0080-￿])import[ \t\r\n]*$/;
const onlyImport = /^[ \t\r\n]*import[ \t\r\n]*$/;

function kindOf(text: string, beforeCode: boolean): CommentKind {
  if (beforeCode && generatedMarker.test(text)) return "generated";
  if (suppression.test(text)) return "suppression";
  if (directive.test(text)) return "directive";
  return "prose";
}

export function goComments(source: string): Comment[] {
  const lines = new LineIndex(source);
  const comments: Comment[] = [];
  let pending: Comment[] = [];
  let sawCode = false;
  let lastImport: ImportToken | undefined;
  let segment = 0;
  let i = 0;

  const codeUntil = (end: number) => {
    const text = source.slice(segment, end);
    if (!code.test(text)) return;
    sawCode = true;
    lastImport = endsWithImport.test(text)
      ? { line: lines.lineAt(segment + text.lastIndexOf("import")), before: onlyImport.test(text) ? pending : [] }
      : undefined;
    pending = [];
  };

  while (i < source.length) {
    const at = nextOf(stops, source, i);
    if (at < 0) break;
    const c = source[at];
    const next = source[at + 1];
    if (c === "/" && next !== "/" && next !== "*") {
      i = at + 1;
      continue;
    }
    codeUntil(at);
    if (c === "/") {
      const end = next === "/" ? lineEnd(source, at) : blockCommentEnd(source, at);
      const text = next === "/" ? source.slice(at, end).replace(/\r$/, "") : source.slice(at, end);
      const comment = { line: lines.lineAt(at), offset: at, text, kind: kindOf(text, !sawCode) };
      comments.push(comment);
      pending.push(comment);
      i = segment = end;
      continue;
    }
    const end = c === "`" ? afterNext("`", source, at + 1) : quotedEnd(source, at + 1, c === '"' ? interpretedStringStops : runeStops);
    if (lastImport && end - at === 3 && source.startsWith('"C"', at)) markPreamble(lastImport, lines);
    sawCode = true;
    lastImport = undefined;
    pending = [];
    i = segment = end;
  }
  return comments;
}

function markPreamble(importToken: ImportToken, lines: LineIndex): void {
  let expected = importToken.line - 1;
  for (let k = importToken.before.length - 1; k >= 0; k--) {
    const comment = importToken.before[k];
    const end = lines.lineAt(comment.offset + comment.text.length - 1);
    if (end < expected || end > expected + 1) break;
    comment.kind = "directive";
    expected = comment.line - 1;
  }
}
