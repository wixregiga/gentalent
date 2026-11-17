import { defineRule, s } from "arclint";
import type { Ctx } from "arclint";
import { commentsIn, firstLine } from "./lib/comment";
import type { Comment } from "./lib/comment";

interface Keep {
  path: string;
  comment: string;
  reason: string;
}

const proseHint = "Say it in a name, a type, or a test, or delete it. A comment about something outside our control goes in this rule's keep list with the proof.";
const suppressionHint = "Fix what it silences. A suppression proven necessary goes in this rule's keep list with the proof.";

export default defineRule({
  type: "source/no-comments",
  description: "report comments and lint or type-checker suppressions in Go, Python, and TypeScript source, except directives, license identifiers, generated files, and kept comments with recorded proof",
  capability: "heuristic",
  params: s.object({
    keep: s.array(s.object({
      path: s.string().describe("Repository-relative path of the file holding the comment."),
      comment: s.string().describe("The comment's first line, exactly as written."),
      reason: s.string().describe("Proof that the comment records something we cannot change."),
    })).default([]).describe("Comments that stay, each with the proof it is necessary."),
  }),
  check(ctx, raw) {
    const keeps: Keep[] = ((raw.keep ?? []) as Keep[]).map(({ path, comment, reason }) => ({ path, comment, reason }));
    const scope = new Set(ctx.files().map(file => file.path));
    for (const keep of keeps) {
      if (keep.reason.trim() === "") throw new Error(`keep entry for ${JSON.stringify(keep.comment)} in ${keep.path} has no reason; state the proof that it records something we cannot change`);
      if (!scope.has(keep.path)) throw new Error(`keep entry for ${JSON.stringify(keep.comment)} names ${keep.path}, which is not a file this rule judges; remove or correct the entry`);
    }
    const used = new Set<Keep>();
    for (const file of ctx.files()) {
      const comments = commentsIn(file.path, ctx.read(file.path));
      if (!comments || comments.some(comment => comment.kind === "generated")) continue;
      for (const comment of comments) {
        if (comment.kind !== "prose" && comment.kind !== "suppression") continue;
        const keep = keeps.find(keep => keep.path === file.path && keep.comment === firstLine(comment.text));
        if (keep) {
          used.add(keep);
          continue;
        }
        report(ctx, file.path, comment);
      }
    }
    for (const keep of keeps.filter(keep => !used.has(keep))) {
      ctx.report({
        path: keep.path,
        line: 1,
        message: `Keep entry for ${JSON.stringify(keep.comment)} matches no comment.`,
        fixHint: "Remove the entry, or correct its path and first line.",
      });
    }
  },
});

function report(ctx: Ctx, path: string, comment: Comment): void {
  const suppression = comment.kind === "suppression";
  ctx.report({
    path,
    line: comment.line,
    message: `${suppression ? "Suppression" : "Comment"} ${JSON.stringify(preview(comment.text))} is not allowed.`,
    fixHint: suppression ? suppressionHint : proseHint,
  });
}

function preview(text: string): string {
  const line = firstLine(text);
  return line.length > 60 ? `${line.slice(0, 57)}...` : line;
}
