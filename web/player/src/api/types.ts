export interface Track {
    id: string;
    name: string;
    artist: string;
    path: string;
    duration: number;
    bitRate: number;
}

export interface PlaybackState {
    currentTrack: Track | null;
    currentNetEaseID: number;
    currentTrackElapsed: number;
    isPlaying: boolean;
}

export interface PlaybackLyrics {
    songID: number;
    kind: "word" | "line" | "text" | "none";
    yrc: string;
    lrc: string;
    text: string;
}

export interface PlaybackHistory {
    id: number;
    playedAt: number;
    trackName: string;
}

export interface StationInfo {
    name: string;
    description: string;
    faviconURL: string;
    logoURL: string;
    location: string;
    timezone: string;
    links: string;
    theme: string;
}

export interface ResponseErr {
    message: string;
}

export interface ResponseOK {
    message: string;
}
