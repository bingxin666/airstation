import { Accessor, Component, createSignal, onMount } from "solid-js";
import pageStyles from "./Page.module.css";
import styles from "./StationInformation.module.css";
import { DESKTOP_WIDTH } from "../const";
import { addEventListener, EVENTS } from "../store/events";
import { loadStationInfo, refreshStationInfo, stationStore } from "../store/station";

export const StationInformation = () => {
    const [isOpen, setIsOpen] = createSignal(false);
    const open = () => setIsOpen(true);
    const close = () => setIsOpen(false);

    return (
        <>
            <div role="button" class={`${isOpen() ? "empty_icon" : styles.info_icon}`} onClick={open} />
            <Card isOpen={isOpen} close={close} />
        </>
    );
};

const parseLinks = (rawLinks: string): { title: string; url: string }[] => {
    const regex = /\[([^\]]+)]\((https?:\/\/[^\s)]+)\)/g;
    return Array.from(rawLinks.matchAll(regex), (m) => ({
        title: m[1],
        url: m[2],
    }));
};

const Card: Component<{ isOpen: Accessor<boolean>; close: () => void }> = ({ isOpen, close }) => {
    onMount(() => {
        loadStationInfo().catch((error) => console.log(error));

        addEventListener(EVENTS.changeTheme, (_e: MessageEvent<string>) => {
            refreshStationInfo().catch((error) => console.log(error));
        });
    });

    return (
        <div
            class={`${styles.info_menu} ${isOpen() ? styles.info_open : ""} ${
                window.screen.width > DESKTOP_WIDTH ? pageStyles.menu_desktop : pageStyles.menu_mobile
            }`}
        >
            <div class={styles.header}>
                <div role="button" class={pageStyles.close_icon} onClick={close}></div>
            </div>

            {stationStore.info?.logoURL && (
                <img src={stationStore.info.logoURL} alt={stationStore.info.name} class={styles.logo} />
            )}

            <div class={styles.content}>
                <div class={styles.title}>{stationStore.info?.name}</div>

                <div class={styles.metadata}>
                    {stationStore.info?.location && <span class={styles.location}>{stationStore.info.location}</span>}
                    {stationStore.info?.timezone && <span class={styles.timezone}>{stationStore.info.timezone}</span>}
                </div>

                <div class={styles.description} innerHTML={stationStore.info?.description} />

                {stationStore.info?.links && (
                    <div class={styles.footer}>
                        {parseLinks(stationStore.info.links).map((link) => (
                            <a href={link.url} target="_blank" rel="noreferrer">
                                {link.title}
                            </a>
                        ))}
                    </div>
                )}
            </div>
        </div>
    );
};
