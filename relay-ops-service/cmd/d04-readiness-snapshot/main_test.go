package main

import "testing"

func TestRunRequiresAllInputPaths(t *testing.T) {
	t.Parallel()

	if err := run(nil); err == nil {
		t.Fatal("missing paths were accepted")
	}
}
