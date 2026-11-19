# Beeper Desktop API: what GenTalent relies on

GenTalent talks to the Beeper Desktop local API (Beeper 4.3.308, API spec version 5.0.0).
The running app serves its own spec at `http://127.0.0.1:23373/v1/spec`. Treat that spec
as the source of truth and re-read it after a Beeper update.

Each item below is labeled:

- **Documented**: stated in the spec.
- **Observed**: seen in the live Google Messages group on 2026-11-16, but not promised by
  the spec. It can change in any Beeper or bridge update.

Two checks guard these assumptions:

- `go run ./cmd/gentalent doctor --contract` reads the live spec and the three newest
  delivered replies from Beeper, and fails if items 1 to 3 no longer hold. It is
  read-only, and `mise run deploy` runs it before every restart.
- `TestCapturedDeliveryEvidenceThroughSDK` in `internal/messaging/infra/beeper/messages_test.go`
  replays items 3 to 5 and 8 through the Beeper adapter using the captured response shapes.
  It proves the code handles them; it cannot notice Beeper changing.

## Sending a reply

1. **Documented.** `POST /v1/chats/{chatID}/messages` returns `{chatID, pendingMessageID}`.
   To track it, either call `GET /v1/chats/{chatID}/messages/{pendingMessageID}` or wait
   for `message.upserted` on the WebSocket. GenTalent uses the GET.
2. **Documented.** `Message.sendStatus` is optional. The schema describes it as the send
   status "when reported by the bridge", and the spec says optional fields "may be omitted
   when unknown". Its values are `SUCCESS`, `PENDING`, `FAIL_RETRIABLE`, `FAIL_PERMANENT`.
3. **Observed.** Google Messages never reported `SUCCESS` in this group. A sent message
   gets its final numeric ID while `sendStatus` can still say `PENDING`
   (`"reason": "Awaiting send confirmation"`), usually for seconds, and on 2026-11-18 for
   8 minutes while the bridge stalled. Then the `sendStatus` key disappears. That status is
   not delivery evidence either way: after the stall, Beeper's own store marked messages
   `FAIL_RETRIABLE` that people had received, while the API left `sendStatus` out.
   GenTalent therefore counts a reply as sent once its message ID differs from the pending
   ID, unless `sendStatus` reports a failure. The API does not tell whether people received
   a message.
4. **Observed.** Pending IDs look like `~beeper-mautrix-go_1794872979269056428_2`. The
   spec's example is `m1694783291234567`, so nothing may depend on the `~` prefix.
5. **Observed.** About 58 minutes after a send, its pending ID returned HTTP 404 (the spec
   lists 404 as a possible response but says nothing about expiry). GenTalent marks that
   reply `uncertain` and never resends it. A person checks the thread and records the
   outcome with `resolve-send`.
6. **Documented.** The send body has no required fields, and `attachment` is a "Single
   attachment to send with the message". So a photo can be sent with no text, and three
   photos take three sends. GenTalent's `individual` photo layout relies on both.
   **Not yet observed:** a photo-only send through the Google Messages bridge. The first
   `!!gt` after deploying is the first real test; a rejected photo is marked `failed` and
   never resent, and the nomination text is a separate message, so it still arrives.
7. **Documented.** Message text may be plain text or Markdown; Beeper converts it to rich
   text. **Not documented:** how a single line break is treated (Markdown would join the
   lines). Replies therefore end each line inside a block with a Markdown hard break (two
   trailing spaces, invisible as plain text), separate blocks with a blank line, and never
   start a line with Markdown syntax.
   **Observed** (2026-11-16, Jordana Brewster nomination): Beeper stores the result as HTML,
   blocks as `<p>` and hard breaks as `<br>`, and phones show separate lines. A plain
   single-line reply is stored unchanged. Text comparisons and the bot's own-message check
   therefore use the text without tags, with entities decoded.
   Text people typed (shame reasons and quotes) is backslash-escaped before sending, so it
   renders as typed and cannot open a code span or link across lines. **Not yet observed:**
   how the escapes look on phones; CommonMark removes them when rendering.
8. **Observed.** A reply with a photo arrives as two messages: an `IMAGE` message
   (`attachments[].fileName == "strip.jpg"`, no `text` key), then a separate `TEXT`
   message. The pending ID resolves to the `IMAGE` message.

## Reading the group

9. **Documented.** `GET /v1/chats/{chatID}/messages` pages with an opaque `cursor` and
   `direction`. Without a cursor it returns the newest page (20 items in this group).
10. **Observed.** The bot sends as the paired account, so its replies, and your own
   messages from the phone or desktop app, all carry your sender ID with `isSender: true`.
   The bot tells its own replies apart by the `@gentalent-bot says:` prefix and by
   recorded pending and final IDs.
11. **Observed.** Messages from friends' phones show `sendStatus` absent and their own
    bridge sender IDs. Nobody needs the Beeper app on a phone.

12. **Documented.** A reply carries `linkedMessageID`, "ID of the message this is a reply
    to". `mentions` lists tagged user IDs, or is null "for legacy messages that require text
    scanning". **Observed:** both come through from Google Messages; 11 of the last 94
    messages were replies, and a reply also lists the replied-to author in `mentions`.
    `!gtshame` uses `linkedMessageID` and falls back to `mentions`.
13. **Observed.** GIFs from phones arrive as attachments with `mimeType: image/gif` and
    `isGif: true`, up to 2.9 MB so far. **Not yet observed:** the bot sending one
    (`attachment.type = "gif"`, which the spec lists).

14. **Documented.** `Message.reactions` lists `{id, reactionKey, participantID, emoji}`;
    `id` equals the participant ID when a network allows one reaction per person.
    **Observed:** Google Messages reactions from all three phones come through as plain
    emoji keys (`👍`, `😂`, `😡`), one per person, on any message including the bot's.
    Reacting creates no new message, so GenTalent re-reads the nomination announcement on
    each poll while a vote is open (the WebSocket is "at-most-once", with "no replay on
    reconnect"). **Not yet observed:** how a removed reaction or an iPhone tapback appears.
    In the `stitched` photo layout the announcement's ID is its photo, so only reactions on
    the photo count there.

## Failure history (2026-11-16)

| What happened | Cause | Now |
|---|---|---|
| First three replies stuck `pending`; bot later crashed on every poll | Code required `sendStatus == SUCCESS`, which never came in this group (item 3) | Item 3 rule; unit and contract tests |
| Crash loop | Pending ID 404 (item 5) raised out of every poll | 404 marks `uncertain`, polling continues |
| A 404 "recovery" marked three delivered replies `uncertain` | It guessed by matching text in only the newest 20 messages, by which time the replies were hundreds of IDs back (#264 against #738) | Guessing removed; `resolve-send` checks one named message |
| A database viewer showed no recent cases | Most likely SQLite WAL (viewer not confirmed): recent commits live in `nominations.sqlite3-wal` and `messaging.sqlite3-wal` until a checkpoint, and WAL does not work over network filesystems ([sqlite.org/wal.html](https://www.sqlite.org/wal.html)) | Use `gentalent status`, or a viewer opened inside WSL |
| During an 8-minute bridge stall, 11 replies stayed `pending`; the bot looked each one up every poll (about 5 times its normal Beeper load), and reaction votes on them could not count | A final ID counted only once `sendStatus` cleared, and the bridge did not clear it (item 3) | A final ID counts as sent unless `sendStatus` reports a failure |
