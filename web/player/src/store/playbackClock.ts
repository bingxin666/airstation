import type { Fragment } from "hls.js";

const STREAM_SEGMENT_MS = 5000;
const AUDIO_CLOCK_MAX_AGE_MS = 8000;

type AudioClock = {
    songID: number;
    elapsedMs: number;
    updatedAt: number;
    playbackRate: number;
};

type PlaybackClockInput = {
    netEaseID: number;
    elapsedMs: number;
    durationMs: number;
    updatedAt: number;
    isPlay: boolean;
};

let activeFragment: Fragment | null = null;
let audioClock: AudioClock | null = null;

export const resetPlaybackAudioClock = () => {
    activeFragment = null;
    audioClock = null;
};

export const setActivePlaybackFragment = (fragment: Fragment, media?: HTMLMediaElement) => {
    activeFragment = fragment;
    updatePlaybackAudioClock(media);
};

export const updatePlaybackAudioClock = (media?: HTMLMediaElement) => {
    if (!media || !activeFragment) return;

    const segment = segmentFromFragment(activeFragment);
    if (!segment) return;

    const fragmentOffsetMs = Math.max(0, media.currentTime - activeFragment.start) * 1000;
    audioClock = {
        songID: segment.songID,
        elapsedMs: segment.index * STREAM_SEGMENT_MS + fragmentOffsetMs,
        updatedAt: Date.now(),
        playbackRate: finitePositive(media.playbackRate) || 1,
    };
};

export const correctedPlaybackTimeMs = (input: PlaybackClockInput) => {
    const audioElapsedMs = currentAudioElapsedMs(input.netEaseID, input.isPlay);
    if (audioElapsedMs !== null) {
        return clampPlaybackTime(audioElapsedMs, input.durationMs);
    }

    const serverElapsedMs = input.elapsedMs + (input.isPlay ? Math.max(0, Date.now() - input.updatedAt) : 0);
    return clampPlaybackTime(serverElapsedMs, input.durationMs);
};

const currentAudioElapsedMs = (songID: number, isPlay: boolean) => {
    if (!audioClock || audioClock.songID !== songID) return null;

    const ageMs = Date.now() - audioClock.updatedAt;
    if (ageMs > AUDIO_CLOCK_MAX_AGE_MS) return null;

    if (!isPlay) return audioClock.elapsedMs;
    return audioClock.elapsedMs + Math.max(0, ageMs) * audioClock.playbackRate;
};

const segmentFromFragment = (fragment: Fragment) => {
    const rawURL = fragment.relurl || fragment.url;
    const match = rawURL.match(/(?:^|\/)netease-(\d+)-\d+-(\d+)\.(?:m4s|ts)(?:$|\?)/);
    if (!match) return null;

    const songID = Number(match[1]);
    const index = Number(match[2]);
    if (!Number.isSafeInteger(songID) || !Number.isSafeInteger(index) || songID <= 0 || index < 0) {
        return null;
    }

    return { songID, index };
};

const finitePositive = (value: number) => {
    return Number.isFinite(value) && value > 0 ? value : 0;
};

const clampPlaybackTime = (elapsedMs: number, durationMs: number) => {
    if (!Number.isFinite(elapsedMs)) return 0;

    const lowerBounded = Math.max(0, elapsedMs);
    if (!Number.isFinite(durationMs) || durationMs <= 0) return lowerBounded;
    return Math.min(lowerBounded, durationMs);
};
