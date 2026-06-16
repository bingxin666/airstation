import HLS, { type FragChangedData, type HlsConfig } from "hls.js";
import styles from "./RadioButton.module.css";
import { setTrackStore, trackStore } from "../store/track";
import { Component, onCleanup, onMount } from "solid-js";
import { addEventListener, EVENTS } from "../store/events";
import { getCssVariable } from "../utils/document";
import { getHueFromHex } from "../utils/color";
import { syncPlaybackTrack } from "../store/playback";
import { resetPlaybackAudioClock, setActivePlaybackFragment, updatePlaybackAudioClock } from "../store/playbackClock";
import {
    clearMediaSessionActionHandlers,
    refreshMediaSessionPosition,
    refreshMediaSessionSoon,
    setMediaSessionActionHandlers,
} from "./MediaSession";

const STREAM_SOURCE = "/stream";
const PLAYBACK_SYNC_INTERVAL_MS = 15000;
const HLS_CONFIG = {
    lowLatencyMode: false,
    initialLiveManifestSize: 1,
    liveSyncDurationCount: 1,
    liveMaxLatencyDurationCount: 3,
    maxBufferLength: 30,
    maxMaxBufferLength: 60,
    backBufferLength: 15,
    startFragPrefetch: true,
    manifestLoadingMaxRetry: 6,
    levelLoadingMaxRetry: 8,
    fragLoadingMaxRetry: 10,
    manifestLoadingTimeOut: 30000,
    levelLoadingTimeOut: 30000,
    fragLoadingTimeOut: 45000,
} satisfies Partial<HlsConfig>;

export const RadioButton = () => {
    let videoRef: HTMLAudioElement | undefined;
    let hls: HLS | undefined;
    let syncTimer = 0;

    const initStream = () => {
        if (!videoRef || hls) return;

        if (HLS.isSupported()) {
            hls = new HLS(HLS_CONFIG);
            hls.on(HLS.Events.FRAG_CHANGED, (_event, data: FragChangedData) => {
                setActivePlaybackFragment(data.frag, videoRef);
            });
            hls.loadSource(STREAM_SOURCE);
            hls.attachMedia(videoRef as unknown as HTMLMediaElement);
            return;
        }

        if (videoRef.canPlayType("application/vnd.apple.mpegurl")) {
            videoRef.src = STREAM_SOURCE;
        }
    };

    const playStream = () => {
        initStream();
        videoRef?.play().catch((error) => console.log(error));
    };

    const startPlaybackSync = () => {
        if (syncTimer) return;

        syncTimer = window.setInterval(() => {
            syncPlaybackTrack()
                .then(() => refreshMediaSessionSoon())
                .catch((error) => console.log(error));
            updatePlaybackAudioClock(videoRef);
            refreshMediaSessionPosition();
        }, PLAYBACK_SYNC_INTERVAL_MS);
    };

    const stopPlaybackSync = () => {
        if (!syncTimer) return;

        window.clearInterval(syncTimer);
        syncTimer = 0;
    };

    const handlePlay = () => {
        initStream();
        setTrackStore("isPlay", true);
        startPlaybackSync();
        refreshMediaSessionSoon();

        if (!trackStore.netEaseID) {
            syncPlaybackTrack()
                .then(() => refreshMediaSessionSoon())
                .catch((error) => console.log(error));
        }
    };

    const handlePause = () => {
        setTrackStore("isPlay", false);
        stopPlaybackSync();
        hls?.destroy();
        hls = undefined;
        resetPlaybackAudioClock();
        refreshMediaSessionSoon();
    };

    onMount(() => {
        setMediaSessionActionHandlers(videoRef, playStream);

        addEventListener(EVENTS.pause, (_e: MessageEvent<string>) => {
            stopPlaybackSync();
            resetPlaybackAudioClock();
            setTrackStore({
                trackName: "",
                trackArtist: "",
                coverURL: "",
                trackID: "",
                netEaseID: 0,
                elapsedMs: 0,
                durationMs: 0,
                updatedAt: Date.now(),
                lyrics: null,
            });
            (() => videoRef?.pause())();
        });

        addEventListener(EVENTS.play, async (e: MessageEvent<string>) => {
            resetPlaybackAudioClock();
            setTrackStore("lyrics", null);
            try {
                await syncPlaybackTrack();
            } catch (error) {
                console.log(error);
                setTrackStore({
                    trackName: e.data,
                    trackArtist: "",
                    coverURL: "",
                    trackID: "",
                    netEaseID: 0,
                    elapsedMs: 0,
                    durationMs: 0,
                    updatedAt: Date.now(),
                    lyrics: null,
                });
            }

            if (trackStore.isPlay) (() => videoRef?.pause())();
            playStream();
            refreshMediaSessionSoon();
        });

        document.body.addEventListener("keydown", (event) => {
            if (event.key === " ") {
                event.preventDefault();
                trackStore.isPlay ? videoRef?.pause() : playStream();
            }
        });
    });

    onCleanup(() => {
        stopPlaybackSync();
        hls?.destroy();
        resetPlaybackAudioClock();
        clearMediaSessionActionHandlers();
    });

    return (
        <div class={styles.container}>
            <audio
                id="video"
                ref={videoRef}
                onPause={handlePause}
                onPlay={handlePlay}
                onPlaying={() => {
                    updatePlaybackAudioClock(videoRef);
                    refreshMediaSessionSoon();
                }}
                onTimeUpdate={() => {
                    updatePlaybackAudioClock(videoRef);
                    refreshMediaSessionPosition();
                }}
                onRateChange={() => {
                    updatePlaybackAudioClock(videoRef);
                    refreshMediaSessionSoon();
                }}
            ></audio>
            <div class={styles.box}>
                {trackStore.isPlay ? (
                    <AnimatedPauseButton pause={() => videoRef?.pause()} media={videoRef} />
                ) : (
                    <div class={styles.play_icon} tabIndex={0} role="button" onClick={playStream}></div>
                )}
            </div>
        </div>
    );
};

let audioSource: MediaElementAudioSourceNode | null = null;
let audioContext: AudioContext | null = null;

const AnimatedPauseButton: Component<{ pause: () => void; media?: HTMLAudioElement }> = (props) => {
    let pauseIconRef: HTMLDivElement | undefined;
    let analyser: AnalyserNode | null = null;
    let dataArray: Uint8Array | null = null;
    let animationId: number | null = null;
    let gainNode: GainNode | null = null;
    let accentHue: number | null = null;
    let currentHue = 0;
    let currentSaturation = 50;
    let currentLightness = 60;

    const loadAccentColor = () => {
        const accentColor = getCssVariable("--accent-color");
        accentHue = accentColor ? getHueFromHex(accentColor) : null;

        currentHue = accentHue !== null ? accentHue : 0;
        currentSaturation = accentHue !== null ? 100 : 50;
    };

    onMount(async () => {
        loadAccentColor();
        setInterval(loadAccentColor, 1000); // Need for hot reload

        if (!pauseIconRef || !props.media) return;
        await initAudio();
        draw();
    });

    onCleanup(async () => {
        if (animationId !== null) {
            cancelAnimationFrame(animationId);
            animationId = null;
        }

        if (gainNode) {
            gainNode.disconnect();
            gainNode = null;
        }

        if (analyser) {
            analyser.disconnect();
            analyser = null;
        }

        dataArray = null;

        if (pauseIconRef) {
            pauseIconRef.style.transform = "scale(1)";
            pauseIconRef.style.backgroundColor = "white";
            pauseIconRef.style.boxShadow = "none";
        }
    });

    const initAudio = async () => {
        try {
            if (!props.media) return;
            if (!audioContext) audioContext = new window.AudioContext();

            analyser = audioContext.createAnalyser();
            analyser.fftSize = 256;
            gainNode = audioContext.createGain();
            gainNode.gain.value = 1;

            if (!audioSource) audioSource = audioContext.createMediaElementSource(props.media);
            audioSource.connect(gainNode);
            gainNode.connect(analyser);
            analyser.connect(audioContext.destination);

            const bufferLength = analyser.frequencyBinCount;
            dataArray = new Uint8Array(bufferLength);
        } catch (err) {
            console.error("Error initializing audio:", err);
        }
    };

    const draw = () => {
        if (!pauseIconRef || !analyser || !dataArray) return;

        animationId = requestAnimationFrame(draw);
        analyser.getByteFrequencyData(dataArray as Uint8Array<ArrayBuffer>);

        let bass = 0;
        let treble = 0;
        const bassEnd = Math.floor(dataArray.length * 0.3);
        const trebleStart = Math.floor(dataArray.length * 0.6);

        for (let i = 0; i < dataArray.length; i++) {
            if (i < bassEnd) bass += dataArray[i];
            else if (i > trebleStart) treble += dataArray[i];
        }

        bass /= bassEnd;
        treble /= dataArray.length - trebleStart;

        const scale = 1 + bass / 300;
        const jump = (bass / 300) * 20;

        pauseIconRef.style.transform = `translateY(${-jump}px) scale(${scale})`;

        const bassImpact = bass / 255;
        const trebleImpact = treble / 255;

        if (accentHue == null) {
            currentHue += (Math.random() - 0.5) * bassImpact * 120;
            currentHue += trebleImpact * 2;
            currentHue = (currentHue + 360) % 360;
        }

        const color = `hsl(${currentHue}, ${currentSaturation}%, ${currentLightness}%)`;
        pauseIconRef.style.backgroundColor = color;

        const glowIntensity = bass / 2 + 20;
        pauseIconRef.style.boxShadow = `0 0 ${glowIntensity}px ${color}`;
    };

    return <div ref={pauseIconRef} tabIndex={0} role="button" class={styles.pause_icon} onClick={props.pause}></div>;
};
