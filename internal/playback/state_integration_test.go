package playback

import (
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/cheatsnake/airstation/internal/netease"
	"github.com/cheatsnake/airstation/internal/pkg/ffmpeg"
	"github.com/cheatsnake/airstation/internal/station"
)

const exampleNetEasePlaylistURL = "https://music.163.com/playlist?id=5006183200"

type integrationStationStore struct {
	props map[string]string
}

func newIntegrationStationStore() *integrationStationStore {
	return &integrationStationStore{
		props: map[string]string{
			"netease_playlist_url": exampleNetEasePlaylistURL,
			"netease_quality":      string(netease.QualityStandard),
		},
	}
}

func (s *integrationStationStore) StationProperties() ([]*station.Property, error) {
	props := make([]*station.Property, 0, len(s.props))
	for key, value := range s.props {
		props = append(props, &station.Property{Key: key, Value: value})
	}
	return props, nil
}

func (s *integrationStationStore) UpsertStationProperty(key, value string) (*station.Property, error) {
	s.props[key] = value
	return &station.Property{Key: key, Value: value}, nil
}

func (s *integrationStationStore) DeleteStationProperty(key string) error {
	delete(s.props, key)
	return nil
}

func TestIntegration_StatePlayCanGenerateExamplePlaylistStream(t *testing.T) {
	if os.Getenv("AIRSTATION_NETEASE_INTEGRATION") != "1" {
		t.Skip("set AIRSTATION_NETEASE_INTEGRATION=1 to call NetEase and FFmpeg")
	}

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	netEaseService := netease.NewService(newIntegrationStationStore(), nil, log)
	if err := netEaseService.Load(); err != nil {
		t.Fatalf("load netease example playlist: %v", err)
	}

	state := NewState(
		netEaseService,
		ffmpeg.NewCLI(),
		t.TempDir(),
		log,
	)

	go func() {
		for range state.PlayNotify {
		}
	}()

	if err := state.Play(); err != nil {
		t.Fatalf("play example playlist: %v", err)
	}

	if state.CurrentTrack == nil {
		t.Fatal("current track is nil after Play")
	}
	if state.CurrentNetEaseID == 0 {
		t.Fatal("current netease song id is empty after Play")
	}
	if state.PlaylistStr == "" {
		t.Fatal("playlist string is empty after Play")
	}
	if !strings.Contains(state.PlaylistStr, "#EXTM3U") {
		t.Fatalf("playlist string is not an m3u8 playlist:\n%s", state.PlaylistStr)
	}
	if !strings.Contains(state.PlaylistStr, ".ts") {
		t.Fatalf("playlist string contains no segments:\n%s", state.PlaylistStr)
	}
}
