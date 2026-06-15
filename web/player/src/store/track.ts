import { createStore } from "solid-js/store";
import { PlaybackLyrics } from "../api/types";

export const [trackStore, setTrackStore] = createStore({
    trackName: "",
    trackID: "",
    netEaseID: 0,
    elapsedMs: 0,
    updatedAt: 0,
    lyrics: null as PlaybackLyrics | null,
    isPlay: false,
});
