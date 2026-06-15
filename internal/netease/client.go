package netease

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const neteaseBaseURL = "https://music.163.com"

type HTTPClient struct {
	client *http.Client
}

func NewHTTPClient() *HTTPClient {
	return &HTTPClient{
		client: &http.Client{
			Timeout: 25 * time.Second,
		},
	}
}

func (c *HTTPClient) Playlist(id string, cookie string) (*Playlist, error) {
	endpoint := neteaseBaseURL + "/api/v6/playlist/detail?id=" + url.QueryEscape(id)
	var resp playlistResponse
	if err := c.getJSON(endpoint, cookie, &resp); err != nil {
		return nil, err
	}

	if resp.Code != 0 && resp.Code != http.StatusOK {
		return nil, fmt.Errorf("netease playlist request failed with code %d", resp.Code)
	}

	playlist := resp.Playlist
	if playlist.ID == 0 && len(resp.Result.Tracks) > 0 {
		playlist.ID = resp.Result.ID
		playlist.Name = resp.Result.Name
	}

	tracks := make([]*Song, 0, len(playlist.Tracks)+len(resp.Result.Tracks))
	for _, raw := range playlist.Tracks {
		song := raw.song()
		if song.ID == 0 || song.Name == "" || song.Duration <= 0 {
			continue
		}
		tracks = append(tracks, song)
	}
	for _, raw := range resp.Result.Tracks {
		song := raw.song()
		if song.ID == 0 || song.Name == "" || song.Duration <= 0 {
			continue
		}
		tracks = append(tracks, song)
	}

	playlistResult := &Playlist{
		ID:     strconv.FormatInt(playlist.ID, 10),
		Name:   playlist.Name,
		Tracks: tracks,
	}

	if len(resp.Playlist.TrackIDs) > len(playlistResult.Tracks) {
		detailTracks, err := c.songDetails(resp.Playlist.trackIDs(), cookie)
		if err == nil && len(detailTracks) > len(playlistResult.Tracks) {
			playlistResult.Tracks = detailTracks
		}
	}

	if len(playlistResult.Tracks) >= len(resp.Playlist.TrackIDs) || len(playlistResult.Tracks) >= 20 {
		return playlistResult, nil
	}

	oldPlaylist, err := c.legacyPlaylist(id, cookie)
	if err == nil && len(oldPlaylist.Tracks) > len(playlistResult.Tracks) {
		return oldPlaylist, nil
	}

	return playlistResult, nil
}

func (c *HTTPClient) songDetails(ids []int64, cookie string) ([]*Song, error) {
	const batchSize = 200
	tracks := make([]*Song, 0, len(ids))

	for start := 0; start < len(ids); start += batchSize {
		end := min(start+batchSize, len(ids))
		rawIDs, err := json.Marshal(ids[start:end])
		if err != nil {
			return nil, err
		}

		endpoint := neteaseBaseURL + "/api/song/detail?ids=" + url.QueryEscape(string(rawIDs))
		var resp songDetailResponse
		if err := c.getJSON(endpoint, cookie, &resp); err != nil {
			return nil, err
		}
		if resp.Code != 0 && resp.Code != http.StatusOK {
			return nil, fmt.Errorf("netease song detail request failed with code %d", resp.Code)
		}

		for _, raw := range resp.Songs {
			song := raw.song()
			if song.ID == 0 || song.Name == "" || song.Duration <= 0 {
				continue
			}
			tracks = append(tracks, song)
		}
	}

	return tracks, nil
}

func (c *HTTPClient) legacyPlaylist(id string, cookie string) (*Playlist, error) {
	endpoint := neteaseBaseURL + "/api/playlist/detail?id=" + url.QueryEscape(id)
	var resp playlistResponse
	if err := c.getJSON(endpoint, cookie, &resp); err != nil {
		return nil, err
	}

	if resp.Code != 0 && resp.Code != http.StatusOK {
		return nil, fmt.Errorf("netease legacy playlist request failed with code %d", resp.Code)
	}

	tracks := make([]*Song, 0, len(resp.Result.Tracks))
	for _, raw := range resp.Result.Tracks {
		song := raw.song()
		if song.ID == 0 || song.Name == "" || song.Duration <= 0 {
			continue
		}
		tracks = append(tracks, song)
	}

	return &Playlist{
		ID:     strconv.FormatInt(resp.Result.ID, 10),
		Name:   resp.Result.Name,
		Tracks: tracks,
	}, nil
}

func (c *HTTPClient) SongURL(songID int64, quality Quality, cookie string) (*SongURL, error) {
	values := url.Values{}
	values.Set("id", strconv.FormatInt(songID, 10))
	values.Set("ids", "["+strconv.FormatInt(songID, 10)+"]")
	values.Set("br", strconv.Itoa(bitrateForQuality(quality)))

	endpoint := neteaseBaseURL + "/api/song/enhance/player/url?" + values.Encode()
	var resp songURLResponse
	if err := c.getJSON(endpoint, cookie, &resp); err != nil {
		return nil, err
	}

	if resp.Code != 0 && resp.Code != http.StatusOK {
		return nil, fmt.Errorf("netease song URL request failed with code %d", resp.Code)
	}
	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("netease song %d returned no URL data", songID)
	}

	data := resp.Data[0]
	if data.Code != http.StatusOK || data.URL == "" {
		return nil, fmt.Errorf("netease song %d is not playable, code %d", songID, data.Code)
	}

	return &SongURL{
		URL:     data.URL,
		BitRate: max(data.BR/1000, bitrateForQuality(quality)/1000),
	}, nil
}

func (c *HTTPClient) Lyrics(songID int64, cookie string) (*Lyrics, error) {
	values := url.Values{}
	values.Set("id", strconv.FormatInt(songID, 10))
	values.Set("lv", "-1")
	values.Set("kv", "-1")
	values.Set("tv", "-1")
	values.Set("yv", "-1")
	values.Set("ytv", "-1")
	values.Set("rv", "-1")

	endpoint := neteaseBaseURL + "/api/song/lyric/v1?" + values.Encode()
	var resp lyricResponse
	if err := c.getJSON(endpoint, cookie, &resp); err != nil {
		endpoint = neteaseBaseURL + "/api/song/lyric?" + values.Encode()
		if fallbackErr := c.getJSON(endpoint, cookie, &resp); fallbackErr != nil {
			return nil, err
		}
	}

	if resp.Code != 0 && resp.Code != http.StatusOK {
		return nil, fmt.Errorf("netease lyric request failed with code %d", resp.Code)
	}

	lyrics := &Lyrics{
		SongID: songID,
		YRC:    strings.TrimSpace(resp.YRC.Lyric),
		LRC:    strings.TrimSpace(resp.LRC.Lyric),
	}
	lyrics.Text = plainLyricText(firstNonEmpty(lyrics.LRC, lyrics.YRC))
	switch {
	case lyrics.YRC != "":
		lyrics.Kind = "word"
	case lyrics.LRC != "":
		lyrics.Kind = "line"
	case lyrics.Text != "":
		lyrics.Kind = "text"
	default:
		lyrics.Kind = "none"
	}

	return lyrics, nil
}

func (c *HTTPClient) Account(cookie string) (*Account, error) {
	if cookie == "" {
		return &Account{}, nil
	}

	var resp accountResponse
	if err := c.getJSON(neteaseBaseURL+"/api/nuser/account/get", cookie, &resp); err != nil {
		return nil, err
	}

	if resp.Code != 0 && resp.Code != http.StatusOK {
		return nil, fmt.Errorf("netease account request failed with code %d", resp.Code)
	}

	return &Account{Nickname: resp.Profile.Nickname}, nil
}

func (c *HTTPClient) getJSON(endpoint, cookie string, target any) error {
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Referer", neteaseBaseURL)
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; Airstation/1.0)")
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("netease responded with %s", resp.Status)
	}

	decoder := json.NewDecoder(resp.Body)
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode netease response failed: %w", err)
	}

	return nil
}

type playlistResponse struct {
	Code     int            `json:"code"`
	Playlist rawPlaylist    `json:"playlist"`
	Result   rawPlaylistOld `json:"result"`
}

type rawPlaylist struct {
	ID       int64        `json:"id"`
	Name     string       `json:"name"`
	Tracks   []rawTrack   `json:"tracks"`
	TrackIDs []rawTrackID `json:"trackIds"`
}

type rawPlaylistOld struct {
	ID     int64         `json:"id"`
	Name   string        `json:"name"`
	Tracks []rawTrackOld `json:"tracks"`
}

type rawTrack struct {
	ID      int64       `json:"id"`
	Name    string      `json:"name"`
	Artists []rawArtist `json:"ar"`
	Album   rawAlbum    `json:"al"`
	DT      int64       `json:"dt"`
}

type rawTrackOld struct {
	ID       int64          `json:"id"`
	Name     string         `json:"name"`
	Artists  []rawArtistOld `json:"artists"`
	Album    rawAlbum       `json:"album"`
	Duration int64          `json:"duration"`
}

type rawTrackID struct {
	ID int64 `json:"id"`
}

type rawArtist struct {
	Name string `json:"name"`
}

type rawArtistOld struct {
	Name string `json:"name"`
}

type rawAlbum struct {
	Name string `json:"name"`
}

func (p rawPlaylist) trackIDs() []int64 {
	ids := make([]int64, 0, len(p.TrackIDs))
	for _, trackID := range p.TrackIDs {
		if trackID.ID > 0 {
			ids = append(ids, trackID.ID)
		}
	}
	return ids
}

func (t rawTrack) song() *Song {
	artists := make([]string, 0, len(t.Artists))
	for _, artist := range t.Artists {
		if strings.TrimSpace(artist.Name) != "" {
			artists = append(artists, artist.Name)
		}
	}

	return &Song{
		ID:       t.ID,
		Name:     t.Name,
		Artists:  artists,
		Album:    t.Album.Name,
		Duration: float64(t.DT) / 1000,
	}
}

func (t rawTrackOld) song() *Song {
	artists := make([]string, 0, len(t.Artists))
	for _, artist := range t.Artists {
		if strings.TrimSpace(artist.Name) != "" {
			artists = append(artists, artist.Name)
		}
	}

	return &Song{
		ID:       t.ID,
		Name:     t.Name,
		Artists:  artists,
		Album:    t.Album.Name,
		Duration: float64(t.Duration) / 1000,
	}
}

type songURLResponse struct {
	Code int `json:"code"`
	Data []struct {
		URL  string `json:"url"`
		BR   int    `json:"br"`
		Code int    `json:"code"`
	} `json:"data"`
}

type songDetailResponse struct {
	Code  int           `json:"code"`
	Songs []rawTrackOld `json:"songs"`
}

type lyricResponse struct {
	Code int `json:"code"`
	LRC  struct {
		Lyric string `json:"lyric"`
	} `json:"lrc"`
	YRC struct {
		Lyric string `json:"lyric"`
	} `json:"yrc"`
}

type accountResponse struct {
	Code    int `json:"code"`
	Profile struct {
		Nickname string `json:"nickname"`
	} `json:"profile"`
}

var (
	lrcTimestampPattern     = regexp.MustCompile(`\[[0-9]{1,2}:[0-9]{2}(?:\.[0-9]{1,3})?\]`)
	yrcLineTimestampPattern = regexp.MustCompile(`\[[0-9]+,[0-9]+\]`)
	yrcWordTimestampPattern = regexp.MustCompile(`\([0-9]+,[0-9]+,[0-9]+\)`)
)

func plainLyricText(raw string) string {
	lines := strings.Split(raw, "\n")
	clean := make([]string, 0, len(lines))
	for _, line := range lines {
		line = lrcTimestampPattern.ReplaceAllString(line, "")
		line = yrcLineTimestampPattern.ReplaceAllString(line, "")
		line = yrcWordTimestampPattern.ReplaceAllString(line, "")
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "{") || strings.HasPrefix(line, "[") {
			continue
		}
		clean = append(clean, line)
	}
	return strings.Join(clean, "\n")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
