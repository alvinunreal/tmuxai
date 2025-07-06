// internal/process_response_test.go
package internal

import (
	"reflect"
	"testing"
)

// Test: Single tag, inline
func TestParseAIResponse_WaitingForUserResponse(t *testing.T) {
	m := &Manager{}
	input := "Just let me know what you'd like me to do. <WaitingForUserResponse>1</WaitingForUserResponse>"
	want := AIResponse{
		Message:                "Just let me know what you'd like me to do.",
		WaitingForUserResponse: true,
	}
	got, err := m.parseAIResponse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

// Test: Tag inside code block
func TestParseAIResponse_RequestAccomplished_CodeBlock(t *testing.T) {
	m := &Manager{}
	input := "Here is some lines and than the tag.\n```xml\n<RequestAccomplished>1</RequestAccomplished>\n```"
	want := AIResponse{
		Message:             "Here is some lines and than the tag.",
		RequestAccomplished: true,
	}
	got, err := m.parseAIResponse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

// Test: Multiple tags, mixed content
func TestParseAIResponse_MultipleTags_MixedContent(t *testing.T) {
	m := &Manager{}
	input := "Here is some lines and than the tag.\n```\n<TmuxSendKeys>SOmething</TmuxSendKeys>\n```\nMore content\n```<ExecPaneSeemsBusy>1</ExecPaneSeemsBusy>```"
	want := AIResponse{
		Message:           "Here is some lines and than the tag.\nMore content",
		SendKeys:          []SendKeysInfo{{Keys: "SOmething", PaneID: ""}},
		ExecPaneSeemsBusy: true,
	}
	got, err := m.parseAIResponse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

// Test: Array field extraction
func TestParseAIResponse_SendKeys_Array(t *testing.T) {
	m := &Manager{}
	input := "<TmuxSendKeys>foo</TmuxSendKeys><TmuxSendKeys>bar</TmuxSendKeys>"
	want := AIResponse{
		SendKeys: []SendKeysInfo{{Keys: "foo", PaneID: ""}, {Keys: "bar", PaneID: ""}},
	}
	got, err := m.parseAIResponse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

// Test: No tags, only message
func TestParseAIResponse_OnlyMessage(t *testing.T) {
	m := &Manager{}
	input := "Just a message with no tags."
	want := AIResponse{
		Message: "Just a message with no tags.",
	}
	got, err := m.parseAIResponse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

// Test: Only tags, no message
func TestParseAIResponse_OnlyTags(t *testing.T) {
	m := &Manager{}
	input := "<RequestAccomplished>1</RequestAccomplished>"
	want := AIResponse{
		RequestAccomplished: true,
	}
	got, err := m.parseAIResponse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

// Test: Tags with extra whitespace/newlines
func TestParseAIResponse_TagsWithWhitespace(t *testing.T) {
	m := &Manager{}
	input := "Some text\n\n<RequestAccomplished> 1 </RequestAccomplished>\n"
	want := AIResponse{
		Message:             "Some text",
		RequestAccomplished: true,
	}
	got, err := m.parseAIResponse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

// Test: NoComment tag
func TestParseAIResponse_NoComment(t *testing.T) {
	m := &Manager{}
	input := "Some text <NoComment>1</NoComment>"
	want := AIResponse{
		Message:   "Some text",
		NoComment: true,
	}
	got, err := m.parseAIResponse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

// Test: Multiline tag content
func TestParseAIResponse_SendKeys_Multiline(t *testing.T) {
	m := &Manager{}
	input := "<TmuxSendKeys>line1\nline2</TmuxSendKeys>"
	want := AIResponse{
		SendKeys: []SendKeysInfo{{Keys: "line1\nline2", PaneID: ""}},
	}
	got, err := m.parseAIResponse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

// Test: Tags wrapped in quotes or backticks should still be parsed
func TestParseAIResponse_TagsInQuotesOrBackticks(t *testing.T) {
	m := &Manager{}
	input := "The AI said `<RequestAccomplished>1</RequestAccomplished>`"
	want := AIResponse{
		Message:             "The AI said ``",
		RequestAccomplished: true,
	}
	got, err := m.parseAIResponse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

// Test: Non-AI XML tags and code blocks are preserved in the message
func TestParseAIResponse_NonAIXMLTagsAndBackticksPreserved(t *testing.T) {
	m := &Manager{}
	input := "This is a message with a code block:\n```\n<NotAIResponse>foo</NotAIResponse>\n```\nAnd a non-AI tag: <OtherTag>bar</OtherTag>"
	want := AIResponse{
		Message: "This is a message with a code block:\n```\n<NotAIResponse>foo</NotAIResponse>\n```\nAnd a non-AI tag: <OtherTag>bar</OtherTag>",
	}
	got, err := m.parseAIResponse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

// Test: XML entity decoding in TmuxSendKeys
func TestParseAIResponse_SendKeys_XMLEntities(t *testing.T) {
	m := &Manager{}
	input := "<TmuxSendKeys>foo &amp; bar &lt;baz&gt; &quot;qux&quot; &apos;zap&apos;</TmuxSendKeys>"
	want := AIResponse{
		SendKeys: []SendKeysInfo{{Keys: `foo & bar <baz> "qux" 'zap'`, PaneID: ""}},
	}
	got, err := m.parseAIResponse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

// Test: Mixed encoded and unencoded XML entities in TmuxSendKeys
func TestParseAIResponse_SendKeys_MixedEntities(t *testing.T) {
	m := &Manager{}
	input := "<TmuxSendKeys>foo &amp; bar & baz</TmuxSendKeys>"
	want := AIResponse{
		SendKeys: []SendKeysInfo{{Keys: "foo & bar & baz", PaneID: ""}},
	}
	got, err := m.parseAIResponse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

// Test: Multiline content with XML entities in TmuxSendKeys
func TestParseAIResponse_SendKeys_MultilineEntities(t *testing.T) {
	m := &Manager{}
	input := "<TmuxSendKeys>line1 &lt;tag&gt;\nline2 &amp; more</TmuxSendKeys>"
	want := AIResponse{
		SendKeys: []SendKeysInfo{{Keys: "line1 <tag>\nline2 & more", PaneID: ""}},
	}
	got, err := m.parseAIResponse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

// Test: Mixed AI and non-AI XML tags, only AI tags are stripped, others preserved
func TestParseAIResponse_MixedAIAndNonAIXMLTags(t *testing.T) {
	m := &Manager{}
	input := "Message before.\n<TmuxSendKeys>foo</TmuxSendKeys>\n```\n<NotAIResponse>foo</NotAIResponse>\n```\n<MessageTag>bar</MessageTag>\n<RequestAccomplished>1</RequestAccomplished>\nAfter."
	want := AIResponse{
		Message:             "Message before.\n```\n<NotAIResponse>foo</NotAIResponse>\n```\n<MessageTag>bar</MessageTag>\nAfter.",
		SendKeys:            []SendKeysInfo{{Keys: "foo", PaneID: ""}},
		RequestAccomplished: true,
	}
	got, err := m.parseAIResponse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

// Test: Tags with pane_id attribute
func TestParseAIResponse_WithPaneID(t *testing.T) {
	m := &Manager{}
	input := `<ExecCommand pane_id="%1">ls -l</ExecCommand><TmuxSendKeys pane_id="%2">vim</TmuxSendKeys><PasteMultilineContent pane_id="%3">hello</PasteMultilineContent>`
	want := AIResponse{
		ExecCommand:           []ExecCommandInfo{{Command: "ls -l", PaneID: "%1"}},
		SendKeys:              []SendKeysInfo{{Keys: "vim", PaneID: "%2"}},
		PasteMultilineContent: []PasteInfo{{Content: "hello", PaneID: "%3"}},
	}
	got, err := m.parseAIResponse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

// Test: Tags without pane_id attribute
func TestParseAIResponse_WithoutPaneID(t *testing.T) {
	m := &Manager{}
	input := `<ExecCommand>ls -l</ExecCommand><TmuxSendKeys>vim</TmuxSendKeys>`
	want := AIResponse{
		ExecCommand: []ExecCommandInfo{{Command: "ls -l", PaneID: ""}},
		SendKeys:    []SendKeysInfo{{Keys: "vim", PaneID: ""}},
	}
	got, err := m.parseAIResponse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}
