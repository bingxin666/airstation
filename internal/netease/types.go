package netease

import (
	"strings"

	"github.com/cheatsnake/airstation/internal/station"
	"github.com/cheatsnake/airstation/internal/track"
)

type Quality string

const (
	QualityStandard Quality = "standard"
	QualityHigher   Quality = "higher"
	QualityExHigh   Quality = "exhigh"
	QualityLossless Quality = "lossless"
	QualityHiRes    Quality = "hires"
)

type Config struct {
	PlaylistURL string  `json:"playlistURL"`
	Quality     Quality `json:"quality"`
	Cookie      string  `json:"cookie,omitempty"`
	ClearCookie bool    `json:"clearCookie,omitempty"`
}

type PublicConfig struct {
	PlaylistURL  string  `json:"playlistURL"`
	PlaylistID   string  `json:"playlistID"`
	Quality      Quality `json:"quality"`
	HasCookie    bool    `json:"hasCookie"`
	AccountName  string  `json:"accountName"`
	TrackCount   int     `json:"trackCount"`
	LastError    string  `json:"lastError"`
	LastSyncedAt int64   `json:"lastSyncedAt"`
}

type Store interface {
	StationProperties() ([]*station.Property, error)
	UpsertStationProperty(key, value string) (*station.Property, error)
	DeleteStationProperty(key string) error
}

type Client interface {
	Playlist(id string, cookie string) (*Playlist, error)
	SongURL(songID int64, quality Quality, cookie string) (*SongURL, error)
	Lyrics(songID int64, cookie string) (*Lyrics, error)
	Account(cookie string) (*Account, error)
}

type Playlist struct {
	ID     string
	Name   string
	Tracks []*Song
}

type Song struct {
	ID       int64
	Name     string
	Artists  []string
	Album    string
	Duration float64
}

type SongURL struct {
	URL     string
	BitRate int
}

type Lyrics struct {
	SongID      int64  `json:"songID"`
	Kind        string `json:"kind"`
	YRC         string `json:"yrc"`
	LRC         string `json:"lrc"`
	Translation string `json:"translation"`
	Text        string `json:"text"`
}

type Account struct {
	Nickname string
}

type PlayableTrack struct {
	Track  *track.Track
	SongID int64
	URL    string
}

func (s *Song) Track(bitRate int) *track.Track {
	artist := ""
	if len(s.Artists) > 0 {
		artist = strings.Join(s.Artists, ", ")
	}

	return &track.Track{
		ID:       songTrackID(s.ID),
		Name:     s.Name,
		Artist:   artist,
		Path:     "",
		Duration: s.Duration,
		BitRate:  bitRate,
	}
}
