package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	aidomain "gentalent/internal/ai/domain"
	aiport "gentalent/internal/ai/port"
	mdomain "gentalent/internal/messaging/domain"
	"gentalent/internal/messaging/port"
)

type fakeChat struct {
	mu        sync.Mutex
	group     mdomain.Group
	clock     time.Time
	log       []mdomain.Message
	sent      []mdomain.Outgoing
	unread    map[string]bool
	reactions map[string][]mdomain.Reaction
}

func newFakeChat(group mdomain.Group, start time.Time) *fakeChat {
	return &fakeChat{group: group, clock: start, unread: map[string]bool{}, reactions: map[string][]mdomain.Reaction{}}
}

func (c *fakeChat) tick() time.Time {
	c.clock = c.clock.Add(time.Second)
	return c.clock
}

func (c *fakeChat) say(sender, text string) string {
	return c.reply(sender, text, "")
}

func (c *fakeChat) reply(sender, text, linkedID string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	id := fmt.Sprintf("member-message-%d", len(c.log)+1)
	c.log = append(c.log, mdomain.Message{
		ID: id, ChatID: c.group.ChatID, AccountID: c.group.AccountID, Sender: sender, Text: text,
		SortKey: strconv.Itoa(len(c.log)), SentAt: c.tick(), LinkedID: linkedID,
	})
	return id
}

func (c *fakeChat) react(messageID, sender, key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.reactions[messageID] = append(c.reactions[messageID], mdomain.Reaction{Sender: sender, Key: key})
}

func (c *fakeChat) unreadable(messageID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.unread[messageID] = true
}

func (c *fakeChat) delivered() []mdomain.Outgoing {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]mdomain.Outgoing(nil), c.sent...)
}

func (c *fakeChat) VerifyChat(ctx context.Context) error {
	return ctx.Err()
}

func (c *fakeChat) Messages(ctx context.Context, cursor string) (port.Page, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	from := 0
	if cursor != "" {
		var err error
		if from, err = strconv.Atoi(cursor); err != nil {
			return port.Page{}, err
		}
	}
	return port.Page{Messages: append([]mdomain.Message(nil), c.log[from:]...), NewestCursor: strconv.Itoa(len(c.log))}, nil
}

func (c *fakeChat) Retrieve(ctx context.Context, id string) (mdomain.Message, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.unread[id] {
		return mdomain.Message{}, errors.New("platform unreachable")
	}
	if final, pending := strings.CutPrefix(id, "pending-"); pending {
		return mdomain.Message{ID: "sent-" + final, FromAccount: true, Status: mdomain.Confirmed}, nil
	}
	for _, message := range c.log {
		if message.ID == id {
			message.Reactions = c.reactions[id]
			return message, nil
		}
	}
	return mdomain.Message{}, port.ErrMessageMissing
}

func (c *fakeChat) Send(ctx context.Context, reply mdomain.Outgoing) (port.Receipt, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sent = append(c.sent, reply)
	number := strconv.Itoa(len(c.sent))
	own := mdomain.Message{
		ID: "sent-" + number, ChatID: c.group.ChatID, AccountID: c.group.AccountID, Sender: c.group.Owner,
		Text: reply.Text, SortKey: strconv.Itoa(len(c.log)), SentAt: c.tick(), FromAccount: true,
	}
	if reply.Attachment != "" {
		own.Attachments = []mdomain.Attachment{{Name: filepath.Base(reply.Attachment)}}
	}
	c.log = append(c.log, own)
	return port.Receipt{PendingID: "pending-" + number}, nil
}

type blockingAgent struct {
	started  chan string
	release  chan struct{}
	mu       sync.Mutex
	runs     int
	canceled int
}

func newBlockingAgent() *blockingAgent {
	return &blockingAgent{started: make(chan string, 10), release: make(chan struct{})}
}

func (a *blockingAgent) Run(ctx context.Context, task aiport.Task) (aidomain.Manifest, error) {
	a.mu.Lock()
	a.runs++
	a.mu.Unlock()
	if err := os.MkdirAll(task.Raw, 0o700); err != nil {
		return aidomain.Manifest{}, err
	}
	a.started <- task.Folder
	select {
	case <-a.release:
	case <-ctx.Done():
		a.mu.Lock()
		a.canceled++
		a.mu.Unlock()
		return aidomain.Manifest{}, ctx.Err()
	}
	photos := make([]aidomain.PhotoSource, aidomain.PhotoCount)
	for i := range photos {
		name := fmt.Sprintf("0%d.jpg", i+1)
		if err := os.WriteFile(filepath.Join(task.Raw, name), []byte("raw"), 0o600); err != nil {
			return aidomain.Manifest{}, err
		}
		photos[i] = aidomain.PhotoSource{File: name, PageURL: "https://example.test/page", ImageURL: "https://example.test/image", Caption: "Shot"}
	}
	return aidomain.NewManifest("Example Person", "A fictional person.", photos)
}

func (a *blockingAgent) counts() (runs, canceled int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.runs, a.canceled
}

type copyingImages struct{}

func (copyingImages) Check(ctx context.Context, path string) error {
	return ctx.Err()
}

func (copyingImages) Optimize(ctx context.Context, source, destination string) error {
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	return os.WriteFile(destination, data, 0o600)
}

func (copyingImages) Stitch(ctx context.Context, photos []string, destination string) error {
	return os.WriteFile(destination, []byte(strings.Join(photos, "|")), 0o600)
}
