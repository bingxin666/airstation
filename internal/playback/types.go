package playback

import "github.com/cheatsnake/airstation/internal/track"

type PublicState struct {
	CurrentTrack        *track.Track `json:"currentTrack"`
	CurrentNetEaseID    int64        `json:"currentNetEaseID"`
	CurrentTrackElapsed float64      `json:"currentTrackElapsed"`
	IsPlaying           bool         `json:"isPlaying"`
	UpdatedAt           int64        `json:"updatedAt"`
}
