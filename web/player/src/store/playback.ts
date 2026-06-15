import { airstationAPI } from "../api";
import { resetPlaybackAudioClock } from "./playbackClock";
import { setTrackStore, trackStore } from "./track";

export const syncPlaybackTrack = async () => {
    const playback = await airstationAPI.getPlayback();
    if (!playback.isPlaying || !playback.currentTrack) {
        resetPlaybackAudioClock();
        setTrackStore({
            trackName: "",
            trackArtist: "",
            trackID: "",
            netEaseID: 0,
            elapsedMs: 0,
            durationMs: 0,
            updatedAt: Date.now(),
            lyrics: null,
        });
        return null;
    }

    const requestedNetEaseID = playback.currentNetEaseID;
    const preservedLyrics =
        trackStore.netEaseID === requestedNetEaseID && trackStore.lyrics?.songID === requestedNetEaseID
            ? trackStore.lyrics
            : null;
    if (trackStore.netEaseID !== requestedNetEaseID) {
        resetPlaybackAudioClock();
    }

    setTrackStore({
        trackName: playback.currentTrack.name,
        trackArtist: playback.currentTrack.artist || "",
        trackID: playback.currentTrack.id,
        netEaseID: requestedNetEaseID,
        elapsedMs: playback.currentTrackElapsed * 1000,
        durationMs: playback.currentTrack.duration * 1000,
        updatedAt: Date.now(),
        lyrics: preservedLyrics,
    });

    if (!requestedNetEaseID || preservedLyrics) {
        return playback;
    }

    const lyrics = await airstationAPI.getPlaybackLyrics();
    if (trackStore.netEaseID === requestedNetEaseID && lyrics.songID === requestedNetEaseID) {
        setTrackStore("lyrics", lyrics);
    }
    return playback;
};
