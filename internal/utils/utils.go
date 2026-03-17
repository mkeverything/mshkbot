package utils

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

var mdV2Replacer = strings.NewReplacer(
	`\`, `\\`,
	`_`, `\_`,
	`*`, `\*`,
	`[`, `\[`,
	`]`, `\]`,
	`(`, `\(`,
	`)`, `\)`,
	`~`, `\~`,
	"`", "\\`",
	`>`, `\>`,
	`#`, `\#`,
	`+`, `\+`,
	`-`, `\-`,
	`=`, `\=`,
	`|`, `\|`,
	`{`, `\{`,
	`}`, `\}`,
	`.`, `\.`,
	`!`, `\!`,
)

func EscapeMDV2(s string) string {
	return mdV2Replacer.Replace(s)
}

type TopRatings struct {
	Blitz     int
	Rapid     int
	Classical int
}

func LoadEnv(requiredVars []string) (map[string]string, error) {
	if err := godotenv.Load(); err != nil {
		return nil, fmt.Errorf("error loading .env file: %w", err)
	}

	envVars := make(map[string]string)

	for _, key := range requiredVars {
		value := os.Getenv(key)
		if value == "" {
			return nil, fmt.Errorf("missing required environment variable: %s", key)
		}
		envVars[key] = value
	}

	return envVars, nil
}

func ConvertToMoscowTime(t time.Time) string {
	moscowLocation := time.FixedZone("Moscow Time", 3*60*60)
	return t.In(moscowLocation).Format("15:04:05")
}

func GetLichessAllTimeHigh(username string) (TopRatings, error) {
	url := fmt.Sprintf("https://lichess.org/api/user/%s/rating-history", username)
	resp, err := http.Get(url)
	if err != nil {
		return TopRatings{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return TopRatings{}, fmt.Errorf("failed to fetch lichess data: %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return TopRatings{}, err
	}

	var ratingHistory []struct {
		Name   string  `json:"name"`
		Points [][]int `json:"points"`
	}

	if err := json.Unmarshal(body, &ratingHistory); err != nil {
		return TopRatings{}, err
	}

	var topRatings TopRatings

	for _, gameType := range ratingHistory {
		var maxRating int
		for i, point := range gameType.Points {
			if i < 5 {
				continue
			}
			if len(point) >= 4 {
				rating := point[3]
				if rating > maxRating {
					maxRating = rating
				}
			}
		}

		switch gameType.Name {
		case "Blitz":
			topRatings.Blitz = maxRating
		case "Rapid":
			topRatings.Rapid = maxRating
		case "Classical":
			topRatings.Classical = maxRating
		}
	}

	return topRatings, nil
}

func GetChessComAllTimeHigh(username string) (TopRatings, error) {
	url := fmt.Sprintf("https://api.chess.com/pub/player/%s/stats", username)
	resp, err := http.Get(url)
	if err != nil {
		return TopRatings{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return TopRatings{}, fmt.Errorf("failed to fetch chess.com data: %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return TopRatings{}, err
	}

	var stats struct {
		ChessRapid struct {
			Best struct {
				Rating int `json:"rating"`
			} `json:"best"`
		} `json:"chess_rapid"`
		ChessBlitz struct {
			Best struct {
				Rating int `json:"rating"`
			} `json:"best"`
		} `json:"chess_blitz"`
		ChessClassical struct {
			Best struct {
				Rating int `json:"rating"`
			} `json:"best"`
		} `json:"chess_daily"`
	}

	if err := json.Unmarshal(body, &stats); err != nil {
		return TopRatings{}, err
	}

	topRatings := TopRatings{
		Blitz:     stats.ChessBlitz.Best.Rating,
		Rapid:     stats.ChessRapid.Best.Rating,
		Classical: stats.ChessClassical.Best.Rating,
	}

	return topRatings, nil
}
