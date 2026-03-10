(globalThis.TURBOPACK || (globalThis.TURBOPACK = [])).push([typeof document === "object" ? document.currentScript : undefined,
"[project]/apps/website/src/app/(landing)/components/blog/pagination.tsx [app-client] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "default",
    ()=>__TURBOPACK__default__export__
]);
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/compiled/react/jsx-dev-runtime.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$compiler$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/compiled/react/compiler-runtime.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$hugeicons$2b$core$2d$free$2d$icons$40$4$2e$0$2e$0$2f$node_modules$2f40$hugeicons$2f$core$2d$free$2d$icons$2f$dist$2f$esm$2f$ArrowLeft01Icon$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$export__default__as__ArrowLeft01Icon$3e$__ = __turbopack_context__.i("[project]/node_modules/.bun/@hugeicons+core-free-icons@4.0.0/node_modules/@hugeicons/core-free-icons/dist/esm/ArrowLeft01Icon.js [app-client] (ecmascript) <export default as ArrowLeft01Icon>");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$hugeicons$2b$core$2d$free$2d$icons$40$4$2e$0$2e$0$2f$node_modules$2f40$hugeicons$2f$core$2d$free$2d$icons$2f$dist$2f$esm$2f$ArrowRight01Icon$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$export__default__as__ArrowRight01Icon$3e$__ = __turbopack_context__.i("[project]/node_modules/.bun/@hugeicons+core-free-icons@4.0.0/node_modules/@hugeicons/core-free-icons/dist/esm/ArrowRight01Icon.js [app-client] (ecmascript) <export default as ArrowRight01Icon>");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$hugeicons$2b$react$40$1$2e$1$2e$6$2b$b1ab299f0a400331$2f$node_modules$2f40$hugeicons$2f$react$2f$dist$2f$esm$2f$HugeiconsIcon$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@hugeicons+react@1.1.6+b1ab299f0a400331/node_modules/@hugeicons/react/dist/esm/HugeiconsIcon.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$packages$2f$ui$2f$src$2f$components$2f$button$2e$tsx__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/packages/ui/src/components/button.tsx [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$packages$2f$ui$2f$src$2f$utils$2f$index$2e$ts__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/packages/ui/src/utils/index.ts [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$client$2f$app$2d$dir$2f$link$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/client/app-dir/link.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$navigation$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/navigation.js [app-client] (ecmascript)");
;
var _s = __turbopack_context__.k.signature();
"use client";
;
;
;
;
;
;
;
const createPageNumbers = (currentPage, totalPages)=>{
    const MAX_VISIBLE = 5;
    if (totalPages <= MAX_VISIBLE) {
        return Array.from({
            length: totalPages
        }, (_, i)=>i + 1);
    }
    const pages = [
        1
    ];
    const showStartEllipsis = currentPage > 3;
    const showEndEllipsis = currentPage < totalPages - 2;
    if (showStartEllipsis) {
        pages.push("ellipsis-start");
    }
    const start = Math.max(2, currentPage - 1);
    const end = Math.min(totalPages - 1, currentPage + 1);
    for(let i = start; i <= end; i++){
        pages.push(i);
    }
    if (showEndEllipsis) {
        pages.push("ellipsis-end");
    }
    if (!pages.includes(totalPages)) {
        pages.push(totalPages);
    }
    return pages;
};
const Pagination = (t0)=>{
    _s();
    const $ = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$compiler$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["c"])(37);
    if ($[0] !== "716a4568abf2dba63376530ac52f150a2c5873241ea6f06da2d0fa66a26ea72f") {
        for(let $i = 0; $i < 37; $i += 1){
            $[$i] = Symbol.for("react.memo_cache_sentinel");
        }
        $[0] = "716a4568abf2dba63376530ac52f150a2c5873241ea6f06da2d0fa66a26ea72f";
    }
    const { currentPage, totalPages, className } = t0;
    const pathname = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$navigation$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["usePathname"])();
    const searchParams = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$navigation$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useSearchParams"])();
    let t1;
    if ($[1] !== pathname || $[2] !== searchParams) {
        t1 = (page)=>{
            const params = new URLSearchParams(searchParams.toString());
            if (page === 1) {
                params.delete("page");
            } else {
                params.set("page", page.toString());
            }
            const queryString = params.toString();
            return queryString ? `${pathname}?${queryString}` : pathname;
        };
        $[1] = pathname;
        $[2] = searchParams;
        $[3] = t1;
    } else {
        t1 = $[3];
    }
    const createPageUrl = t1;
    const hasPreviousPage = currentPage > 1;
    const hasNextPage = currentPage < totalPages;
    let t2;
    let t3;
    let t4;
    let t5;
    let t6;
    let t7;
    if ($[4] !== className || $[5] !== createPageUrl || $[6] !== currentPage || $[7] !== hasPreviousPage || $[8] !== totalPages) {
        t7 = Symbol.for("react.early_return_sentinel");
        bb0: {
            const pageNumbers = createPageNumbers(currentPage, totalPages);
            if (totalPages <= 1) {
                t7 = null;
                break bb0;
            }
            t4 = "Blog pagination";
            if ($[15] !== className) {
                t5 = (0, __TURBOPACK__imported__module__$5b$project$5d2f$packages$2f$ui$2f$src$2f$utils$2f$index$2e$ts__$5b$app$2d$client$5d$__$28$ecmascript$29$__["cn"])("flex items-center justify-center gap-2", className);
                $[15] = className;
                $[16] = t5;
            } else {
                t5 = $[16];
            }
            if ($[17] !== createPageUrl || $[18] !== currentPage || $[19] !== hasPreviousPage) {
                t6 = hasPreviousPage ? /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])(__TURBOPACK__imported__module__$5b$project$5d2f$packages$2f$ui$2f$src$2f$components$2f$button$2e$tsx__$5b$app$2d$client$5d$__$28$ecmascript$29$__["Button"], {
                    className: "cursor-default gap-1.5 text-foreground",
                    render: /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])(__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$client$2f$app$2d$dir$2f$link$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["default"], {
                        href: createPageUrl(currentPage - 1),
                        prefetch: true
                    }, void 0, false, {
                        fileName: "[project]/apps/website/src/app/(landing)/components/blog/pagination.tsx",
                        lineNumber: 100,
                        columnNumber: 99
                    }, void 0),
                    variant: "ghost",
                    children: [
                        /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])(__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$hugeicons$2b$react$40$1$2e$1$2e$6$2b$b1ab299f0a400331$2f$node_modules$2f40$hugeicons$2f$react$2f$dist$2f$esm$2f$HugeiconsIcon$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["HugeiconsIcon"], {
                            className: "size-4",
                            icon: __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$hugeicons$2b$core$2d$free$2d$icons$40$4$2e$0$2e$0$2f$node_modules$2f40$hugeicons$2f$core$2d$free$2d$icons$2f$dist$2f$esm$2f$ArrowLeft01Icon$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$export__default__as__ArrowLeft01Icon$3e$__["ArrowLeft01Icon"]
                        }, void 0, false, {
                            fileName: "[project]/apps/website/src/app/(landing)/components/blog/pagination.tsx",
                            lineNumber: 100,
                            columnNumber: 179
                        }, ("TURBOPACK compile-time value", void 0)),
                        /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])("span", {
                            className: "hidden sm:inline",
                            children: "Previous"
                        }, void 0, false, {
                            fileName: "[project]/apps/website/src/app/(landing)/components/blog/pagination.tsx",
                            lineNumber: 100,
                            columnNumber: 238
                        }, ("TURBOPACK compile-time value", void 0))
                    ]
                }, void 0, true, {
                    fileName: "[project]/apps/website/src/app/(landing)/components/blog/pagination.tsx",
                    lineNumber: 100,
                    columnNumber: 32
                }, ("TURBOPACK compile-time value", void 0)) : /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])(__TURBOPACK__imported__module__$5b$project$5d2f$packages$2f$ui$2f$src$2f$components$2f$button$2e$tsx__$5b$app$2d$client$5d$__$28$ecmascript$29$__["Button"], {
                    className: "pointer-events-auto cursor-not-allowed gap-1.5 text-foreground",
                    disabled: true,
                    variant: "ghost",
                    children: [
                        /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])(__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$hugeicons$2b$react$40$1$2e$1$2e$6$2b$b1ab299f0a400331$2f$node_modules$2f40$hugeicons$2f$react$2f$dist$2f$esm$2f$HugeiconsIcon$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["HugeiconsIcon"], {
                            className: "size-4",
                            icon: __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$hugeicons$2b$core$2d$free$2d$icons$40$4$2e$0$2e$0$2f$node_modules$2f40$hugeicons$2f$core$2d$free$2d$icons$2f$dist$2f$esm$2f$ArrowLeft01Icon$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$export__default__as__ArrowLeft01Icon$3e$__["ArrowLeft01Icon"]
                        }, void 0, false, {
                            fileName: "[project]/apps/website/src/app/(landing)/components/blog/pagination.tsx",
                            lineNumber: 100,
                            columnNumber: 415
                        }, ("TURBOPACK compile-time value", void 0)),
                        /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])("span", {
                            className: "hidden sm:inline",
                            children: "Previous"
                        }, void 0, false, {
                            fileName: "[project]/apps/website/src/app/(landing)/components/blog/pagination.tsx",
                            lineNumber: 100,
                            columnNumber: 474
                        }, ("TURBOPACK compile-time value", void 0))
                    ]
                }, void 0, true, {
                    fileName: "[project]/apps/website/src/app/(landing)/components/blog/pagination.tsx",
                    lineNumber: 100,
                    columnNumber: 300
                }, ("TURBOPACK compile-time value", void 0));
                $[17] = createPageUrl;
                $[18] = currentPage;
                $[19] = hasPreviousPage;
                $[20] = t6;
            } else {
                t6 = $[20];
            }
            t2 = "flex items-center gap-1";
            let t8;
            if ($[21] !== createPageUrl || $[22] !== currentPage) {
                t8 = (page_0)=>{
                    if (typeof page_0 === "string") {
                        return /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])("span", {
                            className: "flex size-10 items-center justify-center text-muted-foreground",
                            children: "..."
                        }, page_0, false, {
                            fileName: "[project]/apps/website/src/app/(landing)/components/blog/pagination.tsx",
                            lineNumber: 113,
                            columnNumber: 20
                        }, ("TURBOPACK compile-time value", void 0));
                    }
                    if (page_0 === currentPage) {
                        return /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])(__TURBOPACK__imported__module__$5b$project$5d2f$packages$2f$ui$2f$src$2f$components$2f$button$2e$tsx__$5b$app$2d$client$5d$__$28$ecmascript$29$__["Button"], {
                            className: "pointer-events-none min-w-10 cursor-default",
                            variant: "default",
                            children: page_0
                        }, page_0, false, {
                            fileName: "[project]/apps/website/src/app/(landing)/components/blog/pagination.tsx",
                            lineNumber: 116,
                            columnNumber: 20
                        }, ("TURBOPACK compile-time value", void 0));
                    }
                    return /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])(__TURBOPACK__imported__module__$5b$project$5d2f$packages$2f$ui$2f$src$2f$components$2f$button$2e$tsx__$5b$app$2d$client$5d$__$28$ecmascript$29$__["Button"], {
                        className: "min-w-10 cursor-default",
                        render: /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])(__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$client$2f$app$2d$dir$2f$link$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["default"], {
                            href: createPageUrl(page_0),
                            prefetch: true
                        }, void 0, false, {
                            fileName: "[project]/apps/website/src/app/(landing)/components/blog/pagination.tsx",
                            lineNumber: 118,
                            columnNumber: 83
                        }, void 0),
                        variant: "ghost",
                        children: page_0
                    }, page_0, false, {
                        fileName: "[project]/apps/website/src/app/(landing)/components/blog/pagination.tsx",
                        lineNumber: 118,
                        columnNumber: 18
                    }, ("TURBOPACK compile-time value", void 0));
                };
                $[21] = createPageUrl;
                $[22] = currentPage;
                $[23] = t8;
            } else {
                t8 = $[23];
            }
            t3 = pageNumbers.map(t8);
        }
        $[4] = className;
        $[5] = createPageUrl;
        $[6] = currentPage;
        $[7] = hasPreviousPage;
        $[8] = totalPages;
        $[9] = t2;
        $[10] = t3;
        $[11] = t4;
        $[12] = t5;
        $[13] = t6;
        $[14] = t7;
    } else {
        t2 = $[9];
        t3 = $[10];
        t4 = $[11];
        t5 = $[12];
        t6 = $[13];
        t7 = $[14];
    }
    if (t7 !== Symbol.for("react.early_return_sentinel")) {
        return t7;
    }
    let t8;
    if ($[24] !== t2 || $[25] !== t3) {
        t8 = /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])("div", {
            className: t2,
            children: t3
        }, void 0, false, {
            fileName: "[project]/apps/website/src/app/(landing)/components/blog/pagination.tsx",
            lineNumber: 152,
            columnNumber: 10
        }, ("TURBOPACK compile-time value", void 0));
        $[24] = t2;
        $[25] = t3;
        $[26] = t8;
    } else {
        t8 = $[26];
    }
    let t9;
    if ($[27] !== createPageUrl || $[28] !== currentPage || $[29] !== hasNextPage) {
        t9 = hasNextPage ? /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])(__TURBOPACK__imported__module__$5b$project$5d2f$packages$2f$ui$2f$src$2f$components$2f$button$2e$tsx__$5b$app$2d$client$5d$__$28$ecmascript$29$__["Button"], {
            className: "cursor-default gap-1.5 text-foreground",
            render: /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])(__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$client$2f$app$2d$dir$2f$link$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["default"], {
                href: createPageUrl(currentPage + 1),
                prefetch: true
            }, void 0, false, {
                fileName: "[project]/apps/website/src/app/(landing)/components/blog/pagination.tsx",
                lineNumber: 161,
                columnNumber: 91
            }, void 0),
            variant: "ghost",
            children: [
                /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])("span", {
                    className: "hidden sm:inline",
                    children: "Next"
                }, void 0, false, {
                    fileName: "[project]/apps/website/src/app/(landing)/components/blog/pagination.tsx",
                    lineNumber: 161,
                    columnNumber: 171
                }, ("TURBOPACK compile-time value", void 0)),
                /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])(__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$hugeicons$2b$react$40$1$2e$1$2e$6$2b$b1ab299f0a400331$2f$node_modules$2f40$hugeicons$2f$react$2f$dist$2f$esm$2f$HugeiconsIcon$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["HugeiconsIcon"], {
                    className: "size-4",
                    icon: __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$hugeicons$2b$core$2d$free$2d$icons$40$4$2e$0$2e$0$2f$node_modules$2f40$hugeicons$2f$core$2d$free$2d$icons$2f$dist$2f$esm$2f$ArrowRight01Icon$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$export__default__as__ArrowRight01Icon$3e$__["ArrowRight01Icon"]
                }, void 0, false, {
                    fileName: "[project]/apps/website/src/app/(landing)/components/blog/pagination.tsx",
                    lineNumber: 161,
                    columnNumber: 217
                }, ("TURBOPACK compile-time value", void 0))
            ]
        }, void 0, true, {
            fileName: "[project]/apps/website/src/app/(landing)/components/blog/pagination.tsx",
            lineNumber: 161,
            columnNumber: 24
        }, ("TURBOPACK compile-time value", void 0)) : /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])(__TURBOPACK__imported__module__$5b$project$5d2f$packages$2f$ui$2f$src$2f$components$2f$button$2e$tsx__$5b$app$2d$client$5d$__$28$ecmascript$29$__["Button"], {
            className: "pointer-events-auto cursor-not-allowed gap-1.5 text-foreground",
            disabled: true,
            variant: "ghost",
            children: [
                /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])("span", {
                    className: "hidden sm:inline",
                    children: "Next"
                }, void 0, false, {
                    fileName: "[project]/apps/website/src/app/(landing)/components/blog/pagination.tsx",
                    lineNumber: 161,
                    columnNumber: 404
                }, ("TURBOPACK compile-time value", void 0)),
                /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])(__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$hugeicons$2b$react$40$1$2e$1$2e$6$2b$b1ab299f0a400331$2f$node_modules$2f40$hugeicons$2f$react$2f$dist$2f$esm$2f$HugeiconsIcon$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["HugeiconsIcon"], {
                    className: "size-4",
                    icon: __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$hugeicons$2b$core$2d$free$2d$icons$40$4$2e$0$2e$0$2f$node_modules$2f40$hugeicons$2f$core$2d$free$2d$icons$2f$dist$2f$esm$2f$ArrowRight01Icon$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$export__default__as__ArrowRight01Icon$3e$__["ArrowRight01Icon"]
                }, void 0, false, {
                    fileName: "[project]/apps/website/src/app/(landing)/components/blog/pagination.tsx",
                    lineNumber: 161,
                    columnNumber: 450
                }, ("TURBOPACK compile-time value", void 0))
            ]
        }, void 0, true, {
            fileName: "[project]/apps/website/src/app/(landing)/components/blog/pagination.tsx",
            lineNumber: 161,
            columnNumber: 289
        }, ("TURBOPACK compile-time value", void 0));
        $[27] = createPageUrl;
        $[28] = currentPage;
        $[29] = hasNextPage;
        $[30] = t9;
    } else {
        t9 = $[30];
    }
    let t10;
    if ($[31] !== t4 || $[32] !== t5 || $[33] !== t6 || $[34] !== t8 || $[35] !== t9) {
        t10 = /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])("nav", {
            "aria-label": t4,
            className: t5,
            children: [
                t6,
                t8,
                t9
            ]
        }, void 0, true, {
            fileName: "[project]/apps/website/src/app/(landing)/components/blog/pagination.tsx",
            lineNumber: 171,
            columnNumber: 11
        }, ("TURBOPACK compile-time value", void 0));
        $[31] = t4;
        $[32] = t5;
        $[33] = t6;
        $[34] = t8;
        $[35] = t9;
        $[36] = t10;
    } else {
        t10 = $[36];
    }
    return t10;
};
_s(Pagination, "AxA9T5G2Po78UC4hL8ljCdvMciE=", false, function() {
    return [
        __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$navigation$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["usePathname"],
        __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$navigation$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useSearchParams"]
    ];
});
_c = Pagination;
const __TURBOPACK__default__export__ = Pagination;
var _c;
__turbopack_context__.k.register(_c, "Pagination");
if (typeof globalThis.$RefreshHelpers$ === 'object' && globalThis.$RefreshHelpers !== null) {
    __turbopack_context__.k.registerExports(__turbopack_context__.m, globalThis.$RefreshHelpers$);
}
}),
"[project]/apps/website/src/app/(landing)/components/blog/post-share.tsx [app-client] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "default",
    ()=>__TURBOPACK__default__export__
]);
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$build$2f$polyfills$2f$process$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = /*#__PURE__*/ __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/build/polyfills/process.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/compiled/react/jsx-dev-runtime.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$compiler$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/compiled/react/compiler-runtime.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$hugeicons$2b$core$2d$free$2d$icons$40$4$2e$0$2e$0$2f$node_modules$2f40$hugeicons$2f$core$2d$free$2d$icons$2f$dist$2f$esm$2f$Copy01Icon$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$export__default__as__Copy01Icon$3e$__ = __turbopack_context__.i("[project]/node_modules/.bun/@hugeicons+core-free-icons@4.0.0/node_modules/@hugeicons/core-free-icons/dist/esm/Copy01Icon.js [app-client] (ecmascript) <export default as Copy01Icon>");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$hugeicons$2b$core$2d$free$2d$icons$40$4$2e$0$2e$0$2f$node_modules$2f40$hugeicons$2f$core$2d$free$2d$icons$2f$dist$2f$esm$2f$Share01Icon$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$export__default__as__Share01Icon$3e$__ = __turbopack_context__.i("[project]/node_modules/.bun/@hugeicons+core-free-icons@4.0.0/node_modules/@hugeicons/core-free-icons/dist/esm/Share01Icon.js [app-client] (ecmascript) <export default as Share01Icon>");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$hugeicons$2b$core$2d$free$2d$icons$40$4$2e$0$2e$0$2f$node_modules$2f40$hugeicons$2f$core$2d$free$2d$icons$2f$dist$2f$esm$2f$Tick01Icon$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$export__default__as__Tick01Icon$3e$__ = __turbopack_context__.i("[project]/node_modules/.bun/@hugeicons+core-free-icons@4.0.0/node_modules/@hugeicons/core-free-icons/dist/esm/Tick01Icon.js [app-client] (ecmascript) <export default as Tick01Icon>");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$hugeicons$2b$react$40$1$2e$1$2e$6$2b$b1ab299f0a400331$2f$node_modules$2f40$hugeicons$2f$react$2f$dist$2f$esm$2f$HugeiconsIcon$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@hugeicons+react@1.1.6+b1ab299f0a400331/node_modules/@hugeicons/react/dist/esm/HugeiconsIcon.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$packages$2f$ui$2f$src$2f$components$2f$button$2e$tsx__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/packages/ui/src/components/button.tsx [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/compiled/react/index.js [app-client] (ecmascript)");
;
var _s = __turbopack_context__.k.signature();
"use client";
;
;
;
;
;
const BASE_URL = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$build$2f$polyfills$2f$process$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["default"].env.NEXT_PUBLIC_WEBSITE_URL || "https://trystrait.ai";
const PostShare = (t0)=>{
    _s();
    const $ = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$compiler$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["c"])(22);
    if ($[0] !== "bf2f26669c795958bcd3b4bbd681c1059593f9555bf3bf2521a93e16f1aa0301") {
        for(let $i = 0; $i < 22; $i += 1){
            $[$i] = Symbol.for("react.memo_cache_sentinel");
        }
        $[0] = "bf2f26669c795958bcd3b4bbd681c1059593f9555bf3bf2521a93e16f1aa0301";
    }
    const { title, slug } = t0;
    const [copied, setCopied] = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useState"])(false);
    const shareUrl = `${BASE_URL}/blog/${slug}`;
    let t1;
    if ($[1] !== shareUrl) {
        t1 = async ()=>{
            try {
                await navigator.clipboard.writeText(shareUrl);
                setCopied(true);
                setTimeout(()=>setCopied(false), 2000);
            } catch  {
                console.error("Failed to copy link");
            }
        };
        $[1] = shareUrl;
        $[2] = t1;
    } else {
        t1 = $[2];
    }
    const handleCopy = t1;
    let t2;
    if ($[3] !== handleCopy || $[4] !== shareUrl || $[5] !== title) {
        t2 = async ()=>{
            if (navigator.share) {
                try {
                    await navigator.share({
                        title,
                        url: shareUrl
                    });
                } catch  {
                    handleCopy();
                }
            } else {
                handleCopy();
            }
        };
        $[3] = handleCopy;
        $[4] = shareUrl;
        $[5] = title;
        $[6] = t2;
    } else {
        t2 = $[6];
    }
    const handleShare = t2;
    let t3;
    let t4;
    if ($[7] === Symbol.for("react.memo_cache_sentinel")) {
        t3 = /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])(__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$hugeicons$2b$react$40$1$2e$1$2e$6$2b$b1ab299f0a400331$2f$node_modules$2f40$hugeicons$2f$react$2f$dist$2f$esm$2f$HugeiconsIcon$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["HugeiconsIcon"], {
            className: "size-4",
            icon: __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$hugeicons$2b$core$2d$free$2d$icons$40$4$2e$0$2e$0$2f$node_modules$2f40$hugeicons$2f$core$2d$free$2d$icons$2f$dist$2f$esm$2f$Share01Icon$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$export__default__as__Share01Icon$3e$__["Share01Icon"]
        }, void 0, false, {
            fileName: "[project]/apps/website/src/app/(landing)/components/blog/post-share.tsx",
            lineNumber: 71,
            columnNumber: 10
        }, ("TURBOPACK compile-time value", void 0));
        t4 = /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])("span", {
            children: "Share"
        }, void 0, false, {
            fileName: "[project]/apps/website/src/app/(landing)/components/blog/post-share.tsx",
            lineNumber: 72,
            columnNumber: 10
        }, ("TURBOPACK compile-time value", void 0));
        $[7] = t3;
        $[8] = t4;
    } else {
        t3 = $[7];
        t4 = $[8];
    }
    let t5;
    if ($[9] !== handleShare) {
        t5 = /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])(__TURBOPACK__imported__module__$5b$project$5d2f$packages$2f$ui$2f$src$2f$components$2f$button$2e$tsx__$5b$app$2d$client$5d$__$28$ecmascript$29$__["Button"], {
            className: "gap-2",
            onClick: handleShare,
            size: "sm",
            variant: "outline",
            children: [
                t3,
                t4
            ]
        }, void 0, true, {
            fileName: "[project]/apps/website/src/app/(landing)/components/blog/post-share.tsx",
            lineNumber: 81,
            columnNumber: 10
        }, ("TURBOPACK compile-time value", void 0));
        $[9] = handleShare;
        $[10] = t5;
    } else {
        t5 = $[10];
    }
    const t6 = copied ? __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$hugeicons$2b$core$2d$free$2d$icons$40$4$2e$0$2e$0$2f$node_modules$2f40$hugeicons$2f$core$2d$free$2d$icons$2f$dist$2f$esm$2f$Tick01Icon$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$export__default__as__Tick01Icon$3e$__["Tick01Icon"] : __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$hugeicons$2b$core$2d$free$2d$icons$40$4$2e$0$2e$0$2f$node_modules$2f40$hugeicons$2f$core$2d$free$2d$icons$2f$dist$2f$esm$2f$Copy01Icon$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$export__default__as__Copy01Icon$3e$__["Copy01Icon"];
    let t7;
    if ($[11] !== t6) {
        t7 = /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])(__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$hugeicons$2b$react$40$1$2e$1$2e$6$2b$b1ab299f0a400331$2f$node_modules$2f40$hugeicons$2f$react$2f$dist$2f$esm$2f$HugeiconsIcon$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["HugeiconsIcon"], {
            className: "size-4",
            icon: t6
        }, void 0, false, {
            fileName: "[project]/apps/website/src/app/(landing)/components/blog/post-share.tsx",
            lineNumber: 90,
            columnNumber: 10
        }, ("TURBOPACK compile-time value", void 0));
        $[11] = t6;
        $[12] = t7;
    } else {
        t7 = $[12];
    }
    const t8 = copied ? "Copied!" : "Copy link";
    let t9;
    if ($[13] !== t8) {
        t9 = /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])("span", {
            children: t8
        }, void 0, false, {
            fileName: "[project]/apps/website/src/app/(landing)/components/blog/post-share.tsx",
            lineNumber: 99,
            columnNumber: 10
        }, ("TURBOPACK compile-time value", void 0));
        $[13] = t8;
        $[14] = t9;
    } else {
        t9 = $[14];
    }
    let t10;
    if ($[15] !== handleCopy || $[16] !== t7 || $[17] !== t9) {
        t10 = /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])(__TURBOPACK__imported__module__$5b$project$5d2f$packages$2f$ui$2f$src$2f$components$2f$button$2e$tsx__$5b$app$2d$client$5d$__$28$ecmascript$29$__["Button"], {
            className: "gap-2",
            onClick: handleCopy,
            size: "sm",
            variant: "ghost",
            children: [
                t7,
                t9
            ]
        }, void 0, true, {
            fileName: "[project]/apps/website/src/app/(landing)/components/blog/post-share.tsx",
            lineNumber: 107,
            columnNumber: 11
        }, ("TURBOPACK compile-time value", void 0));
        $[15] = handleCopy;
        $[16] = t7;
        $[17] = t9;
        $[18] = t10;
    } else {
        t10 = $[18];
    }
    let t11;
    if ($[19] !== t10 || $[20] !== t5) {
        t11 = /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])("div", {
            className: "flex items-center gap-2",
            children: [
                t5,
                t10
            ]
        }, void 0, true, {
            fileName: "[project]/apps/website/src/app/(landing)/components/blog/post-share.tsx",
            lineNumber: 117,
            columnNumber: 11
        }, ("TURBOPACK compile-time value", void 0));
        $[19] = t10;
        $[20] = t5;
        $[21] = t11;
    } else {
        t11 = $[21];
    }
    return t11;
};
_s(PostShare, "NE86rL3vg4NVcTTWDavsT0hUBJs=");
_c = PostShare;
const __TURBOPACK__default__export__ = PostShare;
var _c;
__turbopack_context__.k.register(_c, "PostShare");
if (typeof globalThis.$RefreshHelpers$ === 'object' && globalThis.$RefreshHelpers !== null) {
    __turbopack_context__.k.registerExports(__turbopack_context__.m, globalThis.$RefreshHelpers$);
}
}),
"[project]/apps/website/src/app/(landing)/components/blog/post-toc.client.tsx [app-client] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "default",
    ()=>__TURBOPACK__default__export__
]);
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/compiled/react/jsx-dev-runtime.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$compiler$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/compiled/react/compiler-runtime.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$hugeicons$2b$core$2d$free$2d$icons$40$4$2e$0$2e$0$2f$node_modules$2f40$hugeicons$2f$core$2d$free$2d$icons$2f$dist$2f$esm$2f$Menu01Icon$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$export__default__as__Menu01Icon$3e$__ = __turbopack_context__.i("[project]/node_modules/.bun/@hugeicons+core-free-icons@4.0.0/node_modules/@hugeicons/core-free-icons/dist/esm/Menu01Icon.js [app-client] (ecmascript) <export default as Menu01Icon>");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$hugeicons$2b$react$40$1$2e$1$2e$6$2b$b1ab299f0a400331$2f$node_modules$2f40$hugeicons$2f$react$2f$dist$2f$esm$2f$HugeiconsIcon$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@hugeicons+react@1.1.6+b1ab299f0a400331/node_modules/@hugeicons/react/dist/esm/HugeiconsIcon.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$packages$2f$ui$2f$src$2f$components$2f$button$2e$tsx__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/packages/ui/src/components/button.tsx [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$packages$2f$ui$2f$src$2f$utils$2f$index$2e$ts__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/packages/ui/src/utils/index.ts [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/compiled/react/index.js [app-client] (ecmascript)");
;
var _s = __turbopack_context__.k.signature();
"use client";
;
;
;
;
;
;
const PostTocClient = (t0)=>{
    _s();
    const $ = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$compiler$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["c"])(31);
    if ($[0] !== "c4e7aec4d0623524a9edd9d97d91026f7f9ee71eadde0b1ea52fd54adaff643c") {
        for(let $i = 0; $i < 31; $i += 1){
            $[$i] = Symbol.for("react.memo_cache_sentinel");
        }
        $[0] = "c4e7aec4d0623524a9edd9d97d91026f7f9ee71eadde0b1ea52fd54adaff643c";
    }
    const { headings } = t0;
    const [activeId, setActiveId] = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useState"])("");
    const [isOpen, setIsOpen] = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useState"])(false);
    let t1;
    let t2;
    if ($[1] !== headings) {
        t1 = ()=>{
            const headingElements = headings.map(_temp).filter(Boolean);
            if (headingElements.length === 0) {
                return;
            }
            const observer = new IntersectionObserver((entries)=>{
                for (const entry of entries){
                    if (entry.isIntersecting) {
                        setActiveId(entry.target.id);
                        break;
                    }
                }
            }, {
                rootMargin: "-80px 0px -80% 0px",
                threshold: 0
            });
            for (const el of headingElements){
                observer.observe(el);
            }
            return ()=>observer.disconnect();
        };
        t2 = [
            headings
        ];
        $[1] = headings;
        $[2] = t1;
        $[3] = t2;
    } else {
        t1 = $[2];
        t2 = $[3];
    }
    (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useEffect"])(t1, t2);
    let t3;
    if ($[4] === Symbol.for("react.memo_cache_sentinel")) {
        t3 = (e, id)=>{
            e.preventDefault();
            const element = document.getElementById(id);
            if (element) {
                const elementPosition = element.getBoundingClientRect().top;
                const offsetPosition = elementPosition + window.scrollY - 100;
                window.scrollTo({
                    top: offsetPosition,
                    behavior: "smooth"
                });
                setActiveId(id);
                setIsOpen(false);
            }
        };
        $[4] = t3;
    } else {
        t3 = $[4];
    }
    const handleClick = t3;
    if (headings.length === 0) {
        return null;
    }
    let t4;
    if ($[5] !== isOpen) {
        t4 = ()=>setIsOpen(!isOpen);
        $[5] = isOpen;
        $[6] = t4;
    } else {
        t4 = $[6];
    }
    let t5;
    if ($[7] === Symbol.for("react.memo_cache_sentinel")) {
        t5 = /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])("span", {
            className: "flex items-center gap-2",
            children: [
                /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])(__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$hugeicons$2b$react$40$1$2e$1$2e$6$2b$b1ab299f0a400331$2f$node_modules$2f40$hugeicons$2f$react$2f$dist$2f$esm$2f$HugeiconsIcon$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["HugeiconsIcon"], {
                    className: "size-4",
                    icon: __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$hugeicons$2b$core$2d$free$2d$icons$40$4$2e$0$2e$0$2f$node_modules$2f40$hugeicons$2f$core$2d$free$2d$icons$2f$dist$2f$esm$2f$Menu01Icon$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$export__default__as__Menu01Icon$3e$__["Menu01Icon"]
                }, void 0, false, {
                    fileName: "[project]/apps/website/src/app/(landing)/components/blog/post-toc.client.tsx",
                    lineNumber: 93,
                    columnNumber: 52
                }, ("TURBOPACK compile-time value", void 0)),
                "Table of Contents"
            ]
        }, void 0, true, {
            fileName: "[project]/apps/website/src/app/(landing)/components/blog/post-toc.client.tsx",
            lineNumber: 93,
            columnNumber: 10
        }, ("TURBOPACK compile-time value", void 0));
        $[7] = t5;
    } else {
        t5 = $[7];
    }
    let t6;
    if ($[8] !== headings.length) {
        t6 = /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])("span", {
            className: "text-muted-foreground text-sm",
            children: [
                headings.length,
                " sections"
            ]
        }, void 0, true, {
            fileName: "[project]/apps/website/src/app/(landing)/components/blog/post-toc.client.tsx",
            lineNumber: 100,
            columnNumber: 10
        }, ("TURBOPACK compile-time value", void 0));
        $[8] = headings.length;
        $[9] = t6;
    } else {
        t6 = $[9];
    }
    let t7;
    if ($[10] !== t4 || $[11] !== t6) {
        t7 = /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])(__TURBOPACK__imported__module__$5b$project$5d2f$packages$2f$ui$2f$src$2f$components$2f$button$2e$tsx__$5b$app$2d$client$5d$__$28$ecmascript$29$__["Button"], {
            className: "w-full justify-between",
            onClick: t4,
            variant: "outline",
            children: [
                t5,
                t6
            ]
        }, void 0, true, {
            fileName: "[project]/apps/website/src/app/(landing)/components/blog/post-toc.client.tsx",
            lineNumber: 108,
            columnNumber: 10
        }, ("TURBOPACK compile-time value", void 0));
        $[10] = t4;
        $[11] = t6;
        $[12] = t7;
    } else {
        t7 = $[12];
    }
    let t8;
    if ($[13] !== activeId || $[14] !== headings || $[15] !== isOpen) {
        t8 = isOpen && /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])("nav", {
            className: "mt-2 rounded-lg border border-border bg-card p-4",
            children: /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])("ul", {
                className: "space-y-2",
                children: headings.map((heading)=>/*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])("li", {
                        children: /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])("a", {
                            className: (0, __TURBOPACK__imported__module__$5b$project$5d2f$packages$2f$ui$2f$src$2f$utils$2f$index$2e$ts__$5b$app$2d$client$5d$__$28$ecmascript$29$__["cn"])("block text-sm transition-colors hover:text-primary", heading.level === 3 && "pl-4", activeId === heading.id ? "font-medium text-primary" : "text-muted-foreground"),
                            href: `#${heading.id}`,
                            onClick: (e_0)=>handleClick(e_0, heading.id),
                            children: heading.text
                        }, void 0, false, {
                            fileName: "[project]/apps/website/src/app/(landing)/components/blog/post-toc.client.tsx",
                            lineNumber: 117,
                            columnNumber: 158
                        }, ("TURBOPACK compile-time value", void 0))
                    }, heading.id, false, {
                        fileName: "[project]/apps/website/src/app/(landing)/components/blog/post-toc.client.tsx",
                        lineNumber: 117,
                        columnNumber: 137
                    }, ("TURBOPACK compile-time value", void 0)))
            }, void 0, false, {
                fileName: "[project]/apps/website/src/app/(landing)/components/blog/post-toc.client.tsx",
                lineNumber: 117,
                columnNumber: 86
            }, ("TURBOPACK compile-time value", void 0))
        }, void 0, false, {
            fileName: "[project]/apps/website/src/app/(landing)/components/blog/post-toc.client.tsx",
            lineNumber: 117,
            columnNumber: 20
        }, ("TURBOPACK compile-time value", void 0));
        $[13] = activeId;
        $[14] = headings;
        $[15] = isOpen;
        $[16] = t8;
    } else {
        t8 = $[16];
    }
    let t9;
    if ($[17] !== t7 || $[18] !== t8) {
        t9 = /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])("div", {
            className: "mb-6 lg:hidden",
            children: [
                t7,
                t8
            ]
        }, void 0, true, {
            fileName: "[project]/apps/website/src/app/(landing)/components/blog/post-toc.client.tsx",
            lineNumber: 127,
            columnNumber: 10
        }, ("TURBOPACK compile-time value", void 0));
        $[17] = t7;
        $[18] = t8;
        $[19] = t9;
    } else {
        t9 = $[19];
    }
    let t10;
    if ($[20] === Symbol.for("react.memo_cache_sentinel")) {
        t10 = /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])("p", {
            className: "mb-4 font-semibold text-foreground text-sm",
            children: "On this page"
        }, void 0, false, {
            fileName: "[project]/apps/website/src/app/(landing)/components/blog/post-toc.client.tsx",
            lineNumber: 136,
            columnNumber: 11
        }, ("TURBOPACK compile-time value", void 0));
        $[20] = t10;
    } else {
        t10 = $[20];
    }
    let t11;
    if ($[21] !== activeId || $[22] !== headings) {
        let t12;
        if ($[24] !== activeId) {
            t12 = (heading_0)=>/*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])("li", {
                    children: /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])("a", {
                        className: (0, __TURBOPACK__imported__module__$5b$project$5d2f$packages$2f$ui$2f$src$2f$utils$2f$index$2e$ts__$5b$app$2d$client$5d$__$28$ecmascript$29$__["cn"])("-ml-px block border-l-2 py-1 pl-4 text-sm transition-colors hover:border-primary hover:text-primary", heading_0.level === 3 && "pl-8", activeId === heading_0.id ? "border-primary font-medium text-primary" : "border-transparent text-muted-foreground"),
                        href: `#${heading_0.id}`,
                        onClick: (e_1)=>handleClick(e_1, heading_0.id),
                        children: heading_0.text
                    }, void 0, false, {
                        fileName: "[project]/apps/website/src/app/(landing)/components/blog/post-toc.client.tsx",
                        lineNumber: 145,
                        columnNumber: 49
                    }, ("TURBOPACK compile-time value", void 0))
                }, heading_0.id, false, {
                    fileName: "[project]/apps/website/src/app/(landing)/components/blog/post-toc.client.tsx",
                    lineNumber: 145,
                    columnNumber: 26
                }, ("TURBOPACK compile-time value", void 0));
            $[24] = activeId;
            $[25] = t12;
        } else {
            t12 = $[25];
        }
        t11 = headings.map(t12);
        $[21] = activeId;
        $[22] = headings;
        $[23] = t11;
    } else {
        t11 = $[23];
    }
    let t12;
    if ($[26] !== t11) {
        t12 = /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])("nav", {
            "aria-label": "Table of Contents",
            className: "sticky top-24 hidden max-h-[calc(100vh-8rem)] overflow-y-auto lg:block",
            children: [
                t10,
                /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])("ul", {
                    className: "space-y-2 border-border border-l",
                    children: t11
                }, void 0, false, {
                    fileName: "[project]/apps/website/src/app/(landing)/components/blog/post-toc.client.tsx",
                    lineNumber: 160,
                    columnNumber: 135
                }, ("TURBOPACK compile-time value", void 0))
            ]
        }, void 0, true, {
            fileName: "[project]/apps/website/src/app/(landing)/components/blog/post-toc.client.tsx",
            lineNumber: 160,
            columnNumber: 11
        }, ("TURBOPACK compile-time value", void 0));
        $[26] = t11;
        $[27] = t12;
    } else {
        t12 = $[27];
    }
    let t13;
    if ($[28] !== t12 || $[29] !== t9) {
        t13 = /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])(__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["Fragment"], {
            children: [
                t9,
                t12
            ]
        }, void 0, true);
        $[28] = t12;
        $[29] = t9;
        $[30] = t13;
    } else {
        t13 = $[30];
    }
    return t13;
};
_s(PostTocClient, "lHl1EAMgUC/sXoZDd7Z4GSdDB0k=");
_c = PostTocClient;
const __TURBOPACK__default__export__ = PostTocClient;
function _temp(h) {
    return document.getElementById(h.id);
}
var _c;
__turbopack_context__.k.register(_c, "PostTocClient");
if (typeof globalThis.$RefreshHelpers$ === 'object' && globalThis.$RefreshHelpers !== null) {
    __turbopack_context__.k.registerExports(__turbopack_context__.m, globalThis.$RefreshHelpers$);
}
}),
"[project]/node_modules/.bun/basehub@9.5.3+7111ff09de72ce5c/node_modules/basehub/dist/chunk-Z6OIZIMQ.js [app-client] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "aliasSeparator",
    ()=>aliasSeparator,
    "replaceSystemAliases",
    ()=>replaceSystemAliases
]);
// src/genql/runtime/_aliasing.ts
var aliasSeparator = "__alias__";
function replaceSystemAliases(obj) {
    if (typeof obj !== "object" || obj === null) {
        return obj;
    }
    if (Array.isArray(obj)) {
        return obj.map((item)=>replaceSystemAliases(item));
    }
    const newObj = {};
    for (const [key, value] of Object.entries(obj)){
        if (key.includes(aliasSeparator)) {
            const [_prefix, ...rest] = key.split(aliasSeparator);
            const newKey = rest.join(aliasSeparator);
            newObj[newKey] = replaceSystemAliases(value);
        } else {
            newObj[key] = replaceSystemAliases(value);
        }
    }
    return newObj;
}
;
}),
"[project]/node_modules/.bun/basehub@9.5.3+7111ff09de72ce5c/node_modules/basehub/dist/client-pump-J7K23F3Q.js [app-client] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "ClientPump",
    ()=>ClientPump
]);
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$basehub$40$9$2e$5$2e$3$2b$7111ff09de72ce5c$2f$node_modules$2f$basehub$2f$dist$2f$chunk$2d$Z6OIZIMQ$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/basehub@9.5.3+7111ff09de72ce5c/node_modules/basehub/dist/chunk-Z6OIZIMQ.js [app-client] (ecmascript)");
// src/react/pump/client-pump.tsx
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/compiled/react/index.js [app-client] (ecmascript)");
"use client";
;
;
;
var pusherMounted = false;
var subscribers = /* @__PURE__ */ new Set();
var clientCache = /* @__PURE__ */ new Map();
var lastResponseHashCache = /* @__PURE__ */ new Map();
var DEDUPE_TIME_MS = 32;
var ClientPump = ({ children, rawQueries: _rawQueries, pumpEndpoint, pumpHeaders: _pumpHeaders, pumpToken: initialPumpToken, initialState: _initialState, initialResolvedChildren, apiVersion, previewRef: _previewRef, explicitRef })=>{
    const childrenRef = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useRef"](children);
    childrenRef.current = children;
    const [initialState] = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useState"](_initialState);
    const [rawQueries] = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useState"](_rawQueries);
    const [pumpHeaders] = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useState"](_pumpHeaders);
    const pumpTokenRef = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useRef"](initialPumpToken);
    const [result, setResult] = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useState"](initialState);
    const initialStateRef = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useRef"](initialState);
    initialStateRef.current = initialState;
    const [previewRef, setPreviewRef] = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useState"](_previewRef);
    const previewRefRef = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useRef"](previewRef);
    previewRefRef.current = previewRef;
    const refetch = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useCallback"]({
        "ClientPump.useCallback[refetch]": async ()=>{
            let newPumpToken;
            let pusherData = void 0;
            let spaceID = void 0;
            const responses = await Promise.all(rawQueries.map({
                "ClientPump.useCallback[refetch]": async (rawQueryOp, index)=>{
                    if (!pumpTokenRef.current) {
                        console.warn("No pump token found. Skipping query.");
                        return null;
                    }
                    const queryHash = JSON.stringify(rawQueryOp);
                    const responseHashCacheKey = queryHash;
                    const queryCacheKey = queryHash + previewRef;
                    const lastResponseHash = lastResponseHashCache.get(responseHashCacheKey) || initialStateRef.current?.responseHashes?.[index] || "";
                    if (clientCache.has(queryCacheKey)) {
                        const cached = clientCache.get(queryCacheKey);
                        if (performance.now() - cached.start < DEDUPE_TIME_MS) {
                            const response2 = await cached.response;
                            if (!response2) return null;
                            if (response2.newPumpToken) {
                                newPumpToken = response2.newPumpToken;
                            }
                            pusherData = response2.pusherData;
                            spaceID = response2.spaceID;
                            return response2;
                        }
                    }
                    const responsePromise = fetch(pumpEndpoint, {
                        cache: "no-store",
                        method: "POST",
                        headers: {
                            ...pumpHeaders,
                            "content-type": "application/json",
                            "x-basehub-api-version": apiVersion,
                            "x-basehub-ref": previewRef
                        },
                        body: JSON.stringify({
                            ...rawQueryOp,
                            pumpToken: pumpTokenRef.current,
                            lastResponseHash
                        })
                    }).then({
                        "ClientPump.useCallback[refetch].responsePromise": async (response2)=>{
                            const { data = null, errors = null, newPumpToken: newPumpToken2, spaceID: spaceID2, pusherData: pusherData2, responseHash } = await response2.json();
                            lastResponseHashCache.set(responseHashCacheKey, responseHash);
                            return {
                                data: (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$basehub$40$9$2e$5$2e$3$2b$7111ff09de72ce5c$2f$node_modules$2f$basehub$2f$dist$2f$chunk$2d$Z6OIZIMQ$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["replaceSystemAliases"])(data),
                                spaceID: spaceID2,
                                pusherData: pusherData2,
                                newPumpToken: newPumpToken2,
                                errors,
                                responseHash,
                                changed: lastResponseHash !== responseHash
                            };
                        }
                    }["ClientPump.useCallback[refetch].responsePromise"]).catch({
                        "ClientPump.useCallback[refetch].responsePromise": (err)=>{
                            console.error(`Error fetching data from the BaseHub Draft API:
              
${JSON.stringify(err, null, 2)}
              
Contact support@basehub.com for help.`);
                        }
                    }["ClientPump.useCallback[refetch].responsePromise"]);
                    clientCache.set(queryCacheKey, {
                        start: performance.now(),
                        response: responsePromise
                    });
                    const response = await responsePromise;
                    if (!response) return null;
                    if (response.newPumpToken) {
                        newPumpToken = response.newPumpToken;
                    }
                    pusherData = response.pusherData;
                    spaceID = response.spaceID;
                    return response;
                }
            }["ClientPump.useCallback[refetch]"]));
            const shouldUpdate = responses.some({
                "ClientPump.useCallback[refetch].shouldUpdate": (r)=>r?.changed
            }["ClientPump.useCallback[refetch].shouldUpdate"]);
            if (shouldUpdate) {
                if (!pusherData || !spaceID) return;
                setResult({
                    "ClientPump.useCallback[refetch]": (p)=>{
                        if (!pusherData || !spaceID) return p;
                        return {
                            data: responses.map({
                                "ClientPump.useCallback[refetch]": (r, i)=>{
                                    if (!r?.changed) return p?.data?.[i] ?? null;
                                    return r?.data ?? null;
                                }
                            }["ClientPump.useCallback[refetch]"]),
                            errors: responses.map({
                                "ClientPump.useCallback[refetch]": (r, i)=>{
                                    if (!r?.changed) return p?.errors?.[i] ?? null;
                                    return r?.errors ?? null;
                                }
                            }["ClientPump.useCallback[refetch]"]),
                            responseHashes: responses.map({
                                "ClientPump.useCallback[refetch]": (r)=>r?.responseHash ?? ""
                            }["ClientPump.useCallback[refetch]"]),
                            pusherData,
                            spaceID
                        };
                    }
                }["ClientPump.useCallback[refetch]"]);
            }
            if (newPumpToken) {
                pumpTokenRef.current = newPumpToken;
            }
        }
    }["ClientPump.useCallback[refetch]"], [
        pumpEndpoint,
        pumpHeaders,
        rawQueries,
        apiVersion,
        previewRef
    ]);
    __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useEffect"]({
        "ClientPump.useEffect": ()=>{
            if (!result?.errors) return;
            const mainError = result.errors[0]?.[0];
            if (!mainError) return;
            console.error(`Error fetching data from the BaseHub Draft API: ${mainError.message}${mainError.path ? ` at ${mainError.path.join(".")}` : ""}`);
        }
    }["ClientPump.useEffect"], [
        result?.errors
    ]);
    __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useEffect"]({
        "ClientPump.useEffect": ()=>{
            function boundRefetch() {
                refetch();
            }
            boundRefetch();
            subscribers.add(boundRefetch);
            return ({
                "ClientPump.useEffect": ()=>{
                    subscribers.delete(boundRefetch);
                }
            })["ClientPump.useEffect"];
        }
    }["ClientPump.useEffect"], [
        refetch
    ]);
    const [pusher, setPusher] = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useState"](null);
    const pusherChannelKey = result?.pusherData?.channel_key;
    const pusherAppKey = result?.pusherData.app_key;
    const pusherCluster = result?.pusherData.cluster;
    __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useEffect"]({
        "ClientPump.useEffect": ()=>{
            if (pusherMounted) return;
            if (!pusherAppKey || !pusherCluster) return;
            pusherMounted = true;
            __turbopack_context__.A("[project]/node_modules/.bun/pusher-js@8.4.0/node_modules/pusher-js/dist/web/pusher.js [app-client] (ecmascript, async loader)").then({
                "ClientPump.useEffect": (mod)=>{
                    setPusher(new mod.default(pusherAppKey, {
                        cluster: pusherCluster
                    }));
                }
            }["ClientPump.useEffect"]).catch({
                "ClientPump.useEffect": (err)=>{
                    console.log("error importing pusher");
                    console.error(err);
                }
            }["ClientPump.useEffect"]);
            return ({
                "ClientPump.useEffect": ()=>{
                    pusherMounted = false;
                }
            })["ClientPump.useEffect"];
        }
    }["ClientPump.useEffect"], [
        pusherAppKey,
        pusherCluster
    ]);
    __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useEffect"]({
        "ClientPump.useEffect": ()=>{
            if (!pusherChannelKey) return;
            if (!pusher) return;
            const channel = pusher.subscribe(pusherChannelKey);
            channel.bind("poke", {
                "ClientPump.useEffect": (message)=>{
                    if (message?.mutatedEntryTypes?.includes("block") && message.branch === previewRefRef.current) {
                        subscribers.forEach({
                            "ClientPump.useEffect": (sub)=>sub()
                        }["ClientPump.useEffect"]);
                    }
                }
            }["ClientPump.useEffect"]);
            return ({
                "ClientPump.useEffect": ()=>{
                    channel.unsubscribe();
                }
            })["ClientPump.useEffect"];
        }
    }["ClientPump.useEffect"], [
        pusher,
        pusherChannelKey
    ]);
    __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useEffect"]({
        "ClientPump.useEffect": ()=>{
            if (explicitRef) {
                return;
            }
            function handleRefChange() {
                const previewRef2 = // @ts-ignore
                window.__bshb_ref;
                if (!previewRef2 || typeof previewRef2 !== "string") return;
                setPreviewRef(previewRef2);
            }
            handleRefChange();
            window.addEventListener("__bshb_ref_changed", handleRefChange);
            return ({
                "ClientPump.useEffect": ()=>{
                    window.removeEventListener("__bshb_ref_changed", handleRefChange);
                }
            })["ClientPump.useEffect"];
        }
    }["ClientPump.useEffect"], [
        explicitRef
    ]);
    const resolvedData = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useMemo"]({
        "ClientPump.useMemo[resolvedData]": ()=>{
            return result?.data.map({
                "ClientPump.useMemo[resolvedData]": (r, i)=>r ?? initialState?.data?.[i] ?? null
            }["ClientPump.useMemo[resolvedData]"]);
        }
    }["ClientPump.useMemo[resolvedData]"], [
        initialState?.data,
        result?.data
    ]);
    const [resolvedChildren, setResolvedChildren] = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useState"](typeof childrenRef.current === "function" ? // if function, we'll resolve in React.useEffect below
    initialResolvedChildren : childrenRef.current);
    __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useEffect"]({
        "ClientPump.useEffect": ()=>{
            if (!resolvedData) return;
            if (typeof childrenRef.current === "function") {
                const res = childrenRef.current(resolvedData);
                if (res instanceof Promise) {
                    res.then(setResolvedChildren);
                } else {
                    setResolvedChildren(res);
                }
            } else {
                setResolvedChildren(childrenRef.current);
            }
        }
    }["ClientPump.useEffect"], [
        resolvedData,
        // keep this dep so next.js fast-refresh works OK
        children
    ]);
    return resolvedChildren ?? initialResolvedChildren;
};
;
}),
"[project]/node_modules/.bun/@hugeicons+core-free-icons@4.0.0/node_modules/@hugeicons/core-free-icons/dist/esm/ArrowLeft01Icon.js [app-client] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "default",
    ()=>ArrowLeft01Icon
]);
const ArrowLeft01Icon = /*#__PURE__*/ [
    [
        "path",
        {
            d: "M15 6C15 6 9.00001 10.4189 9 12C8.99999 13.5812 15 18 15 18",
            stroke: "currentColor",
            strokeLinecap: "round",
            strokeLinejoin: "round",
            strokeWidth: "1.5",
            key: "0"
        }
    ]
];
;
 //# sourceMappingURL=ArrowLeft01Icon.js.map
}),
"[project]/node_modules/.bun/@hugeicons+core-free-icons@4.0.0/node_modules/@hugeicons/core-free-icons/dist/esm/ArrowLeft01Icon.js [app-client] (ecmascript) <export default as ArrowLeft01Icon>", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "ArrowLeft01Icon",
    ()=>__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$hugeicons$2b$core$2d$free$2d$icons$40$4$2e$0$2e$0$2f$node_modules$2f40$hugeicons$2f$core$2d$free$2d$icons$2f$dist$2f$esm$2f$ArrowLeft01Icon$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["default"]
]);
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$hugeicons$2b$core$2d$free$2d$icons$40$4$2e$0$2e$0$2f$node_modules$2f40$hugeicons$2f$core$2d$free$2d$icons$2f$dist$2f$esm$2f$ArrowLeft01Icon$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@hugeicons+core-free-icons@4.0.0/node_modules/@hugeicons/core-free-icons/dist/esm/ArrowLeft01Icon.js [app-client] (ecmascript)");
}),
"[project]/node_modules/.bun/@hugeicons+core-free-icons@4.0.0/node_modules/@hugeicons/core-free-icons/dist/esm/ArrowRight01Icon.js [app-client] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "default",
    ()=>ArrowRight01Icon
]);
const ArrowRight01Icon = /*#__PURE__*/ [
    [
        "path",
        {
            d: "M9.00005 6C9.00005 6 15 10.4189 15 12C15 13.5812 9 18 9 18",
            stroke: "currentColor",
            strokeLinecap: "round",
            strokeLinejoin: "round",
            strokeWidth: "1.5",
            key: "0"
        }
    ]
];
;
 //# sourceMappingURL=ArrowRight01Icon.js.map
}),
"[project]/node_modules/.bun/@hugeicons+core-free-icons@4.0.0/node_modules/@hugeicons/core-free-icons/dist/esm/ArrowRight01Icon.js [app-client] (ecmascript) <export default as ArrowRight01Icon>", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "ArrowRight01Icon",
    ()=>__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$hugeicons$2b$core$2d$free$2d$icons$40$4$2e$0$2e$0$2f$node_modules$2f40$hugeicons$2f$core$2d$free$2d$icons$2f$dist$2f$esm$2f$ArrowRight01Icon$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["default"]
]);
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$hugeicons$2b$core$2d$free$2d$icons$40$4$2e$0$2e$0$2f$node_modules$2f40$hugeicons$2f$core$2d$free$2d$icons$2f$dist$2f$esm$2f$ArrowRight01Icon$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@hugeicons+core-free-icons@4.0.0/node_modules/@hugeicons/core-free-icons/dist/esm/ArrowRight01Icon.js [app-client] (ecmascript)");
}),
"[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/navigation.js [app-client] (ecmascript)", ((__turbopack_context__, module, exports) => {

module.exports = __turbopack_context__.r("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/client/components/navigation.js [app-client] (ecmascript)");
}),
"[project]/node_modules/.bun/@hugeicons+core-free-icons@4.0.0/node_modules/@hugeicons/core-free-icons/dist/esm/Copy01Icon.js [app-client] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "default",
    ()=>Copy01Icon
]);
const Copy01Icon = /*#__PURE__*/ [
    [
        "path",
        {
            d: "M9 15C9 12.1716 9 10.7574 9.87868 9.87868C10.7574 9 12.1716 9 15 9L16 9C18.8284 9 20.2426 9 21.1213 9.87868C22 10.7574 22 12.1716 22 15V16C22 18.8284 22 20.2426 21.1213 21.1213C20.2426 22 18.8284 22 16 22H15C12.1716 22 10.7574 22 9.87868 21.1213C9 20.2426 9 18.8284 9 16L9 15Z",
            stroke: "currentColor",
            strokeLinecap: "round",
            strokeLinejoin: "round",
            strokeWidth: "1.5",
            key: "0"
        }
    ],
    [
        "path",
        {
            d: "M16.9999 9C16.9975 6.04291 16.9528 4.51121 16.092 3.46243C15.9258 3.25989 15.7401 3.07418 15.5376 2.90796C14.4312 2 12.7875 2 9.5 2C6.21252 2 4.56878 2 3.46243 2.90796C3.25989 3.07417 3.07418 3.25989 2.90796 3.46243C2 4.56878 2 6.21252 2 9.5C2 12.7875 2 14.4312 2.90796 15.5376C3.07417 15.7401 3.25989 15.9258 3.46243 16.092C4.51121 16.9528 6.04291 16.9975 9 16.9999",
            stroke: "currentColor",
            strokeLinecap: "round",
            strokeLinejoin: "round",
            strokeWidth: "1.5",
            key: "1"
        }
    ]
];
;
 //# sourceMappingURL=Copy01Icon.js.map
}),
"[project]/node_modules/.bun/@hugeicons+core-free-icons@4.0.0/node_modules/@hugeicons/core-free-icons/dist/esm/Copy01Icon.js [app-client] (ecmascript) <export default as Copy01Icon>", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "Copy01Icon",
    ()=>__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$hugeicons$2b$core$2d$free$2d$icons$40$4$2e$0$2e$0$2f$node_modules$2f40$hugeicons$2f$core$2d$free$2d$icons$2f$dist$2f$esm$2f$Copy01Icon$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["default"]
]);
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$hugeicons$2b$core$2d$free$2d$icons$40$4$2e$0$2e$0$2f$node_modules$2f40$hugeicons$2f$core$2d$free$2d$icons$2f$dist$2f$esm$2f$Copy01Icon$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@hugeicons+core-free-icons@4.0.0/node_modules/@hugeicons/core-free-icons/dist/esm/Copy01Icon.js [app-client] (ecmascript)");
}),
"[project]/node_modules/.bun/@hugeicons+core-free-icons@4.0.0/node_modules/@hugeicons/core-free-icons/dist/esm/Share01Icon.js [app-client] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "default",
    ()=>Share01Icon
]);
const Share01Icon = /*#__PURE__*/ [
    [
        "path",
        {
            d: "M9.39584 4.5H8.35417C5.40789 4.5 3.93475 4.5 3.01946 5.37868C2.10417 6.25736 2.10417 7.67157 2.10417 10.5V14.5C2.10417 17.3284 2.10417 18.7426 3.01946 19.6213C3.93475 20.5 5.40789 20.5 8.35417 20.5H12.5608C15.5071 20.5 16.9802 20.5 17.8955 19.6213C18.4885 19.052 18.6973 18.2579 18.7708 17",
            stroke: "currentColor",
            strokeLinecap: "round",
            strokeLinejoin: "round",
            strokeWidth: "1.5",
            key: "0"
        }
    ],
    [
        "path",
        {
            d: "M16.1667 7V3.85355C16.1667 3.65829 16.3316 3.5 16.535 3.5C16.6326 3.5 16.7263 3.53725 16.7954 3.60355L21.5275 8.14645C21.7634 8.37282 21.8958 8.67986 21.8958 9C21.8958 9.32014 21.7634 9.62718 21.5275 9.85355L16.7954 14.3964C16.7263 14.4628 16.6326 14.5 16.535 14.5C16.3316 14.5 16.1667 14.3417 16.1667 14.1464V11H13.1157C8.875 11 7.3125 14.5 7.3125 14.5V12C7.3125 9.23858 9.64435 7 12.5208 7H16.1667Z",
            stroke: "currentColor",
            strokeLinecap: "round",
            strokeLinejoin: "round",
            strokeWidth: "1.5",
            key: "1"
        }
    ]
];
;
 //# sourceMappingURL=Share01Icon.js.map
}),
"[project]/node_modules/.bun/@hugeicons+core-free-icons@4.0.0/node_modules/@hugeicons/core-free-icons/dist/esm/Share01Icon.js [app-client] (ecmascript) <export default as Share01Icon>", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "Share01Icon",
    ()=>__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$hugeicons$2b$core$2d$free$2d$icons$40$4$2e$0$2e$0$2f$node_modules$2f40$hugeicons$2f$core$2d$free$2d$icons$2f$dist$2f$esm$2f$Share01Icon$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["default"]
]);
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$hugeicons$2b$core$2d$free$2d$icons$40$4$2e$0$2e$0$2f$node_modules$2f40$hugeicons$2f$core$2d$free$2d$icons$2f$dist$2f$esm$2f$Share01Icon$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@hugeicons+core-free-icons@4.0.0/node_modules/@hugeicons/core-free-icons/dist/esm/Share01Icon.js [app-client] (ecmascript)");
}),
"[project]/node_modules/.bun/@hugeicons+core-free-icons@4.0.0/node_modules/@hugeicons/core-free-icons/dist/esm/Tick01Icon.js [app-client] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "default",
    ()=>Tick01Icon
]);
const Tick01Icon = /*#__PURE__*/ [
    [
        "path",
        {
            d: "M5 14.5C5 14.5 6.5 14.5 8.5 18C8.5 18 14.0588 8.83333 19 7",
            stroke: "currentColor",
            strokeLinecap: "round",
            strokeLinejoin: "round",
            strokeWidth: "1.5",
            key: "0"
        }
    ]
];
;
 //# sourceMappingURL=Tick01Icon.js.map
}),
"[project]/node_modules/.bun/@hugeicons+core-free-icons@4.0.0/node_modules/@hugeicons/core-free-icons/dist/esm/Tick01Icon.js [app-client] (ecmascript) <export default as Tick01Icon>", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "Tick01Icon",
    ()=>__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$hugeicons$2b$core$2d$free$2d$icons$40$4$2e$0$2e$0$2f$node_modules$2f40$hugeicons$2f$core$2d$free$2d$icons$2f$dist$2f$esm$2f$Tick01Icon$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["default"]
]);
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$hugeicons$2b$core$2d$free$2d$icons$40$4$2e$0$2e$0$2f$node_modules$2f40$hugeicons$2f$core$2d$free$2d$icons$2f$dist$2f$esm$2f$Tick01Icon$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@hugeicons+core-free-icons@4.0.0/node_modules/@hugeicons/core-free-icons/dist/esm/Tick01Icon.js [app-client] (ecmascript)");
}),
]);

//# sourceMappingURL=_32f3647f._.js.map