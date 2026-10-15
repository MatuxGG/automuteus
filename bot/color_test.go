package bot

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/automuteus/automuteus/v8/pkg/game"
	"github.com/automuteus/automuteus/v8/pkg/settings"
	"github.com/bwmarrin/discordgo"
)

// withCustomColors adds n custom colors, named mod0, mod1, ..., to the color table for the rest of the test.
func withCustomColors(t *testing.T, n int) {
	t.Helper()
	custom := make([]game.Color, n)
	for i := range custom {
		custom[i] = game.Color{ID: game.VanillaColorCount + i, Name: fmt.Sprintf("mod%d", i)}
	}
	if err := game.SetCustomColors(custom); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := game.SetCustomColors(nil); err != nil {
			t.Fatal(err)
		}
	})
}

func TestKnownEmojis_FollowTheColorTable(t *testing.T) {
	known := knownEmojis()
	if len(known[true]) != game.VanillaColorCount || len(known[false]) != game.VanillaColorCount {
		t.Fatalf("vanilla table has %d/%d emojis", len(known[true]), len(known[false]))
	}

	withCustomColors(t, 2)
	known = knownEmojis()
	if len(known[true]) != 20 || len(known[false]) != 20 {
		t.Fatalf("got %d/%d emojis for 20 colors", len(known[true]), len(known[false]))
	}
	if known[true][19].Name != "aumod1" || known[false][19].Name != "aumod1dead" || known[true][19].ID != "" {
		t.Errorf("custom emojis = %+v, %+v", known[true][19], known[false][19])
	}
	if known[true][game.Coral] != GlobalAlivenessEmojis[true][game.Coral] {
		t.Error("vanilla emojis changed")
	}
	if len(GlobalAlivenessEmojis[true]) != game.VanillaColorCount {
		t.Error("GlobalAlivenessEmojis grew")
	}
	if e := emptyStatusEmojis(); len(e[true]) != 20 || len(e[false]) != 20 {
		t.Errorf("status emojis sized %d/%d for 20 colors", len(e[true]), len(e[false]))
	}
}

// fakeEmojiGuild accepts uploads until its slots run out, then answers as Discord does
type fakeEmojiGuild struct {
	slots    int
	attempts int
}

func (g *fakeEmojiGuild) GuildEmojiCreate(_ string, data *discordgo.EmojiParams, _ ...discordgo.RequestOption) (*discordgo.Emoji, error) {
	g.attempts++
	if g.slots == 0 {
		return nil, &discordgo.RESTError{Message: &discordgo.APIErrorMessage{Code: discordgo.ErrCodeMaximumNumberOfEmojisReached}}
	}
	g.slots--
	return &discordgo.Emoji{Name: data.Name, ID: fmt.Sprint(g.attempts)}, nil
}

func TestSyncEmojis_StopsWhenTheGuildIsFull(t *testing.T) {
	image := filepath.Join(t.TempDir(), "emoji.png")
	if err := os.WriteFile(image, []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR"), 0o600); err != nil {
		t.Fatal(err)
	}
	custom := make([]game.Color, 20)
	for i := range custom {
		custom[i] = game.Color{ID: game.VanillaColorCount + i, Name: fmt.Sprintf("mod%d", i), AliveImage: image, DeadImage: image}
	}
	if err := game.SetCustomColors(custom); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := game.SetCustomColors(nil); err != nil {
			t.Fatal(err)
		}
	})

	// the guild already has the vanilla emojis, and room for 25 more
	var server []*discordgo.Emoji
	for _, alive := range []bool{true, false} {
		for _, e := range GlobalAlivenessEmojis[alive] {
			server = append(server, &discordgo.Emoji{Name: e.Name, ID: e.ID})
		}
	}
	guild := &fakeEmojiGuild{slots: 25}
	bot := &Bot{StatusEmojis: emptyStatusEmojis()}
	bot.syncEmojis(guild, "1", server, true)

	// every alive emoji fits, then 5 dead ones; one refused upload stops the rest
	if guild.attempts != 26 {
		t.Errorf("%d uploads attempted, want 26", guild.attempts)
	}
	for i := 0; i < game.ColorCount(); i++ {
		if bot.StatusEmojis[true][i].ID == "" {
			t.Errorf("alive emoji %d missing", i)
		}
		if wantDead := i < game.VanillaColorCount+5; (bot.StatusEmojis[false][i].ID != "") != wantDead {
			t.Errorf("dead emoji %d = %+v, want present %v", i, bot.StatusEmojis[false][i], wantDead)
		}
	}
	if got := bot.StatusEmojis.missing(); got != 15 {
		t.Errorf("%d emojis missing, want 15", got)
	}
}

func TestToEmojiEmbedFields_CustomColors(t *testing.T) {
	withCustomColors(t, 2)
	dgs := voiceTestState(game.LOBBY)
	addLinkedUserWithColor(dgs, "10", "alice", game.Red, true, false, false)
	addLinkedUserWithColor(dgs, "11", "bob", 18, true, false, false)
	addLinkedUserWithColor(dgs, "12", "carol", 19, false, false, false)
	// a color missing from the table hides only that player
	addLinkedUserWithColor(dgs, "13", "dave", 40, true, false, false)

	emojis := knownEmojis()
	emojis[true][18].ID = "1818"
	fields := dgs.ToEmojiEmbedFields(emojis, settings.MakeGuildSettings())

	var values []string
	for _, f := range fields {
		if f.Name != "​" {
			values = append(values, f.Name+"="+f.Value)
		}
	}
	want := []string{
		"alice=" + GlobalAlivenessEmojis[true][game.Red].FormatForInline() + " <@!10>",
		"bob=<:aumod0:1818> <@!11>",
		"carol=" + MissingEmoji + " <@!12>",
	}
	if strings.Join(values, "\n") != strings.Join(want, "\n") {
		t.Errorf("fields:\n%s\nwant:\n%s", strings.Join(values, "\n"), strings.Join(want, "\n"))
	}
}

func selectOptions(t *testing.T, components []discordgo.MessageComponent) []discordgo.SelectMenuOption {
	t.Helper()
	row, ok := components[0].(discordgo.ActionsRow)
	if !ok {
		t.Fatalf("first component is %T", components[0])
	}
	menu, ok := row.Components[0].(discordgo.SelectMenu)
	if !ok {
		t.Fatalf("first control is %T", row.Components[0])
	}
	return menu.Options
}

func optionValues(options []discordgo.SelectMenuOption) []string {
	values := make([]string, len(options))
	for i, o := range options {
		values[i] = o.Value
	}
	return values
}

func TestStatusComponents_ColorSelect(t *testing.T) {
	sett := settings.MakeGuildSettings()
	dgs := voiceTestState(game.LOBBY)
	addLinkedUserWithColor(dgs, "10", "alice", game.Red, true, false, false)

	t.Run("vanilla lists every color", func(t *testing.T) {
		options := selectOptions(t, dgs.statusComponents(emptyStatusEmojis(), sett))
		if len(options) != game.VanillaColorCount+1 || options[game.Coral].Value != "coral" ||
			options[game.Coral].Emoji.ID != GlobalAlivenessEmojis[true][game.Coral].ID {
			t.Errorf("options = %v", optionValues(options))
		}
		if options[len(options)-1].Value != UnlinkEmojiName {
			t.Error("unlink isn't the last option")
		}
	})

	t.Run("a table that fits lists every color", func(t *testing.T) {
		withCustomColors(t, maxSelectColors-game.VanillaColorCount)
		verified := emptyStatusEmojis()
		verified[true][18] = Emoji{Name: "aumod0", ID: "1818"}
		options := selectOptions(t, dgs.statusComponents(verified, sett))
		if len(options) != maxSelectColors+1 {
			t.Fatalf("%d options for %d colors", len(options), maxSelectColors)
		}
		if options[18].Value != "mod0" || options[18].Emoji.ID != "1818" {
			t.Errorf("verified custom color option = %+v", options[18])
		}
		if options[19].Emoji.ID != "" || options[19].Emoji.Name != MissingEmoji {
			t.Errorf("custom color without an emoji = %+v", options[19].Emoji)
		}
	})

	t.Run("a larger table lists the game's colors", func(t *testing.T) {
		withCustomColors(t, 20)
		addLinkedUserWithColor(dgs, "11", "bob", 30, true, false, false)
		addLinkedUserWithColor(dgs, "12", "carol", 19, true, false, false)
		got := strings.Join(optionValues(selectOptions(t, dgs.statusComponents(emptyStatusEmojis(), sett))), ",")
		if want := "red,mod1,mod12," + UnlinkEmojiName; got != want {
			t.Errorf("options = %s, want %s", got, want)
		}
	})
}

func TestProcessPlayer_CustomColor(t *testing.T) {
	t.Run("ignored without a custom table", func(t *testing.T) {
		s := newPlayerScenario(t, game.LOBBY, nil)
		s.send(game.Player{Action: game.JOINED, Name: "alice", Color: 18})
		if _, ok := s.state().GameData.PlayerData["alice"]; ok {
			t.Error("player with an unknown color was recorded")
		}
	})

	t.Run("tracked with one", func(t *testing.T) {
		withCustomColors(t, 1)
		s := newPlayerScenario(t, game.LOBBY, func(dgs *GameState) { addDiscordUser(dgs, "10", "alice") })
		got := s.send(game.Player{Action: game.JOINED, Name: "alice", Color: 18})
		s.expect(playerOutcome{userID: "10", edit: true, handleTracked: true}, got)
		if p := s.state().GameData.PlayerData["alice"]; p.Color != 18 || !p.IsAlive {
			t.Errorf("player data = %+v, want alive mod0 alice", p)
		}
	})
}
