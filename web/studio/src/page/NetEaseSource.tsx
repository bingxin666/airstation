import {
    Badge,
    Button,
    Checkbox,
    Flex,
    Group,
    PasswordInput,
    Paper,
    Select,
    SimpleGrid,
    Text,
    TextInput,
} from "@mantine/core";
import { useForm } from "@mantine/form";
import { useEffect, useState } from "react";
import { airstationAPI } from "../api";
import { NetEaseConfig, NetEasePublicConfig, NetEaseQuality } from "../api/types";
import { errNotify, okNotify } from "../notifications";
import styles from "./styles.module.css";

const qualityOptions: { value: NetEaseQuality; label: string }[] = [
    { value: "standard", label: "Standard 128 kbps" },
    { value: "higher", label: "Higher 192 kbps" },
    { value: "exhigh", label: "Ex-high 320 kbps" },
    { value: "lossless", label: "Lossless" },
    { value: "hires", label: "Hi-Res" },
];

export const NetEaseSource = () => {
    const [loading, setLoading] = useState(false);
    const [syncing, setSyncing] = useState(false);
    const [config, setConfig] = useState<NetEasePublicConfig | null>(null);
    const form = useForm<NetEaseConfig>({
        initialValues: {
            playlistURL: "",
            quality: "standard",
            cookie: "",
            clearCookie: false,
        },
    });

    const loadConfig = async () => {
        setLoading(true);
        try {
            const next = await airstationAPI.getNetEaseConfig();
            setConfig(next);
            form.setValues({
                playlistURL: next.playlistURL,
                quality: next.quality,
                cookie: "",
                clearCookie: false,
            });
        } catch (error) {
            errNotify(error);
        } finally {
            setLoading(false);
        }
    };

    const saveConfig = async () => {
        setLoading(true);
        try {
            const next = await airstationAPI.editNetEaseConfig(form.values);
            setConfig(next);
            form.setFieldValue("cookie", "");
            form.setFieldValue("clearCookie", false);
            okNotify("NetEase source saved");
        } catch (error) {
            errNotify(error);
        } finally {
            setLoading(false);
        }
    };

    const syncPlaylist = async () => {
        setSyncing(true);
        try {
            const next = await airstationAPI.syncNetEasePlaylist();
            setConfig(next);
            okNotify("Playlist synced");
        } catch (error) {
            errNotify(error);
        } finally {
            setSyncing(false);
        }
    };

    useEffect(() => {
        loadConfig();
    }, []);

    const lastSync = config?.lastSyncedAt
        ? new Date(config.lastSyncedAt * 1000).toLocaleString()
        : "Not synced";

    return (
        <Paper w="100%" radius="md" className={styles.transparent_paper}>
            <Flex p="md" direction="column" gap="md">
                <Group justify="space-between" align="flex-start">
                    <div>
                        <Text fw={600}>NetEase Cloud source</Text>
                        <Text size="sm" c="dimmed">
                            The backend randomly pulls playable songs from this playlist and publishes one HLS stream.
                        </Text>
                    </div>
                    <Badge color={config?.trackCount ? "green" : "gray"} variant="light">
                        {config?.trackCount || 0} tracks
                    </Badge>
                </Group>

                <TextInput
                    label="Playlist link"
                    placeholder="https://music.163.com/#/playlist?id=3778678"
                    disabled={loading}
                    key={form.key("playlistURL")}
                    {...form.getInputProps("playlistURL")}
                />

                <SimpleGrid cols={{ base: 1, sm: 2 }} spacing="sm">
                    <Select
                        label="Quality"
                        data={qualityOptions}
                        disabled={loading}
                        allowDeselect={false}
                        key={form.key("quality")}
                        {...form.getInputProps("quality")}
                    />
                    <PasswordInput
                        label="Login cookie"
                        placeholder={config?.hasCookie ? "Saved. Leave empty to keep it." : "Paste MUSIC_U cookie"}
                        description={config?.accountName ? `Logged in as ${config.accountName}` : "Used for VIP or private playlists."}
                        disabled={loading}
                        key={form.key("cookie")}
                        {...form.getInputProps("cookie")}
                    />
                </SimpleGrid>

                <Checkbox
                    label="Clear saved login cookie"
                    disabled={loading || !config?.hasCookie}
                    key={form.key("clearCookie")}
                    {...form.getInputProps("clearCookie", { type: "checkbox" })}
                />

                {config?.lastError ? (
                    <Text size="sm" c="red">
                        {config.lastError}
                    </Text>
                ) : null}

                <Group justify="space-between">
                    <Text size="sm" c="dimmed">
                        Last sync: {lastSync}
                    </Text>
                    <Group>
                        <Button variant="light" loading={syncing} onClick={syncPlaylist} disabled={!config?.playlistURL}>
                            Sync
                        </Button>
                        <Button loading={loading} onClick={saveConfig}>
                            Save
                        </Button>
                    </Group>
                </Group>
            </Flex>
        </Paper>
    );
};
