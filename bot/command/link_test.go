package command

import (
	"fmt"
	"testing"

	"github.com/automuteus/automuteus/v8/pkg/game"
)

func setCustomColors(t *testing.T, n int) {
	t.Helper()
	custom := make([]game.Color, n)
	for i := range custom {
		custom[i] = game.Color{ID: game.VanillaColorCount + i, Name: fmt.Sprintf("mod%d", i)}
	}
	if err := game.SetCustomColors(custom); err != nil {
		t.Fatal(err)
	}
	UpdateColorChoices()
}

func TestLinkColorChoices(t *testing.T) {
	t.Cleanup(func() {
		if err := game.SetCustomColors(nil); err != nil {
			t.Fatal(err)
		}
		UpdateColorChoices()
	})
	choices := func() []string {
		var names []string
		for _, c := range Link.Options[1].Choices {
			names = append(names, c.Name)
		}
		return names
	}

	got := choices()
	if len(got) != game.VanillaColorCount || got[0] != "red" || got[game.Coral] != "coral" {
		t.Errorf("vanilla choices = %v", got)
	}

	setCustomColors(t, maxCommandChoices-game.VanillaColorCount)
	if got := choices(); len(got) != maxCommandChoices || got[maxCommandChoices-1] != "mod6" {
		t.Errorf("choices for a table that fits = %v", got)
	}

	// Discord refuses more choices than that; the color is typed instead
	setCustomColors(t, maxCommandChoices-game.VanillaColorCount+1)
	if got := choices(); got != nil {
		t.Errorf("choices for a table that doesn't fit = %v", got)
	}
}
