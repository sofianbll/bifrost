package main

import "testing"

func TestReturnErrorPrefixPayload(t *testing.T) {
	if got, want := returnErrorMessage("prefix"), "Error: synthetic fixture error value"; got != want {
		t.Fatalf("prefix error = %q, want %q", got, want)
	}
}
