# Beeper Linux installation result

Beeper 4.3.308 (x86_64) was downloaded from the stable endpoint resolved by Beeper's official Linux download page and launched normally through WSLg.

- App: ~/.local/opt/beeper/Beeper-4.3.308-x86_64.AppImage
- Official entry point: https://www.beeper.com/download/linux
- Source endpoint: https://api.beeper.com/desktop/download/linux/x64/stable/com.automattic.beeper.desktop
- Download host: beeper-desktop.download.beeper.com
- Size: 269727603 bytes
- SHA-256: 157e169033b95f28835bdeaff7acc523ec930440dd44ee6bc1e248c85c876646
- Format: ELF x86_64 with AppImage type2 runtime and a SquashFS v4 filesystem
- Runtime version: https://github.com/AppImage/type2-runtime/commit/dd6cebe
- Environment: Ubuntu 24.04 under WSL2, GLIBC 2.39, existing FUSE library and WSLg display
- A Beeper window was visually inspected at its welcome screen. It offered Continue with Google and Continue with Email.
- Local API listener: 127.0.0.1:23373. Public OAuth discovery returned local authorization/token endpoints.
- No account-authenticated API calls or chat reads were made.

The SHA-256 is a record of the downloaded bytes, not an independently published checksum/signature verification. An initial format check assumed the AI marker at ELF offset 8; this release lacks that marker. Its ELF format, embedded AppImage runtime, SquashFS structure, and runtime-version execution were checked before launch.

The initial launch emitted GTK accelerator assertions and a Node deprecation warning; it remained running and rendered the welcome window. No additional packages, root installation, sandbox-disabling switches, autostart service, Windows system changes, or networking changes were needed.

Beeper's own local app data lives in its normal Linux profile directories. GenTalent source remains at ~/ai/gentalent.

## Reopen

From a WSL terminal:

    ~/.local/opt/beeper/Beeper-4.3.308-x86_64.AppImage

Quit through Beeper's menu when finished. GenTalent itself is stopped with Ctrl+C in its terminal; quitting Beeper makes its API unavailable.

## Human setup still required

1. In the currently open Linux Beeper window, choose Continue with Google or Continue with Email and complete Beeper sign-in. Do not send passwords, recovery keys, or tokens through chat.
2. On the Android phone, confirm an active SIM, Google Messages as the default SMS app, and RCS status Connected.
3. In Google Messages, choose Device pairing and Google Account Pairing.
4. In Beeper, choose Settings → Accounts → Google Messages → Connect. Sign into the matching Google account, complete 2FA, and select the matching emoji on the phone.
5. After pairing, authorize GenTalent through the local Desktop API settings. The current guide uses Settings → Integrations → Approved connections → +. Store the token through the project's hidden set-secret prompt, not in chat.
6. Identify and confirm the exact underlying group, Google Messages account, three eligible sender IDs, and each phone platform. Then coordinate fresh test messages and the one clearly labeled test reply.

Sources:
- https://help.beeper.com/en_US/chat-networks/google-messages-getting-started-guide
- https://developers.beeper.com/desktop-api/auth/

No group message was sent. No Tumblr access/publication was performed. No historical nominee eligibility or verdict was assumed.
