# Messaging

Messaging stores incoming messages and sends replies. The application uses ports
for the conversation, the inbox, the outbox, and local attachment evidence.
Only the platform adapter translates platform payloads. The composition root
passes reply bodies to `command.Enqueuer` after formatting and escaping them.

## Worker wiring

1. Compute the group binding with `worker.Binding`. Hold `worker.LockDatabase`
   through migration, arming, or polling. For a live worker, also hold
   `worker.LockLiveGroup` through shutdown.
2. Open and migrate the messaging database in composition using
   `sqlite.Migrations(pythonDatabase)`. Use the dry-run database path and the
   dry-run Python source in preview mode. Pass an empty source path for a fresh
   database. The adapter imports existing history in a single source snapshot.
3. Call `Store.StartWorker` under the worker lock. It verifies the binding and
   marks interrupted sends uncertain. Status and resolve-send use independent
   connections and skip worker startup.
4. Use `Armer.Arm` to establish a baseline and `Poller.Poll` to fetch messages after
   it. The optional `Poller.NeedsLinked` predicate receives normalized text and
   lets composition fetch linked messages only for commands that need them.
5. Drain `query.Inbox.Next` in order. Call `Acknowledger.Handled` after the
   message's effects and replies have been committed and relayed. A failure
   leaves the oldest message available for the next poll.
6. Flush with the verified platform as both sender and reader in live mode. In
   preview mode use `preview.Sender` with a supplied writer and leave the flush
   reader nil. The poller can still read the group in preview mode.

`ReactionReader.Reactions` returns nil when no delivered message is tagged and
returns an error if a tagged message cannot be read. On a successful lookup, an
empty map means the message currently has no eligible reactions. Multiple keys
from the same member are kept.

The pinned SDK covers message reads and sends, uploads, account discovery, chat
discovery, and connectivity. The adapter uses the SDK's raw GET helper for the
local API spec endpoint. Transport tests replay sanitized response shapes and
exercise the no-retry policy with an HTTP server.
