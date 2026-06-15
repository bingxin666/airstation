package netease

import (
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net/url"
	"regexp"
	"strconv"
	"sync"
	"time"
)

type Service struct {
	store  Store
	client Client
	log    *slog.Logger

	mutex        sync.RWMutex
	config       Config
	playlistID   string
	playlistName string
	tracks       []*Song
	lyricsCache  map[int64]*Lyrics
	accountName  string
	lastError    string
	lastSyncedAt int64
}

func NewService(store Store, client Client, log *slog.Logger) *Service {
	if client == nil {
		client = NewHTTPClient()
	}

	return &Service{
		store:  store,
		client: client,
		log:    log,
		config: Config{
			Quality: defaultQuality,
		},
		lyricsCache: map[int64]*Lyrics{},
	}
}

func (s *Service) Load() error {
	props, err := s.store.StationProperties()
	if err != nil {
		return err
	}

	conf := Config{
		Quality: defaultQuality,
	}
	for _, prop := range props {
		switch prop.Key {
		case propPlaylistURL:
			conf.PlaylistURL = prop.Value
		case propQuality:
			conf.Quality = Quality(prop.Value)
		case propCookie:
			conf.Cookie = prop.Value
		}
	}

	if !isValidQuality(conf.Quality) {
		conf.Quality = defaultQuality
	}

	s.mutex.Lock()
	s.config = conf
	s.mutex.Unlock()

	if conf.Cookie != "" {
		s.refreshAccount(conf.Cookie)
	}

	if conf.PlaylistURL != "" {
		return s.Sync()
	}

	return nil
}

func (s *Service) Config() PublicConfig {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	return PublicConfig{
		PlaylistURL:  s.config.PlaylistURL,
		PlaylistID:   s.playlistID,
		Quality:      s.config.Quality,
		HasCookie:    s.config.Cookie != "",
		AccountName:  s.accountName,
		TrackCount:   len(s.tracks),
		LastError:    s.lastError,
		LastSyncedAt: s.lastSyncedAt,
	}
}

func (s *Service) EditConfig(conf Config) (PublicConfig, error) {
	current := s.rawConfig()
	if conf.ClearCookie {
		conf.Cookie = ""
	} else if conf.Cookie == "" {
		conf.Cookie = current.Cookie
	}

	playlistID := ""
	if conf.PlaylistURL != "" {
		var err error
		playlistID, err = PlaylistIDFromURL(conf.PlaylistURL)
		if err != nil {
			return s.Config(), err
		}
	}

	if !isValidQuality(conf.Quality) {
		return s.Config(), fmt.Errorf("unsupported quality: %s", conf.Quality)
	}

	if current.PlaylistURL != conf.PlaylistURL {
		if _, err := s.store.UpsertStationProperty(propPlaylistURL, conf.PlaylistURL); err != nil {
			return s.Config(), err
		}
	}
	if current.Quality != conf.Quality {
		if _, err := s.store.UpsertStationProperty(propQuality, string(conf.Quality)); err != nil {
			return s.Config(), err
		}
	}
	if current.Cookie != conf.Cookie {
		if conf.Cookie == "" {
			_ = s.store.DeleteStationProperty(propCookie)
		} else if _, err := s.store.UpsertStationProperty(propCookie, conf.Cookie); err != nil {
			return s.Config(), err
		}
	}

	s.mutex.Lock()
	s.config = conf
	s.playlistID = playlistID
	s.lyricsCache = map[int64]*Lyrics{}
	s.lastError = ""
	s.mutex.Unlock()

	if conf.Cookie != "" {
		s.refreshAccount(conf.Cookie)
	} else {
		s.mutex.Lock()
		s.accountName = ""
		s.mutex.Unlock()
	}

	if conf.PlaylistURL != "" {
		if err := s.Sync(); err != nil {
			return s.Config(), err
		}
	}

	return s.Config(), nil
}

func (s *Service) Sync() error {
	conf := s.rawConfig()
	if conf.PlaylistURL == "" {
		return errors.New("netease playlist URL is not configured")
	}

	playlistID, err := PlaylistIDFromURL(conf.PlaylistURL)
	if err != nil {
		return err
	}

	playlist, err := s.client.Playlist(playlistID, conf.Cookie)
	if err != nil {
		s.setLastError(err)
		return err
	}
	if len(playlist.Tracks) == 0 {
		err := errors.New("netease playlist has no tracks")
		s.setLastError(err)
		return err
	}

	s.mutex.Lock()
	s.playlistID = playlistID
	s.playlistName = playlist.Name
	s.tracks = playlist.Tracks
	s.lastSyncedAt = time.Now().Unix()
	s.lastError = ""
	s.mutex.Unlock()

	return nil
}

func (s *Service) RandomPlayableTrack() (*PlayableTrack, error) {
	if len(s.snapshotTracks()) == 0 {
		if err := s.Sync(); err != nil {
			return nil, err
		}
	}

	conf := s.rawConfig()
	tracks := s.snapshotTracks()
	if len(tracks) == 0 {
		return nil, errors.New("netease playlist has no tracks")
	}

	start := rand.IntN(len(tracks))
	var lastErr error
	for i := 0; i < len(tracks); i++ {
		song := tracks[(start+i)%len(tracks)]
		source, err := s.client.SongURL(song.ID, conf.Quality, conf.Cookie)
		if err != nil {
			lastErr = err
			continue
		}
		if source == nil || source.URL == "" {
			lastErr = fmt.Errorf("song %d has no playable URL", song.ID)
			continue
		}

		return &PlayableTrack{
			Track:  song.Track(source.BitRate),
			SongID: song.ID,
			URL:    source.URL,
		}, nil
	}

	if lastErr == nil {
		lastErr = errors.New("no playable tracks found")
	}
	s.setLastError(lastErr)
	return nil, lastErr
}

func (s *Service) Lyrics(songID int64) (*Lyrics, error) {
	if songID <= 0 {
		return nil, errors.New("song id is empty")
	}

	s.mutex.RLock()
	cached := s.lyricsCache[songID]
	conf := s.config
	s.mutex.RUnlock()
	if cached != nil {
		return cached, nil
	}

	lyrics, err := s.client.Lyrics(songID, conf.Cookie)
	if err != nil {
		return nil, err
	}

	s.mutex.Lock()
	s.lyricsCache[songID] = lyrics
	s.mutex.Unlock()

	return lyrics, nil
}

func (s *Service) rawConfig() Config {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.config
}

func (s *Service) snapshotTracks() []*Song {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	tracks := make([]*Song, len(s.tracks))
	copy(tracks, s.tracks)
	return tracks
}

func (s *Service) refreshAccount(cookie string) {
	account, err := s.client.Account(cookie)
	if err != nil {
		s.log.Warn("Failed to check NetEase account", slog.String("error", err.Error()))
		return
	}

	s.mutex.Lock()
	s.accountName = account.Nickname
	s.mutex.Unlock()
}

func (s *Service) setLastError(err error) {
	s.mutex.Lock()
	s.lastError = err.Error()
	s.mutex.Unlock()
}

func isValidQuality(q Quality) bool {
	_, ok := bitrateByQuality[q]
	return ok
}

func bitrateForQuality(q Quality) int {
	bitrate, ok := bitrateByQuality[q]
	if !ok {
		return bitrateByQuality[defaultQuality]
	}
	return bitrate
}

func songTrackID(id int64) string {
	return "netease-" + strconv.FormatInt(id, 10)
}

var playlistIDPattern = regexp.MustCompile(`(?:playlist[/#?].*?id=|playlist\?id=|[?&]id=)(\d+)`)

func PlaylistIDFromURL(raw string) (string, error) {
	if raw == "" {
		return "", errors.New("playlist URL is empty")
	}

	if _, err := strconv.ParseInt(raw, 10, 64); err == nil {
		return raw, nil
	}

	parsed, err := url.Parse(raw)
	if err == nil {
		if id := parsed.Query().Get("id"); id != "" {
			if _, err := strconv.ParseInt(id, 10, 64); err == nil {
				return id, nil
			}
		}
	}

	matches := playlistIDPattern.FindStringSubmatch(raw)
	if len(matches) == 2 {
		return matches[1], nil
	}

	return "", fmt.Errorf("could not extract playlist id from %q", raw)
}
