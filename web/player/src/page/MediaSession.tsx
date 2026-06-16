import { createEffect, onCleanup, onMount } from "solid-js";
import { correctedPlaybackTimeMs } from "../store/playbackClock";
import { loadStationInfo, stationStore } from "../store/station";
import { trackStore } from "../store/track";
import { isValidURL } from "../utils/url";

const DEFAULT_STATION_TITLE = "Airstation";
const MEDIA_SESSION_REAPPLY_DELAYS_MS = [150, 750, 1500];

export const MediaSession = () => {
    onMount(() => {
        loadStationInfo()
            .then(() => refreshMediaSessionSoon())
            .catch((error) => console.log(error));
    });

    createEffect(() => {
        refreshMediaSessionMetadata();
    });

    createEffect(() => {
        refreshMediaSessionPlaybackState();
    });

    createEffect(() => {
        refreshMediaSessionPosition();
    });

    onCleanup(() => {
        const mediaSession = getMediaSession();
        if (!mediaSession) return;

        mediaSession.metadata = null;
        mediaSession.playbackState = "none";
        clearMediaSessionPosition();
    });

    return null;
};

export const setMediaSessionActionHandlers = (media?: HTMLMediaElement) => {
    const mediaSession = getMediaSession();
    if (!mediaSession || !media) return;

    setActionHandler(mediaSession, "play", () => {
        media.play().catch((error) => console.log(error));
    });
    setActionHandler(mediaSession, "pause", () => {
        media.pause();
    });
    setActionHandler(mediaSession, "stop", () => {
        media.pause();
    });
};

export const refreshMediaSession = () => {
    refreshMediaSessionMetadata();
    refreshMediaSessionPlaybackState();
    refreshMediaSessionPosition();
};

export const refreshMediaSessionSoon = () => {
    refreshMediaSession();
    window.requestAnimationFrame(refreshMediaSession);

    MEDIA_SESSION_REAPPLY_DELAYS_MS.forEach((delay) => {
        window.setTimeout(refreshMediaSession, delay);
    });
};

export const clearMediaSessionActionHandlers = () => {
    const mediaSession = getMediaSession();
    if (!mediaSession) return;

    setActionHandler(mediaSession, "play", null);
    setActionHandler(mediaSession, "pause", null);
    setActionHandler(mediaSession, "stop", null);
};

const refreshMediaSessionMetadata = () => {
    const mediaSession = getMediaSession();
    if (!mediaSession || typeof MediaMetadata === "undefined") return;

    mediaSession.metadata = new MediaMetadata({
        title: metadataTitle(),
        artist: metadataArtist(),
        album: metadataAlbum(),
        artwork: metadataArtwork(),
    });
};

const refreshMediaSessionPlaybackState = () => {
    const mediaSession = getMediaSession();
    if (!mediaSession) return;

    if (trackStore.isPlay) {
        mediaSession.playbackState = "playing";
        return;
    }

    mediaSession.playbackState = trackStore.trackName ? "paused" : "none";
};

export const refreshMediaSessionPosition = () => {
    const mediaSession = getMediaSession();
    if (!mediaSession || typeof mediaSession.setPositionState !== "function") return;
    if (!hasTrackMetadata() || trackStore.durationMs <= 0) {
        clearMediaSessionPosition();
        return;
    }

    const duration = trackStore.durationMs / 1000;
    const position = Math.min(currentTimeMs() / 1000, duration);

    try {
        mediaSession.setPositionState({
            duration,
            playbackRate: 1,
            position,
        });
    } catch (error) {
        console.log(error);
    }
};

const clearMediaSessionPosition = () => {
    const mediaSession = getMediaSession();
    if (!mediaSession || typeof mediaSession.setPositionState !== "function") return;

    try {
        mediaSession.setPositionState();
    } catch (error) {
        console.log(error);
    }
};

const metadataTitle = () => {
    if (hasTrackMetadata()) return trackStore.trackName;
    return stationStore.info?.name || DEFAULT_STATION_TITLE;
};

const metadataArtist = () => {
    if (hasTrackMetadata()) return trackStore.trackArtist || stationStore.info?.name || DEFAULT_STATION_TITLE;
    return stationStore.info?.location || "";
};

const metadataAlbum = () => {
    return stationStore.info?.name || DEFAULT_STATION_TITLE;
};

const metadataArtwork = (): MediaImage[] => {
    const artwork = [hasTrackMetadata() ? trackStore.coverURL : "", stationStore.info?.logoURL, stationStore.info?.faviconURL]
        .filter((url): url is string => Boolean(url && isValidURL(url)))
        .flatMap((src) => mediaArtwork(src));

    return artwork;
};

const mediaArtwork = (src: string): MediaImage[] => {
    return [
        { src, sizes: "512x512", type: imageMimeType(src) },
        { src, sizes: "256x256", type: imageMimeType(src) },
        { src, sizes: "128x128", type: imageMimeType(src) },
    ];
};

const imageMimeType = (src: string) => {
    const pathname = new URL(src, window.location.href).pathname.toLowerCase();
    if (pathname.endsWith(".png")) return "image/png";
    if (pathname.endsWith(".webp")) return "image/webp";
    return "image/jpeg";
};

const hasTrackMetadata = () => {
    return trackStore.isPlay && trackStore.trackName.trim().length > 0;
};

const currentTimeMs = () => {
    return correctedPlaybackTimeMs({
        netEaseID: trackStore.netEaseID,
        elapsedMs: trackStore.elapsedMs,
        durationMs: trackStore.durationMs,
        updatedAt: trackStore.updatedAt,
        isPlay: trackStore.isPlay,
    });
};

const getMediaSession = () => {
    if (!("mediaSession" in navigator)) return null;
    return navigator.mediaSession;
};

const setActionHandler = (
    mediaSession: MediaSession,
    action: MediaSessionAction,
    handler: MediaSessionActionHandler | null,
) => {
    try {
        mediaSession.setActionHandler(action, handler);
    } catch {
        // Browsers expose different subsets of the Media Session action list.
    }
};
