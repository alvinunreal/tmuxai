package internal

import (
	"testing"
	"time"

	"github.com/alvinunreal/tmuxai/config"
)

// TestTodoItem tests the basic TodoItem functionality
func TestTodoItem(t *testing.T) {
	item := TodoItem{
		ID:          1,
		Description: "Test task",
		Status:      "pending",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if item.ID != 1 {
		t.Errorf("Expected ID to be 1, got %d", item.ID)
	}
	if item.Status != "pending" {
		t.Errorf("Expected status to be pending, got %s", item.Status)
	}
}

// TestTodoList tests the TodoList functionality
func TestTodoList(t *testing.T) {
	todoList := TodoList{
		ID:    1,
		Title: "Test List",
		Items: []TodoItem{
			{ID: 1, Status: "completed"},
			{ID: 2, Status: "pending"},
			{ID: 3, Status: "in_progress"},
		},
	}

	// Test GetProgress
	completed, total := todoList.GetProgress()
	if completed != 1 {
		t.Errorf("Expected 1 completed item, got %d", completed)
	}
	if total != 3 {
		t.Errorf("Expected 3 total items, got %d", total)
	}

	// Test GetCurrentItem
	currentItem := todoList.GetCurrentItem()
	if currentItem == nil {
		t.Error("Expected to find a current item")
	} else if currentItem.ID != 2 {
		t.Errorf("Expected current item ID to be 2, got %d", currentItem.ID)
	}
}

// TestTodoListGetCurrentItemAllCompleted tests GetCurrentItem when all items are completed
func TestTodoListGetCurrentItemAllCompleted(t *testing.T) {
	todoList := TodoList{
		ID:    1,
		Title: "Test List",
		Items: []TodoItem{
			{ID: 1, Status: "completed"},
			{ID: 2, Status: "completed"},
		},
	}

	currentItem := todoList.GetCurrentItem()
	if currentItem != nil {
		t.Error("Expected no current item when all are completed")
	}
}

// TestManagerCreateTodoList tests the Manager's CreateTodoList functionality
func TestManagerCreateTodoList(t *testing.T) {
	// Create a minimal config for testing
	cfg := &config.Config{
		OpenRouter: config.OpenRouter{
			APIKey: "test-key",
		},
	}

	manager := &Manager{
		Config:      cfg,
		TodoHistory: []TodoList{},
	}

	items := []string{"Task 1", "Task 2", "Task 3"}
	todoList := manager.CreateTodoList("Test Project", items)

	if todoList == nil {
		t.Fatal("Expected todoList to be created")
	}

	if todoList.Title != "Test Project" {
		t.Errorf("Expected title to be 'Test Project', got %s", todoList.Title)
	}

	if len(todoList.Items) != 3 {
		t.Errorf("Expected 3 items, got %d", len(todoList.Items))
	}

	if manager.CurrentTodoList != todoList {
		t.Error("Expected CurrentTodoList to be set")
	}

	// Verify items are created correctly
	for i, item := range todoList.Items {
		if item.ID != i+1 {
			t.Errorf("Expected item %d to have ID %d, got %d", i, i+1, item.ID)
		}
		if item.Status != "pending" {
			t.Errorf("Expected item status to be 'pending', got %s", item.Status)
		}
		if item.Description != items[i] {
			t.Errorf("Expected item description to be '%s', got %s", items[i], item.Description)
		}
	}
}

// TestManagerUpdateTodoItem tests updating TODO item status
func TestManagerUpdateTodoItem(t *testing.T) {
	manager := &Manager{
		CurrentTodoList: &TodoList{
			ID:    1,
			Title: "Test List",
			Items: []TodoItem{
				{ID: 1, Status: "pending"},
				{ID: 2, Status: "pending"},
			},
		},
	}

	// Test successful update
	success := manager.UpdateTodoItem(1, "in_progress")
	if !success {
		t.Error("Expected update to succeed")
	}

	if manager.CurrentTodoList.Items[0].Status != "in_progress" {
		t.Errorf("Expected status to be 'in_progress', got %s", manager.CurrentTodoList.Items[0].Status)
	}

	// Test update non-existent item
	success = manager.UpdateTodoItem(99, "completed")
	if success {
		t.Error("Expected update to fail for non-existent item")
	}

	// Test with no current todo list
	manager.CurrentTodoList = nil
	success = manager.UpdateTodoItem(1, "completed")
	if success {
		t.Error("Expected update to fail when no current todo list")
	}
}

// TestManagerMarkCurrentTodoCompleted tests marking current TODO as completed
func TestManagerMarkCurrentTodoCompleted(t *testing.T) {
	manager := &Manager{
		CurrentTodoList: &TodoList{
			ID:    1,
			Title: "Test List",
			Items: []TodoItem{
				{ID: 1, Status: "completed"},
				{ID: 2, Status: "in_progress"},
				{ID: 3, Status: "pending"},
			},
		},
	}

	success := manager.MarkCurrentTodoCompleted()
	if !success {
		t.Error("Expected marking current todo as completed to succeed")
	}

	if manager.CurrentTodoList.Items[1].Status != "completed" {
		t.Errorf("Expected item 2 status to be 'completed', got %s", manager.CurrentTodoList.Items[1].Status)
	}
}

// TestManagerCompleteTodoList tests completing the entire TODO list
func TestManagerCompleteTodoList(t *testing.T) {
	todoList := &TodoList{
		ID:    1,
		Title: "Test List",
		Items: []TodoItem{
			{ID: 1, Status: "completed"},
		},
	}

	manager := &Manager{
		CurrentTodoList: todoList,
		TodoHistory:     []TodoList{},
	}

	manager.CompleteTodoList()

	if manager.CurrentTodoList != nil {
		t.Error("Expected CurrentTodoList to be nil after completion")
	}

	if len(manager.TodoHistory) != 1 {
		t.Errorf("Expected 1 item in TodoHistory, got %d", len(manager.TodoHistory))
	}

	if manager.TodoHistory[0].ID != 1 {
		t.Errorf("Expected first history item to have ID 1, got %d", manager.TodoHistory[0].ID)
	}
}

// TestAIResponseWithTodoFields tests AIResponse with TODO fields
func TestAIResponseWithTodoFields(t *testing.T) {
	response := AIResponse{
		Message:            "Working on task",
		CreateTodoList:     []string{"Task 1", "Task 2"},
		UpdateTodoStatus:   "in_progress",
		UpdateTodoID:       1,
		TodoCompleted:      true,
		RequestAccomplished: false,
	}

	if len(response.CreateTodoList) != 2 {
		t.Errorf("Expected 2 todo items, got %d", len(response.CreateTodoList))
	}

	if response.UpdateTodoStatus != "in_progress" {
		t.Errorf("Expected UpdateTodoStatus to be 'in_progress', got %s", response.UpdateTodoStatus)
	}

	if response.UpdateTodoID != 1 {
		t.Errorf("Expected UpdateTodoID to be 1, got %d", response.UpdateTodoID)
	}

	if !response.TodoCompleted {
		t.Error("Expected TodoCompleted to be true")
	}
}