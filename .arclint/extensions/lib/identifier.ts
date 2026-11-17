function letterOrUnderscore(code: number): boolean {
  return code >= 97 && code <= 122 || code >= 65 && code <= 90 || code === 95 || code > 127;
}

export function startsIdentifier(c: string, extra = ""): boolean {
  return letterOrUnderscore(c.charCodeAt(0)) || extra.includes(c);
}

export function continuesIdentifier(c: string, extra = ""): boolean {
  const code = c.charCodeAt(0);
  return letterOrUnderscore(code) || code >= 48 && code <= 57 || extra.includes(c);
}
