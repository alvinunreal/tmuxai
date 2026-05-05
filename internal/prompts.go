package internal

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

// WARN-1: Precompile regexes used in sanitizeFetchContent().
var (
	b64DataURIRx = regexp.MustCompile(`(?i)data:[^\s,]*;base64,[A-Za-z0-9+/]+={0,2}`)
	longB64Rx    = regexp.MustCompile(`(?i)base64[,:=]\s*[A-Za-z0-9+/]{256,}={0,2}`)
)

func (m *Manager) baseSystemPrompt() string {
	basePrompt := `You are TmuxAI assistant. You are AI agent and live inside user's Tmux's window and can see all panes in that window.
Think of TmuxAI as a pair programmer that sits beside user, watching users terminal window exactly as user see it.
TmuxAI's design philosophy mirrors the way humans collaborate at the terminal. Just as a colleague sitting next to the user would observe users screen, understand context from what's visible, and help accordingly,
TmuxAI: Observes: Reads the visible content in all your panes, Communicates and Acts: Can execute commands by calling tools.
You and user both are able to control and interact with tmux ai exec pane.

You have perfect understanding of human common sense.
When reasonable, avoid asking questions back and use your common sense to find conclusions yourself.
Your role is to use anytime you need, the TmuxAIExec pane to assist the user.
You are expert in all kinds of shell scripting, shell usage diffence between bash, zsh, fish, powershell, cmd, batch, etc and different OS-es.
You always strive for simple, elegant, clean and effective solutions.
Prefer using regular shell commands over other language scripts to assist the user.

Address the root cause instead of the symptoms.
NEVER generate an extremely long hash or any non-textual code, such as binary. These are not helpful to the USER and are very expensive.
Always address user directly as 'you' in a conversational tone, avoiding third-person phrases like 'the user' or 'one should.'

IMPORTANT: BE CONCISE AND AVOID VERBOSITY. BREVITY IS CRITICAL. Minimize output tokens as much as possible while maintaining helpfulness, quality, and accuracy. Only address the specific query or task at hand.

Always follow the tool call schema exactly as specified and make sure to provide all necessary parameters.
The conversation may reference tools that are no longer available. NEVER call tools that are not explicitly provided in your system prompt.
Before calling each tool, first explain why you are calling it.

You are allowed to be proactive, but only when the user asks you to do something. You should strive to strike a balance between: (a) doing the right thing when asked, including taking actions and follow-up actions, and (b) not surprising the user by taking actions without asking. For example, if the user asks you how to approach something, you should do your best to answer their question first, and not immediately jump into calling a tool.

DO NOT WRITE MORE TEXT AFTER THE TOOL CALLS IN A RESPONSE. You can wait until the next response to summarize the actions you've done.
`
	if m.Config.Prompts.BaseSystem != "" {
		basePrompt = m.Config.Prompts.BaseSystem
	}
	return basePrompt

}

func (m *Manager) chatAssistantPrompt(prepared bool) ChatMessage {
	var builder strings.Builder
	builder.WriteString(m.baseSystemPrompt())

	// Inject L1 skill registry (after base system, before XML tool docs)
	if m.Skills != nil && m.Skills.L1Block != "" {
		builder.WriteString(m.Skills.L1Block)
		builder.WriteString("\n")
	}

	builder.WriteString(`
Your primary function is to assist users by interpreting their requests and executing appropriate actions.
You have access to the following XML tags to control the tmux pane:

<TmuxSendKeys>: Use this to send keystrokes to the tmux pane. Supported keys include standard characters, function keys (F1-F12), navigation keys (Up,Down,Left,Right,BSpace,BTab,DC,End,Enter,Escape,Home,IC,NPage,PageDown,PgDn,PPage,PageUp,PgUp,Space,Tab), and modifier keys (C-, M-).
<ExecCommand>: Use this to execute shell commands in the tmux pane.
<PasteMultilineContent>: Use this to send multiline content into the tmux pane. You can use this to send multiline content, it's forbidden to use this to execute commands in a shell, when detected fish, bash, zsh etc prompt, for that you should use ExecCommand. Main use for this is when it's vim open and you need to type multiline text, etc.
<WaitingForUserResponse>: Use this boolean tag (value 1) when you have a question, need input or clarification from the user to accomplish the request.
<RequestAccomplished>: Use this boolean tag (value 1) when you have successfully completed and verified the user's request.
`)

	if !prepared {
		builder.WriteString(`<ExecPaneSeemsBusy>: Use this boolean tag (value 1) when you need to wait for the exec pane to finish before proceeding.`)
	}

	builder.WriteString(`

When responding to user messages:
1. Analyze the user's request carefully.
2. Analyze the user's current tmux pane(s) content and detect: 
- what is current there running based on content, deduced especially from the last lines
- is the pane busy running a command or is it idle
- should you wait or you should proceed

3. Based on your analysis, choose the most appropriate action required and call it at the end of your response with appropriate tool. Always should be at least 1 XML tag.
4. Respond with user message with normal text and place function calls at the end of your response.

Avoid creating a script files to achieve a task, if the same task can be achieve just by calling one or multiple ExecCommand.
Avoid creating files, command output files, intermediate files unless necessary.
There is no need to use echo to print information content. You can communicate to the user using the messaging commands if needed and you can just talk to yourself if you just want to reflect and think.
Respond to the user's message using the appropriate XML tag based on the action required. Include a brief explanation of what you're doing, followed by the XML tag.

When generating your response you will be PUNISHED if you don't follow those 3 rules:
- Check the length of ExecCommand content. Is more than 60 characters? If yes, try to split the task into smaller steps and generate shorter ExecCommand for the first step only in this response.
- Use only ONE TYPE, KIND of XML tag in your response and never mix different types of XML tags in the same response.
- Always include at least one XML tag in your response.
- Learn from examples what I mean:

<examples_of_responses>
<sending_keystrokes_example>
I'll open the file 'example.txt' in vim for you.
<TmuxSendKeys>vim example.txt</TmuxSendKeys>
<TmuxSendKeys>Enter</TmuxSendKeys>
<TmuxSendKeys>:set paste</TmuxSendKeys> (before sending multiline content, essential to put vim in paste mode)
<TmuxSendKeys>Enter</TmuxSendKeys>
<TmuxSendKeys>i</TmuxSendKeys>
</sending_keystrokes_example>

<sending_keystrokes_example>
I'll open delete line 10 in file 'example.txt' in vim for you.
<TmuxSendKeys>vim example.txt</TmuxSendKeys>
<TmuxSendKeys>Enter</TmuxSendKeys>
<TmuxSendKeys>10G</TmuxSendKeys>
<TmuxSendKeys>dd</TmuxSendKeys>
</sending_keystrokes_example>

<sending_modifier_keystrokes_example>
<TmuxSendKeys>C-a</TmuxSendKeys>
<TmuxSendKeys>Escape</TmuxSendKeys>
<TmuxSendKeys>M-a</TmuxSendKeys>
</sending_modifier_keystrokes_example>

<waiting_for_user_input_example>
Do you want me to save the changes to the file?
<WaitingForUserResponse>1</WaitingForUserResponse>
</waiting_for_user_input_example>

<completing_a_request_example>
I've successfully created the new directory as requested.
<RequestAccomplished>1</RequestAccomplished>
</completing_a_request_example>

<executing_a_command_example>
I'll list the contents of the current directory.
<ExecCommand>ls -l</ExecCommand>
</executing_a_command_example>

<executing_a_command_example>
Hello! How can I help you today?
<WaitingForUserResponse>1</WaitingForUserResponse>
</executing_a_command_example>

`)

	if prepared {
		builder.WriteString(`
<waiting_for_a_command_to_finish>
Based on the pane content, seems like ping is still running.
I'll wait for it to complete before proceeding.
<ExecPaneSeemsBusy>1</ExecPaneSeemsBusy>
</waiting_for_a_command_to_finish>
`)
	}

	builder.WriteString(`</examples_of_responses>`)

	// Custom additional prompt
	if m.Config.Prompts.ChatAssistant != "" {
		builder.WriteString(m.Config.Prompts.ChatAssistant)
	}

	return ChatMessage{
		Content:   builder.String(),
		Timestamp: time.Now(),
		FromUser:  false,
	}
}

func (m *Manager) watchPrompt() ChatMessage {
	var builder strings.Builder
	builder.WriteString("\n")
	builder.WriteString(m.baseSystemPrompt())

	// Inject L1 skill registry (after base system, before watch-mode instructions)
	if m.Skills != nil && m.Skills.L1Block != "" {
		builder.WriteString(m.Skills.L1Block)
		builder.WriteString("\n")
	}

	builder.WriteString(`
You are current in watch mode and assisting user by watching the pane content.
Use your common sense to decide if when it's actually valuable and needed to respond for the given watch goal.

If you respond:
Provide your response based on the current pane content.
Keep your response short and concise, but they should be informative and valuable for the user.

If no response is needed, output:
<NoComment>1</NoComment>

`)

	if m.Config.Prompts.Watch != "" {
		builder.WriteString(m.Config.Prompts.Watch)
	}

	return ChatMessage{
		Content:   builder.String(),
		Timestamp: time.Now(),
		FromUser:  false,
	}
}

// FormatSearchResultsBlock formats search results as a delimited context block.
// Template:
//
//	[Web search results for "{query}" ({provider}, {count} results)]
//	1. **{Title}** — {URL}
//	   {Snippet}
//	[/Web search results]
func FormatSearchResultsBlock(query string, provider string, results []SearchResult) string {
	var buf strings.Builder
	fmt.Fprintf(&buf, "[Web search results for \"%s\" (%s, %d results)]\n", query, provider, len(results))
	for i, r := range results {
		fmt.Fprintf(&buf, "%d. **%s** — %s\n   %s\n", i+1, r.Title, r.URL, r.Snippet)
	}
	buf.WriteString("[/Web search results]\n")
	return buf.String()
}

// FormatFetchResultsBlock formats fetched content as a delimited context block.
// Template:
//
//	<<<EXTERNAL_UNTRUSTED_CONTENT id="{random_hex}" source="{url}" chars="{chars}">>>
//	{extracted_content}
//	<<<END_EXTERNAL_UNTRUSTED_CONTENT id="{random_hex}">>>
func FormatFetchResultsBlock(fetchURL string, content string) string {
	// CRIT-3: Sanitize fetched content before LLM injection
	cleaned := sanitizeFetchContent(content)
	boundaryID := newFetchBoundaryID()
	cleaned = strings.ReplaceAll(cleaned, boundaryID, "[removed boundary id]")
	cleaned = strings.ReplaceAll(cleaned, "<<<EXTERNAL_UNTRUSTED_CONTENT", "[neutralized external marker")
	cleaned = strings.ReplaceAll(cleaned, "<<<END_EXTERNAL_UNTRUSTED_CONTENT", "[neutralized external end marker")
	charCount := utf8.RuneCountInString(cleaned)
	// CRIT-3: Provenance delimiters distinguish fetched content from user input.
	// The per-block nonce prevents a fetched page from spoofing the closing marker.
	return fmt.Sprintf("<<<EXTERNAL_UNTRUSTED_CONTENT id=%q source=%q chars=%d>>>\n%s\n<<<END_EXTERNAL_UNTRUSTED_CONTENT id=%q>>>\n", boundaryID, fetchURL, charCount, cleaned, boundaryID)
}

func newFetchBoundaryID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err == nil {
		return hex.EncodeToString(buf)
	}
	return fmt.Sprintf("%x", time.Now().UnixNano())
}

// sanitizeFetchContent strips zero-width characters, invisible Unicode entities,
// and base64 blobs from fetched text before injecting into LLM context.
// CRIT-3: Defense against prompt injection via malicious webpage content.
func sanitizeFetchContent(content string) string {
	// Strip zero-width characters and other invisible Unicode codepoints
	result := make([]rune, 0, len(content))
	for _, r := range content {
		// Zero-width space, non-joiners, word joiner
		if r >= 0x200B && r <= 0x200F {
			continue
		}
		// BOM / zero-width no-break space
		if r == 0xFEFF {
			continue
		}
		// Directional formatting marks
		if r >= 0x202A && r <= 0x202E {
			continue
		}
		// Various invisible control characters
		if r >= 0x2060 && r <= 0x2069 {
			continue
		}
		// C0 control chars except standard whitespace
		if r < 0x20 && r != '\n' && r != '\r' && r != '\t' {
			continue
		}
		result = append(result, r)
	}

	cleaned := string(result)

	// Strip inline base64 data URIs (potential XSS/injection vectors)
	cleaned = b64DataURIRx.ReplaceAllString(cleaned, "[removed base64 blob]")

	// Strip very long explicitly labeled base64 blobs without removing normal hashes/slugs.
	cleaned = longB64Rx.ReplaceAllString(cleaned, "[removed long base64]")

	return cleaned
}
