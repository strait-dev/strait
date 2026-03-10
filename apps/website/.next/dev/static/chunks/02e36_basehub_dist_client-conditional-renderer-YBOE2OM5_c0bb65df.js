(globalThis.TURBOPACK || (globalThis.TURBOPACK = [])).push([typeof document === "object" ? document.currentScript : undefined,
"[project]/node_modules/.bun/basehub@9.5.3+7111ff09de72ce5c/node_modules/basehub/dist/client-conditional-renderer-YBOE2OM5.js [app-client] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "ClientConditionalRenderer",
    ()=>ClientConditionalRenderer
]);
// src/next/toolbar/client-conditional-renderer.tsx
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/compiled/react/index.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2d$dom$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/compiled/react-dom/index.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/compiled/react/jsx-runtime.js [app-client] (ecmascript)");
"use client";
;
;
;
;
var LazyClientToolbar = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["lazy"](()=>__turbopack_context__.A("[project]/node_modules/.bun/basehub@9.5.3+7111ff09de72ce5c/node_modules/basehub/dist/client-toolbar-GQ6ROCY6.js [app-client] (ecmascript, async loader)").then((mod)=>({
            default: mod.ClientToolbar
        })));
var ClientConditionalRenderer = ({ draft, isForcedDraft, resolvedRef: _resolvedRef, revalidateTags, ...actions })=>{
    const [hasRendered, setHasRendered] = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useState"](false);
    const [resolvedRef] = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useState"](_resolvedRef);
    const revalidateTagsRef = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useRef"](revalidateTags);
    revalidateTagsRef.current = revalidateTags;
    __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useEffect"]({
        "ClientConditionalRenderer.useEffect": ()=>{
            setHasRendered(true);
        }
    }["ClientConditionalRenderer.useEffect"], []);
    const bshbPreviewLSName = `bshb-preview-${resolvedRef.repoHash}`;
    const seekAndStoreBshbPreviewToken = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useCallback"]({
        "ClientConditionalRenderer.useCallback[seekAndStoreBshbPreviewToken]": (type)=>{
            if (typeof window === "undefined") return;
            const urlParams = new URLSearchParams(window.location.search);
            const bshbPreviewToken2 = urlParams.get("bshb-preview");
            if (bshbPreviewToken2) {
                try {
                    window.localStorage?.setItem(bshbPreviewLSName, bshbPreviewToken2);
                } catch (e) {}
                return bshbPreviewToken2;
            }
            if (type === "url-only") return;
            try {
                const fromStorage = window.localStorage?.getItem(bshbPreviewLSName);
                if (fromStorage) return fromStorage;
            } catch (e) {}
        }
    }["ClientConditionalRenderer.useCallback[seekAndStoreBshbPreviewToken]"], [
        bshbPreviewLSName
    ]);
    const [bshbPreviewToken, setBshbPreviewToken] = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useState"](seekAndStoreBshbPreviewToken);
    const [shouldAutoEnableDraft, setShouldAutoEnableDraft] = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useState"]();
    __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useLayoutEffect"]({
        "ClientConditionalRenderer.useLayoutEffect": ()=>{
            if (draft || isForcedDraft) {
                setShouldAutoEnableDraft(false);
                return;
            }
            const previewToken = seekAndStoreBshbPreviewToken("url-only");
            if (!previewToken) {
                setShouldAutoEnableDraft(false);
                return;
            }
            setBshbPreviewToken(previewToken);
            setShouldAutoEnableDraft(true);
        }
    }["ClientConditionalRenderer.useLayoutEffect"], [
        draft,
        isForcedDraft,
        seekAndStoreBshbPreviewToken
    ]);
    __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useEffect"]({
        "ClientConditionalRenderer.useEffect": ()=>{
            const url = new URL(window.location.href);
            const shouldRevalidate = url.searchParams.get("__bshb-odr") === "true" && !document.documentElement.dataset.basehubOdrStatus;
            const odrToken = url.searchParams.get("__bshb-odr-token");
            const ref = url.searchParams.get("__bshb-odr-ref");
            if (shouldRevalidate && odrToken) {
                revalidateTagsRef.current({
                    warmupRun: true,
                    bshbPreviewToken: odrToken,
                    ...ref ? {
                        ref
                    } : {}
                }).then({
                    "ClientConditionalRenderer.useEffect": async (dryRunResult)=>{
                        if (dryRunResult.success && dryRunResult.fetchData) {
                            await fetch(dryRunResult.fetchData.url, dryRunResult.fetchData.options);
                        }
                        const { success, message } = await revalidateTagsRef.current({
                            bshbPreviewToken: odrToken,
                            ...ref ? {
                                ref
                            } : {}
                        });
                        document.documentElement.dataset.basehubOdrStatus = success ? "success" : "error";
                        if (!success) {
                            document.documentElement.dataset.basehubOdrErrorMessage = "Response failed";
                        }
                        if (message) {
                            document.documentElement.dataset.basehubOdrMessage = message;
                        }
                    }
                }["ClientConditionalRenderer.useEffect"]).catch({
                    "ClientConditionalRenderer.useEffect": (e)=>{
                        document.documentElement.dataset.basehubOdrStatus = "error";
                        let errorMessage = "";
                        try {
                            errorMessage = e.message;
                        } catch (err) {
                            console.error(err);
                            errorMessage = "Unknown error";
                        }
                        document.documentElement.dataset.basehubOdrErrorMessage = errorMessage;
                    }
                }["ClientConditionalRenderer.useEffect"]);
            }
        }
    }["ClientConditionalRenderer.useEffect"], []);
    if (!bshbPreviewToken && !isForcedDraft || !hasRendered || typeof document === "undefined") {
        return null;
    }
    const Portal = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2d$dom$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["createPortal"])(/* @__PURE__ */ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsx"])(LazyClientToolbar, {
        ...actions,
        draft,
        isForcedDraft,
        bshbPreviewToken,
        shouldAutoEnableDraft,
        seekAndStoreBshbPreviewToken,
        resolvedRef,
        bshbPreviewLSName
    }), document.body);
    return Portal;
};
;
}),
]);

//# sourceMappingURL=02e36_basehub_dist_client-conditional-renderer-YBOE2OM5_c0bb65df.js.map