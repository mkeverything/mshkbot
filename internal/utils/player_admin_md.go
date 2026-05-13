package utils

import (
	"fmt"

	"github.com/sukalov/mshkbot/internal/types"
)

func playerAdminBodyMarkdownV2(player types.Player) (body string, emptyName bool) {
	if player.SavedName == "" {
		return "", true
	}

	escapedName := EscapeMDV2(player.SavedName)

	var core string
	if player.CheckinMessageID != 0 && player.CheckinChatID != 0 {
		chatIDForLink := player.CheckinChatID
		if chatIDForLink < 0 {
			chatIDForLink = -chatIDForLink - 1000000000000
		}
		if chatIDForLink > 0 {
			messageLink := fmt.Sprintf("https://t.me/c/%d/%d", chatIDForLink, player.CheckinMessageID)
			core = fmt.Sprintf("[%s](%s)", escapedName, messageLink)
		} else {
			core = escapedName
		}
	} else {
		core = escapedName
	}

	if player.Username != "" {
		core += fmt.Sprintf(" \\(@%s\\)", EscapeMDV2(player.Username))
	}

	if player.PeakRating != nil {
		rating := EscapeMDV2(fmt.Sprintf("%d", player.PeakRating.BlitzPeak))
		switch player.PeakRating.Site {
		case types.SiteLichess:
			if player.PeakRating.SiteUsername != "" {
				siteURL := fmt.Sprintf("https://lichess.org/@/%s", player.PeakRating.SiteUsername)
				core += fmt.Sprintf(" \\([lichess](%s) %s\\)", siteURL, rating)
			} else {
				core += fmt.Sprintf(" \\(lichess %s\\)", rating)
			}
		case types.SiteChesscom:
			if player.PeakRating.SiteUsername != "" {
				siteURL := fmt.Sprintf("https://www.chess.com/member/%s", player.PeakRating.SiteUsername)
				core += fmt.Sprintf(" \\([chesscom](%s) %s\\)", siteURL, rating)
			} else {
				core += fmt.Sprintf(" \\(chesscom %s\\)", rating)
			}
		default:
			core += fmt.Sprintf(" \\(%s %s\\)", EscapeMDV2(string(player.PeakRating.Site)), rating)
		}
	}

	return core, false
}

func FormatPlayerLineAdminMarkdownV2(num int, player types.Player) string {
	body, emptyName := playerAdminBodyMarkdownV2(player)
	if emptyName {
		return fmt.Sprintf("%d\\. \\(unknown\\)", num)
	}
	return fmt.Sprintf("%d\\. %s", num, body)
}

func FormatPlayerMentionAdminMarkdownV2(player types.Player) string {
	body, emptyName := playerAdminBodyMarkdownV2(player)
	if emptyName {
		return "\\(unknown\\)"
	}
	return body
}

func FormatPlayerLineAdminWithMetadataMarkdownV2(num int, player types.Player) string {
	line := FormatPlayerLineAdminMarkdownV2(num, player)
	if player.AllowToGreen {
		line += " 🍀"
	}
	return line
}

func FormatPlayerMentionAdminWithMetadataMarkdownV2(player types.Player) string {
	m := FormatPlayerMentionAdminMarkdownV2(player)
	if player.AllowToGreen {
		m += " 🍀"
	}
	return m
}
