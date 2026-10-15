package game

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"

	"github.com/BurntSushi/toml"
)

// Color : Int constant mapping
const (
	Red    = 0
	Blue   = 1
	Green  = 2
	Pink   = 3
	Orange = 4
	Yellow = 5
	Black  = 6
	White  = 7
	Purple = 8
	Brown  = 9
	Cyan   = 10
	Lime   = 11
	Maroon = 12
	Rose   = 13
	Banana = 14
	Gray   = 15
	Tan    = 16
	Coral  = 17
)

// VanillaColorCount is the number of colors in the unmodded game; custom colors take the IDs after them.
const VanillaColorCount = 18

// MaxEmojiImageBytes is Discord's size limit for an emoji image.
const MaxEmojiImageBytes = 256 * 1024

// Color is one entry of the color table: the ID the capture reports for it, and the lowercase name players use
// to pick it in Discord.
type Color struct {
	ID   int
	Name string
	// Hex is the color as #RRGGBB, for swatches. Optional for custom colors.
	Hex string
	// AliveImage and DeadImage are the paths of the images uploaded as this color's emojis when the emoji guild
	// lacks them. Only custom colors have them; the vanilla emojis are copied from the official bot's.
	AliveImage string
	DeadImage  string
}

var vanillaColors = []Color{
	{ID: Red, Name: "red", Hex: "#C51111"},
	{ID: Blue, Name: "blue", Hex: "#132ED1"},
	{ID: Green, Name: "green", Hex: "#117F2D"},
	{ID: Pink, Name: "pink", Hex: "#ED54BA"},
	{ID: Orange, Name: "orange", Hex: "#EF7D0D"},
	{ID: Yellow, Name: "yellow", Hex: "#F5F557"},
	{ID: Black, Name: "black", Hex: "#3F474E"},
	{ID: White, Name: "white", Hex: "#D6E0F0"},
	{ID: Purple, Name: "purple", Hex: "#6B2FBB"},
	{ID: Brown, Name: "brown", Hex: "#71491E"},
	{ID: Cyan, Name: "cyan", Hex: "#38FEDC"},
	{ID: Lime, Name: "lime", Hex: "#50EF39"},
	{ID: Maroon, Name: "maroon", Hex: "#6B2B3C"},
	{ID: Rose, Name: "rose", Hex: "#ECC0D3"},
	{ID: Banana, Name: "banana", Hex: "#FFFEBE"},
	{ID: Gray, Name: "gray", Hex: "#708496"},
	{ID: Tan, Name: "tan", Hex: "#928776"},
	{ID: Coral, Name: "coral", Hex: "#EC7578"},
}

// colors is the color table, indexed by ID. It is the vanilla table unless SetCustomColors extended it at startup;
// it is not safe to change once the bot is running.
var colors = vanillaColors

// ColorStrings for lowercase, possibly for translation if needed
var ColorStrings = colorStringsOf(colors)

func colorStringsOf(table []Color) map[string]int {
	m := make(map[string]int, len(table))
	for _, c := range table {
		m[c.Name] = c.ID
	}
	return m
}

// Colors returns the color table, ordered by ID.
func Colors() []Color {
	return append([]Color(nil), colors...)
}

// ColorCount is the number of colors in the table; valid IDs run from 0 to ColorCount()-1.
func ColorCount() int {
	return len(colors)
}

// IsValidColor determines if an ID reported by the capture is one of our colors
func IsValidColor(colorint int) bool {
	return colorint >= 0 && colorint < len(colors)
}

// GetColorStringForInt does what it sounds like
func GetColorStringForInt(colorint int) string {
	if !IsValidColor(colorint) {
		return ""
	}
	return colors[colorint].Name
}

// IsColorString determines if a string is actually one of our colors
func IsColorString(test string) bool {
	_, ok := ColorStrings[test]
	return ok
}

// colorNamePattern keeps names usable everywhere the bot shows or parses them: /link lowercases its input and
// strips spaces, and the emoji names "au<name>" and "au<name>dead" must fit Discord's 32 characters.
var colorNamePattern = regexp.MustCompile(`^[a-z0-9_]{1,26}$`)

var hexPattern = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)

type colorFile struct {
	Colors []colorEntry `toml:"colors"`
}

type colorEntry struct {
	ID         *int   `toml:"id"`
	Name       string `toml:"name"`
	Hex        string `toml:"hex"`
	AliveImage string `toml:"alive_image"`
	DeadImage  string `toml:"dead_image"`
}

// LoadCustomColors reads a TOML file of colors added by a mod and installs them after the vanilla ones; see
// SetCustomColors for the rules they must follow. Image paths are relative to the file's directory.
func LoadCustomColors(path string) ([]Color, error) {
	var file colorFile
	meta, err := toml.DecodeFile(path, &file)
	if err != nil {
		return nil, err
	}
	if undecoded := meta.Undecoded(); len(undecoded) > 0 {
		return nil, fmt.Errorf("unknown key %q", undecoded[0].String())
	}
	dir := filepath.Dir(path)
	custom := make([]Color, 0, len(file.Colors))
	for i, e := range file.Colors {
		if e.ID == nil {
			return nil, fmt.Errorf("color #%d (%q) has no id", i+1, e.Name)
		}
		c := Color{ID: *e.ID, Name: e.Name, Hex: e.Hex}
		if e.AliveImage != "" {
			c.AliveImage = resolvePath(dir, e.AliveImage)
		}
		if e.DeadImage != "" {
			c.DeadImage = resolvePath(dir, e.DeadImage)
		}
		custom = append(custom, c)
	}
	if err := SetCustomColors(custom); err != nil {
		return nil, err
	}
	return Colors()[VanillaColorCount:], nil
}

func resolvePath(dir, p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(dir, p)
}

// SetCustomColors replaces the colors after the vanilla ones. Their IDs must continue the vanilla table without
// gaps, as mods number the colors they add, and their names must be unique among all colors. An empty list
// restores the vanilla table.
func SetCustomColors(custom []Color) error {
	sorted := append([]Color(nil), custom...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].ID < sorted[j].ID })

	// a name taken as "x" also takes the emoji name "auxdead", so no other color may be called "xdead"
	emojiNames := make(map[string]string)
	for _, c := range vanillaColors {
		emojiNames[c.Name] = c.Name
		emojiNames[c.Name+"dead"] = c.Name
	}
	for i, c := range sorted {
		if want := VanillaColorCount + i; c.ID != want {
			if c.ID < VanillaColorCount {
				return fmt.Errorf("color %q: id %d is a vanilla color; custom ids start at %d", c.Name, c.ID, VanillaColorCount)
			}
			return fmt.Errorf("color %q: id %d leaves a gap or repeats an id; expected %d", c.Name, c.ID, want)
		}
		if !colorNamePattern.MatchString(c.Name) {
			return fmt.Errorf("color %d: name %q must be 1 to 26 lowercase letters, digits or underscores", c.ID, c.Name)
		}
		for _, n := range []string{c.Name, c.Name + "dead"} {
			if other, taken := emojiNames[n]; taken {
				return fmt.Errorf("color %d: name %q clashes with color %q", c.ID, c.Name, other)
			}
		}
		emojiNames[c.Name] = c.Name
		emojiNames[c.Name+"dead"] = c.Name
		if c.Hex != "" && !hexPattern.MatchString(c.Hex) {
			return fmt.Errorf("color %q: hex %q must look like #RRGGBB", c.Name, c.Hex)
		}
		for _, img := range []string{c.AliveImage, c.DeadImage} {
			if img == "" {
				continue
			}
			if err := checkEmojiImage(img); err != nil {
				return fmt.Errorf("color %q: %w", c.Name, err)
			}
		}
	}

	table := make([]Color, 0, VanillaColorCount+len(sorted))
	table = append(table, vanillaColors...)
	table = append(table, sorted...)
	colors = table
	ColorStrings = colorStringsOf(table)
	return nil
}

var emojiImageTypes = map[string]bool{"image/png": true, "image/gif": true, "image/jpeg": true, "image/webp": true}

func checkEmojiImage(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if len(data) > MaxEmojiImageBytes {
		return fmt.Errorf("%s is %d KiB; Discord accepts emoji images up to %d KiB", path, len(data)/1024, MaxEmojiImageBytes/1024)
	}
	if !emojiImageTypes[http.DetectContentType(data)] {
		return errors.New(path + " is not a PNG, GIF, JPEG or WebP image")
	}
	return nil
}
