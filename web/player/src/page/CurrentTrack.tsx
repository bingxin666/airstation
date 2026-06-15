import { onMount, Show } from "solid-js";
import { airstationAPI } from "../api";
import styles from "./CurrentTrack.module.css";
import { addEventListener, EVENTS } from "../store/events";
import { setTrackStore, trackStore } from "../store/track";
import { addHistory } from "../store/history";
import { getUnixTime } from "../utils/date";

export const CurrentTrack = () => {
    onMount(async () => {
        try {
            const cs = await airstationAPI.getPlayback();
            if (cs.isPlaying && cs.currentTrack) {
                setTrackStore({
                    trackName: cs.currentTrack.name,
                    trackID: cs.currentTrack.id,
                    netEaseID: cs.currentNetEaseID,
                    elapsedMs: cs.currentTrackElapsed * 1000,
                    updatedAt: Date.now(),
                });
                const lyrics = await airstationAPI.getPlaybackLyrics();
                setTrackStore("lyrics", lyrics);
            }
        } catch (error) {
            console.log(error);
        }

        addEventListener(EVENTS.newTrack, async (e: MessageEvent<string>) => {
            const unixTime = getUnixTime();
            setTrackStore("trackName", e.data);
            addHistory({ id: unixTime, playedAt: unixTime, trackName: e.data });
            try {
                const cs = await airstationAPI.getPlayback();
                setTrackStore({
                    trackName: cs.currentTrack?.name || e.data,
                    trackID: cs.currentTrack?.id || "",
                    netEaseID: cs.currentNetEaseID,
                    elapsedMs: cs.currentTrackElapsed * 1000,
                    updatedAt: Date.now(),
                });
                const lyrics = await airstationAPI.getPlaybackLyrics();
                setTrackStore("lyrics", lyrics);
            } catch (error) {
                console.log(error);
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

    return (
        <div class={styles.box}>
            <Show when={trackStore.trackName.length > 0} fallback={<OfflineLabel />}>
                <div onClick={copyToClipboard} class={styles.label}>
                    {trackStore.trackName}
                </div>
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
