import { createEffect, createMemo, onCleanup, Show } from "solid-js";
import { LayoutAlignAnchor, LyricLine, LyricPlayer } from "@applemusic-like-lyrics/core";
import "@applemusic-like-lyrics/core/style.css";
import { parseLrc, parseYrc } from "@applemusic-like-lyrics/lyric";
import { trackStore } from "../store/track";
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
                const lines = toCoreLines(parseYrc(lyrics.yrc));
                if (lines.length > 0) return { mode: "word", lines, text: lyrics.text };
            } catch (error) {
                console.log("Failed to parse YRC lyrics:", error);
            }
        }

        if (lyrics.lrc) {
            try {
                const lines = toCoreLines(parseLrc(lyrics.lrc));
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
        player?.setLyricLines(current.lines, currentTimeMs());
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
    if (!trackStore.isPlay) return trackStore.elapsedMs;
    return trackStore.elapsedMs + Math.max(0, Date.now() - trackStore.updatedAt);
};

const toCoreLines = (lines: import("@applemusic-like-lyrics/lyric").LyricLine[]): LyricLine[] => {
    return lines
        .map((line, index) => {
            const nextLine = lines[index + 1];
            const startTime = line.startTime || line.words[0]?.startTime || 0;
            const inferredEndTime = nextLine?.startTime || line.words[line.words.length - 1]?.endTime || startTime + 4000;
            const endTime = Number.isFinite(line.endTime) && line.endTime > startTime ? line.endTime : inferredEndTime;
            const words = line.words.length
                ? line.words.map((word) => ({
                      word: word.word,
                      startTime: word.startTime || startTime,
                      endTime: word.endTime > word.startTime ? word.endTime : endTime,
                      romanWord: word.romanWord,
                  }))
                : [{ word: "", startTime, endTime }];

            return {
                words,
                translatedLyric: line.translatedLyric || "",
                romanLyric: line.romanLyric || "",
                startTime,
                endTime,
                isBG: line.isBG || false,
                isDuet: line.isDuet || false,
            };
        })
        .filter((line) => line.words.some((word) => word.word.trim() !== ""));
};
