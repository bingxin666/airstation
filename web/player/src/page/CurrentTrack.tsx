import { onMount, Show } from "solid-js";
import styles from "./CurrentTrack.module.css";
import { addEventListener, EVENTS } from "../store/events";
import { setTrackStore, trackStore } from "../store/track";
import { syncPlaybackTrack } from "../store/playback";

export const CurrentTrack = () => {
    onMount(async () => {
        try {
            await syncPlaybackTrack();
        } catch (error) {
            console.log(error);
        }

        addEventListener(EVENTS.newTrack, async (e: MessageEvent<string>) => {
            setTrackStore("lyrics", null);
            try {
                await syncPlaybackTrack();
            } catch (error) {
                console.log(error);
                setTrackStore({
                    trackName: e.data,
                    trackArtist: "",
                    trackID: "",
                    netEaseID: 0,
                    elapsedMs: 0,
                    updatedAt: Date.now(),
                    lyrics: null,
                });
            }
        });
    });

    const copyToClipboard = async () => {
        try {
            await navigator.clipboard.writeText(trackStore.trackName);
        } catch (error) {
            console.log(error);
        }
    };

    const netEaseURL = () => {
        if (!trackStore.netEaseID) return "";
        return `https://music.163.com/#/song?id=${trackStore.netEaseID}`;
    };

    const TrackLabel = () => (
        <>
            <span>{trackStore.trackName}</span>
            <Show when={trackStore.trackArtist.length > 0}>
                <span class={styles.artist}>{trackStore.trackArtist}</span>
            </Show>
        </>
    );

    return (
        <div class={styles.box}>
            <Show when={trackStore.trackName.length > 0} fallback={<OfflineLabel />}>
                <Show
                    when={netEaseURL()}
                    fallback={
                        <button type="button" onClick={copyToClipboard} class={styles.label}>
                            <TrackLabel />
                        </button>
                    }
                >
                    {(url) => (
                        <a href={url()} target="_blank" rel="noreferrer" class={styles.label}>
                            <TrackLabel />
                        </a>
                    )}
                </Show>
            </Show>
        </div>
    );
};

const OfflineLabel = () => {
    return (
        <div class={styles.offline_label}>
            <div class={styles.offline_label_icon}></div>
            <div class={styles.offline_label_title}>Stream offline</div>
        </div>
    );
};
