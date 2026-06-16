import { createStore } from "solid-js/store";
import { airstationAPI } from "../api";
import type { StationInfo } from "../api/types";
import { isValidHexColor } from "../utils/color";
import { setCssVariable, setFavicon, setPageTitle } from "../utils/document";
import { isValidURL } from "../utils/url";

type StationStore = {
    info: StationInfo | null;
};

export const [stationStore, setStationStore] = createStore<StationStore>({
    info: null,
});

let stationInfoRequest: Promise<StationInfo> | null = null;

export const loadStationInfo = async () => {
    if (!stationInfoRequest) {
        stationInfoRequest = airstationAPI
            .getStationInfo()
            .then((info) => {
                setStationStore("info", info);
                applyStationInfo(info);
                return info;
            })
            .catch((error) => {
                stationInfoRequest = null;
                throw error;
            });
    }

    return stationInfoRequest;
};

export const refreshStationInfo = async () => {
    stationInfoRequest = null;
    return loadStationInfo();
};

const applyStationInfo = (info: StationInfo) => {
    if (info.name) setPageTitle(info.name);
    if (isValidURL(info.faviconURL)) setFavicon(info.faviconURL);
    if (info.theme) applyTheme(info.theme);
};

const applyTheme = (rawTheme: string) => {
    const [bgStart, bgEnd, bgIcon, text, accent, bgImage] = rawTheme.split(";");

    if (bgStart && isValidHexColor(bgStart)) setCssVariable("--bg-gradient-start", bgStart);
    if (bgEnd && isValidHexColor(bgEnd)) setCssVariable("--bg-gradient-end", bgEnd);
    if (bgIcon && isValidHexColor(bgIcon)) setCssVariable("--bg-icon", bgIcon);
    if (text && isValidHexColor(text)) setCssVariable("--text-color", text);

    if (accent && isValidHexColor(accent)) {
        setCssVariable("--accent-color", accent);
    } else {
        setCssVariable("--accent-color", "");
    }

    if (bgImage && isValidURL(bgImage)) {
        document.body.style.backgroundImage = `url(${bgImage})`;
    } else {
        document.body.style.backgroundImage = "";
    }
};
