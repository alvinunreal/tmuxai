package internal

import (
	"bufio"
	"bytes"
	"testing"
	"time"
)

func TestHandleEscapeSequence_LeftArrow(t *testing.T) {
	buffer := []rune{'a', 'b', 'c'}
	cursor := 3
	redrawCalled := false
	beepCalled := false

	redraw := func() { redrawCalled = true }
	beep := func() { beepCalled = true }

	// Test left arrow moves cursor left
	seq := []byte{27, '[', 'D'}
	handleEscapeSequence(seq, &buffer, &cursor, redraw, beep)

	if cursor != 2 {
		t.Errorf("Expected cursor at position 2, got %d", cursor)
	}
	if !redrawCalled {
		t.Error("Expected redraw to be called")
	}
	if beepCalled {
		t.Error("Expected beep not to be called")
	}
}

func TestHandleEscapeSequence_LeftArrowAtStart(t *testing.T) {
	buffer := []rune{'a', 'b', 'c'}
	cursor := 0
	beepCalled := false

	redraw := func() {}
	beep := func() { beepCalled = true }

	// Test left arrow at start position beeps
	seq := []byte{27, '[', 'D'}
	handleEscapeSequence(seq, &buffer, &cursor, redraw, beep)

	if cursor != 0 {
		t.Errorf("Expected cursor to stay at position 0, got %d", cursor)
	}
	if !beepCalled {
		t.Error("Expected beep to be called when at start")
	}
}

func TestHandleEscapeSequence_RightArrow(t *testing.T) {
	buffer := []rune{'a', 'b', 'c'}
	cursor := 1
	redrawCalled := false
	beepCalled := false

	redraw := func() { redrawCalled = true }
	beep := func() { beepCalled = true }

	// Test right arrow moves cursor right
	seq := []byte{27, '[', 'C'}
	handleEscapeSequence(seq, &buffer, &cursor, redraw, beep)

	if cursor != 2 {
		t.Errorf("Expected cursor at position 2, got %d", cursor)
	}
	if !redrawCalled {
		t.Error("Expected redraw to be called")
	}
	if beepCalled {
		t.Error("Expected beep not to be called")
	}
}

func TestHandleEscapeSequence_RightArrowAtEnd(t *testing.T) {
	buffer := []rune{'a', 'b', 'c'}
	cursor := 3
	beepCalled := false

	redraw := func() {}
	beep := func() { beepCalled = true }

	// Test right arrow at end position beeps
	seq := []byte{27, '[', 'C'}
	handleEscapeSequence(seq, &buffer, &cursor, redraw, beep)

	if cursor != 3 {
		t.Errorf("Expected cursor to stay at position 3, got %d", cursor)
	}
	if !beepCalled {
		t.Error("Expected beep to be called when at end")
	}
}

func TestHandleEscapeSequence_Home(t *testing.T) {
	buffer := []rune{'a', 'b', 'c'}
	cursor := 2
	redrawCalled := false

	redraw := func() { redrawCalled = true }
	beep := func() {}

	// Test Home key moves to start
	seq := []byte{27, '[', 'H'}
	handleEscapeSequence(seq, &buffer, &cursor, redraw, beep)

	if cursor != 0 {
		t.Errorf("Expected cursor at position 0, got %d", cursor)
	}
	if !redrawCalled {
		t.Error("Expected redraw to be called")
	}
}

func TestHandleEscapeSequence_End(t *testing.T) {
	buffer := []rune{'a', 'b', 'c'}
	cursor := 0
	redrawCalled := false

	redraw := func() { redrawCalled = true }
	beep := func() {}

	// Test End key moves to end
	seq := []byte{27, '[', 'F'}
	handleEscapeSequence(seq, &buffer, &cursor, redraw, beep)

	if cursor != 3 {
		t.Errorf("Expected cursor at position 3, got %d", cursor)
	}
	if !redrawCalled {
		t.Error("Expected redraw to be called")
	}
}

func TestHandleEscapeSequence_Delete(t *testing.T) {
	buffer := []rune{'a', 'b', 'c'}
	cursor := 1
	redrawCalled := false

	redraw := func() { redrawCalled = true }
	beep := func() {}

	// Test Delete key removes character at cursor
	seq := []byte{27, '[', '3', '~'}
	handleEscapeSequence(seq, &buffer, &cursor, redraw, beep)

	if len(buffer) != 2 {
		t.Errorf("Expected buffer length 2, got %d", len(buffer))
	}
	expected := []rune{'a', 'c'}
	if string(buffer) != string(expected) {
		t.Errorf("Expected buffer %v, got %v", expected, buffer)
	}
	if !redrawCalled {
		t.Error("Expected redraw to be called")
	}
}

func TestHandleEscapeSequence_UpDownArrows(t *testing.T) {
	buffer := []rune{'a', 'b', 'c'}
	cursor := 1
	beepCalled := false

	redraw := func() {}
	beep := func() { beepCalled = true }

	// Test up arrow beeps (no history support)
	seq := []byte{27, '[', 'A'}
	handleEscapeSequence(seq, &buffer, &cursor, redraw, beep)
	if !beepCalled {
		t.Error("Expected beep to be called for up arrow")
	}

	beepCalled = false
	// Test down arrow beeps (no history support)
	seq = []byte{27, '[', 'B'}
	handleEscapeSequence(seq, &buffer, &cursor, redraw, beep)
	if !beepCalled {
		t.Error("Expected beep to be called for down arrow")
	}
}

func TestHandleEscapeSequence_InvalidSequence(t *testing.T) {
	buffer := []rune{'a', 'b', 'c'}
	cursor := 1
	beepCalled := false

	redraw := func() {}
	beep := func() { beepCalled = true }

	// Test invalid escape sequence beeps
	seq := []byte{27, 'X'}
	handleEscapeSequence(seq, &buffer, &cursor, redraw, beep)
	if !beepCalled {
		t.Error("Expected beep to be called for invalid sequence")
	}
}

func TestHandleEscapeSequence_ShortSequence(t *testing.T) {
	buffer := []rune{'a', 'b', 'c'}
	cursor := 1
	originalCursor := cursor

	redraw := func() {}
	beep := func() {}

	// Test sequence too short does nothing
	seq := []byte{27}
	handleEscapeSequence(seq, &buffer, &cursor, redraw, beep)
	if cursor != originalCursor {
		t.Errorf("Expected cursor to stay at %d, got %d", originalCursor, cursor)
	}
}

func TestWaitForInput_PositiveTimeout(t *testing.T) {
	// This test verifies waitForInput handles positive timeout without errors
	// We can't easily test actual waiting behavior without mocking, but we can
	// verify the function accepts valid parameters
	timeout := 100 * time.Millisecond

	// Use stderr fd which is unlikely to have pending input
	_, err := waitForInput(2, timeout)

	if err != nil {
		t.Errorf("Expected no error for valid timeout, got %v", err)
	}
}

func TestWaitForInput_ZeroTimeout(t *testing.T) {
	// Test that zero timeout gets converted to minimum timeout
	timeout := time.Duration(0)

	// Use stderr fd which is unlikely to have pending input
	_, err := waitForInput(2, timeout)

	if err != nil {
		t.Errorf("Expected no error for zero timeout, got %v", err)
	}
}

func TestWaitForInput_NegativeTimeout(t *testing.T) {
	// Test that negative timeout gets converted to minimum timeout
	timeout := -1 * time.Millisecond

	// Use stderr fd which is unlikely to have pending input
	_, err := waitForInput(2, timeout)

	if err != nil {
		t.Errorf("Expected no error for negative timeout, got %v", err)
	}
}

func TestReadEscapeSequence_StandaloneESC(t *testing.T) {
	// Test ESC key without following characters (standalone ESC)
	// This tests the timeout-based detection of standalone ESC
	input := []byte{} // No bytes after ESC
	reader := bufio.NewReader(bytes.NewReader(input))

	// Using a very short timeout to simulate no additional input
	seq, err := readEscapeSequence(reader, 2, 1*time.Millisecond)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	// Should return just the ESC byte (27) when timeout occurs
	if len(seq) != 1 || seq[0] != 27 {
		t.Errorf("Expected sequence [27], got %v", seq)
	}
}

