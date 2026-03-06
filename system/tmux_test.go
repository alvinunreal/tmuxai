package system

import (
	"reflect"
	"testing"
)

func TestBuildSplitWindowArgs_DefaultFallback(t *testing.T) {
	got := buildSplitWindowArgs("@1:0", nil)
	want := []string{"split-window", "-d", "-h", "-t", "@1:0", "-P", "-F", "#{pane_id}"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected args\nwant: %#v\n got: %#v", want, got)
	}
}

func TestBuildSplitWindowArgs_UsesConfiguredArgs(t *testing.T) {
	got := buildSplitWindowArgs("@1:2", []string{"-d", "-v", "-p", "30"})
	want := []string{"split-window", "-d", "-v", "-p", "30", "-t", "@1:2", "-P", "-F", "#{pane_id}"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected args\nwant: %#v\n got: %#v", want, got)
	}
}

func TestBuildSplitWindowArgs_EmptyConfiguredArgsFallsBack(t *testing.T) {
	got := buildSplitWindowArgs("%7", []string{})
	want := []string{"split-window", "-d", "-h", "-t", "%7", "-P", "-F", "#{pane_id}"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected args\nwant: %#v\n got: %#v", want, got)
	}
}
