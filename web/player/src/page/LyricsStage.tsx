import { createEffect, createMemo, onCleanup, Show, untrack } from "solid-js";
import { LayoutAlignAnchor, LyricPlayer, type LyricLine } from "@applemusic-like-lyrics/core";
import "@applemusic-like-lyrics/core/style.css";
import { parseLrc, parseYrc } from "@applemusic-like-lyrics/lyric";
import { correctedPlaybackTimeMs } from "../store/playbackClock";
import { trackStore } from "../store/track";
import { withTranslatedLyrics } from "./lyricsTiming";
import styles from "./LyricsStage.module.css";

type ParsedLyrics = {
    mode: "word" | "line" | "text" | "none";
    lines: LyricLine[];
    text: string;
};

export const LyricsStage = () => {
    let hostRef: HTMLDivElement | undefined;
    let player: LyricPlayer | undefined;
    let frame = 0;
    let lastFrameAt = performance.now();

    const parsed = createMemo<ParsedLyrics>(() => {
        const lyrics = trackStore.lyrics;
        if (!lyrics || lyrics.kind === "none") return { mode: "none", lines: [], text: "" };

        if (lyrics.yrc) {
            try {
                const lines = withTranslatedLyrics(parseYrc(lyrics.yrc), lyrics.translation);
                if (lines.length > 0) return { mode: "word", lines, text: lyrics.text };
            } catch (error) {
                console.log("Failed to parse YRC lyrics:", error);
            }
        }

        if (lyrics.lrc) {
            try {
                const lines = withTranslatedLyrics(parseLrc(lyrics.lrc), lyrics.translation);
                if (lines.length > 0) return { mode: "line", lines, text: lyrics.text };
            } catch (error) {
                console.log("Failed to parse LRC lyrics:", error);
            }
        }

        if (lyrics.text) return { mode: "text", lines: [], text: lyrics.text };
        return { mode: "none", lines: [], text: "" };
    });

    const ensurePlayer = () => {
        if (!hostRef || player || parsed().mode === "text" || parsed().mode === "none") return;

        player = new LyricPlayer();
        player.setOptimizeOptions({ tryAdvanceStartTime: false });
        player.setEnableBlur(true);
        player.setEnableScale(true);
        player.setAlignAnchor(LayoutAlignAnchor.Center);
        player.setAlignPosition(0.5);
        player.setWordFadeWidth(parsed().mode === "word" ? 0.55 : 0.0001);
        hostRef.appendChild(player.getElement());
    };

    createEffect(() => {
        const current = parsed();
        if (current.mode === "text" || current.mode === "none") {
            player?.dispose();
            player = undefined;
            if (hostRef) hostRef.replaceChildren();
            return;
        }

        ensurePlayer();
        player?.setWordFadeWidth(current.mode === "word" ? 0.55 : 0.0001);
        player?.setLyricLines(current.lines, untrack(currentTimeMs));
        player?.calcLayout(true, true);
    });

    const tick = () => {
        const now = performance.now();
        const delta = now - lastFrameAt;
        lastFrameAt = now;

        if (player) {
            player.setCurrentTime(currentTimeMs());
            trackStore.isPlay ? player.resume() : player.pause();
            player.update(delta);
        }

        frame = requestAnimationFrame(tick);
    };

    frame = requestAnimationFrame(tick);

    onCleanup(() => {
        cancelAnimationFrame(frame);
        player?.dispose();
    });

    return (
        <div class={styles.stage}>
            <Show
                when={parsed().mode !== "none"}
                fallback={<div class={styles.empty}>No lyrics available</div>}
            >
                <Show
                    when={parsed().mode !== "text"}
                    fallback={<div class={styles.textFallback}>{parsed().text}</div>}
                >
                    <div ref={(el) => (hostRef = el)} class={styles.amllHost}></div>
                </Show>
            </Show>
        </div>
    );
};

const currentTimeMs = () => {
    return correctedPlaybackTimeMs({
        netEaseID: trackStore.netEaseID,
        elapsedMs: trackStore.elapsedMs,
        durationMs: trackStore.durationMs,
        updatedAt: trackStore.updatedAt,
        isPlay: trackStore.isPlay,
    });
};
