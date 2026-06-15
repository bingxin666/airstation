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

	currentSourceURL string
	nextNetEaseID    int64
	nextTrack        *track.Track
	nextSourceURL    string

	netEaseService  *netease.Service
	ffmpegCLI       *ffmpeg.CLI
	playbackService *Service

	log   *slog.Logger
	mutex sync.Mutex
}

// NewState creates and initializes a new playback State instance.
func NewState(ns *netease.Service, ffmpegCLI *ffmpeg.CLI, ps *Service, tmpDir string, log *slog.Logger) *State {
	return &State{
		CurrentTrack:        nil,
		CurrentNetEaseID:    0,
		CurrentTrackElapsed: 0,
		IsPlaying:           false,
		UpdatedAt:           time.Now().Unix(),

		NewTrackNotify: make(chan string),
		PlayNotify:     make(chan string),
		PauseNotify:    make(chan bool),

		netEaseService:  ns,
		ffmpegCLI:       ffmpegCLI,
		playbackService: ps,

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
		if !s.IsPlaying {
			continue
		}

		s.mutex.Lock()
		s.CurrentTrackElapsed += s.refreshInterval
		s.refreshCount++

		if s.CurrentTrackElapsed >= s.CurrentTrack.Duration {
			err := s.loadNextTrack()
			if err != nil {
				s.log.Error(err.Error())
				s.pauseLocked()
				s.mutex.Unlock()
				s.PauseNotify <- false
				continue
			}

			go s.playbackService.AddPlaybackHistory(s.CurrentTrack.Name)
		}

		s.PlaylistStr = s.playlist.Generate(s.CurrentTrackElapsed)
		s.UpdatedAt = time.Now().Unix()
		s.mutex.Unlock()
	}
}

// Play starts playback by loading the current and next tracks into the HLS playlist.
func (s *State) Play() error {
	current, err := s.netEaseService.RandomPlayableTrack()
	if err != nil {
		return err
	}
	if current == nil || current.Track == nil || current.URL == "" {
		return errors.New("netease playlist returned no playable current track")
	}

	next, err := s.netEaseService.RandomPlayableTrack()
	if err != nil {
		return err
	}
	if next == nil || next.Track == nil || next.URL == "" {
		return errors.New("netease playlist returned no playable next track")
	}

	err = s.initHLSPlaylist(current, next)
	if err != nil {
		return err
	}

	s.mutex.Lock()
	s.CurrentTrack = current.Track
	s.CurrentNetEaseID = current.SongID
	s.currentSourceURL = current.URL
	s.nextNetEaseID = next.SongID
	s.nextTrack = next.Track
	s.nextSourceURL = next.URL
	s.CurrentTrackElapsed = 0
	s.PlaylistStr = s.playlist.Generate(s.CurrentTrackElapsed)
	s.UpdatedAt = time.Now().Unix()
	s.IsPlaying = true
	s.mutex.Unlock()

	s.PlayNotify <- current.Track.Name
	go s.playbackService.AddPlaybackHistory(current.Track.Name)

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
	s.nextNetEaseID = 0
	s.nextTrack = nil
	s.nextSourceURL = ""
	s.CurrentTrackElapsed = 0
	s.playlist = nil
	s.PlaylistStr = ""
	s.IsPlaying = false
	s.UpdatedAt = time.Now().Unix()
}

// Reload refreshes the current playlist based on updated queue state, used after queue changes.
func (s *State) Reload() error {
	if !s.IsPlaying {
		return nil
	}

	s.Pause()
	if err := s.Play(); err != nil {
		return err
	}

	return nil
}

// initHLSPlaylist prepares HLS segments for the current and next tracks, initializing a new playlist.
func (s *State) initHLSPlaylist(current, next *netease.PlayableTrack) error {
	currentSeg, err := s.makeHLSSegments(current, s.playlistDir)
	if err != nil {
		return err
	}

	nextSeg, err := s.makeHLSSegments(next, s.playlistDir)
	if err != nil {
		return err
	}

	s.mutex.Lock()
	s.playlist = hls.NewPlaylist(currentSeg, nextSeg)
	s.UpdatedAt = time.Now().Unix()
	s.mutex.Unlock()

	return nil
}

// loadNextTrack advances the queue, resets elapsed time, and updates playlist with next segments.
func (s *State) loadNextTrack() error {
	s.CurrentTrackElapsed = 0
	current := &netease.PlayableTrack{
		Track:  s.nextTrack,
		SongID: s.nextNetEaseID,
		URL:    s.nextSourceURL,
	}
	next, err := s.netEaseService.RandomPlayableTrack()
	if err != nil {
		return err
	}

	if current.Track == nil || current.URL == "" {
		return errors.New("next netease track is not prepared")
	}

	s.CurrentTrack = current.Track
	s.CurrentNetEaseID = current.SongID
	s.currentSourceURL = current.URL
	s.nextNetEaseID = next.SongID
	s.nextTrack = next.Track
	s.nextSourceURL = next.URL

	nextTrackSegments, err := s.makeHLSSegments(next, s.playlistDir)
	if err != nil {
		return err
	}

	s.NewTrackNotify <- current.Track.Name
	s.playlist.Next(nextTrackSegments)
	return nil
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

func (s *State) Lyrics() (*netease.Lyrics, error) {
	s.mutex.Lock()
	songID := s.CurrentNetEaseID
	s.mutex.Unlock()

	return s.netEaseService.Lyrics(songID)
}
