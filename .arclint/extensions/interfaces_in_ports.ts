import { defineRule, s } from "arclint";

export default defineRule({
  type: "ports/interfaces-in-port-zone",
  description: "report a Go interface declared outside the zones that hold ports, so every abstraction a reader meets is a port found in one place",
  capability: "exact",
  params: s.object({
    portZones: s.array(s.string()).describe("Zones whose files may declare interfaces."),
  }),
  check(ctx, params) {
    const portZones = params.portZones as string[];
    for (const file of ctx.files("**/*.go")) {
      if (file.path.endsWith("_test.go")) continue;
      const facts = ctx.facts(file.path);
      if (!facts) continue;
      if (!facts.declarationsAvailable) {
        ctx.report({ path: file.path, line: 1, message: `declarations of ${file.path} could not be read${facts.parseError ? `: ${facts.parseError}` : ""}` });
        continue;
      }
      if (facts.zones.some(zone => portZones.includes(zone))) continue;
      for (const decl of facts.decls) {
        if (decl.kind !== "interface") continue;
        ctx.report({
          path: file.path,
          line: decl.startLine,
          message: `interface ${decl.name} is declared outside a port folder; declare it in the hexagon's port package, or use the concrete type`,
        });
      }
    }
  },
});
