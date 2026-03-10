(globalThis.TURBOPACK || (globalThis.TURBOPACK = [])).push([typeof document === "object" ? document.currentScript : undefined,
"[project]/packages/billing/src/products.ts [app-client] (ecmascript)", ((__turbopack_context__) => {
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
if (typeof globalThis.$RefreshHelpers$ === 'object' && globalThis.$RefreshHelpers !== null) {
    __turbopack_context__.k.registerExports(__turbopack_context__.m, globalThis.$RefreshHelpers$);
}
}),
"[project]/apps/website/src/app/(landing)/components/pricing/static-pricing-table.tsx [app-client] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "StaticPricingTable",
    ()=>StaticPricingTable
]);
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/compiled/react/jsx-dev-runtime.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$compiler$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/compiled/react/compiler-runtime.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$hugeicons$2b$core$2d$free$2d$icons$40$4$2e$0$2e$0$2f$node_modules$2f40$hugeicons$2f$core$2d$free$2d$icons$2f$dist$2f$esm$2f$ArrowRight02Icon$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$export__default__as__ArrowRight02Icon$3e$__ = __turbopack_context__.i("[project]/node_modules/.bun/@hugeicons+core-free-icons@4.0.0/node_modules/@hugeicons/core-free-icons/dist/esm/ArrowRight02Icon.js [app-client] (ecmascript) <export default as ArrowRight02Icon>");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$hugeicons$2b$core$2d$free$2d$icons$40$4$2e$0$2e$0$2f$node_modules$2f40$hugeicons$2f$core$2d$free$2d$icons$2f$dist$2f$esm$2f$CheckmarkCircle02Icon$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$export__default__as__CheckmarkCircle02Icon$3e$__ = __turbopack_context__.i("[project]/node_modules/.bun/@hugeicons+core-free-icons@4.0.0/node_modules/@hugeicons/core-free-icons/dist/esm/CheckmarkCircle02Icon.js [app-client] (ecmascript) <export default as CheckmarkCircle02Icon>");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$hugeicons$2b$react$40$1$2e$1$2e$6$2b$b1ab299f0a400331$2f$node_modules$2f40$hugeicons$2f$react$2f$dist$2f$esm$2f$HugeiconsIcon$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@hugeicons+react@1.1.6+b1ab299f0a400331/node_modules/@hugeicons/react/dist/esm/HugeiconsIcon.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$packages$2f$billing$2f$src$2f$products$2e$ts__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/packages/billing/src/products.ts [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$packages$2f$ui$2f$src$2f$components$2f$button$2e$tsx__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/packages/ui/src/components/button.tsx [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$packages$2f$ui$2f$src$2f$utils$2f$index$2e$ts__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/packages/ui/src/utils/index.ts [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$client$2f$app$2d$dir$2f$link$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/client/app-dir/link.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/compiled/react/index.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$apps$2f$website$2f$src$2f$lib$2f$urls$2e$ts__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/apps/website/src/lib/urls.ts [app-client] (ecmascript)");
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
;
;
const staticPlans = [
    {
        id: "personal",
        name: __TURBOPACK__imported__module__$5b$project$5d2f$packages$2f$billing$2f$src$2f$products$2e$ts__$5b$app$2d$client$5d$__$28$ecmascript$29$__["PLANS"].personal.name,
        monthlyPrice: __TURBOPACK__imported__module__$5b$project$5d2f$packages$2f$billing$2f$src$2f$products$2e$ts__$5b$app$2d$client$5d$__$28$ecmascript$29$__["PLANS"].personal.prices.monthly,
        yearlyPrice: __TURBOPACK__imported__module__$5b$project$5d2f$packages$2f$billing$2f$src$2f$products$2e$ts__$5b$app$2d$client$5d$__$28$ecmascript$29$__["PLANS"].personal.prices.yearly,
        description: __TURBOPACK__imported__module__$5b$project$5d2f$packages$2f$billing$2f$src$2f$products$2e$ts__$5b$app$2d$client$5d$__$28$ecmascript$29$__["PLANS"].personal.description,
        features: [
            ...__TURBOPACK__imported__module__$5b$project$5d2f$packages$2f$billing$2f$src$2f$products$2e$ts__$5b$app$2d$client$5d$__$28$ecmascript$29$__["PLANS"].personal.features
        ],
        cta: "Start Personal"
    },
    {
        id: "pro",
        name: __TURBOPACK__imported__module__$5b$project$5d2f$packages$2f$billing$2f$src$2f$products$2e$ts__$5b$app$2d$client$5d$__$28$ecmascript$29$__["PLANS"].pro.name,
        monthlyPrice: __TURBOPACK__imported__module__$5b$project$5d2f$packages$2f$billing$2f$src$2f$products$2e$ts__$5b$app$2d$client$5d$__$28$ecmascript$29$__["PLANS"].pro.prices.monthly,
        yearlyPrice: __TURBOPACK__imported__module__$5b$project$5d2f$packages$2f$billing$2f$src$2f$products$2e$ts__$5b$app$2d$client$5d$__$28$ecmascript$29$__["PLANS"].pro.prices.yearly,
        description: __TURBOPACK__imported__module__$5b$project$5d2f$packages$2f$billing$2f$src$2f$products$2e$ts__$5b$app$2d$client$5d$__$28$ecmascript$29$__["PLANS"].pro.description,
        features: [
            ...__TURBOPACK__imported__module__$5b$project$5d2f$packages$2f$billing$2f$src$2f$products$2e$ts__$5b$app$2d$client$5d$__$28$ecmascript$29$__["PLANS"].pro.features
        ],
        cta: "Start Pro",
        popular: true
    }
];
function StaticPricingTable() {
    _s();
    const $ = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$compiler$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["c"])(24);
    if ($[0] !== "7b95087bed821db008838236cbbb48550979fd678aac494dca3d8a8885644d08") {
        for(let $i = 0; $i < 24; $i += 1){
            $[$i] = Symbol.for("react.memo_cache_sentinel");
        }
        $[0] = "7b95087bed821db008838236cbbb48550979fd678aac494dca3d8a8885644d08";
    }
    const [interval, setInterval] = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useState"])("yearly");
    const monthly = __TURBOPACK__imported__module__$5b$project$5d2f$packages$2f$billing$2f$src$2f$products$2e$ts__$5b$app$2d$client$5d$__$28$ecmascript$29$__["PLANS"].personal.prices.monthly;
    const yearly = __TURBOPACK__imported__module__$5b$project$5d2f$packages$2f$billing$2f$src$2f$products$2e$ts__$5b$app$2d$client$5d$__$28$ecmascript$29$__["PLANS"].personal.prices.yearly;
    let t0;
    if ($[1] === Symbol.for("react.memo_cache_sentinel")) {
        t0 = Math.round((monthly - yearly) / monthly * 100);
        $[1] = t0;
    } else {
        t0 = $[1];
    }
    const savingsPercent = t0;
    const t1 = interval === "monthly" ? "bg-primary text-primary-foreground shadow-sm" : "text-muted-foreground hover:text-foreground";
    let t2;
    if ($[2] !== t1) {
        t2 = (0, __TURBOPACK__imported__module__$5b$project$5d2f$packages$2f$ui$2f$src$2f$utils$2f$index$2e$ts__$5b$app$2d$client$5d$__$28$ecmascript$29$__["cn"])("min-h-11 rounded-full px-5 py-2.5 font-medium text-sm transition-all", t1);
        $[2] = t1;
        $[3] = t2;
    } else {
        t2 = $[3];
    }
    let t3;
    if ($[4] === Symbol.for("react.memo_cache_sentinel")) {
        t3 = ({
            "StaticPricingTable[<button>.onClick]": ()=>setInterval("monthly")
        })["StaticPricingTable[<button>.onClick]"];
        $[4] = t3;
    } else {
        t3 = $[4];
    }
    let t4;
    if ($[5] !== t2) {
        t4 = /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])("button", {
            className: t2,
            onClick: t3,
            type: "button",
            children: "Monthly"
        }, void 0, false, {
            fileName: "[project]/apps/website/src/app/(landing)/components/pricing/static-pricing-table.tsx",
            lineNumber: 79,
            columnNumber: 10
        }, this);
        $[5] = t2;
        $[6] = t4;
    } else {
        t4 = $[6];
    }
    const t5 = interval === "yearly" ? "bg-primary text-primary-foreground shadow-sm" : "text-muted-foreground hover:text-foreground";
    let t6;
    if ($[7] !== t5) {
        t6 = (0, __TURBOPACK__imported__module__$5b$project$5d2f$packages$2f$ui$2f$src$2f$utils$2f$index$2e$ts__$5b$app$2d$client$5d$__$28$ecmascript$29$__["cn"])("min-h-11 rounded-full px-5 py-2.5 font-medium text-sm transition-all", t5);
        $[7] = t5;
        $[8] = t6;
    } else {
        t6 = $[8];
    }
    let t7;
    if ($[9] === Symbol.for("react.memo_cache_sentinel")) {
        t7 = ({
            "StaticPricingTable[<button>.onClick]": ()=>setInterval("yearly")
        })["StaticPricingTable[<button>.onClick]"];
        $[9] = t7;
    } else {
        t7 = $[9];
    }
    let t8;
    if ($[10] !== t6) {
        t8 = /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])("button", {
            className: t6,
            onClick: t7,
            type: "button",
            children: "Yearly"
        }, void 0, false, {
            fileName: "[project]/apps/website/src/app/(landing)/components/pricing/static-pricing-table.tsx",
            lineNumber: 105,
            columnNumber: 10
        }, this);
        $[10] = t6;
        $[11] = t8;
    } else {
        t8 = $[11];
    }
    let t9;
    if ($[12] === Symbol.for("react.memo_cache_sentinel")) {
        t9 = /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])("span", {
            className: "mr-2 ml-1 rounded-full bg-primary/10 px-3 py-1 font-medium text-primary text-xs",
            children: [
                "Save ",
                savingsPercent,
                "%"
            ]
        }, void 0, true, {
            fileName: "[project]/apps/website/src/app/(landing)/components/pricing/static-pricing-table.tsx",
            lineNumber: 113,
            columnNumber: 10
        }, this);
        $[12] = t9;
    } else {
        t9 = $[12];
    }
    let t10;
    if ($[13] !== t4 || $[14] !== t8) {
        t10 = /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])("div", {
            className: "flex justify-center px-1",
            children: /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])("div", {
                className: "inline-flex items-center gap-1 rounded-full border border-border/60 bg-card p-1",
                children: [
                    t4,
                    t8,
                    t9
                ]
            }, void 0, true, {
                fileName: "[project]/apps/website/src/app/(landing)/components/pricing/static-pricing-table.tsx",
                lineNumber: 120,
                columnNumber: 53
            }, this)
        }, void 0, false, {
            fileName: "[project]/apps/website/src/app/(landing)/components/pricing/static-pricing-table.tsx",
            lineNumber: 120,
            columnNumber: 11
        }, this);
        $[13] = t4;
        $[14] = t8;
        $[15] = t10;
    } else {
        t10 = $[15];
    }
    let t11;
    if ($[16] !== interval) {
        t11 = staticPlans.map({
            "StaticPricingTable[staticPlans.map()]": (plan)=>{
                const price = interval === "monthly" ? plan.monthlyPrice : plan.yearlyPrice;
                return /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])("div", {
                    className: (0, __TURBOPACK__imported__module__$5b$project$5d2f$packages$2f$ui$2f$src$2f$utils$2f$index$2e$ts__$5b$app$2d$client$5d$__$28$ecmascript$29$__["cn"])("relative flex h-full flex-col overflow-hidden rounded-2xl border transition-shadow duration-300", plan.popular ? "border-primary/40 shadow-lg shadow-primary/10" : "border-border/60 bg-card hover:border-border hover:shadow-md"),
                    children: [
                        plan.popular ? /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])("div", {
                            className: "relative bg-primary px-6 py-8 sm:px-8",
                            children: [
                                /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])("div", {
                                    className: "showcase-dots pointer-events-none absolute inset-0"
                                }, void 0, false, {
                                    fileName: "[project]/apps/website/src/app/(landing)/components/pricing/static-pricing-table.tsx",
                                    lineNumber: 132,
                                    columnNumber: 349
                                }, this),
                                /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])("div", {
                                    className: "pointer-events-none absolute inset-0 opacity-30",
                                    style: {
                                        background: "radial-gradient(circle at 30% 20%, oklch(1 0 0 / 0.2), transparent 50%), radial-gradient(circle at 70% 80%, oklch(1 0 0 / 0.1), transparent 50%)"
                                    }
                                }, void 0, false, {
                                    fileName: "[project]/apps/website/src/app/(landing)/components/pricing/static-pricing-table.tsx",
                                    lineNumber: 132,
                                    columnNumber: 419
                                }, this),
                                /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])("div", {
                                    className: "relative z-10",
                                    children: [
                                        /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])("span", {
                                            className: "mb-4 inline-block rounded-md bg-primary-foreground/20 px-3 py-1.5 font-medium text-primary-foreground text-xs backdrop-blur-sm",
                                            children: "Most popular"
                                        }, void 0, false, {
                                            fileName: "[project]/apps/website/src/app/(landing)/components/pricing/static-pricing-table.tsx",
                                            lineNumber: 134,
                                            columnNumber: 49
                                        }, this),
                                        /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])("h3", {
                                            className: "font-bold text-2xl text-primary-foreground tracking-tight",
                                            children: plan.name
                                        }, void 0, false, {
                                            fileName: "[project]/apps/website/src/app/(landing)/components/pricing/static-pricing-table.tsx",
                                            lineNumber: 134,
                                            columnNumber: 213
                                        }, this),
                                        /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])("p", {
                                            className: "mt-2 max-w-sm text-pretty text-primary-foreground/70 text-sm leading-relaxed",
                                            children: plan.description
                                        }, void 0, false, {
                                            fileName: "[project]/apps/website/src/app/(landing)/components/pricing/static-pricing-table.tsx",
                                            lineNumber: 134,
                                            columnNumber: 303
                                        }, this)
                                    ]
                                }, void 0, true, {
                                    fileName: "[project]/apps/website/src/app/(landing)/components/pricing/static-pricing-table.tsx",
                                    lineNumber: 134,
                                    columnNumber: 18
                                }, this)
                            ]
                        }, void 0, true, {
                            fileName: "[project]/apps/website/src/app/(landing)/components/pricing/static-pricing-table.tsx",
                            lineNumber: 132,
                            columnNumber: 294
                        }, this) : /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])("div", {
                            className: "px-6 pt-8 sm:px-8",
                            children: [
                                /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])("h3", {
                                    className: "font-bold text-2xl text-foreground tracking-tight",
                                    children: plan.name
                                }, void 0, false, {
                                    fileName: "[project]/apps/website/src/app/(landing)/components/pricing/static-pricing-table.tsx",
                                    lineNumber: 134,
                                    columnNumber: 467
                                }, this),
                                /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])("p", {
                                    className: "mt-2 max-w-sm text-pretty text-muted-foreground text-sm leading-relaxed",
                                    children: plan.description
                                }, void 0, false, {
                                    fileName: "[project]/apps/website/src/app/(landing)/components/pricing/static-pricing-table.tsx",
                                    lineNumber: 134,
                                    columnNumber: 549
                                }, this)
                            ]
                        }, void 0, true, {
                            fileName: "[project]/apps/website/src/app/(landing)/components/pricing/static-pricing-table.tsx",
                            lineNumber: 134,
                            columnNumber: 432
                        }, this),
                        /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])("div", {
                            className: "flex flex-1 flex-col px-6 pb-8 sm:px-8",
                            children: [
                                /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])("div", {
                                    className: "mt-8 mb-8 flex items-baseline gap-1",
                                    children: [
                                        /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])("span", {
                                            className: "font-bold text-5xl text-foreground tabular-nums tracking-tight",
                                            children: (0, __TURBOPACK__imported__module__$5b$project$5d2f$packages$2f$billing$2f$src$2f$products$2e$ts__$5b$app$2d$client$5d$__$28$ecmascript$29$__["formatPrice"])(price)
                                        }, void 0, false, {
                                            fileName: "[project]/apps/website/src/app/(landing)/components/pricing/static-pricing-table.tsx",
                                            lineNumber: 134,
                                            columnNumber: 774
                                        }, this),
                                        /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])("span", {
                                            className: "text-muted-foreground text-sm",
                                            children: [
                                                "/",
                                                interval === "monthly" ? "mo" : "mo billed yearly"
                                            ]
                                        }, void 0, true, {
                                            fileName: "[project]/apps/website/src/app/(landing)/components/pricing/static-pricing-table.tsx",
                                            lineNumber: 134,
                                            columnNumber: 882
                                        }, this)
                                    ]
                                }, void 0, true, {
                                    fileName: "[project]/apps/website/src/app/(landing)/components/pricing/static-pricing-table.tsx",
                                    lineNumber: 134,
                                    columnNumber: 721
                                }, this),
                                /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])("div", {
                                    className: "mb-8 border-border/40 border-t"
                                }, void 0, false, {
                                    fileName: "[project]/apps/website/src/app/(landing)/components/pricing/static-pricing-table.tsx",
                                    lineNumber: 134,
                                    columnNumber: 996
                                }, this),
                                /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])("ul", {
                                    className: "mb-10 flex-1 space-y-3.5",
                                    children: plan.features.map(_StaticPricingTableStaticPlansMapPlanFeaturesMap)
                                }, void 0, false, {
                                    fileName: "[project]/apps/website/src/app/(landing)/components/pricing/static-pricing-table.tsx",
                                    lineNumber: 134,
                                    columnNumber: 1046
                                }, this),
                                /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])(__TURBOPACK__imported__module__$5b$project$5d2f$packages$2f$ui$2f$src$2f$components$2f$button$2e$tsx__$5b$app$2d$client$5d$__$28$ecmascript$29$__["Button"], {
                                    className: (0, __TURBOPACK__imported__module__$5b$project$5d2f$packages$2f$ui$2f$src$2f$utils$2f$index$2e$ts__$5b$app$2d$client$5d$__$28$ecmascript$29$__["cn"])("w-full", plan.popular ? "shadow-lg shadow-primary/20 transition-all duration-300 hover:shadow-primary/25 hover:shadow-xl" : "transition-all duration-300"),
                                    render: /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])(__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$client$2f$app$2d$dir$2f$link$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["default"], {
                                        href: (0, __TURBOPACK__imported__module__$5b$project$5d2f$apps$2f$website$2f$src$2f$lib$2f$urls$2e$ts__$5b$app$2d$client$5d$__$28$ecmascript$29$__["dashboardHref"])("/login")
                                    }, void 0, false, {
                                        fileName: "[project]/apps/website/src/app/(landing)/components/pricing/static-pricing-table.tsx",
                                        lineNumber: 134,
                                        columnNumber: 1348
                                    }, void 0),
                                    size: "lg",
                                    variant: plan.popular ? "default" : "outline",
                                    children: [
                                        plan.cta,
                                        /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])(__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$hugeicons$2b$react$40$1$2e$1$2e$6$2b$b1ab299f0a400331$2f$node_modules$2f40$hugeicons$2f$react$2f$dist$2f$esm$2f$HugeiconsIcon$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["HugeiconsIcon"], {
                                            className: "size-4",
                                            icon: __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$hugeicons$2b$core$2d$free$2d$icons$40$4$2e$0$2e$0$2f$node_modules$2f40$hugeicons$2f$core$2d$free$2d$icons$2f$dist$2f$esm$2f$ArrowRight02Icon$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$export__default__as__ArrowRight02Icon$3e$__["ArrowRight02Icon"]
                                        }, void 0, false, {
                                            fileName: "[project]/apps/website/src/app/(landing)/components/pricing/static-pricing-table.tsx",
                                            lineNumber: 134,
                                            columnNumber: 1456
                                        }, this)
                                    ]
                                }, void 0, true, {
                                    fileName: "[project]/apps/website/src/app/(landing)/components/pricing/static-pricing-table.tsx",
                                    lineNumber: 134,
                                    columnNumber: 1161
                                }, this)
                            ]
                        }, void 0, true, {
                            fileName: "[project]/apps/website/src/app/(landing)/components/pricing/static-pricing-table.tsx",
                            lineNumber: 134,
                            columnNumber: 665
                        }, this)
                    ]
                }, plan.id, true, {
                    fileName: "[project]/apps/website/src/app/(landing)/components/pricing/static-pricing-table.tsx",
                    lineNumber: 132,
                    columnNumber: 16
                }, this);
            }
        }["StaticPricingTable[staticPlans.map()]"]);
        $[16] = interval;
        $[17] = t11;
    } else {
        t11 = $[17];
    }
    let t12;
    if ($[18] !== t11) {
        t12 = /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])("div", {
            className: "mt-10 grid grid-cols-1 gap-6 md:grid-cols-2 lg:gap-8 xl:gap-10",
            children: t11
        }, void 0, false, {
            fileName: "[project]/apps/website/src/app/(landing)/components/pricing/static-pricing-table.tsx",
            lineNumber: 144,
            columnNumber: 11
        }, this);
        $[18] = t11;
        $[19] = t12;
    } else {
        t12 = $[19];
    }
    let t13;
    if ($[20] === Symbol.for("react.memo_cache_sentinel")) {
        t13 = /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])("p", {
            className: "mt-8 text-center text-muted-foreground/60 text-sm",
            children: "All plans include core orchestration capabilities. Cancel anytime."
        }, void 0, false, {
            fileName: "[project]/apps/website/src/app/(landing)/components/pricing/static-pricing-table.tsx",
            lineNumber: 152,
            columnNumber: 11
        }, this);
        $[20] = t13;
    } else {
        t13 = $[20];
    }
    let t14;
    if ($[21] !== t10 || $[22] !== t12) {
        t14 = /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])("div", {
            className: "mt-10 sm:mt-12",
            children: [
                t10,
                t12,
                t13
            ]
        }, void 0, true, {
            fileName: "[project]/apps/website/src/app/(landing)/components/pricing/static-pricing-table.tsx",
            lineNumber: 159,
            columnNumber: 11
        }, this);
        $[21] = t10;
        $[22] = t12;
        $[23] = t14;
    } else {
        t14 = $[23];
    }
    return t14;
}
_s(StaticPricingTable, "BqsWhmjLHZrNXPytWTpOjqBe5r4=");
_c = StaticPricingTable;
function _StaticPricingTableStaticPlansMapPlanFeaturesMap(feature) {
    return /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])("li", {
        className: "flex items-start gap-3 text-sm leading-relaxed",
        children: [
            /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])(__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$hugeicons$2b$react$40$1$2e$1$2e$6$2b$b1ab299f0a400331$2f$node_modules$2f40$hugeicons$2f$react$2f$dist$2f$esm$2f$HugeiconsIcon$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["HugeiconsIcon"], {
                className: "mt-0.5 size-4 shrink-0 text-primary",
                icon: __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$hugeicons$2b$core$2d$free$2d$icons$40$4$2e$0$2e$0$2f$node_modules$2f40$hugeicons$2f$core$2d$free$2d$icons$2f$dist$2f$esm$2f$CheckmarkCircle02Icon$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$export__default__as__CheckmarkCircle02Icon$3e$__["CheckmarkCircle02Icon"]
            }, void 0, false, {
                fileName: "[project]/apps/website/src/app/(landing)/components/pricing/static-pricing-table.tsx",
                lineNumber: 169,
                columnNumber: 87
            }, this),
            /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])("span", {
                className: "text-pretty text-muted-foreground",
                children: feature
            }, void 0, false, {
                fileName: "[project]/apps/website/src/app/(landing)/components/pricing/static-pricing-table.tsx",
                lineNumber: 169,
                columnNumber: 181
            }, this)
        ]
    }, feature, true, {
        fileName: "[project]/apps/website/src/app/(landing)/components/pricing/static-pricing-table.tsx",
        lineNumber: 169,
        columnNumber: 10
    }, this);
}
var _c;
__turbopack_context__.k.register(_c, "StaticPricingTable");
if (typeof globalThis.$RefreshHelpers$ === 'object' && globalThis.$RefreshHelpers !== null) {
    __turbopack_context__.k.registerExports(__turbopack_context__.m, globalThis.$RefreshHelpers$);
}
}),
"[project]/packages/ui/src/components/accordion.tsx [app-client] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "Accordion",
    ()=>Accordion,
    "AccordionContent",
    ()=>AccordionContent,
    "AccordionItem",
    ()=>AccordionItem,
    "AccordionTrigger",
    ()=>AccordionTrigger
]);
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/compiled/react/jsx-dev-runtime.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$compiler$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/compiled/react/compiler-runtime.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$accordion$2f$index$2e$parts$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$export__$2a$__as__Accordion$3e$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/accordion/index.parts.js [app-client] (ecmascript) <export * as Accordion>");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$hugeicons$2b$core$2d$free$2d$icons$40$4$2e$0$2e$0$2f$node_modules$2f40$hugeicons$2f$core$2d$free$2d$icons$2f$dist$2f$esm$2f$ArrowDown01Icon$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$export__default__as__ArrowDown01Icon$3e$__ = __turbopack_context__.i("[project]/node_modules/.bun/@hugeicons+core-free-icons@4.0.0/node_modules/@hugeicons/core-free-icons/dist/esm/ArrowDown01Icon.js [app-client] (ecmascript) <export default as ArrowDown01Icon>");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$hugeicons$2b$core$2d$free$2d$icons$40$4$2e$0$2e$0$2f$node_modules$2f40$hugeicons$2f$core$2d$free$2d$icons$2f$dist$2f$esm$2f$ArrowUp01Icon$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$export__default__as__ArrowUp01Icon$3e$__ = __turbopack_context__.i("[project]/node_modules/.bun/@hugeicons+core-free-icons@4.0.0/node_modules/@hugeicons/core-free-icons/dist/esm/ArrowUp01Icon.js [app-client] (ecmascript) <export default as ArrowUp01Icon>");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$hugeicons$2b$react$40$1$2e$1$2e$6$2b$b1ab299f0a400331$2f$node_modules$2f40$hugeicons$2f$react$2f$dist$2f$esm$2f$HugeiconsIcon$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@hugeicons+react@1.1.6+b1ab299f0a400331/node_modules/@hugeicons/react/dist/esm/HugeiconsIcon.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$packages$2f$ui$2f$src$2f$utils$2f$index$2e$ts__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/packages/ui/src/utils/index.ts [app-client] (ecmascript)");
"use client";
;
;
;
;
;
;
function Accordion(t0) {
    const $ = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$compiler$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["c"])(9);
    if ($[0] !== "2b4b3d43d6bb4ff8ee2409dc0b6b75edc0e8d35841a4e71571d73b384506bf76") {
        for(let $i = 0; $i < 9; $i += 1){
            $[$i] = Symbol.for("react.memo_cache_sentinel");
        }
        $[0] = "2b4b3d43d6bb4ff8ee2409dc0b6b75edc0e8d35841a4e71571d73b384506bf76";
    }
    let className;
    let props;
    if ($[1] !== t0) {
        ({ className, ...props } = t0);
        $[1] = t0;
        $[2] = className;
        $[3] = props;
    } else {
        className = $[2];
        props = $[3];
    }
    let t1;
    if ($[4] !== className) {
        t1 = (0, __TURBOPACK__imported__module__$5b$project$5d2f$packages$2f$ui$2f$src$2f$utils$2f$index$2e$ts__$5b$app$2d$client$5d$__$28$ecmascript$29$__["cn"])("flex w-full flex-col overflow-hidden rounded-md border", className);
        $[4] = className;
        $[5] = t1;
    } else {
        t1 = $[5];
    }
    let t2;
    if ($[6] !== props || $[7] !== t1) {
        t2 = /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])(__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$accordion$2f$index$2e$parts$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$export__$2a$__as__Accordion$3e$__["Accordion"].Root, {
            className: t1,
            "data-slot": "accordion",
            ...props
        }, void 0, false, {
            fileName: "[project]/packages/ui/src/components/accordion.tsx",
            lineNumber: 40,
            columnNumber: 10
        }, this);
        $[6] = props;
        $[7] = t1;
        $[8] = t2;
    } else {
        t2 = $[8];
    }
    return t2;
}
_c = Accordion;
function AccordionItem(t0) {
    const $ = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$compiler$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["c"])(9);
    if ($[0] !== "2b4b3d43d6bb4ff8ee2409dc0b6b75edc0e8d35841a4e71571d73b384506bf76") {
        for(let $i = 0; $i < 9; $i += 1){
            $[$i] = Symbol.for("react.memo_cache_sentinel");
        }
        $[0] = "2b4b3d43d6bb4ff8ee2409dc0b6b75edc0e8d35841a4e71571d73b384506bf76";
    }
    let className;
    let props;
    if ($[1] !== t0) {
        ({ className, ...props } = t0);
        $[1] = t0;
        $[2] = className;
        $[3] = props;
    } else {
        className = $[2];
        props = $[3];
    }
    let t1;
    if ($[4] !== className) {
        t1 = (0, __TURBOPACK__imported__module__$5b$project$5d2f$packages$2f$ui$2f$src$2f$utils$2f$index$2e$ts__$5b$app$2d$client$5d$__$28$ecmascript$29$__["cn"])("not-last:border-b data-open:bg-muted/50", className);
        $[4] = className;
        $[5] = t1;
    } else {
        t1 = $[5];
    }
    let t2;
    if ($[6] !== props || $[7] !== t1) {
        t2 = /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])(__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$accordion$2f$index$2e$parts$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$export__$2a$__as__Accordion$3e$__["Accordion"].Item, {
            className: t1,
            "data-slot": "accordion-item",
            ...props
        }, void 0, false, {
            fileName: "[project]/packages/ui/src/components/accordion.tsx",
            lineNumber: 81,
            columnNumber: 10
        }, this);
        $[6] = props;
        $[7] = t1;
        $[8] = t2;
    } else {
        t2 = $[8];
    }
    return t2;
}
_c1 = AccordionItem;
function AccordionTrigger(t0) {
    const $ = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$compiler$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["c"])(13);
    if ($[0] !== "2b4b3d43d6bb4ff8ee2409dc0b6b75edc0e8d35841a4e71571d73b384506bf76") {
        for(let $i = 0; $i < 13; $i += 1){
            $[$i] = Symbol.for("react.memo_cache_sentinel");
        }
        $[0] = "2b4b3d43d6bb4ff8ee2409dc0b6b75edc0e8d35841a4e71571d73b384506bf76";
    }
    let children;
    let className;
    let props;
    if ($[1] !== t0) {
        ({ className, children, ...props } = t0);
        $[1] = t0;
        $[2] = children;
        $[3] = className;
        $[4] = props;
    } else {
        children = $[2];
        className = $[3];
        props = $[4];
    }
    let t1;
    if ($[5] !== className) {
        t1 = (0, __TURBOPACK__imported__module__$5b$project$5d2f$packages$2f$ui$2f$src$2f$utils$2f$index$2e$ts__$5b$app$2d$client$5d$__$28$ecmascript$29$__["cn"])("group/accordion-trigger relative flex flex-1 items-start justify-between gap-6 border border-transparent p-2 text-left font-medium text-xs/relaxed outline-none transition-all hover:underline focus-visible:border-ring focus-visible:ring-2 focus-visible:ring-ring/50 aria-disabled:pointer-events-none aria-disabled:opacity-50 **:data-[slot=accordion-trigger-icon]:ml-auto **:data-[slot=accordion-trigger-icon]:size-4 **:data-[slot=accordion-trigger-icon]:text-muted-foreground", className);
        $[5] = className;
        $[6] = t1;
    } else {
        t1 = $[6];
    }
    let t2;
    let t3;
    if ($[7] === Symbol.for("react.memo_cache_sentinel")) {
        t2 = /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])(__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$hugeicons$2b$react$40$1$2e$1$2e$6$2b$b1ab299f0a400331$2f$node_modules$2f40$hugeicons$2f$react$2f$dist$2f$esm$2f$HugeiconsIcon$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["HugeiconsIcon"], {
            className: "pointer-events-none shrink-0 group-aria-expanded/accordion-trigger:hidden",
            "data-slot": "accordion-trigger-icon",
            icon: __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$hugeicons$2b$core$2d$free$2d$icons$40$4$2e$0$2e$0$2f$node_modules$2f40$hugeicons$2f$core$2d$free$2d$icons$2f$dist$2f$esm$2f$ArrowDown01Icon$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$export__default__as__ArrowDown01Icon$3e$__["ArrowDown01Icon"],
            strokeWidth: 2
        }, void 0, false, {
            fileName: "[project]/packages/ui/src/components/accordion.tsx",
            lineNumber: 127,
            columnNumber: 10
        }, this);
        t3 = /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])(__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$hugeicons$2b$react$40$1$2e$1$2e$6$2b$b1ab299f0a400331$2f$node_modules$2f40$hugeicons$2f$react$2f$dist$2f$esm$2f$HugeiconsIcon$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["HugeiconsIcon"], {
            className: "pointer-events-none hidden shrink-0 group-aria-expanded/accordion-trigger:inline",
            "data-slot": "accordion-trigger-icon",
            icon: __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$hugeicons$2b$core$2d$free$2d$icons$40$4$2e$0$2e$0$2f$node_modules$2f40$hugeicons$2f$core$2d$free$2d$icons$2f$dist$2f$esm$2f$ArrowUp01Icon$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$export__default__as__ArrowUp01Icon$3e$__["ArrowUp01Icon"],
            strokeWidth: 2
        }, void 0, false, {
            fileName: "[project]/packages/ui/src/components/accordion.tsx",
            lineNumber: 128,
            columnNumber: 10
        }, this);
        $[7] = t2;
        $[8] = t3;
    } else {
        t2 = $[7];
        t3 = $[8];
    }
    let t4;
    if ($[9] !== children || $[10] !== props || $[11] !== t1) {
        t4 = /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])(__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$accordion$2f$index$2e$parts$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$export__$2a$__as__Accordion$3e$__["Accordion"].Header, {
            className: "flex",
            children: /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])(__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$accordion$2f$index$2e$parts$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$export__$2a$__as__Accordion$3e$__["Accordion"].Trigger, {
                className: t1,
                "data-slot": "accordion-trigger",
                ...props,
                children: [
                    children,
                    t2,
                    t3
                ]
            }, void 0, true, {
                fileName: "[project]/packages/ui/src/components/accordion.tsx",
                lineNumber: 137,
                columnNumber: 54
            }, this)
        }, void 0, false, {
            fileName: "[project]/packages/ui/src/components/accordion.tsx",
            lineNumber: 137,
            columnNumber: 10
        }, this);
        $[9] = children;
        $[10] = props;
        $[11] = t1;
        $[12] = t4;
    } else {
        t4 = $[12];
    }
    return t4;
}
_c2 = AccordionTrigger;
function AccordionContent(t0) {
    const $ = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$compiler$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["c"])(13);
    if ($[0] !== "2b4b3d43d6bb4ff8ee2409dc0b6b75edc0e8d35841a4e71571d73b384506bf76") {
        for(let $i = 0; $i < 13; $i += 1){
            $[$i] = Symbol.for("react.memo_cache_sentinel");
        }
        $[0] = "2b4b3d43d6bb4ff8ee2409dc0b6b75edc0e8d35841a4e71571d73b384506bf76";
    }
    let children;
    let className;
    let props;
    if ($[1] !== t0) {
        ({ className, children, ...props } = t0);
        $[1] = t0;
        $[2] = children;
        $[3] = className;
        $[4] = props;
    } else {
        children = $[2];
        className = $[3];
        props = $[4];
    }
    let t1;
    if ($[5] !== className) {
        t1 = (0, __TURBOPACK__imported__module__$5b$project$5d2f$packages$2f$ui$2f$src$2f$utils$2f$index$2e$ts__$5b$app$2d$client$5d$__$28$ecmascript$29$__["cn"])("h-(--accordion-panel-height) pt-0 pb-4 data-ending-style:h-0 data-starting-style:h-0 [&_a]:underline [&_a]:underline-offset-3 [&_a]:hover:text-foreground [&_p:not(:last-child)]:mb-4", className);
        $[5] = className;
        $[6] = t1;
    } else {
        t1 = $[6];
    }
    let t2;
    if ($[7] !== children || $[8] !== t1) {
        t2 = /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])("div", {
            className: t1,
            children: children
        }, void 0, false, {
            fileName: "[project]/packages/ui/src/components/accordion.tsx",
            lineNumber: 183,
            columnNumber: 10
        }, this);
        $[7] = children;
        $[8] = t1;
        $[9] = t2;
    } else {
        t2 = $[9];
    }
    let t3;
    if ($[10] !== props || $[11] !== t2) {
        t3 = /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])(__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$accordion$2f$index$2e$parts$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$export__$2a$__as__Accordion$3e$__["Accordion"].Panel, {
            className: "overflow-hidden px-2 text-xs/relaxed data-closed:animate-accordion-up data-open:animate-accordion-down",
            "data-slot": "accordion-content",
            ...props,
            children: t2
        }, void 0, false, {
            fileName: "[project]/packages/ui/src/components/accordion.tsx",
            lineNumber: 192,
            columnNumber: 10
        }, this);
        $[10] = props;
        $[11] = t2;
        $[12] = t3;
    } else {
        t3 = $[12];
    }
    return t3;
}
_c3 = AccordionContent;
;
var _c, _c1, _c2, _c3;
__turbopack_context__.k.register(_c, "Accordion");
__turbopack_context__.k.register(_c1, "AccordionItem");
__turbopack_context__.k.register(_c2, "AccordionTrigger");
__turbopack_context__.k.register(_c3, "AccordionContent");
if (typeof globalThis.$RefreshHelpers$ === 'object' && globalThis.$RefreshHelpers !== null) {
    __turbopack_context__.k.registerExports(__turbopack_context__.m, globalThis.$RefreshHelpers$);
}
}),
"[project]/apps/website/src/components/pricing/pricing-faq.client.tsx [app-client] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "default",
    ()=>__TURBOPACK__default__export__
]);
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/compiled/react/jsx-dev-runtime.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$compiler$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/compiled/react/compiler-runtime.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$hugeicons$2b$core$2d$free$2d$icons$40$4$2e$0$2e$0$2f$node_modules$2f40$hugeicons$2f$core$2d$free$2d$icons$2f$dist$2f$esm$2f$ArrowRight02Icon$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$export__default__as__ArrowRight02Icon$3e$__ = __turbopack_context__.i("[project]/node_modules/.bun/@hugeicons+core-free-icons@4.0.0/node_modules/@hugeicons/core-free-icons/dist/esm/ArrowRight02Icon.js [app-client] (ecmascript) <export default as ArrowRight02Icon>");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$hugeicons$2b$react$40$1$2e$1$2e$6$2b$b1ab299f0a400331$2f$node_modules$2f40$hugeicons$2f$react$2f$dist$2f$esm$2f$HugeiconsIcon$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@hugeicons+react@1.1.6+b1ab299f0a400331/node_modules/@hugeicons/react/dist/esm/HugeiconsIcon.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$packages$2f$ui$2f$src$2f$components$2f$accordion$2e$tsx__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/packages/ui/src/components/accordion.tsx [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$packages$2f$ui$2f$src$2f$components$2f$button$2e$tsx__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/packages/ui/src/components/button.tsx [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$client$2f$app$2d$dir$2f$link$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/client/app-dir/link.js [app-client] (ecmascript)");
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
;
const PricingFaqClient = (t0)=>{
    _s();
    const $ = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$compiler$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["c"])(28);
    if ($[0] !== "6d4c41ee32df8aafb8eacce51a452d46b6110aaff3d968f59409de37248c4ff9") {
        for(let $i = 0; $i < 28; $i += 1){
            $[$i] = Symbol.for("react.memo_cache_sentinel");
        }
        $[0] = "6d4c41ee32df8aafb8eacce51a452d46b6110aaff3d968f59409de37248c4ff9";
    }
    const { badge, title, description, items } = t0;
    const sectionId = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useId"])();
    let t1;
    if ($[1] !== badge) {
        t1 = /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])("span", {
            className: "kicker",
            children: badge
        }, void 0, false, {
            fileName: "[project]/apps/website/src/components/pricing/pricing-faq.client.tsx",
            lineNumber: 38,
            columnNumber: 10
        }, ("TURBOPACK compile-time value", void 0));
        $[1] = badge;
        $[2] = t1;
    } else {
        t1 = $[2];
    }
    let t2;
    if ($[3] !== title) {
        t2 = /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])("h2", {
            className: "text-balance font-semibold text-4xl text-foreground tracking-tight sm:text-5xl lg:text-6xl",
            children: title
        }, void 0, false, {
            fileName: "[project]/apps/website/src/components/pricing/pricing-faq.client.tsx",
            lineNumber: 46,
            columnNumber: 10
        }, ("TURBOPACK compile-time value", void 0));
        $[3] = title;
        $[4] = t2;
    } else {
        t2 = $[4];
    }
    let t3;
    if ($[5] !== description) {
        t3 = description ? /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])("p", {
            className: "max-w-2xl text-balance text-lg text-muted-foreground leading-relaxed",
            children: description
        }, void 0, false, {
            fileName: "[project]/apps/website/src/components/pricing/pricing-faq.client.tsx",
            lineNumber: 54,
            columnNumber: 24
        }, ("TURBOPACK compile-time value", void 0)) : null;
        $[5] = description;
        $[6] = t3;
    } else {
        t3 = $[6];
    }
    let t4;
    if ($[7] !== t1 || $[8] !== t2 || $[9] !== t3) {
        t4 = /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])("div", {
            className: "mx-auto flex max-w-3xl flex-col items-center gap-4 text-center",
            children: [
                t1,
                t2,
                t3
            ]
        }, void 0, true, {
            fileName: "[project]/apps/website/src/components/pricing/pricing-faq.client.tsx",
            lineNumber: 62,
            columnNumber: 10
        }, ("TURBOPACK compile-time value", void 0));
        $[7] = t1;
        $[8] = t2;
        $[9] = t3;
        $[10] = t4;
    } else {
        t4 = $[10];
    }
    const t5 = items[0]?._id;
    let t6;
    if ($[11] !== t5) {
        t6 = [
            t5
        ];
        $[11] = t5;
        $[12] = t6;
    } else {
        t6 = $[12];
    }
    let t7;
    if ($[13] !== items) {
        let t8;
        if ($[15] !== items.length) {
            t8 = (item, index)=>{
                const isLast = index === items.length - 1;
                return /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])(__TURBOPACK__imported__module__$5b$project$5d2f$packages$2f$ui$2f$src$2f$components$2f$accordion$2e$tsx__$5b$app$2d$client$5d$__$28$ecmascript$29$__["AccordionItem"], {
                    className: isLast ? "" : "border-border/50 border-b",
                    value: item._id,
                    children: [
                        /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])(__TURBOPACK__imported__module__$5b$project$5d2f$packages$2f$ui$2f$src$2f$components$2f$accordion$2e$tsx__$5b$app$2d$client$5d$__$28$ecmascript$29$__["AccordionTrigger"], {
                            className: "px-6 py-5 font-medium text-base text-foreground hover:bg-muted/30 hover:no-underline",
                            children: item.question
                        }, void 0, false, {
                            fileName: "[project]/apps/website/src/components/pricing/pricing-faq.client.tsx",
                            lineNumber: 85,
                            columnNumber: 117
                        }, ("TURBOPACK compile-time value", void 0)),
                        /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])(__TURBOPACK__imported__module__$5b$project$5d2f$packages$2f$ui$2f$src$2f$components$2f$accordion$2e$tsx__$5b$app$2d$client$5d$__$28$ecmascript$29$__["AccordionContent"], {
                            className: "px-6 pb-6 text-base text-muted-foreground leading-relaxed",
                            children: item.answer
                        }, void 0, false, {
                            fileName: "[project]/apps/website/src/components/pricing/pricing-faq.client.tsx",
                            lineNumber: 85,
                            columnNumber: 266
                        }, ("TURBOPACK compile-time value", void 0))
                    ]
                }, item._id, true, {
                    fileName: "[project]/apps/website/src/components/pricing/pricing-faq.client.tsx",
                    lineNumber: 85,
                    columnNumber: 16
                }, ("TURBOPACK compile-time value", void 0));
            };
            $[15] = items.length;
            $[16] = t8;
        } else {
            t8 = $[16];
        }
        t7 = items.map(t8);
        $[13] = items;
        $[14] = t7;
    } else {
        t7 = $[14];
    }
    let t8;
    if ($[17] !== t6 || $[18] !== t7) {
        t8 = /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])("div", {
            className: "mx-auto mt-12 max-w-3xl sm:mt-16",
            children: /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])("div", {
                className: "border-border/50 border-y",
                children: /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])("div", {
                    className: "border-border/50 border-x",
                    children: /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])(__TURBOPACK__imported__module__$5b$project$5d2f$packages$2f$ui$2f$src$2f$components$2f$accordion$2e$tsx__$5b$app$2d$client$5d$__$28$ecmascript$29$__["Accordion"], {
                        className: "rounded-none border-0",
                        defaultValue: t6,
                        children: t7
                    }, void 0, false, {
                        fileName: "[project]/apps/website/src/components/pricing/pricing-faq.client.tsx",
                        lineNumber: 100,
                        columnNumber: 146
                    }, ("TURBOPACK compile-time value", void 0))
                }, void 0, false, {
                    fileName: "[project]/apps/website/src/components/pricing/pricing-faq.client.tsx",
                    lineNumber: 100,
                    columnNumber: 103
                }, ("TURBOPACK compile-time value", void 0))
            }, void 0, false, {
                fileName: "[project]/apps/website/src/components/pricing/pricing-faq.client.tsx",
                lineNumber: 100,
                columnNumber: 60
            }, ("TURBOPACK compile-time value", void 0))
        }, void 0, false, {
            fileName: "[project]/apps/website/src/components/pricing/pricing-faq.client.tsx",
            lineNumber: 100,
            columnNumber: 10
        }, ("TURBOPACK compile-time value", void 0));
        $[17] = t6;
        $[18] = t7;
        $[19] = t8;
    } else {
        t8 = $[19];
    }
    let t9;
    if ($[20] === Symbol.for("react.memo_cache_sentinel")) {
        t9 = /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])(__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$client$2f$app$2d$dir$2f$link$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["default"], {
            href: "mailto:leonardomso11@gmail.com"
        }, void 0, false, {
            fileName: "[project]/apps/website/src/components/pricing/pricing-faq.client.tsx",
            lineNumber: 109,
            columnNumber: 10
        }, ("TURBOPACK compile-time value", void 0));
        $[20] = t9;
    } else {
        t9 = $[20];
    }
    let t10;
    if ($[21] === Symbol.for("react.memo_cache_sentinel")) {
        t10 = /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])("div", {
            className: "mt-10 flex justify-center sm:mt-12",
            children: /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])("p", {
                className: "text-center text-muted-foreground",
                children: [
                    "Still have questions?",
                    " ",
                    /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])(__TURBOPACK__imported__module__$5b$project$5d2f$packages$2f$ui$2f$src$2f$components$2f$button$2e$tsx__$5b$app$2d$client$5d$__$28$ecmascript$29$__["Button"], {
                        className: "inline-flex",
                        render: t9,
                        size: "sm",
                        variant: "link",
                        children: [
                            "Contact our team",
                            /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])(__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$hugeicons$2b$react$40$1$2e$1$2e$6$2b$b1ab299f0a400331$2f$node_modules$2f40$hugeicons$2f$react$2f$dist$2f$esm$2f$HugeiconsIcon$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["HugeiconsIcon"], {
                                className: "size-4",
                                icon: __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$hugeicons$2b$core$2d$free$2d$icons$40$4$2e$0$2e$0$2f$node_modules$2f40$hugeicons$2f$core$2d$free$2d$icons$2f$dist$2f$esm$2f$ArrowRight02Icon$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$export__default__as__ArrowRight02Icon$3e$__["ArrowRight02Icon"]
                            }, void 0, false, {
                                fileName: "[project]/apps/website/src/components/pricing/pricing-faq.client.tsx",
                                lineNumber: 116,
                                columnNumber: 223
                            }, ("TURBOPACK compile-time value", void 0))
                        ]
                    }, void 0, true, {
                        fileName: "[project]/apps/website/src/components/pricing/pricing-faq.client.tsx",
                        lineNumber: 116,
                        columnNumber: 138
                    }, ("TURBOPACK compile-time value", void 0))
                ]
            }, void 0, true, {
                fileName: "[project]/apps/website/src/components/pricing/pricing-faq.client.tsx",
                lineNumber: 116,
                columnNumber: 63
            }, ("TURBOPACK compile-time value", void 0))
        }, void 0, false, {
            fileName: "[project]/apps/website/src/components/pricing/pricing-faq.client.tsx",
            lineNumber: 116,
            columnNumber: 11
        }, ("TURBOPACK compile-time value", void 0));
        $[21] = t10;
    } else {
        t10 = $[21];
    }
    let t11;
    if ($[22] !== t4 || $[23] !== t8) {
        t11 = /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])("div", {
            className: "mx-auto max-w-7xl px-6 lg:px-8",
            children: [
                t4,
                t8,
                t10
            ]
        }, void 0, true, {
            fileName: "[project]/apps/website/src/components/pricing/pricing-faq.client.tsx",
            lineNumber: 123,
            columnNumber: 11
        }, ("TURBOPACK compile-time value", void 0));
        $[22] = t4;
        $[23] = t8;
        $[24] = t11;
    } else {
        t11 = $[24];
    }
    let t12;
    if ($[25] !== sectionId || $[26] !== t11) {
        t12 = /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxDEV"])("section", {
            className: "bg-background py-20 sm:py-28",
            id: sectionId,
            children: t11
        }, void 0, false, {
            fileName: "[project]/apps/website/src/components/pricing/pricing-faq.client.tsx",
            lineNumber: 132,
            columnNumber: 11
        }, ("TURBOPACK compile-time value", void 0));
        $[25] = sectionId;
        $[26] = t11;
        $[27] = t12;
    } else {
        t12 = $[27];
    }
    return t12;
};
_s(PricingFaqClient, "ytu0JuD7crPt4xMlETl2/QsOYv8=", false, function() {
    return [
        __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useId"]
    ];
});
_c = PricingFaqClient;
const __TURBOPACK__default__export__ = PricingFaqClient;
var _c;
__turbopack_context__.k.register(_c, "PricingFaqClient");
if (typeof globalThis.$RefreshHelpers$ === 'object' && globalThis.$RefreshHelpers !== null) {
    __turbopack_context__.k.registerExports(__turbopack_context__.m, globalThis.$RefreshHelpers$);
}
}),
]);

//# sourceMappingURL=_344e1eb3._.js.map