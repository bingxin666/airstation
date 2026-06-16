import { createEffect, onCleanup, onMount } from "solid-js";
import { correctedPlaybackTimeMs } from "../store/playbackClock";
import { loadStationInfo, stationStore } from "../store/station";
import { trackStore } from "../store/track";
import { isValidURL } from "../utils/url";

const DEFAULT_STATION_TITLE = "Airstation";

export const MediaSession = () => {
    onMount(() => {
        loadStationInfo().catch((error) => console.log(error));
    });

    createEffect(() => {
        updateMediaSessionMetadata();
    });

    createEffect(() => {
        updateMediaSessionPlaybackState();
    });

    createEffect(() => {
        updateMediaSessionPosition();
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

export const clearMediaSessionActionHandlers = () => {
    const mediaSession = getMediaSession();
    if (!mediaSession) return;

    setActionHandler(mediaSession, "play", null);
    setActionHandler(mediaSession, "pause", null);
    setActionHandler(mediaSession, "stop", null);
};

const updateMediaSessionMetadata = () => {
    const mediaSession = getMediaSession();
    if (!mediaSession || typeof MediaMetadata === "undefined") return;

    mediaSession.metadata = new MediaMetadata({
        title: metadataTitle(),
        artist: metadataArtist(),
        album: metadataAlbum(),
        artwork: metadataArtwork(),
    });
};

const updateMediaSessionPlaybackState = () => {
    const mediaSession = getMediaSession();
    if (!mediaSession) return;

    if (trackStore.isPlay) {
        mediaSession.playbackState = "playing";
        return;
    }

    mediaSession.playbackState = trackStore.trackName ? "paused" : "none";
};

const updateMediaSessionPosition = () => {
    const mediaSession = getMediaSession();
    if (!mediaSession || typeof mediaSession.setPositionState !== "function") return;
    if (!hasSongMetadata() || trackStore.durationMs <= 0) {
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
    if (hasSongMetadata()) return trackStore.trackName;
    return stationStore.info?.name || DEFAULT_STATION_TITLE;
};

const metadataArtist = () => {
    if (hasSongMetadata()) return trackStore.trackArtist || stationStore.info?.name || DEFAULT_STATION_TITLE;
    return stationStore.info?.location || "";
};

const metadataAlbum = () => {
    return stationStore.info?.name || DEFAULT_STATION_TITLE;
};

const metadataArtwork = (): MediaImage[] => {
    const artwork = [stationStore.info?.logoURL, stationStore.info?.faviconURL]
        .filter((url): url is string => Boolean(url && isValidURL(url)))
        .map((src) => ({ src }));

    return artwork;
};

const hasSongMetadata = () => {
    return trackStore.netEaseID > 0 && trackStore.trackName.trim().length > 0;
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
