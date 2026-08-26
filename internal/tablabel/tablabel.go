// Package tablabel names herdr tabs after Slack agent threads with a
// small-model API call. It is the fork's one Anthropic API dependency;
// callers own triggering, sanitizing the reply, and every fallback (the
// deterministic label stands whenever Label errors).
package tablabel

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

const systemPrompt = "You label terminal tabs. The user message started a Slack thread " +
	"where a coding agent was asked to do a task. Write a label naming that task: " +
	"2 to 5 words, 30 characters maximum, no emoji, no quotes, no trailing " +
	"punctuation, no ticket or task IDs. Reply with the label only."

const relabelSystemPrompt = "You label terminal tabs. The user message is a transcript " +
	"of a Slack thread where a coding agent works on a task. Reply with exactly two " +
	"lines. Line 1: the tracker task id or PR/issue number this whole thread is about " +
	"(e.g. colony-123 or #1170) — judge from the full thread, a passing mention is not " +
	"the thread's task — or the word none when it has no id. Line 2: a label naming " +
	"what the thread is doing now: 2 to 5 words, 30 characters maximum, no emoji, " +
	"no quotes, no trailing punctuation, no ids."

// maxRootBytes caps the prompt: the root message carries the ask, and a
// label needs nothing past its opening.
const maxRootBytes = 2000

// maxTranscriptBytes is Relabel's defensive cap; the caller owns the real
// budget (assembled newest-first), so a prefix-keeping clip here only
// guards against a caller that didn't.
const maxTranscriptBytes = 400000

// Client labels threads via one fixed model.
type Client struct {
	model  string
	client anthropic.Client
}

// New returns a Client calling model with the environment's credentials
// (ANTHROPIC_API_KEY).
func New(model string) *Client {
	return &Client{model: model, client: anthropic.NewClient()}
}

func newForTest(model, baseURL string) *Client {
	return &Client{
		model: model,
		client: anthropic.NewClient(
			option.WithBaseURL(baseURL),
			option.WithAPIKey("test"),
		),
	}
}

// Label asks the model for a short tab label for the thread whose root
// message is root, flattened plain text. The reply is returned
// whitespace-trimmed but otherwise as the model wrote it.
func (c *Client) Label(ctx context.Context, root string) (string, error) {
	return c.complete(ctx, systemPrompt, clip(root, maxRootBytes))
}

// Relabel asks the model to judge, from a whole-thread transcript, which
// task id the thread is about and what it is doing now. hints are freeform
// per-user guidance lines appended to the system prompt. id is "" when the
// model judged the thread has no task id; label follows Label's contract.
func (c *Client) Relabel(ctx context.Context, transcript string, hints []string) (id, label string, err error) {
	system := relabelSystemPrompt
	if len(hints) > 0 {
		system += "\nHints from this user about naming their tabs:\n- " + strings.Join(hints, "\n- ")
	}
	reply, err := c.complete(ctx, system, clip(transcript, maxTranscriptBytes))
	if err != nil {
		return "", "", err
	}
	return parseRelabelReply(reply)
}

// parseRelabelReply splits the two-line id/label contract, tolerating a
// model that skips the id line: a single line is a label with no id.
func parseRelabelReply(reply string) (id, label string, err error) {
	lines := strings.SplitN(reply, "\n", 2)
	if len(lines) == 1 {
		return "", strings.TrimSpace(lines[0]), nil
	}
	id = strings.Trim(strings.TrimSpace(lines[0]), "[]")
	if strings.EqualFold(id, "none") {
		id = ""
	}
	label = strings.TrimSpace(lines[1])
	if id == "" && label == "" {
		return "", "", errors.New("empty completion")
	}
	return id, label, nil
}

func (c *Client) complete(ctx context.Context, system, user string) (string, error) {
	resp, err := c.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(c.model),
		MaxTokens: 64,
		System:    []anthropic.TextBlockParam{{Text: system}},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(user)),
		},
	})
	if err != nil {
		return "", err
	}
	for _, block := range resp.Content {
		if t, ok := block.AsAny().(anthropic.TextBlock); ok {
			if label := strings.TrimSpace(t.Text); label != "" {
				return label, nil
			}
		}
	}
	return "", errors.New("empty completion")
}

// clip caps s at max bytes, backing off to a rune boundary so the cut
// never emits invalid UTF-8.
func clip(s string, max int) string {
	if len(s) <= max {
		return s
	}
	for max > 0 && !utf8.RuneStart(s[max]) {
		max--
	}
	return s[:max]
}
