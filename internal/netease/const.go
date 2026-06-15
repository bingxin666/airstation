package netease

import "time"

const (
	propPlaylistURL = "netease_playlist_url"
	propQuality     = "netease_quality"
	propCookie      = "netease_cookie"

	defaultQuality = QualityStandard
	SyncInterval   = 24 * time.Hour
)

var bitrateByQuality = map[Quality]int{
	QualityStandard: 128000,
	QualityHigher:   192000,
	QualityExHigh:   320000,
	QualityLossless: 999000,
	QualityHiRes:    1999000,
}
