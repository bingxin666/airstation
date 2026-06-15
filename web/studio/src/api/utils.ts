export const jsonRequestParams = (method: string, body: Record<string, any>) => {
    return {
        method,
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(body),
    };
};
