import { Container, Flex } from "@mantine/core";
import { FC } from "react";
import { Playback } from "./Playback";
import { useSettingsStore } from "../store/settings";
import { NetEaseSource } from "./NetEaseSource";

const DesktopPage: FC<{ windowWidth: number }> = ({ windowWidth }) => {
    const interfaceWidth = useSettingsStore((s) => s.interfaceWidth);
    const defineWidth = () => {
        if (interfaceWidth) return interfaceWidth;
        return windowWidth >= 2400 ? "xl" : "lg";
    };

    return (
        <Container size={defineWidth()}>
            <Flex p="sm" direction="column" justify="center" align="center" h="100vh">
                <Playback />
                <Flex mt="sm" w="100%">
                    <NetEaseSource />
                </Flex>
            </Flex>
        </Container>
    );
};

export default DesktopPage;
