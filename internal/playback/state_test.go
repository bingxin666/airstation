package playback

import (
	"io"
	"log/slog"
	"strconv"
	"strings"
	"testing"

	"github.com/cheatsnake/airstation/internal/netease"
	"github.com/cheatsnake/airstation/internal/pkg/hls"
	"github.com/cheatsnake/airstation/internal/station"
)

type stateStationStore struct {
	props map[string]string
}

func newStateStationStore() *stateStationStore {
	return &stateStationStore{
		props: map[string]string{
			"netease_playlist_url": "1",
			"netease_quality":      string(netease.QualityStandard),
		},
	}
}

func (s *stateStationStore) StationProperties() ([]*station.Property, error) {
	props := make([]*station.Property, 0, len(s.props))
	for key, value := range s.props {
		props = append(props, &station.Property{Key: key, Value: value})
	}
	return props, nil
}

func (s *stateStationStore) UpsertStationProperty(key, value string) (*station.Property, error) {
	s.props[key] = value
	return &station.Property{Key: key, Value: value}, nil
}

func (s *stateStationStore) DeleteStationProperty(key string) error {
	delete(s.props, key)
	return nil
}

type stateNetEaseClient struct {
	playlist *netease.Playlist
}

func (c *stateNetEaseClient) Playlist(_ string, _ string) (*netease.Playlist, error) {
	return c.playlist, nil
}

func (c *stateNetEaseClient) SongURL(songID int64, _ netease.Quality, _ string) (*netease.SongURL, error) {
	return &netease.SongURL{URL: "https://example.test/" + strconv.FormatInt(songID, 10) + ".mp3", BitRate: 128}, nil
}

func (c *stateNetEaseClient) Lyrics(songID int64, _ string) (*netease.Lyrics, error) {
	return &netease.Lyrics{SongID: songID, Kind: "none"}, nil
}

func (c *stateNetEaseClient) Account(_ string) (*netease.Account, error) {
	return &netease.Account{}, nil
}

type stateHLSMaker struct {
	calls int
}

func (m *stateHLSMaker) MakeRemoteHLSPlaylist(_ string, _ string, _ string, _ int, _ int) error {
	m.calls++
	return nil
}

func TestState_LoadNextTrackUsesPreloadedSegmentsAndMetadata(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := newStateStationStore()
	netEaseService := netease.NewService(store, &stateNetEaseClient{
		playlist: &netease.Playlist{
			ID:   "1",
			Name: "Playlist",
			Tracks: []*netease.Song{
				{ID: 1, Name: "One", Artists: []string{"Artist A"}, Duration: 10},
				{ID: 2, Name: "Two", Artists: []string{"Artist B"}, Duration: 10},
			},
		},
	}, log)
	if err := netEaseService.Load(); err != nil {
		t.Fatalf("load netease service: %v", err)
	}

	hlsMaker := &stateHLSMaker{}
	state := NewStateWithHLSMaker(netEaseService, hlsMaker, t.TempDir(), log)
	current := stateTrack(1, "One", "Artist A", "current-seg-", 10)
	next := stateTrack(2, "Two", "Artist B", "next-seg-", 10)
	following := stateTrack(3, "Three", "Artist C", "following-seg-", 10)
	state.CurrentTrack = current.track
	state.CurrentNetEaseID = current.songID
	state.currentSourceURL = current.url
	state.nextPrepared = next
	state.followingPrepared = following
	state.CurrentTrackElapsed = current.track.Duration
	state.IsPlaying = true
	state.playlist = hls.NewPlaylist(current.segments, next.segments)
	state.PlaylistStr = state.playlist.Generate(0)

	callsBefore := hlsMaker.calls
	trackName, songID, err := state.loadNextTrackLocked()
	if err != nil {
		t.Fatalf("load next track: %v", err)
	}

	if trackName != "Two - Artist B" {
		t.Fatalf("track name = %q, want %q", trackName, "Two - Artist B")
	}
	if state.CurrentTrack.Name != "Two" || state.CurrentTrack.Artist != "Artist B" {
		t.Fatalf("current track = %#v", state.CurrentTrack)
	}
	if state.CurrentNetEaseID != 2 {
		t.Fatalf("current netease id = %d, want 2", state.CurrentNetEaseID)
	}
	if songID != 2 {
		t.Fatalf("loaded song id = %d, want 2", songID)
	}
	if state.nextPrepared == nil || state.nextPrepared.songID != 3 {
		t.Fatalf("next prepared = %#v, want song 3", state.nextPrepared)
	}
	state.recordPlayedSong(songID)
	recent := recentNetEaseSongIDs(store)
	if len(recent) != 1 || recent[0] != 2 {
		t.Fatalf("recent netease song ids = %#v, want [2]", recent)
	}
	if hlsMaker.calls != callsBefore {
		t.Fatalf("loadNextTrackLocked called HLS maker: before=%d after=%d", callsBefore, hlsMaker.calls)
	}

	playlist := state.playlist.Generate(0)
	if !strings.Contains(playlist, "next-seg-0"+hls.SegmentExtension) {
		t.Fatalf("playlist did not switch to preloaded next segments:\n%s", playlist)
	}
	if !strings.Contains(playlist, "following-seg-0"+hls.SegmentExtension) {
		t.Fatalf("playlist did not carry following preloaded segments:\n%s", playlist)
	}
}

func TestState_PlayRecordsOnlyCurrentSong(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := newStateStationStore()
	netEaseService := netease.NewService(store, &stateNetEaseClient{
		playlist: &netease.Playlist{
			ID:   "1",
			Name: "Playlist",
			Tracks: []*netease.Song{
				{ID: 1, Name: "One", Artists: []string{"Artist A"}, Duration: 10},
				{ID: 2, Name: "Two", Artists: []string{"Artist B"}, Duration: 10},
			},
		},
	}, log)
	if err := netEaseService.Load(); err != nil {
		t.Fatalf("load netease service: %v", err)
	}

	state := NewStateWithHLSMaker(netEaseService, &stateHLSMaker{}, t.TempDir(), log)
	state.PlayNotify = make(chan string, 1)

	if err := state.Play(); err != nil {
		t.Fatalf("play: %v", err)
	}

	recent := recentNetEaseSongIDs(store)
	if len(recent) != 1 || recent[0] != state.CurrentNetEaseID {
		t.Fatalf("recent netease song ids = %#v, want current song %d only", recent, state.CurrentNetEaseID)
	}
	if state.nextPrepared == nil {
		t.Fatal("next track was not prepared")
	}
	if state.nextPrepared.songID == state.CurrentNetEaseID {
		t.Fatalf("next prepared song duplicates current song %d", state.CurrentNetEaseID)
	}
}

func stateTrack(songID int64, name, artist, segmentPrefix string, duration float64) *preparedTrack {
	song := &netease.Song{ID: songID, Name: name, Artists: []string{artist}, Duration: duration}
	return &preparedTrack{
		track:    song.Track(128),
		songID:   songID,
		url:      "https://example.test/" + strconv.FormatInt(songID, 10) + ".mp3",
		segments: stateSegments(segmentPrefix, duration),
	}
}

func stateSegments(prefix string, duration float64) []*hls.Segment {
	return []*hls.Segment{
		{Duration: duration / 2, Path: prefix + "0" + hls.SegmentExtension, IsFirst: true},
		{Duration: duration / 2, Path: prefix + "1" + hls.SegmentExtension},
	}
}

func recentNetEaseSongIDs(store *stateStationStore) []int64 {
	raw := store.props["netease_recent_song_ids"]
	if raw == "" {
		return nil
	}

	trimmed := strings.Trim(raw, "[]")
	if trimmed == "" {
		return nil
	}

	parts := strings.Split(trimmed, ",")
	ids := make([]int64, 0, len(parts))
	for _, part := range parts {
		id, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
		if err == nil {
			ids = append(ids, id)
		}
	}
	return ids
}
