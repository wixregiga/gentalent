import { defineRule, s } from "arclint";

export default defineRule({
  type: "aggregate/members-by-identity",
  description: "report a member entity of a recorded aggregate held as a field anywhere but its root, so each entity lives once and every other member refers to it by its identity",
  capability: "exact",
  params: s.object({
    zones: s.array(s.string()).describe("Zones whose declarations are the domain model."),
  }),
  check(ctx, params) {
    const zones = params.zones as string[];
    const memberOf = new Map<string, { aggregate: string; identity: string }>();
    for (const context of ctx.domain().contexts) {
      for (const aggregate of context.aggregates ?? []) {
        for (const entity of aggregate.entities ?? []) {
          memberOf.set(entity.name, { aggregate: aggregate.name, identity: entity.identity ?? `its ${aggregate.name}-local identity` });
        }
      }
    }
    if (memberOf.size === 0) return;
    for (const file of ctx.files("**/*.go")) {
      if (file.path.endsWith("_test.go")) continue;
      const facts = ctx.facts(file.path);
      if (!facts || !facts.zones.some(zone => zones.includes(zone))) continue;
      if (!facts.declarationsAvailable || !facts.fieldTypesAvailable) {
        ctx.report({ path: file.path, line: 1, message: `field types of ${file.path} could not be read${facts.parseError ? `: ${facts.parseError}` : ""}` });
        continue;
      }
      for (const decl of facts.decls) {
        if (decl.kind !== "field") continue;
        for (const held of decl.holds ?? []) {
          if (held.resolution !== "declared" || !held.name) continue;
          const member = memberOf.get(held.name);
          if (!member || decl.owner === member.aggregate) continue;
          ctx.report({
            path: file.path,
            line: decl.startLine,
            message: `aggregate ${member.aggregate}: field ${decl.owner}.${decl.name} holds ${held.name}, a member of ${member.aggregate}; only the root holds a member, and ${decl.owner} refers to it by ${member.identity}`,
          });
        }
      }
    }
  },
});
