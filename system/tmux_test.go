package system

import (
	"reflect"
	"strings"
	"testing"
)

func TestBuildSplitWindowArgs_UsesConfiguredArgs(t *testing.T) {
	got, err := buildSplitWindowArgs("@1:2", []string{"-d", "-v", "-p", "30"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []string{"split-window", "-d", "-v", "-p", "30", "-t", "@1:2", "-P", "-F", "#{pane_id}"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected args\nwant: %#v\n got: %#v", want, got)
	}
}

func TestBuildSplitWindowArgs_EmptyConfiguredArgsKeepsAppendedDefaultsOnly(t *testing.T) {
	got, err := buildSplitWindowArgs("%7", []string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []string{"split-window", "-t", "%7", "-P", "-F", "#{pane_id}"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected args\nwant: %#v\n got: %#v", want, got)
	}
}

func TestBuildSplitWindowArgs_RejectsReservedFlags(t *testing.T) {
	tests := [][]string{
		{"-F"},
		{"-d", "-t", "%9"},
		{"-P"},
	}

	for _, splitArgs := range tests {
		_, err := buildSplitWindowArgs("@1:0", splitArgs)
		if err == nil {
			t.Fatalf("expected error for args %v", splitArgs)
		}
		if !strings.Contains(err.Error(), "reserved") {
			t.Fatalf("expected reserved-flag error for args %v, got: %v", splitArgs, err)
		}
	}
}

func TestBuildSplitWindowArgs_AllowsReservedFlagStringsAsValues(t *testing.T) {
	got, err := buildSplitWindowArgs("@1:2", []string{"-c", "-t"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []string{"split-window", "-c", "-t", "-t", "@1:2", "-P", "-F", "#{pane_id}"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected args\nwant: %#v\n got: %#v", want, got)
	}
}

func TestBuildSplitWindowArgs_RejectsReservedFlagAfterValuePair(t *testing.T) {
	_, err := buildSplitWindowArgs("@1:2", []string{"-c", "/tmp", "-t"})
	if err == nil {
		t.Fatal("expected error for reserved flag used after value pair")
	}
	if !strings.Contains(err.Error(), "reserved") {
		t.Fatalf("expected reserved-flag error, got: %v", err)
	}
}

func TestBuildSplitWindowArgs_RejectsReservedFlagFAndSkipsValue(t *testing.T) {
	_, err := buildSplitWindowArgs("@1:2", []string{"-F", "#{pane_id}", "-d"})
	if err == nil {
		t.Fatal("expected error for reserved flag -F")
	}
	if !strings.Contains(err.Error(), "reserved") {
		t.Fatalf("expected reserved-flag error, got: %v", err)
	}
}

func TestBuildSplitWindowArgs_AllowsReservedFlagAsValueForFlagTakingArg(t *testing.T) {
	got, err := buildSplitWindowArgs("@1:2", []string{"-c", "-t"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []string{"split-window", "-c", "-t", "-t", "@1:2", "-P", "-F", "#{pane_id}"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected args\nwant: %#v\n got: %#v", want, got)
	}
}
