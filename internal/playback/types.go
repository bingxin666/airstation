package playback

import "github.com/cheatsnake/airstation/internal/track"

type History struct {
	ID        int    `json:"id"`
	PlayedAt  int64  `json:"playedAt"`
	TrackName string `json:"trackName"`
}

type PublicState struct {
	CurrentTrack        *track.Track `json:"currentTrack"`
	CurrentNetEaseID    int64        `json:"currentNetEaseID"`
	CurrentTrackElapsed float64      `json:"currentTrackElapsed"`
	IsPlaying           bool         `json:"isPlaying"`
	UpdatedAt           int64        `json:"updatedAt"`
}

type Store interface {
	AddPlaybackHistory(playedAt int64, trackName string) error
	RecentPlaybackHistory(limit int) ([]*History, error)
	DeleteOldPlaybackHistory() (int64, error)
}
