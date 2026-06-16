import { createEffect, createMemo, onCleanup, Show, untrack } from "solid-js";
import { LayoutAlignAnchor, LyricPlayer, type LyricLine } from "@applemusic-like-lyrics/core";
import "@applemusic-like-lyrics/core/style.css";
import { parseLrc, parseYrc, type LyricLine as ParsedLyricLine } from "@applemusic-like-lyrics/lyric";
import { correctedPlaybackTimeMs } from "../store/playbackClock";
import { trackStore } from "../store/track";
import styles from "./LyricsStage.module.css";

const TRANSLATION_SYNC_TOLERANCE_MS = 1200;
const TRANSLATION_WINDOW_PADDING_MS = 250;

type ParsedLyrics = {
    mode: "word" | "line" | "text" | "none";
    lines: LyricLine[];
    text: string;
};

type TranslationLine = {
    startTime: number;
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

const withTranslatedLyrics = (lines: ParsedLyricLine[], translation: string): LyricLine[] => {
    const translations = parseTranslationLines(translation);
    return applyTranslatedLyrics(toCoreLines(lines), translations);
};

const applyTranslatedLyrics = (lines: LyricLine[], translations: TranslationLine[]): LyricLine[] => {
    if (translations.length === 0) return lines;

    return lines.map((line, index) => ({
        ...line,
        translatedLyric: findTranslation(line, index, lines, translations) || line.translatedLyric,
    }));
};

const parseTranslationLines = (translation: string) => {
    if (!translation.trim()) return [];

    try {
        return parseLrc(translation)
            .map((line) => ({
                startTime: line.startTime,
                text: line.words.map((word) => word.word).join("").trim(),
            }))
            .filter((line) => line.text.length > 0)
            .sort((a, b) => a.startTime - b.startTime);
    } catch (error) {
        console.log("Failed to parse translated lyrics:", error);
        return [];
    }
};

const findTranslation = (
    line: LyricLine,
    index: number,
    lines: LyricLine[],
    translations: TranslationLine[],
) => {
    const previous = lines[index - 1];
    const next = lines[index + 1];
    const windowStart =
        (previous ? midpoint(previous.startTime, line.startTime) : line.startTime - TRANSLATION_SYNC_TOLERANCE_MS) -
        TRANSLATION_WINDOW_PADDING_MS;
    const windowEnd =
        (next ? midpoint(line.startTime, next.startTime) : line.endTime + TRANSLATION_SYNC_TOLERANCE_MS) +
        TRANSLATION_WINDOW_PADDING_MS;

    const windowed = bestTranslationIndex(
        translations,
        line.startTime,
        (translation) => translation.startTime >= windowStart && translation.startTime < windowEnd,
    );
    if (windowed >= 0) {
        return translations[windowed].text;
    }

    const nearby = bestTranslationIndex(
        translations,
        line.startTime,
        (translation) => Math.abs(translation.startTime - line.startTime) <= TRANSLATION_SYNC_TOLERANCE_MS,
    );
    if (nearby >= 0) {
        return translations[nearby].text;
    }

    return "";
};

const bestTranslationIndex = (
    translations: TranslationLine[],
    startTime: number,
    predicate: (translation: TranslationLine) => boolean,
) => {
    let bestIndex = -1;
    let bestDistance = Number.POSITIVE_INFINITY;

    translations.forEach((translation, index) => {
        if (!predicate(translation)) return;

        const distance = Math.abs(translation.startTime - startTime);
        if (distance < bestDistance) {
            bestIndex = index;
            bestDistance = distance;
        }
    });

    return bestIndex;
};

const midpoint = (a: number, b: number) => {
    return a + (b - a) / 2;
};

const toCoreLines = (lines: ParsedLyricLine[]): LyricLine[] => {
    return lines
        .map((line, index) => {
            const nextLine = lines[index + 1];
            const startTime = finiteTimestamp(line.startTime) ?? finiteTimestamp(line.words[0]?.startTime) ?? 0;
            const inferredEndTime =
                finiteTimestamp(nextLine?.startTime) ??
                finiteTimestamp(line.words[line.words.length - 1]?.endTime) ??
                startTime + 4000;
            const endTime = Number.isFinite(line.endTime) && line.endTime > startTime ? line.endTime : inferredEndTime;
            const words = line.words.length
                ? line.words.map((word) => ({
                      word: word.word,
                      startTime: finiteTimestamp(word.startTime) ?? startTime,
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

const finiteTimestamp = (value: number | undefined) => {
    return Number.isFinite(value) ? value : undefined;
};
