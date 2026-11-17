import type { Comment, CommentKind } from "./comment";
import { continuesIdentifier } from "./identifier";
import { LineIndex, lineEnd, nextOf } from "./source_span";

const stringPrefixes = new Set(["r", "u", "b", "br", "rb", "f", "fr", "rf", "t", "tr", "rt"]);
const encodingDeclaration = /^[ \t\f]*#.*?coding[:=][ \t]*[-\w.]+/;
const typeComment = /^#\s*type:/;
const suppression = /#\s*(type:\s*ignore\b|noqa\b|pyright:\s*ignore\b|pylint:\s*disable)/;
const codeStops = /[#'"]/g;
const fieldStops = /[#'"()[\]{}:!]/g;

const stringStops: Record<string, RegExp> = {
  "'": /['\\\n]/g,
  '"': /["\\\n]/g,
  "f'": /['\\\n{]/g,
  'f"': /["\\\n{]/g,
};

const formatSpecStops: Record<string, RegExp> = {
  "'": /[{}']/g,
  '"': /[{}"]/g,
};

interface StringStyle {
  quote: string;
  raw: boolean;
  formatted: boolean;
}

export function pythonComments(source: string): Comment[] {
  const scanner = new PythonScanner(source);
  scanner.code(false);
  return scanner.comments;
}

class PythonScanner {
  readonly comments: Comment[] = [];
  private i = 0;
  private readonly lines: LineIndex;

  constructor(private readonly source: string) {
    this.lines = new LineIndex(source);
  }

  code(insideField: boolean): "close" | "format" | "end" {
    const s = this.source;
    const stops = insideField ? fieldStops : codeStops;
    let depth = 0;
    for (;;) {
      const at = nextOf(stops, s, this.i);
      if (at < 0) {
        this.i = s.length;
        return "end";
      }
      const c = s[at];
      this.i = at;
      if (c === "#") {
        this.comment();
        continue;
      }
      if (c === "'" || c === '"') {
        this.string(this.styleAt(at));
        continue;
      }
      this.i = at + 1;
      if (c === "(" || c === "[" || c === "{") depth++;
      else if (c === ")" || c === "]") depth--;
      else if (c === "}") {
        if (depth === 0) return "close";
        depth--;
      } else if (c === ":" && depth === 0) return "format";
      else if (c === "!" && depth === 0 && s[at + 1] !== "=") {
        while (this.i < s.length && /[A-Za-z]/.test(s[this.i])) this.i++;
      }
    }
  }

  private styleAt(quoteAt: number): StringStyle {
    const s = this.source;
    let start = quoteAt;
    while (start > 0 && continuesIdentifier(s[start - 1])) start--;
    const prefix = s.slice(start, quoteAt).replace(/^[0-9]+/, "").toLowerCase();
    const quote = s[quoteAt];
    if (!stringPrefixes.has(prefix)) return { quote, raw: false, formatted: false };
    return { quote, raw: prefix.includes("r"), formatted: prefix.includes("f") || prefix.includes("t") };
  }

  private comment(): void {
    const s = this.source;
    const start = this.i;
    this.i = lineEnd(s, start);
    const text = s.slice(start, this.i).replace(/\r$/, "");
    const line = this.lines.lineAt(start);
    this.comments.push({ line, offset: start, text, kind: this.kindOf(start, line, text) });
  }

  private kindOf(start: number, line: number, text: string): CommentKind {
    if (suppression.test(text)) return "suppression";
    return this.isDirective(start, line, text) ? "directive" : "prose";
  }

  private isDirective(start: number, line: number, text: string): boolean {
    if (typeComment.test(text)) return true;
    if (line === 1 && start === 0 && text.startsWith("#!")) return true;
    if (line > 2) return false;
    const [first, second] = this.source.split("\n", 2).map(part => part.replace(/\r$/, ""));
    const whole = line === 1 ? first : second;
    if (!encodingDeclaration.test(whole) || whole.trim() !== text) return false;
    if (line === 1) return true;
    return first.trim() === "" || first.trim().startsWith("#");
  }

  private string(style: StringStyle): void {
    const s = this.source;
    const triple = s.startsWith(style.quote.repeat(3), this.i);
    const closing = triple ? style.quote.repeat(3) : style.quote;
    const stops = stringStops[(style.formatted ? "f" : "") + style.quote];
    this.i += closing.length;
    for (;;) {
      const at = nextOf(stops, s, this.i);
      if (at < 0) {
        this.i = s.length;
        return;
      }
      const c = s[at];
      if (c === style.quote) {
        if (s.startsWith(closing, at)) {
          this.i = at + closing.length;
          return;
        }
        this.i = at + 1;
        continue;
      }
      if (c === "\n") {
        if (!triple) {
          this.i = at;
          return;
        }
        this.i = at + 1;
        continue;
      }
      if (c === "\\") {
        if (style.formatted && !style.raw && s[at + 1] === "N" && s[at + 2] === "{") {
          this.i = Math.max(s.indexOf("}", at), at + 2) + 1;
          continue;
        }
        this.i = style.formatted && (s[at + 1] === "{" || s[at + 1] === "}") ? at + 1 : at + 2;
        continue;
      }
      if (s[at + 1] === "{") {
        this.i = at + 2;
        continue;
      }
      this.i = at + 1;
      this.field(closing);
    }
  }

  private field(closing: string): void {
    let ending = this.code(true);
    while (ending === "format") ending = this.formatSpec(closing);
  }

  private formatSpec(closing: string): "close" | "end" {
    const s = this.source;
    const stops = formatSpecStops[closing[0]];
    for (;;) {
      const at = nextOf(stops, s, this.i);
      if (at < 0) {
        this.i = s.length;
        return "end";
      }
      const c = s[at];
      if (c === "}") {
        this.i = at + 1;
        return "close";
      }
      if (c === "{") {
        this.i = at + 1;
        this.field(closing);
        continue;
      }
      if (s.startsWith(closing, at)) {
        this.i = at;
        return "end";
      }
      this.i = at + 1;
    }
  }
}
