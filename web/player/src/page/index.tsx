import { onMount, onCleanup } from "solid-js";
import { CurrentTrack } from "./CurrentTrack";
import { ListenersCounter } from "./ListenersCounter";
import { RadioButton } from "./RadioButton";
import { closeEventSource, initEventSource } from "../store/events";
import styles from "./Page.module.css";
import { StationInformation } from "./StationInformation";
import { LyricsStage } from "./LyricsStage";
import { MediaSession } from "./MediaSession";

export const Page = () => {
    onMount(() => {
        initEventSource();
    });

    onCleanup(() => {
        closeEventSource();
    });

    return (
        <div class={styles.page}>
            <MediaSession />
            <div class={styles.header}>
                <ListenersCounter />
                <StationInformation />
            </div>
            <div class={styles.content}>
                <RadioButton />
                <LyricsStage />
            </div>
            <CurrentTrack />
        </div>
    );
};
