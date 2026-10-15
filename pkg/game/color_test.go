package game

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// pngHeader is enough of a PNG for content sniffing.
var pngHeader = []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR")

func resetColors(t *testing.T) {
	t.Cleanup(func() {
		if err := SetCustomColors(nil); err != nil {
			t.Fatal(err)
		}
	})
}

func TestColors_VanillaTable(t *testing.T) {
	if ColorCount() != VanillaColorCount || len(ColorStrings) != VanillaColorCount {
		t.Fatalf("ColorCount() = %d, len(ColorStrings) = %d; want %d", ColorCount(), len(ColorStrings), VanillaColorCount)
	}
	for i, c := range Colors() {
		if c.ID != i || ColorStrings[c.Name] != i || GetColorStringForInt(i) != c.Name || !hexPattern.MatchString(c.Hex) {
			t.Errorf("color %d: %+v doesn't round-trip", i, c)
		}
	}
	if GetColorStringForInt(Coral) != "coral" || GetColorStringForInt(Red) != "red" {
		t.Error("vanilla constants no longer match their names")
	}
	for _, id := range []int{-1, VanillaColorCount} {
		if IsValidColor(id) || GetColorStringForInt(id) != "" {
			t.Errorf("color %d reported as valid", id)
		}
	}
}

func TestSetCustomColors_ExtendsAndResets(t *testing.T) {
	resetColors(t)
	// listed out of order; the table orders them by ID
	err := SetCustomColors([]Color{{ID: 19, Name: "sky_blue"}, {ID: 18, Name: "watermelon", Hex: "#A8323C"}})
	if err != nil {
		t.Fatal(err)
	}
	if ColorCount() != 20 || !IsValidColor(19) || IsValidColor(20) {
		t.Fatalf("ColorCount() = %d after adding 2 colors", ColorCount())
	}
	if GetColorStringForInt(18) != "watermelon" || GetColorStringForInt(19) != "sky_blue" {
		t.Errorf("custom names = %q, %q", GetColorStringForInt(18), GetColorStringForInt(19))
	}
	if !IsColorString("watermelon") || ColorStrings["sky_blue"] != 19 || GetColorStringForInt(Red) != "red" {
		t.Error("ColorStrings doesn't cover both tables")
	}

	if err := SetCustomColors(nil); err != nil {
		t.Fatal(err)
	}
	if ColorCount() != VanillaColorCount || IsColorString("watermelon") {
		t.Error("an empty list didn't restore the vanilla table")
	}
}

func TestSetCustomColors_Rejects(t *testing.T) {
	resetColors(t)
	dir := t.TempDir()
	big := filepath.Join(dir, "big.png")
	if err := os.WriteFile(big, append(pngHeader, make([]byte, MaxEmojiImageBytes)...), 0o600); err != nil {
		t.Fatal(err)
	}
	text := filepath.Join(dir, "notes.png")
	if err := os.WriteFile(text, []byte("not an image"), 0o600); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name   string
		colors []Color
		want   string
	}{
		{"vanilla id", []Color{{ID: 5, Name: "gold"}}, "is a vanilla color"},
		{"gap", []Color{{ID: 18, Name: "gold"}, {ID: 20, Name: "mint"}}, "expected 19"},
		{"repeated id", []Color{{ID: 18, Name: "gold"}, {ID: 18, Name: "mint"}}, "expected 19"},
		{"not starting at 18", []Color{{ID: 19, Name: "gold"}}, "expected 18"},
		{"uppercase name", []Color{{ID: 18, Name: "Gold"}}, "lowercase"},
		{"space in name", []Color{{ID: 18, Name: "sky blue"}}, "lowercase"},
		{"empty name", []Color{{ID: 18}}, "lowercase"},
		{"name too long", []Color{{ID: 18, Name: strings.Repeat("a", 27)}}, "26"},
		{"vanilla name", []Color{{ID: 18, Name: "red"}}, `clashes with color "red"`},
		{"repeated name", []Color{{ID: 18, Name: "gold"}, {ID: 19, Name: "gold"}}, `clashes with color "gold"`},
		{"dead emoji clash", []Color{{ID: 18, Name: "reddead"}}, `clashes with color "red"`},
		{"custom dead emoji clash", []Color{{ID: 18, Name: "gold"}, {ID: 19, Name: "golddead"}}, `clashes with color "gold"`},
		{"bad hex", []Color{{ID: 18, Name: "gold", Hex: "gold"}}, "#RRGGBB"},
		{"missing image", []Color{{ID: 18, Name: "gold", AliveImage: filepath.Join(dir, "nope.png")}}, "nope.png"},
		{"image too big", []Color{{ID: 18, Name: "gold", DeadImage: big}}, "KiB"},
		{"not an image", []Color{{ID: 18, Name: "gold", AliveImage: text}}, "not a PNG"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := SetCustomColors(tc.colors)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, want one containing %q", err, tc.want)
			}
			if ColorCount() != VanillaColorCount {
				t.Errorf("a rejected table was installed: %d colors", ColorCount())
			}
		})
	}
}

func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestLoadCustomColors(t *testing.T) {
	resetColors(t)
	dir := t.TempDir()
	alive := filepath.Join(dir, "emojis", "auwatermelon.png")
	writeFile(t, alive, pngHeader)
	dead := filepath.Join(t.TempDir(), "dead.png")
	writeFile(t, dead, pngHeader)
	path := filepath.Join(dir, "colors.toml")
	writeFile(t, path, []byte(`
[[colors]]
id = 18
name = "watermelon"
hex = "#A8323C"
alive_image = "emojis/auwatermelon.png"
dead_image = '`+dead+`'

[[colors]]
id = 19
name = "chocolate"
`))

	custom, err := LoadCustomColors(path)
	if err != nil {
		t.Fatal(err)
	}
	want := []Color{
		{ID: 18, Name: "watermelon", Hex: "#A8323C", AliveImage: alive, DeadImage: dead},
		{ID: 19, Name: "chocolate"},
	}
	if len(custom) != len(want) || custom[0] != want[0] || custom[1] != want[1] {
		t.Errorf("custom colors = %+v, want %+v", custom, want)
	}
	if ColorCount() != 20 || GetColorStringForInt(19) != "chocolate" {
		t.Errorf("table not installed: %d colors", ColorCount())
	}
}

func TestLoadCustomColors_Rejects(t *testing.T) {
	resetColors(t)
	cases := []struct {
		name, file, want string
	}{
		{"unknown key", "[[colors]]\nid = 18\nname = \"gold\"\ncolour = \"#FFD700\"\n", `"colors.colour"`},
		{"no id", "[[colors]]\nname = \"gold\"\n", `("gold") has no id`},
		{"invalid toml", "[[colors]\n", "line 2"},
		{"invalid color", "[[colors]]\nid = 18\nname = \"Gold\"\n", "lowercase"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "colors.toml")
			writeFile(t, path, []byte(tc.file))
			_, err := LoadCustomColors(path)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, want one containing %q", err, tc.want)
			}
		})
	}
	if _, err := LoadCustomColors(filepath.Join(t.TempDir(), "missing.toml")); err == nil {
		t.Error("a missing file loaded")
	}
	if ColorCount() != VanillaColorCount {
		t.Errorf("a rejected file was installed: %d colors", ColorCount())
	}
}
