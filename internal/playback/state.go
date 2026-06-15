// Package playback manages audio playback state, track transitions, and HLS playlist generation.
// It coordinates the timing and sequencing of audio tracks, maintaining synchronized state
// for streaming playback, including current position, play/pause control, and playlist updates.
// This package interacts with the track service to load tracks, generate segments, and handle
// queue changes in a thread-safe manner.
package playback

import (
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/cheatsnake/airstation/internal/netease"
	"github.com/cheatsnake/airstation/internal/pkg/ffmpeg"
	"github.com/cheatsnake/airstation/internal/pkg/hls"
	"github.com/cheatsnake/airstation/internal/track"
)

type HLSMaker interface {
	MakeRemoteHLSPlaylist(trackURL, outDir, segName string, segDuration, bitRate int) error
}

// State represents the current playback state of the application, including the currently playing track,
// elapsed playback time, playlist management, and synchronization tools for safe concurrent access.
type State struct {
	CurrentTrack        *track.Track `json:"currentTrack"`        // The currently playing track
	CurrentNetEaseID    int64        `json:"currentNetEaseID"`    // The NetEase song ID of the currently playing track
	CurrentTrackElapsed float64      `json:"currentTrackElapsed"` // Seconds elapsed since the current track started playing
	IsPlaying           bool         `json:"isPlaying"`           // Whether a track is currently playing
	UpdatedAt           int64        `json:"updatedAt"`           // Unix timestamp of the last state update

	NewTrackNotify chan string `json:"-"` // Channel to notify when a new track starts playing
	PlayNotify     chan string `json:"-"` // Channel to notify when playback starts
	PauseNotify    chan bool   `json:"-"` // Channel to notify when playback is paused

	PlaylistStr string        `json:"-"` // Current HLS playlist as a string
	playlist    *hls.Playlist // Internal representation of the HLS playlist
	playlistDir string        // Directory where HLS playlist segments are stored

	refreshCount    int64   // Number of state refresh cycles completed
	refreshInterval float64 // Time interval (in seconds) between state updates

	currentSourceURL  string
	nextPrepared      *preparedTrack
	followingPrepared *preparedTrack
	preloadInFlight   bool
	preloadGeneration int64

	netEaseService *netease.Service
	ffmpegCLI      HLSMaker

	log   *slog.Logger
	mutex sync.Mutex
}

type preparedTrack struct {
	track    *track.Track
	songID   int64
	url      string
	segments []*hls.Segment
}

// NewState creates and initializes a new playback State instance.
func NewState(ns *netease.Service, ffmpegCLI *ffmpeg.CLI, tmpDir string, log *slog.Logger) *State {
	return NewStateWithHLSMaker(ns, ffmpegCLI, tmpDir, log)
}

func NewStateWithHLSMaker(ns *netease.Service, hlsMaker HLSMaker, tmpDir string, log *slog.Logger) *State {
	return &State{
		CurrentTrack:        nil,
		CurrentNetEaseID:    0,
		CurrentTrackElapsed: 0,
		IsPlaying:           false,
		UpdatedAt:           time.Now().Unix(),

		NewTrackNotify: make(chan string),
		PlayNotify:     make(chan string),
		PauseNotify:    make(chan bool),

		netEaseService: ns,
		ffmpegCLI:      hlsMaker,

		refreshCount:    0,
		playlistDir:     tmpDir,
		refreshInterval: 1,

		log: log,
	}
}

// Run starts the state update loop which refreshes playback progress and switches tracks when needed.
func (s *State) Run() {
	ticker := time.NewTicker(time.Duration(s.refreshInterval) * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		s.mutex.Lock()
		if !s.IsPlaying || s.CurrentTrack == nil || s.playlist == nil {
			s.mutex.Unlock()
			continue
		}

		s.CurrentTrackElapsed += s.refreshInterval
		s.refreshCount++

		if s.CurrentTrackElapsed >= s.CurrentTrack.Duration {
			trackName, err := s.loadNextTrackLocked()
			if err != nil {
				s.log.Error(err.Error())
				s.pauseLocked()
				s.mutex.Unlock()
				s.PauseNotify <- false
				continue
			}

			s.PlaylistStr = s.playlist.Generate(s.CurrentTrackElapsed)
			s.UpdatedAt = time.Now().Unix()
			s.mutex.Unlock()

			s.ensurePreloaded()
			s.NewTrackNotify <- trackName
			continue
		}

		s.PlaylistStr = s.playlist.Generate(s.CurrentTrackElapsed)
		s.UpdatedAt = time.Now().Unix()
		s.mutex.Unlock()
	}
}

// Play starts playback by loading the current and next tracks into the HLS playlist.
func (s *State) Play() error {
	current, err := s.prepareRandomTrack()
	if err != nil {
		return err
	}
	if current == nil || current.track == nil || current.url == "" || len(current.segments) == 0 {
		return errors.New("netease playlist returned no playable current track")
	}

	next, err := s.prepareRandomTrack()
	if err != nil {
		return err
	}
	if next == nil || next.track == nil || next.url == "" || len(next.segments) == 0 {
		return errors.New("netease playlist returned no playable next track")
	}

	s.mutex.Lock()
	s.preloadGeneration++
	s.CurrentTrack = current.track
	s.CurrentNetEaseID = current.songID
	s.currentSourceURL = current.url
	s.nextPrepared = next
	s.followingPrepared = nil
	s.preloadInFlight = false
	s.CurrentTrackElapsed = 0
	s.playlist = hls.NewPlaylist(current.segments, next.segments)
	s.PlaylistStr = s.playlist.Generate(s.CurrentTrackElapsed)
	s.UpdatedAt = time.Now().Unix()
	s.IsPlaying = true
	s.mutex.Unlock()

	trackName := current.track.DisplayName()
	s.ensurePreloaded()
	s.PlayNotify <- trackName

	return nil
}

// Pause stops playback, clears current playback state and playlist.
func (s *State) Pause() {
	s.mutex.Lock()
	s.pauseLocked()
	s.mutex.Unlock()

	s.PauseNotify <- false
}

func (s *State) pauseLocked() {
	s.CurrentTrack = nil
	s.CurrentNetEaseID = 0
	s.currentSourceURL = ""
	s.nextPrepared = nil
	s.followingPrepared = nil
	s.preloadInFlight = false
	s.preloadGeneration++
	s.CurrentTrackElapsed = 0
	s.playlist = nil
	s.PlaylistStr = ""
	s.IsPlaying = false
	s.UpdatedAt = time.Now().Unix()
}

// Reload refreshes the current playlist based on updated queue state, used after queue changes.
func (s *State) Reload() error {
	if !s.Snapshot().IsPlaying {
		return nil
	}

	s.mutex.Lock()
	s.pauseLocked()
	s.mutex.Unlock()

	if err := s.Play(); err != nil {
		return err
	}

	return nil
}

// loadNextTrack advances the queue, resets elapsed time, and updates playlist with next segments.
func (s *State) loadNextTrackLocked() (string, error) {
	if s.nextPrepared == nil || s.nextPrepared.track == nil || len(s.nextPrepared.segments) == 0 {
		return "", errors.New("next netease track is not prepared")
	}

	s.CurrentTrackElapsed = 0
	current := s.nextPrepared
	s.CurrentTrack = current.track
	s.CurrentNetEaseID = current.songID
	s.currentSourceURL = current.url
	s.nextPrepared = s.followingPrepared
	s.followingPrepared = nil
	var nextSegments []*hls.Segment
	if s.nextPrepared != nil {
		nextSegments = s.nextPrepared.segments
	}
	s.playlist.Next(nextSegments)

	return current.track.DisplayName(), nil
}

func (s *State) ensurePreloaded() {
	s.mutex.Lock()
	target := ""
	if s.IsPlaying && !s.preloadInFlight {
		switch {
		case s.nextPrepared == nil:
			target = "next"
		case s.followingPrepared == nil:
			target = "following"
		}
	}
	if target == "" {
		s.mutex.Unlock()
		return
	}
	s.preloadInFlight = true
	generation := s.preloadGeneration
	s.mutex.Unlock()

	go func() {
		next, err := s.prepareRandomTrack()

		s.mutex.Lock()
		needsMorePreload := false
		s.preloadInFlight = false
		if err != nil {
			s.log.Error("Next track preload failed", slog.String("error", err.Error()))
			s.mutex.Unlock()
			return
		}
		if generation != s.preloadGeneration || !s.IsPlaying || s.playlist == nil {
			s.mutex.Unlock()
			return
		}
		if s.nextPrepared == nil {
			s.nextPrepared = next
			s.playlist.ChangeNext(next.segments)
			s.PlaylistStr = s.playlist.Generate(s.CurrentTrackElapsed)
			s.UpdatedAt = time.Now().Unix()
			needsMorePreload = s.followingPrepared == nil
		} else if s.followingPrepared == nil {
			s.followingPrepared = next
		}
		s.mutex.Unlock()

		if needsMorePreload {
			s.ensurePreloaded()
		}
	}()
}

func (s *State) prepareRandomTrack() (*preparedTrack, error) {
	source, err := s.netEaseService.RandomPlayableTrack()
	if err != nil {
		return nil, err
	}
	if source == nil || source.Track == nil || source.URL == "" {
		return nil, errors.New("netease playlist returned no playable track")
	}

	segments, err := s.makeHLSSegments(source, s.playlistDir)
	if err != nil {
		return nil, err
	}
	if len(segments) == 0 {
		return nil, fmt.Errorf("track %s generated no HLS segments", source.Track.ID)
	}

	return &preparedTrack{
		track:    source.Track,
		songID:   source.SongID,
		url:      source.URL,
		segments: segments,
	}, nil
}

// makeHLSSegments generates HLS segments for a given track.
func (s *State) makeHLSSegments(source *netease.PlayableTrack, dir string) ([]*hls.Segment, error) {
	if source == nil || source.Track == nil {
		return []*hls.Segment{}, nil
	}

	if source.URL == "" {
		return nil, fmt.Errorf("missing stream URL for track %s", source.Track.ID)
	}

	segmentID := fmt.Sprintf("%s-%d-", source.Track.ID, time.Now().UnixNano())
	err := s.ffmpegCLI.MakeRemoteHLSPlaylist(source.URL, dir, segmentID, hls.DefaultMaxSegmentDuration, source.Track.BitRate)
	if err != nil {
		return nil, err
	}

	segments := hls.GenerateSegments(
		source.Track.Duration,
		hls.DefaultMaxSegmentDuration,
		segmentID,
		"/static/tmp",
	)

	return segments, nil
}

func (s *State) Snapshot() PublicState {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	return PublicState{
		CurrentTrack:        s.CurrentTrack,
		CurrentNetEaseID:    s.CurrentNetEaseID,
		CurrentTrackElapsed: s.CurrentTrackElapsed,
		IsPlaying:           s.IsPlaying,
		UpdatedAt:           s.UpdatedAt,
	}
}

func (s *State) Playlist() string {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if !s.IsPlaying {
		return ""
	}
	return s.PlaylistStr
}

func (s *State) Lyrics() (*netease.Lyrics, error) {
	s.mutex.Lock()
	songID := s.CurrentNetEaseID
	s.mutex.Unlock()

	return s.netEaseService.Lyrics(songID)
}
