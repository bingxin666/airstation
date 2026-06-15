package netease

import (
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/cheatsnake/airstation/internal/pkg/ffmpeg"
	"github.com/cheatsnake/airstation/internal/station"
)

const examplePlaylistURL = "https://music.163.com/playlist?id=5006183200"

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
	mutex       sync.Mutex
	playlistN   int
}

func (c *fakeClient) Playlist(id string, cookie string) (*Playlist, error) {
	c.mutex.Lock()
	c.playlistN++
	c.mutex.Unlock()
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

func (c *fakeClient) playlistCalls() int {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return c.playlistN
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
		{name: "example playlist URL", raw: examplePlaylistURL, want: "5006183200"},
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

func TestHTTPClient_PlaylistParsesExamplePlaylistShape(t *testing.T) {
	raw := rawPlaylist{
		ID:   5006183200,
		Name: "_笨蛋冰_喜欢的音乐",
		Tracks: []rawTrack{
			{
				ID:   3382894554,
				Name: "Herb Tea (feat. Nanahira)",
				Artists: []rawArtist{
					{Name: "Kirara Magic"},
				},
				Album: rawAlbum{Name: "A Pocket of Moss and Magic"},
				DT:    211015,
			},
		},
		TrackIDs: []rawTrackID{
			{ID: 3382894554},
			{ID: 2156452103},
		},
	}

	song := raw.Tracks[0].song()
	if song.ID != 3382894554 {
		t.Fatalf("song id = %d, want 3382894554", song.ID)
	}
	if song.Name != "Herb Tea (feat. Nanahira)" {
		t.Fatalf("song name = %q", song.Name)
	}
	if song.Duration != 211.015 {
		t.Fatalf("song duration = %f, want 211.015", song.Duration)
	}
	if len(song.Artists) != 1 || song.Artists[0] != "Kirara Magic" {
		t.Fatalf("artists = %#v", song.Artists)
	}
	if song.Album != "A Pocket of Moss and Magic" {
		t.Fatalf("album = %q", song.Album)
	}

	ids := raw.trackIDs()
	if len(ids) != 2 || ids[0] != 3382894554 || ids[1] != 2156452103 {
		t.Fatalf("track ids = %#v", ids)
	}
}

func TestHTTPClient_SongDetailParsesExamplePlaylistShape(t *testing.T) {
	raw := rawTrackOld{
		ID:   3382894554,
		Name: "Herb Tea (feat. Nanahira)",
		Artists: []rawArtistOld{
			{Name: "Kirara Magic"},
		},
		Album:    rawAlbum{Name: "A Pocket of Moss and Magic"},
		Duration: 211015,
	}

	song := raw.song()
	if song.ID != 3382894554 {
		t.Fatalf("song id = %d, want 3382894554", song.ID)
	}
	if song.Name != "Herb Tea (feat. Nanahira)" {
		t.Fatalf("song name = %q", song.Name)
	}
	if song.Duration != 211.015 {
		t.Fatalf("song duration = %f, want 211.015", song.Duration)
	}
	if len(song.Artists) != 1 || song.Artists[0] != "Kirara Magic" {
		t.Fatalf("artists = %#v", song.Artists)
	}
	if song.Album != "A Pocket of Moss and Magic" {
		t.Fatalf("album = %q", song.Album)
	}
}

func TestIntegration_ExamplePlaylistCanGenerateHLS(t *testing.T) {
	if os.Getenv("AIRSTATION_NETEASE_INTEGRATION") != "1" {
		t.Skip("set AIRSTATION_NETEASE_INTEGRATION=1 to call NetEase and FFmpeg")
	}

	client := NewHTTPClient()
	playlistID, err := PlaylistIDFromURL(examplePlaylistURL)
	if err != nil {
		t.Fatalf("parse playlist id: %v", err)
	}

	playlist, err := client.Playlist(playlistID, "")
	if err != nil {
		t.Fatalf("fetch playlist: %v", err)
	}
	if playlist.ID != "5006183200" {
		t.Fatalf("playlist id = %q, want 5006183200", playlist.ID)
	}
	if len(playlist.Tracks) == 0 {
		t.Fatal("playlist returned no tracks")
	}

	var playable *PlayableTrack
	var lastErr error
	for _, song := range playlist.Tracks {
		source, err := client.SongURL(song.ID, QualityStandard, "")
		if err != nil {
			lastErr = err
			continue
		}
		playable = &PlayableTrack{
			Track:  song.Track(source.BitRate),
			SongID: song.ID,
			URL:    source.URL,
		}
		break
	}
	if playable == nil {
		t.Fatalf("no playable tracks in example playlist; last error: %v", lastErr)
	}

	outDir := t.TempDir()
	if err := ffmpeg.NewCLI().MakeRemoteHLSPlaylist(playable.URL, outDir, "example-", 5, playable.Track.BitRate); err != nil {
		t.Fatalf("generate hls for song %d %q: %v", playable.SongID, playable.Track.Name, err)
	}

	playlistPath := filepath.Join(outDir, "example-.m3u8")
	data, err := os.ReadFile(playlistPath)
	if err != nil {
		t.Fatalf("read generated hls playlist: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("generated hls playlist is empty")
	}
	matches, err := filepath.Glob(filepath.Join(outDir, "example-*.ts"))
	if err != nil {
		t.Fatalf("glob generated hls segments: %v", err)
	}
	if len(matches) == 0 {
		t.Fatal("generated no hls segments")
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

func TestService_RunAutoSyncRefreshesConfiguredPlaylist(t *testing.T) {
	store := newMemoryStore()
	store.props[propPlaylistURL] = examplePlaylistURL
	store.props[propQuality] = string(QualityStandard)
	client := &fakeClient{
		playlist: &Playlist{
			ID:   "5006183200",
			Name: "Playlist",
			Tracks: []*Song{
				{ID: 1, Name: "Song", Duration: 180},
			},
		},
	}
	service := NewService(store, client, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := service.Load(); err != nil {
		t.Fatalf("load service: %v", err)
	}
	if client.playlistCalls() != 1 {
		t.Fatalf("playlist calls after load = %d, want 1", client.playlistCalls())
	}

	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		service.runAutoSync(time.Millisecond, stop)
		close(done)
	}()

	deadline := time.After(250 * time.Millisecond)
	for client.playlistCalls() < 2 {
		select {
		case <-deadline:
			close(stop)
			<-done
			t.Fatalf("auto-sync did not refresh playlist; calls=%d", client.playlistCalls())
		default:
			time.Sleep(time.Millisecond)
		}
	}
	close(stop)
	<-done
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
