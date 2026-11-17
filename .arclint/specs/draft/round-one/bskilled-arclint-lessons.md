# ArcLint lessons from bskilled

Received from the bskilled orchestrator thread "(O) Redesign Dashboard Navigation" on 2026-11-17, in answer to the layout question. Its Go statements are marked [INFERENCE] by the sender.

## Worth keeping
1. Zone import contracts and feature independence rules. They forced the cross-feature UI slot into the app shell, at the cost of extra props.
2. go/dependencies-inward and go/dependencies-acyclic [INFERENCE]: one short layer list encodes the hexagonal direction. Adopt these and per-zone "imports only" tables first.
3. app/no-direct-io: a regex forbidding os, os/exec, net/http, database/sql imports in use cases. Exact for its pattern only; a tripwire, not a guarantee.

## Caused trouble
1. source/no-comments retrofitted: 3,843 active violations, whole-file churn, a check that always exits 1. Adopt on day one or not at all.
2. polymorphism/contract-only-consumers (bskilled extension): a hand-kept list of about 50 contract types drifts. Better: discover ports by location (ctx.files, ctx.zones, ctx.facts(path).decls with kind "interface"). ctx.facts returns null outside the rule's files scope.

For GenTalent: fix where ports live from the start as their own zone, cover it with dependencies-inward and the import contracts, have the polymorphism rule discover contracts from that zone, and optionally reject interfaces declared outside it.

## Do not port
legacy/no-retired-dashboard, storage/no-legacy-install, storage/no-legacy-lifecycle, delivery/no-retired-server-directory, delivery/no-legacy-skill-observation.

## Process
- One check at a time: `flock <lockfile> arclint check . --format json --no-baseline`. Exit 3 or any not_evaluated entry decides nothing; rerun.
- Always `--format json` and `--no-baseline`.
- Run `arclint context <paths>` before editing.

## Short list for GenTalent
1. Zones with "imports only" lists.
2. dependencies-inward and dependencies-acyclic.
3. no-direct-io on the application layer.
4. contract-only-consumers discovering ports from a fixed port zone.
