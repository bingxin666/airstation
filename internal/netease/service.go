package netease

import (
	"encoding/json"
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

	mutex         sync.RWMutex
	config        Config
	playlistID    string
	playlistName  string
	tracks        []*Song
	recentSongIDs []int64
	lyricsCache   map[int64]*Lyrics
	accountName   string
	lastError     string
	lastSyncedAt  int64
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
	var recentSongIDs []int64
	for _, prop := range props {
		switch prop.Key {
		case propPlaylistURL:
			conf.PlaylistURL = prop.Value
		case propQuality:
			conf.Quality = Quality(prop.Value)
		case propCookie:
			conf.Cookie = prop.Value
		case propRecentSongIDs:
			recentSongIDs = recentSongIDsFromJSON(prop.Value)
		}
	}

	if !isValidQuality(conf.Quality) {
		conf.Quality = defaultQuality
	}

	s.mutex.Lock()
	s.config = conf
	s.recentSongIDs = recentSongIDs
	s.mutex.Unlock()

	if conf.Cookie != "" {
		s.refreshAccount(conf.Cookie)
	}

	if conf.PlaylistURL != "" {
		return s.Sync()
	}

	return nil
}

func (s *Service) RunAutoSync(stop <-chan struct{}) {
	s.runAutoSync(SyncInterval, stop)
}

func (s *Service) runAutoSync(interval time.Duration, stop <-chan struct{}) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if s.rawConfig().PlaylistURL == "" {
				continue
			}
			if err := s.Sync(); err != nil {
				s.log.Warn("NetEase playlist auto-sync failed", slog.String("error", err.Error()))
			}
		case <-stop:
			return
		}
	}
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

func (s *Service) RandomPlayableTrack(excludeSongIDs ...int64) (*PlayableTrack, error) {
	return s.RandomPlayableTrackAfter(nil, excludeSongIDs...)
}

func (s *Service) RandomPlayableTrackAfter(recentlyPlayedSongIDs []int64, excludeSongIDs ...int64) (*PlayableTrack, error) {
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

	excluded := songIDSet(s.recentSongIDsAfter(recentlyPlayedSongIDs))
	for _, id := range excludeSongIDs {
		if id > 0 {
			excluded[id] = struct{}{}
		}
	}

	candidates := make([]*Song, 0, len(tracks))
	for _, song := range tracks {
		if song == nil {
			continue
		}
		if _, ok := excluded[song.ID]; ok {
			continue
		}
		candidates = append(candidates, song)
	}
	if len(candidates) == 0 {
		err := errors.New("netease playlist has no tracks outside the recent playback list")
		s.setLastError(err)
		return nil, err
	}

	start := rand.IntN(len(candidates))
	var lastErr error
	for i := 0; i < len(candidates); i++ {
		song := candidates[(start+i)%len(candidates)]
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

func (s *Service) recentSongIDsAfter(songIDs []int64) []int64 {
	recent := s.snapshotRecentSongIDs()
	for _, id := range songIDs {
		if id <= 0 {
			continue
		}
		recent = trimRecentSongIDs(append([]int64{id}, recent...))
	}
	return recent
}

func (s *Service) RecordPlayedSong(songID int64) error {
	if songID <= 0 {
		return nil
	}

	s.mutex.Lock()
	recent := append([]int64{songID}, s.recentSongIDs...)
	recent = trimRecentSongIDs(recent)
	s.recentSongIDs = recent
	s.mutex.Unlock()

	raw, err := json.Marshal(recent)
	if err != nil {
		return err
	}
	_, err = s.store.UpsertStationProperty(propRecentSongIDs, string(raw))
	return err
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

func (s *Service) snapshotRecentSongIDs() []int64 {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	ids := make([]int64, len(s.recentSongIDs))
	copy(ids, s.recentSongIDs)
	return ids
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

func recentSongIDsFromJSON(raw string) []int64 {
	if raw == "" {
		return nil
	}

	var ids []int64
	if err := json.Unmarshal([]byte(raw), &ids); err != nil {
		return nil
	}
	return trimRecentSongIDs(ids)
}

func trimRecentSongIDs(ids []int64) []int64 {
	recent := make([]int64, 0, min(len(ids), maxRecentSongCount))
	seen := map[int64]struct{}{}
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		recent = append(recent, id)
		if len(recent) == maxRecentSongCount {
			break
		}
	}
	return recent
}

func songIDSet(ids []int64) map[int64]struct{} {
	set := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		set[id] = struct{}{}
	}
	return set
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
