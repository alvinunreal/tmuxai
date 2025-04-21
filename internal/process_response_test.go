// Unit tests for parseAIResponse in process_response.go
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
	input := "Here is some lines and than the tag.\n```\n<TmuxSendKeys>SOmething</TmuxSendKeys>\n```\nMore content\n```<ExecPaneSeemsBusy>```"
	want := AIResponse{
		Message:           "Here is some lines and than the tag.\nMore content",
		SendKeys:          []string{"SOmething"},
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
		SendKeys: []string{"foo", "bar"},
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
		SendKeys: []string{"line1\nline2"},
	}
	got, err := m.parseAIResponse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

// Test: Tags wrapped in quotes or backticks
func TestParseAIResponse_TagsInQuotesOrBackticks(t *testing.T) {
	m := &Manager{}
	input := "`<RequestAccomplished>1</RequestAccomplished>`"
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

// Test: Non-AI XML tags and code blocks are preserved
func TestParseAIResponse_NonAIXMLTagsAndBackticksPreserved(t *testing.T) {
	m := &Manager{}
	input := "This is a message with a code block:\n```\n<NotAIResponse>foo</NotAIResponse>\n```\nAnd a backtick: `<OtherTag>bar</OtherTag>`"
	want := AIResponse{
		Message: "This is a message with a code block:\n```\n<NotAIResponse>foo</NotAIResponse>\n```\nAnd a backtick: `<OtherTag>bar</OtherTag>`",
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
		SendKeys: []string{`foo & bar <baz> "qux" 'zap'`},
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
		SendKeys: []string{"foo & bar & baz"},
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
		SendKeys: []string{"line1 <tag>\nline2 & more"},
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
		SendKeys:            []string{"foo"},
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
