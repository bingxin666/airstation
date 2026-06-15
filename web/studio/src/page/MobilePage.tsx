import { Flex } from "@mantine/core";
import { useState } from "react";
import { MobileBar } from "./MobileBar";
import { NetEaseSource } from "./NetEaseSource";
import { Playback } from "./Playback";

const MobilePage = () => {
    const [activeBar, setActiveBar] = useState("Playback");
    const isVisible = (bar: string) => (bar === activeBar ? "block" : "none");

    return (
        <Flex direction="column" h="100vh">
            <div style={{ flex: 1, display: isVisible("Playback") }}>
                <Playback isMobile />
            </div>
            <div style={{ flex: 1, display: isVisible("Source"), padding: 8, overflowY: "auto" }}>
                <NetEaseSource />
            </div>

            <MobileBar activeBar={activeBar} setActiveBar={setActiveBar} />
        </Flex>
    );
};

export default MobilePage;
