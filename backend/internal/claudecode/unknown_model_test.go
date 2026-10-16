package claudecode

import "testing"

func TestUnknownModel(t *testing.T) {
	ok := "There's an issue with the selected model (claude-sonnet-5-5). It may not exist or you may not have access to it."
	if !UnknownModel(ok) {
		t.Fatal("expected true")
	}
	if UnknownModel("changed hello.txt") || UnknownModel("selected model only") || UnknownModel("may not exist only") {
		t.Fatal("expected false")
	}
}
