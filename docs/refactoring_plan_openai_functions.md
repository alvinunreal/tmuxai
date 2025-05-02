# Refactoring Plan: Integrate OpenAI Go Library for Function Calling

## Objective

Refactor the tmuxai project to replace custom XML parsing for function calls with the built-in function calling feature of the official OpenAI Golang library (`github.com/openai/openai-go`).

## Current Situation

The project currently uses a custom mechanism to parse function calls from AI responses formatted as XML within the message content. This logic is primarily located in `internal/process_response.go`. The AI client (`internal/ai_client.go`) interacts with the OpenRouter API and formats the response in a way that includes these XML tags. Tool definitions are managed in `internal/tools.go`.

## Goal

To integrate the official `github.com/openai/openai-go` library to leverage its native support for OpenAI function calling. This involves:

- Updating dependencies.
- Modifying the AI client to use the library's API for sending messages and receiving structured function call responses.
- Replacing the custom XML parsing logic with processing of the structured response from the library.
- Ensuring existing tool definitions are compatible or adapted for the new library.
- Updating prompts to reflect the change in how function calls are handled.

## Detailed Plan

1.  **Add OpenAI Go Library Dependency:**

    - Update the `go.mod` file to include `github.com/openai/openai-go`.

2.  **Update Tool Definitions (`internal/tools.go`):**

    - Modify the `Tool` and `FunctionDefinition` structs to align with the `shared.FunctionDefinitionParam` type from the OpenAI library.
    - Adjust the `tools` slice to use the new struct definitions and ensure the `Parameters` field is correctly formatted as a JSON schema compatible with the OpenAI API.

3.  **Refactor AI Client (`internal/ai_client.go`):**

    - Replace the custom `Message`, `ToolCall`, `FunctionCall`, `ChatCompletionRequest`, and `ChatCompletionResponse` structs with the corresponding types from the `github.com/openai/openai-go` library (e.g., `openai.Message`, `openai.ToolCall`, `openai.FunctionCall`, `openai.ChatCompletionNewParams`, `openai.ChatCompletion`).
    - Update the `NewAiClient` function to initialize the official OpenAI client.
    - Modify the `GetResponseFromChatMessages` function to construct the chat completion request using the new library types and call the appropriate method on the OpenAI client (likely `client.Chat.Completions.New`).
    - Update the logic that processes the API response to extract message content and tool calls directly from the structured `openai.ChatCompletion` object returned by the library.
    - Implement robust error handling using the error types provided by the OpenAI library.

4.  **Remove Custom Parsing Logic (`internal/process_response.go`):**

    - Delete the `toolCodeRegex`, `functionPattern`, `reFunctions`, `removeToolCode` variables and functions.
    - Completely rewrite or significantly modify the `parseAIResponse` function to process the structured response object from the OpenAI library instead of parsing a raw string. This function should now extract the message content and a list of tool calls.
    - Adjust the `ActionStep` struct or introduce new types to represent the actions derived from the OpenAI response (e.g., a message action and a tool call action).

5.  **Update Message Processing (`internal/process_message.go`):**

    - Modify the `ProcessUserMessage` function to use the updated `AiClient` and the new response parsing logic.
    - Update the loop that iterates through the `r.Sequence` (or equivalent) to handle the new action step types, executing the appropriate logic for message content and each tool call.

6.  **Review and Update Prompts (`internal/prompts.go`):**
    - Examine the existing prompts (`baseSystemPrompt`, `chatAssistantPrompt`, `chatAssistantPreparedPrompt`, `watchPrompt`) and remove any instructions or examples related to the old XML formatting for tool calls.
    - Ensure the prompts accurately reflect that the AI will be using built-in function calling and should respond with structured tool calls when appropriate.

## Flow Diagram

```mermaid
graph TD
    A[User Request] --> B{ProcessUserMessage};
    B --> C[Build Chat History];
    C --> D[Call AiClient.ChatCompletion];
    D --> E[OpenAI Go Library];
    E --> F[OpenAI API];
    F --> E;
    E --> G{OpenAI Response (Structured)};
    G --> H[Process Response (Extract Content & Tool Calls)];
    H --> I{Execute Actions};
    I -- Message Content --> J[Display Message];
    I -- Tool Call --> K[Execute Tool Function];
    K --> L{Update State / Loop};
    J --> L;
    L -- Request Accomplished --> M[Task Complete];
    L -- Waiting for User --> A;
    L -- Exec Pane Busy --> A;

    subgraph Refactored Components
    D
    E
    G
    H
    I
    end

    subgraph Files Involved
    B,C,I,J,K,L,M --> internal/process_message.go;
    D,E,G --> internal/ai_client.go;
    H --> internal/process_response.go;
    D,E --> internal/tools.go;
    C --> internal/prompts.go;
    end
```
