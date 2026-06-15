import { onMount, Show } from "solid-js";
import styles from "./CurrentTrack.module.css";
import { addEventListener, EVENTS } from "../store/events";
import { setTrackStore, trackStore } from "../store/track";
import { addHistory } from "../store/history";
import { getUnixTime } from "../utils/date";
import { syncPlaybackTrack } from "../store/playback";

export const CurrentTrack = () => {
    onMount(async () => {
        try {
            await syncPlaybackTrack();
        } catch (error) {
            console.log(error);
        }

        addEventListener(EVENTS.newTrack, async (e: MessageEvent<string>) => {
            const unixTime = getUnixTime();
            addHistory({ id: unixTime, playedAt: unixTime, trackName: e.data });
            setTrackStore("lyrics", null);
            try {
                await syncPlaybackTrack();
            } catch (error) {
                console.log(error);
                setTrackStore({ trackName: e.data, trackArtist: "" });
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
                    <span>{trackStore.trackName}</span>
                    <Show when={trackStore.trackArtist.length > 0}>
                        <span class={styles.artist}>{trackStore.trackArtist}</span>
                    </Show>
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
