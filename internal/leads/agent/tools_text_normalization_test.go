package agent

import "testing"

func TestNormalizeSuggestedContactMessage(t *testing.T) {
	input := "Beste klant,\\n\\nwe hebben nog nodie info nodig.  Kun je die sturen?\\tDank!"
	got := normalizeSuggestedContactMessage(input)
	want := "Beste klant,\n\nwe hebben nog nodig info nodig. Kun je die sturen? Dank!"
	if got != want {
		t.Fatalf("unexpected normalized message:\nwant: %q\ngot:  %q", want, got)
	}
}

