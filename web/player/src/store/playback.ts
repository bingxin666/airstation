import { airstationAPI } from "../api";
import { setTrackStore, trackStore } from "./track";

export const syncPlaybackTrack = async () => {
    const playback = await airstationAPI.getPlayback();
    if (!playback.isPlaying || !playback.currentTrack) {
        setTrackStore({
            trackName: "",
            trackArtist: "",
            trackID: "",
            netEaseID: 0,
            elapsedMs: 0,
            updatedAt: Date.now(),
            lyrics: null,
        });
        return null;
    }

    setTrackStore({
        trackName: playback.currentTrack.name,
        trackArtist: playback.currentTrack.artist || "",
        trackID: playback.currentTrack.id,
        netEaseID: playback.currentNetEaseID,
        elapsedMs: playback.currentTrackElapsed * 1000,
        updatedAt: Date.now(),
        lyrics: null,
    });

    const requestedNetEaseID = playback.currentNetEaseID;
    const lyrics = await airstationAPI.getPlaybackLyrics();
    if (trackStore.netEaseID === requestedNetEaseID && lyrics.songID === requestedNetEaseID) {
        setTrackStore("lyrics", lyrics);
    }
    return playback;
};
