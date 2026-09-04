package highlight

import "testing"

func TestUnknownSyntaxStaysPlain(t *testing.T) {
	src := []byte("A `name` can change while it is not in use.\n")
	if got := File("notes.adoc", src); got != nil {
		t.Fatalf("unknown syntax produced %d highlighted lines, want none", len(got))
	}
}

func TestKnownSyntaxIsHighlighted(t *testing.T) {
	lines := File("main.go", []byte("package main\n"))
	if lines == nil {
		t.Fatal("known Go syntax was not highlighted")
	}

	for _, span := range lines.Line(1) {
		if span.Class == Keyword {
			return
		}
	}
	t.Fatal("the Go keyword was not highlighted")
}
