package organize

import (
	"encoding/json"
	"fmt"
	"strings"
)

// systemPrompt tells the model what shape the answer has to be.
//
// It names JSON explicitly. That is not belt and braces: the spec's own
// description of `response_format: {"type":"json_object"}` says the model
// "will not generate JSON without a system or user message instructing it to
// do so", and this has to work against servers that do not know the field at
// all.
const systemPrompt = `You arrange a person's notes into groups.

You are given notes, one per line, as "<id>: <text>". Put related notes
together. A group is worth making when its notes share a subject, a project,
a question or a piece of work — not when they merely share a word.

Rules:
- Use only the ids you were given. Never invent one.
- A note may be left out. Not everything belongs in a group.
- Put a note in one group, the best one.
- Make few, meaningful groups rather than many thin ones. A group of one is
  almost never worth making.
- Name a group in the language its notes are written in.
- A name is a short noun phrase, not a sentence. A summary is one sentence
  saying what the group is about.

Answer with JSON and nothing else, in exactly this shape:

{"groups": [{"name": "...", "summary": "...", "note_ids": ["...", "..."]}]}

If nothing is worth grouping, answer {"groups": []}.`

// renderItems lays the notes out the way the prompt describes them.
//
// Newlines inside a note are folded to spaces: the format is one note per
// line, and a note containing a line break would otherwise look like two.
func renderItems(items []Item) string {
	var b strings.Builder
	for _, item := range items {
		content := strings.Join(strings.Fields(item.Content), " ")
		if len(content) > MaxContent {
			// Rune-safe: cutting a byte slice mid-rune sends the
			// model a replacement character for a Turkish letter.
			content = string([]rune(content)[:MaxContent]) + "…"
		}
		fmt.Fprintf(&b, "%s: %s\n", item.ID, content)
	}
	return b.String()
}

// parseGroups reads the model's answer.
//
// It looks for the first JSON object in the text rather than unmarshalling
// the whole reply, because a model that was asked for JSON often wraps it in
// a code fence or a sentence, and a server that ignored `response_format`
// gives no guarantee at all. Finding the object is the difference between
// working against a local llama.cpp and not.
func parseGroups(text string) ([]Group, error) {
	raw, err := firstJSONObject(text)
	if err != nil {
		return nil, err
	}
	var answer struct {
		Groups []Group `json:"groups"`
	}
	if err := json.Unmarshal([]byte(raw), &answer); err != nil {
		return nil, fmt.Errorf("organize: the model's answer was not the expected JSON: %w", err)
	}
	return answer.Groups, nil
}

// firstJSONObject returns the first balanced {...} in s.
//
// Braces inside strings do not count, and a backslash escapes the next
// character: a note whose text contains a brace is ordinary, and counting it
// would cut the object in the wrong place.
func firstJSONObject(s string) (string, error) {
	start := strings.IndexByte(s, '{')
	if start < 0 {
		return "", fmt.Errorf("organize: the model's answer had no JSON object in it: %s", firstLine([]byte(s)))
	}

	depth, inString, escaped := 0, false, false
	for i := start; i < len(s); i++ {
		c := s[i]
		switch {
		case escaped:
			escaped = false
		case c == '\\' && inString:
			escaped = true
		case c == '"':
			inString = !inString
		case inString:
			// Nothing structural inside a string.
		case c == '{':
			depth++
		case c == '}':
			depth--
			if depth == 0 {
				return s[start : i+1], nil
			}
		}
	}
	return "", fmt.Errorf("organize: the model's answer stopped in the middle of its JSON")
}
