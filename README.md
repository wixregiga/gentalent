# GenTalent

This project lives entirely in WSL2 at ~/ai/gentalent. Source, SQLite state, credentials, and exports stay in Linux. It uses Python 3.12 already present on this machine; no pip packages, model provider, paid subscription, daemon, or autostart task is required.

The initial implementation is ready for an offline demo and a coordinated live connection test. Beeper installation/pairing, the actual RCS round trip, and Tumblr publication have not been verified against real accounts.

## Try the offline demo

Run these in a WSL terminal:

    cd ~/ai/gentalent
    python3 -m gentalent demo
    python3 -m unittest discover -s tests -v

The demo uses fictional people and in-memory fake external services. It demonstrates:
- nomination with the nominator's automatic final yes;
- selected exhibits and optional structured summary;
- all three final votes, unanimous certification, and a denied case;
- a permanent denial check across aliases and restarts;
- one mock Tumblr post created and then updated after the verdict;
- replay handling without duplicate votes or duplicate replies.

The printed example.tumblr.com URL is simulated. It is not a published demo.

## Beeper installed in WSL

Beeper 4.3.308 is downloaded and running through WSLg at its welcome/sign-in screen. Its local API is listening on 127.0.0.1:23373. Account sign-in, Google Messages pairing, local API authorization, and the real group round trip are still pending.

App: ~/.local/opt/beeper/Beeper-4.3.308-x86_64.AppImage

Run that path from a WSL terminal to reopen it. Quit through Beeper's menu when finished. No extra packages, sandbox-disabling switches, autostart services, Windows system changes, or networking changes were needed. Source, app, and runtime files remain on the Linux filesystem.

See INSTALLATION.md for the download checksum, observed launch result, and next human setup steps. The user approved the Linux download and launch; that approval does not need to be requested again.

## Fees and privacy, checked 2026-11-13

One Google Messages account fits Beeper Free. No separate Desktop API fee is documented in the reviewed official material. Optional paid plans exist; none is needed for this MVP. The code makes no paid model calls.

Google Messages currently uses Beeper Cloud even when this program and the Desktop API are local. Beeper's policy describes storage of messaging-service messages, contacts, and credentials on its servers, with zero-access encryption for stored chat history. This adds Beeper as a trusted service. The connection applies to the messaging account; it is not restricted to the GenTalent group. Beeper describes initial history backfill across recent chats.

The API token can read messaging data and send as your connected account. GenTalent's exact-thread allowlist narrows this program's behavior; it does not narrow the token's underlying privileges. Keep Beeper Remote Access off. Secrets are saved through hidden prompts with mode 600 in ~/.config/gentalent; never paste them into a conversation, command argument, source file, or repository. This is filesystem protection, not encryption at rest.

Official references:
- https://help.beeper.com/en_US/beeper-plus/beeper-plus-and-beeper-plus-plus-faqs
- https://help.beeper.com/en_US/chat-networks/google-messages-getting-started-guide
- https://www.beeper.com/privacy
- https://developers.beeper.com/desktop-api/auth/
- https://developers.beeper.com/desktop-api/advanced/remote-access/
- https://learn.microsoft.com/en-us/windows/wsl/tutorials/gui-apps

## Pair Google Messages after installation

Human-required steps:
1. Sign into Beeper.
2. On Android, confirm an active SIM, Google Messages as the default SMS app, and RCS status Connected.
3. In Google Messages, choose Device pairing and Google Account Pairing.
4. In Beeper, choose Settings → Accounts → Google Messages → Connect.
5. Sign into the matching Google account, complete 2FA, and approve the matching emoji on the phone.
6. Enable Desktop API in Beeper. The current authorization guide uses Settings → Integrations → Approved connections → + to create a token. Follow the installed app's current menu names.
7. Keep the Android phone online and Beeper Desktop running while using the local API.

The user must complete credentials, 2FA, and phone approval. If a computer-use tool is available later, it may navigate the surrounding setup after permission. No such control was available during this implementation.

## Configure exactly one group

    cd ~/ai/gentalent
    python3 -m gentalent init
    python3 -m gentalent set-secret
    python3 -m gentalent doctor
    python3 -m gentalent find-chat "exact words from your group title"

set-secret asks for the API token without echoing it. Discovery works while config IDs are still placeholders. find-chat prints matching metadata rather than message bodies; it does not automatically select a result. If there are several candidates, select the exact account, underlying chat, and full participant roster with the user.

    python3 -m gentalent inspect-chat 'EXACT_CHAT_ID'

Edit config.json in WSL with:
- the exact underlying Google Messages chat_id and account_id;
- the three eligible members' stable sender IDs;
- your sender ID as owner_id;
- a confirmed android or iphone platform for each member.

Do not infer a phone platform from a sender ID or display name. Match participants to the actual people. The API may omit your own account from the participants array; the adapter accepts exactly the roster, or exactly the roster minus the configured owner. Verify the owner's sender ID using a fresh message.

    python3 -m gentalent doctor --chat

This deliberately reads the configured group's metadata. A changed/incomplete roster or wrong account stops the integration. Use an unmerged underlying chat ID; a send response routed elsewhere is treated as uncertain.

## Coordinate the first round trip

Do not start voting yet. Arrange a short test with the three participants.

    python3 -m gentalent probe-read --seconds 120

While it watches, send a fresh labeled test message from each represented phone platform, including a message from the owner. Send one photo or link. The probe reports the sender, declared platform, isSender value, text preview, and attachment/link counts. It never processes nominations or votes and never replies.

Only when the exact destination and test window are confirmed:

    python3 -m gentalent probe-send --live
    python3 -m gentalent status --live

This sends one clearly labeled connectivity-test reply with a unique reference marker. It comes from the account paired to Google Messages, ordinarily your number. Ask a participant on the other platform to confirm receipt in that same thread. A pending ID or network success is not recipient confirmation. Keep this result recorded with the user before starting the actual nomination.

## Run one nomination through verdict

In production, `mise` + `pitchfork` (`mise.toml`) runs both Linux Beeper (`beeper`) and live GenTalent (`gentalent`) under `systemd --user` (`pitchfork.service`) with `boot_start = true`, isolated from T3 Code restarts/updates:

    cd ~/ai/gentalent
    mise daemons status
    mise daemons logs gentalent -n 50
    mise daemons restart

For manual/foreground control instead:

    python3 -m gentalent arm --live
    python3 -m gentalent run --live

arm establishes a baseline for new messages and is used once per ballot database. On restart, run again without rearming; the cursor and votes persist. Stop with Ctrl+C. There is no background service or autostart to disable.

Every inbound group message runs through the four-stage pipeline in `gentalent/workflow.py`:
1. Check if a keyword (`!gt`, `!!gt`, `!!Y`/`!!N`/`!!C`, `!final`, `!gthelp`/`!!gthelp`, `!gtstatus`, `!gtlist`, or an admin `!` command) was invoked.
2. Parse the requested action and which roster member sent it.
3. Validate the request against the current SQLite ballot, nominee, and permanent-denial state.
4. Execute the matching strategy and queue the `@gentalent-bot says:` reply.

Primary chat workflow in the configured group:
- `!gt Name. Pitch...` — checks the database for prior nomination or permanent denial, registers the nominee if new, records the nominator's automatic `yes` (1/3) and pitch exhibit, opens voting immediately, and states the voting rules concisely.
- `!!gt Name. Pitch...` — same nomination flow, plus runs the subscription-only multi-model media pipeline (`codex` → `claude` → `agy`) once per nominee to generate and attach a 3-photo strip (`strip.jpg`). Never invokes AI twice for an active nominee or for already certified/denied nominees.
- `!!Y` / `!!Yes`, `!!N` / `!!No`, `!!C` / `!!Clear` (case-insensitive) — any member (including the nominator) may cast, change, or clear their vote while `< 3` votes are in. Duplicate same-vote repeats, clearing an empty vote, casual banter, and post-finalization votes stay silent.
- `!final` — once 3/3 votes lock in, voting finalizes after 1 minute automatically or immediately when the initial nominator sends `!final`.
- `!gthelp` / `!!gthelp` — prints the concise rules and command list (never triggers AI).
- `!gtstatus` — shows the active nominee, per-member vote breakdown, record counts, and relevant next commands.
- `!gtlist` / `!gtlist winners` / `!gtlist losers` — lists certified winners (default) or permanently denied losers alongside the current nominee.
- `!gt summary` / `!gt exhibit ...` — shows recorded exhibits or attaches extra source reasons/links while open.

Optional pre-registration and prior-denial import via CLI:

    python3 -m gentalent register --live CANONICAL_ID 'Exact Name' --alias 'Known short name'
    python3 -m gentalent register --live CANONICAL_ID 'Exact Name' --previously-denied

The denial ledger is persistent and has no application deletion/reset command. Keep the database and backups even if a Tumblr post is removed. A different/empty database loses history and must not replace the real ledger.

Wait for all three final votes. Three yes votes certify (`Congratulations <Name>! You are officially a Generational Talent!`). Any no among the three denies permanently. An early no does not close the ballot before the third vote. Edited/deleted/hidden messages, outsiders, unrelated threads, casual banter without keywords, and the bot's own `@gentalent-bot says:` replies cannot affect the ballot.

For a dry run, omit --live. Dry and live ballots use separate SQLite databases. Register and arm in that same mode. A dry preview is never later sent as a live reply.

## Prepare and publish the public record

In the group, the owner explicitly selects the public text and gallery links:

    !public The exact case text approved for the public record.
    !public-link https://public.example/an-approved-exhibit
    !approve-public

Private exhibits and raw discussion are not automatically copied to Tumblr. Roster IDs and individual votes are not included by the renderer. The owner should not place private details in explicitly public text.

In WSL:

    python3 -m gentalent preview --live
    python3 -m gentalent export --live

The export includes a JSON public-post preview and an SVG award card for a certified case. This MVP publishes selected public links as its gallery. Downloading and reposting private photo/video attachments or uploading the SVG to Tumblr is not implemented or verified; the connection probe only reports exposed attachment metadata.

Register a Tumblr application at https://www.tumblr.com/oauth/apps and set its OAuth2 redirect URL to http://localhost:8766/callback. Set tumblr_blog in config.json to the exact lowercase hostname you own, such as your-blog.tumblr.com.

    python3 -m gentalent connect-tumblr

This prompts privately for app credentials, prints an authorization URL for you to open, checks OAuth state on a temporary local callback listener, and stores tokens privately in WSL. It does not publish. The browser approval requires you. A token with write scope is necessary; Tumblr is a separate external service.

With the exact post preview approved in the group:

    python3 -m gentalent publish --live --blog your-blog.tumblr.com

The publisher verifies blog ownership, creates one NPF post per case, and stores its ID. Later approved changes update that same post. It reads the resulting post to verify published content, then queues the public link for the next live group run. A verdict or change to the public content invalidates the approval; approve that new version before updating the post. Publish/update the completed case before moving to the next nomination.

Official Tumblr API specification: https://github.com/tumblr/docs/blob/master/api.md

## Recovery and limits

Processed message IDs, ballots, outbox entries, publication IDs, and cursors are saved in SQLite. A process lock prevents two local workers sharing a database. API write timeouts are treated as uncertain rather than automatically retried.

For a pending send, a subsequent live run checks its pending ID. If a send is uncertain/failed, inspect the exact thread and unique GT reference before recording what happened:

    python3 -m gentalent resolve-send --live OUTBOX_ID --outcome sent
    python3 -m gentalent resolve-send --live OUTBOX_ID --outcome not-sent

Only choose not-sent after checking; it makes the same reply eligible for a later send. No program can guarantee exactly-once delivery across a crash without remote idempotency support. The implementation favors pausing uncertain writes over creating duplicates.

For an uncertain Tumblr operation, inspect that exact blog, find the resulting post ID, and reconcile it:

    python3 -m gentalent reconcile-post --live POST_ID

The content must match the approved record. Never create a new post simply to work around an uncertain response. If no post exists, keep publication paused and have the operator inspect/reset the specific operation with evidence.

The MVP fixes the three-person roster in the database binding. To change membership later, stop the worker and migrate the roster deliberately while retaining the existing denial ledger and past ballots; creating a fresh empty database is not a roster-update procedure.

Remaining live checks: Linux Beeper/WSLg compatibility, phone pairing, exact thread/roster confirmation, fresh Android/iPhone message and attachment visibility, labeled reply receipt, and real Tumblr authorization/publication. No live result is claimed by the offline tests.
