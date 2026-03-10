module.exports = [
"[externals]/next/dist/shared/lib/no-fallback-error.external.js [external] (next/dist/shared/lib/no-fallback-error.external.js, cjs)", ((__turbopack_context__, module, exports) => {

const mod = __turbopack_context__.x("next/dist/shared/lib/no-fallback-error.external.js", () => require("next/dist/shared/lib/no-fallback-error.external.js"));

module.exports = mod;
}),
"[project]/apps/website/src/app/global-error.tsx [app-rsc] (ecmascript, Next.js Server Component)", ((__turbopack_context__) => {

__turbopack_context__.n(__turbopack_context__.i("[project]/apps/website/src/app/global-error.tsx [app-rsc] (ecmascript)"));
}),
"[project]/apps/website/src/app/layout.tsx [app-rsc] (ecmascript, Next.js Server Component)", ((__turbopack_context__) => {

__turbopack_context__.n(__turbopack_context__.i("[project]/apps/website/src/app/layout.tsx [app-rsc] (ecmascript)"));
}),
"[project]/apps/website/src/app/not-found.tsx [app-rsc] (ecmascript, Next.js Server Component)", ((__turbopack_context__) => {

__turbopack_context__.n(__turbopack_context__.i("[project]/apps/website/src/app/not-found.tsx [app-rsc] (ecmascript)"));
}),
"[project]/apps/website/src/app/(landing)/layout.tsx [app-rsc] (ecmascript, Next.js Server Component)", ((__turbopack_context__) => {

__turbopack_context__.n(__turbopack_context__.i("[project]/apps/website/src/app/(landing)/layout.tsx [app-rsc] (ecmascript)"));
}),
"[project]/apps/website/src/app/(landing)/error.tsx [app-rsc] (ecmascript, Next.js Server Component)", ((__turbopack_context__) => {

__turbopack_context__.n(__turbopack_context__.i("[project]/apps/website/src/app/(landing)/error.tsx [app-rsc] (ecmascript)"));
}),
"[project]/packages/billing/src/products.ts [app-rsc] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "PLANS",
    ()=>PLANS,
    "formatPrice",
    ()=>formatPrice,
    "formatPriceWithCents",
    ()=>formatPriceWithCents
]);
const PLANS = {
    personal: {
        name: "Personal",
        description: "For individual writers and creators.",
        prices: {
            monthly: 1900,
            yearly: 19000
        },
        features: [
            "AI interview",
            "Multi-draft generation",
            "Style guidance",
            "Export options"
        ]
    },
    pro: {
        name: "Pro",
        description: "For teams and power users who need advanced workflows.",
        prices: {
            monthly: 4900,
            yearly: 49000
        },
        features: [
            "Everything in Personal",
            "Advanced editing tools",
            "Priority support",
            "Higher usage limits"
        ]
    }
};
function formatPrice(cents, currency = "USD") {
    return new Intl.NumberFormat("en-US", {
        style: "currency",
        currency,
        maximumFractionDigits: 0
    }).format(cents / 100);
}
function formatPriceWithCents(cents, currency = "USD") {
    return new Intl.NumberFormat("en-US", {
        style: "currency",
        currency,
        minimumFractionDigits: 2,
        maximumFractionDigits: 2
    }).format(cents / 100);
}
}),
"[project]/apps/website/src/app/(landing)/components/common/cta/cta.tsx [app-rsc] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "default",
    ()=>__TURBOPACK__default__export__
]);
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$server$2f$route$2d$modules$2f$app$2d$page$2f$vendored$2f$rsc$2f$react$2d$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/server/route-modules/app-page/vendored/rsc/react-jsx-dev-runtime.js [app-rsc] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$hugeicons$2b$core$2d$free$2d$icons$40$4$2e$0$2e$0$2f$node_modules$2f40$hugeicons$2f$core$2d$free$2d$icons$2f$dist$2f$esm$2f$ArrowRight02Icon$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__$3c$export__default__as__ArrowRight02Icon$3e$__ = __turbopack_context__.i("[project]/node_modules/.bun/@hugeicons+core-free-icons@4.0.0/node_modules/@hugeicons/core-free-icons/dist/esm/ArrowRight02Icon.js [app-rsc] (ecmascript) <export default as ArrowRight02Icon>");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$hugeicons$2b$react$40$1$2e$1$2e$6$2b$b1ab299f0a400331$2f$node_modules$2f40$hugeicons$2f$react$2f$dist$2f$esm$2f$HugeiconsIcon$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@hugeicons+react@1.1.6+b1ab299f0a400331/node_modules/@hugeicons/react/dist/esm/HugeiconsIcon.js [app-rsc] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$packages$2f$ui$2f$src$2f$components$2f$button$2e$tsx__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/packages/ui/src/components/button.tsx [app-rsc] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$client$2f$app$2d$dir$2f$link$2e$react$2d$server$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/client/app-dir/link.react-server.js [app-rsc] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$apps$2f$website$2f$src$2f$lib$2f$urls$2e$ts__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/apps/website/src/lib/urls.ts [app-rsc] (ecmascript)");
;
;
;
;
;
;
const CTA = ()=>{
    const title = "Ready to run every workflow from one place?";
    const description = "Give your team a cleaner way to launch work, monitor progress, and recover fast when runs fail.";
    const buttonText = "Start with Strait";
    const buttonHref = "/login";
    const subtext = "Built for modern engineering teams that want fewer moving parts and faster recovery.";
    const headingId = "cta-title";
    return /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$server$2f$route$2d$modules$2f$app$2d$page$2f$vendored$2f$rsc$2f$react$2d$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["jsxDEV"])("section", {
        "aria-labelledby": headingId,
        className: "relative bg-primary py-20 sm:py-28",
        children: [
            /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$server$2f$route$2d$modules$2f$app$2d$page$2f$vendored$2f$rsc$2f$react$2d$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["jsxDEV"])("div", {
                className: "showcase-dots pointer-events-none absolute inset-0"
            }, void 0, false, {
                fileName: "[project]/apps/website/src/app/(landing)/components/common/cta/cta.tsx",
                lineNumber: 24,
                columnNumber: 7
            }, ("TURBOPACK compile-time value", void 0)),
            /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$server$2f$route$2d$modules$2f$app$2d$page$2f$vendored$2f$rsc$2f$react$2d$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["jsxDEV"])("div", {
                className: "pointer-events-none absolute inset-0 opacity-30",
                style: {
                    background: "radial-gradient(circle at 50% 40%, oklch(1 0 0 / 0.15), transparent 60%)"
                }
            }, void 0, false, {
                fileName: "[project]/apps/website/src/app/(landing)/components/common/cta/cta.tsx",
                lineNumber: 25,
                columnNumber: 7
            }, ("TURBOPACK compile-time value", void 0)),
            /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$server$2f$route$2d$modules$2f$app$2d$page$2f$vendored$2f$rsc$2f$react$2d$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["jsxDEV"])("div", {
                className: "relative z-10 mx-auto max-w-[1600px] px-4 sm:px-6 lg:px-8",
                children: /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$server$2f$route$2d$modules$2f$app$2d$page$2f$vendored$2f$rsc$2f$react$2d$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["jsxDEV"])("div", {
                    className: "flex flex-col items-center text-center",
                    children: [
                        /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$server$2f$route$2d$modules$2f$app$2d$page$2f$vendored$2f$rsc$2f$react$2d$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["jsxDEV"])("h2", {
                            className: "max-w-3xl font-bold text-3xl text-primary-foreground leading-[1.1] tracking-tighter sm:text-4xl lg:text-5xl",
                            id: headingId,
                            children: [
                                title,
                                /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$server$2f$route$2d$modules$2f$app$2d$page$2f$vendored$2f$rsc$2f$react$2d$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["jsxDEV"])("span", {
                                    "aria-hidden": "true",
                                    className: "ml-1 inline-block h-[0.6em] w-[3px] animate-pulse bg-primary-foreground/60 align-baseline"
                                }, void 0, false, {
                                    fileName: "[project]/apps/website/src/app/(landing)/components/common/cta/cta.tsx",
                                    lineNumber: 40,
                                    columnNumber: 13
                                }, ("TURBOPACK compile-time value", void 0))
                            ]
                        }, void 0, true, {
                            fileName: "[project]/apps/website/src/app/(landing)/components/common/cta/cta.tsx",
                            lineNumber: 35,
                            columnNumber: 11
                        }, ("TURBOPACK compile-time value", void 0)),
                        /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$server$2f$route$2d$modules$2f$app$2d$page$2f$vendored$2f$rsc$2f$react$2d$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["jsxDEV"])("p", {
                            className: "mt-6 max-w-2xl text-base text-primary-foreground/70 leading-relaxed sm:text-lg",
                            children: description
                        }, void 0, false, {
                            fileName: "[project]/apps/website/src/app/(landing)/components/common/cta/cta.tsx",
                            lineNumber: 46,
                            columnNumber: 11
                        }, ("TURBOPACK compile-time value", void 0)),
                        /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$server$2f$route$2d$modules$2f$app$2d$page$2f$vendored$2f$rsc$2f$react$2d$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["jsxDEV"])("div", {
                            className: "mt-10 flex flex-col items-center gap-4",
                            children: [
                                /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$server$2f$route$2d$modules$2f$app$2d$page$2f$vendored$2f$rsc$2f$react$2d$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["jsxDEV"])(__TURBOPACK__imported__module__$5b$project$5d2f$packages$2f$ui$2f$src$2f$components$2f$button$2e$tsx__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["Button"], {
                                    className: "bg-primary-foreground text-primary shadow-lg transition-all duration-300 hover:bg-primary-foreground/90 hover:shadow-xl",
                                    render: /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$server$2f$route$2d$modules$2f$app$2d$page$2f$vendored$2f$rsc$2f$react$2d$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["jsxDEV"])(__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$client$2f$app$2d$dir$2f$link$2e$react$2d$server$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["default"], {
                                        href: (0, __TURBOPACK__imported__module__$5b$project$5d2f$apps$2f$website$2f$src$2f$lib$2f$urls$2e$ts__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["dashboardHref"])(buttonHref)
                                    }, void 0, false, {
                                        fileName: "[project]/apps/website/src/app/(landing)/components/common/cta/cta.tsx",
                                        lineNumber: 53,
                                        columnNumber: 23
                                    }, void 0),
                                    size: "lg",
                                    children: [
                                        buttonText,
                                        /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$server$2f$route$2d$modules$2f$app$2d$page$2f$vendored$2f$rsc$2f$react$2d$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["jsxDEV"])(__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$hugeicons$2b$react$40$1$2e$1$2e$6$2b$b1ab299f0a400331$2f$node_modules$2f40$hugeicons$2f$react$2f$dist$2f$esm$2f$HugeiconsIcon$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["HugeiconsIcon"], {
                                            className: "size-4",
                                            icon: __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$hugeicons$2b$core$2d$free$2d$icons$40$4$2e$0$2e$0$2f$node_modules$2f40$hugeicons$2f$core$2d$free$2d$icons$2f$dist$2f$esm$2f$ArrowRight02Icon$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__$3c$export__default__as__ArrowRight02Icon$3e$__["ArrowRight02Icon"]
                                        }, void 0, false, {
                                            fileName: "[project]/apps/website/src/app/(landing)/components/common/cta/cta.tsx",
                                            lineNumber: 57,
                                            columnNumber: 15
                                        }, ("TURBOPACK compile-time value", void 0))
                                    ]
                                }, void 0, true, {
                                    fileName: "[project]/apps/website/src/app/(landing)/components/common/cta/cta.tsx",
                                    lineNumber: 51,
                                    columnNumber: 13
                                }, ("TURBOPACK compile-time value", void 0)),
                                /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$server$2f$route$2d$modules$2f$app$2d$page$2f$vendored$2f$rsc$2f$react$2d$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["jsxDEV"])("p", {
                                    className: "text-primary-foreground/50 text-sm",
                                    children: subtext
                                }, void 0, false, {
                                    fileName: "[project]/apps/website/src/app/(landing)/components/common/cta/cta.tsx",
                                    lineNumber: 59,
                                    columnNumber: 13
                                }, ("TURBOPACK compile-time value", void 0))
                            ]
                        }, void 0, true, {
                            fileName: "[project]/apps/website/src/app/(landing)/components/common/cta/cta.tsx",
                            lineNumber: 50,
                            columnNumber: 11
                        }, ("TURBOPACK compile-time value", void 0))
                    ]
                }, void 0, true, {
                    fileName: "[project]/apps/website/src/app/(landing)/components/common/cta/cta.tsx",
                    lineNumber: 34,
                    columnNumber: 9
                }, ("TURBOPACK compile-time value", void 0))
            }, void 0, false, {
                fileName: "[project]/apps/website/src/app/(landing)/components/common/cta/cta.tsx",
                lineNumber: 33,
                columnNumber: 7
            }, ("TURBOPACK compile-time value", void 0))
        ]
    }, void 0, true, {
        fileName: "[project]/apps/website/src/app/(landing)/components/common/cta/cta.tsx",
        lineNumber: 20,
        columnNumber: 5
    }, ("TURBOPACK compile-time value", void 0));
};
const __TURBOPACK__default__export__ = CTA;
}),
"[project]/apps/website/src/app/(landing)/components/pricing/static-pricing-table.tsx [app-rsc] (client reference proxy) <module evaluation>", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "StaticPricingTable",
    ()=>StaticPricingTable
]);
// This file is generated by next-core EcmascriptClientReferenceModule.
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$server$2f$route$2d$modules$2f$app$2d$page$2f$vendored$2f$rsc$2f$react$2d$server$2d$dom$2d$turbopack$2d$server$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/server/route-modules/app-page/vendored/rsc/react-server-dom-turbopack-server.js [app-rsc] (ecmascript)");
;
const StaticPricingTable = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$server$2f$route$2d$modules$2f$app$2d$page$2f$vendored$2f$rsc$2f$react$2d$server$2d$dom$2d$turbopack$2d$server$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["registerClientReference"])(function() {
    throw new Error("Attempted to call StaticPricingTable() from the server but StaticPricingTable is on the client. It's not possible to invoke a client function from the server, it can only be rendered as a Component or passed to props of a Client Component.");
}, "[project]/apps/website/src/app/(landing)/components/pricing/static-pricing-table.tsx <module evaluation>", "StaticPricingTable");
}),
"[project]/apps/website/src/app/(landing)/components/pricing/static-pricing-table.tsx [app-rsc] (client reference proxy)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "StaticPricingTable",
    ()=>StaticPricingTable
]);
// This file is generated by next-core EcmascriptClientReferenceModule.
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$server$2f$route$2d$modules$2f$app$2d$page$2f$vendored$2f$rsc$2f$react$2d$server$2d$dom$2d$turbopack$2d$server$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/server/route-modules/app-page/vendored/rsc/react-server-dom-turbopack-server.js [app-rsc] (ecmascript)");
;
const StaticPricingTable = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$server$2f$route$2d$modules$2f$app$2d$page$2f$vendored$2f$rsc$2f$react$2d$server$2d$dom$2d$turbopack$2d$server$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["registerClientReference"])(function() {
    throw new Error("Attempted to call StaticPricingTable() from the server but StaticPricingTable is on the client. It's not possible to invoke a client function from the server, it can only be rendered as a Component or passed to props of a Client Component.");
}, "[project]/apps/website/src/app/(landing)/components/pricing/static-pricing-table.tsx", "StaticPricingTable");
}),
"[project]/apps/website/src/app/(landing)/components/pricing/static-pricing-table.tsx [app-rsc] (ecmascript)", ((__turbopack_context__) => {
"use strict";

var __TURBOPACK__imported__module__$5b$project$5d2f$apps$2f$website$2f$src$2f$app$2f28$landing$292f$components$2f$pricing$2f$static$2d$pricing$2d$table$2e$tsx__$5b$app$2d$rsc$5d$__$28$client__reference__proxy$29$__$3c$module__evaluation$3e$__ = __turbopack_context__.i("[project]/apps/website/src/app/(landing)/components/pricing/static-pricing-table.tsx [app-rsc] (client reference proxy) <module evaluation>");
var __TURBOPACK__imported__module__$5b$project$5d2f$apps$2f$website$2f$src$2f$app$2f28$landing$292f$components$2f$pricing$2f$static$2d$pricing$2d$table$2e$tsx__$5b$app$2d$rsc$5d$__$28$client__reference__proxy$29$__ = __turbopack_context__.i("[project]/apps/website/src/app/(landing)/components/pricing/static-pricing-table.tsx [app-rsc] (client reference proxy)");
;
__turbopack_context__.n(__TURBOPACK__imported__module__$5b$project$5d2f$apps$2f$website$2f$src$2f$app$2f28$landing$292f$components$2f$pricing$2f$static$2d$pricing$2d$table$2e$tsx__$5b$app$2d$rsc$5d$__$28$client__reference__proxy$29$__);
}),
"[project]/apps/website/src/components/pricing/pricing-faq.client.tsx [app-rsc] (client reference proxy) <module evaluation>", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "default",
    ()=>__TURBOPACK__default__export__
]);
// This file is generated by next-core EcmascriptClientReferenceModule.
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$server$2f$route$2d$modules$2f$app$2d$page$2f$vendored$2f$rsc$2f$react$2d$server$2d$dom$2d$turbopack$2d$server$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/server/route-modules/app-page/vendored/rsc/react-server-dom-turbopack-server.js [app-rsc] (ecmascript)");
;
const __TURBOPACK__default__export__ = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$server$2f$route$2d$modules$2f$app$2d$page$2f$vendored$2f$rsc$2f$react$2d$server$2d$dom$2d$turbopack$2d$server$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["registerClientReference"])(function() {
    throw new Error("Attempted to call the default export of [project]/apps/website/src/components/pricing/pricing-faq.client.tsx <module evaluation> from the server, but it's on the client. It's not possible to invoke a client function from the server, it can only be rendered as a Component or passed to props of a Client Component.");
}, "[project]/apps/website/src/components/pricing/pricing-faq.client.tsx <module evaluation>", "default");
}),
"[project]/apps/website/src/components/pricing/pricing-faq.client.tsx [app-rsc] (client reference proxy)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "default",
    ()=>__TURBOPACK__default__export__
]);
// This file is generated by next-core EcmascriptClientReferenceModule.
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$server$2f$route$2d$modules$2f$app$2d$page$2f$vendored$2f$rsc$2f$react$2d$server$2d$dom$2d$turbopack$2d$server$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/server/route-modules/app-page/vendored/rsc/react-server-dom-turbopack-server.js [app-rsc] (ecmascript)");
;
const __TURBOPACK__default__export__ = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$server$2f$route$2d$modules$2f$app$2d$page$2f$vendored$2f$rsc$2f$react$2d$server$2d$dom$2d$turbopack$2d$server$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["registerClientReference"])(function() {
    throw new Error("Attempted to call the default export of [project]/apps/website/src/components/pricing/pricing-faq.client.tsx from the server, but it's on the client. It's not possible to invoke a client function from the server, it can only be rendered as a Component or passed to props of a Client Component.");
}, "[project]/apps/website/src/components/pricing/pricing-faq.client.tsx", "default");
}),
"[project]/apps/website/src/components/pricing/pricing-faq.client.tsx [app-rsc] (ecmascript)", ((__turbopack_context__) => {
"use strict";

var __TURBOPACK__imported__module__$5b$project$5d2f$apps$2f$website$2f$src$2f$components$2f$pricing$2f$pricing$2d$faq$2e$client$2e$tsx__$5b$app$2d$rsc$5d$__$28$client__reference__proxy$29$__$3c$module__evaluation$3e$__ = __turbopack_context__.i("[project]/apps/website/src/components/pricing/pricing-faq.client.tsx [app-rsc] (client reference proxy) <module evaluation>");
var __TURBOPACK__imported__module__$5b$project$5d2f$apps$2f$website$2f$src$2f$components$2f$pricing$2f$pricing$2d$faq$2e$client$2e$tsx__$5b$app$2d$rsc$5d$__$28$client__reference__proxy$29$__ = __turbopack_context__.i("[project]/apps/website/src/components/pricing/pricing-faq.client.tsx [app-rsc] (client reference proxy)");
;
__turbopack_context__.n(__TURBOPACK__imported__module__$5b$project$5d2f$apps$2f$website$2f$src$2f$components$2f$pricing$2f$pricing$2d$faq$2e$client$2e$tsx__$5b$app$2d$rsc$5d$__$28$client__reference__proxy$29$__);
}),
"[project]/apps/website/src/components/pricing/pricing-faq.tsx [app-rsc] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "PRICING_FAQ_ITEMS",
    ()=>PRICING_FAQ_ITEMS,
    "default",
    ()=>__TURBOPACK__default__export__
]);
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$server$2f$route$2d$modules$2f$app$2d$page$2f$vendored$2f$rsc$2f$react$2d$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/server/route-modules/app-page/vendored/rsc/react-jsx-dev-runtime.js [app-rsc] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$apps$2f$website$2f$src$2f$components$2f$pricing$2f$pricing$2d$faq$2e$client$2e$tsx__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/apps/website/src/components/pricing/pricing-faq.client.tsx [app-rsc] (ecmascript)");
;
;
const PRICING_FAQ_ITEMS = [
    {
        _id: "faq-1",
        question: "Can I cancel anytime?",
        answer: "Yes. You can cancel your subscription at any time from your billing settings."
    },
    {
        _id: "faq-2",
        question: "Do you offer yearly billing?",
        answer: "Yes. Yearly billing gives you a discounted monthly effective rate compared to monthly billing."
    },
    {
        _id: "faq-3",
        question: "What happens if I hit plan limits?",
        answer: "You can upgrade at any time to unlock higher run volume and additional orchestration controls."
    },
    {
        _id: "faq-4",
        question: "Do both plans include core runtime capabilities?",
        answer: "Yes. Both plans include core job execution, workflow orchestration, and operational visibility features."
    }
];
const PricingFaq = ()=>{
    return /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$server$2f$route$2d$modules$2f$app$2d$page$2f$vendored$2f$rsc$2f$react$2d$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["jsxDEV"])(__TURBOPACK__imported__module__$5b$project$5d2f$apps$2f$website$2f$src$2f$components$2f$pricing$2f$pricing$2d$faq$2e$client$2e$tsx__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["default"], {
        badge: "FAQ",
        description: "Everything you need to know before choosing a plan.",
        items: PRICING_FAQ_ITEMS,
        title: "Frequently asked questions"
    }, void 0, false, {
        fileName: "[project]/apps/website/src/components/pricing/pricing-faq.tsx",
        lineNumber: 38,
        columnNumber: 5
    }, ("TURBOPACK compile-time value", void 0));
};
const __TURBOPACK__default__export__ = PricingFaq;
}),
"[project]/apps/website/src/lib/metadata.ts [app-rsc] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "generateMetadata",
    ()=>generateMetadata
]);
var __TURBOPACK__imported__module__$5b$project$5d2f$apps$2f$website$2f$src$2f$config$2f$site$2e$ts__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/apps/website/src/config/site.ts [app-rsc] (ecmascript)");
;
function generateMetadata({ title, description, path, noIndex = false, ogImage, appendSiteTitle = true, keywords, siteName, locale, ogTitle, ogDescription, twitterTitle, twitterDescription, article, canonical }) {
    const displayTitle = (()=>{
        if (!title) {
            return __TURBOPACK__imported__module__$5b$project$5d2f$apps$2f$website$2f$src$2f$config$2f$site$2e$ts__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["siteConfig"].title;
        }
        if (appendSiteTitle) {
            return `${title} — ${__TURBOPACK__imported__module__$5b$project$5d2f$apps$2f$website$2f$src$2f$config$2f$site$2e$ts__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["siteConfig"].title}`;
        }
        return title;
    })();
    const displayDescription = description ?? __TURBOPACK__imported__module__$5b$project$5d2f$apps$2f$website$2f$src$2f$config$2f$site$2e$ts__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["siteConfig"].description;
    const url = `${__TURBOPACK__imported__module__$5b$project$5d2f$apps$2f$website$2f$src$2f$config$2f$site$2e$ts__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["siteConfig"].url}${path}`;
    const displayImage = ogImage ?? __TURBOPACK__imported__module__$5b$project$5d2f$apps$2f$website$2f$src$2f$config$2f$site$2e$ts__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["siteConfig"].ogImage;
    const displayKeywords = keywords ?? __TURBOPACK__imported__module__$5b$project$5d2f$apps$2f$website$2f$src$2f$config$2f$site$2e$ts__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["siteConfig"].metadata.keywords;
    const displaySiteName = siteName ?? __TURBOPACK__imported__module__$5b$project$5d2f$apps$2f$website$2f$src$2f$config$2f$site$2e$ts__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["siteConfig"].metadata.siteName ?? __TURBOPACK__imported__module__$5b$project$5d2f$apps$2f$website$2f$src$2f$config$2f$site$2e$ts__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["siteConfig"].openGraph?.siteName ?? __TURBOPACK__imported__module__$5b$project$5d2f$apps$2f$website$2f$src$2f$config$2f$site$2e$ts__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["siteConfig"].title;
    const displayLocale = locale ?? __TURBOPACK__imported__module__$5b$project$5d2f$apps$2f$website$2f$src$2f$config$2f$site$2e$ts__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["siteConfig"].metadata.locale ?? __TURBOPACK__imported__module__$5b$project$5d2f$apps$2f$website$2f$src$2f$config$2f$site$2e$ts__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["siteConfig"].openGraph?.locale;
    const displayOgTitle = ogTitle ?? displayTitle;
    const displayOgDescription = ogDescription ?? displayDescription;
    const displayTwitterTitle = twitterTitle ?? displayOgTitle;
    const displayTwitterDescription = twitterDescription ?? displayOgDescription;
    const openGraphBase = {
        ...__TURBOPACK__imported__module__$5b$project$5d2f$apps$2f$website$2f$src$2f$config$2f$site$2e$ts__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["siteConfig"].openGraph,
        title: displayOgTitle,
        description: displayOgDescription,
        url,
        images: [
            {
                url: displayImage,
                width: 1200,
                height: 630,
                alt: displayOgTitle
            }
        ],
        siteName: displaySiteName,
        locale: displayLocale
    };
    const openGraph = article ? {
        ...openGraphBase,
        type: "article",
        publishedTime: article.publishedTime,
        ...article.modifiedTime && {
            modifiedTime: article.modifiedTime
        },
        ...article.authors && {
            authors: article.authors
        },
        ...article.section && {
            section: article.section
        },
        ...article.tags && {
            tags: article.tags
        }
    } : openGraphBase;
    const displayCanonical = canonical ?? url;
    return {
        title: displayTitle,
        description: displayDescription,
        keywords: displayKeywords,
        metadataBase: new URL(__TURBOPACK__imported__module__$5b$project$5d2f$apps$2f$website$2f$src$2f$config$2f$site$2e$ts__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["siteConfig"].url),
        alternates: {
            canonical: displayCanonical
        },
        openGraph,
        twitter: {
            ...__TURBOPACK__imported__module__$5b$project$5d2f$apps$2f$website$2f$src$2f$config$2f$site$2e$ts__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["siteConfig"].twitter,
            title: displayTwitterTitle,
            description: displayTwitterDescription,
            images: [
                displayImage
            ]
        },
        icons: __TURBOPACK__imported__module__$5b$project$5d2f$apps$2f$website$2f$src$2f$config$2f$site$2e$ts__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["siteConfig"].icons,
        manifest: __TURBOPACK__imported__module__$5b$project$5d2f$apps$2f$website$2f$src$2f$config$2f$site$2e$ts__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["siteConfig"].manifest,
        robots: {
            index: !noIndex,
            follow: !noIndex
        }
    };
}
}),
"[project]/apps/website/src/lib/structured-data.tsx [app-rsc] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "JsonLd",
    ()=>JsonLd,
    "JsonLdMultiple",
    ()=>JsonLdMultiple,
    "getBlogPostingSchema",
    ()=>getBlogPostingSchema,
    "getBreadcrumbSchema",
    ()=>getBreadcrumbSchema,
    "getCollectionPageSchema",
    ()=>getCollectionPageSchema,
    "getFAQPageSchema",
    ()=>getFAQPageSchema,
    "getHowToSchema",
    ()=>getHowToSchema,
    "getOrganizationSchema",
    ()=>getOrganizationSchema,
    "getPersonSchema",
    ()=>getPersonSchema,
    "getPricingProductsSchema",
    ()=>getPricingProductsSchema,
    "getProductSchema",
    ()=>getProductSchema,
    "getSoftwareApplicationSchema",
    ()=>getSoftwareApplicationSchema,
    "getWebSiteSchema",
    ()=>getWebSiteSchema
]);
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$server$2f$route$2d$modules$2f$app$2d$page$2f$vendored$2f$rsc$2f$react$2d$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/server/route-modules/app-page/vendored/rsc/react-jsx-dev-runtime.js [app-rsc] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$packages$2f$billing$2f$src$2f$products$2e$ts__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/packages/billing/src/products.ts [app-rsc] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$apps$2f$website$2f$src$2f$config$2f$site$2e$ts__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/apps/website/src/config/site.ts [app-rsc] (ecmascript)");
;
;
;
const BASE_URL = process.env.NEXT_PUBLIC_WEBSITE_URL || "https://trystrait.ai";
const LOGO_URL = `${BASE_URL}/android-chrome-512x512.png`;
function getOrganizationSchema() {
    return {
        "@context": "https://schema.org",
        "@type": "Organization",
        "@id": `${BASE_URL}/#organization`,
        name: "Strait",
        url: BASE_URL,
        logo: {
            "@type": "ImageObject",
            url: LOGO_URL,
            width: 512,
            height: 512
        },
        description: __TURBOPACK__imported__module__$5b$project$5d2f$apps$2f$website$2f$src$2f$config$2f$site$2e$ts__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["siteConfig"].description,
        sameAs: [
            __TURBOPACK__imported__module__$5b$project$5d2f$apps$2f$website$2f$src$2f$config$2f$site$2e$ts__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["siteConfig"].links.twitter,
            __TURBOPACK__imported__module__$5b$project$5d2f$apps$2f$website$2f$src$2f$config$2f$site$2e$ts__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["siteConfig"].links.linkedin,
            __TURBOPACK__imported__module__$5b$project$5d2f$apps$2f$website$2f$src$2f$config$2f$site$2e$ts__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["siteConfig"].links.instagram,
            __TURBOPACK__imported__module__$5b$project$5d2f$apps$2f$website$2f$src$2f$config$2f$site$2e$ts__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["siteConfig"].links.github
        ].filter(Boolean)
    };
}
function getWebSiteSchema() {
    return {
        "@context": "https://schema.org",
        "@type": "WebSite",
        "@id": `${BASE_URL}/#website`,
        name: "Strait",
        url: BASE_URL,
        description: __TURBOPACK__imported__module__$5b$project$5d2f$apps$2f$website$2f$src$2f$config$2f$site$2e$ts__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["siteConfig"].description,
        publisher: {
            "@id": `${BASE_URL}/#organization`
        },
        potentialAction: {
            "@type": "SearchAction",
            target: {
                "@type": "EntryPoint",
                urlTemplate: `${BASE_URL}/blog?search={search_term_string}`
            },
            "query-input": "required name=search_term_string"
        }
    };
}
function getBlogPostingSchema(article) {
    const authors = article.authors.map((author)=>({
            "@type": "Person",
            name: author.name,
            ...author.url && {
                url: author.url
            },
            ...author.image && {
                image: {
                    "@type": "ImageObject",
                    url: author.image
                }
            }
        }));
    return {
        "@context": "https://schema.org",
        "@type": "BlogPosting",
        "@id": `${article.url}#article`,
        headline: article.headline,
        description: article.description,
        image: {
            "@type": "ImageObject",
            url: article.image,
            width: 1200,
            height: 630
        },
        datePublished: article.datePublished,
        ...article.dateModified && {
            dateModified: article.dateModified
        },
        author: authors.length === 1 ? authors[0] : authors,
        publisher: {
            "@type": "Organization",
            "@id": `${BASE_URL}/#organization`,
            name: "Strait",
            logo: {
                "@type": "ImageObject",
                url: LOGO_URL,
                width: 512,
                height: 512
            }
        },
        mainEntityOfPage: {
            "@type": "WebPage",
            "@id": article.url
        },
        isPartOf: {
            "@id": `${BASE_URL}/#website`
        },
        ...article.section && {
            articleSection: article.section
        },
        ...article.keywords && article.keywords.length > 0 && {
            keywords: article.keywords.join(", ")
        },
        ...article.wordCount && {
            wordCount: article.wordCount
        },
        inLanguage: "en-US"
    };
}
function getBreadcrumbSchema(items) {
    return {
        "@context": "https://schema.org",
        "@type": "BreadcrumbList",
        itemListElement: items.map((item, index)=>({
                "@type": "ListItem",
                position: index + 1,
                name: item.name,
                item: item.url
            }))
    };
}
function getPersonSchema(person) {
    return {
        "@context": "https://schema.org",
        "@type": "Person",
        "@id": `${person.url}#person`,
        name: person.name,
        url: person.url,
        ...person.image && {
            image: {
                "@type": "ImageObject",
                url: person.image
            }
        },
        ...person.jobTitle && {
            jobTitle: person.jobTitle
        },
        ...person.worksFor && {
            worksFor: {
                "@type": "Organization",
                "@id": `${BASE_URL}/#organization`,
                name: person.worksFor
            }
        },
        ...person.description && {
            description: person.description
        },
        ...person.sameAs && person.sameAs.length > 0 && {
            sameAs: person.sameAs
        }
    };
}
function getCollectionPageSchema(options) {
    return {
        "@context": "https://schema.org",
        "@type": "CollectionPage",
        "@id": `${options.url}#webpage`,
        name: options.name,
        description: options.description,
        url: options.url,
        isPartOf: {
            "@id": `${BASE_URL}/#website`
        },
        about: {
            "@id": `${BASE_URL}/#organization`
        },
        inLanguage: "en-US"
    };
}
function getSoftwareApplicationSchema() {
    const offers = [
        {
            "@type": "Offer",
            name: "Personal",
            price: (__TURBOPACK__imported__module__$5b$project$5d2f$packages$2f$billing$2f$src$2f$products$2e$ts__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["PLANS"].personal.prices.monthly / 100).toFixed(2),
            priceCurrency: "USD",
            priceValidUntil: new Date(Date.now() + 365 * 24 * 60 * 60 * 1000).toISOString(),
            availability: "https://schema.org/InStock",
            url: `${BASE_URL}/pricing`,
            description: __TURBOPACK__imported__module__$5b$project$5d2f$packages$2f$billing$2f$src$2f$products$2e$ts__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["PLANS"].personal.description
        },
        {
            "@type": "Offer",
            name: "Pro",
            price: (__TURBOPACK__imported__module__$5b$project$5d2f$packages$2f$billing$2f$src$2f$products$2e$ts__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["PLANS"].pro.prices.monthly / 100).toFixed(2),
            priceCurrency: "USD",
            priceValidUntil: new Date(Date.now() + 365 * 24 * 60 * 60 * 1000).toISOString(),
            availability: "https://schema.org/InStock",
            url: `${BASE_URL}/pricing`,
            description: __TURBOPACK__imported__module__$5b$project$5d2f$packages$2f$billing$2f$src$2f$products$2e$ts__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["PLANS"].pro.description
        }
    ];
    return {
        "@context": "https://schema.org",
        "@type": "SoftwareApplication",
        "@id": `${BASE_URL}/#software`,
        name: "Strait",
        description: __TURBOPACK__imported__module__$5b$project$5d2f$apps$2f$website$2f$src$2f$config$2f$site$2e$ts__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["siteConfig"].description,
        url: BASE_URL,
        applicationCategory: "ProductivityApplication",
        operatingSystem: "Web",
        browserRequirements: "Requires Chrome 90+, Firefox 88+, Safari 14+, or Edge 90+",
        softwareVersion: "2026.1",
        author: {
            "@id": `${BASE_URL}/#organization`
        },
        provider: {
            "@id": `${BASE_URL}/#organization`
        },
        offers,
        featureList: [
            "PostgreSQL queue with SKIP LOCKED",
            "13-state run lifecycle management",
            "Workflow DAG orchestration",
            "Step conditions and approval gates",
            "Retry strategies with jitter",
            "Dead letter queue and replay",
            "SDK endpoints for run telemetry",
            "Debug bundles and execution tracing",
            "Cost budget enforcement",
            "API, CLI, and TUI operations"
        ],
        screenshot: {
            "@type": "ImageObject",
            url: `${BASE_URL}/opengraph-image.jpg`,
            width: 1200,
            height: 630
        }
    };
}
function getFAQPageSchema(items) {
    // Filter out items with null or empty answers
    const validItems = items.filter((item)=>item.answer && item.answer.trim().length > 0);
    // Return null if fewer than 3 valid Q&A pairs (Google requirement)
    if (validItems.length < 3) {
        return null;
    }
    return {
        "@context": "https://schema.org",
        "@type": "FAQPage",
        "@id": `${BASE_URL}/pricing#faq`,
        mainEntity: validItems.map((item)=>({
                "@type": "Question",
                name: item.question,
                acceptedAnswer: {
                    "@type": "Answer",
                    text: item.answer
                }
            }))
    };
}
function getProductSchema(plan) {
    return {
        "@context": "https://schema.org",
        "@type": "Product",
        "@id": `${BASE_URL}/pricing#${plan.slug}`,
        name: `Strait ${plan.name}`,
        description: plan.description,
        brand: {
            "@id": `${BASE_URL}/#organization`
        },
        offers: {
            "@type": "Offer",
            price: (plan.price / 100).toFixed(2),
            priceCurrency: "USD",
            priceValidUntil: new Date(Date.now() + 365 * 24 * 60 * 60 * 1000).toISOString(),
            availability: "https://schema.org/InStock",
            url: `${BASE_URL}/pricing`,
            seller: {
                "@id": `${BASE_URL}/#organization`
            }
        },
        category: "Workflow Orchestration Software"
    };
}
function getHowToSchema(steps) {
    if (steps.length === 0) {
        return null;
    }
    return {
        "@context": "https://schema.org",
        "@type": "HowTo",
        "@id": `${BASE_URL}/#howto`,
        name: "How to Get Started with Strait",
        description: "Learn how to get started with Strait job orchestration in three simple steps.",
        totalTime: "PT10M",
        estimatedCost: {
            "@type": "MonetaryAmount",
            currency: "USD",
            value: "0"
        },
        step: steps.map((step, index)=>({
                "@type": "HowToStep",
                position: index + 1,
                name: step.title,
                text: step.description,
                url: `${BASE_URL}/#step-${index + 1}`
            })),
        tool: [
            {
                "@type": "HowToTool",
                name: "Web browser"
            },
            {
                "@type": "HowToTool",
                name: "Strait API or CLI"
            }
        ]
    };
}
function getPricingProductsSchema() {
    const plans = [
        {
            name: "Personal",
            description: __TURBOPACK__imported__module__$5b$project$5d2f$packages$2f$billing$2f$src$2f$products$2e$ts__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["PLANS"].personal.description,
            price: __TURBOPACK__imported__module__$5b$project$5d2f$packages$2f$billing$2f$src$2f$products$2e$ts__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["PLANS"].personal.prices.monthly,
            slug: "personal"
        },
        {
            name: "Pro",
            description: __TURBOPACK__imported__module__$5b$project$5d2f$packages$2f$billing$2f$src$2f$products$2e$ts__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["PLANS"].pro.description,
            price: __TURBOPACK__imported__module__$5b$project$5d2f$packages$2f$billing$2f$src$2f$products$2e$ts__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["PLANS"].pro.prices.monthly,
            slug: "pro"
        }
    ];
    return plans.map((plan)=>getProductSchema(plan));
}
function JsonLd({ data }) {
    return /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$server$2f$route$2d$modules$2f$app$2d$page$2f$vendored$2f$rsc$2f$react$2d$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["jsxDEV"])("script", {
        // biome-ignore lint/security/noDangerouslySetInnerHtml: JSON-LD requires dangerouslySetInnerHTML for structured data
        dangerouslySetInnerHTML: {
            __html: JSON.stringify(data)
        },
        type: "application/ld+json"
    }, void 0, false, {
        fileName: "[project]/apps/website/src/lib/structured-data.tsx",
        lineNumber: 402,
        columnNumber: 5
    }, this);
}
function JsonLdMultiple({ data }) {
    return /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$server$2f$route$2d$modules$2f$app$2d$page$2f$vendored$2f$rsc$2f$react$2d$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["jsxDEV"])("script", {
        // biome-ignore lint/security/noDangerouslySetInnerHtml: JSON-LD requires dangerouslySetInnerHTML for structured data
        dangerouslySetInnerHTML: {
            __html: JSON.stringify(data)
        },
        type: "application/ld+json"
    }, void 0, false, {
        fileName: "[project]/apps/website/src/lib/structured-data.tsx",
        lineNumber: 416,
        columnNumber: 5
    }, this);
}
}),
"[project]/apps/website/src/app/(landing)/pricing/page.tsx [app-rsc] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "default",
    ()=>PricingPage,
    "metadata",
    ()=>metadata
]);
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$server$2f$route$2d$modules$2f$app$2d$page$2f$vendored$2f$rsc$2f$react$2d$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/server/route-modules/app-page/vendored/rsc/react-jsx-dev-runtime.js [app-rsc] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$packages$2f$billing$2f$src$2f$products$2e$ts__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/packages/billing/src/products.ts [app-rsc] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$server$2f$route$2d$modules$2f$app$2d$page$2f$vendored$2f$rsc$2f$react$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/server/route-modules/app-page/vendored/rsc/react.js [app-rsc] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$apps$2f$website$2f$src$2f$app$2f28$landing$292f$components$2f$common$2f$cta$2f$cta$2e$tsx__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/apps/website/src/app/(landing)/components/common/cta/cta.tsx [app-rsc] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$apps$2f$website$2f$src$2f$app$2f28$landing$292f$components$2f$pricing$2f$static$2d$pricing$2d$table$2e$tsx__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/apps/website/src/app/(landing)/components/pricing/static-pricing-table.tsx [app-rsc] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$apps$2f$website$2f$src$2f$components$2f$layout$2f$shell$2e$tsx__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/apps/website/src/components/layout/shell.tsx [app-rsc] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$apps$2f$website$2f$src$2f$components$2f$pricing$2f$pricing$2d$faq$2e$tsx__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/apps/website/src/components/pricing/pricing-faq.tsx [app-rsc] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$apps$2f$website$2f$src$2f$lib$2f$metadata$2e$ts__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/apps/website/src/lib/metadata.ts [app-rsc] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$apps$2f$website$2f$src$2f$lib$2f$structured$2d$data$2e$tsx__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/apps/website/src/lib/structured-data.tsx [app-rsc] (ecmascript)");
;
;
;
;
;
;
;
;
;
const metadata = (0, __TURBOPACK__imported__module__$5b$project$5d2f$apps$2f$website$2f$src$2f$lib$2f$metadata$2e$ts__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["generateMetadata"])({
    title: "Pricing",
    description: "Simple, transparent pricing for production-grade job orchestration. Two plans, no hidden fees, cancel anytime.",
    path: "/pricing",
    keywords: [
        "Strait pricing",
        "job orchestration pricing",
        "workflow platform plans",
        "background job platform subscription",
        "Strait plans"
    ]
});
function PricingPage() {
    const personalYearly = (0, __TURBOPACK__imported__module__$5b$project$5d2f$packages$2f$billing$2f$src$2f$products$2e$ts__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["formatPriceWithCents"])(__TURBOPACK__imported__module__$5b$project$5d2f$packages$2f$billing$2f$src$2f$products$2e$ts__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["PLANS"].personal.prices.yearly);
    const proYearly = (0, __TURBOPACK__imported__module__$5b$project$5d2f$packages$2f$billing$2f$src$2f$products$2e$ts__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["formatPriceWithCents"])(__TURBOPACK__imported__module__$5b$project$5d2f$packages$2f$billing$2f$src$2f$products$2e$ts__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["PLANS"].pro.prices.yearly);
    const softwareAppSchema = (0, __TURBOPACK__imported__module__$5b$project$5d2f$apps$2f$website$2f$src$2f$lib$2f$structured$2d$data$2e$tsx__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["getSoftwareApplicationSchema"])();
    const pricingProductsSchema = (0, __TURBOPACK__imported__module__$5b$project$5d2f$apps$2f$website$2f$src$2f$lib$2f$structured$2d$data$2e$tsx__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["getPricingProductsSchema"])();
    const faqSchema = (0, __TURBOPACK__imported__module__$5b$project$5d2f$apps$2f$website$2f$src$2f$lib$2f$structured$2d$data$2e$tsx__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["getFAQPageSchema"])(__TURBOPACK__imported__module__$5b$project$5d2f$apps$2f$website$2f$src$2f$components$2f$pricing$2f$pricing$2d$faq$2e$tsx__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["PRICING_FAQ_ITEMS"]);
    return /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$server$2f$route$2d$modules$2f$app$2d$page$2f$vendored$2f$rsc$2f$react$2d$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["jsxDEV"])("main", {
        className: "pt-32",
        children: [
            /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$server$2f$route$2d$modules$2f$app$2d$page$2f$vendored$2f$rsc$2f$react$2d$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["jsxDEV"])(__TURBOPACK__imported__module__$5b$project$5d2f$apps$2f$website$2f$src$2f$lib$2f$structured$2d$data$2e$tsx__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["JsonLd"], {
                data: softwareAppSchema
            }, void 0, false, {
                fileName: "[project]/apps/website/src/app/(landing)/pricing/page.tsx",
                lineNumber: 43,
                columnNumber: 7
            }, this),
            /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$server$2f$route$2d$modules$2f$app$2d$page$2f$vendored$2f$rsc$2f$react$2d$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["jsxDEV"])(__TURBOPACK__imported__module__$5b$project$5d2f$apps$2f$website$2f$src$2f$lib$2f$structured$2d$data$2e$tsx__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["JsonLdMultiple"], {
                data: pricingProductsSchema
            }, void 0, false, {
                fileName: "[project]/apps/website/src/app/(landing)/pricing/page.tsx",
                lineNumber: 44,
                columnNumber: 7
            }, this),
            faqSchema ? /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$server$2f$route$2d$modules$2f$app$2d$page$2f$vendored$2f$rsc$2f$react$2d$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["jsxDEV"])(__TURBOPACK__imported__module__$5b$project$5d2f$apps$2f$website$2f$src$2f$lib$2f$structured$2d$data$2e$tsx__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["JsonLd"], {
                data: faqSchema
            }, void 0, false, {
                fileName: "[project]/apps/website/src/app/(landing)/pricing/page.tsx",
                lineNumber: 45,
                columnNumber: 20
            }, this) : null,
            /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$server$2f$route$2d$modules$2f$app$2d$page$2f$vendored$2f$rsc$2f$react$2d$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["jsxDEV"])("section", {
                className: "relative isolate overflow-hidden pb-16 sm:pb-20",
                children: [
                    /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$server$2f$route$2d$modules$2f$app$2d$page$2f$vendored$2f$rsc$2f$react$2d$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["jsxDEV"])("div", {
                        className: "absolute inset-0 -z-10 bg-[linear-gradient(to_bottom,_var(--primary)/0.06,_transparent_40%)]"
                    }, void 0, false, {
                        fileName: "[project]/apps/website/src/app/(landing)/pricing/page.tsx",
                        lineNumber: 48,
                        columnNumber: 9
                    }, this),
                    /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$server$2f$route$2d$modules$2f$app$2d$page$2f$vendored$2f$rsc$2f$react$2d$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["jsxDEV"])("div", {
                        className: "absolute inset-0 -z-10 bg-[linear-gradient(to_bottom,_transparent,_var(--background)_70%)]"
                    }, void 0, false, {
                        fileName: "[project]/apps/website/src/app/(landing)/pricing/page.tsx",
                        lineNumber: 49,
                        columnNumber: 9
                    }, this),
                    /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$server$2f$route$2d$modules$2f$app$2d$page$2f$vendored$2f$rsc$2f$react$2d$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["jsxDEV"])("div", {
                        className: "paper-texture absolute inset-0 -z-10 opacity-[0.02]"
                    }, void 0, false, {
                        fileName: "[project]/apps/website/src/app/(landing)/pricing/page.tsx",
                        lineNumber: 50,
                        columnNumber: 9
                    }, this),
                    /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$server$2f$route$2d$modules$2f$app$2d$page$2f$vendored$2f$rsc$2f$react$2d$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["jsxDEV"])(__TURBOPACK__imported__module__$5b$project$5d2f$apps$2f$website$2f$src$2f$components$2f$layout$2f$shell$2e$tsx__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["default"], {
                        variant: "wide",
                        children: /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$server$2f$route$2d$modules$2f$app$2d$page$2f$vendored$2f$rsc$2f$react$2d$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["jsxDEV"])("div", {
                            className: "mx-auto max-w-3xl text-center",
                            children: [
                                /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$server$2f$route$2d$modules$2f$app$2d$page$2f$vendored$2f$rsc$2f$react$2d$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["jsxDEV"])("span", {
                                    className: "kicker",
                                    children: "Pricing"
                                }, void 0, false, {
                                    fileName: "[project]/apps/website/src/app/(landing)/pricing/page.tsx",
                                    lineNumber: 54,
                                    columnNumber: 13
                                }, this),
                                /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$server$2f$route$2d$modules$2f$app$2d$page$2f$vendored$2f$rsc$2f$react$2d$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["jsxDEV"])("h1", {
                                    className: "mt-6 text-balance text-4xl leading-[1.12] tracking-tight sm:text-5xl lg:text-6xl",
                                    children: [
                                        /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$server$2f$route$2d$modules$2f$app$2d$page$2f$vendored$2f$rsc$2f$react$2d$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["jsxDEV"])("span", {
                                            className: "font-bold text-foreground",
                                            children: "Simple pricing, built for reliable orchestration."
                                        }, void 0, false, {
                                            fileName: "[project]/apps/website/src/app/(landing)/pricing/page.tsx",
                                            lineNumber: 56,
                                            columnNumber: 15
                                        }, this),
                                        " ",
                                        /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$server$2f$route$2d$modules$2f$app$2d$page$2f$vendored$2f$rsc$2f$react$2d$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["jsxDEV"])("span", {
                                            className: "text-muted-foreground",
                                            children: "Two plans. Pick the one that fits."
                                        }, void 0, false, {
                                            fileName: "[project]/apps/website/src/app/(landing)/pricing/page.tsx",
                                            lineNumber: 59,
                                            columnNumber: 15
                                        }, this)
                                    ]
                                }, void 0, true, {
                                    fileName: "[project]/apps/website/src/app/(landing)/pricing/page.tsx",
                                    lineNumber: 55,
                                    columnNumber: 13
                                }, this),
                                /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$server$2f$route$2d$modules$2f$app$2d$page$2f$vendored$2f$rsc$2f$react$2d$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["jsxDEV"])("p", {
                                    className: "mt-6 text-pretty text-base text-muted-foreground/70 leading-relaxed sm:text-lg",
                                    children: "No hidden fees. Cancel anytime. Everything you need to run jobs, workflows, and operational controls in one platform."
                                }, void 0, false, {
                                    fileName: "[project]/apps/website/src/app/(landing)/pricing/page.tsx",
                                    lineNumber: 63,
                                    columnNumber: 13
                                }, this),
                                /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$server$2f$route$2d$modules$2f$app$2d$page$2f$vendored$2f$rsc$2f$react$2d$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["jsxDEV"])("div", {
                                    className: "mt-8 flex flex-wrap items-center justify-center gap-2.5",
                                    children: [
                                        /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$server$2f$route$2d$modules$2f$app$2d$page$2f$vendored$2f$rsc$2f$react$2d$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["jsxDEV"])("span", {
                                            className: "rounded-full border border-border/60 bg-card px-4 py-1.5 text-muted-foreground text-xs sm:text-sm",
                                            children: [
                                                "Personal from ",
                                                personalYearly,
                                                "/mo"
                                            ]
                                        }, void 0, true, {
                                            fileName: "[project]/apps/website/src/app/(landing)/pricing/page.tsx",
                                            lineNumber: 69,
                                            columnNumber: 15
                                        }, this),
                                        /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$server$2f$route$2d$modules$2f$app$2d$page$2f$vendored$2f$rsc$2f$react$2d$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["jsxDEV"])("span", {
                                            className: "rounded-full border border-border/60 bg-card px-4 py-1.5 text-muted-foreground text-xs sm:text-sm",
                                            children: [
                                                "Pro from ",
                                                proYearly,
                                                "/mo"
                                            ]
                                        }, void 0, true, {
                                            fileName: "[project]/apps/website/src/app/(landing)/pricing/page.tsx",
                                            lineNumber: 72,
                                            columnNumber: 15
                                        }, this),
                                        /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$server$2f$route$2d$modules$2f$app$2d$page$2f$vendored$2f$rsc$2f$react$2d$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["jsxDEV"])("span", {
                                            className: "rounded-full border border-primary/30 bg-primary/8 px-4 py-1.5 font-medium text-primary text-xs sm:text-sm",
                                            children: "Save 20% yearly"
                                        }, void 0, false, {
                                            fileName: "[project]/apps/website/src/app/(landing)/pricing/page.tsx",
                                            lineNumber: 75,
                                            columnNumber: 15
                                        }, this)
                                    ]
                                }, void 0, true, {
                                    fileName: "[project]/apps/website/src/app/(landing)/pricing/page.tsx",
                                    lineNumber: 68,
                                    columnNumber: 13
                                }, this)
                            ]
                        }, void 0, true, {
                            fileName: "[project]/apps/website/src/app/(landing)/pricing/page.tsx",
                            lineNumber: 53,
                            columnNumber: 11
                        }, this)
                    }, void 0, false, {
                        fileName: "[project]/apps/website/src/app/(landing)/pricing/page.tsx",
                        lineNumber: 52,
                        columnNumber: 9
                    }, this)
                ]
            }, void 0, true, {
                fileName: "[project]/apps/website/src/app/(landing)/pricing/page.tsx",
                lineNumber: 47,
                columnNumber: 7
            }, this),
            /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$server$2f$route$2d$modules$2f$app$2d$page$2f$vendored$2f$rsc$2f$react$2d$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["jsxDEV"])("section", {
                className: "pb-20 sm:pb-28",
                children: /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$server$2f$route$2d$modules$2f$app$2d$page$2f$vendored$2f$rsc$2f$react$2d$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["jsxDEV"])(__TURBOPACK__imported__module__$5b$project$5d2f$apps$2f$website$2f$src$2f$components$2f$layout$2f$shell$2e$tsx__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["default"], {
                    variant: "wide",
                    children: [
                        /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$server$2f$route$2d$modules$2f$app$2d$page$2f$vendored$2f$rsc$2f$react$2d$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["jsxDEV"])("div", {
                            className: "mx-auto max-w-3xl",
                            children: /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$server$2f$route$2d$modules$2f$app$2d$page$2f$vendored$2f$rsc$2f$react$2d$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["jsxDEV"])("h2", {
                                className: "text-balance text-2xl leading-[1.2] tracking-tight sm:text-3xl lg:text-4xl",
                                children: [
                                    /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$server$2f$route$2d$modules$2f$app$2d$page$2f$vendored$2f$rsc$2f$react$2d$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["jsxDEV"])("span", {
                                        className: "font-bold text-foreground",
                                        children: "Pick the plan that matches your workload."
                                    }, void 0, false, {
                                        fileName: "[project]/apps/website/src/app/(landing)/pricing/page.tsx",
                                        lineNumber: 87,
                                        columnNumber: 15
                                    }, this),
                                    " ",
                                    /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$server$2f$route$2d$modules$2f$app$2d$page$2f$vendored$2f$rsc$2f$react$2d$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["jsxDEV"])("span", {
                                        className: "text-muted-foreground",
                                        children: "Both plans include the core job runtime."
                                    }, void 0, false, {
                                        fileName: "[project]/apps/website/src/app/(landing)/pricing/page.tsx",
                                        lineNumber: 90,
                                        columnNumber: 15
                                    }, this)
                                ]
                            }, void 0, true, {
                                fileName: "[project]/apps/website/src/app/(landing)/pricing/page.tsx",
                                lineNumber: 86,
                                columnNumber: 13
                            }, this)
                        }, void 0, false, {
                            fileName: "[project]/apps/website/src/app/(landing)/pricing/page.tsx",
                            lineNumber: 85,
                            columnNumber: 11
                        }, this),
                        /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$server$2f$route$2d$modules$2f$app$2d$page$2f$vendored$2f$rsc$2f$react$2d$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["jsxDEV"])(__TURBOPACK__imported__module__$5b$project$5d2f$apps$2f$website$2f$src$2f$app$2f28$landing$292f$components$2f$pricing$2f$static$2d$pricing$2d$table$2e$tsx__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["StaticPricingTable"], {}, void 0, false, {
                            fileName: "[project]/apps/website/src/app/(landing)/pricing/page.tsx",
                            lineNumber: 96,
                            columnNumber: 11
                        }, this)
                    ]
                }, void 0, true, {
                    fileName: "[project]/apps/website/src/app/(landing)/pricing/page.tsx",
                    lineNumber: 84,
                    columnNumber: 9
                }, this)
            }, void 0, false, {
                fileName: "[project]/apps/website/src/app/(landing)/pricing/page.tsx",
                lineNumber: 83,
                columnNumber: 7
            }, this),
            /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$server$2f$route$2d$modules$2f$app$2d$page$2f$vendored$2f$rsc$2f$react$2d$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["jsxDEV"])(__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$server$2f$route$2d$modules$2f$app$2d$page$2f$vendored$2f$rsc$2f$react$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["Suspense"], {
                fallback: null,
                children: /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$server$2f$route$2d$modules$2f$app$2d$page$2f$vendored$2f$rsc$2f$react$2d$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["jsxDEV"])(__TURBOPACK__imported__module__$5b$project$5d2f$apps$2f$website$2f$src$2f$components$2f$pricing$2f$pricing$2d$faq$2e$tsx__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["default"], {}, void 0, false, {
                    fileName: "[project]/apps/website/src/app/(landing)/pricing/page.tsx",
                    lineNumber: 101,
                    columnNumber: 9
                }, this)
            }, void 0, false, {
                fileName: "[project]/apps/website/src/app/(landing)/pricing/page.tsx",
                lineNumber: 100,
                columnNumber: 7
            }, this),
            /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$server$2f$route$2d$modules$2f$app$2d$page$2f$vendored$2f$rsc$2f$react$2d$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["jsxDEV"])(__TURBOPACK__imported__module__$5b$project$5d2f$apps$2f$website$2f$src$2f$app$2f28$landing$292f$components$2f$common$2f$cta$2f$cta$2e$tsx__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["default"], {}, void 0, false, {
                fileName: "[project]/apps/website/src/app/(landing)/pricing/page.tsx",
                lineNumber: 104,
                columnNumber: 7
            }, this)
        ]
    }, void 0, true, {
        fileName: "[project]/apps/website/src/app/(landing)/pricing/page.tsx",
        lineNumber: 42,
        columnNumber: 5
    }, this);
}
}),
"[project]/apps/website/src/app/(landing)/pricing/page.tsx [app-rsc] (ecmascript, Next.js Server Component)", ((__turbopack_context__) => {

__turbopack_context__.n(__turbopack_context__.i("[project]/apps/website/src/app/(landing)/pricing/page.tsx [app-rsc] (ecmascript)"));
}),
];

//# sourceMappingURL=%5Broot-of-the-server%5D__690d87fd._.js.map