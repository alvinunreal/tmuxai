package internal

import (
	"strings"
	"testing"

	"github.com/alvinunreal/tmuxai/config"
)

// TestParseAIResponseWithTodoTags tests parsing AI responses with TODO XML tags
func TestParseAIResponseWithTodoTags(t *testing.T) {
	manager := &Manager{
		Config: &config.Config{},
	}

	tests := []struct {
		name     string
		response string
		expected AIResponse
	}{
		{
			name: "CreateTodoList tags",
			response: `I'll create a TODO list for this complex task.
<CreateTodoList>Set up project structure</CreateTodoList>
<CreateTodoList>Install dependencies</CreateTodoList>
<CreateTodoList>Write tests</CreateTodoList>`,
			expected: AIResponse{
				Message:        "I'll create a TODO list for this complex task.",
				CreateTodoList: []string{"Set up project structure", "Install dependencies", "Write tests"},
			},
		},
		{
			name: "TodoCompleted tag",
			response: `Task completed successfully!
<TodoCompleted>1</TodoCompleted>`,
			expected: AIResponse{
				Message:       "Task completed successfully!",
				TodoCompleted: true,
			},
		},
		{
			name: "UpdateTodoStatus and UpdateTodoID tags",
			response: `Updating TODO item status.
<UpdateTodoStatus>in_progress</UpdateTodoStatus>
<UpdateTodoID>2</UpdateTodoID>`,
			expected: AIResponse{
				Message:          "Updating TODO item status.",
				UpdateTodoStatus: "in_progress",
				UpdateTodoID:     2,
			},
		},
		{
			name: "Mixed with existing tags",
			response: `Starting work on the next item.
<UpdateTodoStatus>in_progress</UpdateTodoStatus>
<UpdateTodoID>3</UpdateTodoID>
<ExecCommand>npm install</ExecCommand>`,
			expected: AIResponse{
				Message:          "Starting work on the next item.",
				UpdateTodoStatus: "in_progress",
				UpdateTodoID:     3,
				ExecCommand:      []string{"npm install"},
			},
		},
		{
			name: "Boolean TodoCompleted variations",
			response: `Done!
<TodoCompleted/>`,
			expected: AIResponse{
				Message:       "Done!",
				TodoCompleted: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := manager.parseAIResponse(tt.response)
			if err != nil {
				t.Fatalf("parseAIResponse failed: %v", err)
			}

			// Check Message
			if result.Message != tt.expected.Message {
				t.Errorf("Expected message '%s', got '%s'", tt.expected.Message, result.Message)
			}

			// Check CreateTodoList
			if len(result.CreateTodoList) != len(tt.expected.CreateTodoList) {
				t.Errorf("Expected %d CreateTodoList items, got %d", len(tt.expected.CreateTodoList), len(result.CreateTodoList))
			}
			for i, expected := range tt.expected.CreateTodoList {
				if i >= len(result.CreateTodoList) || result.CreateTodoList[i] != expected {
					t.Errorf("Expected CreateTodoList[%d] to be '%s', got '%s'", i, expected, result.CreateTodoList[i])
				}
			}

			// Check UpdateTodoStatus
			if result.UpdateTodoStatus != tt.expected.UpdateTodoStatus {
				t.Errorf("Expected UpdateTodoStatus '%s', got '%s'", tt.expected.UpdateTodoStatus, result.UpdateTodoStatus)
			}

			// Check UpdateTodoID
			if result.UpdateTodoID != tt.expected.UpdateTodoID {
				t.Errorf("Expected UpdateTodoID %d, got %d", tt.expected.UpdateTodoID, result.UpdateTodoID)
			}

			// Check TodoCompleted
			if result.TodoCompleted != tt.expected.TodoCompleted {
				t.Errorf("Expected TodoCompleted %t, got %t", tt.expected.TodoCompleted, result.TodoCompleted)
			}

			// Check ExecCommand
			if len(result.ExecCommand) != len(tt.expected.ExecCommand) {
				t.Errorf("Expected %d ExecCommand items, got %d", len(tt.expected.ExecCommand), len(result.ExecCommand))
			}
		})
	}
}

// TestProcessTodoOperations tests the processTodoOperations function
func TestProcessTodoOperations(t *testing.T) {
	manager := &Manager{
		Config:      &config.Config{},
		TodoHistory: []TodoList{},
	}

	// Test creating TODO list
	createResponse := AIResponse{
		CreateTodoList: []string{"Task 1", "Task 2", "Task 3"},
	}

	manager.processTodoOperations(createResponse)

	if manager.CurrentTodoList == nil {
		t.Fatal("Expected CurrentTodoList to be created")
	}

	if len(manager.CurrentTodoList.Items) != 3 {
		t.Errorf("Expected 3 TODO items, got %d", len(manager.CurrentTodoList.Items))
	}

	// First item should be in_progress
	if manager.CurrentTodoList.Items[0].Status != "in_progress" {
		t.Errorf("Expected first item to be in_progress, got %s", manager.CurrentTodoList.Items[0].Status)
	}

	// Test updating specific TODO item
	updateResponse := AIResponse{
		UpdateTodoStatus: "completed",
		UpdateTodoID:     1,
	}

	manager.processTodoOperations(updateResponse)

	if manager.CurrentTodoList.Items[0].Status != "completed" {
		t.Errorf("Expected item 1 to be completed, got %s", manager.CurrentTodoList.Items[0].Status)
	}

	// Test marking current TODO as completed
	completeResponse := AIResponse{
		TodoCompleted: true,
	}

	manager.processTodoOperations(completeResponse)

	// Should move to next item and mark it in_progress
	currentItem := manager.CurrentTodoList.GetCurrentItem()
	if currentItem == nil {
		t.Error("Expected to find current item after completing first")
	} else if currentItem.Status != "in_progress" {
		t.Errorf("Expected current item to be in_progress, got %s", currentItem.Status)
	}
}

// TestFormatTodoList tests the TODO list formatting
func TestFormatTodoList(t *testing.T) {
	manager := &Manager{
		CurrentTodoList: &TodoList{
			Title: "Test Project",
			Items: []TodoItem{
				{ID: 1, Description: "Completed task", Status: "completed"},
				{ID: 2, Description: "In progress task", Status: "in_progress"},
				{ID: 3, Description: "Pending task", Status: "pending"},
			},
		},
	}

	formatted := manager.FormatTodoList()

	// Check that it contains the title
	if !strings.Contains(formatted, "Test Project") {
		t.Error("Expected formatted output to contain title")
	}

	// Check that it contains progress
	if !strings.Contains(formatted, "(1/3)") {
		t.Error("Expected formatted output to contain progress (1/3)")
	}

	// Check that it contains task descriptions
	if !strings.Contains(formatted, "Completed task") {
		t.Error("Expected formatted output to contain task descriptions")
	}

	// Check for different status symbols
	if !strings.Contains(formatted, "☑") { // completed
		t.Error("Expected formatted output to contain completed checkbox")
	}

	// Test with no current todo list
	manager.CurrentTodoList = nil
	formatted = manager.FormatTodoList()
	if formatted != "" {
		t.Error("Expected empty string when no current todo list")
	}
}