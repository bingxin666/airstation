package http

import (
	"log/slog"
	"mime"
	"net/http"
	"strconv"
	"time"

	"github.com/cheatsnake/airstation/internal/config"
	"github.com/cheatsnake/airstation/internal/netease"
	"github.com/cheatsnake/airstation/internal/pkg/ffmpeg"
	"github.com/cheatsnake/airstation/internal/pkg/hls"
	"github.com/cheatsnake/airstation/internal/pkg/sse"
	"github.com/cheatsnake/airstation/internal/playback"
	"github.com/cheatsnake/airstation/internal/station"
	"github.com/cheatsnake/airstation/internal/storage"
	"github.com/rs/cors"
)

type Server struct {
	playbackState   *playback.State
	eventsEmitter   *sse.Emitter
	playbackService *playback.Service
	stationService  *station.Service
	netEaseService  *netease.Service
	config          *config.Config
	logger          *slog.Logger
	router          *http.ServeMux
}

func NewServer(store storage.Storage, conf *config.Config, logger *slog.Logger) *Server {
	ffmpegCLI := ffmpeg.NewCLI()
	ps := playback.NewService(store)
	ss := station.NewService(store)
	ns := netease.NewService(store, nil, logger.WithGroup("netease"))
	state := playback.NewState(ns, ffmpegCLI, ps, conf.TmpDir, logger.WithGroup("playback"))

	return &Server{
		playbackState:   state,
		eventsEmitter:   sse.NewEmitter(),
		playbackService: ps,
		stationService:  ss,
		netEaseService:  ns,
		config:          conf,
		logger:          logger.WithGroup("http"),
		router:          http.NewServeMux(),
	}
}

func (s *Server) Run() {
	s.registerMP2TMimeType()

	// Public handlers
	s.router.HandleFunc("GET /stream", s.handleHLSPlaylist)
	s.router.HandleFunc("GET /api/v1/events", s.handleEvents)
	s.router.HandleFunc("GET /api/v1/station/info", s.handleStationInfo)
	s.router.HandleFunc("POST /api/v1/login", s.handleLogin)
	s.router.Handle("GET /static/tmp/", s.handleStaticDirWithoutCache("/static/tmp", s.config.TmpDir))
	s.router.Handle("GET /api/v1/playback", http.HandlerFunc(s.handlePlaybackState))
	s.router.Handle("GET /api/v1/playback/history", http.HandlerFunc(s.handlePlaybackHistory))
	s.router.Handle("GET /api/v1/playback/lyrics", http.HandlerFunc(s.handlePlaybackLyrics))

	// Protected handlers
	s.router.Handle("POST /api/v1/playback/pause", s.jwtAuth(http.HandlerFunc(s.handlePausePlayback)))
	s.router.Handle("POST /api/v1/playback/play", s.jwtAuth(http.HandlerFunc(s.handlePlayPlayback)))
	s.router.Handle("GET /api/v1/netease/config", s.jwtAuth(http.HandlerFunc(s.handleNetEaseConfig)))
	s.router.Handle("PUT /api/v1/netease/config", s.jwtAuth(http.HandlerFunc(s.handleEditNetEaseConfig)))
	s.router.Handle("POST /api/v1/netease/sync", s.jwtAuth(http.HandlerFunc(s.handleSyncNetEasePlaylist)))
	s.router.Handle("PUT /api/v1/station/info", s.jwtAuth(http.HandlerFunc(s.handleEditStationInfo)))

	s.router.Handle("GET /studio/", s.handleStaticDir("/studio/", s.config.StudioDir))
	s.router.Handle("GET /", s.handleStaticDir("/", s.config.PlayerDir))

	s.listenEvents()

	if err := s.netEaseService.Load(); err != nil {
		s.logger.Warn("NetEase config loading failed: " + err.Error())
	}

	err := s.playbackState.Play()
	if err != nil {
		s.logger.Warn("Auto start playing failed: " + err.Error())
	}

	go s.playbackState.Run()
	go s.netEaseService.RunAutoSync(nil)
	s.playbackService.DeleteOldPlaybackHistory()

	s.logger.Info("Server starts on http://localhost:" + s.config.HTTPPort)
	err = http.ListenAndServe(":"+s.config.HTTPPort, cors.Default().Handler(s.router))
	if err != nil {
		s.logger.Error("Listen and serve failed", slog.String("info", err.Error()))
	}
}

func (s *Server) registerMP2TMimeType() {
	err := mime.AddExtensionType(hls.SegmentExtension, "video/mp2t")
	if err != nil {
		s.logger.Error("MP2T mime type registration failed", slog.String("info", err.Error()))
	}
}

func (s *Server) countListeners() *sse.Event {
	count := s.eventsEmitter.CountSubscribers()
	return sse.NewEvent(eventCountListeners, strconv.Itoa(count))
}

func (s *Server) listenEvents() {
	countConnectionTicker := time.Tick(5 * time.Second)

	// TODO: add context for gracefull shutdown

	go func() {
		for range countConnectionTicker {
			event := s.countListeners()
			s.eventsEmitter.RegisterEvent(event.Name, event.Data)
		}
	}()

	go func() {
		for {
			select {
			case trackName := <-s.playbackState.PlayNotify:
				s.eventsEmitter.RegisterEvent(eventPlay, trackName)
			case <-s.playbackState.PauseNotify:
				s.eventsEmitter.RegisterEvent(eventPause, " ")
			case trackName := <-s.playbackState.NewTrackNotify:
				s.eventsEmitter.RegisterEvent(eventNewTrack, trackName)
			}
		}
	}()
}
