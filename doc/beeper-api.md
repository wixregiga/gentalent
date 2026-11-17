# Beeper Desktop API: what GenTalent relies on

GenTalent talks to the Beeper Desktop local API (Beeper 4.3.308, API spec version 5.0.0).
The running app serves its own spec at `http://127.0.0.1:23373/v1/spec`. Treat that spec
as the source of truth and re-read it after a Beeper update.

Each item below is labeled:

- **Documented**: stated in the spec.
- **Observed**: seen in the live Google Messages group on 2026-11-16, but not promised by
  the spec. It can change in any Beeper or bridge update.

Two checks guard these assumptions:

- `python3 -m gentalent doctor --contract` reads the live spec and the three newest
  delivered replies from Beeper, and fails if items 1 to 3 no longer hold. It is
  read-only, and `mise run deploy` runs it before every restart.
- `tests/test_beeper_contract.py` replays items 3 to 6 through the send code using the
  captured response shapes. It proves the code handles them; it cannot notice Beeper
  changing.

## Sending a reply

1. **Documented.** `POST /v1/chats/{chatID}/messages` returns `{chatID, pendingMessageID}`.
   To track it, either call `GET /v1/chats/{chatID}/messages/{pendingMessageID}` or wait
   for `message.upserted` on the WebSocket. GenTalent uses the GET.
2. **Documented.** `Message.sendStatus` is optional. The schema describes it as the send
   status "when reported by the bridge", and the spec says optional fields "may be omitted
   when unknown". Its values are `SUCCESS`, `PENDING`, `FAIL_RETRIABLE`, `FAIL_PERMANENT`.
3. **Observed.** Google Messages never reported `SUCCESS` in this group. A sent message
   gets its final numeric ID while `sendStatus` still says `PENDING`
   (`"reason": "Awaiting send confirmation"`). Then the `sendStatus` key disappears.
   GenTalent therefore counts a reply as sent when `sendStatus` is `SUCCESS`, or when it is
   absent and the message ID differs from the pending ID.
4. **Observed.** Pending IDs look like `~beeper-mautrix-go_1794872979269056428_2`. The
   spec's example is `m1694783291234567`, so nothing may depend on the `~` prefix.
5. **Observed.** About 58 minutes after a send, its pending ID returned HTTP 404 (the spec
   lists 404 as a possible response but says nothing about expiry). GenTalent marks that
   reply `uncertain` and never resends it. A person checks the thread and records the
   outcome with `resolve-send`.
6. **Observed.** A reply with a photo arrives as two messages: an `IMAGE` message
   (`attachments[].fileName == "strip.jpg"`, no `text` key), then a separate `TEXT`
   message. The pending ID resolves to the `IMAGE` message.

## Reading the group

7. **Documented.** `GET /v1/chats/{chatID}/messages` pages with an opaque `cursor` and
   `direction`. Without a cursor it returns the newest page (20 items in this group).
8. **Observed.** The bot sends as the paired account, so its replies, and your own
   messages from the phone or desktop app, all carry your sender ID with `isSender: true`.
   The bot tells its own replies apart by the `@gentalent-bot says:` prefix and by
   recorded pending and final IDs.
9. **Observed.** Messages from friends' phones show `sendStatus` absent and their own
   bridge sender IDs. Nobody needs the Beeper app on a phone.

## Failure history (2026-11-16)

| What happened | Cause | Now |
|---|---|---|
| First three replies stuck `pending`; bot later crashed on every poll | Code required `sendStatus == SUCCESS`, which never came in this group (item 3) | Item 3 rule; unit and contract tests |
| Crash loop | Pending ID 404 (item 5) raised out of every poll | 404 marks `uncertain`, polling continues |
| A 404 "recovery" marked three delivered replies `uncertain` | It guessed by matching text in only the newest 20 messages, by which time the replies were hundreds of IDs back (#264 against #738) | Guessing removed; `resolve-send` checks one named message |
| A database viewer showed no recent cases | Most likely SQLite WAL (viewer not confirmed): recent commits live in `gentalent.sqlite3-wal` until a checkpoint, and WAL does not work over network filesystems ([sqlite.org/wal.html](https://www.sqlite.org/wal.html)) | Use `gentalent status`, or a viewer opened inside WSL |
