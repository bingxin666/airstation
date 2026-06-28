import type { LyricLine, LyricWord } from "@applemusic-like-lyrics/core";
import { parseLrc, type LyricLine as ParsedLyricLine } from "@applemusic-like-lyrics/lyric";

const TRANSLATION_SYNC_TOLERANCE_MS = 1200;
const TRANSLATION_WINDOW_PADDING_MS = 250;
const MIN_VISIBLE_WORD_DURATION_MS = 90;

type TranslationLine = {
    startTime: number;
    text: string;
};

export const withTranslatedLyrics = (lines: ParsedLyricLine[], translation: string): LyricLine[] => {
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
            const rawWords = line.words.length
                ? line.words.map((word) => ({
                      word: word.word,
                      startTime: finiteTimestamp(word.startTime) ?? startTime,
                      endTime: word.endTime > word.startTime ? word.endTime : endTime,
                      romanWord: word.romanWord,
                  }))
                : [{ word: "", startTime, endTime }];
            const words = sanitizeTimedWords(rawWords, endTime);
            const lineStartTime = finiteTimestamp(words[0]?.startTime) ?? startTime;
            const lineEndTime =
                finiteTimestamp(words[words.length - 1]?.endTime) ?? Math.max(endTime, lineStartTime + 1);

            return {
                words,
                translatedLyric: line.translatedLyric || "",
                romanLyric: line.romanLyric || "",
                startTime: lineStartTime,
                endTime: Math.max(lineEndTime, lineStartTime + 1),
                isBG: line.isBG || false,
                isDuet: line.isDuet || false,
            };
        })
        .filter((line) => line.words.some((word) => word.word.trim() !== ""));
};

const sanitizeTimedWords = (words: LyricWord[], lineEndTime: number): LyricWord[] => {
    const normalized = words.map(normalizedWord).filter((word) => word.word.length > 0);
    const withoutCollisions = resolveTimedWordCollisions(normalized);
    const withoutZeroDuration = mergeZeroDurationWords(withoutCollisions, lineEndTime);
    const withoutUnreadableDurations = mergeTooShortWords(withoutZeroDuration, lineEndTime);
    const stable = resolveTimedWordCollisions(withoutUnreadableDurations);

    return stable.filter((word) => word.endTime > word.startTime);
};

const normalizedWord = (word: LyricWord): LyricWord => {
    const startTime = finiteTimestamp(word.startTime) ?? 0;
    const endTime = Math.max(finiteTimestamp(word.endTime) ?? startTime, startTime);

    return {
        ...word,
        startTime,
        endTime,
    };
};

const resolveTimedWordCollisions = (words: LyricWord[]): LyricWord[] => {
    const result: LyricWord[] = [];

    words.forEach((word) => {
        const current = { ...word };
        const previous = result[result.length - 1];
        if (!previous) {
            result.push(current);
            return;
        }

        if (current.startTime >= previous.endTime) {
            result.push(current);
            return;
        }

        const clippedPreviousEndTime = Math.max(previous.startTime, current.startTime);
        if (clippedPreviousEndTime - previous.startTime >= MIN_VISIBLE_WORD_DURATION_MS) {
            previous.endTime = clippedPreviousEndTime;
            result.push(current);
            return;
        }

        appendWordText(result, current);
    });

    return result;
};

const mergeZeroDurationWords = (words: LyricWord[], lineEndTime: number): LyricWord[] => {
    const result: LyricWord[] = [];

    words.forEach((word, index) => {
        if (word.endTime > word.startTime) {
            result.push(word);
            return;
        }

        if (result.length > 0) {
            appendWordText(result, word);
            return;
        }

        const next = words[index + 1];
        if (next) {
            next.word = word.word + next.word;
            next.startTime = Math.min(word.startTime, next.startTime);
            return;
        }

        result.push(expandWord(word, lineEndTime));
    });

    return result;
};

const mergeTooShortWords = (words: LyricWord[], lineEndTime: number): LyricWord[] => {
    const result: LyricWord[] = [];

    for (let index = 0; index < words.length; index++) {
        const word = words[index];
        const duration = word.endTime - word.startTime;
        if (duration >= MIN_VISIBLE_WORD_DURATION_MS) {
            result.push(word);
            continue;
        }

        const next = words[index + 1];
        if (next) {
            next.word = word.word + next.word;
            next.startTime = Math.min(word.startTime, next.startTime);
            continue;
        }

        if (result.length > 0) {
            appendWordText(result, word);
            continue;
        }

        result.push(expandWord(word, lineEndTime));
    }

    return result;
};

const appendWordText = (words: LyricWord[], word: LyricWord) => {
    const previous = words[words.length - 1];
    words[words.length - 1] = {
        ...previous,
        word: previous.word + word.word,
        endTime: Math.max(previous.endTime, word.endTime),
    };
};

const expandWord = (word: LyricWord, lineEndTime: number): LyricWord => {
    const targetEndTime = word.startTime + MIN_VISIBLE_WORD_DURATION_MS;
    const endTime =
        Number.isFinite(lineEndTime) && lineEndTime > word.startTime
            ? Math.min(targetEndTime, lineEndTime)
            : targetEndTime;

    return {
        ...word,
        endTime: Math.max(endTime, word.startTime + 1),
    };
};

const finiteTimestamp = (value: number | undefined) => {
    return Number.isFinite(value) ? value : undefined;
};
