// Package tablabel names herdr tabs after Slack agent threads with a
// small-model API call. It is the fork's one Anthropic API dependency;
// callers own triggering, sanitizing the reply, and every fallback (the
// deterministic label stands whenever Relabel errors).
package tablabel

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

const relabelSystemPrompt = "You label terminal tabs. The user message is a transcript " +
	"of a Slack thread where a coding agent works on a task. Reply with exactly two " +
	"lines.\n" +
	"Line 1: the tracker task id or PR/issue number this whole thread is about " +
	"(e.g. PROJ-123 or #1170), or the word none when it has no id. Judge from the full " +
	"thread: a passing mention is not the thread's task. An id is text a person or the " +
	"agent wrote as the name of the work. Never build one from other words or numbers. " +
	"Text inside a URL or a link target is never an id. When unsure, answer none.\n" +
	"Line 2: a short name for the work the thread is about, meaning the defect, " +
	"feature or component (e.g. login redirect loop, csv export, rate limiter). 1 to 3 " +
	"words, 30 characters maximum, lowercase unless a proper noun, no emoji, no quotes, " +
	"no trailing punctuation, no ids. Prefer the words the thread's opening message " +
	"uses for the work, so the name stays the same as the thread grows; only when the " +
	"thread has moved on to different work, name the new work. Name the thing, not the " +
	"activity: never the current step or a status (testing, review, waiting, done, in " +
	"progress), and no filler like issue, fix, investigation or update.\n" +
	"Hints from the user may follow. They take priority: where a hint conflicts with a " +
	"rule above, follow the hint."

// relabelReminder rides after the transcript: on a 30 to 90 KB thread the
// system prompt is far from the answer, and the model drifts from its
// format and its limits without the contract restated next to the reply.
const relabelReminder = "That was the whole thread. Reply with exactly two lines: line 1 " +
	"the id or the word none, line 2 the name of the work in 1 to 3 words. The rules in " +
	"the system prompt still apply."

// maxTranscriptBytes is Relabel's defensive cap; the caller owns the real
// budget (assembled newest-first), so a prefix-keeping clip here only
// guards against a caller that didn't.
const maxTranscriptBytes = 400000

// Client labels threads via one fixed model.
type Client struct {
	model  string
	client anthropic.Client
}

// New returns a Client calling model with apiKey. The key is passed
// explicitly because the SDK's default client otherwise reads
// ANTHROPIC_API_KEY on its own.
func New(model, apiKey string) *Client {
	return &Client{model: model, client: anthropic.NewClient(option.WithAPIKey(apiKey))}
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

// Relabel asks the model to judge, from a thread transcript (as much of the
// thread as the caller has), which task id the thread is about and to name
// the work. hints are freeform
// per-user guidance lines, sent both with the system prompt and again
// after the transcript: measured on 30 to 90 KB threads, the copy before
// the transcript alone did not hold (ids and word counts drifted), and the
// copy after it alone did worse than both. id is "" when the model judged
// the thread has no task id; label is whitespace-trimmed but otherwise as
// the model wrote it.
func (c *Client) Relabel(ctx context.Context, transcript string, hints []string) (id, label string, err error) {
	system, reminder := relabelSystemPrompt, relabelReminder
	if len(hints) > 0 {
		hintLines := "\nHints from this user about naming their tabs:\n- " + strings.Join(hints, "\n- ")
		system += hintLines
		reminder += hintLines
	}
	reply, err := c.complete(ctx, system, clip(transcript, maxTranscriptBytes), reminder)
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

// complete sends user as the text blocks of one user message.
func (c *Client) complete(ctx context.Context, system string, user ...string) (string, error) {
	blocks := make([]anthropic.ContentBlockParamUnion, len(user))
	for i, text := range user {
		blocks[i] = anthropic.NewTextBlock(text)
	}
	resp, err := c.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(c.model),
		MaxTokens: 64,
		// Models that think by default spend the whole token budget on a
		// long thread before writing any text.
		Thinking: anthropic.ThinkingConfigParamUnion{OfDisabled: &anthropic.ThinkingConfigDisabledParam{}},
		System:   []anthropic.TextBlockParam{{Text: system}},
		Messages: []anthropic.MessageParam{anthropic.NewUserMessage(blocks...)},
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
