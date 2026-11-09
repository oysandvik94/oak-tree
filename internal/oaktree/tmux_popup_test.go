package oaktree

import (
	"slices"
	"testing"
)

func TestDisplayPopupArgsUsesLargerDashboard(t *testing.T) {
	binary := "/path with spaces/oak-tree"
	want := []string{"display-popup", "-E", "-w", "95%", "-h", "95%", "--", binary}
	if got := DisplayPopupArgs(binary); !slices.Equal(got, want) {
		t.Fatalf("DisplayPopupArgs() = %#v, want %#v", got, want)
	}
}
