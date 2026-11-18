import { goComments } from "./go_comments";
import { typescriptComments } from "./typescript_comments";

export type CommentKind = "prose" | "suppression" | "directive" | "license" | "generated";

export interface Comment {
  line: number;
  offset: number;
  text: string;
  kind: CommentKind;
}

const licenseIdentifier = /SPDX-License-Identifier:/;

const scanners: Record<string, (source: string) => Comment[]> = {
  ".go": goComments,
  ".ts": source => typescriptComments(source, false),
  ".mts": source => typescriptComments(source, false),
  ".cts": source => typescriptComments(source, false),
  ".tsx": source => typescriptComments(source, true),
};

export function commentsIn(path: string, source: string): Comment[] | undefined {
  const dot = path.lastIndexOf(".");
  const scan = dot < 0 ? undefined : scanners[path.slice(dot)];
  const comments = scan?.(source);
  for (const comment of comments ?? []) {
    if (comment.kind === "prose" && licenseIdentifier.test(comment.text)) comment.kind = "license";
  }
  return comments;
}

export function firstLine(text: string): string {
  return text.split("\n")[0].trim();
}
