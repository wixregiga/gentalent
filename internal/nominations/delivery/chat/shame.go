package chat

import (
	"context"
	"strings"
	"unicode"
	"unicode/utf8"

	"gentalent/internal/nominations/app/command"
	"gentalent/internal/nominations/domain"
	"gentalent/internal/nominations/port"
)

const (
	shameUsage  = "Reply to their message with !gtshame reason, or tag them: !gtshame @Name reason"
	reasonLimit = 300
	quoteLimit  = 200
)

func (b *Bot) shameOver(ctx context.Context, from Member, msg Message, match []string) error {
	body := strings.TrimSpace(match[1])
	shame := command.Shame{MessageID: msg.ID, By: from.ID, At: msg.SentAt}
	if quoted := msg.ReplyTo; quoted != nil {
		switch {
		case quoted.Missing:
			return b.say(ctx, msg, "I couldn't load the message you replied to. "+shameUsage)
		case quoted.FromBot:
			return b.say(ctx, msg)
		}
		shame.Target = domain.MemberID(quoted.Author)
		shame.Shamed = domain.ShamedMessage{ID: quoted.ID, Text: quote(*quoted), SentAt: quoted.SentAt}
		if tagged, after, ok := b.tagged(body, msg.Mentions); ok {
			if tagged != shame.Target {
				return b.say(ctx, msg, "You replied to one person but tagged another. Reply without a tag, or tag without replying.")
			}
			body = after
		}
	} else {
		tagged, after, ok := b.tagged(body, msg.Mentions)
		if !ok {
			return b.say(ctx, msg, shameUsage)
		}
		shame.Target, body = tagged, after
	}
	known, err := b.names.Handle(ctx)
	if err != nil {
		return err
	}
	about := names(known).leading(body)
	if about.refused != "" {
		return b.say(ctx, msg, about.refused)
	}
	if len(about.refs) > 0 {
		shame.About = about.refs[0]
	}
	shame.Reason = inline(about.rest, reasonLimit)
	return b.shame.Handle(ctx, shame, present(b, msg.ID, nil, func(w wording, given command.ShameGiven) []port.Reply {
		return []port.Reply{w.shameReply(given)}
	}))
}

func (w wording) shameReply(given command.ShameGiven) port.Reply {
	var shame domain.Shame
	for _, recorded := range w.group.Shames() {
		if recorded.ID == given.ID {
			shame = recorded
		}
	}
	reason, said, vote, empty := "", "", "not cast", ""
	if shame.Reason != "" {
		reason = "Reason: " + literal(shame.Reason)
	}
	if shame.Shamed.Text != "" {
		said = `They said: "` + literal(shame.Shamed.Text) + `"`
	}
	if shame.TargetVote != domain.NotVoted {
		vote = strings.ToUpper(string(shame.TargetVote))
	}
	if given.GifPath == "" {
		empty = "(No shame GIFs in the bank yet.)"
	}
	return port.Reply{
		Text: lines("🔔 SHAME 🔔", w.member(shame.By)+" shames "+w.member(shame.Target)+" over "+w.name(shame.Nomination)+".",
			reason, said, "Their vote right now: "+vote, empty),
		Attachment: given.GifPath,
	}
}

func (b *Bot) tagged(body string, mentions []string) (domain.MemberID, string, bool) {
	written, ok := strings.CutPrefix(body, "@")
	if !ok {
		return "", body, false
	}
	var best Member
	bestLength, after := 0, ""
	for _, member := range b.config.Members {
		full := member.Name
		first, _, _ := strings.Cut(full, " ")
		for _, name := range []string{full, first} {
			rest, matched := cutFold(written, name)
			length := utf8.RuneCountInString(name)
			if !matched || (rest != "" && startsAlphanumeric(rest)) {
				continue
			}
			if length > bestLength || (length == bestLength && member.ID > best.ID) {
				best, bestLength, after = member, length, rest
			}
		}
	}
	if bestLength > 0 {
		return best.ID, strings.TrimSpace(after), true
	}
	var tagged []domain.MemberID
	for _, mention := range mentions {
		if member, ok := b.member(mention); ok {
			tagged = append(tagged, member.ID)
		}
	}
	if len(tagged) == 1 {
		_, rest, _ := strings.Cut(written, " ")
		return tagged[0], strings.TrimSpace(rest), true
	}
	return "", body, false
}

func cutFold(text, prefix string) (string, bool) {
	runes, wanted := []rune(text), []rune(prefix)
	if len(wanted) == 0 || len(runes) < len(wanted) || !strings.EqualFold(string(runes[:len(wanted)]), prefix) {
		return "", false
	}
	return string(runes[len(wanted):]), true
}

func startsAlphanumeric(text string) bool {
	r, _ := utf8.DecodeRuneInString(text)
	return unicode.IsLetter(r) || unicode.IsNumber(r)
}

func quote(message Quoted) string {
	if text := inline(message.Text, quoteLimit); text != "" {
		return text
	}
	if message.HasAttachment {
		return "[photo]"
	}
	return ""
}
