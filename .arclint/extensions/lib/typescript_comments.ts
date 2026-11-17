import type { Comment, CommentKind } from "./comment";
import { continuesIdentifier, startsIdentifier } from "./identifier";
import { afterNext, blockCommentEnd, LineIndex, lineEnd, nextOf, quotedEnd } from "./source_span";

const expressionKeywords = new Set([
  "return", "typeof", "instanceof", "in", "of", "new", "delete", "void", "throw",
  "case", "do", "else", "yield", "await", "extends", "keyof", "satisfies", "as",
]);
const conditionKeywords = new Set(["if", "while", "for", "with"]);
const blockKeywords = new Set(["else", "do", "try", "finally"]);
const tripleSlash = /^\/\/\/\s*<(reference|amd-module|amd-dependency)\b/;
const checkerSwitch = /^\/[/*]\**\s*@ts-check\b/;
const suppression = /^\/[/*]\**\s*(@ts-(expect-error|ignore|nocheck)\b|eslint-disable|biome-ignore\b)/;
const jsxPragma = /@jsx(Frag|ImportSource|Runtime)?\s+\S/;
const codeStops = /[\/'"`(){}[\]]/g;
const tsxCodeStops = /[\/'"`(){}[\]<]/g;
const doubleQuoteStops = /["\\\n]/g;
const singleQuoteStops = /['\\\n]/g;
const templateStops = /[`\\$]/g;
const regexStops = /[\\[\]\/\n]/g;
const jsxTagStops = /[\/>"'{<]/g;
const jsxChildStops = /[{<]/g;

function isWhitespace(c: string): boolean {
  const code = c.charCodeAt(0);
  if (code === 32 || code === 9 || code === 10 || code === 13) return true;
  return (code === 11 || code === 12 || code > 127) && /\s/.test(c);
}

function isAsciiWhitespace(c: string): boolean {
  const code = c.charCodeAt(0);
  return code === 32 || code >= 9 && code <= 13;
}

function isDigit(c: string | undefined): boolean {
  const code = c === undefined ? -1 : c.charCodeAt(0);
  return code >= 48 && code <= 57;
}

function isKeyword(name: string): boolean {
  return expressionKeywords.has(name) || conditionKeywords.has(name) || blockKeywords.has(name);
}

function kindOf(text: string): CommentKind {
  if (suppression.test(text)) return "suppression";
  if (tripleSlash.test(text) || checkerSwitch.test(text) || jsxPragma.test(text)) return "directive";
  return "prose";
}

interface Previous {
  text: string;
  endsExpression: boolean;
}

interface Token {
  text: string;
  kind: "name" | "number" | "operator";
  end: number;
}

interface Chunk {
  start: number;
  end: number;
}

export function typescriptComments(source: string, tsx: boolean): Comment[] {
  const scanner = new TypeScriptScanner(source, tsx);
  scanner.skipHashbang();
  scanner.code(false);
  return scanner.comments;
}

class TypeScriptScanner {
  readonly comments: Comment[] = [];
  private i = 0;
  private readonly lines: LineIndex;

  constructor(private readonly source: string, private readonly tsx: boolean) {
    this.lines = new LineIndex(source);
  }

  skipHashbang(): void {
    if (this.source.startsWith("#!")) this.i = lineEnd(this.source, 0);
  }

  code(nested: boolean): void {
    const s = this.source;
    const stops = this.tsx ? tsxCodeStops : codeStops;
    const parens: boolean[] = [];
    const braces: boolean[] = [];
    let previous: Previous = { text: nested ? "${" : "", endsExpression: false };
    let segment = this.i;
    for (;;) {
      const at = nextOf(stops, s, this.i);
      if (at < 0) {
        this.i = s.length;
        return;
      }
      const c = s[at];
      if (c === "/" || c === "<" || c === "(" || c === "{") previous = this.lastToken(segment, at, previous);
      this.i = at;
      if (c === "/") {
        if (this.comment()) {
          segment = this.i;
          continue;
        }
        if (previous.endsExpression) {
          this.i = at + 1;
          previous = { text: "/", endsExpression: false };
        } else {
          this.regex();
          previous = { text: "regex", endsExpression: true };
        }
      } else if (c === "'" || c === '"') {
        this.i = quotedEnd(s, at + 1, c === '"' ? doubleQuoteStops : singleQuoteStops);
        previous = { text: "string", endsExpression: true };
      } else if (c === "`") {
        this.template();
        previous = { text: "template", endsExpression: true };
      } else if (c === "<") {
        if (!previous.endsExpression && this.startsJsx()) {
          this.jsxElement();
          previous = { text: "jsx", endsExpression: true };
        } else {
          this.i = at + 1;
          previous = { text: "<", endsExpression: false };
        }
      } else if (c === "(") {
        parens.push(conditionKeywords.has(previous.text));
        this.i = at + 1;
        previous = { text: "(", endsExpression: false };
      } else if (c === ")") {
        const condition = parens.pop() ?? false;
        this.i = at + 1;
        previous = { text: ")", endsExpression: !condition };
      } else if (c === "[" || c === "]") {
        this.i = at + 1;
        previous = { text: c, endsExpression: c === "]" };
      } else if (c === "{") {
        braces.push(this.opensExpression(previous));
        this.i = at + 1;
        previous = { text: "{", endsExpression: false };
      } else {
        this.i = at + 1;
        if (braces.length === 0 && nested) return;
        previous = { text: "}", endsExpression: braces.pop() ?? false };
      }
      segment = this.i;
    }
  }

  private lastToken(start: number, end: number, before: Previous): Previous {
    const s = this.source;
    let last = end;
    while (last > start && isWhitespace(s[last - 1])) last--;
    if (last === start) return before;
    const c = s[last - 1];
    if (continuesIdentifier(c, "$\\")) {
      let first = last - 1;
      while (first > start && continuesIdentifier(s[first - 1], "$\\") && !isWhitespace(s[first - 1])) first--;
      if (isDigit(s[first])) return { text: "number", endsExpression: true };
      let p = first;
      while (p > start && isWhitespace(s[p - 1])) p--;
      if (p > start && s[p - 1] === "." && isDigit(s[p - 2])) return this.tokenizedLastToken(start, end, before);
      const name = s.slice(first, last);
      const dotted = p > start ? s[p - 1] === "." : before.text === "." || before.text === "?.";
      return { text: name, endsExpression: dotted || !isKeyword(name) };
    }
    if ("+-!>.".includes(c)) return this.tokenizedLastToken(start, end, before);
    return { text: c, endsExpression: false };
  }

  private tokenizedLastToken(start: number, end: number, before: Previous): Previous {
    const chunk = this.lastChunk(start, end);
    if (!chunk) return before;
    const tokens = this.tokens(chunk);
    return this.resolve(
      tokens,
      tokens.length - 1,
      () => this.tokenizedLastToken(start, chunk.start, before),
      () => this.lastText(start, chunk.start, before),
    );
  }

  private lastText(start: number, end: number, before: Previous): string {
    const chunk = this.lastChunk(start, end);
    if (!chunk) return before.text;
    const tokens = this.tokens(chunk);
    return tokens[tokens.length - 1].text;
  }

  private lastChunk(start: number, end: number): Chunk | undefined {
    const s = this.source;
    let last = end;
    while (last > start && isWhitespace(s[last - 1])) last--;
    if (last === start) return undefined;
    let first = last;
    while (first > start && !isAsciiWhitespace(s[first - 1])) first--;
    return { start: first, end: last };
  }

  private tokens(chunk: Chunk): Token[] {
    const s = this.source;
    const tokens: Token[] = [];
    let i = chunk.start;
    while (i < chunk.end) {
      const c = s[i];
      if (isWhitespace(c)) {
        i++;
        continue;
      }
      const start = i;
      if (startsIdentifier(c, "$\\")) {
        i++;
        while (i < chunk.end && continuesIdentifier(s[i], "$\\") && !isWhitespace(s[i])) i++;
        tokens.push({ text: s.slice(start, i), kind: "name", end: i });
        continue;
      }
      if (isDigit(c) || c === "." && isDigit(s[i + 1])) {
        const hexadecimal = c === "0" && (s[i + 1] === "x" || s[i + 1] === "X");
        i++;
        while (i < chunk.end && /[A-Za-z0-9_.]/.test(s[i])) {
          const exponent = !hexadecimal && (s[i] === "e" || s[i] === "E") && (s[i + 1] === "+" || s[i + 1] === "-");
          i += exponent ? 2 : 1;
        }
        tokens.push({ text: "number", kind: "number", end: i });
        continue;
      }
      const pair = s.slice(i, i + 2);
      const paired = pair === "=>" || pair === "++" || pair === "--" || pair === "?." && !isDigit(s[i + 2]);
      i += paired ? 2 : 1;
      tokens.push({ text: s.slice(start, i), kind: "operator", end: i });
    }
    return tokens;
  }

  private resolve(tokens: Token[], index: number, outer: () => Previous, outerText: () => string): Previous {
    const token = tokens[index];
    const predecessor = () => index > 0 ? this.resolve(tokens, index - 1, outer, outerText) : outer();
    if (token.kind === "number") return { text: "number", endsExpression: true };
    if (token.kind === "name") {
      const before = index > 0 ? tokens[index - 1].text : outerText();
      return { text: token.text, endsExpression: before === "." || before === "?." || !isKeyword(token.text) };
    }
    if (token.text === "++" || token.text === "--") return { text: token.text, endsExpression: predecessor().endsExpression };
    if (token.text === "!") return { text: "!", endsExpression: this.source[token.end] !== "=" && predecessor().endsExpression };
    return { text: token.text, endsExpression: false };
  }

  private opensExpression(previous: Previous): boolean {
    if (previous.endsExpression) return false;
    if (["", ";", "{", "}", "=>", ")"].includes(previous.text)) return false;
    return !blockKeywords.has(previous.text);
  }

  private comment(): boolean {
    const s = this.source;
    const start = this.i;
    if (s[start] !== "/") return false;
    if (s[start + 1] === "/") this.i = lineEnd(s, start);
    else if (s[start + 1] === "*") this.i = blockCommentEnd(s, start);
    else return false;
    const text = s.slice(start, this.i).replace(/\r$/, "");
    this.comments.push({ line: this.lines.lineAt(start), offset: start, text, kind: kindOf(text) });
    return true;
  }

  private template(): void {
    const s = this.source;
    this.i++;
    for (;;) {
      const at = nextOf(templateStops, s, this.i);
      if (at < 0) {
        this.i = s.length;
        return;
      }
      const c = s[at];
      if (c === "`") {
        this.i = at + 1;
        return;
      }
      if (c === "\\") {
        this.i = at + 2;
        continue;
      }
      this.i = at + 1;
      if (s[at + 1] === "{") {
        this.i = at + 2;
        this.code(true);
      }
    }
  }

  private regex(): void {
    const s = this.source;
    let inClass = false;
    this.i++;
    for (;;) {
      const at = nextOf(regexStops, s, this.i);
      if (at < 0) {
        this.i = s.length;
        break;
      }
      const c = s[at];
      if (c === "\n") {
        this.i = at;
        break;
      }
      if (c === "\\") {
        this.i = at + 2;
        continue;
      }
      this.i = at + 1;
      if (c === "[") inClass = true;
      else if (c === "]") inClass = false;
      else if (!inClass) break;
    }
    while (this.i < s.length && /[A-Za-z0-9_$]/.test(s[this.i])) this.i++;
  }

  private startsJsx(): boolean {
    const after = this.source.slice(this.i + 1, this.i + 200);
    if (after.startsWith(">")) return true;
    const opening = /^([A-Za-z_$\u0080-￿][\w$\u0080-￿]*)\s*(,|extends\b)?/.exec(after);
    return opening !== null && opening[2] === undefined;
  }

  private jsxElement(): void {
    const s = this.source;
    this.i++;
    for (;;) {
      const at = nextOf(jsxTagStops, s, this.i);
      if (at < 0) {
        this.i = s.length;
        return;
      }
      const c = s[at];
      this.i = at;
      if (c === "/") {
        if (this.comment()) continue;
        if (s[at + 1] === ">") {
          this.i = at + 2;
          return;
        }
        this.i = at + 1;
      } else if (c === ">") {
        this.i = at + 1;
        this.jsxChildren();
        return;
      } else if (c === '"' || c === "'") {
        this.i = afterNext(c, s, at + 1);
      } else if (c === "{") {
        this.i = at + 1;
        this.code(true);
      } else if (this.startsJsx()) {
        this.jsxElement();
      } else {
        this.i = at + 1;
      }
    }
  }

  private jsxChildren(): void {
    const s = this.source;
    for (;;) {
      const at = nextOf(jsxChildStops, s, this.i);
      if (at < 0) {
        this.i = s.length;
        return;
      }
      if (s[at] === "{") {
        this.i = at + 1;
        this.code(true);
        continue;
      }
      this.i = at;
      if (/^<\s*\//.test(s.slice(at, at + 64))) {
        this.i = afterNext(">", s, at);
        return;
      }
      this.jsxElement();
    }
  }
}
