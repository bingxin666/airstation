package netease

import (
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/cheatsnake/airstation/internal/station"
)

type memoryStore struct {
	props map[string]string
}

func newMemoryStore() *memoryStore {
	return &memoryStore{props: map[string]string{}}
}

func (s *memoryStore) StationProperties() ([]*station.Property, error) {
	props := make([]*station.Property, 0, len(s.props))
	for key, value := range s.props {
		props = append(props, &station.Property{Key: key, Value: value})
	}
	return props, nil
}

func (s *memoryStore) UpsertStationProperty(key, value string) (*station.Property, error) {
	s.props[key] = value
	return &station.Property{Key: key, Value: value}, nil
}

func (s *memoryStore) DeleteStationProperty(key string) error {
	delete(s.props, key)
	return nil
}

type fakeClient struct {
	playlist    *Playlist
	account     *Account
	songURL     *SongURL
	lyrics      *Lyrics
	playlistErr error
	songURLErr  error
}

func (c *fakeClient) Playlist(id string, cookie string) (*Playlist, error) {
	if c.playlistErr != nil {
		return nil, c.playlistErr
	}
	return c.playlist, nil
}

func (c *fakeClient) SongURL(songID int64, quality Quality, cookie string) (*SongURL, error) {
	if c.songURLErr != nil {
		return nil, c.songURLErr
	}
	return c.songURL, nil
}

func (c *fakeClient) Lyrics(songID int64, cookie string) (*Lyrics, error) {
	if c.lyrics == nil {
		return &Lyrics{SongID: songID, Kind: "none"}, nil
	}
	return c.lyrics, nil
}

func (c *fakeClient) Account(cookie string) (*Account, error) {
	if c.account == nil {
		return &Account{}, nil
	}
	return c.account, nil
}

func TestService_LyricsCachesClientResult(t *testing.T) {
	store := newMemoryStore()
	client := &fakeClient{lyrics: &Lyrics{SongID: 1, Kind: "word", YRC: "[0,1000](0,1000,0)test"}}
	service := NewService(store, client, slog.New(slog.NewTextHandler(io.Discard, nil)))

	lyrics, err := service.Lyrics(1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if lyrics.Kind != "word" {
		t.Fatalf("kind = %q, want word", lyrics.Kind)
	}
}

func TestPlainLyricText(t *testing.T) {
	raw := "[00:01.00]first line\n[00:02.30]second line\n[ti:metadata]\n{\"t\":0,\"c\":[{\"tx\":\"metadata\"}]}"
	got := plainLyricText(raw)
	want := "first line\nsecond line"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestPlainLyricTextFromYRC(t *testing.T) {
	raw := "[0,2300](0,900,0)first (900,1400,0)line\n[2300,1800](2300,1800,0)second"
	got := plainLyricText(raw)
	want := "first line\nsecond"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestPlaylistIDFromURL(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{name: "plain id", raw: "3778678", want: "3778678"},
		{name: "web URL", raw: "https://music.163.com/#/playlist?id=3778678", want: "3778678"},
		{name: "query URL", raw: "https://music.163.com/playlist?id=3778678&userid=1", want: "3778678"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := PlaylistIDFromURL(tc.raw)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestService_EditConfigPersistsAndSyncs(t *testing.T) {
	store := newMemoryStore()
	client := &fakeClient{
		playlist: &Playlist{
			ID:   "3778678",
			Name: "Playlist",
			Tracks: []*Song{
				{ID: 1, Name: "Song", Duration: 180},
			},
		},
		account: &Account{Nickname: "User"},
		songURL: &SongURL{
			URL:     "https://example.test/song.mp3",
			BitRate: 128,
		},
	}
	service := NewService(store, client, slog.New(slog.NewTextHandler(io.Discard, nil)))

	conf, err := service.EditConfig(Config{
		PlaylistURL: "https://music.163.com/#/playlist?id=3778678",
		Quality:     QualityExHigh,
		Cookie:      "MUSIC_U=test",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if conf.PlaylistID != "3778678" {
		t.Fatalf("playlist id = %q, want 3778678", conf.PlaylistID)
	}
	if conf.TrackCount != 1 {
		t.Fatalf("track count = %d, want 1", conf.TrackCount)
	}
	if !conf.HasCookie {
		t.Fatal("expected cookie flag")
	}
	if conf.AccountName != "User" {
		t.Fatalf("account name = %q, want User", conf.AccountName)
	}
}

func TestService_RandomPlayableTrackSkipsUnplayable(t *testing.T) {
	store := newMemoryStore()
	store.props[propPlaylistURL] = "3778678"
	store.props[propQuality] = string(QualityStandard)
	client := &fakeClient{
		playlist: &Playlist{
			ID:   "3778678",
			Name: "Playlist",
			Tracks: []*Song{
				{ID: 1, Name: "Song", Duration: 180},
			},
		},
		songURLErr: errors.New("not playable"),
	}
	service := NewService(store, client, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := service.Load(); err != nil {
		t.Fatalf("unexpected load error: %v", err)
	}

	_, err := service.RandomPlayableTrack()
	if err == nil {
		t.Fatal("expected error")
	}
}
