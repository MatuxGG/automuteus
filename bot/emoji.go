package bot

import (
	"encoding/base64"
	"errors"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/automuteus/automuteus/v8/pkg/game"

	"github.com/bwmarrin/discordgo"
)

const (
	UnlinkEmojiName = "auunlink"
	X               = "❌"
	ThumbsUp        = "👍"
	Hourglass       = "⌛"
	// MissingEmoji stands in for a color's emoji when the emoji guild has none and the bot couldn't upload it
	MissingEmoji = "❔"
)

// Emoji struct for discord
type Emoji struct {
	Name string
	ID   string
	// imagePath is the file uploaded for a custom color's emoji; vanilla emojis are copied from the official bot's
	imagePath string
}

// FormatForInline does what it sounds like
func (e *Emoji) FormatForInline() string {
	if e.ID == "" {
		return MissingEmoji
	}
	return "<:" + e.Name + ":" + e.ID + ">"
}

// GetDiscordCDNUrl does what it sounds like
func (e *Emoji) GetDiscordCDNUrl() string {
	return "https://cdn.discordapp.com/emojis/" + e.ID + ".png"
}

// DownloadAndBase64Encode does what it sounds like
func (e *Emoji) DownloadAndBase64Encode() string {
	url := e.GetDiscordCDNUrl()
	response, err := http.Get(url)
	if err != nil {
		log.Println(err)
	}
	defer response.Body.Close()
	bytes, err := io.ReadAll(response.Body)
	if err != nil {
		log.Println(err)
	}
	encodedStr := base64.StdEncoding.EncodeToString(bytes)
	return "data:image/png;base64," + encodedStr
}

// imageData is the emoji's image as a data URI, for uploading it to the emoji guild
func (e *Emoji) imageData() (string, error) {
	if e.imagePath == "" {
		return e.DownloadAndBase64Encode(), nil
	}
	bytes, err := os.ReadFile(e.imagePath)
	if err != nil {
		return "", err
	}
	return "data:" + http.DetectContentType(bytes) + ";base64," + base64.StdEncoding.EncodeToString(bytes), nil
}

func (a AlivenessEmojis) isEmpty() bool {
	if v, ok := a[true]; ok {
		for _, vv := range v {
			if vv.Name == "" || vv.ID == "" {
				return true
			}
		}
	} else {
		return true
	}
	if v, ok := a[false]; ok {
		for _, vv := range v {
			if vv.Name == "" || vv.ID == "" {
				return true
			}
		}
	} else {
		return true
	}
	return false
}

func emptyStatusEmojis() AlivenessEmojis {
	topMap := make(AlivenessEmojis)
	topMap[true] = make([]Emoji, game.ColorCount()) // one per color for alive/dead
	topMap[false] = make([]Emoji, game.ColorCount())
	return topMap
}

// knownEmojis are the emojis of every color in the table: the vanilla ones as the official bot has them, then
// the custom colors', which have no ID until the bot finds or uploads them in the emoji guild.
func knownEmojis() AlivenessEmojis {
	known := AlivenessEmojis{
		true:  append([]Emoji(nil), GlobalAlivenessEmojis[true]...),
		false: append([]Emoji(nil), GlobalAlivenessEmojis[false]...),
	}
	for _, c := range game.Colors()[game.VanillaColorCount:] {
		known[true] = append(known[true], Emoji{Name: "au" + c.Name, imagePath: c.AliveImage})
		known[false] = append(known[false], Emoji{Name: "au" + c.Name + "dead", imagePath: c.DeadImage})
	}
	return known
}

// get is the emoji for a color, or an empty one (shown as MissingEmoji) when there is none
func (a AlivenessEmojis) get(alive bool, color int) Emoji {
	if color < 0 || color >= len(a[alive]) {
		return Emoji{}
	}
	return a[alive][color]
}

// emojiCreator uploads emojis to a guild, as *discordgo.Session does
type emojiCreator interface {
	GuildEmojiCreate(guildID string, data *discordgo.EmojiParams, options ...discordgo.RequestOption) (*discordgo.Emoji, error)
}

// missing counts the colors' emojis that have no ID, so show as MissingEmoji
func (a AlivenessEmojis) missing() (n int) {
	for _, emojis := range a {
		for _, e := range emojis {
			if e.ID == "" {
				n++
			}
		}
	}
	return n
}

// syncEmojis finds the color emojis in the emoji guild, uploading those it lacks when add is set. The alive emojis
// go first, as the color select shows them too, so a guild without room for every color loses dead emojis first.
func (bot *Bot) syncEmojis(s emojiCreator, guildID string, serverEmojis []*discordgo.Emoji, add bool) {
	full := bot.verifyEmojis(s, guildID, true, serverEmojis, add)
	if bot.verifyEmojis(s, guildID, false, serverEmojis, add && !full) {
		full = true
	}
	if full {
		log.Printf("Emoji guild %s has no emoji slots left: %d color emojis show as %s. Free or add slots (boosting raises the limit), and the bot uploads them the next time it starts\n",
			guildID, bot.StatusEmojis.missing(), MissingEmoji)
	}
}

// verifyEmojis reports whether the guild ran out of emoji slots, after which it stops uploading
func (bot *Bot) verifyEmojis(s emojiCreator, guildID string, alive bool, serverEmojis []*discordgo.Emoji, add bool) (full bool) {
	for i, emoji := range knownEmojis()[alive] {
		alreadyExists := false
		for _, v := range serverEmojis {
			if v.Name == emoji.Name {
				emoji.ID = v.ID
				bot.StatusEmojis[alive][i] = emoji
				alreadyExists = true
				break
			}
		}
		if add && !full && !alreadyExists {
			if emoji.ID == "" && emoji.imagePath == "" {
				log.Printf("Emoji %s is not in the emoji guild, and its color has no image to upload\n", emoji.Name)
				continue
			}
			b64, err := emoji.imageData()
			if err != nil {
				log.Println(err)
				continue
			}
			p := discordgo.EmojiParams{
				Name:  emoji.Name,
				Image: b64,
				Roles: nil,
			}
			em, err := s.GuildEmojiCreate(guildID, &p)
			var restErr *discordgo.RESTError
			if errors.As(err, &restErr) && restErr.Message != nil && restErr.Message.Code == discordgo.ErrCodeMaximumNumberOfEmojisReached {
				full = true
			} else if err != nil {
				log.Println(err)
			} else {
				log.Printf("Added emoji %s successfully!\n", emoji.Name)
				emoji.ID = em.ID
				bot.StatusEmojis[alive][i] = emoji
			}
		}
	}
	return full
}

// EmojisToSelectMenuOptions lists the given colors, each with its emoji from emojis (indexed by color)
func EmojisToSelectMenuOptions(emojis []Emoji, colors []int, unlinkEmoji string) (arr []discordgo.SelectMenuOption) {
	for _, c := range colors {
		var emoji Emoji
		if c >= 0 && c < len(emojis) {
			emoji = emojis[c]
		}
		arr = append(arr, emoji.toSelectMenuOption(game.GetColorStringForInt(c)))
	}
	arr = append(arr, discordgo.SelectMenuOption{
		Label:   "unlink",
		Value:   UnlinkEmojiName,
		Emoji:   discordgo.ComponentEmoji{Name: unlinkEmoji},
		Default: false,
	})
	return arr
}

func (e Emoji) toSelectMenuOption(displayName string) discordgo.SelectMenuOption {
	emoji := discordgo.ComponentEmoji{ID: e.ID}
	if e.ID == "" {
		emoji = discordgo.ComponentEmoji{Name: MissingEmoji}
	}
	return discordgo.SelectMenuOption{
		Label:   displayName,
		Value:   displayName, // use the Name for listen events later
		Emoji:   emoji,
		Default: false,
	}
}

// AlivenessEmojis map
type AlivenessEmojis map[bool][]Emoji

// GlobalAlivenessEmojis keys are IsAlive, Color
var GlobalAlivenessEmojis = AlivenessEmojis{
	true: []Emoji{
		game.Red: {
			Name: "aured",
			ID:   "866558066921177108",
		},
		game.Blue: {
			Name: "aublue",
			ID:   "866558066484183060",
		},
		game.Green: {
			Name: "augreen",
			ID:   "866558066568986664",
		},
		game.Pink: {
			Name: "aupink",
			ID:   "866558067004538891",
		},
		game.Orange: {
			Name: "auorange",
			ID:   "866558066902958090",
		},
		game.Yellow: {
			Name: "auyellow",
			ID:   "866558067243221002",
		},
		game.Black: {
			Name: "aublack",
			ID:   "866558066442895370",
		},
		game.White: {
			Name: "auwhite",
			ID:   "866558067026165770",
		},
		game.Purple: {
			Name: "aupurple",
			ID:   "866558066966396928",
		},
		game.Brown: {
			Name: "aubrown",
			ID:   "866558066564136970",
		},
		game.Cyan: {
			Name: "aucyan",
			ID:   "866558066525601853",
		},
		game.Lime: {
			Name: "aulime",
			ID:   "866558066963382282",
		},
		game.Maroon: {
			Name: "aumaroon",
			ID:   "866558066917113886",
		},
		game.Rose: {
			Name: "aurose",
			ID:   "866558066921439242",
		},
		game.Banana: {
			Name: "aubanana",
			ID:   "866558065917558797",
		},
		game.Gray: {
			Name: "augray",
			ID:   "866558066174459905",
		},
		game.Tan: {
			Name: "autan",
			ID:   "866558066820382721",
		},
		game.Coral: {
			Name: "aucoral",
			ID:   "866558066552209448",
		},
	},
	false: []Emoji{
		game.Red: {
			Name: "aureddead",
			ID:   "866558067255279636",
		},
		game.Blue: {
			Name: "aubluedead",
			ID:   "866558066660999218",
		},
		game.Green: {
			Name: "augreendead",
			ID:   "866558067088949258",
		},
		game.Pink: {
			Name: "aupinkdead",
			ID:   "866558066945556512",
		},
		game.Orange: {
			Name: "auorangedead",
			ID:   "866558067508510730",
		},
		game.Yellow: {
			Name: "auyellowdead",
			ID:   "866558067206520862",
		},
		game.Black: {
			Name: "aublackdead",
			ID:   "866558066668339250",
		},
		game.White: {
			Name: "auwhitedead",
			ID:   "866558067231293450",
		},
		game.Purple: {
			Name: "aupurpledead",
			ID:   "866558067223298048",
		},
		game.Brown: {
			Name: "aubrowndead",
			ID:   "866558066945163304",
		},
		game.Cyan: {
			Name: "aucyandead",
			ID:   "866558067051200512",
		},
		game.Lime: {
			Name: "aulimedead",
			ID:   "866558067344408596",
		},
		game.Maroon: {
			Name: "aumaroondead",
			ID:   "866558067238895626",
		},
		game.Rose: {
			Name: "aurosedead",
			ID:   "866558067083444225",
		},
		game.Banana: {
			Name: "aubananadead",
			ID:   "866558066342625350",
		},
		game.Gray: {
			Name: "augraydead",
			ID:   "866558067049758740",
		},
		game.Tan: {
			Name: "autandead",
			ID:   "866558067230638120",
		},
		game.Coral: {
			Name: "aucoraldead",
			ID:   "866558067024723978",
		},
	},
}

/*
Helpful for copy/paste into Discord to get new emoji IDs when they are re-uploaded...
\:aured:
\:aublue:
\:augreen:
\:aupink:
\:auorange:
\:auyellow:
\:aublack:
\:auwhite:
\:aupurple:
\:aubrown:
\:aucyan:
\:aulime:
\:aumaroon:
\:aurose:
\:aubanana:
\:augray:
\:autan:
\:aucoral:

\:aureddead:
\:aubluedead:
\:augreendead:
\:aupinkdead:
\:auorangedead:
\:auyellowdead:
\:aublackdead:
\:auwhitedead:
\:aupurpledead:
\:aubrowndead:
\:aucyandead:
\:aulimedead:
\:aumaroondead:
\:aurosedead:
\:aubananadead:
\:augraydead:
\:autandead:
\:aucoraldead:
*/
