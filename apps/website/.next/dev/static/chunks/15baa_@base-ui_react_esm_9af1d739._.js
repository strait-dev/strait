(globalThis.TURBOPACK || (globalThis.TURBOPACK = [])).push([typeof document === "object" ? document.currentScript : undefined,
"[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/tooltip/index.parts.js [app-client] (ecmascript) <locals>", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([]);
;
;
;
;
;
;
;
;
;
}),
"[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/tooltip/root/TooltipRootContext.js [app-client] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "TooltipRootContext",
    ()=>TooltipRootContext,
    "useTooltipRootContext",
    ()=>useTooltipRootContext
]);
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$build$2f$polyfills$2f$process$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = /*#__PURE__*/ __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/build/polyfills/process.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/compiled/react/index.js [app-client] (ecmascript)");
'use client';
;
;
const TooltipRootContext = /*#__PURE__*/ __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["createContext"](undefined);
if ("TURBOPACK compile-time truthy", 1) TooltipRootContext.displayName = "TooltipRootContext";
function useTooltipRootContext(optional) {
    const context = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useContext"](TooltipRootContext);
    if (context === undefined && !optional) {
        throw new Error(("TURBOPACK compile-time truthy", 1) ? 'Base UI: TooltipRootContext is missing. Tooltip parts must be placed within <Tooltip.Root>.' : "TURBOPACK unreachable");
    }
    return context;
}
}),
"[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/floating-ui-react/utils/constants.js [app-client] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "ACTIVE_KEY",
    ()=>ACTIVE_KEY,
    "ARROW_DOWN",
    ()=>ARROW_DOWN,
    "ARROW_LEFT",
    ()=>ARROW_LEFT,
    "ARROW_RIGHT",
    ()=>ARROW_RIGHT,
    "ARROW_UP",
    ()=>ARROW_UP,
    "FOCUSABLE_ATTRIBUTE",
    ()=>FOCUSABLE_ATTRIBUTE,
    "SELECTED_KEY",
    ()=>SELECTED_KEY,
    "TYPEABLE_SELECTOR",
    ()=>TYPEABLE_SELECTOR
]);
const FOCUSABLE_ATTRIBUTE = 'data-base-ui-focusable';
const ACTIVE_KEY = 'active';
const SELECTED_KEY = 'selected';
const TYPEABLE_SELECTOR = "input:not([type='hidden']):not([disabled])," + "[contenteditable]:not([contenteditable='false']),textarea:not([disabled])";
const ARROW_LEFT = 'ArrowLeft';
const ARROW_RIGHT = 'ArrowRight';
const ARROW_UP = 'ArrowUp';
const ARROW_DOWN = 'ArrowDown';
}),
"[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/floating-ui-react/utils/element.js [app-client] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "activeElement",
    ()=>activeElement,
    "contains",
    ()=>contains,
    "getFloatingFocusElement",
    ()=>getFloatingFocusElement,
    "getTarget",
    ()=>getTarget,
    "isEventTargetWithin",
    ()=>isEventTargetWithin,
    "isRootElement",
    ()=>isRootElement,
    "isTargetInsideEnabledTrigger",
    ()=>isTargetInsideEnabledTrigger,
    "isTypeableCombobox",
    ()=>isTypeableCombobox,
    "isTypeableElement",
    ()=>isTypeableElement,
    "matchesFocusVisible",
    ()=>matchesFocusVisible
]);
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$floating$2d$ui$2b$utils$40$0$2e$2$2e$11$2f$node_modules$2f40$floating$2d$ui$2f$utils$2f$dist$2f$floating$2d$ui$2e$utils$2e$dom$2e$mjs__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@floating-ui+utils@0.2.11/node_modules/@floating-ui/utils/dist/floating-ui.utils.dom.mjs [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$detectBrowser$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+utils@0.2.5+0ea9ec2a211d4613/node_modules/@base-ui/utils/esm/detectBrowser.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$constants$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/floating-ui-react/utils/constants.js [app-client] (ecmascript)");
;
;
;
function activeElement(doc) {
    let element = doc.activeElement;
    while(element?.shadowRoot?.activeElement != null){
        element = element.shadowRoot.activeElement;
    }
    return element;
}
function contains(parent, child) {
    if (!parent || !child) {
        return false;
    }
    const rootNode = child.getRootNode?.();
    // First, attempt with faster native method
    if (parent.contains(child)) {
        return true;
    }
    // then fallback to custom implementation with Shadow DOM support
    if (rootNode && (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$floating$2d$ui$2b$utils$40$0$2e$2$2e$11$2f$node_modules$2f40$floating$2d$ui$2f$utils$2f$dist$2f$floating$2d$ui$2e$utils$2e$dom$2e$mjs__$5b$app$2d$client$5d$__$28$ecmascript$29$__["isShadowRoot"])(rootNode)) {
        let next = child;
        while(next){
            if (parent === next) {
                return true;
            }
            next = next.parentNode || next.host;
        }
    }
    // Give up, the result is false
    return false;
}
function isTargetInsideEnabledTrigger(target, triggerElements) {
    if (!(0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$floating$2d$ui$2b$utils$40$0$2e$2$2e$11$2f$node_modules$2f40$floating$2d$ui$2f$utils$2f$dist$2f$floating$2d$ui$2e$utils$2e$dom$2e$mjs__$5b$app$2d$client$5d$__$28$ecmascript$29$__["isElement"])(target)) {
        return false;
    }
    const targetElement = target;
    if (triggerElements.hasElement(targetElement)) {
        return !targetElement.hasAttribute('data-trigger-disabled');
    }
    for (const [, trigger] of triggerElements.entries()){
        if (contains(trigger, targetElement)) {
            return !trigger.hasAttribute('data-trigger-disabled');
        }
    }
    return false;
}
function getTarget(event) {
    if ('composedPath' in event) {
        return event.composedPath()[0];
    }
    // TS thinks `event` is of type never as it assumes all browsers support
    // `composedPath()`, but browsers without shadow DOM don't.
    return event.target;
}
function isEventTargetWithin(event, node) {
    if (node == null) {
        return false;
    }
    if ('composedPath' in event) {
        return event.composedPath().includes(node);
    }
    // TS thinks `event` is of type never as it assumes all browsers support composedPath, but browsers without shadow dom don't
    const eventAgain = event;
    return eventAgain.target != null && node.contains(eventAgain.target);
}
function isRootElement(element) {
    return element.matches('html,body');
}
function isTypeableElement(element) {
    return (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$floating$2d$ui$2b$utils$40$0$2e$2$2e$11$2f$node_modules$2f40$floating$2d$ui$2f$utils$2f$dist$2f$floating$2d$ui$2e$utils$2e$dom$2e$mjs__$5b$app$2d$client$5d$__$28$ecmascript$29$__["isHTMLElement"])(element) && element.matches(__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$constants$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["TYPEABLE_SELECTOR"]);
}
function isTypeableCombobox(element) {
    if (!element) {
        return false;
    }
    return element.getAttribute('role') === 'combobox' && isTypeableElement(element);
}
function matchesFocusVisible(element) {
    // We don't want to block focus from working with `visibleOnly`
    // (JSDOM doesn't match `:focus-visible` when the element has `:focus`)
    if (!element || __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$detectBrowser$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["isJSDOM"]) {
        return true;
    }
    try {
        return element.matches(':focus-visible');
    } catch (_e) {
        return true;
    }
}
function getFloatingFocusElement(floatingElement) {
    if (!floatingElement) {
        return null;
    }
    // Try to find the element that has `{...getFloatingProps()}` spread on it.
    // This indicates the floating element is acting as a positioning wrapper, and
    // so focus should be managed on the child element with the event handlers and
    // aria props.
    return floatingElement.hasAttribute(__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$constants$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["FOCUSABLE_ATTRIBUTE"]) ? floatingElement : floatingElement.querySelector(`[${__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$constants$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["FOCUSABLE_ATTRIBUTE"]}]`) || floatingElement;
}
}),
"[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/floating-ui-react/utils/event.js [app-client] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "isClickLikeEvent",
    ()=>isClickLikeEvent,
    "isMouseLikePointerType",
    ()=>isMouseLikePointerType,
    "isReactEvent",
    ()=>isReactEvent,
    "isVirtualClick",
    ()=>isVirtualClick,
    "isVirtualPointerEvent",
    ()=>isVirtualPointerEvent,
    "stopEvent",
    ()=>stopEvent
]);
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$detectBrowser$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+utils@0.2.5+0ea9ec2a211d4613/node_modules/@base-ui/utils/esm/detectBrowser.js [app-client] (ecmascript)");
;
function stopEvent(event) {
    event.preventDefault();
    event.stopPropagation();
}
function isReactEvent(event) {
    return 'nativeEvent' in event;
}
function isVirtualClick(event) {
    if (event.pointerType === '' && event.isTrusted) {
        return true;
    }
    if (__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$detectBrowser$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["isAndroid"] && event.pointerType) {
        return event.type === 'click' && event.buttons === 1;
    }
    return event.detail === 0 && !event.pointerType;
}
function isVirtualPointerEvent(event) {
    if (__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$detectBrowser$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["isJSDOM"]) {
        return false;
    }
    return !__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$detectBrowser$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["isAndroid"] && event.width === 0 && event.height === 0 || __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$detectBrowser$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["isAndroid"] && event.width === 1 && event.height === 1 && event.pressure === 0 && event.detail === 0 && event.pointerType === 'mouse' || // iOS VoiceOver returns 0.333• for width/height.
    event.width < 1 && event.height < 1 && event.pressure === 0 && event.detail === 0 && event.pointerType === 'touch';
}
function isMouseLikePointerType(pointerType, strict) {
    // On some Linux machines with Chromium, mouse inputs return a `pointerType`
    // of "pen": https://github.com/floating-ui/floating-ui/issues/2015
    const values = [
        'mouse',
        'pen'
    ];
    if (!strict) {
        values.push('', undefined);
    }
    return values.includes(pointerType);
}
function isClickLikeEvent(event) {
    const type = event.type;
    return type === 'click' || type === 'mousedown' || type === 'keydown' || type === 'keyup';
}
}),
"[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/floating-ui-react/hooks/useClientPoint.js [app-client] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "useClientPoint",
    ()=>useClientPoint
]);
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/compiled/react/index.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$floating$2d$ui$2b$utils$40$0$2e$2$2e$11$2f$node_modules$2f40$floating$2d$ui$2f$utils$2f$dist$2f$floating$2d$ui$2e$utils$2e$dom$2e$mjs__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@floating-ui+utils@0.2.11/node_modules/@floating-ui/utils/dist/floating-ui.utils.dom.mjs [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useStableCallback$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+utils@0.2.5+0ea9ec2a211d4613/node_modules/@base-ui/utils/esm/useStableCallback.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$element$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/floating-ui-react/utils/element.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$event$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/floating-ui-react/utils/event.js [app-client] (ecmascript)");
'use client';
;
;
;
;
function createVirtualElement(domElement, data) {
    let offsetX = null;
    let offsetY = null;
    let isAutoUpdateEvent = false;
    return {
        contextElement: domElement || undefined,
        getBoundingClientRect () {
            const domRect = domElement?.getBoundingClientRect() || {
                width: 0,
                height: 0,
                x: 0,
                y: 0
            };
            const isXAxis = data.axis === 'x' || data.axis === 'both';
            const isYAxis = data.axis === 'y' || data.axis === 'both';
            const canTrackCursorOnAutoUpdate = [
                'mouseenter',
                'mousemove'
            ].includes(data.dataRef.current.openEvent?.type || '') && data.pointerType !== 'touch';
            let width = domRect.width;
            let height = domRect.height;
            let x = domRect.x;
            let y = domRect.y;
            if (offsetX == null && data.x && isXAxis) {
                offsetX = domRect.x - data.x;
            }
            if (offsetY == null && data.y && isYAxis) {
                offsetY = domRect.y - data.y;
            }
            x -= offsetX || 0;
            y -= offsetY || 0;
            width = 0;
            height = 0;
            if (!isAutoUpdateEvent || canTrackCursorOnAutoUpdate) {
                width = data.axis === 'y' ? domRect.width : 0;
                height = data.axis === 'x' ? domRect.height : 0;
                x = isXAxis && data.x != null ? data.x : x;
                y = isYAxis && data.y != null ? data.y : y;
            } else if (isAutoUpdateEvent && !canTrackCursorOnAutoUpdate) {
                height = data.axis === 'x' ? domRect.height : height;
                width = data.axis === 'y' ? domRect.width : width;
            }
            isAutoUpdateEvent = true;
            return {
                width,
                height,
                x,
                y,
                top: y,
                right: x + width,
                bottom: y + height,
                left: x
            };
        }
    };
}
function isMouseBasedEvent(event) {
    return event != null && event.clientX != null;
}
function useClientPoint(context, props = {}) {
    const store = 'rootStore' in context ? context.rootStore : context;
    const open = store.useState('open');
    const floating = store.useState('floatingElement');
    const domReference = store.useState('domReferenceElement');
    const dataRef = store.context.dataRef;
    const { enabled = true, axis = 'both' } = props;
    const initialRef = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useRef"](false);
    const cleanupListenerRef = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useRef"](null);
    const [pointerType, setPointerType] = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useState"]();
    const [reactive, setReactive] = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useState"]([]);
    const setReference = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useStableCallback$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useStableCallback"])({
        "useClientPoint.useStableCallback[setReference]": (newX, newY, referenceElement)=>{
            if (initialRef.current) {
                return;
            }
            // Prevent setting if the open event was not a mouse-like one
            // (e.g. focus to open, then hover over the reference element).
            // Only apply if the event exists.
            if (dataRef.current.openEvent && !isMouseBasedEvent(dataRef.current.openEvent)) {
                return;
            }
            store.set('positionReference', createVirtualElement(referenceElement ?? domReference, {
                x: newX,
                y: newY,
                axis,
                dataRef,
                pointerType
            }));
        }
    }["useClientPoint.useStableCallback[setReference]"]);
    const handleReferenceEnterOrMove = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useStableCallback$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useStableCallback"])({
        "useClientPoint.useStableCallback[handleReferenceEnterOrMove]": (event)=>{
            if (!open) {
                setReference(event.clientX, event.clientY, event.currentTarget);
            } else if (!cleanupListenerRef.current) {
                // If there's no cleanup, there's no listener, but we want to ensure
                // we add the listener if the cursor landed on the floating element and
                // then back on the reference (i.e. it's interactive).
                setReactive([]);
            }
        }
    }["useClientPoint.useStableCallback[handleReferenceEnterOrMove]"]);
    // If the pointer is a mouse-like pointer, we want to continue following the
    // mouse even if the floating element is transitioning out. On touch
    // devices, this is undesirable because the floating element will move to
    // the dismissal touch point.
    const openCheck = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$event$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["isMouseLikePointerType"])(pointerType) ? floating : open;
    const addListener = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useCallback"]({
        "useClientPoint.useCallback[addListener]": ()=>{
            if (!openCheck || !enabled) {
                return undefined;
            }
            const win = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$floating$2d$ui$2b$utils$40$0$2e$2$2e$11$2f$node_modules$2f40$floating$2d$ui$2f$utils$2f$dist$2f$floating$2d$ui$2e$utils$2e$dom$2e$mjs__$5b$app$2d$client$5d$__$28$ecmascript$29$__["getWindow"])(floating);
            function handleMouseMove(event) {
                const target = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$element$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["getTarget"])(event);
                if (!(0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$element$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["contains"])(floating, target)) {
                    setReference(event.clientX, event.clientY);
                } else {
                    win.removeEventListener('mousemove', handleMouseMove);
                    cleanupListenerRef.current = null;
                }
            }
            if (!dataRef.current.openEvent || isMouseBasedEvent(dataRef.current.openEvent)) {
                win.addEventListener('mousemove', handleMouseMove);
                const cleanup = {
                    "useClientPoint.useCallback[addListener].cleanup": ()=>{
                        win.removeEventListener('mousemove', handleMouseMove);
                        cleanupListenerRef.current = null;
                    }
                }["useClientPoint.useCallback[addListener].cleanup"];
                cleanupListenerRef.current = cleanup;
                return cleanup;
            }
            store.set('positionReference', domReference);
            return undefined;
        }
    }["useClientPoint.useCallback[addListener]"], [
        openCheck,
        enabled,
        floating,
        dataRef,
        domReference,
        store,
        setReference
    ]);
    __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useEffect"]({
        "useClientPoint.useEffect": ()=>{
            return addListener();
        }
    }["useClientPoint.useEffect"], [
        addListener,
        reactive
    ]);
    __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useEffect"]({
        "useClientPoint.useEffect": ()=>{
            if (enabled && !floating) {
                initialRef.current = false;
            }
        }
    }["useClientPoint.useEffect"], [
        enabled,
        floating
    ]);
    __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useEffect"]({
        "useClientPoint.useEffect": ()=>{
            if (!enabled && open) {
                initialRef.current = true;
            }
        }
    }["useClientPoint.useEffect"], [
        enabled,
        open
    ]);
    const reference = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useMemo"]({
        "useClientPoint.useMemo[reference]": ()=>{
            function setPointerTypeRef(event) {
                setPointerType(event.pointerType);
            }
            return {
                onPointerDown: setPointerTypeRef,
                onPointerEnter: setPointerTypeRef,
                onMouseMove: handleReferenceEnterOrMove,
                onMouseEnter: handleReferenceEnterOrMove
            };
        }
    }["useClientPoint.useMemo[reference]"], [
        handleReferenceEnterOrMove
    ]);
    return __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useMemo"]({
        "useClientPoint.useMemo": ()=>enabled ? {
                reference,
                trigger: reference
            } : {}
    }["useClientPoint.useMemo"], [
        enabled,
        reference
    ]);
}
}),
"[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/floating-ui-react/utils/nodes.js [app-client] (ecmascript)", ((__turbopack_context__) => {
"use strict";

/* eslint-disable @typescript-eslint/no-loop-func */ __turbopack_context__.s([
    "getDeepestNode",
    ()=>getDeepestNode,
    "getNodeAncestors",
    ()=>getNodeAncestors,
    "getNodeChildren",
    ()=>getNodeChildren
]);
function getNodeChildren(nodes, id, onlyOpenChildren = true) {
    const directChildren = nodes.filter((node)=>node.parentId === id && (!onlyOpenChildren || node.context?.open));
    return directChildren.flatMap((child)=>[
            child,
            ...getNodeChildren(nodes, child.id, onlyOpenChildren)
        ]);
}
function getDeepestNode(nodes, id) {
    let deepestNodeId;
    let maxDepth = -1;
    function findDeepest(nodeId, depth) {
        if (depth > maxDepth) {
            deepestNodeId = nodeId;
            maxDepth = depth;
        }
        const children = getNodeChildren(nodes, nodeId);
        children.forEach((child)=>{
            findDeepest(child.id, depth + 1);
        });
    }
    findDeepest(id, 0);
    return nodes.find((node)=>node.id === deepestNodeId);
}
function getNodeAncestors(nodes, id) {
    let allAncestors = [];
    let currentParentId = nodes.find((node)=>node.id === id)?.parentId;
    while(currentParentId){
        const currentNode = nodes.find((node)=>node.id === currentParentId);
        currentParentId = currentNode?.parentId;
        if (currentNode) {
            allAncestors = allAncestors.concat(currentNode);
        }
    }
    return allAncestors;
}
}),
"[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/floating-ui-react/utils/createEventEmitter.js [app-client] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "createEventEmitter",
    ()=>createEventEmitter
]);
function createEventEmitter() {
    const map = new Map();
    return {
        emit (event, data) {
            map.get(event)?.forEach((listener)=>listener(data));
        },
        on (event, listener) {
            if (!map.has(event)) {
                map.set(event, new Set());
            }
            map.get(event).add(listener);
        },
        off (event, listener) {
            map.get(event)?.delete(listener);
        }
    };
}
}),
"[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/floating-ui-react/components/FloatingTreeStore.js [app-client] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "FloatingTreeStore",
    ()=>FloatingTreeStore
]);
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$createEventEmitter$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/floating-ui-react/utils/createEventEmitter.js [app-client] (ecmascript)");
;
class FloatingTreeStore {
    nodesRef = {
        current: []
    };
    events = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$createEventEmitter$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["createEventEmitter"])();
    addNode(node) {
        this.nodesRef.current.push(node);
    }
    removeNode(node) {
        const index = this.nodesRef.current.findIndex((n)=>n === node);
        if (index !== -1) {
            this.nodesRef.current.splice(index, 1);
        }
    }
}
}),
"[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/floating-ui-react/components/FloatingTree.js [app-client] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "FloatingNode",
    ()=>FloatingNode,
    "FloatingTree",
    ()=>FloatingTree,
    "useFloatingNodeId",
    ()=>useFloatingNodeId,
    "useFloatingParentNodeId",
    ()=>useFloatingParentNodeId,
    "useFloatingTree",
    ()=>useFloatingTree
]);
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$build$2f$polyfills$2f$process$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = /*#__PURE__*/ __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/build/polyfills/process.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/compiled/react/index.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useId$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+utils@0.2.5+0ea9ec2a211d4613/node_modules/@base-ui/utils/esm/useId.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useIsoLayoutEffect$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+utils@0.2.5+0ea9ec2a211d4613/node_modules/@base-ui/utils/esm/useIsoLayoutEffect.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useRefWithInit$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+utils@0.2.5+0ea9ec2a211d4613/node_modules/@base-ui/utils/esm/useRefWithInit.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$components$2f$FloatingTreeStore$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/floating-ui-react/components/FloatingTreeStore.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/compiled/react/jsx-runtime.js [app-client] (ecmascript)");
'use client';
;
;
;
;
;
;
const FloatingNodeContext = /*#__PURE__*/ __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["createContext"](null);
if ("TURBOPACK compile-time truthy", 1) FloatingNodeContext.displayName = "FloatingNodeContext";
const FloatingTreeContext = /*#__PURE__*/ __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["createContext"](null);
/**
 * Returns the parent node id for nested floating elements, if available.
 * Returns `null` for top-level floating elements.
 */ if ("TURBOPACK compile-time truthy", 1) FloatingTreeContext.displayName = "FloatingTreeContext";
const useFloatingParentNodeId = ()=>__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useContext"](FloatingNodeContext)?.id || null;
const useFloatingTree = (externalTree)=>{
    const contextTree = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useContext"](FloatingTreeContext);
    return externalTree ?? contextTree;
};
function useFloatingNodeId(externalTree) {
    const id = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useId$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useId"])();
    const tree = useFloatingTree(externalTree);
    const parentId = useFloatingParentNodeId();
    (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useIsoLayoutEffect$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useIsoLayoutEffect"])({
        "useFloatingNodeId.useIsoLayoutEffect": ()=>{
            if (!id) {
                return undefined;
            }
            const node = {
                id,
                parentId
            };
            tree?.addNode(node);
            return ({
                "useFloatingNodeId.useIsoLayoutEffect": ()=>{
                    tree?.removeNode(node);
                }
            })["useFloatingNodeId.useIsoLayoutEffect"];
        }
    }["useFloatingNodeId.useIsoLayoutEffect"], [
        tree,
        id,
        parentId
    ]);
    return id;
}
function FloatingNode(props) {
    const { children, id } = props;
    const parentId = useFloatingParentNodeId();
    return /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsx"])(FloatingNodeContext.Provider, {
        value: __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useMemo"]({
            "FloatingNode.useMemo": ()=>({
                    id,
                    parentId
                })
        }["FloatingNode.useMemo"], [
            id,
            parentId
        ]),
        children: children
    });
}
function FloatingTree(props) {
    const { children, externalTree } = props;
    const tree = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useRefWithInit$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useRefWithInit"])({
        "FloatingTree.useRefWithInit": ()=>externalTree ?? new __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$components$2f$FloatingTreeStore$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["FloatingTreeStore"]()
    }["FloatingTree.useRefWithInit"]).current;
    return /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsx"])(FloatingTreeContext.Provider, {
        value: tree,
        children: children
    });
}
}),
"[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/utils/createBaseUIEventDetails.js [app-client] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "createChangeEventDetails",
    ()=>createChangeEventDetails,
    "createGenericEventDetails",
    ()=>createGenericEventDetails
]);
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$empty$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+utils@0.2.5+0ea9ec2a211d4613/node_modules/@base-ui/utils/esm/empty.js [app-client] (ecmascript)");
;
;
function createChangeEventDetails(reason, event, trigger, customProperties) {
    let canceled = false;
    let allowPropagation = false;
    const custom = customProperties ?? __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$empty$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["EMPTY_OBJECT"];
    const details = {
        reason,
        event: event ?? new Event('base-ui'),
        cancel () {
            canceled = true;
        },
        allowPropagation () {
            allowPropagation = true;
        },
        get isCanceled () {
            return canceled;
        },
        get isPropagationAllowed () {
            return allowPropagation;
        },
        trigger,
        ...custom
    };
    return details;
}
function createGenericEventDetails(reason, event, customProperties) {
    const custom = customProperties ?? __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$empty$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["EMPTY_OBJECT"];
    const details = {
        reason,
        event: event ?? new Event('base-ui'),
        ...custom
    };
    return details;
}
}),
"[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/utils/reason-parts.js [app-client] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "cancelOpen",
    ()=>cancelOpen,
    "chipRemovePress",
    ()=>chipRemovePress,
    "clearPress",
    ()=>clearPress,
    "closePress",
    ()=>closePress,
    "closeWatcher",
    ()=>closeWatcher,
    "decrementPress",
    ()=>decrementPress,
    "disabled",
    ()=>disabled,
    "drag",
    ()=>drag,
    "escapeKey",
    ()=>escapeKey,
    "focusOut",
    ()=>focusOut,
    "imperativeAction",
    ()=>imperativeAction,
    "incrementPress",
    ()=>incrementPress,
    "inputBlur",
    ()=>inputBlur,
    "inputChange",
    ()=>inputChange,
    "inputClear",
    ()=>inputClear,
    "inputPaste",
    ()=>inputPaste,
    "inputPress",
    ()=>inputPress,
    "itemPress",
    ()=>itemPress,
    "keyboard",
    ()=>keyboard,
    "linkPress",
    ()=>linkPress,
    "listNavigation",
    ()=>listNavigation,
    "none",
    ()=>none,
    "outsidePress",
    ()=>outsidePress,
    "pointer",
    ()=>pointer,
    "scrub",
    ()=>scrub,
    "siblingOpen",
    ()=>siblingOpen,
    "swipe",
    ()=>swipe,
    "trackPress",
    ()=>trackPress,
    "triggerFocus",
    ()=>triggerFocus,
    "triggerHover",
    ()=>triggerHover,
    "triggerPress",
    ()=>triggerPress,
    "wheel",
    ()=>wheel,
    "windowResize",
    ()=>windowResize
]);
const none = 'none';
const triggerPress = 'trigger-press';
const triggerHover = 'trigger-hover';
const triggerFocus = 'trigger-focus';
const outsidePress = 'outside-press';
const itemPress = 'item-press';
const closePress = 'close-press';
const linkPress = 'link-press';
const clearPress = 'clear-press';
const chipRemovePress = 'chip-remove-press';
const trackPress = 'track-press';
const incrementPress = 'increment-press';
const decrementPress = 'decrement-press';
const inputChange = 'input-change';
const inputClear = 'input-clear';
const inputBlur = 'input-blur';
const inputPaste = 'input-paste';
const inputPress = 'input-press';
const focusOut = 'focus-out';
const escapeKey = 'escape-key';
const closeWatcher = 'close-watcher';
const listNavigation = 'list-navigation';
const keyboard = 'keyboard';
const pointer = 'pointer';
const drag = 'drag';
const wheel = 'wheel';
const scrub = 'scrub';
const cancelOpen = 'cancel-open';
const siblingOpen = 'sibling-open';
const disabled = 'disabled';
const imperativeAction = 'imperative-action';
const swipe = 'swipe';
const windowResize = 'window-resize';
}),
"[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/utils/reason-parts.js [app-client] (ecmascript) <export * as REASONS>", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "REASONS",
    ()=>__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$reason$2d$parts$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__
]);
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$reason$2d$parts$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/utils/reason-parts.js [app-client] (ecmascript)");
}),
"[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/floating-ui-react/utils/createAttribute.js [app-client] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "createAttribute",
    ()=>createAttribute
]);
function createAttribute(name) {
    return `data-base-ui-${name}`;
}
}),
"[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/floating-ui-react/hooks/useDismiss.js [app-client] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "normalizeProp",
    ()=>normalizeProp,
    "useDismiss",
    ()=>useDismiss
]);
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/compiled/react/index.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$floating$2d$ui$2b$utils$40$0$2e$2$2e$11$2f$node_modules$2f40$floating$2d$ui$2f$utils$2f$dist$2f$floating$2d$ui$2e$utils$2e$dom$2e$mjs__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@floating-ui+utils@0.2.11/node_modules/@floating-ui/utils/dist/floating-ui.utils.dom.mjs [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useTimeout$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+utils@0.2.5+0ea9ec2a211d4613/node_modules/@base-ui/utils/esm/useTimeout.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useStableCallback$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+utils@0.2.5+0ea9ec2a211d4613/node_modules/@base-ui/utils/esm/useStableCallback.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$owner$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$locals$3e$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+utils@0.2.5+0ea9ec2a211d4613/node_modules/@base-ui/utils/esm/owner.js [app-client] (ecmascript) <locals>");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$element$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/floating-ui-react/utils/element.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$event$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/floating-ui-react/utils/event.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$nodes$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/floating-ui-react/utils/nodes.js [app-client] (ecmascript)");
/* eslint-disable no-underscore-dangle */ var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$components$2f$FloatingTree$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/floating-ui-react/components/FloatingTree.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$createBaseUIEventDetails$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/utils/createBaseUIEventDetails.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$reason$2d$parts$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$export__$2a$__as__REASONS$3e$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/utils/reason-parts.js [app-client] (ecmascript) <export * as REASONS>");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$createAttribute$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/floating-ui-react/utils/createAttribute.js [app-client] (ecmascript)");
'use client';
;
;
;
;
;
;
;
;
;
;
;
const bubbleHandlerKeys = {
    intentional: 'onClick',
    sloppy: 'onPointerDown'
};
function normalizeProp(normalizable) {
    return {
        escapeKey: typeof normalizable === 'boolean' ? normalizable : normalizable?.escapeKey ?? false,
        outsidePress: typeof normalizable === 'boolean' ? normalizable : normalizable?.outsidePress ?? true
    };
}
function useDismiss(context, props = {}) {
    const store = 'rootStore' in context ? context.rootStore : context;
    const open = store.useState('open');
    const floatingElement = store.useState('floatingElement');
    const referenceElement = store.useState('referenceElement');
    const domReferenceElement = store.useState('domReferenceElement');
    const { onOpenChange, dataRef } = store.context;
    const { enabled = true, escapeKey = true, outsidePress: outsidePressProp = true, outsidePressEvent = 'sloppy', referencePress = false, referencePressEvent = 'sloppy', ancestorScroll = false, bubbles, externalTree } = props;
    const tree = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$components$2f$FloatingTree$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useFloatingTree"])(externalTree);
    const outsidePressFn = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useStableCallback$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useStableCallback"])(typeof outsidePressProp === 'function' ? outsidePressProp : ({
        "useDismiss.useStableCallback[outsidePressFn]": ()=>false
    })["useDismiss.useStableCallback[outsidePressFn]"]);
    const outsidePress = typeof outsidePressProp === 'function' ? outsidePressFn : outsidePressProp;
    const endedOrStartedInsideRef = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useRef"](false);
    const { escapeKey: escapeKeyBubbles, outsidePress: outsidePressBubbles } = normalizeProp(bubbles);
    const touchStateRef = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useRef"](null);
    const cancelDismissOnEndTimeout = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useTimeout$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useTimeout"])();
    const clearInsideReactTreeTimeout = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useTimeout$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useTimeout"])();
    const clearInsideReactTree = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useStableCallback$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useStableCallback"])({
        "useDismiss.useStableCallback[clearInsideReactTree]": ()=>{
            clearInsideReactTreeTimeout.clear();
            dataRef.current.insideReactTree = false;
        }
    }["useDismiss.useStableCallback[clearInsideReactTree]"]);
    const isComposingRef = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useRef"](false);
    const currentPointerTypeRef = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useRef"]('');
    const trackPointerType = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useStableCallback$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useStableCallback"])({
        "useDismiss.useStableCallback[trackPointerType]": (event)=>{
            currentPointerTypeRef.current = event.pointerType;
        }
    }["useDismiss.useStableCallback[trackPointerType]"]);
    const getOutsidePressEvent = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useStableCallback$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useStableCallback"])({
        "useDismiss.useStableCallback[getOutsidePressEvent]": ()=>{
            const type = currentPointerTypeRef.current;
            const computedType = type === 'pen' || !type ? 'mouse' : type;
            const resolved = typeof outsidePressEvent === 'function' ? outsidePressEvent() : outsidePressEvent;
            if (typeof resolved === 'string') {
                return resolved;
            }
            return resolved[computedType];
        }
    }["useDismiss.useStableCallback[getOutsidePressEvent]"]);
    const closeOnEscapeKeyDown = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useStableCallback$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useStableCallback"])({
        "useDismiss.useStableCallback[closeOnEscapeKeyDown]": (event)=>{
            if (!open || !enabled || !escapeKey || event.key !== 'Escape') {
                return;
            }
            // Wait until IME is settled. Pressing `Escape` while composing should
            // close the compose menu, but not the floating element.
            if (isComposingRef.current) {
                return;
            }
            const nodeId = dataRef.current.floatingContext?.nodeId;
            const children = tree ? (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$nodes$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["getNodeChildren"])(tree.nodesRef.current, nodeId) : [];
            if (!escapeKeyBubbles) {
                if (children.length > 0) {
                    let shouldDismiss = true;
                    children.forEach({
                        "useDismiss.useStableCallback[closeOnEscapeKeyDown]": (child)=>{
                            if (child.context?.open && !child.context.dataRef.current.__escapeKeyBubbles) {
                                shouldDismiss = false;
                            }
                        }
                    }["useDismiss.useStableCallback[closeOnEscapeKeyDown]"]);
                    if (!shouldDismiss) {
                        return;
                    }
                }
            }
            const native = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$event$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["isReactEvent"])(event) ? event.nativeEvent : event;
            const eventDetails = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$createBaseUIEventDetails$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["createChangeEventDetails"])(__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$reason$2d$parts$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$export__$2a$__as__REASONS$3e$__["REASONS"].escapeKey, native);
            store.setOpen(false, eventDetails);
            if (!escapeKeyBubbles && !eventDetails.isPropagationAllowed) {
                event.stopPropagation();
            }
        }
    }["useDismiss.useStableCallback[closeOnEscapeKeyDown]"]);
    const shouldIgnoreEvent = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useStableCallback$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useStableCallback"])({
        "useDismiss.useStableCallback[shouldIgnoreEvent]": (event)=>{
            const computedOutsidePressEvent = getOutsidePressEvent();
            return computedOutsidePressEvent === 'intentional' && event.type !== 'click' || computedOutsidePressEvent === 'sloppy' && event.type === 'click';
        }
    }["useDismiss.useStableCallback[shouldIgnoreEvent]"]);
    const markInsideReactTree = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useStableCallback$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useStableCallback"])({
        "useDismiss.useStableCallback[markInsideReactTree]": ()=>{
            dataRef.current.insideReactTree = true;
            clearInsideReactTreeTimeout.start(0, clearInsideReactTree);
        }
    }["useDismiss.useStableCallback[markInsideReactTree]"]);
    const closeOnPressOutside = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useStableCallback$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useStableCallback"])({
        "useDismiss.useStableCallback[closeOnPressOutside]": (event, endedOrStartedInside = false)=>{
            if (shouldIgnoreEvent(event)) {
                clearInsideReactTree();
                return;
            }
            if (dataRef.current.insideReactTree) {
                clearInsideReactTree();
                return;
            }
            if (getOutsidePressEvent() === 'intentional' && endedOrStartedInside) {
                return;
            }
            if (typeof outsidePress === 'function' && !outsidePress(event)) {
                return;
            }
            const target = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$element$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["getTarget"])(event);
            const inertSelector = `[${(0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$createAttribute$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["createAttribute"])('inert')}]`;
            const markers = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$owner$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$locals$3e$__["ownerDocument"])(store.select('floatingElement')).querySelectorAll(inertSelector);
            const triggers = store.context.triggerElements;
            // If another trigger is clicked, don't close the floating element.
            if (target && (triggers.hasElement(target) || triggers.hasMatchingElement({
                "useDismiss.useStableCallback[closeOnPressOutside]": (trigger)=>(0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$element$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["contains"])(trigger, target)
            }["useDismiss.useStableCallback[closeOnPressOutside]"]))) {
                return;
            }
            let targetRootAncestor = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$floating$2d$ui$2b$utils$40$0$2e$2$2e$11$2f$node_modules$2f40$floating$2d$ui$2f$utils$2f$dist$2f$floating$2d$ui$2e$utils$2e$dom$2e$mjs__$5b$app$2d$client$5d$__$28$ecmascript$29$__["isElement"])(target) ? target : null;
            while(targetRootAncestor && !(0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$floating$2d$ui$2b$utils$40$0$2e$2$2e$11$2f$node_modules$2f40$floating$2d$ui$2f$utils$2f$dist$2f$floating$2d$ui$2e$utils$2e$dom$2e$mjs__$5b$app$2d$client$5d$__$28$ecmascript$29$__["isLastTraversableNode"])(targetRootAncestor)){
                const nextParent = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$floating$2d$ui$2b$utils$40$0$2e$2$2e$11$2f$node_modules$2f40$floating$2d$ui$2f$utils$2f$dist$2f$floating$2d$ui$2e$utils$2e$dom$2e$mjs__$5b$app$2d$client$5d$__$28$ecmascript$29$__["getParentNode"])(targetRootAncestor);
                if ((0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$floating$2d$ui$2b$utils$40$0$2e$2$2e$11$2f$node_modules$2f40$floating$2d$ui$2f$utils$2f$dist$2f$floating$2d$ui$2e$utils$2e$dom$2e$mjs__$5b$app$2d$client$5d$__$28$ecmascript$29$__["isLastTraversableNode"])(nextParent) || !(0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$floating$2d$ui$2b$utils$40$0$2e$2$2e$11$2f$node_modules$2f40$floating$2d$ui$2f$utils$2f$dist$2f$floating$2d$ui$2e$utils$2e$dom$2e$mjs__$5b$app$2d$client$5d$__$28$ecmascript$29$__["isElement"])(nextParent)) {
                    break;
                }
                targetRootAncestor = nextParent;
            }
            // Check if the click occurred on a third-party element injected after the
            // floating element rendered.
            if (markers.length && (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$floating$2d$ui$2b$utils$40$0$2e$2$2e$11$2f$node_modules$2f40$floating$2d$ui$2f$utils$2f$dist$2f$floating$2d$ui$2e$utils$2e$dom$2e$mjs__$5b$app$2d$client$5d$__$28$ecmascript$29$__["isElement"])(target) && !(0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$element$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["isRootElement"])(target) && // Clicked on a direct ancestor (e.g. FloatingOverlay).
            !(0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$element$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["contains"])(target, store.select('floatingElement')) && // If the target root element contains none of the markers, then the
            // element was injected after the floating element rendered.
            Array.from(markers).every({
                "useDismiss.useStableCallback[closeOnPressOutside]": (marker)=>!(0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$element$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["contains"])(targetRootAncestor, marker)
            }["useDismiss.useStableCallback[closeOnPressOutside]"])) {
                return;
            }
            // Check if the click occurred on the scrollbar
            // Skip for touch events: scrollbars don't receive touch events on most platforms
            if ((0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$floating$2d$ui$2b$utils$40$0$2e$2$2e$11$2f$node_modules$2f40$floating$2d$ui$2f$utils$2f$dist$2f$floating$2d$ui$2e$utils$2e$dom$2e$mjs__$5b$app$2d$client$5d$__$28$ecmascript$29$__["isHTMLElement"])(target) && !('touches' in event)) {
                const lastTraversableNode = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$floating$2d$ui$2b$utils$40$0$2e$2$2e$11$2f$node_modules$2f40$floating$2d$ui$2f$utils$2f$dist$2f$floating$2d$ui$2e$utils$2e$dom$2e$mjs__$5b$app$2d$client$5d$__$28$ecmascript$29$__["isLastTraversableNode"])(target);
                const style = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$floating$2d$ui$2b$utils$40$0$2e$2$2e$11$2f$node_modules$2f40$floating$2d$ui$2f$utils$2f$dist$2f$floating$2d$ui$2e$utils$2e$dom$2e$mjs__$5b$app$2d$client$5d$__$28$ecmascript$29$__["getComputedStyle"])(target);
                const scrollRe = /auto|scroll/;
                const isScrollableX = lastTraversableNode || scrollRe.test(style.overflowX);
                const isScrollableY = lastTraversableNode || scrollRe.test(style.overflowY);
                const canScrollX = isScrollableX && target.clientWidth > 0 && target.scrollWidth > target.clientWidth;
                const canScrollY = isScrollableY && target.clientHeight > 0 && target.scrollHeight > target.clientHeight;
                const isRTL = style.direction === 'rtl';
                // Check click position relative to scrollbar.
                // In some browsers it is possible to change the <body> (or window)
                // scrollbar to the left side, but is very rare and is difficult to
                // check for. Plus, for modal dialogs with backdrops, it is more
                // important that the backdrop is checked but not so much the window.
                const pressedVerticalScrollbar = canScrollY && (isRTL ? event.offsetX <= target.offsetWidth - target.clientWidth : event.offsetX > target.clientWidth);
                const pressedHorizontalScrollbar = canScrollX && event.offsetY > target.clientHeight;
                if (pressedVerticalScrollbar || pressedHorizontalScrollbar) {
                    return;
                }
            }
            const nodeId = dataRef.current.floatingContext?.nodeId;
            const targetIsInsideChildren = tree && (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$nodes$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["getNodeChildren"])(tree.nodesRef.current, nodeId).some({
                "useDismiss.useStableCallback[closeOnPressOutside]": (node)=>(0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$element$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["isEventTargetWithin"])(event, node.context?.elements.floating)
            }["useDismiss.useStableCallback[closeOnPressOutside]"]);
            if ((0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$element$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["isEventTargetWithin"])(event, store.select('floatingElement')) || (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$element$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["isEventTargetWithin"])(event, store.select('domReferenceElement')) || targetIsInsideChildren) {
                return;
            }
            const children = tree ? (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$nodes$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["getNodeChildren"])(tree.nodesRef.current, nodeId) : [];
            if (children.length > 0) {
                let shouldDismiss = true;
                children.forEach({
                    "useDismiss.useStableCallback[closeOnPressOutside]": (child)=>{
                        if (child.context?.open && !child.context.dataRef.current.__outsidePressBubbles) {
                            shouldDismiss = false;
                        }
                    }
                }["useDismiss.useStableCallback[closeOnPressOutside]"]);
                if (!shouldDismiss) {
                    return;
                }
            }
            store.setOpen(false, (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$createBaseUIEventDetails$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["createChangeEventDetails"])(__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$reason$2d$parts$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$export__$2a$__as__REASONS$3e$__["REASONS"].outsidePress, event));
            clearInsideReactTree();
        }
    }["useDismiss.useStableCallback[closeOnPressOutside]"]);
    const handlePointerDown = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useStableCallback$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useStableCallback"])({
        "useDismiss.useStableCallback[handlePointerDown]": (event)=>{
            if (getOutsidePressEvent() !== 'sloppy' || event.pointerType === 'touch' || !store.select('open') || !enabled || (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$element$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["isEventTargetWithin"])(event, store.select('floatingElement')) || (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$element$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["isEventTargetWithin"])(event, store.select('domReferenceElement'))) {
                return;
            }
            closeOnPressOutside(event);
        }
    }["useDismiss.useStableCallback[handlePointerDown]"]);
    const handleTouchStart = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useStableCallback$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useStableCallback"])({
        "useDismiss.useStableCallback[handleTouchStart]": (event)=>{
            if (getOutsidePressEvent() !== 'sloppy' || !store.select('open') || !enabled || (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$element$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["isEventTargetWithin"])(event, store.select('floatingElement')) || (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$element$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["isEventTargetWithin"])(event, store.select('domReferenceElement'))) {
                return;
            }
            const touch = event.touches[0];
            if (touch) {
                touchStateRef.current = {
                    startTime: Date.now(),
                    startX: touch.clientX,
                    startY: touch.clientY,
                    dismissOnTouchEnd: false,
                    dismissOnMouseDown: true
                };
                cancelDismissOnEndTimeout.start(1000, {
                    "useDismiss.useStableCallback[handleTouchStart]": ()=>{
                        if (touchStateRef.current) {
                            touchStateRef.current.dismissOnTouchEnd = false;
                            touchStateRef.current.dismissOnMouseDown = false;
                        }
                    }
                }["useDismiss.useStableCallback[handleTouchStart]"]);
            }
        }
    }["useDismiss.useStableCallback[handleTouchStart]"]);
    const handleTouchStartCapture = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useStableCallback$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useStableCallback"])({
        "useDismiss.useStableCallback[handleTouchStartCapture]": (event)=>{
            const target = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$element$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["getTarget"])(event);
            function callback() {
                handleTouchStart(event);
                target?.removeEventListener(event.type, callback);
            }
            target?.addEventListener(event.type, callback);
        }
    }["useDismiss.useStableCallback[handleTouchStartCapture]"]);
    const closeOnPressOutsideCapture = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useStableCallback$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useStableCallback"])({
        "useDismiss.useStableCallback[closeOnPressOutsideCapture]": (event)=>{
            // When click outside is lazy (`up` event), handle dragging.
            // Don't close if:
            // - The click started inside the floating element.
            // - The click ended inside the floating element.
            const endedOrStartedInside = endedOrStartedInsideRef.current;
            endedOrStartedInsideRef.current = false;
            cancelDismissOnEndTimeout.clear();
            if (event.type === 'mousedown' && touchStateRef.current && !touchStateRef.current.dismissOnMouseDown) {
                return;
            }
            const target = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$element$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["getTarget"])(event);
            function callback() {
                if (event.type === 'pointerdown') {
                    handlePointerDown(event);
                } else {
                    closeOnPressOutside(event, endedOrStartedInside);
                }
                target?.removeEventListener(event.type, callback);
            }
            target?.addEventListener(event.type, callback);
        }
    }["useDismiss.useStableCallback[closeOnPressOutsideCapture]"]);
    const handleTouchMove = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useStableCallback$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useStableCallback"])({
        "useDismiss.useStableCallback[handleTouchMove]": (event)=>{
            if (getOutsidePressEvent() !== 'sloppy' || !touchStateRef.current || (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$element$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["isEventTargetWithin"])(event, store.select('floatingElement')) || (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$element$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["isEventTargetWithin"])(event, store.select('domReferenceElement'))) {
                return;
            }
            const touch = event.touches[0];
            if (!touch) {
                return;
            }
            const deltaX = Math.abs(touch.clientX - touchStateRef.current.startX);
            const deltaY = Math.abs(touch.clientY - touchStateRef.current.startY);
            const distance = Math.sqrt(deltaX * deltaX + deltaY * deltaY);
            if (distance > 5) {
                touchStateRef.current.dismissOnTouchEnd = true;
            }
            if (distance > 10) {
                closeOnPressOutside(event);
                cancelDismissOnEndTimeout.clear();
                touchStateRef.current = null;
            }
        }
    }["useDismiss.useStableCallback[handleTouchMove]"]);
    const handleTouchMoveCapture = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useStableCallback$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useStableCallback"])({
        "useDismiss.useStableCallback[handleTouchMoveCapture]": (event)=>{
            const target = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$element$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["getTarget"])(event);
            function callback() {
                handleTouchMove(event);
                target?.removeEventListener(event.type, callback);
            }
            target?.addEventListener(event.type, callback);
        }
    }["useDismiss.useStableCallback[handleTouchMoveCapture]"]);
    const handleTouchEnd = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useStableCallback$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useStableCallback"])({
        "useDismiss.useStableCallback[handleTouchEnd]": (event)=>{
            if (getOutsidePressEvent() !== 'sloppy' || !touchStateRef.current || (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$element$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["isEventTargetWithin"])(event, store.select('floatingElement')) || (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$element$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["isEventTargetWithin"])(event, store.select('domReferenceElement'))) {
                return;
            }
            if (touchStateRef.current.dismissOnTouchEnd) {
                closeOnPressOutside(event);
            }
            cancelDismissOnEndTimeout.clear();
            touchStateRef.current = null;
        }
    }["useDismiss.useStableCallback[handleTouchEnd]"]);
    const handleTouchEndCapture = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useStableCallback$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useStableCallback"])({
        "useDismiss.useStableCallback[handleTouchEndCapture]": (event)=>{
            const target = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$element$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["getTarget"])(event);
            function callback() {
                handleTouchEnd(event);
                target?.removeEventListener(event.type, callback);
            }
            target?.addEventListener(event.type, callback);
        }
    }["useDismiss.useStableCallback[handleTouchEndCapture]"]);
    __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useEffect"]({
        "useDismiss.useEffect": ()=>{
            if (!open || !enabled) {
                return undefined;
            }
            dataRef.current.__escapeKeyBubbles = escapeKeyBubbles;
            dataRef.current.__outsidePressBubbles = outsidePressBubbles;
            const compositionTimeout = new __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useTimeout$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["Timeout"]();
            function onScroll(event) {
                store.setOpen(false, (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$createBaseUIEventDetails$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["createChangeEventDetails"])(__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$reason$2d$parts$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$export__$2a$__as__REASONS$3e$__["REASONS"].none, event));
            }
            function handleCompositionStart() {
                compositionTimeout.clear();
                isComposingRef.current = true;
            }
            function handleCompositionEnd() {
                // Safari fires `compositionend` before `keydown`, so we need to wait
                // until the next tick to set `isComposing` to `false`.
                // https://bugs.webkit.org/show_bug.cgi?id=165004
                compositionTimeout.start(// 0ms or 1ms don't work in Safari. 5ms appears to consistently work.
                // Only apply to WebKit for the test to remain 0ms.
                (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$floating$2d$ui$2b$utils$40$0$2e$2$2e$11$2f$node_modules$2f40$floating$2d$ui$2f$utils$2f$dist$2f$floating$2d$ui$2e$utils$2e$dom$2e$mjs__$5b$app$2d$client$5d$__$28$ecmascript$29$__["isWebKit"])() ? 5 : 0, {
                    "useDismiss.useEffect.handleCompositionEnd": ()=>{
                        isComposingRef.current = false;
                    }
                }["useDismiss.useEffect.handleCompositionEnd"]);
            }
            const doc = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$owner$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$locals$3e$__["ownerDocument"])(floatingElement);
            doc.addEventListener('pointerdown', trackPointerType, true);
            if (escapeKey) {
                doc.addEventListener('keydown', closeOnEscapeKeyDown);
                doc.addEventListener('compositionstart', handleCompositionStart);
                doc.addEventListener('compositionend', handleCompositionEnd);
            }
            if (outsidePress) {
                doc.addEventListener('click', closeOnPressOutsideCapture, true);
                doc.addEventListener('pointerdown', closeOnPressOutsideCapture, true);
                doc.addEventListener('touchstart', handleTouchStartCapture, true);
                doc.addEventListener('touchmove', handleTouchMoveCapture, true);
                doc.addEventListener('touchend', handleTouchEndCapture, true);
                doc.addEventListener('mousedown', closeOnPressOutsideCapture, true);
            }
            let ancestors = [];
            if (ancestorScroll) {
                if ((0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$floating$2d$ui$2b$utils$40$0$2e$2$2e$11$2f$node_modules$2f40$floating$2d$ui$2f$utils$2f$dist$2f$floating$2d$ui$2e$utils$2e$dom$2e$mjs__$5b$app$2d$client$5d$__$28$ecmascript$29$__["isElement"])(domReferenceElement)) {
                    ancestors = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$floating$2d$ui$2b$utils$40$0$2e$2$2e$11$2f$node_modules$2f40$floating$2d$ui$2f$utils$2f$dist$2f$floating$2d$ui$2e$utils$2e$dom$2e$mjs__$5b$app$2d$client$5d$__$28$ecmascript$29$__["getOverflowAncestors"])(domReferenceElement);
                }
                if ((0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$floating$2d$ui$2b$utils$40$0$2e$2$2e$11$2f$node_modules$2f40$floating$2d$ui$2f$utils$2f$dist$2f$floating$2d$ui$2e$utils$2e$dom$2e$mjs__$5b$app$2d$client$5d$__$28$ecmascript$29$__["isElement"])(floatingElement)) {
                    ancestors = ancestors.concat((0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$floating$2d$ui$2b$utils$40$0$2e$2$2e$11$2f$node_modules$2f40$floating$2d$ui$2f$utils$2f$dist$2f$floating$2d$ui$2e$utils$2e$dom$2e$mjs__$5b$app$2d$client$5d$__$28$ecmascript$29$__["getOverflowAncestors"])(floatingElement));
                }
                if (!(0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$floating$2d$ui$2b$utils$40$0$2e$2$2e$11$2f$node_modules$2f40$floating$2d$ui$2f$utils$2f$dist$2f$floating$2d$ui$2e$utils$2e$dom$2e$mjs__$5b$app$2d$client$5d$__$28$ecmascript$29$__["isElement"])(referenceElement) && referenceElement && referenceElement.contextElement) {
                    ancestors = ancestors.concat((0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$floating$2d$ui$2b$utils$40$0$2e$2$2e$11$2f$node_modules$2f40$floating$2d$ui$2f$utils$2f$dist$2f$floating$2d$ui$2e$utils$2e$dom$2e$mjs__$5b$app$2d$client$5d$__$28$ecmascript$29$__["getOverflowAncestors"])(referenceElement.contextElement));
                }
            }
            // Ignore the visual viewport for scrolling dismissal (allow pinch-zoom)
            ancestors = ancestors.filter({
                "useDismiss.useEffect": (ancestor)=>ancestor !== doc.defaultView?.visualViewport
            }["useDismiss.useEffect"]);
            ancestors.forEach({
                "useDismiss.useEffect": (ancestor)=>{
                    ancestor.addEventListener('scroll', onScroll, {
                        passive: true
                    });
                }
            }["useDismiss.useEffect"]);
            return ({
                "useDismiss.useEffect": ()=>{
                    doc.removeEventListener('pointerdown', trackPointerType, true);
                    if (escapeKey) {
                        doc.removeEventListener('keydown', closeOnEscapeKeyDown);
                        doc.removeEventListener('compositionstart', handleCompositionStart);
                        doc.removeEventListener('compositionend', handleCompositionEnd);
                    }
                    if (outsidePress) {
                        doc.removeEventListener('click', closeOnPressOutsideCapture, true);
                        doc.removeEventListener('pointerdown', closeOnPressOutsideCapture, true);
                        doc.removeEventListener('touchstart', handleTouchStartCapture, true);
                        doc.removeEventListener('touchmove', handleTouchMoveCapture, true);
                        doc.removeEventListener('touchend', handleTouchEndCapture, true);
                        doc.removeEventListener('mousedown', closeOnPressOutsideCapture, true);
                    }
                    ancestors.forEach({
                        "useDismiss.useEffect": (ancestor)=>{
                            ancestor.removeEventListener('scroll', onScroll);
                        }
                    }["useDismiss.useEffect"]);
                    compositionTimeout.clear();
                    endedOrStartedInsideRef.current = false;
                }
            })["useDismiss.useEffect"];
        }
    }["useDismiss.useEffect"], [
        dataRef,
        floatingElement,
        referenceElement,
        domReferenceElement,
        escapeKey,
        outsidePress,
        open,
        onOpenChange,
        ancestorScroll,
        enabled,
        escapeKeyBubbles,
        outsidePressBubbles,
        closeOnEscapeKeyDown,
        closeOnPressOutside,
        closeOnPressOutsideCapture,
        handlePointerDown,
        handleTouchStartCapture,
        handleTouchMoveCapture,
        handleTouchEndCapture,
        trackPointerType,
        store
    ]);
    __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useEffect"](clearInsideReactTree, [
        outsidePress,
        clearInsideReactTree
    ]);
    const reference = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useMemo"]({
        "useDismiss.useMemo[reference]": ()=>({
                onKeyDown: closeOnEscapeKeyDown,
                ...referencePress && {
                    [bubbleHandlerKeys[referencePressEvent]]: ({
                        "useDismiss.useMemo[reference]": (event)=>{
                            store.setOpen(false, (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$createBaseUIEventDetails$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["createChangeEventDetails"])(__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$reason$2d$parts$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$export__$2a$__as__REASONS$3e$__["REASONS"].triggerPress, event.nativeEvent));
                        }
                    })["useDismiss.useMemo[reference]"],
                    ...referencePressEvent !== 'intentional' && {
                        onClick (event) {
                            store.setOpen(false, (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$createBaseUIEventDetails$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["createChangeEventDetails"])(__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$reason$2d$parts$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$export__$2a$__as__REASONS$3e$__["REASONS"].triggerPress, event.nativeEvent));
                        }
                    }
                }
            })
    }["useDismiss.useMemo[reference]"], [
        closeOnEscapeKeyDown,
        store,
        referencePress,
        referencePressEvent
    ]);
    const handlePressedInside = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useStableCallback$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useStableCallback"])({
        "useDismiss.useStableCallback[handlePressedInside]": (event)=>{
            const target = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$element$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["getTarget"])(event.nativeEvent);
            if (!(0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$element$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["contains"])(store.select('floatingElement'), target) || event.button !== 0) {
                return;
            }
            endedOrStartedInsideRef.current = true;
        }
    }["useDismiss.useStableCallback[handlePressedInside]"]);
    const markPressStartedInsideReactTree = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useStableCallback$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useStableCallback"])({
        "useDismiss.useStableCallback[markPressStartedInsideReactTree]": (event)=>{
            if (!open || !enabled || event.button !== 0) {
                return;
            }
            endedOrStartedInsideRef.current = true;
        }
    }["useDismiss.useStableCallback[markPressStartedInsideReactTree]"]);
    const floating = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useMemo"]({
        "useDismiss.useMemo[floating]": ()=>({
                onKeyDown: closeOnEscapeKeyDown,
                // `onMouseDown` may be blocked if `event.preventDefault()` is called in
                // `onPointerDown`, such as with <NumberField.ScrubArea>.
                // See https://github.com/mui/base-ui/pull/3379
                onPointerDown: handlePressedInside,
                onMouseDown: handlePressedInside,
                onMouseUp: handlePressedInside,
                onClickCapture: markInsideReactTree,
                onMouseDownCapture (event) {
                    markInsideReactTree();
                    markPressStartedInsideReactTree(event);
                },
                onPointerDownCapture (event) {
                    markInsideReactTree();
                    markPressStartedInsideReactTree(event);
                },
                onMouseUpCapture: markInsideReactTree,
                onTouchEndCapture: markInsideReactTree,
                onTouchMoveCapture: markInsideReactTree
            })
    }["useDismiss.useMemo[floating]"], [
        closeOnEscapeKeyDown,
        handlePressedInside,
        markInsideReactTree,
        markPressStartedInsideReactTree
    ]);
    return __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useMemo"]({
        "useDismiss.useMemo": ()=>enabled ? {
                reference,
                floating,
                trigger: reference
            } : {}
    }["useDismiss.useMemo"], [
        enabled,
        reference,
        floating
    ]);
}
}),
"[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/floating-ui-react/hooks/useInteractions.js [app-client] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "useInteractions",
    ()=>useInteractions
]);
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/compiled/react/index.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$constants$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/floating-ui-react/utils/constants.js [app-client] (ecmascript)");
;
;
function useInteractions(propsList = []) {
    const referenceDeps = propsList.map((key)=>key?.reference);
    const floatingDeps = propsList.map((key)=>key?.floating);
    const itemDeps = propsList.map((key)=>key?.item);
    const triggerDeps = propsList.map((key)=>key?.trigger);
    const getReferenceProps = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useCallback"]({
        "useInteractions.useCallback[getReferenceProps]": (userProps)=>mergeProps(userProps, propsList, 'reference')
    }["useInteractions.useCallback[getReferenceProps]"], // eslint-disable-next-line react-hooks/exhaustive-deps
    referenceDeps);
    const getFloatingProps = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useCallback"]({
        "useInteractions.useCallback[getFloatingProps]": (userProps)=>mergeProps(userProps, propsList, 'floating')
    }["useInteractions.useCallback[getFloatingProps]"], // eslint-disable-next-line react-hooks/exhaustive-deps
    floatingDeps);
    const getItemProps = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useCallback"]({
        "useInteractions.useCallback[getItemProps]": (userProps)=>mergeProps(userProps, propsList, 'item')
    }["useInteractions.useCallback[getItemProps]"], // eslint-disable-next-line react-hooks/exhaustive-deps
    itemDeps);
    const getTriggerProps = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useCallback"]({
        "useInteractions.useCallback[getTriggerProps]": (userProps)=>mergeProps(userProps, propsList, 'trigger')
    }["useInteractions.useCallback[getTriggerProps]"], // eslint-disable-next-line react-hooks/exhaustive-deps
    triggerDeps);
    return __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useMemo"]({
        "useInteractions.useMemo": ()=>({
                getReferenceProps,
                getFloatingProps,
                getItemProps,
                getTriggerProps
            })
    }["useInteractions.useMemo"], [
        getReferenceProps,
        getFloatingProps,
        getItemProps,
        getTriggerProps
    ]);
}
/* eslint-disable guard-for-in */ function mergeProps(userProps, propsList, elementKey) {
    const eventHandlers = new Map();
    const isItem = elementKey === 'item';
    const outputProps = {};
    if (elementKey === 'floating') {
        outputProps.tabIndex = -1;
        outputProps[__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$constants$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["FOCUSABLE_ATTRIBUTE"]] = '';
    }
    for(const key in userProps){
        if (isItem && userProps) {
            if (key === __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$constants$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["ACTIVE_KEY"] || key === __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$constants$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["SELECTED_KEY"]) {
                continue;
            }
        }
        outputProps[key] = userProps[key];
    }
    for(let i = 0; i < propsList.length; i += 1){
        let props;
        const propsOrGetProps = propsList[i]?.[elementKey];
        if (typeof propsOrGetProps === 'function') {
            props = userProps ? propsOrGetProps(userProps) : null;
        } else {
            props = propsOrGetProps;
        }
        if (!props) {
            continue;
        }
        mutablyMergeProps(outputProps, props, isItem, eventHandlers);
    }
    mutablyMergeProps(outputProps, userProps, isItem, eventHandlers);
    return outputProps;
}
function mutablyMergeProps(outputProps, props, isItem, eventHandlers) {
    for(const key in props){
        const value = props[key];
        if (isItem && (key === __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$constants$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["ACTIVE_KEY"] || key === __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$constants$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["SELECTED_KEY"])) {
            continue;
        }
        if (!key.startsWith('on')) {
            outputProps[key] = value;
        } else {
            if (!eventHandlers.has(key)) {
                eventHandlers.set(key, []);
            }
            if (typeof value === 'function') {
                eventHandlers.get(key)?.push(value);
                outputProps[key] = (...args)=>{
                    return eventHandlers.get(key)?.map((fn)=>fn(...args)).find((val)=>val !== undefined);
                };
            }
        }
    }
}
}),
"[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/utils/useTransitionStatus.js [app-client] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "useTransitionStatus",
    ()=>useTransitionStatus
]);
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/compiled/react/index.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useIsoLayoutEffect$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+utils@0.2.5+0ea9ec2a211d4613/node_modules/@base-ui/utils/esm/useIsoLayoutEffect.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useAnimationFrame$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+utils@0.2.5+0ea9ec2a211d4613/node_modules/@base-ui/utils/esm/useAnimationFrame.js [app-client] (ecmascript)");
'use client';
;
;
;
function useTransitionStatus(open, enableIdleState = false, deferEndingState = false) {
    const [transitionStatus, setTransitionStatus] = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useState"](open && enableIdleState ? 'idle' : undefined);
    const [mounted, setMounted] = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useState"](open);
    if (open && !mounted) {
        setMounted(true);
        setTransitionStatus('starting');
    }
    if (!open && mounted && transitionStatus !== 'ending' && !deferEndingState) {
        setTransitionStatus('ending');
    }
    if (!open && !mounted && transitionStatus === 'ending') {
        setTransitionStatus(undefined);
    }
    (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useIsoLayoutEffect$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useIsoLayoutEffect"])({
        "useTransitionStatus.useIsoLayoutEffect": ()=>{
            if (!open && mounted && transitionStatus !== 'ending' && deferEndingState) {
                const frame = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useAnimationFrame$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["AnimationFrame"].request({
                    "useTransitionStatus.useIsoLayoutEffect.frame": ()=>{
                        setTransitionStatus('ending');
                    }
                }["useTransitionStatus.useIsoLayoutEffect.frame"]);
                return ({
                    "useTransitionStatus.useIsoLayoutEffect": ()=>{
                        __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useAnimationFrame$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["AnimationFrame"].cancel(frame);
                    }
                })["useTransitionStatus.useIsoLayoutEffect"];
            }
            return undefined;
        }
    }["useTransitionStatus.useIsoLayoutEffect"], [
        open,
        mounted,
        transitionStatus,
        deferEndingState
    ]);
    (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useIsoLayoutEffect$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useIsoLayoutEffect"])({
        "useTransitionStatus.useIsoLayoutEffect": ()=>{
            if (!open || enableIdleState) {
                return undefined;
            }
            const frame = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useAnimationFrame$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["AnimationFrame"].request({
                "useTransitionStatus.useIsoLayoutEffect.frame": ()=>{
                    // Avoid `flushSync` here due to Firefox.
                    // See https://github.com/mui/base-ui/pull/3424
                    setTransitionStatus(undefined);
                }
            }["useTransitionStatus.useIsoLayoutEffect.frame"]);
            return ({
                "useTransitionStatus.useIsoLayoutEffect": ()=>{
                    __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useAnimationFrame$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["AnimationFrame"].cancel(frame);
                }
            })["useTransitionStatus.useIsoLayoutEffect"];
        }
    }["useTransitionStatus.useIsoLayoutEffect"], [
        enableIdleState,
        open
    ]);
    (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useIsoLayoutEffect$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useIsoLayoutEffect"])({
        "useTransitionStatus.useIsoLayoutEffect": ()=>{
            if (!open || !enableIdleState) {
                return undefined;
            }
            if (open && mounted && transitionStatus !== 'idle') {
                setTransitionStatus('starting');
            }
            const frame = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useAnimationFrame$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["AnimationFrame"].request({
                "useTransitionStatus.useIsoLayoutEffect.frame": ()=>{
                    setTransitionStatus('idle');
                }
            }["useTransitionStatus.useIsoLayoutEffect.frame"]);
            return ({
                "useTransitionStatus.useIsoLayoutEffect": ()=>{
                    __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useAnimationFrame$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["AnimationFrame"].cancel(frame);
                }
            })["useTransitionStatus.useIsoLayoutEffect"];
        }
    }["useTransitionStatus.useIsoLayoutEffect"], [
        enableIdleState,
        open,
        mounted,
        setTransitionStatus,
        transitionStatus
    ]);
    return __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useMemo"]({
        "useTransitionStatus.useMemo": ()=>({
                mounted,
                setMounted,
                transitionStatus
            })
    }["useTransitionStatus.useMemo"], [
        mounted,
        transitionStatus
    ]);
}
}),
"[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/utils/resolveRef.js [app-client] (ecmascript)", ((__turbopack_context__) => {
"use strict";

/**
 * If the provided argument is a ref object, returns its `current` value.
 * Otherwise, returns the argument itself.
 */ __turbopack_context__.s([
    "resolveRef",
    ()=>resolveRef
]);
function resolveRef(maybeRef) {
    if (maybeRef == null) {
        return maybeRef;
    }
    return 'current' in maybeRef ? maybeRef.current : maybeRef;
}
}),
"[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/utils/stateAttributesMapping.js [app-client] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "TransitionStatusDataAttributes",
    ()=>TransitionStatusDataAttributes,
    "transitionStatusMapping",
    ()=>transitionStatusMapping
]);
let TransitionStatusDataAttributes = /*#__PURE__*/ function(TransitionStatusDataAttributes) {
    /**
   * Present when the component is animating in.
   */ TransitionStatusDataAttributes["startingStyle"] = "data-starting-style";
    /**
   * Present when the component is animating out.
   */ TransitionStatusDataAttributes["endingStyle"] = "data-ending-style";
    return TransitionStatusDataAttributes;
}({});
const STARTING_HOOK = {
    [TransitionStatusDataAttributes.startingStyle]: ''
};
const ENDING_HOOK = {
    [TransitionStatusDataAttributes.endingStyle]: ''
};
const transitionStatusMapping = {
    transitionStatus (value) {
        if (value === 'starting') {
            return STARTING_HOOK;
        }
        if (value === 'ending') {
            return ENDING_HOOK;
        }
        return null;
    }
};
}),
"[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/utils/useAnimationsFinished.js [app-client] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "useAnimationsFinished",
    ()=>useAnimationsFinished
]);
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2d$dom$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/compiled/react-dom/index.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useAnimationFrame$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+utils@0.2.5+0ea9ec2a211d4613/node_modules/@base-ui/utils/esm/useAnimationFrame.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useStableCallback$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+utils@0.2.5+0ea9ec2a211d4613/node_modules/@base-ui/utils/esm/useStableCallback.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$resolveRef$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/utils/resolveRef.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$stateAttributesMapping$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/utils/stateAttributesMapping.js [app-client] (ecmascript)");
'use client';
;
;
;
;
;
function useAnimationsFinished(elementOrRef, waitForStartingStyleRemoved = false, treatAbortedAsFinished = true) {
    const frame = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useAnimationFrame$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useAnimationFrame"])();
    return (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useStableCallback$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useStableCallback"])({
        "useAnimationsFinished.useStableCallback": (fnToExecute, /**
   * An optional [AbortSignal](https://developer.mozilla.org/en-US/docs/Web/API/AbortSignal) that
   * can be used to abort `fnToExecute` before all the animations have finished.
   * @default null
   */ signal = null)=>{
            frame.cancel();
            function done() {
                // Synchronously flush the unmounting of the component so that the browser doesn't
                // paint: https://github.com/mui/base-ui/issues/979
                __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2d$dom$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["flushSync"](fnToExecute);
            }
            const element = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$resolveRef$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["resolveRef"])(elementOrRef);
            if (element == null) {
                return;
            }
            const resolvedElement = element;
            if (typeof resolvedElement.getAnimations !== 'function' || globalThis.BASE_UI_ANIMATIONS_DISABLED) {
                fnToExecute();
            } else {
                function execWaitForStartingStyleRemoved() {
                    const startingStyleAttribute = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$stateAttributesMapping$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["TransitionStatusDataAttributes"].startingStyle;
                    // If `[data-starting-style]` isn't present, fall back to waiting one more frame
                    // to give "open" animations a chance to be registered.
                    if (!resolvedElement.hasAttribute(startingStyleAttribute)) {
                        frame.request(exec);
                        return;
                    }
                    // Wait for `[data-starting-style]` to have been removed.
                    const attributeObserver = new MutationObserver({
                        "useAnimationsFinished.useStableCallback.execWaitForStartingStyleRemoved": ()=>{
                            if (!resolvedElement.hasAttribute(startingStyleAttribute)) {
                                attributeObserver.disconnect();
                                exec();
                            }
                        }
                    }["useAnimationsFinished.useStableCallback.execWaitForStartingStyleRemoved"]);
                    attributeObserver.observe(resolvedElement, {
                        attributes: true,
                        attributeFilter: [
                            startingStyleAttribute
                        ]
                    });
                    signal?.addEventListener('abort', {
                        "useAnimationsFinished.useStableCallback.execWaitForStartingStyleRemoved": ()=>attributeObserver.disconnect()
                    }["useAnimationsFinished.useStableCallback.execWaitForStartingStyleRemoved"], {
                        once: true
                    });
                }
                function exec() {
                    Promise.all(resolvedElement.getAnimations().map({
                        "useAnimationsFinished.useStableCallback.exec": (anim)=>anim.finished
                    }["useAnimationsFinished.useStableCallback.exec"])).then({
                        "useAnimationsFinished.useStableCallback.exec": ()=>{
                            if (signal?.aborted) {
                                return;
                            }
                            done();
                        }
                    }["useAnimationsFinished.useStableCallback.exec"]).catch({
                        "useAnimationsFinished.useStableCallback.exec": ()=>{
                            const currentAnimations = resolvedElement.getAnimations();
                            if (treatAbortedAsFinished) {
                                if (signal?.aborted) {
                                    return;
                                }
                                done();
                            } else if (currentAnimations.length > 0 && currentAnimations.some({
                                "useAnimationsFinished.useStableCallback.exec": (anim)=>anim.pending || anim.playState !== 'finished'
                            }["useAnimationsFinished.useStableCallback.exec"])) {
                                // Sometimes animations can be aborted because a property they depend on changes while the animation plays.
                                // In such cases, we need to re-check if any new animations have started.
                                exec();
                            }
                        }
                    }["useAnimationsFinished.useStableCallback.exec"]);
                }
                if (waitForStartingStyleRemoved) {
                    execWaitForStartingStyleRemoved();
                    return;
                }
                frame.request(exec);
            }
        }
    }["useAnimationsFinished.useStableCallback"]);
}
}),
"[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/utils/useOpenChangeComplete.js [app-client] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "useOpenChangeComplete",
    ()=>useOpenChangeComplete
]);
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/compiled/react/index.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useStableCallback$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+utils@0.2.5+0ea9ec2a211d4613/node_modules/@base-ui/utils/esm/useStableCallback.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$useAnimationsFinished$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/utils/useAnimationsFinished.js [app-client] (ecmascript)");
'use client';
;
;
;
function useOpenChangeComplete(parameters) {
    const { enabled = true, open, ref, onComplete: onCompleteParam } = parameters;
    const onComplete = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useStableCallback$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useStableCallback"])(onCompleteParam);
    const runOnceAnimationsFinish = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$useAnimationsFinished$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useAnimationsFinished"])(ref, open, false);
    __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useEffect"]({
        "useOpenChangeComplete.useEffect": ()=>{
            if (!enabled) {
                return undefined;
            }
            const abortController = new AbortController();
            runOnceAnimationsFinish(onComplete, abortController.signal);
            return ({
                "useOpenChangeComplete.useEffect": ()=>{
                    abortController.abort();
                }
            })["useOpenChangeComplete.useEffect"];
        }
    }["useOpenChangeComplete.useEffect"], [
        enabled,
        open,
        onComplete,
        runOnceAnimationsFinish
    ]);
}
}),
"[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/utils/popups/popupStoreUtils.js [app-client] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "useImplicitActiveTrigger",
    ()=>useImplicitActiveTrigger,
    "useOpenStateTransitions",
    ()=>useOpenStateTransitions,
    "useTriggerDataForwarding",
    ()=>useTriggerDataForwarding,
    "useTriggerRegistration",
    ()=>useTriggerRegistration
]);
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/compiled/react/index.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useStableCallback$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+utils@0.2.5+0ea9ec2a211d4613/node_modules/@base-ui/utils/esm/useStableCallback.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useIsoLayoutEffect$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+utils@0.2.5+0ea9ec2a211d4613/node_modules/@base-ui/utils/esm/useIsoLayoutEffect.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$useTransitionStatus$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/utils/useTransitionStatus.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$useOpenChangeComplete$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/utils/useOpenChangeComplete.js [app-client] (ecmascript)");
'use client';
;
;
;
;
;
function useTriggerRegistration(id, store) {
    // Keep track of the currently registered element to unregister it on unmount or id change.
    const registeredElementIdRef = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useRef"](null);
    const registeredElementRef = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useRef"](null);
    return __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useCallback"]({
        "useTriggerRegistration.useCallback": (element)=>{
            if (id === undefined) {
                return;
            }
            if (registeredElementIdRef.current !== null) {
                const registeredId = registeredElementIdRef.current;
                const registeredElement = registeredElementRef.current;
                const currentElement = store.context.triggerElements.getById(registeredId);
                if (registeredElement && currentElement === registeredElement) {
                    store.context.triggerElements.delete(registeredId);
                }
                registeredElementIdRef.current = null;
                registeredElementRef.current = null;
            }
            if (element !== null) {
                registeredElementIdRef.current = id;
                registeredElementRef.current = element;
                store.context.triggerElements.add(id, element);
            }
        }
    }["useTriggerRegistration.useCallback"], [
        store,
        id
    ]);
}
function useTriggerDataForwarding(triggerId, triggerElementRef, store, stateUpdates) {
    const isMountedByThisTrigger = store.useState('isMountedByTrigger', triggerId);
    const baseRegisterTrigger = useTriggerRegistration(triggerId, store);
    const registerTrigger = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useStableCallback$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useStableCallback"])({
        "useTriggerDataForwarding.useStableCallback[registerTrigger]": (element)=>{
            baseRegisterTrigger(element);
            if (!element || !store.select('open')) {
                return;
            }
            const activeTriggerId = store.select('activeTriggerId');
            if (activeTriggerId === triggerId) {
                store.update({
                    activeTriggerElement: element,
                    ...stateUpdates
                });
                return;
            }
            if (activeTriggerId == null) {
                // This runs when popup is open, but no active trigger is set.
                // It can happen when using controlled mode and the trigger is mounted after opening or if `triggerId` prop is not set explicitly.
                // In such cases the first trigger to run this code becomes the active trigger (store.select('activeTriggerId') should not return null after that).
                // This is mostly for compatibility with contained triggers where no explicit `triggerId` was required in controlled mode.
                store.update({
                    activeTriggerId: triggerId,
                    activeTriggerElement: element,
                    ...stateUpdates
                });
            }
        }
    }["useTriggerDataForwarding.useStableCallback[registerTrigger]"]);
    (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useIsoLayoutEffect$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useIsoLayoutEffect"])({
        "useTriggerDataForwarding.useIsoLayoutEffect": ()=>{
            if (isMountedByThisTrigger) {
                store.update({
                    activeTriggerElement: triggerElementRef.current,
                    ...stateUpdates
                });
            }
        // eslint-disable-next-line react-hooks/exhaustive-deps
        }
    }["useTriggerDataForwarding.useIsoLayoutEffect"], [
        isMountedByThisTrigger,
        store,
        triggerElementRef,
        ...Object.values(stateUpdates)
    ]);
    return {
        registerTrigger,
        isMountedByThisTrigger
    };
}
function useImplicitActiveTrigger(store) {
    const open = store.useState('open');
    (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useIsoLayoutEffect$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useIsoLayoutEffect"])({
        "useImplicitActiveTrigger.useIsoLayoutEffect": ()=>{
            if (open && !store.select('activeTriggerId') && store.context.triggerElements.size === 1) {
                const iteratorResult = store.context.triggerElements.entries().next();
                if (!iteratorResult.done) {
                    const [implicitTriggerId, implicitTriggerElement] = iteratorResult.value;
                    store.update({
                        activeTriggerId: implicitTriggerId,
                        activeTriggerElement: implicitTriggerElement
                    });
                }
            }
        }
    }["useImplicitActiveTrigger.useIsoLayoutEffect"], [
        open,
        store
    ]);
}
function useOpenStateTransitions(open, store, onUnmount) {
    const { mounted, setMounted, transitionStatus } = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$useTransitionStatus$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useTransitionStatus"])(open);
    store.useSyncedValues({
        mounted,
        transitionStatus
    });
    const forceUnmount = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useStableCallback$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useStableCallback"])({
        "useOpenStateTransitions.useStableCallback[forceUnmount]": ()=>{
            setMounted(false);
            store.update({
                activeTriggerId: null,
                activeTriggerElement: null,
                mounted: false
            });
            onUnmount?.();
            store.context.onOpenChangeComplete?.(false);
        }
    }["useOpenStateTransitions.useStableCallback[forceUnmount]"]);
    const preventUnmountingOnClose = store.useState('preventUnmountingOnClose');
    (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$useOpenChangeComplete$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useOpenChangeComplete"])({
        enabled: !preventUnmountingOnClose,
        open,
        ref: store.context.popupRef,
        onComplete () {
            if (!open) {
                forceUnmount();
            }
        }
    });
    return {
        forceUnmount,
        transitionStatus
    };
}
}),
"[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/floating-ui-react/components/FloatingRootStore.js [app-client] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "FloatingRootStore",
    ()=>FloatingRootStore
]);
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$store$2f$createSelector$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+utils@0.2.5+0ea9ec2a211d4613/node_modules/@base-ui/utils/esm/store/createSelector.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$store$2f$ReactStore$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+utils@0.2.5+0ea9ec2a211d4613/node_modules/@base-ui/utils/esm/store/ReactStore.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$createEventEmitter$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/floating-ui-react/utils/createEventEmitter.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$event$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/floating-ui-react/utils/event.js [app-client] (ecmascript)");
;
;
;
const selectors = {
    open: (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$store$2f$createSelector$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["createSelector"])((state)=>state.open),
    domReferenceElement: (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$store$2f$createSelector$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["createSelector"])((state)=>state.domReferenceElement),
    referenceElement: (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$store$2f$createSelector$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["createSelector"])((state)=>state.positionReference ?? state.referenceElement),
    floatingElement: (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$store$2f$createSelector$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["createSelector"])((state)=>state.floatingElement),
    floatingId: (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$store$2f$createSelector$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["createSelector"])((state)=>state.floatingId)
};
class FloatingRootStore extends __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$store$2f$ReactStore$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["ReactStore"] {
    constructor(options){
        const { nested, noEmit, onOpenChange, triggerElements, ...initialState } = options;
        super({
            ...initialState,
            positionReference: initialState.referenceElement,
            domReferenceElement: initialState.referenceElement
        }, {
            onOpenChange,
            dataRef: {
                current: {}
            },
            events: (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$createEventEmitter$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["createEventEmitter"])(),
            nested,
            noEmit,
            triggerElements
        }, selectors);
    }
    /**
   * Emits the `openchange` event through the internal event emitter and calls the `onOpenChange` handler with the provided arguments.
   *
   * @param newOpen The new open state.
   * @param eventDetails Details about the event that triggered the open state change.
   */ setOpen = (newOpen, eventDetails)=>{
        if (!newOpen || !this.state.open || // Prevent a pending hover-open from overwriting a click-open event, while allowing
        // click events to upgrade a hover-open.
        (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$event$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["isClickLikeEvent"])(eventDetails.event)) {
            this.context.dataRef.current.openEvent = newOpen ? eventDetails.event : undefined;
        }
        if (!this.context.noEmit) {
            const details = {
                open: newOpen,
                reason: eventDetails.reason,
                nativeEvent: eventDetails.event,
                nested: this.context.nested,
                triggerElement: eventDetails.trigger
            };
            this.context.events.emit('openchange', details);
        }
        this.context.onOpenChange?.(newOpen, eventDetails);
    };
}
}),
"[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/floating-ui-react/hooks/useSyncedFloatingRootContext.js [app-client] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "useSyncedFloatingRootContext",
    ()=>useSyncedFloatingRootContext
]);
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useId$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+utils@0.2.5+0ea9ec2a211d4613/node_modules/@base-ui/utils/esm/useId.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useRefWithInit$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+utils@0.2.5+0ea9ec2a211d4613/node_modules/@base-ui/utils/esm/useRefWithInit.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useIsoLayoutEffect$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+utils@0.2.5+0ea9ec2a211d4613/node_modules/@base-ui/utils/esm/useIsoLayoutEffect.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$floating$2d$ui$2b$utils$40$0$2e$2$2e$11$2f$node_modules$2f40$floating$2d$ui$2f$utils$2f$dist$2f$floating$2d$ui$2e$utils$2e$dom$2e$mjs__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@floating-ui+utils@0.2.11/node_modules/@floating-ui/utils/dist/floating-ui.utils.dom.mjs [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$components$2f$FloatingTree$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/floating-ui-react/components/FloatingTree.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$components$2f$FloatingRootStore$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/floating-ui-react/components/FloatingRootStore.js [app-client] (ecmascript)");
'use client';
;
;
;
;
;
;
function useSyncedFloatingRootContext(options) {
    const { popupStore, noEmit = false, treatPopupAsFloatingElement = false, onOpenChange } = options;
    const floatingId = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useId$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useId"])();
    const nested = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$components$2f$FloatingTree$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useFloatingParentNodeId"])() != null;
    const open = popupStore.useState('open');
    const referenceElement = popupStore.useState('activeTriggerElement');
    const floatingElement = popupStore.useState(treatPopupAsFloatingElement ? 'popupElement' : 'positionerElement');
    const triggerElements = popupStore.context.triggerElements;
    const store = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useRefWithInit$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useRefWithInit"])({
        "useSyncedFloatingRootContext.useRefWithInit": ()=>new __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$components$2f$FloatingRootStore$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["FloatingRootStore"]({
                open,
                referenceElement,
                floatingElement,
                triggerElements,
                onOpenChange,
                floatingId,
                nested,
                noEmit
            })
    }["useSyncedFloatingRootContext.useRefWithInit"]).current;
    (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useIsoLayoutEffect$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useIsoLayoutEffect"])({
        "useSyncedFloatingRootContext.useIsoLayoutEffect": ()=>{
            const valuesToSync = {
                open,
                floatingId,
                referenceElement,
                floatingElement
            };
            if ((0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$floating$2d$ui$2b$utils$40$0$2e$2$2e$11$2f$node_modules$2f40$floating$2d$ui$2f$utils$2f$dist$2f$floating$2d$ui$2e$utils$2e$dom$2e$mjs__$5b$app$2d$client$5d$__$28$ecmascript$29$__["isElement"])(referenceElement)) {
                valuesToSync.domReferenceElement = referenceElement;
            }
            if (store.state.positionReference === store.state.referenceElement) {
                valuesToSync.positionReference = referenceElement;
            }
            store.update(valuesToSync);
        }
    }["useSyncedFloatingRootContext.useIsoLayoutEffect"], [
        open,
        floatingId,
        referenceElement,
        floatingElement,
        store
    ]);
    // TODO: When `setOpen` is a part of the PopupStore API, we don't need to sync it.
    store.context.onOpenChange = onOpenChange;
    store.context.nested = nested;
    store.context.noEmit = noEmit;
    return store;
}
}),
"[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/utils/popups/popupTriggerMap.js [app-client] (ecmascript)", ((__turbopack_context__) => {
"use strict";

/**
 * Data structure to keep track of popup trigger elements by their IDs.
 * Uses both a set of Elements and a map of IDs to Elements for efficient lookups.
 */ __turbopack_context__.s([
    "PopupTriggerMap",
    ()=>PopupTriggerMap
]);
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$build$2f$polyfills$2f$process$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = /*#__PURE__*/ __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/build/polyfills/process.js [app-client] (ecmascript)");
class PopupTriggerMap {
    constructor(){
        this.elementsSet = new Set();
        this.idMap = new Map();
    }
    /**
   * Adds a trigger element with the given ID.
   *
   * Note: The provided element is assumed to not be registered under multiple IDs.
   */ add(id, element) {
        const existingElement = this.idMap.get(id);
        if (existingElement === element) {
            return;
        }
        if (existingElement !== undefined) {
            // We assume that the same element won't be registered under multiple ids.
            // This is safe considering how useTriggerRegistration is implemented.
            this.elementsSet.delete(existingElement);
        }
        this.elementsSet.add(element);
        this.idMap.set(id, element);
        if ("TURBOPACK compile-time truthy", 1) {
            if (this.elementsSet.size !== this.idMap.size) {
                throw new Error('Base UI: A trigger element cannot be registered under multiple IDs in PopupTriggerMap.');
            }
        }
    }
    /**
   * Removes the trigger element with the given ID.
   */ delete(id) {
        const element = this.idMap.get(id);
        if (element) {
            this.elementsSet.delete(element);
            this.idMap.delete(id);
        }
    }
    /**
   * Whether the given element is registered as a trigger.
   */ hasElement(element) {
        return this.elementsSet.has(element);
    }
    /**
   * Whether there is a registered trigger element matching the given predicate.
   */ hasMatchingElement(predicate) {
        for (const element of this.elementsSet){
            if (predicate(element)) {
                return true;
            }
        }
        return false;
    }
    /**
   * Returns the trigger element associated with the given ID, or undefined if no such element exists.
   */ getById(id) {
        return this.idMap.get(id);
    }
    /**
   * Returns an iterable of all registered trigger entries, where each entry is a tuple of [id, element].
   */ entries() {
        return this.idMap.entries();
    }
    /**
   * Returns an iterable of all registered trigger elements.
   */ elements() {
        return this.elementsSet.values();
    }
    /**
   * Returns the number of registered trigger elements.
   */ get size() {
        return this.idMap.size;
    }
}
}),
"[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/floating-ui-react/utils/getEmptyRootContext.js [app-client] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "getEmptyRootContext",
    ()=>getEmptyRootContext
]);
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$popups$2f$popupTriggerMap$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/utils/popups/popupTriggerMap.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$components$2f$FloatingRootStore$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/floating-ui-react/components/FloatingRootStore.js [app-client] (ecmascript)");
;
;
function getEmptyRootContext() {
    return new __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$components$2f$FloatingRootStore$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["FloatingRootStore"]({
        open: false,
        floatingElement: null,
        referenceElement: null,
        triggerElements: new __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$popups$2f$popupTriggerMap$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["PopupTriggerMap"](),
        floatingId: '',
        nested: false,
        noEmit: false,
        onOpenChange: undefined
    });
}
}),
"[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/utils/popups/store.js [app-client] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "createInitialPopupStoreState",
    ()=>createInitialPopupStoreState,
    "popupStoreSelectors",
    ()=>popupStoreSelectors
]);
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$store$2f$createSelector$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+utils@0.2.5+0ea9ec2a211d4613/node_modules/@base-ui/utils/esm/store/createSelector.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$getEmptyRootContext$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/floating-ui-react/utils/getEmptyRootContext.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$empty$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+utils@0.2.5+0ea9ec2a211d4613/node_modules/@base-ui/utils/esm/empty.js [app-client] (ecmascript)");
;
;
;
function createInitialPopupStoreState() {
    return {
        open: false,
        openProp: undefined,
        mounted: false,
        transitionStatus: 'idle',
        floatingRootContext: (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$getEmptyRootContext$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["getEmptyRootContext"])(),
        preventUnmountingOnClose: false,
        payload: undefined,
        activeTriggerId: null,
        activeTriggerElement: null,
        triggerIdProp: undefined,
        popupElement: null,
        positionerElement: null,
        activeTriggerProps: __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$empty$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["EMPTY_OBJECT"],
        inactiveTriggerProps: __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$empty$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["EMPTY_OBJECT"],
        popupProps: __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$empty$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["EMPTY_OBJECT"]
    };
}
const activeTriggerIdSelector = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$store$2f$createSelector$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["createSelector"])((state)=>state.triggerIdProp ?? state.activeTriggerId);
const popupStoreSelectors = {
    open: (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$store$2f$createSelector$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["createSelector"])((state)=>state.openProp ?? state.open),
    mounted: (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$store$2f$createSelector$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["createSelector"])((state)=>state.mounted),
    transitionStatus: (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$store$2f$createSelector$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["createSelector"])((state)=>state.transitionStatus),
    floatingRootContext: (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$store$2f$createSelector$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["createSelector"])((state)=>state.floatingRootContext),
    preventUnmountingOnClose: (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$store$2f$createSelector$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["createSelector"])((state)=>state.preventUnmountingOnClose),
    payload: (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$store$2f$createSelector$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["createSelector"])((state)=>state.payload),
    activeTriggerId: activeTriggerIdSelector,
    activeTriggerElement: (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$store$2f$createSelector$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["createSelector"])((state)=>state.mounted ? state.activeTriggerElement : null),
    /**
   * Whether the trigger with the given ID was used to open the popup.
   */ isTriggerActive: (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$store$2f$createSelector$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["createSelector"])((state, triggerId)=>triggerId !== undefined && activeTriggerIdSelector(state) === triggerId),
    /**
   * Whether the popup is open and was activated by a trigger with the given ID.
   */ isOpenedByTrigger: (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$store$2f$createSelector$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["createSelector"])((state, triggerId)=>triggerId !== undefined && activeTriggerIdSelector(state) === triggerId && state.open),
    /**
   * Whether the popup is mounted and was activated by a trigger with the given ID.
   */ isMountedByTrigger: (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$store$2f$createSelector$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["createSelector"])((state, triggerId)=>triggerId !== undefined && activeTriggerIdSelector(state) === triggerId && state.mounted),
    triggerProps: (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$store$2f$createSelector$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["createSelector"])((state, isActive)=>isActive ? state.activeTriggerProps : state.inactiveTriggerProps),
    popupProps: (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$store$2f$createSelector$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["createSelector"])((state)=>state.popupProps),
    popupElement: (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$store$2f$createSelector$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["createSelector"])((state)=>state.popupElement),
    positionerElement: (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$store$2f$createSelector$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["createSelector"])((state)=>state.positionerElement)
};
}),
"[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/tooltip/store/TooltipStore.js [app-client] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "TooltipStore",
    ()=>TooltipStore
]);
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/compiled/react/index.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2d$dom$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/compiled/react-dom/index.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$store$2f$createSelector$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+utils@0.2.5+0ea9ec2a211d4613/node_modules/@base-ui/utils/esm/store/createSelector.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$store$2f$ReactStore$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+utils@0.2.5+0ea9ec2a211d4613/node_modules/@base-ui/utils/esm/store/ReactStore.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useRefWithInit$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+utils@0.2.5+0ea9ec2a211d4613/node_modules/@base-ui/utils/esm/useRefWithInit.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$hooks$2f$useSyncedFloatingRootContext$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/floating-ui-react/hooks/useSyncedFloatingRootContext.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$reason$2d$parts$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$export__$2a$__as__REASONS$3e$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/utils/reason-parts.js [app-client] (ecmascript) <export * as REASONS>");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$popups$2f$store$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/utils/popups/store.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$popups$2f$popupTriggerMap$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/utils/popups/popupTriggerMap.js [app-client] (ecmascript)");
;
;
;
;
;
;
;
const selectors = {
    ...__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$popups$2f$store$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["popupStoreSelectors"],
    disabled: (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$store$2f$createSelector$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["createSelector"])((state)=>state.disabled),
    instantType: (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$store$2f$createSelector$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["createSelector"])((state)=>state.instantType),
    isInstantPhase: (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$store$2f$createSelector$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["createSelector"])((state)=>state.isInstantPhase),
    trackCursorAxis: (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$store$2f$createSelector$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["createSelector"])((state)=>state.trackCursorAxis),
    disableHoverablePopup: (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$store$2f$createSelector$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["createSelector"])((state)=>state.disableHoverablePopup),
    lastOpenChangeReason: (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$store$2f$createSelector$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["createSelector"])((state)=>state.openChangeReason),
    closeDelay: (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$store$2f$createSelector$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["createSelector"])((state)=>state.closeDelay),
    hasViewport: (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$store$2f$createSelector$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["createSelector"])((state)=>state.hasViewport)
};
class TooltipStore extends __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$store$2f$ReactStore$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["ReactStore"] {
    constructor(initialState){
        super({
            ...createInitialState(),
            ...initialState
        }, {
            popupRef: /*#__PURE__*/ __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["createRef"](),
            onOpenChange: undefined,
            onOpenChangeComplete: undefined,
            triggerElements: new __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$popups$2f$popupTriggerMap$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["PopupTriggerMap"]()
        }, selectors);
    }
    setOpen = (nextOpen, eventDetails)=>{
        const reason = eventDetails.reason;
        const isHover = reason === __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$reason$2d$parts$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$export__$2a$__as__REASONS$3e$__["REASONS"].triggerHover;
        const isFocusOpen = nextOpen && reason === __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$reason$2d$parts$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$export__$2a$__as__REASONS$3e$__["REASONS"].triggerFocus;
        const isDismissClose = !nextOpen && (reason === __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$reason$2d$parts$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$export__$2a$__as__REASONS$3e$__["REASONS"].triggerPress || reason === __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$reason$2d$parts$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$export__$2a$__as__REASONS$3e$__["REASONS"].escapeKey);
        eventDetails.preventUnmountOnClose = ()=>{
            this.set('preventUnmountingOnClose', true);
        };
        this.context.onOpenChange?.(nextOpen, eventDetails);
        if (eventDetails.isCanceled) {
            return;
        }
        const changeState = ()=>{
            const updatedState = {
                open: nextOpen,
                openChangeReason: reason
            };
            if (isFocusOpen) {
                updatedState.instantType = 'focus';
            } else if (isDismissClose) {
                updatedState.instantType = 'dismiss';
            } else if (reason === __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$reason$2d$parts$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$export__$2a$__as__REASONS$3e$__["REASONS"].triggerHover) {
                updatedState.instantType = undefined;
            }
            // If a popup is closing, the `trigger` may be null.
            // We want to keep the previous value so that exit animations are played and focus is returned correctly.
            const newTriggerId = eventDetails.trigger?.id ?? null;
            if (newTriggerId || nextOpen) {
                updatedState.activeTriggerId = newTriggerId;
                updatedState.activeTriggerElement = eventDetails.trigger ?? null;
            }
            this.update(updatedState);
        };
        if (isHover) {
            // If a hover reason is provided, we need to flush the state synchronously. This ensures
            // `node.getAnimations()` knows about the new state.
            __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2d$dom$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["flushSync"](changeState);
        } else {
            changeState();
        }
    };
    static useStore(externalStore, initialState) {
        // eslint-disable-next-line react-hooks/rules-of-hooks
        const internalStore = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useRefWithInit$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useRefWithInit"])(()=>{
            return new TooltipStore(initialState);
        }).current;
        const store = externalStore ?? internalStore;
        // eslint-disable-next-line react-hooks/rules-of-hooks
        const floatingRootContext = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$hooks$2f$useSyncedFloatingRootContext$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useSyncedFloatingRootContext"])({
            popupStore: store,
            onOpenChange: store.setOpen
        });
        // It's safe to set this here because when this code runs for the first time,
        // nothing has had a chance to subscribe to the `store` yet.
        // For subsequent renders, the `floatingRootContext` reference remains the same,
        // so it's basically a no-op.
        store.state.floatingRootContext = floatingRootContext;
        return store;
    }
}
function createInitialState() {
    return {
        ...(0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$popups$2f$store$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["createInitialPopupStoreState"])(),
        disabled: false,
        instantType: undefined,
        isInstantPhase: false,
        trackCursorAxis: 'none',
        disableHoverablePopup: false,
        openChangeReason: null,
        closeDelay: 0,
        hasViewport: false
    };
}
}),
"[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/tooltip/root/TooltipRoot.js [app-client] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "TooltipRoot",
    ()=>TooltipRoot
]);
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$build$2f$polyfills$2f$process$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = /*#__PURE__*/ __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/build/polyfills/process.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/compiled/react/index.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$fastHooks$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+utils@0.2.5+0ea9ec2a211d4613/node_modules/@base-ui/utils/esm/fastHooks.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useOnFirstRender$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+utils@0.2.5+0ea9ec2a211d4613/node_modules/@base-ui/utils/esm/useOnFirstRender.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useIsoLayoutEffect$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+utils@0.2.5+0ea9ec2a211d4613/node_modules/@base-ui/utils/esm/useIsoLayoutEffect.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$tooltip$2f$root$2f$TooltipRootContext$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/tooltip/root/TooltipRootContext.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$hooks$2f$useClientPoint$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/floating-ui-react/hooks/useClientPoint.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$hooks$2f$useDismiss$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/floating-ui-react/hooks/useDismiss.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$hooks$2f$useInteractions$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/floating-ui-react/hooks/useInteractions.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$createBaseUIEventDetails$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/utils/createBaseUIEventDetails.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$popups$2f$popupStoreUtils$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/utils/popups/popupStoreUtils.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$tooltip$2f$store$2f$TooltipStore$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/tooltip/store/TooltipStore.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$reason$2d$parts$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$export__$2a$__as__REASONS$3e$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/utils/reason-parts.js [app-client] (ecmascript) <export * as REASONS>");
/**
 * Groups all parts of the tooltip.
 * Doesn’t render its own HTML element.
 *
 * Documentation: [Base UI Tooltip](https://base-ui.com/react/components/tooltip)
 */ var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/compiled/react/jsx-runtime.js [app-client] (ecmascript)");
'use client';
;
;
;
;
;
;
;
;
;
;
;
const TooltipRoot = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$fastHooks$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["fastComponent"])(function TooltipRoot(props) {
    const { disabled = false, defaultOpen = false, open: openProp, disableHoverablePopup = false, trackCursorAxis = 'none', actionsRef, onOpenChange, onOpenChangeComplete, handle, triggerId: triggerIdProp, defaultTriggerId: defaultTriggerIdProp = null, children } = props;
    const store = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$tooltip$2f$store$2f$TooltipStore$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["TooltipStore"].useStore(handle?.store, {
        open: defaultOpen,
        openProp,
        activeTriggerId: defaultTriggerIdProp,
        triggerIdProp
    });
    // Support initially open state when uncontrolled
    (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useOnFirstRender$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useOnFirstRender"])({
        "TooltipRoot.TooltipRoot.useOnFirstRender": ()=>{
            if (openProp === undefined && store.state.open === false && defaultOpen === true) {
                store.update({
                    open: true,
                    activeTriggerId: defaultTriggerIdProp
                });
            }
        }
    }["TooltipRoot.TooltipRoot.useOnFirstRender"]);
    store.useControlledProp('openProp', openProp);
    store.useControlledProp('triggerIdProp', triggerIdProp);
    store.useContextCallback('onOpenChange', onOpenChange);
    store.useContextCallback('onOpenChangeComplete', onOpenChangeComplete);
    const openState = store.useState('open');
    const open = !disabled && openState;
    const activeTriggerId = store.useState('activeTriggerId');
    const payload = store.useState('payload');
    store.useSyncedValues({
        trackCursorAxis,
        disableHoverablePopup
    });
    (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useIsoLayoutEffect$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useIsoLayoutEffect"])({
        "TooltipRoot.TooltipRoot.useIsoLayoutEffect": ()=>{
            if (openState && disabled) {
                store.setOpen(false, (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$createBaseUIEventDetails$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["createChangeEventDetails"])(__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$reason$2d$parts$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$export__$2a$__as__REASONS$3e$__["REASONS"].disabled));
            }
        }
    }["TooltipRoot.TooltipRoot.useIsoLayoutEffect"], [
        openState,
        disabled,
        store
    ]);
    store.useSyncedValue('disabled', disabled);
    (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$popups$2f$popupStoreUtils$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useImplicitActiveTrigger"])(store);
    const { forceUnmount, transitionStatus } = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$popups$2f$popupStoreUtils$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useOpenStateTransitions"])(open, store);
    const isInstantPhase = store.useState('isInstantPhase');
    const instantType = store.useState('instantType');
    const lastOpenChangeReason = store.useState('lastOpenChangeReason');
    // Animations should be instant in two cases:
    // 1) Opening during the provider's instant phase (adjacent tooltip opens instantly)
    // 2) Closing because another tooltip opened (reason === 'none')
    // Otherwise, allow the animation to play. In particular, do not disable animations
    // during the 'ending' phase unless it's due to a sibling opening.
    const previousInstantTypeRef = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useRef"](null);
    (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useIsoLayoutEffect$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useIsoLayoutEffect"])({
        "TooltipRoot.TooltipRoot.useIsoLayoutEffect": ()=>{
            if (transitionStatus === 'ending' && lastOpenChangeReason === __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$reason$2d$parts$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$export__$2a$__as__REASONS$3e$__["REASONS"].none || transitionStatus !== 'ending' && isInstantPhase) {
                // Capture the current instant type so we can restore it later
                // and set to 'delay' to disable animations while moving from one trigger to another
                // within a delay group.
                if (instantType !== 'delay') {
                    previousInstantTypeRef.current = instantType;
                }
                store.set('instantType', 'delay');
            } else if (previousInstantTypeRef.current !== null) {
                store.set('instantType', previousInstantTypeRef.current);
                previousInstantTypeRef.current = null;
            }
        }
    }["TooltipRoot.TooltipRoot.useIsoLayoutEffect"], [
        transitionStatus,
        isInstantPhase,
        lastOpenChangeReason,
        instantType,
        store
    ]);
    (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useIsoLayoutEffect$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useIsoLayoutEffect"])({
        "TooltipRoot.TooltipRoot.useIsoLayoutEffect": ()=>{
            if (open) {
                if (activeTriggerId == null) {
                    store.set('payload', undefined);
                }
            }
        }
    }["TooltipRoot.TooltipRoot.useIsoLayoutEffect"], [
        store,
        activeTriggerId,
        open
    ]);
    const handleImperativeClose = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useCallback"]({
        "TooltipRoot.TooltipRoot.useCallback[handleImperativeClose]": ()=>{
            store.setOpen(false, createTooltipEventDetails(store, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$reason$2d$parts$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$export__$2a$__as__REASONS$3e$__["REASONS"].imperativeAction));
        }
    }["TooltipRoot.TooltipRoot.useCallback[handleImperativeClose]"], [
        store
    ]);
    __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useImperativeHandle"](actionsRef, {
        "TooltipRoot.TooltipRoot.useImperativeHandle": ()=>({
                unmount: forceUnmount,
                close: handleImperativeClose
            })
    }["TooltipRoot.TooltipRoot.useImperativeHandle"], [
        forceUnmount,
        handleImperativeClose
    ]);
    const floatingRootContext = store.useState('floatingRootContext');
    const dismiss = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$hooks$2f$useDismiss$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useDismiss"])(floatingRootContext, {
        enabled: !disabled,
        referencePress: true
    });
    const clientPoint = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$hooks$2f$useClientPoint$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useClientPoint"])(floatingRootContext, {
        enabled: !disabled && trackCursorAxis !== 'none',
        axis: trackCursorAxis === 'none' ? undefined : trackCursorAxis
    });
    const { getReferenceProps, getFloatingProps, getTriggerProps } = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$hooks$2f$useInteractions$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useInteractions"])([
        dismiss,
        clientPoint
    ]);
    const activeTriggerProps = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useMemo"]({
        "TooltipRoot.TooltipRoot.useMemo[activeTriggerProps]": ()=>getReferenceProps()
    }["TooltipRoot.TooltipRoot.useMemo[activeTriggerProps]"], [
        getReferenceProps
    ]);
    const inactiveTriggerProps = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useMemo"]({
        "TooltipRoot.TooltipRoot.useMemo[inactiveTriggerProps]": ()=>getTriggerProps()
    }["TooltipRoot.TooltipRoot.useMemo[inactiveTriggerProps]"], [
        getTriggerProps
    ]);
    const popupProps = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useMemo"]({
        "TooltipRoot.TooltipRoot.useMemo[popupProps]": ()=>getFloatingProps()
    }["TooltipRoot.TooltipRoot.useMemo[popupProps]"], [
        getFloatingProps
    ]);
    store.useSyncedValues({
        activeTriggerProps,
        inactiveTriggerProps,
        popupProps
    });
    return /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsx"])(__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$tooltip$2f$root$2f$TooltipRootContext$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["TooltipRootContext"].Provider, {
        value: store,
        children: typeof children === 'function' ? children({
            payload
        }) : children
    });
});
if ("TURBOPACK compile-time truthy", 1) TooltipRoot.displayName = "TooltipRoot";
function createTooltipEventDetails(store, reason) {
    const details = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$createBaseUIEventDetails$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["createChangeEventDetails"])(reason);
    details.preventUnmountOnClose = ()=>{
        store.set('preventUnmountingOnClose', true);
    };
    return details;
}
}),
"[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/utils/popupStateMapping.js [app-client] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "CommonPopupDataAttributes",
    ()=>CommonPopupDataAttributes,
    "CommonTriggerDataAttributes",
    ()=>CommonTriggerDataAttributes,
    "popupStateMapping",
    ()=>popupStateMapping,
    "pressableTriggerOpenStateMapping",
    ()=>pressableTriggerOpenStateMapping,
    "triggerOpenStateMapping",
    ()=>triggerOpenStateMapping
]);
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$stateAttributesMapping$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/utils/stateAttributesMapping.js [app-client] (ecmascript)");
;
let CommonPopupDataAttributes = function(CommonPopupDataAttributes) {
    /**
   * Present when the popup is open.
   */ CommonPopupDataAttributes["open"] = "data-open";
    /**
   * Present when the popup is closed.
   */ CommonPopupDataAttributes["closed"] = "data-closed";
    /**
   * Present when the popup is animating in.
   */ CommonPopupDataAttributes[CommonPopupDataAttributes["startingStyle"] = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$stateAttributesMapping$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["TransitionStatusDataAttributes"].startingStyle] = "startingStyle";
    /**
   * Present when the popup is animating out.
   */ CommonPopupDataAttributes[CommonPopupDataAttributes["endingStyle"] = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$stateAttributesMapping$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["TransitionStatusDataAttributes"].endingStyle] = "endingStyle";
    /**
   * Present when the anchor is hidden.
   */ CommonPopupDataAttributes["anchorHidden"] = "data-anchor-hidden";
    /**
   * Indicates which side the popup is positioned relative to the trigger.
   * @type { 'top' | 'bottom' | 'left' | 'right' | 'inline-end' | 'inline-start'}
   */ CommonPopupDataAttributes["side"] = "data-side";
    /**
   * Indicates how the popup is aligned relative to specified side.
   * @type {'start' | 'center' | 'end'}
   */ CommonPopupDataAttributes["align"] = "data-align";
    return CommonPopupDataAttributes;
}({});
let CommonTriggerDataAttributes = /*#__PURE__*/ function(CommonTriggerDataAttributes) {
    /**
   * Present when the popup is open.
   */ CommonTriggerDataAttributes["popupOpen"] = "data-popup-open";
    /**
   * Present when a pressable trigger is pressed.
   */ CommonTriggerDataAttributes["pressed"] = "data-pressed";
    return CommonTriggerDataAttributes;
}({});
const TRIGGER_HOOK = {
    [CommonTriggerDataAttributes.popupOpen]: ''
};
const PRESSABLE_TRIGGER_HOOK = {
    [CommonTriggerDataAttributes.popupOpen]: '',
    [CommonTriggerDataAttributes.pressed]: ''
};
const POPUP_OPEN_HOOK = {
    [CommonPopupDataAttributes.open]: ''
};
const POPUP_CLOSED_HOOK = {
    [CommonPopupDataAttributes.closed]: ''
};
const ANCHOR_HIDDEN_HOOK = {
    [CommonPopupDataAttributes.anchorHidden]: ''
};
const triggerOpenStateMapping = {
    open (value) {
        if (value) {
            return TRIGGER_HOOK;
        }
        return null;
    }
};
const pressableTriggerOpenStateMapping = {
    open (value) {
        if (value) {
            return PRESSABLE_TRIGGER_HOOK;
        }
        return null;
    }
};
const popupStateMapping = {
    open (value) {
        if (value) {
            return POPUP_OPEN_HOOK;
        }
        return POPUP_CLOSED_HOOK;
    },
    anchorHidden (value) {
        if (value) {
            return ANCHOR_HIDDEN_HOOK;
        }
        return null;
    }
};
}),
"[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/utils/getStateAttributesProps.js [app-client] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "getStateAttributesProps",
    ()=>getStateAttributesProps
]);
function getStateAttributesProps(state, customMapping) {
    const props = {};
    /* eslint-disable-next-line guard-for-in */ for(const key in state){
        const value = state[key];
        if (customMapping?.hasOwnProperty(key)) {
            const customProps = customMapping[key](value);
            if (customProps != null) {
                Object.assign(props, customProps);
            }
            continue;
        }
        if (value === true) {
            props[`data-${key.toLowerCase()}`] = '';
        } else if (value) {
            props[`data-${key.toLowerCase()}`] = value.toString();
        }
    }
    return props;
}
}),
"[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/utils/resolveClassName.js [app-client] (ecmascript)", ((__turbopack_context__) => {
"use strict";

/**
 * If the provided className is a string, it will be returned as is.
 * Otherwise, the function will call the className function with the state as the first argument.
 *
 * @param className
 * @param state
 */ __turbopack_context__.s([
    "resolveClassName",
    ()=>resolveClassName
]);
function resolveClassName(className, state) {
    return typeof className === 'function' ? className(state) : className;
}
}),
"[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/utils/resolveStyle.js [app-client] (ecmascript)", ((__turbopack_context__) => {
"use strict";

/**
 * If the provided style is an object, it will be returned as is.
 * Otherwise, the function will call the style function with the state as the first argument.
 *
 * @param style
 * @param state
 */ __turbopack_context__.s([
    "resolveStyle",
    ()=>resolveStyle
]);
function resolveStyle(style, state) {
    return typeof style === 'function' ? style(state) : style;
}
}),
"[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/merge-props/mergeProps.js [app-client] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "makeEventPreventable",
    ()=>makeEventPreventable,
    "mergeClassNames",
    ()=>mergeClassNames,
    "mergeProps",
    ()=>mergeProps,
    "mergePropsN",
    ()=>mergePropsN
]);
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$mergeObjects$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+utils@0.2.5+0ea9ec2a211d4613/node_modules/@base-ui/utils/esm/mergeObjects.js [app-client] (ecmascript)");
;
const EMPTY_PROPS = {};
function mergeProps(a, b, c, d, e) {
    // We need to mutably own `merged`
    let merged = {
        ...resolvePropsGetter(a, EMPTY_PROPS)
    };
    if (b) {
        merged = mergeOne(merged, b);
    }
    if (c) {
        merged = mergeOne(merged, c);
    }
    if (d) {
        merged = mergeOne(merged, d);
    }
    if (e) {
        merged = mergeOne(merged, e);
    }
    return merged;
}
function mergePropsN(props) {
    if (props.length === 0) {
        return EMPTY_PROPS;
    }
    if (props.length === 1) {
        return resolvePropsGetter(props[0], EMPTY_PROPS);
    }
    // We need to mutably own `merged`
    let merged = {
        ...resolvePropsGetter(props[0], EMPTY_PROPS)
    };
    for(let i = 1; i < props.length; i += 1){
        merged = mergeOne(merged, props[i]);
    }
    return merged;
}
function mergeOne(merged, inputProps) {
    if (isPropsGetter(inputProps)) {
        return inputProps(merged);
    }
    return mutablyMergeInto(merged, inputProps);
}
/**
 * Merges two sets of props. In case of conflicts, the external props take precedence.
 */ function mutablyMergeInto(mergedProps, externalProps) {
    if (!externalProps) {
        return mergedProps;
    }
    // eslint-disable-next-line guard-for-in
    for(const propName in externalProps){
        const externalPropValue = externalProps[propName];
        switch(propName){
            case 'style':
                {
                    mergedProps[propName] = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$mergeObjects$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["mergeObjects"])(mergedProps.style, externalPropValue);
                    break;
                }
            case 'className':
                {
                    mergedProps[propName] = mergeClassNames(mergedProps.className, externalPropValue);
                    break;
                }
            default:
                {
                    if (isEventHandler(propName, externalPropValue)) {
                        mergedProps[propName] = mergeEventHandlers(mergedProps[propName], externalPropValue);
                    } else {
                        mergedProps[propName] = externalPropValue;
                    }
                }
        }
    }
    return mergedProps;
}
function isEventHandler(key, value) {
    // This approach is more efficient than using a regex.
    const code0 = key.charCodeAt(0);
    const code1 = key.charCodeAt(1);
    const code2 = key.charCodeAt(2);
    return code0 === 111 /* o */  && code1 === 110 /* n */  && code2 >= 65 /* A */  && code2 <= 90 /* Z */  && (typeof value === 'function' || typeof value === 'undefined');
}
function isPropsGetter(inputProps) {
    return typeof inputProps === 'function';
}
function resolvePropsGetter(inputProps, previousProps) {
    if (isPropsGetter(inputProps)) {
        return inputProps(previousProps);
    }
    return inputProps ?? EMPTY_PROPS;
}
function mergeEventHandlers(ourHandler, theirHandler) {
    if (!theirHandler) {
        return ourHandler;
    }
    if (!ourHandler) {
        return theirHandler;
    }
    return (event)=>{
        if (isSyntheticEvent(event)) {
            const baseUIEvent = event;
            makeEventPreventable(baseUIEvent);
            const result = theirHandler(baseUIEvent);
            if (!baseUIEvent.baseUIHandlerPrevented) {
                ourHandler?.(baseUIEvent);
            }
            return result;
        }
        const result = theirHandler(event);
        ourHandler?.(event);
        return result;
    };
}
function makeEventPreventable(event) {
    event.preventBaseUIHandler = ()=>{
        event.baseUIHandlerPrevented = true;
    };
    return event;
}
function mergeClassNames(ourClassName, theirClassName) {
    if (theirClassName) {
        if (ourClassName) {
            // eslint-disable-next-line prefer-template
            return theirClassName + ' ' + ourClassName;
        }
        return theirClassName;
    }
    return ourClassName;
}
function isSyntheticEvent(event) {
    return event != null && typeof event === 'object' && 'nativeEvent' in event;
}
}),
"[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/utils/useRenderElement.js [app-client] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "useRenderElement",
    ()=>useRenderElement
]);
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$build$2f$polyfills$2f$process$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = /*#__PURE__*/ __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/build/polyfills/process.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/compiled/react/index.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useMergedRefs$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+utils@0.2.5+0ea9ec2a211d4613/node_modules/@base-ui/utils/esm/useMergedRefs.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$getReactElementRef$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+utils@0.2.5+0ea9ec2a211d4613/node_modules/@base-ui/utils/esm/getReactElementRef.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$mergeObjects$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+utils@0.2.5+0ea9ec2a211d4613/node_modules/@base-ui/utils/esm/mergeObjects.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$getStateAttributesProps$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/utils/getStateAttributesProps.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$resolveClassName$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/utils/resolveClassName.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$resolveStyle$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/utils/resolveStyle.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$merge$2d$props$2f$mergeProps$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/merge-props/mergeProps.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$empty$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+utils@0.2.5+0ea9ec2a211d4613/node_modules/@base-ui/utils/esm/empty.js [app-client] (ecmascript)");
;
;
;
;
;
;
;
;
;
;
;
function useRenderElement(element, componentProps, params = {}) {
    const renderProp = componentProps.render;
    const outProps = useRenderElementProps(componentProps, params);
    if (params.enabled === false) {
        return null;
    }
    const state = params.state ?? __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$empty$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["EMPTY_OBJECT"];
    return evaluateRenderProp(element, renderProp, outProps, state);
}
/**
 * Computes render element final props.
 */ function useRenderElementProps(componentProps, params = {}) {
    const { className: classNameProp, style: styleProp, render: renderProp } = componentProps;
    const { state = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$empty$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["EMPTY_OBJECT"], ref, props, stateAttributesMapping, enabled = true } = params;
    const className = enabled ? (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$resolveClassName$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["resolveClassName"])(classNameProp, state) : undefined;
    const style = enabled ? (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$resolveStyle$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["resolveStyle"])(styleProp, state) : undefined;
    const stateProps = enabled ? (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$getStateAttributesProps$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["getStateAttributesProps"])(state, stateAttributesMapping) : __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$empty$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["EMPTY_OBJECT"];
    const outProps = enabled ? (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$mergeObjects$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["mergeObjects"])(stateProps, Array.isArray(props) ? (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$merge$2d$props$2f$mergeProps$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["mergePropsN"])(props) : props) ?? __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$empty$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["EMPTY_OBJECT"] : __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$empty$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["EMPTY_OBJECT"];
    // SAFETY: The `useMergedRefs` functions use a single hook to store the same value,
    // switching between them at runtime is safe. If this assertion fails, React will
    // throw at runtime anyway.
    // This also skips the `useMergedRefs` call on the server, which is fine because
    // refs are not used on the server side.
    /* eslint-disable react-hooks/rules-of-hooks */ if (typeof document !== 'undefined') {
        if (!enabled) {
            (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useMergedRefs$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useMergedRefs"])(null, null);
        } else if (Array.isArray(ref)) {
            outProps.ref = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useMergedRefs$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useMergedRefsN"])([
                outProps.ref,
                (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$getReactElementRef$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["getReactElementRef"])(renderProp),
                ...ref
            ]);
        } else {
            outProps.ref = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useMergedRefs$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useMergedRefs"])(outProps.ref, (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$getReactElementRef$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["getReactElementRef"])(renderProp), ref);
        }
    }
    if (!enabled) {
        return __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$empty$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["EMPTY_OBJECT"];
    }
    if (className !== undefined) {
        outProps.className = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$merge$2d$props$2f$mergeProps$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["mergeClassNames"])(outProps.className, className);
    }
    if (style !== undefined) {
        outProps.style = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$mergeObjects$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["mergeObjects"])(outProps.style, style);
    }
    return outProps;
}
// The symbol React uses internally for lazy components
// https://github.com/facebook/react/blob/a0566250b210499b4c5677f5ac2eedbd71d51a1b/packages/shared/ReactSymbols.js#L31
//
// TODO delete once https://github.com/facebook/react/issues/32392 is fixed
const REACT_LAZY_TYPE = Symbol.for('react.lazy');
function evaluateRenderProp(element, render, props, state) {
    if (render) {
        if (typeof render === 'function') {
            return render(props, state);
        }
        const mergedProps = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$merge$2d$props$2f$mergeProps$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["mergeProps"])(props, render.props);
        mergedProps.ref = props.ref;
        let newElement = render;
        // Workaround for https://github.com/facebook/react/issues/32392
        // This works because the toArray() logic unwrap lazy element type in
        // https://github.com/facebook/react/blob/a0566250b210499b4c5677f5ac2eedbd71d51a1b/packages/react/src/ReactChildren.js#L186
        if (newElement?.$$typeof === REACT_LAZY_TYPE) {
            const children = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["Children"].toArray(render);
            newElement = children[0];
        }
        // There is a high number of indirections, the error message thrown by React.cloneElement() is
        // hard to use for developers, this logic provides a better context.
        //
        // Our general guideline is to never change the control flow depending on the environment.
        // However, React.cloneElement() throws if React.isValidElement() is false,
        // so we can throw before with custom message.
        if ("TURBOPACK compile-time truthy", 1) {
            if (!/*#__PURE__*/ __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["isValidElement"](newElement)) {
                throw new Error([
                    'Base UI: The `render` prop was provided an invalid React element as `React.isValidElement(render)` is `false`.',
                    'A valid React element must be provided to the `render` prop because it is cloned with props to replace the default element.',
                    'https://base-ui.com/r/invalid-render-prop'
                ].join('\n'));
            }
        }
        return /*#__PURE__*/ __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["cloneElement"](newElement, mergedProps);
    }
    if (element) {
        if (typeof element === 'string') {
            return renderTag(element, props);
        }
    }
    // Unreachable, but the typings on `useRenderElement` need to be reworked
    // to annotate it correctly.
    throw new Error(("TURBOPACK compile-time truthy", 1) ? 'Base UI: Render element or function are not defined.' : "TURBOPACK unreachable");
}
function renderTag(Tag, props) {
    if (Tag === 'button') {
        return /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["createElement"])("button", {
            type: "button",
            ...props,
            key: props.key
        });
    }
    if (Tag === 'img') {
        return /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["createElement"])("img", {
            alt: "",
            ...props,
            key: props.key
        });
    }
    return /*#__PURE__*/ __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["createElement"](Tag, props);
}
}),
"[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/utils/useBaseUiId.js [app-client] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "useBaseUiId",
    ()=>useBaseUiId
]);
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useId$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+utils@0.2.5+0ea9ec2a211d4613/node_modules/@base-ui/utils/esm/useId.js [app-client] (ecmascript)");
'use client';
;
function useBaseUiId(idOverride) {
    return (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useId$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useId"])(idOverride, 'base-ui');
}
}),
"[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/tooltip/provider/TooltipProviderContext.js [app-client] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "TooltipProviderContext",
    ()=>TooltipProviderContext,
    "useTooltipProviderContext",
    ()=>useTooltipProviderContext
]);
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$build$2f$polyfills$2f$process$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = /*#__PURE__*/ __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/build/polyfills/process.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/compiled/react/index.js [app-client] (ecmascript)");
'use client';
;
const TooltipProviderContext = /*#__PURE__*/ __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["createContext"](undefined);
if ("TURBOPACK compile-time truthy", 1) TooltipProviderContext.displayName = "TooltipProviderContext";
function useTooltipProviderContext() {
    return __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useContext"](TooltipProviderContext);
}
}),
"[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/floating-ui-react/safePolygon.js [app-client] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "safePolygon",
    ()=>safePolygon
]);
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$floating$2d$ui$2b$utils$40$0$2e$2$2e$11$2f$node_modules$2f40$floating$2d$ui$2f$utils$2f$dist$2f$floating$2d$ui$2e$utils$2e$dom$2e$mjs__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@floating-ui+utils@0.2.11/node_modules/@floating-ui/utils/dist/floating-ui.utils.dom.mjs [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useTimeout$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+utils@0.2.5+0ea9ec2a211d4613/node_modules/@base-ui/utils/esm/useTimeout.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$element$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/floating-ui-react/utils/element.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$nodes$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/floating-ui-react/utils/nodes.js [app-client] (ecmascript)");
;
;
;
;
/* eslint-disable no-nested-ternary */ function isPointInPolygon(point, polygon) {
    const [x, y] = point;
    let isInsideValue = false;
    const length = polygon.length;
    // eslint-disable-next-line no-plusplus
    for(let i = 0, j = length - 1; i < length; j = i++){
        const [xi, yi] = polygon[i] || [
            0,
            0
        ];
        const [xj, yj] = polygon[j] || [
            0,
            0
        ];
        const intersect = yi >= y !== yj >= y && x <= (xj - xi) * (y - yi) / (yj - yi) + xi;
        if (intersect) {
            isInsideValue = !isInsideValue;
        }
    }
    return isInsideValue;
}
function isInside(point, rect) {
    return point[0] >= rect.x && point[0] <= rect.x + rect.width && point[1] >= rect.y && point[1] <= rect.y + rect.height;
}
function safePolygon(options = {}) {
    const { buffer = 0.5, blockPointerEvents = false, requireIntent = true } = options;
    const timeout = new __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useTimeout$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["Timeout"]();
    let hasLanded = false;
    let lastX = null;
    let lastY = null;
    let lastCursorTime = typeof performance !== 'undefined' ? performance.now() : 0;
    function getCursorSpeed(x, y) {
        const currentTime = performance.now();
        const elapsedTime = currentTime - lastCursorTime;
        if (lastX === null || lastY === null || elapsedTime === 0) {
            lastX = x;
            lastY = y;
            lastCursorTime = currentTime;
            return null;
        }
        const deltaX = x - lastX;
        const deltaY = y - lastY;
        const distance = Math.sqrt(deltaX * deltaX + deltaY * deltaY);
        const speed = distance / elapsedTime; // px / ms
        lastX = x;
        lastY = y;
        lastCursorTime = currentTime;
        return speed;
    }
    const fn = ({ x, y, placement, elements, onClose, nodeId, tree })=>{
        return function onMouseMove(event) {
            function close() {
                timeout.clear();
                onClose();
            }
            timeout.clear();
            if (!elements.domReference || !elements.floating || placement == null || x == null || y == null) {
                return undefined;
            }
            const { clientX, clientY } = event;
            const clientPoint = [
                clientX,
                clientY
            ];
            const target = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$element$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["getTarget"])(event);
            const isLeave = event.type === 'mouseleave';
            const isOverFloatingEl = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$element$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["contains"])(elements.floating, target);
            const isOverReferenceEl = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$element$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["contains"])(elements.domReference, target);
            const refRect = elements.domReference.getBoundingClientRect();
            const rect = elements.floating.getBoundingClientRect();
            const side = placement.split('-')[0];
            const cursorLeaveFromRight = x > rect.right - rect.width / 2;
            const cursorLeaveFromBottom = y > rect.bottom - rect.height / 2;
            const isOverReferenceRect = isInside(clientPoint, refRect);
            const isFloatingWider = rect.width > refRect.width;
            const isFloatingTaller = rect.height > refRect.height;
            const left = (isFloatingWider ? refRect : rect).left;
            const right = (isFloatingWider ? refRect : rect).right;
            const top = (isFloatingTaller ? refRect : rect).top;
            const bottom = (isFloatingTaller ? refRect : rect).bottom;
            if (isOverFloatingEl) {
                hasLanded = true;
                if (!isLeave) {
                    return undefined;
                }
            }
            if (isOverReferenceEl) {
                hasLanded = false;
            }
            if (isOverReferenceEl && !isLeave) {
                hasLanded = true;
                return undefined;
            }
            // Prevent overlapping floating element from being stuck in an open-close
            // loop: https://github.com/floating-ui/floating-ui/issues/1910
            if (isLeave && (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$floating$2d$ui$2b$utils$40$0$2e$2$2e$11$2f$node_modules$2f40$floating$2d$ui$2f$utils$2f$dist$2f$floating$2d$ui$2e$utils$2e$dom$2e$mjs__$5b$app$2d$client$5d$__$28$ecmascript$29$__["isElement"])(event.relatedTarget) && (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$element$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["contains"])(elements.floating, event.relatedTarget)) {
                return undefined;
            }
            // If any nested child is open, abort.
            if (tree && (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$nodes$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["getNodeChildren"])(tree.nodesRef.current, nodeId).some(({ context })=>context?.open)) {
                return undefined;
            }
            // If the pointer is leaving from the opposite side, the "buffer" logic
            // creates a point where the floating element remains open, but should be
            // ignored.
            // A constant of 1 handles floating point rounding errors.
            if (side === 'top' && y >= refRect.bottom - 1 || side === 'bottom' && y <= refRect.top + 1 || side === 'left' && x >= refRect.right - 1 || side === 'right' && x <= refRect.left + 1) {
                return close();
            }
            // Ignore when the cursor is within the rectangular trough between the
            // two elements. Since the triangle is created from the cursor point,
            // which can start beyond the ref element's edge, traversing back and
            // forth from the ref to the floating element can cause it to close. This
            // ensures it always remains open in that case.
            let rectPoly = [];
            switch(side){
                case 'top':
                    rectPoly = [
                        [
                            left,
                            refRect.top + 1
                        ],
                        [
                            left,
                            rect.bottom - 1
                        ],
                        [
                            right,
                            rect.bottom - 1
                        ],
                        [
                            right,
                            refRect.top + 1
                        ]
                    ];
                    break;
                case 'bottom':
                    rectPoly = [
                        [
                            left,
                            rect.top + 1
                        ],
                        [
                            left,
                            refRect.bottom - 1
                        ],
                        [
                            right,
                            refRect.bottom - 1
                        ],
                        [
                            right,
                            rect.top + 1
                        ]
                    ];
                    break;
                case 'left':
                    rectPoly = [
                        [
                            rect.right - 1,
                            bottom
                        ],
                        [
                            rect.right - 1,
                            top
                        ],
                        [
                            refRect.left + 1,
                            top
                        ],
                        [
                            refRect.left + 1,
                            bottom
                        ]
                    ];
                    break;
                case 'right':
                    rectPoly = [
                        [
                            refRect.right - 1,
                            bottom
                        ],
                        [
                            refRect.right - 1,
                            top
                        ],
                        [
                            rect.left + 1,
                            top
                        ],
                        [
                            rect.left + 1,
                            bottom
                        ]
                    ];
                    break;
                default:
            }
            function getPolygon([px, py]) {
                switch(side){
                    case 'top':
                        {
                            const cursorPointOne = [
                                isFloatingWider ? px + buffer / 2 : cursorLeaveFromRight ? px + buffer * 4 : px - buffer * 4,
                                py + buffer + 1
                            ];
                            const cursorPointTwo = [
                                isFloatingWider ? px - buffer / 2 : cursorLeaveFromRight ? px + buffer * 4 : px - buffer * 4,
                                py + buffer + 1
                            ];
                            const commonPoints = [
                                [
                                    rect.left,
                                    cursorLeaveFromRight ? rect.bottom - buffer : isFloatingWider ? rect.bottom - buffer : rect.top
                                ],
                                [
                                    rect.right,
                                    cursorLeaveFromRight ? isFloatingWider ? rect.bottom - buffer : rect.top : rect.bottom - buffer
                                ]
                            ];
                            return [
                                cursorPointOne,
                                cursorPointTwo,
                                ...commonPoints
                            ];
                        }
                    case 'bottom':
                        {
                            const cursorPointOne = [
                                isFloatingWider ? px + buffer / 2 : cursorLeaveFromRight ? px + buffer * 4 : px - buffer * 4,
                                py - buffer
                            ];
                            const cursorPointTwo = [
                                isFloatingWider ? px - buffer / 2 : cursorLeaveFromRight ? px + buffer * 4 : px - buffer * 4,
                                py - buffer
                            ];
                            const commonPoints = [
                                [
                                    rect.left,
                                    cursorLeaveFromRight ? rect.top + buffer : isFloatingWider ? rect.top + buffer : rect.bottom
                                ],
                                [
                                    rect.right,
                                    cursorLeaveFromRight ? isFloatingWider ? rect.top + buffer : rect.bottom : rect.top + buffer
                                ]
                            ];
                            return [
                                cursorPointOne,
                                cursorPointTwo,
                                ...commonPoints
                            ];
                        }
                    case 'left':
                        {
                            const cursorPointOne = [
                                px + buffer + 1,
                                isFloatingTaller ? py + buffer / 2 : cursorLeaveFromBottom ? py + buffer * 4 : py - buffer * 4
                            ];
                            const cursorPointTwo = [
                                px + buffer + 1,
                                isFloatingTaller ? py - buffer / 2 : cursorLeaveFromBottom ? py + buffer * 4 : py - buffer * 4
                            ];
                            const commonPoints = [
                                [
                                    cursorLeaveFromBottom ? rect.right - buffer : isFloatingTaller ? rect.right - buffer : rect.left,
                                    rect.top
                                ],
                                [
                                    cursorLeaveFromBottom ? isFloatingTaller ? rect.right - buffer : rect.left : rect.right - buffer,
                                    rect.bottom
                                ]
                            ];
                            return [
                                ...commonPoints,
                                cursorPointOne,
                                cursorPointTwo
                            ];
                        }
                    case 'right':
                        {
                            const cursorPointOne = [
                                px - buffer,
                                isFloatingTaller ? py + buffer / 2 : cursorLeaveFromBottom ? py + buffer * 4 : py - buffer * 4
                            ];
                            const cursorPointTwo = [
                                px - buffer,
                                isFloatingTaller ? py - buffer / 2 : cursorLeaveFromBottom ? py + buffer * 4 : py - buffer * 4
                            ];
                            const commonPoints = [
                                [
                                    cursorLeaveFromBottom ? rect.left + buffer : isFloatingTaller ? rect.left + buffer : rect.right,
                                    rect.top
                                ],
                                [
                                    cursorLeaveFromBottom ? isFloatingTaller ? rect.left + buffer : rect.right : rect.left + buffer,
                                    rect.bottom
                                ]
                            ];
                            return [
                                cursorPointOne,
                                cursorPointTwo,
                                ...commonPoints
                            ];
                        }
                    default:
                        return [];
                }
            }
            if (isPointInPolygon([
                clientX,
                clientY
            ], rectPoly)) {
                return undefined;
            }
            if (hasLanded && !isOverReferenceRect) {
                return close();
            }
            if (!isLeave && requireIntent) {
                const cursorSpeed = getCursorSpeed(event.clientX, event.clientY);
                const cursorSpeedThreshold = 0.1;
                if (cursorSpeed !== null && cursorSpeed < cursorSpeedThreshold) {
                    return close();
                }
            }
            if (!isPointInPolygon([
                clientX,
                clientY
            ], getPolygon([
                x,
                y
            ]))) {
                close();
            } else if (!hasLanded && requireIntent) {
                timeout.start(40, close);
            }
            return undefined;
        };
    };
    // eslint-disable-next-line no-underscore-dangle
    fn.__options = {
        blockPointerEvents
    };
    return fn;
}
}),
"[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/floating-ui-react/hooks/useHover.js [app-client] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "getDelay",
    ()=>getDelay,
    "useHover",
    ()=>useHover
]);
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/compiled/react/index.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$floating$2d$ui$2b$utils$40$0$2e$2$2e$11$2f$node_modules$2f40$floating$2d$ui$2f$utils$2f$dist$2f$floating$2d$ui$2e$utils$2e$dom$2e$mjs__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@floating-ui+utils@0.2.11/node_modules/@floating-ui/utils/dist/floating-ui.utils.dom.mjs [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useTimeout$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+utils@0.2.5+0ea9ec2a211d4613/node_modules/@base-ui/utils/esm/useTimeout.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useValueAsRef$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+utils@0.2.5+0ea9ec2a211d4613/node_modules/@base-ui/utils/esm/useValueAsRef.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useStableCallback$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+utils@0.2.5+0ea9ec2a211d4613/node_modules/@base-ui/utils/esm/useStableCallback.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useIsoLayoutEffect$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+utils@0.2.5+0ea9ec2a211d4613/node_modules/@base-ui/utils/esm/useIsoLayoutEffect.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$owner$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$locals$3e$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+utils@0.2.5+0ea9ec2a211d4613/node_modules/@base-ui/utils/esm/owner.js [app-client] (ecmascript) <locals>");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$element$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/floating-ui-react/utils/element.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$event$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/floating-ui-react/utils/event.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$components$2f$FloatingTree$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/floating-ui-react/components/FloatingTree.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$createBaseUIEventDetails$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/utils/createBaseUIEventDetails.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$reason$2d$parts$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$export__$2a$__as__REASONS$3e$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/utils/reason-parts.js [app-client] (ecmascript) <export * as REASONS>");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$createAttribute$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/floating-ui-react/utils/createAttribute.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$constants$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/floating-ui-react/utils/constants.js [app-client] (ecmascript)");
'use client';
;
;
;
;
;
;
;
;
;
;
;
;
;
const safePolygonIdentifier = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$createAttribute$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["createAttribute"])('safe-polygon');
const interactiveSelector = `button,[role="button"],select,[tabindex]:not([tabindex="-1"]),${__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$constants$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["TYPEABLE_SELECTOR"]}`;
function isInteractiveElement(element) {
    return element ? Boolean(element.closest(interactiveSelector)) : false;
}
function getDelay(value, prop, pointerType) {
    if (pointerType && !(0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$event$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["isMouseLikePointerType"])(pointerType)) {
        return 0;
    }
    if (typeof value === 'number') {
        return value;
    }
    if (typeof value === 'function') {
        const result = value();
        if (typeof result === 'number') {
            return result;
        }
        return result?.[prop];
    }
    return value?.[prop];
}
function getRestMs(value) {
    if (typeof value === 'function') {
        return value();
    }
    return value;
}
function useHover(context, props = {}) {
    const store = 'rootStore' in context ? context.rootStore : context;
    const open = store.useState('open');
    const floatingElement = store.useState('floatingElement');
    const domReferenceElement = store.useState('domReferenceElement');
    const { dataRef, events } = store.context;
    const { delay = 0, handleClose = null, restMs = 0, move = true } = props;
    const tree = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$components$2f$FloatingTree$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useFloatingTree"])();
    const parentId = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$components$2f$FloatingTree$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useFloatingParentNodeId"])();
    const handleCloseRef = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useValueAsRef$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useValueAsRef"])(handleClose);
    const delayRef = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useValueAsRef$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useValueAsRef"])(delay);
    const restMsRef = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useValueAsRef$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useValueAsRef"])(restMs);
    const pointerTypeRef = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useRef"](undefined);
    const interactedInsideRef = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useRef"](false);
    const timeout = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useTimeout$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useTimeout"])();
    const handlerRef = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useRef"](undefined);
    const restTimeout = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useTimeout$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useTimeout"])();
    const blockMouseMoveRef = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useRef"](true);
    const performedPointerEventsMutationRef = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useRef"](false);
    const unbindMouseMoveRef = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useRef"]({
        "useHover.useRef[unbindMouseMoveRef]": ()=>{}
    }["useHover.useRef[unbindMouseMoveRef]"]);
    const restTimeoutPendingRef = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useRef"](false);
    const isHoverOpen = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useStableCallback$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useStableCallback"])({
        "useHover.useStableCallback[isHoverOpen]": ()=>{
            const type = dataRef.current.openEvent?.type;
            return type?.includes('mouse') && type !== 'mousedown';
        }
    }["useHover.useStableCallback[isHoverOpen]"]);
    const isClickLikeOpenEvent = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useStableCallback$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useStableCallback"])({
        "useHover.useStableCallback[isClickLikeOpenEvent]": ()=>{
            if (interactedInsideRef.current) {
                return true;
            }
            return dataRef.current.openEvent ? [
                'click',
                'mousedown'
            ].includes(dataRef.current.openEvent.type) : false;
        }
    }["useHover.useStableCallback[isClickLikeOpenEvent]"]);
    // When closing before opening, clear the delay timeouts to cancel it
    // from showing.
    __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useEffect"]({
        "useHover.useEffect": ()=>{
            function onOpenChangeLocal(details) {
                if (!details.open) {
                    timeout.clear();
                    restTimeout.clear();
                    blockMouseMoveRef.current = true;
                    restTimeoutPendingRef.current = false;
                }
            }
            events.on('openchange', onOpenChangeLocal);
            return ({
                "useHover.useEffect": ()=>{
                    events.off('openchange', onOpenChangeLocal);
                }
            })["useHover.useEffect"];
        }
    }["useHover.useEffect"], [
        events,
        timeout,
        restTimeout
    ]);
    __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useEffect"]({
        "useHover.useEffect": ()=>{
            if (!handleCloseRef.current) {
                return undefined;
            }
            if (!open) {
                return undefined;
            }
            function onLeave(event) {
                if (isClickLikeOpenEvent()) {
                    return;
                }
                if (isHoverOpen()) {
                    store.setOpen(false, (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$createBaseUIEventDetails$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["createChangeEventDetails"])(__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$reason$2d$parts$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$export__$2a$__as__REASONS$3e$__["REASONS"].triggerHover, event, event.currentTarget ?? undefined));
                }
            }
            const html = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$owner$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$locals$3e$__["ownerDocument"])(floatingElement).documentElement;
            html.addEventListener('mouseleave', onLeave);
            return ({
                "useHover.useEffect": ()=>{
                    html.removeEventListener('mouseleave', onLeave);
                }
            })["useHover.useEffect"];
        }
    }["useHover.useEffect"], [
        floatingElement,
        open,
        store,
        handleCloseRef,
        isHoverOpen,
        isClickLikeOpenEvent
    ]);
    const closeWithDelay = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useCallback"]({
        "useHover.useCallback[closeWithDelay]": (event, runElseBranch = true)=>{
            const closeDelay = getDelay(delayRef.current, 'close', pointerTypeRef.current);
            if (closeDelay && !handlerRef.current) {
                timeout.start(closeDelay, {
                    "useHover.useCallback[closeWithDelay]": ()=>store.setOpen(false, (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$createBaseUIEventDetails$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["createChangeEventDetails"])(__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$reason$2d$parts$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$export__$2a$__as__REASONS$3e$__["REASONS"].triggerHover, event))
                }["useHover.useCallback[closeWithDelay]"]);
            } else if (runElseBranch) {
                timeout.clear();
                store.setOpen(false, (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$createBaseUIEventDetails$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["createChangeEventDetails"])(__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$reason$2d$parts$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$export__$2a$__as__REASONS$3e$__["REASONS"].triggerHover, event));
            }
        }
    }["useHover.useCallback[closeWithDelay]"], [
        delayRef,
        store,
        timeout
    ]);
    const cleanupMouseMoveHandler = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useStableCallback$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useStableCallback"])({
        "useHover.useStableCallback[cleanupMouseMoveHandler]": ()=>{
            unbindMouseMoveRef.current();
            handlerRef.current = undefined;
        }
    }["useHover.useStableCallback[cleanupMouseMoveHandler]"]);
    const clearPointerEvents = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useStableCallback$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useStableCallback"])({
        "useHover.useStableCallback[clearPointerEvents]": ()=>{
            if (performedPointerEventsMutationRef.current) {
                const body = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$owner$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$locals$3e$__["ownerDocument"])(floatingElement).body;
                body.style.pointerEvents = '';
                body.removeAttribute(safePolygonIdentifier);
                performedPointerEventsMutationRef.current = false;
            }
        }
    }["useHover.useStableCallback[clearPointerEvents]"]);
    const handleInteractInside = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useStableCallback$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useStableCallback"])({
        "useHover.useStableCallback[handleInteractInside]": (event)=>{
            const target = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$element$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["getTarget"])(event);
            if (!isInteractiveElement(target)) {
                interactedInsideRef.current = false;
                return;
            }
            interactedInsideRef.current = true;
        }
    }["useHover.useStableCallback[handleInteractInside]"]);
    // Registering the mouse events on the reference directly to bypass React's
    // delegation system. If the cursor was on a disabled element and then entered
    // the reference (no gap), `mouseenter` doesn't fire in the delegation system.
    __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useEffect"]({
        "useHover.useEffect": ()=>{
            function onReferenceMouseEnter(event) {
                timeout.clear();
                blockMouseMoveRef.current = false;
                if (getRestMs(restMsRef.current) > 0 && !getDelay(delayRef.current, 'open')) {
                    return;
                }
                const openDelay = getDelay(delayRef.current, 'open', pointerTypeRef.current);
                const trigger = event.currentTarget ?? undefined;
                const domReference = store.select('domReferenceElement');
                const isOverInactiveTrigger = domReference && trigger && !(0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$element$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["contains"])(domReference, trigger);
                if (openDelay) {
                    timeout.start(openDelay, {
                        "useHover.useEffect.onReferenceMouseEnter": ()=>{
                            if (!store.select('open')) {
                                store.setOpen(true, (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$createBaseUIEventDetails$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["createChangeEventDetails"])(__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$reason$2d$parts$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$export__$2a$__as__REASONS$3e$__["REASONS"].triggerHover, event, trigger));
                            }
                        }
                    }["useHover.useEffect.onReferenceMouseEnter"]);
                } else if (!open || isOverInactiveTrigger) {
                    store.setOpen(true, (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$createBaseUIEventDetails$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["createChangeEventDetails"])(__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$reason$2d$parts$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$export__$2a$__as__REASONS$3e$__["REASONS"].triggerHover, event, trigger));
                }
            }
            function onReferenceMouseLeave(event) {
                if (isClickLikeOpenEvent()) {
                    clearPointerEvents();
                    return;
                }
                unbindMouseMoveRef.current();
                const doc = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$owner$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$locals$3e$__["ownerDocument"])(floatingElement);
                restTimeout.clear();
                restTimeoutPendingRef.current = false;
                const triggers = store.context.triggerElements;
                if (event.relatedTarget && triggers.hasElement(event.relatedTarget)) {
                    // If the mouse is leaving the reference element to another trigger, don't explicitly close the popup
                    // as it will be moved.
                    return;
                }
                if (handleCloseRef.current && dataRef.current.floatingContext) {
                    // Prevent clearing `onScrollMouseLeave` timeout.
                    if (!open) {
                        timeout.clear();
                    }
                    handlerRef.current = handleCloseRef.current({
                        ...dataRef.current.floatingContext,
                        tree,
                        x: event.clientX,
                        y: event.clientY,
                        onClose () {
                            clearPointerEvents();
                            cleanupMouseMoveHandler();
                            if (!isClickLikeOpenEvent()) {
                                closeWithDelay(event, true);
                            }
                        }
                    });
                    const handler = handlerRef.current;
                    doc.addEventListener('mousemove', handler);
                    unbindMouseMoveRef.current = ({
                        "useHover.useEffect.onReferenceMouseLeave": ()=>{
                            doc.removeEventListener('mousemove', handler);
                        }
                    })["useHover.useEffect.onReferenceMouseLeave"];
                    return;
                }
                // Allow interactivity without `safePolygon` on touch devices. With a
                // pointer, a short close delay is an alternative, so it should work
                // consistently.
                const shouldClose = pointerTypeRef.current === 'touch' ? !(0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$element$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["contains"])(floatingElement, event.relatedTarget) : true;
                if (shouldClose) {
                    closeWithDelay(event);
                }
            }
            // Ensure the floating element closes after scrolling even if the pointer
            // did not move.
            // https://github.com/floating-ui/floating-ui/discussions/1692
            function onScrollMouseLeave(event) {
                if (isClickLikeOpenEvent() || !dataRef.current.floatingContext || !store.select('open')) {
                    return;
                }
                const triggers = store.context.triggerElements;
                if (event.relatedTarget && triggers.hasElement(event.relatedTarget)) {
                    // If the mouse is leaving the reference element to another trigger, don't explicitly close the popup
                    // as it will be moved.
                    return;
                }
                handleCloseRef.current?.({
                    ...dataRef.current.floatingContext,
                    tree,
                    x: event.clientX,
                    y: event.clientY,
                    onClose () {
                        clearPointerEvents();
                        cleanupMouseMoveHandler();
                        if (!isClickLikeOpenEvent()) {
                            closeWithDelay(event);
                        }
                    }
                })(event);
            }
            function onFloatingMouseEnter() {
                timeout.clear();
                clearPointerEvents();
            }
            function onFloatingMouseLeave(event) {
                if (!isClickLikeOpenEvent()) {
                    closeWithDelay(event, false);
                }
            }
            const trigger = domReferenceElement;
            if ((0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$floating$2d$ui$2b$utils$40$0$2e$2$2e$11$2f$node_modules$2f40$floating$2d$ui$2f$utils$2f$dist$2f$floating$2d$ui$2e$utils$2e$dom$2e$mjs__$5b$app$2d$client$5d$__$28$ecmascript$29$__["isElement"])(trigger)) {
                const floating = floatingElement;
                if (open) {
                    trigger.addEventListener('mouseleave', onScrollMouseLeave);
                }
                if (move) {
                    trigger.addEventListener('mousemove', onReferenceMouseEnter, {
                        once: true
                    });
                }
                trigger.addEventListener('mouseenter', onReferenceMouseEnter);
                trigger.addEventListener('mouseleave', onReferenceMouseLeave);
                if (floating) {
                    floating.addEventListener('mouseleave', onScrollMouseLeave);
                    floating.addEventListener('mouseenter', onFloatingMouseEnter);
                    floating.addEventListener('mouseleave', onFloatingMouseLeave);
                    floating.addEventListener('pointerdown', handleInteractInside, true);
                }
                return ({
                    "useHover.useEffect": ()=>{
                        if (open) {
                            trigger.removeEventListener('mouseleave', onScrollMouseLeave);
                        }
                        if (move) {
                            trigger.removeEventListener('mousemove', onReferenceMouseEnter);
                        }
                        trigger.removeEventListener('mouseenter', onReferenceMouseEnter);
                        trigger.removeEventListener('mouseleave', onReferenceMouseLeave);
                        if (floating) {
                            floating.removeEventListener('mouseleave', onScrollMouseLeave);
                            floating.removeEventListener('mouseenter', onFloatingMouseEnter);
                            floating.removeEventListener('mouseleave', onFloatingMouseLeave);
                            floating.removeEventListener('pointerdown', handleInteractInside, true);
                        }
                    }
                })["useHover.useEffect"];
            }
            return undefined;
        }
    }["useHover.useEffect"], [
        move,
        domReferenceElement,
        floatingElement,
        store,
        closeWithDelay,
        cleanupMouseMoveHandler,
        clearPointerEvents,
        open,
        tree,
        delayRef,
        handleCloseRef,
        dataRef,
        isClickLikeOpenEvent,
        restMsRef,
        timeout,
        restTimeout,
        handleInteractInside
    ]);
    // Block pointer-events of every element other than the reference and floating
    // while the floating element is open and has a `handleClose` handler. Also
    // handles nested floating elements.
    // https://github.com/floating-ui/floating-ui/issues/1722
    (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useIsoLayoutEffect$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useIsoLayoutEffect"])({
        "useHover.useIsoLayoutEffect": ()=>{
            // eslint-disable-next-line no-underscore-dangle
            if (open && handleCloseRef.current?.__options?.blockPointerEvents && isHoverOpen()) {
                performedPointerEventsMutationRef.current = true;
                const floatingEl = floatingElement;
                if ((0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$floating$2d$ui$2b$utils$40$0$2e$2$2e$11$2f$node_modules$2f40$floating$2d$ui$2f$utils$2f$dist$2f$floating$2d$ui$2e$utils$2e$dom$2e$mjs__$5b$app$2d$client$5d$__$28$ecmascript$29$__["isElement"])(domReferenceElement) && floatingEl) {
                    const body = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$owner$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$locals$3e$__["ownerDocument"])(floatingElement).body;
                    body.setAttribute(safePolygonIdentifier, '');
                    const ref = domReferenceElement;
                    const parentFloating = tree?.nodesRef.current.find({
                        "useHover.useIsoLayoutEffect": (node)=>node.id === parentId
                    }["useHover.useIsoLayoutEffect"])?.context?.elements.floating;
                    if (parentFloating) {
                        parentFloating.style.pointerEvents = '';
                    }
                    body.style.pointerEvents = 'none';
                    ref.style.pointerEvents = 'auto';
                    floatingEl.style.pointerEvents = 'auto';
                    return ({
                        "useHover.useIsoLayoutEffect": ()=>{
                            body.style.pointerEvents = '';
                            ref.style.pointerEvents = '';
                            floatingEl.style.pointerEvents = '';
                        }
                    })["useHover.useIsoLayoutEffect"];
                }
            }
            return undefined;
        }
    }["useHover.useIsoLayoutEffect"], [
        open,
        parentId,
        tree,
        handleCloseRef,
        isHoverOpen,
        domReferenceElement,
        floatingElement
    ]);
    (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useIsoLayoutEffect$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useIsoLayoutEffect"])({
        "useHover.useIsoLayoutEffect": ()=>{
            if (!open) {
                pointerTypeRef.current = undefined;
                restTimeoutPendingRef.current = false;
                interactedInsideRef.current = false;
                cleanupMouseMoveHandler();
                clearPointerEvents();
            }
        }
    }["useHover.useIsoLayoutEffect"], [
        open,
        cleanupMouseMoveHandler,
        clearPointerEvents
    ]);
    __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useEffect"]({
        "useHover.useEffect": ()=>{
            return ({
                "useHover.useEffect": ()=>{
                    cleanupMouseMoveHandler();
                    timeout.clear();
                    restTimeout.clear();
                    interactedInsideRef.current = false;
                }
            })["useHover.useEffect"];
        }
    }["useHover.useEffect"], [
        domReferenceElement,
        cleanupMouseMoveHandler,
        timeout,
        restTimeout
    ]);
    __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useEffect"]({
        "useHover.useEffect": ()=>{
            return clearPointerEvents;
        }
    }["useHover.useEffect"], [
        clearPointerEvents
    ]);
    const reference = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useMemo"]({
        "useHover.useMemo[reference]": ()=>{
            function setPointerRef(event) {
                pointerTypeRef.current = event.pointerType;
            }
            return {
                onPointerDown: setPointerRef,
                onPointerEnter: setPointerRef,
                onMouseMove (event) {
                    const { nativeEvent } = event;
                    const trigger = event.currentTarget;
                    // `true` when there are multiple triggers per floating element and user hovers over the one that
                    // wasn't used to open the floating element.
                    const isOverInactiveTrigger = store.select('domReferenceElement') && !(0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$element$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["contains"])(store.select('domReferenceElement'), event.target);
                    function handleMouseMove() {
                        if (!blockMouseMoveRef.current && (!store.select('open') || isOverInactiveTrigger)) {
                            store.setOpen(true, (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$createBaseUIEventDetails$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["createChangeEventDetails"])(__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$reason$2d$parts$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$export__$2a$__as__REASONS$3e$__["REASONS"].triggerHover, nativeEvent, trigger));
                        }
                    }
                    if (store.select('open') && !isOverInactiveTrigger || getRestMs(restMsRef.current) === 0) {
                        return;
                    }
                    // Ignore insignificant movements to account for tremors.
                    if (!isOverInactiveTrigger && restTimeoutPendingRef.current && event.movementX ** 2 + event.movementY ** 2 < 2) {
                        return;
                    }
                    restTimeout.clear();
                    if (pointerTypeRef.current === 'touch') {
                        handleMouseMove();
                    } else if (isOverInactiveTrigger) {
                        handleMouseMove();
                    } else {
                        restTimeoutPendingRef.current = true;
                        restTimeout.start(getRestMs(restMsRef.current), handleMouseMove);
                    }
                }
            };
        }
    }["useHover.useMemo[reference]"], [
        store,
        restMsRef,
        restTimeout
    ]);
    return __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useMemo"]({
        "useHover.useMemo": ()=>({
                reference
            })
    }["useHover.useMemo"], [
        reference
    ]);
}
}),
"[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/floating-ui-react/components/FloatingDelayGroup.js [app-client] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "FloatingDelayGroup",
    ()=>FloatingDelayGroup,
    "useDelayGroup",
    ()=>useDelayGroup
]);
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$build$2f$polyfills$2f$process$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = /*#__PURE__*/ __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/build/polyfills/process.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/compiled/react/index.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useTimeout$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+utils@0.2.5+0ea9ec2a211d4613/node_modules/@base-ui/utils/esm/useTimeout.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useIsoLayoutEffect$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+utils@0.2.5+0ea9ec2a211d4613/node_modules/@base-ui/utils/esm/useIsoLayoutEffect.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$hooks$2f$useHover$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/floating-ui-react/hooks/useHover.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$createBaseUIEventDetails$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/utils/createBaseUIEventDetails.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$reason$2d$parts$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$export__$2a$__as__REASONS$3e$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/utils/reason-parts.js [app-client] (ecmascript) <export * as REASONS>");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/compiled/react/jsx-runtime.js [app-client] (ecmascript)");
'use client';
;
;
;
;
;
;
;
const FloatingDelayGroupContext = /*#__PURE__*/ __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["createContext"]({
    hasProvider: false,
    timeoutMs: 0,
    delayRef: {
        current: 0
    },
    initialDelayRef: {
        current: 0
    },
    timeout: new __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useTimeout$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["Timeout"](),
    currentIdRef: {
        current: null
    },
    currentContextRef: {
        current: null
    }
});
if ("TURBOPACK compile-time truthy", 1) FloatingDelayGroupContext.displayName = "FloatingDelayGroupContext";
function FloatingDelayGroup(props) {
    const { children, delay, timeoutMs = 0 } = props;
    const delayRef = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useRef"](delay);
    const initialDelayRef = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useRef"](delay);
    const currentIdRef = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useRef"](null);
    const currentContextRef = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useRef"](null);
    const timeout = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useTimeout$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useTimeout"])();
    return /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsx"])(FloatingDelayGroupContext.Provider, {
        value: __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useMemo"]({
            "FloatingDelayGroup.useMemo": ()=>({
                    hasProvider: true,
                    delayRef,
                    initialDelayRef,
                    currentIdRef,
                    timeoutMs,
                    currentContextRef,
                    timeout
                })
        }["FloatingDelayGroup.useMemo"], [
            timeoutMs,
            timeout
        ]),
        children: children
    });
}
function useDelayGroup(context, options = {
    open: false
}) {
    const store = 'rootStore' in context ? context.rootStore : context;
    const floatingId = store.useState('floatingId');
    const { open } = options;
    const groupContext = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useContext"](FloatingDelayGroupContext);
    const { currentIdRef, delayRef, timeoutMs, initialDelayRef, currentContextRef, hasProvider, timeout } = groupContext;
    const [isInstantPhase, setIsInstantPhase] = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useState"](false);
    (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useIsoLayoutEffect$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useIsoLayoutEffect"])({
        "useDelayGroup.useIsoLayoutEffect": ()=>{
            function unset() {
                setIsInstantPhase(false);
                currentContextRef.current?.setIsInstantPhase(false);
                currentIdRef.current = null;
                currentContextRef.current = null;
                delayRef.current = initialDelayRef.current;
            }
            if (!currentIdRef.current) {
                return undefined;
            }
            if (!open && currentIdRef.current === floatingId) {
                setIsInstantPhase(false);
                if (timeoutMs) {
                    const closingId = floatingId;
                    timeout.start(timeoutMs, {
                        "useDelayGroup.useIsoLayoutEffect": ()=>{
                            // If another tooltip has taken over the group, skip resetting.
                            if (store.select('open') || currentIdRef.current && currentIdRef.current !== closingId) {
                                return;
                            }
                            unset();
                        }
                    }["useDelayGroup.useIsoLayoutEffect"]);
                    return ({
                        "useDelayGroup.useIsoLayoutEffect": ()=>{
                            timeout.clear();
                        }
                    })["useDelayGroup.useIsoLayoutEffect"];
                }
                unset();
            }
            return undefined;
        }
    }["useDelayGroup.useIsoLayoutEffect"], [
        open,
        floatingId,
        currentIdRef,
        delayRef,
        timeoutMs,
        initialDelayRef,
        currentContextRef,
        timeout,
        store
    ]);
    (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useIsoLayoutEffect$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useIsoLayoutEffect"])({
        "useDelayGroup.useIsoLayoutEffect": ()=>{
            if (!open) {
                return;
            }
            const prevContext = currentContextRef.current;
            const prevId = currentIdRef.current;
            // A new tooltip is opening, so cancel any pending timeout that would reset
            // the group's delay back to the initial value.
            timeout.clear();
            currentContextRef.current = {
                onOpenChange: store.setOpen,
                setIsInstantPhase
            };
            currentIdRef.current = floatingId;
            delayRef.current = {
                open: 0,
                close: (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$hooks$2f$useHover$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["getDelay"])(initialDelayRef.current, 'close')
            };
            if (prevId !== null && prevId !== floatingId) {
                setIsInstantPhase(true);
                prevContext?.setIsInstantPhase(true);
                prevContext?.onOpenChange(false, (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$createBaseUIEventDetails$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["createChangeEventDetails"])(__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$reason$2d$parts$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$export__$2a$__as__REASONS$3e$__["REASONS"].none));
            } else {
                setIsInstantPhase(false);
                prevContext?.setIsInstantPhase(false);
            }
        }
    }["useDelayGroup.useIsoLayoutEffect"], [
        open,
        floatingId,
        store,
        currentIdRef,
        delayRef,
        timeoutMs,
        initialDelayRef,
        currentContextRef,
        timeout
    ]);
    (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useIsoLayoutEffect$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useIsoLayoutEffect"])({
        "useDelayGroup.useIsoLayoutEffect": ()=>{
            return ({
                "useDelayGroup.useIsoLayoutEffect": ()=>{
                    currentContextRef.current = null;
                }
            })["useDelayGroup.useIsoLayoutEffect"];
        }
    }["useDelayGroup.useIsoLayoutEffect"], [
        currentContextRef
    ]);
    return __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useMemo"]({
        "useDelayGroup.useMemo": ()=>({
                hasProvider,
                delayRef,
                isInstantPhase
            })
    }["useDelayGroup.useMemo"], [
        hasProvider,
        delayRef,
        isInstantPhase
    ]);
}
}),
"[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/floating-ui-react/hooks/useFocus.js [app-client] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "useFocus",
    ()=>useFocus
]);
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/compiled/react/index.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$floating$2d$ui$2b$utils$40$0$2e$2$2e$11$2f$node_modules$2f40$floating$2d$ui$2f$utils$2f$dist$2f$floating$2d$ui$2e$utils$2e$dom$2e$mjs__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@floating-ui+utils@0.2.11/node_modules/@floating-ui/utils/dist/floating-ui.utils.dom.mjs [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$detectBrowser$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+utils@0.2.5+0ea9ec2a211d4613/node_modules/@base-ui/utils/esm/detectBrowser.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useTimeout$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+utils@0.2.5+0ea9ec2a211d4613/node_modules/@base-ui/utils/esm/useTimeout.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$owner$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$locals$3e$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+utils@0.2.5+0ea9ec2a211d4613/node_modules/@base-ui/utils/esm/owner.js [app-client] (ecmascript) <locals>");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$element$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/floating-ui-react/utils/element.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$createBaseUIEventDetails$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/utils/createBaseUIEventDetails.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$reason$2d$parts$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$export__$2a$__as__REASONS$3e$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/utils/reason-parts.js [app-client] (ecmascript) <export * as REASONS>");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$createAttribute$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/floating-ui-react/utils/createAttribute.js [app-client] (ecmascript)");
'use client';
;
;
;
;
;
;
;
;
;
const isMacSafari = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$detectBrowser$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["isMac"] && __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$detectBrowser$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["isSafari"];
function useFocus(context, props = {}) {
    const store = 'rootStore' in context ? context.rootStore : context;
    const { events, dataRef } = store.context;
    const { enabled = true, delay } = props;
    const blockFocusRef = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useRef"](false);
    // Track which reference should be blocked from re-opening after Escape/press dismissal.
    const blockedReferenceRef = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useRef"](null);
    const timeout = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useTimeout$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useTimeout"])();
    const keyboardModalityRef = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useRef"](true);
    __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useEffect"]({
        "useFocus.useEffect": ()=>{
            const domReference = store.select('domReferenceElement');
            if (!enabled) {
                return undefined;
            }
            const win = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$floating$2d$ui$2b$utils$40$0$2e$2$2e$11$2f$node_modules$2f40$floating$2d$ui$2f$utils$2f$dist$2f$floating$2d$ui$2e$utils$2e$dom$2e$mjs__$5b$app$2d$client$5d$__$28$ecmascript$29$__["getWindow"])(domReference);
            // If the reference was focused and the user left the tab/window, and the
            // floating element was not open, the focus should be blocked when they
            // return to the tab/window.
            function onBlur() {
                const currentDomReference = store.select('domReferenceElement');
                if (!store.select('open') && (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$floating$2d$ui$2b$utils$40$0$2e$2$2e$11$2f$node_modules$2f40$floating$2d$ui$2f$utils$2f$dist$2f$floating$2d$ui$2e$utils$2e$dom$2e$mjs__$5b$app$2d$client$5d$__$28$ecmascript$29$__["isHTMLElement"])(currentDomReference) && currentDomReference === (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$element$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["activeElement"])((0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$owner$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$locals$3e$__["ownerDocument"])(currentDomReference))) {
                    blockFocusRef.current = true;
                }
            }
            function onKeyDown() {
                keyboardModalityRef.current = true;
            }
            function onPointerDown() {
                keyboardModalityRef.current = false;
            }
            win.addEventListener('blur', onBlur);
            if (isMacSafari) {
                win.addEventListener('keydown', onKeyDown, true);
                win.addEventListener('pointerdown', onPointerDown, true);
            }
            return ({
                "useFocus.useEffect": ()=>{
                    win.removeEventListener('blur', onBlur);
                    if (isMacSafari) {
                        win.removeEventListener('keydown', onKeyDown, true);
                        win.removeEventListener('pointerdown', onPointerDown, true);
                    }
                }
            })["useFocus.useEffect"];
        }
    }["useFocus.useEffect"], [
        store,
        enabled
    ]);
    __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useEffect"]({
        "useFocus.useEffect": ()=>{
            if (!enabled) {
                return undefined;
            }
            function onOpenChangeLocal(details) {
                if (details.reason === __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$reason$2d$parts$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$export__$2a$__as__REASONS$3e$__["REASONS"].triggerPress || details.reason === __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$reason$2d$parts$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$export__$2a$__as__REASONS$3e$__["REASONS"].escapeKey) {
                    const referenceElement = store.select('domReferenceElement');
                    if ((0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$floating$2d$ui$2b$utils$40$0$2e$2$2e$11$2f$node_modules$2f40$floating$2d$ui$2f$utils$2f$dist$2f$floating$2d$ui$2e$utils$2e$dom$2e$mjs__$5b$app$2d$client$5d$__$28$ecmascript$29$__["isElement"])(referenceElement)) {
                        blockedReferenceRef.current = referenceElement;
                        blockFocusRef.current = true;
                    }
                }
            }
            events.on('openchange', onOpenChangeLocal);
            return ({
                "useFocus.useEffect": ()=>{
                    events.off('openchange', onOpenChangeLocal);
                }
            })["useFocus.useEffect"];
        }
    }["useFocus.useEffect"], [
        events,
        enabled,
        store
    ]);
    const reference = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useMemo"]({
        "useFocus.useMemo[reference]": ()=>({
                onMouseLeave () {
                    blockFocusRef.current = false;
                    blockedReferenceRef.current = null;
                },
                onFocus (event) {
                    const focusTarget = event.currentTarget;
                    if (blockFocusRef.current) {
                        if (blockedReferenceRef.current === focusTarget) {
                            return;
                        }
                        blockFocusRef.current = false;
                        blockedReferenceRef.current = null;
                    }
                    const target = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$element$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["getTarget"])(event.nativeEvent);
                    if ((0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$floating$2d$ui$2b$utils$40$0$2e$2$2e$11$2f$node_modules$2f40$floating$2d$ui$2f$utils$2f$dist$2f$floating$2d$ui$2e$utils$2e$dom$2e$mjs__$5b$app$2d$client$5d$__$28$ecmascript$29$__["isElement"])(target)) {
                        // Safari fails to match `:focus-visible` if focus was initially
                        // outside the document.
                        if (isMacSafari && !event.relatedTarget) {
                            if (!keyboardModalityRef.current && !(0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$element$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["isTypeableElement"])(target)) {
                                return;
                            }
                        } else if (!(0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$element$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["matchesFocusVisible"])(target)) {
                            return;
                        }
                    }
                    const movedFromOtherEnabledTrigger = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$element$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["isTargetInsideEnabledTrigger"])(event.relatedTarget, store.context.triggerElements);
                    const { nativeEvent, currentTarget } = event;
                    const delayValue = typeof delay === 'function' ? delay() : delay;
                    if (store.select('open') && movedFromOtherEnabledTrigger || delayValue === 0 || delayValue === undefined) {
                        store.setOpen(true, (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$createBaseUIEventDetails$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["createChangeEventDetails"])(__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$reason$2d$parts$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$export__$2a$__as__REASONS$3e$__["REASONS"].triggerFocus, nativeEvent, currentTarget));
                        return;
                    }
                    timeout.start(delayValue, {
                        "useFocus.useMemo[reference]": ()=>{
                            if (blockFocusRef.current) {
                                return;
                            }
                            store.setOpen(true, (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$createBaseUIEventDetails$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["createChangeEventDetails"])(__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$reason$2d$parts$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$export__$2a$__as__REASONS$3e$__["REASONS"].triggerFocus, nativeEvent, currentTarget));
                        }
                    }["useFocus.useMemo[reference]"]);
                },
                onBlur (event) {
                    blockFocusRef.current = false;
                    blockedReferenceRef.current = null;
                    const relatedTarget = event.relatedTarget;
                    const nativeEvent = event.nativeEvent;
                    // Hit the non-modal focus management portal guard. Focus will be
                    // moved into the floating element immediately after.
                    const movedToFocusGuard = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$floating$2d$ui$2b$utils$40$0$2e$2$2e$11$2f$node_modules$2f40$floating$2d$ui$2f$utils$2f$dist$2f$floating$2d$ui$2e$utils$2e$dom$2e$mjs__$5b$app$2d$client$5d$__$28$ecmascript$29$__["isElement"])(relatedTarget) && relatedTarget.hasAttribute((0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$createAttribute$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["createAttribute"])('focus-guard')) && relatedTarget.getAttribute('data-type') === 'outside';
                    // Wait for the window blur listener to fire.
                    timeout.start(0, {
                        "useFocus.useMemo[reference]": ()=>{
                            const domReference = store.select('domReferenceElement');
                            const activeEl = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$element$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["activeElement"])(domReference ? domReference.ownerDocument : document);
                            // Focus left the page, keep it open.
                            if (!relatedTarget && activeEl === domReference) {
                                return;
                            }
                            // When focusing the reference element (e.g. regular click), then
                            // clicking into the floating element, prevent it from hiding.
                            // Note: it must be focusable, e.g. `tabindex="-1"`.
                            // We can not rely on relatedTarget to point to the correct element
                            // as it will only point to the shadow host of the newly focused element
                            // and not the element that actually has received focus if it is located
                            // inside a shadow root.
                            if ((0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$element$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["contains"])(dataRef.current.floatingContext?.refs.floating.current, activeEl) || (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$element$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["contains"])(domReference, activeEl) || movedToFocusGuard) {
                                return;
                            }
                            // If the next focused element is one of the triggers, do not close
                            // the floating element. The focus handler of that trigger will
                            // handle the open state.
                            const nextFocusedElement = relatedTarget ?? activeEl;
                            if ((0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$element$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["isTargetInsideEnabledTrigger"])(nextFocusedElement, store.context.triggerElements)) {
                                return;
                            }
                            store.setOpen(false, (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$createBaseUIEventDetails$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["createChangeEventDetails"])(__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$reason$2d$parts$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$export__$2a$__as__REASONS$3e$__["REASONS"].triggerFocus, nativeEvent));
                        }
                    }["useFocus.useMemo[reference]"]);
                }
            })
    }["useFocus.useMemo[reference]"], [
        dataRef,
        store,
        timeout,
        delay
    ]);
    return __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useMemo"]({
        "useFocus.useMemo": ()=>enabled ? {
                reference,
                trigger: reference
            } : {}
    }["useFocus.useMemo"], [
        enabled,
        reference
    ]);
}
}),
"[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/floating-ui-react/hooks/useHoverInteractionSharedState.js [app-client] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "HoverInteraction",
    ()=>HoverInteraction,
    "isInteractiveElement",
    ()=>isInteractiveElement,
    "safePolygonIdentifier",
    ()=>safePolygonIdentifier,
    "useHoverInteractionSharedState",
    ()=>useHoverInteractionSharedState
]);
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useOnMount$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+utils@0.2.5+0ea9ec2a211d4613/node_modules/@base-ui/utils/esm/useOnMount.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useRefWithInit$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+utils@0.2.5+0ea9ec2a211d4613/node_modules/@base-ui/utils/esm/useRefWithInit.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useTimeout$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+utils@0.2.5+0ea9ec2a211d4613/node_modules/@base-ui/utils/esm/useTimeout.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$createAttribute$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/floating-ui-react/utils/createAttribute.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$constants$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/floating-ui-react/utils/constants.js [app-client] (ecmascript)");
'use client';
;
;
;
;
;
const safePolygonIdentifier = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$createAttribute$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["createAttribute"])('safe-polygon');
const interactiveSelector = `button,a,[role="button"],select,[tabindex]:not([tabindex="-1"]),${__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$constants$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["TYPEABLE_SELECTOR"]}`;
function isInteractiveElement(element) {
    return element ? Boolean(element.closest(interactiveSelector)) : false;
}
class HoverInteraction {
    constructor(){
        this.pointerType = undefined;
        this.interactedInside = false;
        this.handler = undefined;
        this.blockMouseMove = true;
        this.performedPointerEventsMutation = false;
        this.unbindMouseMove = ()=>{};
        this.restTimeoutPending = false;
        this.openChangeTimeout = new __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useTimeout$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["Timeout"]();
        this.restTimeout = new __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useTimeout$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["Timeout"]();
        this.handleCloseOptions = undefined;
    }
    static create() {
        return new HoverInteraction();
    }
    dispose = ()=>{
        this.openChangeTimeout.clear();
        this.restTimeout.clear();
    };
    disposeEffect = ()=>{
        return this.dispose;
    };
}
function useHoverInteractionSharedState(store) {
    const instance = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useRefWithInit$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useRefWithInit"])(HoverInteraction.create).current;
    const data = store.context.dataRef.current;
    if (!data.hoverInteractionState) {
        data.hoverInteractionState = instance;
    }
    (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useOnMount$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useOnMount"])(data.hoverInteractionState.disposeEffect);
    return data.hoverInteractionState;
}
}),
"[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/floating-ui-react/hooks/useHoverReferenceInteraction.js [app-client] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "useHoverReferenceInteraction",
    ()=>useHoverReferenceInteraction
]);
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/compiled/react/index.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2d$dom$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/compiled/react-dom/index.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$floating$2d$ui$2b$utils$40$0$2e$2$2e$11$2f$node_modules$2f40$floating$2d$ui$2f$utils$2f$dist$2f$floating$2d$ui$2e$utils$2e$dom$2e$mjs__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@floating-ui+utils@0.2.11/node_modules/@floating-ui/utils/dist/floating-ui.utils.dom.mjs [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useValueAsRef$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+utils@0.2.5+0ea9ec2a211d4613/node_modules/@base-ui/utils/esm/useValueAsRef.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useStableCallback$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+utils@0.2.5+0ea9ec2a211d4613/node_modules/@base-ui/utils/esm/useStableCallback.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$owner$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$locals$3e$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+utils@0.2.5+0ea9ec2a211d4613/node_modules/@base-ui/utils/esm/owner.js [app-client] (ecmascript) <locals>");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$element$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/floating-ui-react/utils/element.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$event$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/floating-ui-react/utils/event.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$createBaseUIEventDetails$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/utils/createBaseUIEventDetails.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$reason$2d$parts$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$export__$2a$__as__REASONS$3e$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/utils/reason-parts.js [app-client] (ecmascript) <export * as REASONS>");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$hooks$2f$useHover$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/floating-ui-react/hooks/useHover.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$components$2f$FloatingTree$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/floating-ui-react/components/FloatingTree.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$hooks$2f$useHoverInteractionSharedState$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/floating-ui-react/hooks/useHoverInteractionSharedState.js [app-client] (ecmascript)");
'use client';
;
;
;
;
;
;
;
;
;
;
;
;
function getRestMs(value) {
    if (typeof value === 'function') {
        return value();
    }
    return value;
}
const EMPTY_REF = {
    current: null
};
function useHoverReferenceInteraction(context, props = {}) {
    const store = 'rootStore' in context ? context.rootStore : context;
    const { dataRef, events } = store.context;
    const { enabled = true, delay = 0, handleClose = null, mouseOnly = false, restMs = 0, move = true, triggerElementRef = EMPTY_REF, externalTree, isActiveTrigger = true } = props;
    const tree = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$components$2f$FloatingTree$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useFloatingTree"])(externalTree);
    const instance = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$hooks$2f$useHoverInteractionSharedState$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useHoverInteractionSharedState"])(store);
    const handleCloseRef = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useValueAsRef$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useValueAsRef"])(handleClose);
    const delayRef = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useValueAsRef$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useValueAsRef"])(delay);
    const restMsRef = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useValueAsRef$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useValueAsRef"])(restMs);
    const enabledRef = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useValueAsRef$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useValueAsRef"])(enabled);
    if (isActiveTrigger) {
        // eslint-disable-next-line no-underscore-dangle
        instance.handleCloseOptions = handleCloseRef.current?.__options;
    }
    const isClickLikeOpenEvent = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useStableCallback$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useStableCallback"])({
        "useHoverReferenceInteraction.useStableCallback[isClickLikeOpenEvent]": ()=>{
            if (instance.interactedInside) {
                return true;
            }
            return dataRef.current.openEvent ? [
                'click',
                'mousedown'
            ].includes(dataRef.current.openEvent.type) : false;
        }
    }["useHoverReferenceInteraction.useStableCallback[isClickLikeOpenEvent]"]);
    const isRelatedTargetInsideEnabledTrigger = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useStableCallback$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useStableCallback"])({
        "useHoverReferenceInteraction.useStableCallback[isRelatedTargetInsideEnabledTrigger]": (target)=>{
            return (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$element$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["isTargetInsideEnabledTrigger"])(target, store.context.triggerElements);
        }
    }["useHoverReferenceInteraction.useStableCallback[isRelatedTargetInsideEnabledTrigger]"]);
    const closeWithDelay = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useCallback"]({
        "useHoverReferenceInteraction.useCallback[closeWithDelay]": (event, runElseBranch = true)=>{
            const closeDelay = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$hooks$2f$useHover$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["getDelay"])(delayRef.current, 'close', instance.pointerType);
            if (closeDelay && !instance.handler) {
                instance.openChangeTimeout.start(closeDelay, {
                    "useHoverReferenceInteraction.useCallback[closeWithDelay]": ()=>store.setOpen(false, (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$createBaseUIEventDetails$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["createChangeEventDetails"])(__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$reason$2d$parts$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$export__$2a$__as__REASONS$3e$__["REASONS"].triggerHover, event))
                }["useHoverReferenceInteraction.useCallback[closeWithDelay]"]);
            } else if (runElseBranch) {
                instance.openChangeTimeout.clear();
                store.setOpen(false, (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$createBaseUIEventDetails$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["createChangeEventDetails"])(__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$reason$2d$parts$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$export__$2a$__as__REASONS$3e$__["REASONS"].triggerHover, event));
            }
        }
    }["useHoverReferenceInteraction.useCallback[closeWithDelay]"], [
        delayRef,
        store,
        instance
    ]);
    const cleanupMouseMoveHandler = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useStableCallback$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useStableCallback"])({
        "useHoverReferenceInteraction.useStableCallback[cleanupMouseMoveHandler]": ()=>{
            instance.unbindMouseMove();
            instance.handler = undefined;
        }
    }["useHoverReferenceInteraction.useStableCallback[cleanupMouseMoveHandler]"]);
    const clearPointerEvents = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useStableCallback$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useStableCallback"])({
        "useHoverReferenceInteraction.useStableCallback[clearPointerEvents]": ()=>{
            if (instance.performedPointerEventsMutation) {
                const body = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$owner$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$locals$3e$__["ownerDocument"])(store.select('domReferenceElement')).body;
                body.style.pointerEvents = '';
                body.removeAttribute(__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$hooks$2f$useHoverInteractionSharedState$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["safePolygonIdentifier"]);
                instance.performedPointerEventsMutation = false;
            }
        }
    }["useHoverReferenceInteraction.useStableCallback[clearPointerEvents]"]);
    // When closing before opening, clear the delay timeouts to cancel it
    // from showing.
    __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useEffect"]({
        "useHoverReferenceInteraction.useEffect": ()=>{
            if (!enabled) {
                return undefined;
            }
            function onOpenChangeLocal(details) {
                if (!details.open) {
                    instance.openChangeTimeout.clear();
                    instance.restTimeout.clear();
                    instance.blockMouseMove = true;
                    instance.restTimeoutPending = false;
                }
            }
            events.on('openchange', onOpenChangeLocal);
            return ({
                "useHoverReferenceInteraction.useEffect": ()=>{
                    events.off('openchange', onOpenChangeLocal);
                }
            })["useHoverReferenceInteraction.useEffect"];
        }
    }["useHoverReferenceInteraction.useEffect"], [
        enabled,
        events,
        instance
    ]);
    const handleScrollMouseLeave = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useStableCallback$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useStableCallback"])({
        "useHoverReferenceInteraction.useStableCallback[handleScrollMouseLeave]": (event)=>{
            if (isClickLikeOpenEvent()) {
                return;
            }
            if (!dataRef.current.floatingContext) {
                return;
            }
            if (isRelatedTargetInsideEnabledTrigger(event.relatedTarget)) {
                return;
            }
            const currentTrigger = triggerElementRef.current;
            handleCloseRef.current?.({
                ...dataRef.current.floatingContext,
                tree,
                x: event.clientX,
                y: event.clientY,
                onClose () {
                    clearPointerEvents();
                    cleanupMouseMoveHandler();
                    if (!isClickLikeOpenEvent() && currentTrigger === store.select('domReferenceElement')) {
                        closeWithDelay(event);
                    }
                }
            })(event);
        }
    }["useHoverReferenceInteraction.useStableCallback[handleScrollMouseLeave]"]);
    __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useEffect"]({
        "useHoverReferenceInteraction.useEffect": ()=>{
            if (!enabled) {
                return undefined;
            }
            const trigger = triggerElementRef.current ?? (isActiveTrigger ? store.select('domReferenceElement') : null);
            if (!(0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$floating$2d$ui$2b$utils$40$0$2e$2$2e$11$2f$node_modules$2f40$floating$2d$ui$2f$utils$2f$dist$2f$floating$2d$ui$2e$utils$2e$dom$2e$mjs__$5b$app$2d$client$5d$__$28$ecmascript$29$__["isElement"])(trigger)) {
                return undefined;
            }
            function onMouseEnter(event) {
                instance.openChangeTimeout.clear();
                instance.blockMouseMove = false;
                if (mouseOnly && !(0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$event$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["isMouseLikePointerType"])(instance.pointerType)) {
                    return;
                }
                // Only rest delay is set; there's no fallback delay.
                // This will be handled by `onMouseMove`.
                if (getRestMs(restMsRef.current) > 0 && !(0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$hooks$2f$useHover$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["getDelay"])(delayRef.current, 'open')) {
                    return;
                }
                const openDelay = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$hooks$2f$useHover$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["getDelay"])(delayRef.current, 'open', instance.pointerType);
                const currentDomReference = store.select('domReferenceElement');
                const allTriggers = store.context.triggerElements;
                const isOverInactiveTrigger = (allTriggers.hasElement(event.target) || allTriggers.hasMatchingElement({
                    "useHoverReferenceInteraction.useEffect.onMouseEnter": (t)=>(0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$element$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["contains"])(t, event.target)
                }["useHoverReferenceInteraction.useEffect.onMouseEnter"])) && (!currentDomReference || !(0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$element$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["contains"])(currentDomReference, event.target));
                const triggerNode = event.currentTarget ?? null;
                const isOpen = store.select('open');
                const shouldOpen = !isOpen || isOverInactiveTrigger;
                // When moving between triggers while already open, open immediately without delay
                if (isOverInactiveTrigger && isOpen) {
                    store.setOpen(true, (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$createBaseUIEventDetails$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["createChangeEventDetails"])(__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$reason$2d$parts$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$export__$2a$__as__REASONS$3e$__["REASONS"].triggerHover, event, triggerNode));
                } else if (openDelay) {
                    instance.openChangeTimeout.start(openDelay, {
                        "useHoverReferenceInteraction.useEffect.onMouseEnter": ()=>{
                            if (shouldOpen) {
                                store.setOpen(true, (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$createBaseUIEventDetails$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["createChangeEventDetails"])(__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$reason$2d$parts$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$export__$2a$__as__REASONS$3e$__["REASONS"].triggerHover, event, triggerNode));
                            }
                        }
                    }["useHoverReferenceInteraction.useEffect.onMouseEnter"]);
                } else if (shouldOpen) {
                    store.setOpen(true, (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$createBaseUIEventDetails$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["createChangeEventDetails"])(__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$reason$2d$parts$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$export__$2a$__as__REASONS$3e$__["REASONS"].triggerHover, event, triggerNode));
                }
            }
            function onMouseLeave(event) {
                if (isClickLikeOpenEvent()) {
                    clearPointerEvents();
                    return;
                }
                instance.unbindMouseMove();
                const domReferenceElement = store.select('domReferenceElement');
                const doc = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$owner$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$locals$3e$__["ownerDocument"])(domReferenceElement);
                instance.restTimeout.clear();
                instance.restTimeoutPending = false;
                if (isRelatedTargetInsideEnabledTrigger(event.relatedTarget)) {
                    return;
                }
                if (handleCloseRef.current && dataRef.current.floatingContext) {
                    if (!store.select('open')) {
                        instance.openChangeTimeout.clear();
                    }
                    const currentTrigger = triggerElementRef.current;
                    instance.handler = handleCloseRef.current({
                        ...dataRef.current.floatingContext,
                        tree,
                        x: event.clientX,
                        y: event.clientY,
                        onClose () {
                            clearPointerEvents();
                            cleanupMouseMoveHandler();
                            if (enabledRef.current && !isClickLikeOpenEvent() && currentTrigger === store.select('domReferenceElement')) {
                                closeWithDelay(event, true);
                            }
                        }
                    });
                    const handler = instance.handler;
                    handler(event);
                    doc.addEventListener('mousemove', handler);
                    instance.unbindMouseMove = ({
                        "useHoverReferenceInteraction.useEffect.onMouseLeave": ()=>{
                            doc.removeEventListener('mousemove', handler);
                        }
                    })["useHoverReferenceInteraction.useEffect.onMouseLeave"];
                    return;
                }
                const shouldClose = instance.pointerType === 'touch' ? !(0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$element$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["contains"])(store.select('floatingElement'), event.relatedTarget) : true;
                if (shouldClose) {
                    closeWithDelay(event);
                }
            }
            function onScrollMouseLeave(event) {
                handleScrollMouseLeave(event);
            }
            if (store.select('open')) {
                trigger.addEventListener('mouseleave', onScrollMouseLeave);
            }
            if (move) {
                trigger.addEventListener('mousemove', onMouseEnter, {
                    once: true
                });
            }
            trigger.addEventListener('mouseenter', onMouseEnter);
            trigger.addEventListener('mouseleave', onMouseLeave);
            return ({
                "useHoverReferenceInteraction.useEffect": ()=>{
                    trigger.removeEventListener('mouseleave', onScrollMouseLeave);
                    if (move) {
                        trigger.removeEventListener('mousemove', onMouseEnter);
                    }
                    trigger.removeEventListener('mouseenter', onMouseEnter);
                    trigger.removeEventListener('mouseleave', onMouseLeave);
                }
            })["useHoverReferenceInteraction.useEffect"];
        }
    }["useHoverReferenceInteraction.useEffect"], [
        cleanupMouseMoveHandler,
        clearPointerEvents,
        dataRef,
        delayRef,
        closeWithDelay,
        store,
        enabled,
        handleCloseRef,
        handleScrollMouseLeave,
        instance,
        isActiveTrigger,
        isClickLikeOpenEvent,
        isRelatedTargetInsideEnabledTrigger,
        mouseOnly,
        move,
        restMsRef,
        triggerElementRef,
        tree,
        enabledRef
    ]);
    return __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useMemo"]({
        "useHoverReferenceInteraction.useMemo": ()=>{
            if (!enabled) {
                return undefined;
            }
            function setPointerRef(event) {
                instance.pointerType = event.pointerType;
            }
            return {
                onPointerDown: setPointerRef,
                onPointerEnter: setPointerRef,
                onMouseMove (event) {
                    const { nativeEvent } = event;
                    const trigger = event.currentTarget;
                    const currentDomReference = store.select('domReferenceElement');
                    const allTriggers = store.context.triggerElements;
                    const currentOpen = store.select('open');
                    const isOverInactiveTrigger = (allTriggers.hasElement(event.target) || allTriggers.hasMatchingElement({
                        "useHoverReferenceInteraction.useMemo": (t)=>(0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$element$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["contains"])(t, event.target)
                    }["useHoverReferenceInteraction.useMemo"])) && (!currentDomReference || !(0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$element$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["contains"])(currentDomReference, event.target));
                    if (mouseOnly && !(0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$event$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["isMouseLikePointerType"])(instance.pointerType)) {
                        return;
                    }
                    if (currentOpen && !isOverInactiveTrigger || getRestMs(restMsRef.current) === 0) {
                        return;
                    }
                    if (!isOverInactiveTrigger && instance.restTimeoutPending && event.movementX ** 2 + event.movementY ** 2 < 2) {
                        return;
                    }
                    instance.restTimeout.clear();
                    function handleMouseMove() {
                        instance.restTimeoutPending = false;
                        // A delayed hover open should not override a click-like open that happened
                        // while the hover delay was pending.
                        if (isClickLikeOpenEvent()) {
                            return;
                        }
                        const latestOpen = store.select('open');
                        if (!instance.blockMouseMove && (!latestOpen || isOverInactiveTrigger)) {
                            store.setOpen(true, (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$createBaseUIEventDetails$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["createChangeEventDetails"])(__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$reason$2d$parts$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$export__$2a$__as__REASONS$3e$__["REASONS"].triggerHover, nativeEvent, trigger));
                        }
                    }
                    if (instance.pointerType === 'touch') {
                        __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2d$dom$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["flushSync"]({
                            "useHoverReferenceInteraction.useMemo": ()=>{
                                handleMouseMove();
                            }
                        }["useHoverReferenceInteraction.useMemo"]);
                    } else if (isOverInactiveTrigger && currentOpen) {
                        handleMouseMove();
                    } else {
                        instance.restTimeoutPending = true;
                        instance.restTimeout.start(getRestMs(restMsRef.current), handleMouseMove);
                    }
                }
            };
        }
    }["useHoverReferenceInteraction.useMemo"], [
        enabled,
        instance,
        isClickLikeOpenEvent,
        mouseOnly,
        store,
        restMsRef
    ]);
}
}),
"[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/tooltip/trigger/TooltipTriggerDataAttributes.js [app-client] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "TooltipTriggerDataAttributes",
    ()=>TooltipTriggerDataAttributes
]);
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$popupStateMapping$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/utils/popupStateMapping.js [app-client] (ecmascript)");
;
let TooltipTriggerDataAttributes = function(TooltipTriggerDataAttributes) {
    /**
   * Present when the corresponding tooltip is open.
   */ TooltipTriggerDataAttributes[TooltipTriggerDataAttributes["popupOpen"] = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$popupStateMapping$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["CommonTriggerDataAttributes"].popupOpen] = "popupOpen";
    /**
   * Present when the trigger is disabled, either by the `disabled` prop or by a parent `<Tooltip.Root>` component.
   */ TooltipTriggerDataAttributes["triggerDisabled"] = "data-trigger-disabled";
    return TooltipTriggerDataAttributes;
}({});
}),
"[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/tooltip/utils/constants.js [app-client] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "OPEN_DELAY",
    ()=>OPEN_DELAY
]);
const OPEN_DELAY = 600;
}),
"[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/tooltip/trigger/TooltipTrigger.js [app-client] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "TooltipTrigger",
    ()=>TooltipTrigger
]);
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$build$2f$polyfills$2f$process$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = /*#__PURE__*/ __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/build/polyfills/process.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/compiled/react/index.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$fastHooks$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+utils@0.2.5+0ea9ec2a211d4613/node_modules/@base-ui/utils/esm/fastHooks.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$tooltip$2f$root$2f$TooltipRootContext$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/tooltip/root/TooltipRootContext.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$popupStateMapping$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/utils/popupStateMapping.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$useRenderElement$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/utils/useRenderElement.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$popups$2f$popupStoreUtils$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/utils/popups/popupStoreUtils.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$useBaseUiId$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/utils/useBaseUiId.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$tooltip$2f$provider$2f$TooltipProviderContext$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/tooltip/provider/TooltipProviderContext.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$safePolygon$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/floating-ui-react/safePolygon.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$components$2f$FloatingDelayGroup$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/floating-ui-react/components/FloatingDelayGroup.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$hooks$2f$useFocus$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/floating-ui-react/hooks/useFocus.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$hooks$2f$useHoverReferenceInteraction$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/floating-ui-react/hooks/useHoverReferenceInteraction.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$tooltip$2f$trigger$2f$TooltipTriggerDataAttributes$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/tooltip/trigger/TooltipTriggerDataAttributes.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$tooltip$2f$utils$2f$constants$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/tooltip/utils/constants.js [app-client] (ecmascript)");
'use client';
;
;
;
;
;
;
;
;
;
;
;
;
const TooltipTrigger = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$fastHooks$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["fastComponentRef"])(function TooltipTrigger(componentProps, forwardedRef) {
    const { className, render, handle, payload, disabled: disabledProp, delay, closeDelay, id: idProp, ...elementProps } = componentProps;
    const rootContext = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$tooltip$2f$root$2f$TooltipRootContext$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useTooltipRootContext"])(true);
    const store = handle?.store ?? rootContext;
    if (!store) {
        throw new Error(("TURBOPACK compile-time truthy", 1) ? 'Base UI: <Tooltip.Trigger> must be either used within a <Tooltip.Root> component or provided with a handle.' : "TURBOPACK unreachable");
    }
    const thisTriggerId = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$useBaseUiId$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useBaseUiId"])(idProp);
    const isTriggerActive = store.useState('isTriggerActive', thisTriggerId);
    const isOpenedByThisTrigger = store.useState('isOpenedByTrigger', thisTriggerId);
    const floatingRootContext = store.useState('floatingRootContext');
    const triggerElementRef = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useRef"](null);
    const delayWithDefault = delay ?? __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$tooltip$2f$utils$2f$constants$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["OPEN_DELAY"];
    const closeDelayWithDefault = closeDelay ?? 0;
    const { registerTrigger, isMountedByThisTrigger } = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$popups$2f$popupStoreUtils$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useTriggerDataForwarding"])(thisTriggerId, triggerElementRef, store, {
        payload,
        closeDelay: closeDelayWithDefault
    });
    const providerContext = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$tooltip$2f$provider$2f$TooltipProviderContext$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useTooltipProviderContext"])();
    const { delayRef, isInstantPhase, hasProvider } = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$components$2f$FloatingDelayGroup$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useDelayGroup"])(floatingRootContext, {
        open: isOpenedByThisTrigger
    });
    store.useSyncedValue('isInstantPhase', isInstantPhase);
    const rootDisabled = store.useState('disabled');
    const disabled = disabledProp ?? rootDisabled;
    const trackCursorAxis = store.useState('trackCursorAxis');
    const disableHoverablePopup = store.useState('disableHoverablePopup');
    const hoverProps = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$hooks$2f$useHoverReferenceInteraction$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useHoverReferenceInteraction"])(floatingRootContext, {
        enabled: !disabled,
        mouseOnly: true,
        move: false,
        handleClose: !disableHoverablePopup && trackCursorAxis !== 'both' ? (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$safePolygon$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["safePolygon"])() : null,
        restMs () {
            const providerDelay = providerContext?.delay;
            const groupOpenValue = typeof delayRef.current === 'object' ? delayRef.current.open : undefined;
            let computedRestMs = delayWithDefault;
            if (hasProvider) {
                if (groupOpenValue !== 0) {
                    computedRestMs = delay ?? providerDelay ?? delayWithDefault;
                } else {
                    computedRestMs = 0;
                }
            }
            return computedRestMs;
        },
        delay () {
            const closeValue = typeof delayRef.current === 'object' ? delayRef.current.close : undefined;
            let computedCloseDelay = closeDelayWithDefault;
            if (closeDelay == null && hasProvider) {
                computedCloseDelay = closeValue;
            }
            return {
                close: computedCloseDelay
            };
        },
        triggerElementRef,
        isActiveTrigger: isTriggerActive
    });
    const focusProps = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$hooks$2f$useFocus$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useFocus"])(floatingRootContext, {
        enabled: !disabled
    }).reference;
    const state = {
        open: isOpenedByThisTrigger
    };
    const rootTriggerProps = store.useState('triggerProps', isMountedByThisTrigger);
    const element = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$useRenderElement$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useRenderElement"])('button', componentProps, {
        state,
        ref: [
            forwardedRef,
            registerTrigger,
            triggerElementRef
        ],
        props: [
            hoverProps,
            focusProps,
            rootTriggerProps,
            {
                id: thisTriggerId,
                [__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$tooltip$2f$trigger$2f$TooltipTriggerDataAttributes$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["TooltipTriggerDataAttributes"].triggerDisabled]: disabled ? '' : undefined
            },
            elementProps
        ],
        stateAttributesMapping: __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$popupStateMapping$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["triggerOpenStateMapping"]
    });
    return element;
});
if ("TURBOPACK compile-time truthy", 1) TooltipTrigger.displayName = "TooltipTrigger";
}),
"[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/tooltip/portal/TooltipPortalContext.js [app-client] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "TooltipPortalContext",
    ()=>TooltipPortalContext,
    "useTooltipPortalContext",
    ()=>useTooltipPortalContext
]);
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$build$2f$polyfills$2f$process$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = /*#__PURE__*/ __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/build/polyfills/process.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/compiled/react/index.js [app-client] (ecmascript)");
'use client';
;
;
const TooltipPortalContext = /*#__PURE__*/ __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["createContext"](undefined);
if ("TURBOPACK compile-time truthy", 1) TooltipPortalContext.displayName = "TooltipPortalContext";
function useTooltipPortalContext() {
    const value = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useContext"](TooltipPortalContext);
    if (value === undefined) {
        throw new Error(("TURBOPACK compile-time truthy", 1) ? 'Base UI: <Tooltip.Portal> is missing.' : "TURBOPACK unreachable");
    }
    return value;
}
}),
"[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/utils/FocusGuard.js [app-client] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "FocusGuard",
    ()=>FocusGuard
]);
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$build$2f$polyfills$2f$process$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = /*#__PURE__*/ __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/build/polyfills/process.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/compiled/react/index.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useIsoLayoutEffect$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+utils@0.2.5+0ea9ec2a211d4613/node_modules/@base-ui/utils/esm/useIsoLayoutEffect.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$detectBrowser$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+utils@0.2.5+0ea9ec2a211d4613/node_modules/@base-ui/utils/esm/detectBrowser.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$visuallyHidden$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+utils@0.2.5+0ea9ec2a211d4613/node_modules/@base-ui/utils/esm/visuallyHidden.js [app-client] (ecmascript)");
/**
 * @internal
 */ var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/compiled/react/jsx-runtime.js [app-client] (ecmascript)");
'use client';
;
;
;
;
;
const FocusGuard = /*#__PURE__*/ __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["forwardRef"](function FocusGuard(props, ref) {
    const [role, setRole] = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useState"]();
    (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useIsoLayoutEffect$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useIsoLayoutEffect"])({
        "FocusGuard.FocusGuard.useIsoLayoutEffect": ()=>{
            if (__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$detectBrowser$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["isSafari"]) {
                // Unlike other screen readers such as NVDA and JAWS, the virtual cursor
                // on VoiceOver does trigger the onFocus event, so we can use the focus
                // trap element. On Safari, only buttons trigger the onFocus event.
                setRole('button');
            }
        }
    }["FocusGuard.FocusGuard.useIsoLayoutEffect"], []);
    const restProps = {
        tabIndex: 0,
        // Role is only for VoiceOver
        role
    };
    return /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsx"])("span", {
        ...props,
        ref: ref,
        style: __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$visuallyHidden$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["visuallyHidden"],
        "aria-hidden": role ? undefined : true,
        ...restProps,
        "data-base-ui-focus-guard": ""
    });
});
if ("TURBOPACK compile-time truthy", 1) FocusGuard.displayName = "FocusGuard";
}),
"[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/floating-ui-react/utils/tabbable.js [app-client] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "disableFocusInside",
    ()=>disableFocusInside,
    "enableFocusInside",
    ()=>enableFocusInside,
    "getNextTabbable",
    ()=>getNextTabbable,
    "getPreviousTabbable",
    ()=>getPreviousTabbable,
    "getTabbableAfterElement",
    ()=>getTabbableAfterElement,
    "getTabbableBeforeElement",
    ()=>getTabbableBeforeElement,
    "getTabbableOptions",
    ()=>getTabbableOptions,
    "isOutsideEvent",
    ()=>isOutsideEvent
]);
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$tabbable$40$6$2e$4$2e$0$2f$node_modules$2f$tabbable$2f$dist$2f$index$2e$esm$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/tabbable@6.4.0/node_modules/tabbable/dist/index.esm.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$owner$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$locals$3e$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+utils@0.2.5+0ea9ec2a211d4613/node_modules/@base-ui/utils/esm/owner.js [app-client] (ecmascript) <locals>");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$element$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/floating-ui-react/utils/element.js [app-client] (ecmascript)");
;
;
;
const getTabbableOptions = ()=>({
        getShadowRoot: true,
        displayCheck: // JSDOM does not support the `tabbable` library. To solve this we can
        // check if `ResizeObserver` is a real function (not polyfilled), which
        // determines if the current environment is JSDOM-like.
        typeof ResizeObserver === 'function' && ResizeObserver.toString().includes('[native code]') ? 'full' : 'none'
    });
function getTabbableIn(container, dir) {
    const list = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$tabbable$40$6$2e$4$2e$0$2f$node_modules$2f$tabbable$2f$dist$2f$index$2e$esm$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["tabbable"])(container, getTabbableOptions());
    const len = list.length;
    if (len === 0) {
        return undefined;
    }
    const active = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$element$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["activeElement"])((0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$owner$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$locals$3e$__["ownerDocument"])(container));
    const index = list.indexOf(active);
    // eslint-disable-next-line no-nested-ternary
    const nextIndex = index === -1 ? dir === 1 ? 0 : len - 1 : index + dir;
    return list[nextIndex];
}
function getNextTabbable(referenceElement) {
    return getTabbableIn((0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$owner$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$locals$3e$__["ownerDocument"])(referenceElement).body, 1) || referenceElement;
}
function getPreviousTabbable(referenceElement) {
    return getTabbableIn((0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$owner$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$locals$3e$__["ownerDocument"])(referenceElement).body, -1) || referenceElement;
}
function getTabbableNearElement(referenceElement, dir) {
    if (!referenceElement) {
        return null;
    }
    const list = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$tabbable$40$6$2e$4$2e$0$2f$node_modules$2f$tabbable$2f$dist$2f$index$2e$esm$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["tabbable"])((0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$owner$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$locals$3e$__["ownerDocument"])(referenceElement).body, getTabbableOptions());
    const elementCount = list.length;
    if (elementCount === 0) {
        return null;
    }
    const index = list.indexOf(referenceElement);
    if (index === -1) {
        return null;
    }
    const nextIndex = (index + dir + elementCount) % elementCount;
    return list[nextIndex];
}
function getTabbableAfterElement(referenceElement) {
    return getTabbableNearElement(referenceElement, 1);
}
function getTabbableBeforeElement(referenceElement) {
    return getTabbableNearElement(referenceElement, -1);
}
function isOutsideEvent(event, container) {
    const containerElement = container || event.currentTarget;
    const relatedTarget = event.relatedTarget;
    return !relatedTarget || !(0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$element$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["contains"])(containerElement, relatedTarget);
}
function disableFocusInside(container) {
    const tabbableElements = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$tabbable$40$6$2e$4$2e$0$2f$node_modules$2f$tabbable$2f$dist$2f$index$2e$esm$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["tabbable"])(container, getTabbableOptions());
    tabbableElements.forEach((element)=>{
        element.dataset.tabindex = element.getAttribute('tabindex') || '';
        element.setAttribute('tabindex', '-1');
    });
}
function enableFocusInside(container) {
    const elements = container.querySelectorAll('[data-tabindex]');
    elements.forEach((element)=>{
        const tabindex = element.dataset.tabindex;
        delete element.dataset.tabindex;
        if (tabindex) {
            element.setAttribute('tabindex', tabindex);
        } else {
            element.removeAttribute('tabindex');
        }
    });
}
}),
"[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/utils/constants.js [app-client] (ecmascript) <locals>", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "CLICK_TRIGGER_IDENTIFIER",
    ()=>CLICK_TRIGGER_IDENTIFIER,
    "DISABLED_TRANSITIONS_STYLE",
    ()=>DISABLED_TRANSITIONS_STYLE,
    "DROPDOWN_COLLISION_AVOIDANCE",
    ()=>DROPDOWN_COLLISION_AVOIDANCE,
    "PATIENT_CLICK_THRESHOLD",
    ()=>PATIENT_CLICK_THRESHOLD,
    "POPUP_COLLISION_AVOIDANCE",
    ()=>POPUP_COLLISION_AVOIDANCE,
    "TYPEAHEAD_RESET_MS",
    ()=>TYPEAHEAD_RESET_MS,
    "ownerVisuallyHidden",
    ()=>ownerVisuallyHidden
]);
const TYPEAHEAD_RESET_MS = 500;
const PATIENT_CLICK_THRESHOLD = 500;
const DISABLED_TRANSITIONS_STYLE = {
    style: {
        transition: 'none'
    }
};
;
const CLICK_TRIGGER_IDENTIFIER = 'data-base-ui-click-trigger';
const DROPDOWN_COLLISION_AVOIDANCE = {
    fallbackAxisSide: 'none'
};
const POPUP_COLLISION_AVOIDANCE = {
    fallbackAxisSide: 'end'
};
const ownerVisuallyHidden = {
    clipPath: 'inset(50%)',
    position: 'fixed',
    top: 0,
    left: 0
};
}),
"[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/floating-ui-react/components/FloatingPortal.js [app-client] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "FloatingPortal",
    ()=>FloatingPortal,
    "useFloatingPortalNode",
    ()=>useFloatingPortalNode,
    "usePortalContext",
    ()=>usePortalContext
]);
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$build$2f$polyfills$2f$process$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = /*#__PURE__*/ __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/build/polyfills/process.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/compiled/react/index.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2d$dom$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/compiled/react-dom/index.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$floating$2d$ui$2b$utils$40$0$2e$2$2e$11$2f$node_modules$2f40$floating$2d$ui$2f$utils$2f$dist$2f$floating$2d$ui$2e$utils$2e$dom$2e$mjs__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@floating-ui+utils@0.2.11/node_modules/@floating-ui/utils/dist/floating-ui.utils.dom.mjs [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useId$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+utils@0.2.5+0ea9ec2a211d4613/node_modules/@base-ui/utils/esm/useId.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useIsoLayoutEffect$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+utils@0.2.5+0ea9ec2a211d4613/node_modules/@base-ui/utils/esm/useIsoLayoutEffect.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useStableCallback$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+utils@0.2.5+0ea9ec2a211d4613/node_modules/@base-ui/utils/esm/useStableCallback.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$FocusGuard$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/utils/FocusGuard.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$tabbable$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/floating-ui-react/utils/tabbable.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$createBaseUIEventDetails$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/utils/createBaseUIEventDetails.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$reason$2d$parts$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$export__$2a$__as__REASONS$3e$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/utils/reason-parts.js [app-client] (ecmascript) <export * as REASONS>");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$createAttribute$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/floating-ui-react/utils/createAttribute.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$useRenderElement$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/utils/useRenderElement.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$empty$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+utils@0.2.5+0ea9ec2a211d4613/node_modules/@base-ui/utils/esm/empty.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$constants$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$locals$3e$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/utils/constants.js [app-client] (ecmascript) <locals>");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/compiled/react/jsx-runtime.js [app-client] (ecmascript)");
'use client';
;
;
;
;
;
;
;
;
;
;
;
;
;
;
const PortalContext = /*#__PURE__*/ __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["createContext"](null);
if ("TURBOPACK compile-time truthy", 1) PortalContext.displayName = "PortalContext";
const usePortalContext = ()=>__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useContext"](PortalContext);
const attr = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$createAttribute$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["createAttribute"])('portal');
function useFloatingPortalNode(props = {}) {
    const { ref, container: containerProp, componentProps = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$empty$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["EMPTY_OBJECT"], elementProps } = props;
    const uniqueId = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useId$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useId"])();
    const portalContext = usePortalContext();
    const parentPortalNode = portalContext?.portalNode;
    const [containerElement, setContainerElement] = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useState"](null);
    const [portalNode, setPortalNode] = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useState"](null);
    const setPortalNodeRef = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useStableCallback$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useStableCallback"])({
        "useFloatingPortalNode.useStableCallback[setPortalNodeRef]": (node)=>{
            if (node !== null) {
                // the useIsoLayoutEffect below watching containerProp / parentPortalNode
                // sets setPortalNode(null) when the container becomes null or changes.
                // So even though the ref callback now ignores null, the portal node still gets cleared.
                setPortalNode(node);
            }
        }
    }["useFloatingPortalNode.useStableCallback[setPortalNodeRef]"]);
    const containerRef = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useRef"](null);
    (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useIsoLayoutEffect$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useIsoLayoutEffect"])({
        "useFloatingPortalNode.useIsoLayoutEffect": ()=>{
            // Wait for the container to be resolved if explicitly `null`.
            if (containerProp === null) {
                if (containerRef.current) {
                    containerRef.current = null;
                    setPortalNode(null);
                    setContainerElement(null);
                }
                return;
            }
            // React 17 does not use React.useId().
            if (uniqueId == null) {
                return;
            }
            const resolvedContainer = (containerProp && ((0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$floating$2d$ui$2b$utils$40$0$2e$2$2e$11$2f$node_modules$2f40$floating$2d$ui$2f$utils$2f$dist$2f$floating$2d$ui$2e$utils$2e$dom$2e$mjs__$5b$app$2d$client$5d$__$28$ecmascript$29$__["isNode"])(containerProp) ? containerProp : containerProp.current)) ?? parentPortalNode ?? document.body;
            if (resolvedContainer == null) {
                if (containerRef.current) {
                    containerRef.current = null;
                    setPortalNode(null);
                    setContainerElement(null);
                }
                return;
            }
            if (containerRef.current !== resolvedContainer) {
                containerRef.current = resolvedContainer;
                setPortalNode(null);
                setContainerElement(resolvedContainer);
            }
        }
    }["useFloatingPortalNode.useIsoLayoutEffect"], [
        containerProp,
        parentPortalNode,
        uniqueId
    ]);
    const portalElement = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$useRenderElement$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useRenderElement"])('div', componentProps, {
        ref: [
            ref,
            setPortalNodeRef
        ],
        props: [
            {
                id: uniqueId,
                [attr]: ''
            },
            elementProps
        ]
    });
    // This `createPortal` call injects `portalElement` into the `container`.
    // Another call inside `FloatingPortal`/`FloatingPortalLite` then injects the children into `portalElement`.
    const portalSubtree = containerElement && portalElement ? /*#__PURE__*/ __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2d$dom$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["createPortal"](portalElement, containerElement) : null;
    return {
        portalNode,
        portalSubtree
    };
}
const FloatingPortal = /*#__PURE__*/ __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["forwardRef"](function FloatingPortal(componentProps, forwardedRef) {
    const { children, container, className, render, renderGuards, ...elementProps } = componentProps;
    const { portalNode, portalSubtree } = useFloatingPortalNode({
        container,
        ref: forwardedRef,
        componentProps,
        elementProps
    });
    const beforeOutsideRef = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useRef"](null);
    const afterOutsideRef = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useRef"](null);
    const beforeInsideRef = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useRef"](null);
    const afterInsideRef = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useRef"](null);
    const [focusManagerState, setFocusManagerState] = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useState"](null);
    const modal = focusManagerState?.modal;
    const open = focusManagerState?.open;
    const shouldRenderGuards = typeof renderGuards === 'boolean' ? renderGuards : !!focusManagerState && !focusManagerState.modal && focusManagerState.open && !!portalNode;
    // https://codesandbox.io/s/tabbable-portal-f4tng?file=/src/TabbablePortal.tsx
    __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useEffect"]({
        "FloatingPortal.FloatingPortal.useEffect": ()=>{
            if (!portalNode || modal) {
                return undefined;
            }
            // Make sure elements inside the portal element are tabbable only when the
            // portal has already been focused, either by tabbing into a focus trap
            // element outside or using the mouse.
            function onFocus(event) {
                if (portalNode && event.relatedTarget && (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$tabbable$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["isOutsideEvent"])(event)) {
                    const focusing = event.type === 'focusin';
                    const manageFocus = focusing ? __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$tabbable$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["enableFocusInside"] : __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$tabbable$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["disableFocusInside"];
                    manageFocus(portalNode);
                }
            }
            // Listen to the event on the capture phase so they run before the focus
            // trap elements onFocus prop is called.
            portalNode.addEventListener('focusin', onFocus, true);
            portalNode.addEventListener('focusout', onFocus, true);
            return ({
                "FloatingPortal.FloatingPortal.useEffect": ()=>{
                    portalNode.removeEventListener('focusin', onFocus, true);
                    portalNode.removeEventListener('focusout', onFocus, true);
                }
            })["FloatingPortal.FloatingPortal.useEffect"];
        }
    }["FloatingPortal.FloatingPortal.useEffect"], [
        portalNode,
        modal
    ]);
    __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useEffect"]({
        "FloatingPortal.FloatingPortal.useEffect": ()=>{
            if (!portalNode || open) {
                return;
            }
            (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$tabbable$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["enableFocusInside"])(portalNode);
        }
    }["FloatingPortal.FloatingPortal.useEffect"], [
        open,
        portalNode
    ]);
    const portalContextValue = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useMemo"]({
        "FloatingPortal.FloatingPortal.useMemo[portalContextValue]": ()=>({
                beforeOutsideRef,
                afterOutsideRef,
                beforeInsideRef,
                afterInsideRef,
                portalNode,
                setFocusManagerState
            })
    }["FloatingPortal.FloatingPortal.useMemo[portalContextValue]"], [
        portalNode
    ]);
    return /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxs"])(__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["Fragment"], {
        children: [
            portalSubtree,
            /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxs"])(PortalContext.Provider, {
                value: portalContextValue,
                children: [
                    shouldRenderGuards && portalNode && /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsx"])(__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$FocusGuard$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["FocusGuard"], {
                        "data-type": "outside",
                        ref: beforeOutsideRef,
                        onFocus: (event)=>{
                            if ((0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$tabbable$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["isOutsideEvent"])(event, portalNode)) {
                                beforeInsideRef.current?.focus();
                            } else {
                                const domReference = focusManagerState ? focusManagerState.domReference : null;
                                const prevTabbable = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$tabbable$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["getPreviousTabbable"])(domReference);
                                prevTabbable?.focus();
                            }
                        }
                    }),
                    shouldRenderGuards && portalNode && /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsx"])("span", {
                        "aria-owns": portalNode.id,
                        style: __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$constants$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$locals$3e$__["ownerVisuallyHidden"]
                    }),
                    portalNode && /*#__PURE__*/ __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2d$dom$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["createPortal"](children, portalNode),
                    shouldRenderGuards && portalNode && /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsx"])(__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$FocusGuard$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["FocusGuard"], {
                        "data-type": "outside",
                        ref: afterOutsideRef,
                        onFocus: (event)=>{
                            if ((0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$tabbable$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["isOutsideEvent"])(event, portalNode)) {
                                afterInsideRef.current?.focus();
                            } else {
                                const domReference = focusManagerState ? focusManagerState.domReference : null;
                                const nextTabbable = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$tabbable$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["getNextTabbable"])(domReference);
                                nextTabbable?.focus();
                                if (focusManagerState?.closeOnFocusOut) {
                                    focusManagerState?.onOpenChange(false, (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$createBaseUIEventDetails$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["createChangeEventDetails"])(__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$reason$2d$parts$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$export__$2a$__as__REASONS$3e$__["REASONS"].focusOut, event.nativeEvent));
                                }
                            }
                        }
                    })
                ]
            })
        ]
    });
});
if ("TURBOPACK compile-time truthy", 1) FloatingPortal.displayName = "FloatingPortal";
}),
"[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/utils/FloatingPortalLite.js [app-client] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "FloatingPortalLite",
    ()=>FloatingPortalLite
]);
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$build$2f$polyfills$2f$process$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = /*#__PURE__*/ __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/build/polyfills/process.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/compiled/react/index.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2d$dom$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/compiled/react-dom/index.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$components$2f$FloatingPortal$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/floating-ui-react/components/FloatingPortal.js [app-client] (ecmascript)");
/**
 * `FloatingPortal` includes tabbable logic handling for focus management.
 * For components that don't need tabbable logic, use `FloatingPortalLite`.
 * @internal
 */ var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/compiled/react/jsx-runtime.js [app-client] (ecmascript)");
'use client';
;
;
;
;
const FloatingPortalLite = /*#__PURE__*/ __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["forwardRef"](function FloatingPortalLite(componentProps, forwardedRef) {
    const { children, container, className, render, ...elementProps } = componentProps;
    const { portalNode, portalSubtree } = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$components$2f$FloatingPortal$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useFloatingPortalNode"])({
        container,
        ref: forwardedRef,
        componentProps,
        elementProps
    });
    if (!portalSubtree && !portalNode) {
        return null;
    }
    return /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxs"])(__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["Fragment"], {
        children: [
            portalSubtree,
            portalNode && /*#__PURE__*/ __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2d$dom$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["createPortal"](children, portalNode)
        ]
    });
});
if ("TURBOPACK compile-time truthy", 1) FloatingPortalLite.displayName = "FloatingPortalLite";
}),
"[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/tooltip/portal/TooltipPortal.js [app-client] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "TooltipPortal",
    ()=>TooltipPortal
]);
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$build$2f$polyfills$2f$process$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = /*#__PURE__*/ __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/build/polyfills/process.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/compiled/react/index.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$tooltip$2f$root$2f$TooltipRootContext$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/tooltip/root/TooltipRootContext.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$tooltip$2f$portal$2f$TooltipPortalContext$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/tooltip/portal/TooltipPortalContext.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$FloatingPortalLite$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/utils/FloatingPortalLite.js [app-client] (ecmascript)");
/**
 * A portal element that moves the popup to a different part of the DOM.
 * By default, the portal element is appended to `<body>`.
 * Renders a `<div>` element.
 *
 * Documentation: [Base UI Tooltip](https://base-ui.com/react/components/tooltip)
 */ var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/compiled/react/jsx-runtime.js [app-client] (ecmascript)");
'use client';
;
;
;
;
;
const TooltipPortal = /*#__PURE__*/ __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["forwardRef"](function TooltipPortal(props, forwardedRef) {
    const { keepMounted = false, ...portalProps } = props;
    const store = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$tooltip$2f$root$2f$TooltipRootContext$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useTooltipRootContext"])();
    const mounted = store.useState('mounted');
    const shouldRender = mounted || keepMounted;
    if (!shouldRender) {
        return null;
    }
    return /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsx"])(__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$tooltip$2f$portal$2f$TooltipPortalContext$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["TooltipPortalContext"].Provider, {
        value: keepMounted,
        children: /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsx"])(__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$FloatingPortalLite$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["FloatingPortalLite"], {
            ref: forwardedRef,
            ...portalProps
        })
    });
});
if ("TURBOPACK compile-time truthy", 1) TooltipPortal.displayName = "TooltipPortal";
}),
"[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/tooltip/positioner/TooltipPositionerContext.js [app-client] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "TooltipPositionerContext",
    ()=>TooltipPositionerContext,
    "useTooltipPositionerContext",
    ()=>useTooltipPositionerContext
]);
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$build$2f$polyfills$2f$process$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = /*#__PURE__*/ __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/build/polyfills/process.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/compiled/react/index.js [app-client] (ecmascript)");
'use client';
;
;
const TooltipPositionerContext = /*#__PURE__*/ __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["createContext"](undefined);
if ("TURBOPACK compile-time truthy", 1) TooltipPositionerContext.displayName = "TooltipPositionerContext";
function useTooltipPositionerContext() {
    const context = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useContext"](TooltipPositionerContext);
    if (context === undefined) {
        throw new Error(("TURBOPACK compile-time truthy", 1) ? 'Base UI: TooltipPositionerContext is missing. TooltipPositioner parts must be placed within <Tooltip.Positioner>.' : "TURBOPACK unreachable");
    }
    return context;
}
}),
"[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/floating-ui-react/hooks/useFloatingRootContext.js [app-client] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "useFloatingRootContext",
    ()=>useFloatingRootContext
]);
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$build$2f$polyfills$2f$process$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = /*#__PURE__*/ __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/build/polyfills/process.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$floating$2d$ui$2b$utils$40$0$2e$2$2e$11$2f$node_modules$2f40$floating$2d$ui$2f$utils$2f$dist$2f$floating$2d$ui$2e$utils$2e$dom$2e$mjs__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@floating-ui+utils@0.2.11/node_modules/@floating-ui/utils/dist/floating-ui.utils.dom.mjs [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useId$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+utils@0.2.5+0ea9ec2a211d4613/node_modules/@base-ui/utils/esm/useId.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useRefWithInit$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+utils@0.2.5+0ea9ec2a211d4613/node_modules/@base-ui/utils/esm/useRefWithInit.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useIsoLayoutEffect$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+utils@0.2.5+0ea9ec2a211d4613/node_modules/@base-ui/utils/esm/useIsoLayoutEffect.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$components$2f$FloatingTree$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/floating-ui-react/components/FloatingTree.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$components$2f$FloatingRootStore$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/floating-ui-react/components/FloatingRootStore.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$popups$2f$popupTriggerMap$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/utils/popups/popupTriggerMap.js [app-client] (ecmascript)");
'use client';
;
;
;
;
;
;
;
function useFloatingRootContext(options) {
    const { open = false, onOpenChange, elements = {} } = options;
    const floatingId = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useId$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useId"])();
    const nested = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$components$2f$FloatingTree$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useFloatingParentNodeId"])() != null;
    if ("TURBOPACK compile-time truthy", 1) {
        const optionDomReference = elements.reference;
        if (optionDomReference && !(0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$floating$2d$ui$2b$utils$40$0$2e$2$2e$11$2f$node_modules$2f40$floating$2d$ui$2f$utils$2f$dist$2f$floating$2d$ui$2e$utils$2e$dom$2e$mjs__$5b$app$2d$client$5d$__$28$ecmascript$29$__["isElement"])(optionDomReference)) {
            console.error('Cannot pass a virtual element to the `elements.reference` option,', 'as it must be a real DOM element. Use `context.setPositionReference()`', 'instead.');
        }
    }
    const store = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useRefWithInit$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useRefWithInit"])({
        "useFloatingRootContext.useRefWithInit": ()=>new __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$components$2f$FloatingRootStore$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["FloatingRootStore"]({
                open,
                onOpenChange,
                referenceElement: elements.reference ?? null,
                floatingElement: elements.floating ?? null,
                triggerElements: new __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$popups$2f$popupTriggerMap$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["PopupTriggerMap"](),
                floatingId,
                nested,
                noEmit: false
            })
    }["useFloatingRootContext.useRefWithInit"]).current;
    (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useIsoLayoutEffect$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useIsoLayoutEffect"])({
        "useFloatingRootContext.useIsoLayoutEffect": ()=>{
            const valuesToSync = {
                open,
                floatingId
            };
            // Only sync elements that are defined to avoid overwriting existing ones
            if (elements.reference !== undefined) {
                valuesToSync.referenceElement = elements.reference;
                valuesToSync.domReferenceElement = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$floating$2d$ui$2b$utils$40$0$2e$2$2e$11$2f$node_modules$2f40$floating$2d$ui$2f$utils$2f$dist$2f$floating$2d$ui$2e$utils$2e$dom$2e$mjs__$5b$app$2d$client$5d$__$28$ecmascript$29$__["isElement"])(elements.reference) ? elements.reference : null;
            }
            if (elements.floating !== undefined) {
                valuesToSync.floatingElement = elements.floating;
            }
            store.update(valuesToSync);
        }
    }["useFloatingRootContext.useIsoLayoutEffect"], [
        open,
        floatingId,
        elements.reference,
        elements.floating,
        store
    ]);
    store.context.onOpenChange = onOpenChange;
    store.context.nested = nested;
    store.context.noEmit = false;
    return store;
}
}),
"[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/floating-ui-react/hooks/useFloating.js [app-client] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "useFloating",
    ()=>useFloating
]);
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/compiled/react/index.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$floating$2d$ui$2b$react$2d$dom$40$2$2e$1$2e$8$2b$bf16f8eded5e12ee$2f$node_modules$2f40$floating$2d$ui$2f$react$2d$dom$2f$dist$2f$floating$2d$ui$2e$react$2d$dom$2e$mjs__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$locals$3e$__ = __turbopack_context__.i("[project]/node_modules/.bun/@floating-ui+react-dom@2.1.8+bf16f8eded5e12ee/node_modules/@floating-ui/react-dom/dist/floating-ui.react-dom.mjs [app-client] (ecmascript) <locals>");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$floating$2d$ui$2b$utils$40$0$2e$2$2e$11$2f$node_modules$2f40$floating$2d$ui$2f$utils$2f$dist$2f$floating$2d$ui$2e$utils$2e$dom$2e$mjs__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@floating-ui+utils@0.2.11/node_modules/@floating-ui/utils/dist/floating-ui.utils.dom.mjs [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useIsoLayoutEffect$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+utils@0.2.5+0ea9ec2a211d4613/node_modules/@base-ui/utils/esm/useIsoLayoutEffect.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$components$2f$FloatingTree$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/floating-ui-react/components/FloatingTree.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$hooks$2f$useFloatingRootContext$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/floating-ui-react/hooks/useFloatingRootContext.js [app-client] (ecmascript)");
'use client';
;
;
;
;
;
;
function useFloating(options = {}) {
    const { nodeId, externalTree } = options;
    const internalRootStore = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$hooks$2f$useFloatingRootContext$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useFloatingRootContext"])(options);
    const rootContext = options.rootContext || internalRootStore;
    const rootContextElements = {
        reference: rootContext.useState('referenceElement'),
        floating: rootContext.useState('floatingElement'),
        domReference: rootContext.useState('domReferenceElement')
    };
    const [positionReference, setPositionReferenceRaw] = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useState"](null);
    const domReferenceRef = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useRef"](null);
    const tree = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$components$2f$FloatingTree$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useFloatingTree"])(externalTree);
    (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useIsoLayoutEffect$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useIsoLayoutEffect"])({
        "useFloating.useIsoLayoutEffect": ()=>{
            if (rootContextElements.domReference) {
                domReferenceRef.current = rootContextElements.domReference;
            }
        }
    }["useFloating.useIsoLayoutEffect"], [
        rootContextElements.domReference
    ]);
    const position = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$floating$2d$ui$2b$react$2d$dom$40$2$2e$1$2e$8$2b$bf16f8eded5e12ee$2f$node_modules$2f40$floating$2d$ui$2f$react$2d$dom$2f$dist$2f$floating$2d$ui$2e$react$2d$dom$2e$mjs__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$locals$3e$__["useFloating"])({
        ...options,
        elements: {
            ...rootContextElements,
            ...positionReference && {
                reference: positionReference
            }
        }
    });
    const setPositionReference = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useCallback"]({
        "useFloating.useCallback[setPositionReference]": (node)=>{
            const computedPositionReference = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$floating$2d$ui$2b$utils$40$0$2e$2$2e$11$2f$node_modules$2f40$floating$2d$ui$2f$utils$2f$dist$2f$floating$2d$ui$2e$utils$2e$dom$2e$mjs__$5b$app$2d$client$5d$__$28$ecmascript$29$__["isElement"])(node) ? {
                getBoundingClientRect: ({
                    "useFloating.useCallback[setPositionReference]": ()=>node.getBoundingClientRect()
                })["useFloating.useCallback[setPositionReference]"],
                getClientRects: ({
                    "useFloating.useCallback[setPositionReference]": ()=>node.getClientRects()
                })["useFloating.useCallback[setPositionReference]"],
                contextElement: node
            } : node;
            // Store the positionReference in state if the DOM reference is specified externally via the
            // `elements.reference` option. This ensures that it won't be overridden on future renders.
            setPositionReferenceRaw(computedPositionReference);
            position.refs.setReference(computedPositionReference);
        }
    }["useFloating.useCallback[setPositionReference]"], [
        position.refs
    ]);
    const [localDomReference, setLocalDomReference] = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useState"](null);
    const [localFloatingElement, setLocalFloatingElement] = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useState"](null);
    rootContext.useSyncedValue('referenceElement', localDomReference);
    rootContext.useSyncedValue('domReferenceElement', (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$floating$2d$ui$2b$utils$40$0$2e$2$2e$11$2f$node_modules$2f40$floating$2d$ui$2f$utils$2f$dist$2f$floating$2d$ui$2e$utils$2e$dom$2e$mjs__$5b$app$2d$client$5d$__$28$ecmascript$29$__["isElement"])(localDomReference) ? localDomReference : null);
    rootContext.useSyncedValue('floatingElement', localFloatingElement);
    const setReference = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useCallback"]({
        "useFloating.useCallback[setReference]": (node)=>{
            if ((0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$floating$2d$ui$2b$utils$40$0$2e$2$2e$11$2f$node_modules$2f40$floating$2d$ui$2f$utils$2f$dist$2f$floating$2d$ui$2e$utils$2e$dom$2e$mjs__$5b$app$2d$client$5d$__$28$ecmascript$29$__["isElement"])(node) || node === null) {
                domReferenceRef.current = node;
                setLocalDomReference(node);
            }
            // Backwards-compatibility for passing a virtual element to `reference`
            // after it has set the DOM reference.
            if ((0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$floating$2d$ui$2b$utils$40$0$2e$2$2e$11$2f$node_modules$2f40$floating$2d$ui$2f$utils$2f$dist$2f$floating$2d$ui$2e$utils$2e$dom$2e$mjs__$5b$app$2d$client$5d$__$28$ecmascript$29$__["isElement"])(position.refs.reference.current) || position.refs.reference.current === null || // Don't allow setting virtual elements using the old technique back to
            // `null` to support `positionReference` + an unstable `reference`
            // callback ref.
            node !== null && !(0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$floating$2d$ui$2b$utils$40$0$2e$2$2e$11$2f$node_modules$2f40$floating$2d$ui$2f$utils$2f$dist$2f$floating$2d$ui$2e$utils$2e$dom$2e$mjs__$5b$app$2d$client$5d$__$28$ecmascript$29$__["isElement"])(node)) {
                position.refs.setReference(node);
            }
        }
    }["useFloating.useCallback[setReference]"], [
        position.refs,
        setLocalDomReference
    ]);
    const setFloating = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useCallback"]({
        "useFloating.useCallback[setFloating]": (node)=>{
            setLocalFloatingElement(node);
            position.refs.setFloating(node);
        }
    }["useFloating.useCallback[setFloating]"], [
        position.refs
    ]);
    const refs = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useMemo"]({
        "useFloating.useMemo[refs]": ()=>({
                ...position.refs,
                setReference,
                setFloating,
                setPositionReference,
                domReference: domReferenceRef
            })
    }["useFloating.useMemo[refs]"], [
        position.refs,
        setReference,
        setFloating,
        setPositionReference
    ]);
    const elements = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useMemo"]({
        "useFloating.useMemo[elements]": ()=>({
                ...position.elements,
                domReference: rootContextElements.domReference
            })
    }["useFloating.useMemo[elements]"], [
        position.elements,
        rootContextElements.domReference
    ]);
    const open = rootContext.useState('open');
    const floatingId = rootContext.useState('floatingId');
    const context = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useMemo"]({
        "useFloating.useMemo[context]": ()=>({
                ...position,
                dataRef: rootContext.context.dataRef,
                open,
                onOpenChange: rootContext.setOpen,
                events: rootContext.context.events,
                floatingId,
                refs,
                elements,
                nodeId,
                rootStore: rootContext
            })
    }["useFloating.useMemo[context]"], [
        position,
        refs,
        elements,
        nodeId,
        rootContext,
        open,
        floatingId
    ]);
    (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useIsoLayoutEffect$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useIsoLayoutEffect"])({
        "useFloating.useIsoLayoutEffect": ()=>{
            rootContext.context.dataRef.current.floatingContext = context;
            const node = tree?.nodesRef.current.find({
                "useFloating.useIsoLayoutEffect": (n)=>n.id === nodeId
            }["useFloating.useIsoLayoutEffect"]);
            if (node) {
                node.context = context;
            }
        }
    }["useFloating.useIsoLayoutEffect"]);
    return __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useMemo"]({
        "useFloating.useMemo": ()=>({
                ...position,
                context,
                refs,
                elements,
                rootStore: rootContext
            })
    }["useFloating.useMemo"], [
        position,
        refs,
        elements,
        context,
        rootContext
    ]);
}
}),
"[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/direction-provider/DirectionContext.js [app-client] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "DirectionContext",
    ()=>DirectionContext,
    "useDirection",
    ()=>useDirection
]);
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$build$2f$polyfills$2f$process$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = /*#__PURE__*/ __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/build/polyfills/process.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/compiled/react/index.js [app-client] (ecmascript)");
'use client';
;
const DirectionContext = /*#__PURE__*/ __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["createContext"](undefined);
if ("TURBOPACK compile-time truthy", 1) DirectionContext.displayName = "DirectionContext";
function useDirection() {
    const context = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useContext"](DirectionContext);
    return context?.direction ?? 'ltr';
}
}),
"[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/floating-ui-react/middleware/arrow.js [app-client] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "arrow",
    ()=>arrow,
    "baseArrow",
    ()=>baseArrow
]);
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$floating$2d$ui$2b$utils$40$0$2e$2$2e$11$2f$node_modules$2f40$floating$2d$ui$2f$utils$2f$dist$2f$floating$2d$ui$2e$utils$2e$mjs__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@floating-ui+utils@0.2.11/node_modules/@floating-ui/utils/dist/floating-ui.utils.mjs [app-client] (ecmascript)");
;
const baseArrow = (options)=>({
        name: 'arrow',
        options,
        async fn (state) {
            const { x, y, placement, rects, platform, elements, middlewareData } = state;
            // Since `element` is required, we don't Partial<> the type.
            const { element, padding = 0, offsetParent = 'real' } = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$floating$2d$ui$2b$utils$40$0$2e$2$2e$11$2f$node_modules$2f40$floating$2d$ui$2f$utils$2f$dist$2f$floating$2d$ui$2e$utils$2e$mjs__$5b$app$2d$client$5d$__$28$ecmascript$29$__["evaluate"])(options, state) || {};
            if (element == null) {
                return {};
            }
            const paddingObject = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$floating$2d$ui$2b$utils$40$0$2e$2$2e$11$2f$node_modules$2f40$floating$2d$ui$2f$utils$2f$dist$2f$floating$2d$ui$2e$utils$2e$mjs__$5b$app$2d$client$5d$__$28$ecmascript$29$__["getPaddingObject"])(padding);
            const coords = {
                x,
                y
            };
            const axis = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$floating$2d$ui$2b$utils$40$0$2e$2$2e$11$2f$node_modules$2f40$floating$2d$ui$2f$utils$2f$dist$2f$floating$2d$ui$2e$utils$2e$mjs__$5b$app$2d$client$5d$__$28$ecmascript$29$__["getAlignmentAxis"])(placement);
            const length = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$floating$2d$ui$2b$utils$40$0$2e$2$2e$11$2f$node_modules$2f40$floating$2d$ui$2f$utils$2f$dist$2f$floating$2d$ui$2e$utils$2e$mjs__$5b$app$2d$client$5d$__$28$ecmascript$29$__["getAxisLength"])(axis);
            const arrowDimensions = await platform.getDimensions(element);
            const isYAxis = axis === 'y';
            const minProp = isYAxis ? 'top' : 'left';
            const maxProp = isYAxis ? 'bottom' : 'right';
            const clientProp = isYAxis ? 'clientHeight' : 'clientWidth';
            const endDiff = rects.reference[length] + rects.reference[axis] - coords[axis] - rects.floating[length];
            const startDiff = coords[axis] - rects.reference[axis];
            const arrowOffsetParent = offsetParent === 'real' ? await platform.getOffsetParent?.(element) : elements.floating;
            let clientSize = elements.floating[clientProp] || rects.floating[length];
            // DOM platform can return `window` as the `offsetParent`.
            if (!clientSize || !await platform.isElement?.(arrowOffsetParent)) {
                clientSize = elements.floating[clientProp] || rects.floating[length];
            }
            const centerToReference = endDiff / 2 - startDiff / 2;
            // If the padding is large enough that it causes the arrow to no longer be
            // centered, modify the padding so that it is centered.
            const largestPossiblePadding = clientSize / 2 - arrowDimensions[length] / 2 - 1;
            const minPadding = Math.min(paddingObject[minProp], largestPossiblePadding);
            const maxPadding = Math.min(paddingObject[maxProp], largestPossiblePadding);
            // Make sure the arrow doesn't overflow the floating element if the center
            // point is outside the floating element's bounds.
            const min = minPadding;
            const max = clientSize - arrowDimensions[length] - maxPadding;
            const center = clientSize / 2 - arrowDimensions[length] / 2 + centerToReference;
            const offset = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$floating$2d$ui$2b$utils$40$0$2e$2$2e$11$2f$node_modules$2f40$floating$2d$ui$2f$utils$2f$dist$2f$floating$2d$ui$2e$utils$2e$mjs__$5b$app$2d$client$5d$__$28$ecmascript$29$__["clamp"])(min, center, max);
            // If the reference is small enough that the arrow's padding causes it to
            // to point to nothing for an aligned placement, adjust the offset of the
            // floating element itself. To ensure `shift()` continues to take action,
            // a single reset is performed when this is true.
            const shouldAddOffset = !middlewareData.arrow && (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$floating$2d$ui$2b$utils$40$0$2e$2$2e$11$2f$node_modules$2f40$floating$2d$ui$2f$utils$2f$dist$2f$floating$2d$ui$2e$utils$2e$mjs__$5b$app$2d$client$5d$__$28$ecmascript$29$__["getAlignment"])(placement) != null && center !== offset && rects.reference[length] / 2 - (center < min ? minPadding : maxPadding) - arrowDimensions[length] / 2 < 0;
            // eslint-disable-next-line no-nested-ternary
            const alignmentOffset = shouldAddOffset ? center < min ? center - min : center - max : 0;
            return {
                [axis]: coords[axis] + alignmentOffset,
                data: {
                    [axis]: offset,
                    centerOffset: center - offset - alignmentOffset,
                    ...shouldAddOffset && {
                        alignmentOffset
                    }
                },
                reset: shouldAddOffset
            };
        }
    });
const arrow = (options, deps)=>({
        ...baseArrow(options),
        options: [
            options,
            deps
        ]
    });
}),
"[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/utils/hideMiddleware.js [app-client] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "hide",
    ()=>hide
]);
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$floating$2d$ui$2b$react$2d$dom$40$2$2e$1$2e$8$2b$bf16f8eded5e12ee$2f$node_modules$2f40$floating$2d$ui$2f$react$2d$dom$2f$dist$2f$floating$2d$ui$2e$react$2d$dom$2e$mjs__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$locals$3e$__ = __turbopack_context__.i("[project]/node_modules/.bun/@floating-ui+react-dom@2.1.8+bf16f8eded5e12ee/node_modules/@floating-ui/react-dom/dist/floating-ui.react-dom.mjs [app-client] (ecmascript) <locals>");
;
const hide = {
    name: 'hide',
    async fn (state) {
        const { width, height, x, y } = state.rects.reference;
        const anchorHidden = width === 0 && height === 0 && x === 0 && y === 0;
        const nativeHideResult = await (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$floating$2d$ui$2b$react$2d$dom$40$2$2e$1$2e$8$2b$bf16f8eded5e12ee$2f$node_modules$2f40$floating$2d$ui$2f$react$2d$dom$2f$dist$2f$floating$2d$ui$2e$react$2d$dom$2e$mjs__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$locals$3e$__["hide"])().fn(state);
        return {
            data: {
                referenceHidden: nativeHideResult.data?.referenceHidden || anchorHidden
            }
        };
    }
};
}),
"[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/utils/adaptiveOriginMiddleware.js [app-client] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "DEFAULT_SIDES",
    ()=>DEFAULT_SIDES,
    "adaptiveOrigin",
    ()=>adaptiveOrigin
]);
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$owner$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$locals$3e$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+utils@0.2.5+0ea9ec2a211d4613/node_modules/@base-ui/utils/esm/owner.js [app-client] (ecmascript) <locals>");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$floating$2d$ui$2b$utils$40$0$2e$2$2e$11$2f$node_modules$2f40$floating$2d$ui$2f$utils$2f$dist$2f$floating$2d$ui$2e$utils$2e$dom$2e$mjs__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$export__getWindow__as__ownerWindow$3e$__ = __turbopack_context__.i("[project]/node_modules/.bun/@floating-ui+utils@0.2.11/node_modules/@floating-ui/utils/dist/floating-ui.utils.dom.mjs [app-client] (ecmascript) <export getWindow as ownerWindow>");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$floating$2d$ui$2b$utils$40$0$2e$2$2e$11$2f$node_modules$2f40$floating$2d$ui$2f$utils$2f$dist$2f$floating$2d$ui$2e$utils$2e$mjs__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@floating-ui+utils@0.2.11/node_modules/@floating-ui/utils/dist/floating-ui.utils.mjs [app-client] (ecmascript)");
;
;
const DEFAULT_SIDES = {
    sideX: 'left',
    sideY: 'top'
};
const adaptiveOrigin = {
    name: 'adaptiveOrigin',
    async fn (state) {
        const { x: rawX, y: rawY, rects: { floating: floatRect }, elements: { floating }, platform, strategy, placement } = state;
        const win = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$floating$2d$ui$2b$utils$40$0$2e$2$2e$11$2f$node_modules$2f40$floating$2d$ui$2f$utils$2f$dist$2f$floating$2d$ui$2e$utils$2e$dom$2e$mjs__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$export__getWindow__as__ownerWindow$3e$__["ownerWindow"])(floating);
        const styles = win.getComputedStyle(floating);
        const hasTransition = styles.transitionDuration !== '0s' && styles.transitionDuration !== '';
        if (!hasTransition) {
            return {
                x: rawX,
                y: rawY,
                data: DEFAULT_SIDES
            };
        }
        const offsetParent = await platform.getOffsetParent?.(floating);
        let offsetDimensions = {
            width: 0,
            height: 0
        };
        // For fixed strategy, prefer visualViewport if available
        if (strategy === 'fixed' && win?.visualViewport) {
            offsetDimensions = {
                width: win.visualViewport.width,
                height: win.visualViewport.height
            };
        } else if (offsetParent === win) {
            const doc = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$owner$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$locals$3e$__["ownerDocument"])(floating);
            offsetDimensions = {
                width: doc.documentElement.clientWidth,
                height: doc.documentElement.clientHeight
            };
        } else if (await platform.isElement?.(offsetParent)) {
            offsetDimensions = await platform.getDimensions(offsetParent);
        }
        const currentSide = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$floating$2d$ui$2b$utils$40$0$2e$2$2e$11$2f$node_modules$2f40$floating$2d$ui$2f$utils$2f$dist$2f$floating$2d$ui$2e$utils$2e$mjs__$5b$app$2d$client$5d$__$28$ecmascript$29$__["getSide"])(placement);
        let x = rawX;
        let y = rawY;
        if (currentSide === 'left') {
            x = offsetDimensions.width - (rawX + floatRect.width);
        }
        if (currentSide === 'top') {
            y = offsetDimensions.height - (rawY + floatRect.height);
        }
        const sideX = currentSide === 'left' ? 'right' : DEFAULT_SIDES.sideX;
        const sideY = currentSide === 'top' ? 'bottom' : DEFAULT_SIDES.sideY;
        return {
            x,
            y,
            data: {
                sideX,
                sideY
            }
        };
    }
};
}),
"[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/utils/useAnchorPositioning.js [app-client] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "useAnchorPositioning",
    ()=>useAnchorPositioning
]);
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/compiled/react/index.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$floating$2d$ui$2b$utils$40$0$2e$2$2e$11$2f$node_modules$2f40$floating$2d$ui$2f$utils$2f$dist$2f$floating$2d$ui$2e$utils$2e$mjs__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@floating-ui+utils@0.2.11/node_modules/@floating-ui/utils/dist/floating-ui.utils.mjs [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$owner$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$locals$3e$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+utils@0.2.5+0ea9ec2a211d4613/node_modules/@base-ui/utils/esm/owner.js [app-client] (ecmascript) <locals>");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useIsoLayoutEffect$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+utils@0.2.5+0ea9ec2a211d4613/node_modules/@base-ui/utils/esm/useIsoLayoutEffect.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useValueAsRef$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+utils@0.2.5+0ea9ec2a211d4613/node_modules/@base-ui/utils/esm/useValueAsRef.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useStableCallback$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+utils@0.2.5+0ea9ec2a211d4613/node_modules/@base-ui/utils/esm/useStableCallback.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$floating$2d$ui$2b$dom$40$1$2e$7$2e$6$2f$node_modules$2f40$floating$2d$ui$2f$dom$2f$dist$2f$floating$2d$ui$2e$dom$2e$mjs__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$locals$3e$__ = __turbopack_context__.i("[project]/node_modules/.bun/@floating-ui+dom@1.7.6/node_modules/@floating-ui/dom/dist/floating-ui.dom.mjs [app-client] (ecmascript) <locals>");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$floating$2d$ui$2b$react$2d$dom$40$2$2e$1$2e$8$2b$bf16f8eded5e12ee$2f$node_modules$2f40$floating$2d$ui$2f$react$2d$dom$2f$dist$2f$floating$2d$ui$2e$react$2d$dom$2e$mjs__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$locals$3e$__ = __turbopack_context__.i("[project]/node_modules/.bun/@floating-ui+react-dom@2.1.8+bf16f8eded5e12ee/node_modules/@floating-ui/react-dom/dist/floating-ui.react-dom.mjs [app-client] (ecmascript) <locals>");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$hooks$2f$useFloating$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/floating-ui-react/hooks/useFloating.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$direction$2d$provider$2f$DirectionContext$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/direction-provider/DirectionContext.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$middleware$2f$arrow$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/floating-ui-react/middleware/arrow.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$hideMiddleware$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/utils/hideMiddleware.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$adaptiveOriginMiddleware$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/utils/adaptiveOriginMiddleware.js [app-client] (ecmascript)");
'use client';
;
;
;
;
;
;
;
;
;
;
;
function getLogicalSide(sideParam, renderedSide, isRtl) {
    const isLogicalSideParam = sideParam === 'inline-start' || sideParam === 'inline-end';
    const logicalRight = isRtl ? 'inline-start' : 'inline-end';
    const logicalLeft = isRtl ? 'inline-end' : 'inline-start';
    return ({
        top: 'top',
        right: isLogicalSideParam ? logicalRight : 'right',
        bottom: 'bottom',
        left: isLogicalSideParam ? logicalLeft : 'left'
    })[renderedSide];
}
function getOffsetData(state, sideParam, isRtl) {
    const { rects, placement } = state;
    const data = {
        side: getLogicalSide(sideParam, (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$floating$2d$ui$2b$utils$40$0$2e$2$2e$11$2f$node_modules$2f40$floating$2d$ui$2f$utils$2f$dist$2f$floating$2d$ui$2e$utils$2e$mjs__$5b$app$2d$client$5d$__$28$ecmascript$29$__["getSide"])(placement), isRtl),
        align: (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$floating$2d$ui$2b$utils$40$0$2e$2$2e$11$2f$node_modules$2f40$floating$2d$ui$2f$utils$2f$dist$2f$floating$2d$ui$2e$utils$2e$mjs__$5b$app$2d$client$5d$__$28$ecmascript$29$__["getAlignment"])(placement) || 'center',
        anchor: {
            width: rects.reference.width,
            height: rects.reference.height
        },
        positioner: {
            width: rects.floating.width,
            height: rects.floating.height
        }
    };
    return data;
}
function useAnchorPositioning(params) {
    const { // Public parameters
    anchor, positionMethod = 'absolute', side: sideParam = 'bottom', sideOffset = 0, align = 'center', alignOffset = 0, collisionBoundary, collisionPadding: collisionPaddingParam = 5, sticky = false, arrowPadding = 5, disableAnchorTracking = false, // Private parameters
    keepMounted = false, floatingRootContext, mounted, collisionAvoidance, shiftCrossAxis = false, nodeId, adaptiveOrigin, lazyFlip = false, externalTree } = params;
    const [mountSide, setMountSide] = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useState"](null);
    if (!mounted && mountSide !== null) {
        setMountSide(null);
    }
    const collisionAvoidanceSide = collisionAvoidance.side || 'flip';
    const collisionAvoidanceAlign = collisionAvoidance.align || 'flip';
    const collisionAvoidanceFallbackAxisSide = collisionAvoidance.fallbackAxisSide || 'end';
    const anchorFn = typeof anchor === 'function' ? anchor : undefined;
    const anchorFnCallback = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useStableCallback$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useStableCallback"])(anchorFn);
    const anchorDep = anchorFn ? anchorFnCallback : anchor;
    const anchorValueRef = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useValueAsRef$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useValueAsRef"])(anchor);
    const direction = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$direction$2d$provider$2f$DirectionContext$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useDirection"])();
    const isRtl = direction === 'rtl';
    const side = mountSide || ({
        top: 'top',
        right: 'right',
        bottom: 'bottom',
        left: 'left',
        'inline-end': isRtl ? 'left' : 'right',
        'inline-start': isRtl ? 'right' : 'left'
    })[sideParam];
    const placement = align === 'center' ? side : `${side}-${align}`;
    let collisionPadding = collisionPaddingParam;
    // Create a bias to the preferred side.
    // On iOS, when the mobile software keyboard opens, the input is exactly centered
    // in the viewport, but this can cause it to flip to the top undesirably.
    const bias = 1;
    const biasTop = sideParam === 'bottom' ? bias : 0;
    const biasBottom = sideParam === 'top' ? bias : 0;
    const biasLeft = sideParam === 'right' ? bias : 0;
    const biasRight = sideParam === 'left' ? bias : 0;
    if (typeof collisionPadding === 'number') {
        collisionPadding = {
            top: collisionPadding + biasTop,
            right: collisionPadding + biasRight,
            bottom: collisionPadding + biasBottom,
            left: collisionPadding + biasLeft
        };
    } else if (collisionPadding) {
        collisionPadding = {
            top: (collisionPadding.top || 0) + biasTop,
            right: (collisionPadding.right || 0) + biasRight,
            bottom: (collisionPadding.bottom || 0) + biasBottom,
            left: (collisionPadding.left || 0) + biasLeft
        };
    }
    const commonCollisionProps = {
        boundary: collisionBoundary === 'clipping-ancestors' ? 'clippingAncestors' : collisionBoundary,
        padding: collisionPadding
    };
    // Using a ref assumes that the arrow element is always present in the DOM for the lifetime of the
    // popup. If this assumption ends up being false, we can switch to state to manage the arrow's
    // presence.
    const arrowRef = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useRef"](null);
    // Keep these reactive if they're not functions
    const sideOffsetRef = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useValueAsRef$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useValueAsRef"])(sideOffset);
    const alignOffsetRef = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useValueAsRef$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useValueAsRef"])(alignOffset);
    const sideOffsetDep = typeof sideOffset !== 'function' ? sideOffset : 0;
    const alignOffsetDep = typeof alignOffset !== 'function' ? alignOffset : 0;
    const middleware = [
        (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$floating$2d$ui$2b$react$2d$dom$40$2$2e$1$2e$8$2b$bf16f8eded5e12ee$2f$node_modules$2f40$floating$2d$ui$2f$react$2d$dom$2f$dist$2f$floating$2d$ui$2e$react$2d$dom$2e$mjs__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$locals$3e$__["offset"])((state)=>{
            const data = getOffsetData(state, sideParam, isRtl);
            const sideAxis = typeof sideOffsetRef.current === 'function' ? sideOffsetRef.current(data) : sideOffsetRef.current;
            const alignAxis = typeof alignOffsetRef.current === 'function' ? alignOffsetRef.current(data) : alignOffsetRef.current;
            return {
                mainAxis: sideAxis,
                crossAxis: alignAxis,
                alignmentAxis: alignAxis
            };
        }, [
            sideOffsetDep,
            alignOffsetDep,
            isRtl,
            sideParam
        ])
    ];
    const shiftDisabled = collisionAvoidanceAlign === 'none' && collisionAvoidanceSide !== 'shift';
    const crossAxisShiftEnabled = !shiftDisabled && (sticky || shiftCrossAxis || collisionAvoidanceSide === 'shift');
    const flipMiddleware = collisionAvoidanceSide === 'none' ? null : (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$floating$2d$ui$2b$react$2d$dom$40$2$2e$1$2e$8$2b$bf16f8eded5e12ee$2f$node_modules$2f40$floating$2d$ui$2f$react$2d$dom$2f$dist$2f$floating$2d$ui$2e$react$2d$dom$2e$mjs__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$locals$3e$__["flip"])({
        ...commonCollisionProps,
        // Ensure the popup flips if it's been limited by its --available-height and it resizes.
        // Since the size() padding is smaller than the flip() padding, flip() will take precedence.
        padding: {
            top: collisionPadding.top + bias,
            right: collisionPadding.right + bias,
            bottom: collisionPadding.bottom + bias,
            left: collisionPadding.left + bias
        },
        mainAxis: !shiftCrossAxis && collisionAvoidanceSide === 'flip',
        crossAxis: collisionAvoidanceAlign === 'flip' ? 'alignment' : false,
        fallbackAxisSideDirection: collisionAvoidanceFallbackAxisSide
    });
    const shiftMiddleware = shiftDisabled ? null : (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$floating$2d$ui$2b$react$2d$dom$40$2$2e$1$2e$8$2b$bf16f8eded5e12ee$2f$node_modules$2f40$floating$2d$ui$2f$react$2d$dom$2f$dist$2f$floating$2d$ui$2e$react$2d$dom$2e$mjs__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$locals$3e$__["shift"])((data)=>{
        const html = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$owner$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$locals$3e$__["ownerDocument"])(data.elements.floating).documentElement;
        return {
            ...commonCollisionProps,
            // Use the Layout Viewport to avoid shifting around when pinch-zooming
            // for context menus.
            rootBoundary: shiftCrossAxis ? {
                x: 0,
                y: 0,
                width: html.clientWidth,
                height: html.clientHeight
            } : undefined,
            mainAxis: collisionAvoidanceAlign !== 'none',
            crossAxis: crossAxisShiftEnabled,
            limiter: sticky || shiftCrossAxis ? undefined : (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$floating$2d$ui$2b$react$2d$dom$40$2$2e$1$2e$8$2b$bf16f8eded5e12ee$2f$node_modules$2f40$floating$2d$ui$2f$react$2d$dom$2f$dist$2f$floating$2d$ui$2e$react$2d$dom$2e$mjs__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$locals$3e$__["limitShift"])((limitData)=>{
                if (!arrowRef.current) {
                    return {};
                }
                const { width, height } = arrowRef.current.getBoundingClientRect();
                const sideAxis = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$floating$2d$ui$2b$utils$40$0$2e$2$2e$11$2f$node_modules$2f40$floating$2d$ui$2f$utils$2f$dist$2f$floating$2d$ui$2e$utils$2e$mjs__$5b$app$2d$client$5d$__$28$ecmascript$29$__["getSideAxis"])((0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$floating$2d$ui$2b$utils$40$0$2e$2$2e$11$2f$node_modules$2f40$floating$2d$ui$2f$utils$2f$dist$2f$floating$2d$ui$2e$utils$2e$mjs__$5b$app$2d$client$5d$__$28$ecmascript$29$__["getSide"])(limitData.placement));
                const arrowSize = sideAxis === 'y' ? width : height;
                const offsetAmount = sideAxis === 'y' ? collisionPadding.left + collisionPadding.right : collisionPadding.top + collisionPadding.bottom;
                return {
                    offset: arrowSize / 2 + offsetAmount / 2
                };
            })
        };
    }, [
        commonCollisionProps,
        sticky,
        shiftCrossAxis,
        collisionPadding,
        collisionAvoidanceAlign
    ]);
    // https://floating-ui.com/docs/flip#combining-with-shift
    if (collisionAvoidanceSide === 'shift' || collisionAvoidanceAlign === 'shift' || align === 'center') {
        middleware.push(shiftMiddleware, flipMiddleware);
    } else {
        middleware.push(flipMiddleware, shiftMiddleware);
    }
    middleware.push((0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$floating$2d$ui$2b$react$2d$dom$40$2$2e$1$2e$8$2b$bf16f8eded5e12ee$2f$node_modules$2f40$floating$2d$ui$2f$react$2d$dom$2f$dist$2f$floating$2d$ui$2e$react$2d$dom$2e$mjs__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$locals$3e$__["size"])({
        ...commonCollisionProps,
        apply ({ elements: { floating }, rects: { reference }, availableWidth, availableHeight }) {
            const floatingStyle = floating.style;
            floatingStyle.setProperty('--available-width', `${availableWidth}px`);
            floatingStyle.setProperty('--available-height', `${availableHeight}px`);
            floatingStyle.setProperty('--anchor-width', `${reference.width}px`);
            floatingStyle.setProperty('--anchor-height', `${reference.height}px`);
        }
    }), (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$middleware$2f$arrow$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["arrow"])(()=>({
            // `transform-origin` calculations rely on an element existing. If the arrow hasn't been set,
            // we'll create a fake element.
            element: arrowRef.current || document.createElement('div'),
            padding: arrowPadding,
            offsetParent: 'floating'
        }), [
        arrowPadding
    ]), {
        name: 'transformOrigin',
        fn (state) {
            const { elements, middlewareData, placement: renderedPlacement, rects, y } = state;
            const currentRenderedSide = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$floating$2d$ui$2b$utils$40$0$2e$2$2e$11$2f$node_modules$2f40$floating$2d$ui$2f$utils$2f$dist$2f$floating$2d$ui$2e$utils$2e$mjs__$5b$app$2d$client$5d$__$28$ecmascript$29$__["getSide"])(renderedPlacement);
            const currentRenderedAxis = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$floating$2d$ui$2b$utils$40$0$2e$2$2e$11$2f$node_modules$2f40$floating$2d$ui$2f$utils$2f$dist$2f$floating$2d$ui$2e$utils$2e$mjs__$5b$app$2d$client$5d$__$28$ecmascript$29$__["getSideAxis"])(currentRenderedSide);
            const arrowEl = arrowRef.current;
            const arrowX = middlewareData.arrow?.x || 0;
            const arrowY = middlewareData.arrow?.y || 0;
            const arrowWidth = arrowEl?.clientWidth || 0;
            const arrowHeight = arrowEl?.clientHeight || 0;
            const transformX = arrowX + arrowWidth / 2;
            const transformY = arrowY + arrowHeight / 2;
            const shiftY = Math.abs(middlewareData.shift?.y || 0);
            const halfAnchorHeight = rects.reference.height / 2;
            const sideOffsetValue = typeof sideOffset === 'function' ? sideOffset(getOffsetData(state, sideParam, isRtl)) : sideOffset;
            const isOverlappingAnchor = shiftY > sideOffsetValue;
            const adjacentTransformOrigin = {
                top: `${transformX}px calc(100% + ${sideOffsetValue}px)`,
                bottom: `${transformX}px ${-sideOffsetValue}px`,
                left: `calc(100% + ${sideOffsetValue}px) ${transformY}px`,
                right: `${-sideOffsetValue}px ${transformY}px`
            }[currentRenderedSide];
            const overlapTransformOrigin = `${transformX}px ${rects.reference.y + halfAnchorHeight - y}px`;
            elements.floating.style.setProperty('--transform-origin', crossAxisShiftEnabled && currentRenderedAxis === 'y' && isOverlappingAnchor ? overlapTransformOrigin : adjacentTransformOrigin);
            return {};
        }
    }, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$hideMiddleware$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["hide"], adaptiveOrigin);
    (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useIsoLayoutEffect$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useIsoLayoutEffect"])({
        "useAnchorPositioning.useIsoLayoutEffect": ()=>{
            // Ensure positioning doesn't run initially for `keepMounted` elements that
            // aren't initially open.
            if (!mounted && floatingRootContext) {
                floatingRootContext.update({
                    referenceElement: null,
                    floatingElement: null,
                    domReferenceElement: null
                });
            }
        }
    }["useAnchorPositioning.useIsoLayoutEffect"], [
        mounted,
        floatingRootContext
    ]);
    const autoUpdateOptions = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useMemo"]({
        "useAnchorPositioning.useMemo[autoUpdateOptions]": ()=>({
                elementResize: !disableAnchorTracking && typeof ResizeObserver !== 'undefined',
                layoutShift: !disableAnchorTracking && typeof IntersectionObserver !== 'undefined'
            })
    }["useAnchorPositioning.useMemo[autoUpdateOptions]"], [
        disableAnchorTracking
    ]);
    const { refs, elements, x, y, middlewareData, update, placement: renderedPlacement, context, isPositioned, floatingStyles: originalFloatingStyles } = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$hooks$2f$useFloating$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useFloating"])({
        rootContext: floatingRootContext,
        placement,
        middleware,
        strategy: positionMethod,
        whileElementsMounted: keepMounted ? undefined : ({
            "useAnchorPositioning.useFloating": (...args)=>(0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$floating$2d$ui$2b$dom$40$1$2e$7$2e$6$2f$node_modules$2f40$floating$2d$ui$2f$dom$2f$dist$2f$floating$2d$ui$2e$dom$2e$mjs__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$locals$3e$__["autoUpdate"])(...args, autoUpdateOptions)
        })["useAnchorPositioning.useFloating"],
        nodeId,
        externalTree
    });
    const { sideX, sideY } = middlewareData.adaptiveOrigin || __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$adaptiveOriginMiddleware$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["DEFAULT_SIDES"];
    // Default to `fixed` when not positioned to prevent `autoFocus` scroll jumps.
    // This ensures the popup is inside the viewport initially before it gets positioned.
    const resolvedPosition = isPositioned ? positionMethod : 'fixed';
    const floatingStyles = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useMemo"]({
        "useAnchorPositioning.useMemo[floatingStyles]": ()=>{
            const base = adaptiveOrigin ? {
                position: resolvedPosition,
                [sideX]: x,
                [sideY]: y
            } : {
                position: resolvedPosition,
                ...originalFloatingStyles
            };
            if (!isPositioned) {
                base.opacity = 0;
            }
            return base;
        }
    }["useAnchorPositioning.useMemo[floatingStyles]"], [
        adaptiveOrigin,
        resolvedPosition,
        sideX,
        x,
        sideY,
        y,
        originalFloatingStyles,
        isPositioned
    ]);
    const registeredPositionReferenceRef = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useRef"](null);
    (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useIsoLayoutEffect$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useIsoLayoutEffect"])({
        "useAnchorPositioning.useIsoLayoutEffect": ()=>{
            if (!mounted) {
                return;
            }
            const anchorValue = anchorValueRef.current;
            const resolvedAnchor = typeof anchorValue === 'function' ? anchorValue() : anchorValue;
            const unwrappedElement = (isRef(resolvedAnchor) ? resolvedAnchor.current : resolvedAnchor) || null;
            const finalAnchor = unwrappedElement || null;
            if (finalAnchor !== registeredPositionReferenceRef.current) {
                refs.setPositionReference(finalAnchor);
                registeredPositionReferenceRef.current = finalAnchor;
            }
        }
    }["useAnchorPositioning.useIsoLayoutEffect"], [
        mounted,
        refs,
        anchorDep,
        anchorValueRef
    ]);
    __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useEffect"]({
        "useAnchorPositioning.useEffect": ()=>{
            if (!mounted) {
                return;
            }
            const anchorValue = anchorValueRef.current;
            // Refs from parent components are set after useLayoutEffect runs and are available in useEffect.
            // Therefore, if the anchor is a ref, we need to update the position reference in useEffect.
            if (typeof anchorValue === 'function') {
                return;
            }
            if (isRef(anchorValue) && anchorValue.current !== registeredPositionReferenceRef.current) {
                refs.setPositionReference(anchorValue.current);
                registeredPositionReferenceRef.current = anchorValue.current;
            }
        }
    }["useAnchorPositioning.useEffect"], [
        mounted,
        refs,
        anchorDep,
        anchorValueRef
    ]);
    __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useEffect"]({
        "useAnchorPositioning.useEffect": ()=>{
            if (keepMounted && mounted && elements.domReference && elements.floating) {
                return (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$floating$2d$ui$2b$dom$40$1$2e$7$2e$6$2f$node_modules$2f40$floating$2d$ui$2f$dom$2f$dist$2f$floating$2d$ui$2e$dom$2e$mjs__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$locals$3e$__["autoUpdate"])(elements.domReference, elements.floating, update, autoUpdateOptions);
            }
            return undefined;
        }
    }["useAnchorPositioning.useEffect"], [
        keepMounted,
        mounted,
        elements,
        update,
        autoUpdateOptions
    ]);
    const renderedSide = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$floating$2d$ui$2b$utils$40$0$2e$2$2e$11$2f$node_modules$2f40$floating$2d$ui$2f$utils$2f$dist$2f$floating$2d$ui$2e$utils$2e$mjs__$5b$app$2d$client$5d$__$28$ecmascript$29$__["getSide"])(renderedPlacement);
    const logicalRenderedSide = getLogicalSide(sideParam, renderedSide, isRtl);
    const renderedAlign = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$floating$2d$ui$2b$utils$40$0$2e$2$2e$11$2f$node_modules$2f40$floating$2d$ui$2f$utils$2f$dist$2f$floating$2d$ui$2e$utils$2e$mjs__$5b$app$2d$client$5d$__$28$ecmascript$29$__["getAlignment"])(renderedPlacement) || 'center';
    const anchorHidden = Boolean(middlewareData.hide?.referenceHidden);
    /**
   * Locks the flip (makes it "sticky") so it doesn't prefer a given placement
   * and flips back lazily, not eagerly. Ideal for filtered lists that change
   * the size of the popup dynamically to avoid unwanted flipping when typing.
   */ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useIsoLayoutEffect$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useIsoLayoutEffect"])({
        "useAnchorPositioning.useIsoLayoutEffect": ()=>{
            if (lazyFlip && mounted && isPositioned) {
                setMountSide(renderedSide);
            }
        }
    }["useAnchorPositioning.useIsoLayoutEffect"], [
        lazyFlip,
        mounted,
        isPositioned,
        renderedSide
    ]);
    const arrowStyles = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useMemo"]({
        "useAnchorPositioning.useMemo[arrowStyles]": ()=>({
                position: 'absolute',
                top: middlewareData.arrow?.y,
                left: middlewareData.arrow?.x
            })
    }["useAnchorPositioning.useMemo[arrowStyles]"], [
        middlewareData.arrow
    ]);
    const arrowUncentered = middlewareData.arrow?.centerOffset !== 0;
    return __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useMemo"]({
        "useAnchorPositioning.useMemo": ()=>({
                positionerStyles: floatingStyles,
                arrowStyles,
                arrowRef,
                arrowUncentered,
                side: logicalRenderedSide,
                align: renderedAlign,
                physicalSide: renderedSide,
                anchorHidden,
                refs,
                context,
                isPositioned,
                update
            })
    }["useAnchorPositioning.useMemo"], [
        floatingStyles,
        arrowStyles,
        arrowRef,
        arrowUncentered,
        logicalRenderedSide,
        renderedAlign,
        renderedSide,
        anchorHidden,
        refs,
        context,
        isPositioned,
        update
    ]);
}
function isRef(param) {
    return param != null && 'current' in param;
}
}),
"[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/utils/getDisabledMountTransitionStyles.js [app-client] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "getDisabledMountTransitionStyles",
    ()=>getDisabledMountTransitionStyles
]);
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$constants$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$locals$3e$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/utils/constants.js [app-client] (ecmascript) <locals>");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$empty$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+utils@0.2.5+0ea9ec2a211d4613/node_modules/@base-ui/utils/esm/empty.js [app-client] (ecmascript)");
;
function getDisabledMountTransitionStyles(transitionStatus) {
    return transitionStatus === 'starting' ? __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$constants$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$locals$3e$__["DISABLED_TRANSITIONS_STYLE"] : __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$empty$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["EMPTY_OBJECT"];
}
}),
"[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/tooltip/positioner/TooltipPositioner.js [app-client] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "TooltipPositioner",
    ()=>TooltipPositioner
]);
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$build$2f$polyfills$2f$process$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = /*#__PURE__*/ __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/build/polyfills/process.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/compiled/react/index.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$tooltip$2f$root$2f$TooltipRootContext$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/tooltip/root/TooltipRootContext.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$tooltip$2f$positioner$2f$TooltipPositionerContext$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/tooltip/positioner/TooltipPositionerContext.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$useAnchorPositioning$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/utils/useAnchorPositioning.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$popupStateMapping$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/utils/popupStateMapping.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$tooltip$2f$portal$2f$TooltipPortalContext$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/tooltip/portal/TooltipPortalContext.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$useRenderElement$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/utils/useRenderElement.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$constants$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$locals$3e$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/utils/constants.js [app-client] (ecmascript) <locals>");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$adaptiveOriginMiddleware$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/utils/adaptiveOriginMiddleware.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$getDisabledMountTransitionStyles$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/utils/getDisabledMountTransitionStyles.js [app-client] (ecmascript)");
/**
 * Positions the tooltip against the trigger.
 * Renders a `<div>` element.
 *
 * Documentation: [Base UI Tooltip](https://base-ui.com/react/components/tooltip)
 */ var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/compiled/react/jsx-runtime.js [app-client] (ecmascript)");
'use client';
;
;
;
;
;
;
;
;
;
;
;
const TooltipPositioner = /*#__PURE__*/ __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["forwardRef"](function TooltipPositioner(componentProps, forwardedRef) {
    const { render, className, anchor, positionMethod = 'absolute', side = 'top', align = 'center', sideOffset = 0, alignOffset = 0, collisionBoundary = 'clipping-ancestors', collisionPadding = 5, arrowPadding = 5, sticky = false, disableAnchorTracking = false, collisionAvoidance = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$constants$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$locals$3e$__["POPUP_COLLISION_AVOIDANCE"], ...elementProps } = componentProps;
    const store = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$tooltip$2f$root$2f$TooltipRootContext$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useTooltipRootContext"])();
    const keepMounted = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$tooltip$2f$portal$2f$TooltipPortalContext$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useTooltipPortalContext"])();
    const open = store.useState('open');
    const mounted = store.useState('mounted');
    const trackCursorAxis = store.useState('trackCursorAxis');
    const disableHoverablePopup = store.useState('disableHoverablePopup');
    const floatingRootContext = store.useState('floatingRootContext');
    const instantType = store.useState('instantType');
    const transitionStatus = store.useState('transitionStatus');
    const hasViewport = store.useState('hasViewport');
    const positioning = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$useAnchorPositioning$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useAnchorPositioning"])({
        anchor,
        positionMethod,
        floatingRootContext,
        mounted,
        side,
        sideOffset,
        align,
        alignOffset,
        collisionBoundary,
        collisionPadding,
        sticky,
        arrowPadding,
        disableAnchorTracking,
        keepMounted,
        collisionAvoidance,
        adaptiveOrigin: hasViewport ? __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$adaptiveOriginMiddleware$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["adaptiveOrigin"] : undefined
    });
    const defaultProps = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useMemo"]({
        "TooltipPositioner.TooltipPositioner.useMemo[defaultProps]": ()=>{
            const hiddenStyles = {};
            if (!open || trackCursorAxis === 'both' || disableHoverablePopup) {
                hiddenStyles.pointerEvents = 'none';
            }
            return {
                role: 'presentation',
                hidden: !mounted,
                style: {
                    ...positioning.positionerStyles,
                    ...hiddenStyles
                }
            };
        }
    }["TooltipPositioner.TooltipPositioner.useMemo[defaultProps]"], [
        open,
        trackCursorAxis,
        disableHoverablePopup,
        mounted,
        positioning.positionerStyles
    ]);
    const state = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useMemo"]({
        "TooltipPositioner.TooltipPositioner.useMemo[state]": ()=>({
                open,
                side: positioning.side,
                align: positioning.align,
                anchorHidden: positioning.anchorHidden,
                instant: trackCursorAxis !== 'none' ? 'tracking-cursor' : instantType
            })
    }["TooltipPositioner.TooltipPositioner.useMemo[state]"], [
        open,
        positioning.side,
        positioning.align,
        positioning.anchorHidden,
        trackCursorAxis,
        instantType
    ]);
    const contextValue = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useMemo"]({
        "TooltipPositioner.TooltipPositioner.useMemo[contextValue]": ()=>({
                ...state,
                arrowRef: positioning.arrowRef,
                arrowStyles: positioning.arrowStyles,
                arrowUncentered: positioning.arrowUncentered
            })
    }["TooltipPositioner.TooltipPositioner.useMemo[contextValue]"], [
        state,
        positioning.arrowRef,
        positioning.arrowStyles,
        positioning.arrowUncentered
    ]);
    const element = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$useRenderElement$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useRenderElement"])('div', componentProps, {
        state,
        props: [
            defaultProps,
            (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$getDisabledMountTransitionStyles$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["getDisabledMountTransitionStyles"])(transitionStatus),
            elementProps
        ],
        ref: [
            forwardedRef,
            store.useStateSetter('positionerElement')
        ],
        stateAttributesMapping: __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$popupStateMapping$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["popupStateMapping"]
    });
    return /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsx"])(__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$tooltip$2f$positioner$2f$TooltipPositionerContext$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["TooltipPositionerContext"].Provider, {
        value: contextValue,
        children: element
    });
});
if ("TURBOPACK compile-time truthy", 1) TooltipPositioner.displayName = "TooltipPositioner";
}),
"[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/floating-ui-react/hooks/useHoverFloatingInteraction.js [app-client] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "getDelay",
    ()=>getDelay,
    "useHoverFloatingInteraction",
    ()=>useHoverFloatingInteraction
]);
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/compiled/react/index.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$floating$2d$ui$2b$utils$40$0$2e$2$2e$11$2f$node_modules$2f40$floating$2d$ui$2f$utils$2f$dist$2f$floating$2d$ui$2e$utils$2e$dom$2e$mjs__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@floating-ui+utils@0.2.11/node_modules/@floating-ui/utils/dist/floating-ui.utils.dom.mjs [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useStableCallback$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+utils@0.2.5+0ea9ec2a211d4613/node_modules/@base-ui/utils/esm/useStableCallback.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useIsoLayoutEffect$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+utils@0.2.5+0ea9ec2a211d4613/node_modules/@base-ui/utils/esm/useIsoLayoutEffect.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$owner$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$locals$3e$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+utils@0.2.5+0ea9ec2a211d4613/node_modules/@base-ui/utils/esm/owner.js [app-client] (ecmascript) <locals>");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$element$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/floating-ui-react/utils/element.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$event$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/floating-ui-react/utils/event.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$createBaseUIEventDetails$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/utils/createBaseUIEventDetails.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$reason$2d$parts$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$export__$2a$__as__REASONS$3e$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/utils/reason-parts.js [app-client] (ecmascript) <export * as REASONS>");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$components$2f$FloatingTree$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/floating-ui-react/components/FloatingTree.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$hooks$2f$useHoverInteractionSharedState$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/floating-ui-react/hooks/useHoverInteractionSharedState.js [app-client] (ecmascript)");
'use client';
;
;
;
;
;
;
;
;
;
;
const clickLikeEvents = new Set([
    'click',
    'mousedown'
]);
function useHoverFloatingInteraction(context, parameters = {}) {
    const store = 'rootStore' in context ? context.rootStore : context;
    const open = store.useState('open');
    const floatingElement = store.useState('floatingElement');
    const domReferenceElement = store.useState('domReferenceElement');
    const { dataRef } = store.context;
    const { enabled = true, closeDelay: closeDelayProp = 0 } = parameters;
    const instance = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$hooks$2f$useHoverInteractionSharedState$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useHoverInteractionSharedState"])(store);
    const tree = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$components$2f$FloatingTree$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useFloatingTree"])();
    const parentId = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$components$2f$FloatingTree$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useFloatingParentNodeId"])();
    const isClickLikeOpenEvent = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useStableCallback$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useStableCallback"])({
        "useHoverFloatingInteraction.useStableCallback[isClickLikeOpenEvent]": ()=>{
            if (instance.interactedInside) {
                return true;
            }
            return dataRef.current.openEvent ? clickLikeEvents.has(dataRef.current.openEvent.type) : false;
        }
    }["useHoverFloatingInteraction.useStableCallback[isClickLikeOpenEvent]"]);
    const isHoverOpen = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useStableCallback$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useStableCallback"])({
        "useHoverFloatingInteraction.useStableCallback[isHoverOpen]": ()=>{
            const type = dataRef.current.openEvent?.type;
            return type?.includes('mouse') && type !== 'mousedown';
        }
    }["useHoverFloatingInteraction.useStableCallback[isHoverOpen]"]);
    const isRelatedTargetInsideEnabledTrigger = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useStableCallback$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useStableCallback"])({
        "useHoverFloatingInteraction.useStableCallback[isRelatedTargetInsideEnabledTrigger]": (target)=>{
            return (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$element$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["isTargetInsideEnabledTrigger"])(target, store.context.triggerElements);
        }
    }["useHoverFloatingInteraction.useStableCallback[isRelatedTargetInsideEnabledTrigger]"]);
    const closeWithDelay = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useCallback"]({
        "useHoverFloatingInteraction.useCallback[closeWithDelay]": (event, runElseBranch = true)=>{
            const closeDelay = getDelay(closeDelayProp, instance.pointerType);
            if (closeDelay && !instance.handler) {
                instance.openChangeTimeout.start(closeDelay, {
                    "useHoverFloatingInteraction.useCallback[closeWithDelay]": ()=>store.setOpen(false, (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$createBaseUIEventDetails$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["createChangeEventDetails"])(__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$reason$2d$parts$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$export__$2a$__as__REASONS$3e$__["REASONS"].triggerHover, event))
                }["useHoverFloatingInteraction.useCallback[closeWithDelay]"]);
            } else if (runElseBranch) {
                instance.openChangeTimeout.clear();
                store.setOpen(false, (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$createBaseUIEventDetails$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["createChangeEventDetails"])(__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$reason$2d$parts$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$export__$2a$__as__REASONS$3e$__["REASONS"].triggerHover, event));
            }
        }
    }["useHoverFloatingInteraction.useCallback[closeWithDelay]"], [
        closeDelayProp,
        store,
        instance
    ]);
    const cleanupMouseMoveHandler = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useStableCallback$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useStableCallback"])({
        "useHoverFloatingInteraction.useStableCallback[cleanupMouseMoveHandler]": ()=>{
            instance.unbindMouseMove();
            instance.handler = undefined;
        }
    }["useHoverFloatingInteraction.useStableCallback[cleanupMouseMoveHandler]"]);
    const clearPointerEvents = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useStableCallback$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useStableCallback"])({
        "useHoverFloatingInteraction.useStableCallback[clearPointerEvents]": ()=>{
            if (instance.performedPointerEventsMutation) {
                const body = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$owner$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$locals$3e$__["ownerDocument"])(floatingElement).body;
                body.style.pointerEvents = '';
                body.removeAttribute(__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$hooks$2f$useHoverInteractionSharedState$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["safePolygonIdentifier"]);
                instance.performedPointerEventsMutation = false;
            }
        }
    }["useHoverFloatingInteraction.useStableCallback[clearPointerEvents]"]);
    const handleInteractInside = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useStableCallback$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useStableCallback"])({
        "useHoverFloatingInteraction.useStableCallback[handleInteractInside]": (event)=>{
            const target = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$element$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["getTarget"])(event);
            if (!(0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$hooks$2f$useHoverInteractionSharedState$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["isInteractiveElement"])(target)) {
                instance.interactedInside = false;
                return;
            }
            instance.interactedInside = true;
        }
    }["useHoverFloatingInteraction.useStableCallback[handleInteractInside]"]);
    (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useIsoLayoutEffect$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useIsoLayoutEffect"])({
        "useHoverFloatingInteraction.useIsoLayoutEffect": ()=>{
            if (!open) {
                instance.pointerType = undefined;
                instance.restTimeoutPending = false;
                instance.interactedInside = false;
                cleanupMouseMoveHandler();
                clearPointerEvents();
            }
        }
    }["useHoverFloatingInteraction.useIsoLayoutEffect"], [
        open,
        instance,
        cleanupMouseMoveHandler,
        clearPointerEvents
    ]);
    __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useEffect"]({
        "useHoverFloatingInteraction.useEffect": ()=>{
            return ({
                "useHoverFloatingInteraction.useEffect": ()=>{
                    cleanupMouseMoveHandler();
                }
            })["useHoverFloatingInteraction.useEffect"];
        }
    }["useHoverFloatingInteraction.useEffect"], [
        cleanupMouseMoveHandler
    ]);
    __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useEffect"]({
        "useHoverFloatingInteraction.useEffect": ()=>{
            return clearPointerEvents;
        }
    }["useHoverFloatingInteraction.useEffect"], [
        clearPointerEvents
    ]);
    (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useIsoLayoutEffect$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useIsoLayoutEffect"])({
        "useHoverFloatingInteraction.useIsoLayoutEffect": ()=>{
            if (!enabled) {
                return undefined;
            }
            if (open && instance.handleCloseOptions?.blockPointerEvents && isHoverOpen() && (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$floating$2d$ui$2b$utils$40$0$2e$2$2e$11$2f$node_modules$2f40$floating$2d$ui$2f$utils$2f$dist$2f$floating$2d$ui$2e$utils$2e$dom$2e$mjs__$5b$app$2d$client$5d$__$28$ecmascript$29$__["isElement"])(domReferenceElement) && floatingElement) {
                instance.performedPointerEventsMutation = true;
                const body = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$owner$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$locals$3e$__["ownerDocument"])(floatingElement).body;
                body.setAttribute(__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$hooks$2f$useHoverInteractionSharedState$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["safePolygonIdentifier"], '');
                const ref = domReferenceElement;
                const floatingEl = floatingElement;
                const parentFloating = tree?.nodesRef.current.find({
                    "useHoverFloatingInteraction.useIsoLayoutEffect": (node)=>node.id === parentId
                }["useHoverFloatingInteraction.useIsoLayoutEffect"])?.context?.elements.floating;
                if (parentFloating) {
                    parentFloating.style.pointerEvents = '';
                }
                body.style.pointerEvents = 'none';
                ref.style.pointerEvents = 'auto';
                floatingEl.style.pointerEvents = 'auto';
                return ({
                    "useHoverFloatingInteraction.useIsoLayoutEffect": ()=>{
                        body.style.pointerEvents = '';
                        ref.style.pointerEvents = '';
                        floatingEl.style.pointerEvents = '';
                    }
                })["useHoverFloatingInteraction.useIsoLayoutEffect"];
            }
            return undefined;
        }
    }["useHoverFloatingInteraction.useIsoLayoutEffect"], [
        enabled,
        open,
        domReferenceElement,
        floatingElement,
        instance,
        isHoverOpen,
        tree,
        parentId
    ]);
    __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useEffect"]({
        "useHoverFloatingInteraction.useEffect": ()=>{
            if (!enabled) {
                return undefined;
            }
            // Ensure the floating element closes after scrolling even if the pointer
            // did not move.
            // https://github.com/floating-ui/floating-ui/discussions/1692
            function onScrollMouseLeave(event) {
                if (isClickLikeOpenEvent() || !dataRef.current.floatingContext || !store.select('open')) {
                    return;
                }
                if (isRelatedTargetInsideEnabledTrigger(event.relatedTarget)) {
                    // If the mouse is leaving the reference element to another trigger, don't explicitly close the popup
                    // as it will be moved.
                    return;
                }
                clearPointerEvents();
                cleanupMouseMoveHandler();
                if (!isClickLikeOpenEvent()) {
                    closeWithDelay(event);
                }
            }
            function onFloatingMouseEnter(event) {
                instance.openChangeTimeout.clear();
                clearPointerEvents();
                instance.handler?.(event);
                cleanupMouseMoveHandler();
            }
            function onFloatingMouseLeave(event) {
                if (!isClickLikeOpenEvent()) {
                    closeWithDelay(event, false);
                }
            }
            const floating = floatingElement;
            if (floating) {
                floating.addEventListener('mouseleave', onScrollMouseLeave);
                floating.addEventListener('mouseenter', onFloatingMouseEnter);
                floating.addEventListener('mouseleave', onFloatingMouseLeave);
                floating.addEventListener('pointerdown', handleInteractInside, true);
            }
            return ({
                "useHoverFloatingInteraction.useEffect": ()=>{
                    if (floating) {
                        floating.removeEventListener('mouseleave', onScrollMouseLeave);
                        floating.removeEventListener('mouseenter', onFloatingMouseEnter);
                        floating.removeEventListener('mouseleave', onFloatingMouseLeave);
                        floating.removeEventListener('pointerdown', handleInteractInside, true);
                    }
                }
            })["useHoverFloatingInteraction.useEffect"];
        }
    }["useHoverFloatingInteraction.useEffect"], [
        enabled,
        floatingElement,
        store,
        dataRef,
        isClickLikeOpenEvent,
        isRelatedTargetInsideEnabledTrigger,
        closeWithDelay,
        clearPointerEvents,
        cleanupMouseMoveHandler,
        handleInteractInside,
        instance
    ]);
}
function getDelay(value, pointerType) {
    if (pointerType && !(0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$utils$2f$event$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["isMouseLikePointerType"])(pointerType)) {
        return 0;
    }
    if (typeof value === 'function') {
        return value();
    }
    return value;
}
}),
"[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/tooltip/popup/TooltipPopup.js [app-client] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "TooltipPopup",
    ()=>TooltipPopup
]);
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$build$2f$polyfills$2f$process$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = /*#__PURE__*/ __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/build/polyfills/process.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/compiled/react/index.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$tooltip$2f$root$2f$TooltipRootContext$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/tooltip/root/TooltipRootContext.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$tooltip$2f$positioner$2f$TooltipPositionerContext$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/tooltip/positioner/TooltipPositionerContext.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$popupStateMapping$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/utils/popupStateMapping.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$stateAttributesMapping$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/utils/stateAttributesMapping.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$useOpenChangeComplete$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/utils/useOpenChangeComplete.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$useRenderElement$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/utils/useRenderElement.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$getDisabledMountTransitionStyles$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/utils/getDisabledMountTransitionStyles.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$hooks$2f$useHoverFloatingInteraction$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/floating-ui-react/hooks/useHoverFloatingInteraction.js [app-client] (ecmascript)");
'use client';
;
;
;
;
;
;
;
;
;
const stateAttributesMapping = {
    ...__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$popupStateMapping$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["popupStateMapping"],
    ...__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$stateAttributesMapping$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["transitionStatusMapping"]
};
const TooltipPopup = /*#__PURE__*/ __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["forwardRef"](function TooltipPopup(componentProps, forwardedRef) {
    const { className, render, ...elementProps } = componentProps;
    const store = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$tooltip$2f$root$2f$TooltipRootContext$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useTooltipRootContext"])();
    const { side, align } = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$tooltip$2f$positioner$2f$TooltipPositionerContext$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useTooltipPositionerContext"])();
    const open = store.useState('open');
    const instantType = store.useState('instantType');
    const transitionStatus = store.useState('transitionStatus');
    const popupProps = store.useState('popupProps');
    const floatingContext = store.useState('floatingRootContext');
    (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$useOpenChangeComplete$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useOpenChangeComplete"])({
        open,
        ref: store.context.popupRef,
        onComplete () {
            if (open) {
                store.context.onOpenChangeComplete?.(true);
            }
        }
    });
    const disabled = store.useState('disabled');
    const closeDelay = store.useState('closeDelay');
    (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$hooks$2f$useHoverFloatingInteraction$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useHoverFloatingInteraction"])(floatingContext, {
        enabled: !disabled,
        closeDelay
    });
    const state = {
        open,
        side,
        align,
        instant: instantType,
        transitionStatus
    };
    const element = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$useRenderElement$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useRenderElement"])('div', componentProps, {
        state,
        ref: [
            forwardedRef,
            store.context.popupRef,
            store.useStateSetter('popupElement')
        ],
        props: [
            popupProps,
            (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$getDisabledMountTransitionStyles$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["getDisabledMountTransitionStyles"])(transitionStatus),
            elementProps
        ],
        stateAttributesMapping
    });
    return element;
});
if ("TURBOPACK compile-time truthy", 1) TooltipPopup.displayName = "TooltipPopup";
}),
"[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/tooltip/arrow/TooltipArrow.js [app-client] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "TooltipArrow",
    ()=>TooltipArrow
]);
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$build$2f$polyfills$2f$process$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = /*#__PURE__*/ __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/build/polyfills/process.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/compiled/react/index.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$tooltip$2f$positioner$2f$TooltipPositionerContext$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/tooltip/positioner/TooltipPositionerContext.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$popupStateMapping$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/utils/popupStateMapping.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$useRenderElement$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/utils/useRenderElement.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$tooltip$2f$root$2f$TooltipRootContext$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/tooltip/root/TooltipRootContext.js [app-client] (ecmascript)");
'use client';
;
;
;
;
;
const TooltipArrow = /*#__PURE__*/ __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["forwardRef"](function TooltipArrow(componentProps, forwardedRef) {
    const { className, render, ...elementProps } = componentProps;
    const store = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$tooltip$2f$root$2f$TooltipRootContext$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useTooltipRootContext"])();
    const instantType = store.useState('instantType');
    const { open, arrowRef, side, align, arrowUncentered, arrowStyles } = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$tooltip$2f$positioner$2f$TooltipPositionerContext$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useTooltipPositionerContext"])();
    const state = {
        open,
        side,
        align,
        uncentered: arrowUncentered,
        instant: instantType
    };
    const element = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$useRenderElement$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useRenderElement"])('div', componentProps, {
        state,
        ref: [
            forwardedRef,
            arrowRef
        ],
        props: [
            {
                style: arrowStyles,
                'aria-hidden': true
            },
            elementProps
        ],
        stateAttributesMapping: __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$popupStateMapping$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["popupStateMapping"]
    });
    return element;
});
if ("TURBOPACK compile-time truthy", 1) TooltipArrow.displayName = "TooltipArrow";
}),
"[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/tooltip/provider/TooltipProvider.js [app-client] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "TooltipProvider",
    ()=>TooltipProvider
]);
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$build$2f$polyfills$2f$process$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = /*#__PURE__*/ __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/build/polyfills/process.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/compiled/react/index.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$components$2f$FloatingDelayGroup$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/floating-ui-react/components/FloatingDelayGroup.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$tooltip$2f$provider$2f$TooltipProviderContext$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/tooltip/provider/TooltipProviderContext.js [app-client] (ecmascript)");
/**
 * Provides a shared delay for multiple tooltips. The grouping logic ensures that
 * once a tooltip becomes visible, the adjacent tooltips will be shown instantly.
 *
 * Documentation: [Base UI Tooltip](https://base-ui.com/react/components/tooltip)
 */ var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/compiled/react/jsx-runtime.js [app-client] (ecmascript)");
'use client';
;
;
;
;
const TooltipProvider = function TooltipProvider(props) {
    const { delay, closeDelay, timeout = 400 } = props;
    const contextValue = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useMemo"]({
        "TooltipProvider.useMemo[contextValue]": ()=>({
                delay,
                closeDelay
            })
    }["TooltipProvider.useMemo[contextValue]"], [
        delay,
        closeDelay
    ]);
    const delayValue = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useMemo"]({
        "TooltipProvider.useMemo[delayValue]": ()=>({
                open: delay,
                close: closeDelay
            })
    }["TooltipProvider.useMemo[delayValue]"], [
        delay,
        closeDelay
    ]);
    return /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsx"])(__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$tooltip$2f$provider$2f$TooltipProviderContext$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["TooltipProviderContext"].Provider, {
        value: contextValue,
        children: /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsx"])(__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$floating$2d$ui$2d$react$2f$components$2f$FloatingDelayGroup$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["FloatingDelayGroup"], {
            delay: delayValue,
            timeoutMs: timeout,
            children: props.children
        })
    });
};
if ("TURBOPACK compile-time truthy", 1) TooltipProvider.displayName = "TooltipProvider";
}),
"[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/tooltip/viewport/TooltipViewportCssVars.js [app-client] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "TooltipViewportCssVars",
    ()=>TooltipViewportCssVars
]);
let TooltipViewportCssVars = /*#__PURE__*/ function(TooltipViewportCssVars) {
    /**
   * The width of the parent popup.
   * This variable is placed on the 'previous' container and stores the width of the popup when the previous content was rendered.
   * It can be used to freeze the dimensions of the popup when animating between different content.
   */ TooltipViewportCssVars["popupWidth"] = "--popup-width";
    /**
   * The height of the parent popup.
   * This variable is placed on the 'previous' container and stores the height of the popup when the previous content was rendered.
   * It can be used to freeze the dimensions of the popup when animating between different content.
   */ TooltipViewportCssVars["popupHeight"] = "--popup-height";
    return TooltipViewportCssVars;
}({});
}),
"[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/utils/getCssDimensions.js [app-client] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "getCssDimensions",
    ()=>getCssDimensions
]);
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$floating$2d$ui$2b$utils$40$0$2e$2$2e$11$2f$node_modules$2f40$floating$2d$ui$2f$utils$2f$dist$2f$floating$2d$ui$2e$utils$2e$mjs__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@floating-ui+utils@0.2.11/node_modules/@floating-ui/utils/dist/floating-ui.utils.mjs [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$floating$2d$ui$2b$utils$40$0$2e$2$2e$11$2f$node_modules$2f40$floating$2d$ui$2f$utils$2f$dist$2f$floating$2d$ui$2e$utils$2e$dom$2e$mjs__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@floating-ui+utils@0.2.11/node_modules/@floating-ui/utils/dist/floating-ui.utils.dom.mjs [app-client] (ecmascript)");
;
;
function getCssDimensions(element) {
    const css = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$floating$2d$ui$2b$utils$40$0$2e$2$2e$11$2f$node_modules$2f40$floating$2d$ui$2f$utils$2f$dist$2f$floating$2d$ui$2e$utils$2e$dom$2e$mjs__$5b$app$2d$client$5d$__$28$ecmascript$29$__["getComputedStyle"])(element);
    // In testing environments, the `width` and `height` properties are empty
    // strings for SVG elements, returning NaN. Fallback to `0` in this case.
    let width = parseFloat(css.width) || 0;
    let height = parseFloat(css.height) || 0;
    const hasOffset = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$floating$2d$ui$2b$utils$40$0$2e$2$2e$11$2f$node_modules$2f40$floating$2d$ui$2f$utils$2f$dist$2f$floating$2d$ui$2e$utils$2e$dom$2e$mjs__$5b$app$2d$client$5d$__$28$ecmascript$29$__["isHTMLElement"])(element);
    const offsetWidth = hasOffset ? element.offsetWidth : width;
    const offsetHeight = hasOffset ? element.offsetHeight : height;
    const shouldFallback = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$floating$2d$ui$2b$utils$40$0$2e$2$2e$11$2f$node_modules$2f40$floating$2d$ui$2f$utils$2f$dist$2f$floating$2d$ui$2e$utils$2e$mjs__$5b$app$2d$client$5d$__$28$ecmascript$29$__["round"])(width) !== offsetWidth || (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$floating$2d$ui$2b$utils$40$0$2e$2$2e$11$2f$node_modules$2f40$floating$2d$ui$2f$utils$2f$dist$2f$floating$2d$ui$2e$utils$2e$mjs__$5b$app$2d$client$5d$__$28$ecmascript$29$__["round"])(height) !== offsetHeight;
    if (shouldFallback) {
        width = offsetWidth;
        height = offsetHeight;
    }
    return {
        width,
        height
    };
}
}),
"[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/utils/usePopupAutoResize.js [app-client] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "usePopupAutoResize",
    ()=>usePopupAutoResize
]);
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/compiled/react/index.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useAnimationFrame$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+utils@0.2.5+0ea9ec2a211d4613/node_modules/@base-ui/utils/esm/useAnimationFrame.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useIsoLayoutEffect$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+utils@0.2.5+0ea9ec2a211d4613/node_modules/@base-ui/utils/esm/useIsoLayoutEffect.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useStableCallback$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+utils@0.2.5+0ea9ec2a211d4613/node_modules/@base-ui/utils/esm/useStableCallback.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$empty$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+utils@0.2.5+0ea9ec2a211d4613/node_modules/@base-ui/utils/esm/empty.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$useAnimationsFinished$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/utils/useAnimationsFinished.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$getCssDimensions$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/utils/getCssDimensions.js [app-client] (ecmascript)");
'use client';
;
;
;
;
;
;
;
;
const supportsResizeObserver = typeof ResizeObserver !== 'undefined';
const DEFAULT_ENABLED = ()=>true;
function usePopupAutoResize(parameters) {
    const { popupElement, positionerElement, content, mounted, enabled = DEFAULT_ENABLED, onMeasureLayout: onMeasureLayoutParam, onMeasureLayoutComplete: onMeasureLayoutCompleteParam, side, direction } = parameters;
    const runOnceAnimationsFinish = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$useAnimationsFinished$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useAnimationsFinished"])(popupElement, true, false);
    const animationFrame = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useAnimationFrame$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useAnimationFrame"])();
    const committedDimensionsRef = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useRef"](null);
    const liveDimensionsRef = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useRef"](null);
    const isInitialRenderRef = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useRef"](true);
    const restoreAnchoringStylesRef = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useRef"](__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$empty$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["NOOP"]);
    const onMeasureLayout = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useStableCallback$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useStableCallback"])(onMeasureLayoutParam);
    const onMeasureLayoutComplete = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useStableCallback$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useStableCallback"])(onMeasureLayoutCompleteParam);
    const anchoringStyles = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useMemo"]({
        "usePopupAutoResize.useMemo[anchoringStyles]": ()=>{
            // Ensure popup size transitions correctly when anchored to `bottom` (side=top) or `right` (side=left).
            let isOriginSide = side === 'top';
            let isPhysicalLeft = side === 'left';
            if (direction === 'rtl') {
                isOriginSide = isOriginSide || side === 'inline-end';
                isPhysicalLeft = isPhysicalLeft || side === 'inline-end';
            } else {
                isOriginSide = isOriginSide || side === 'inline-start';
                isPhysicalLeft = isPhysicalLeft || side === 'inline-start';
            }
            return isOriginSide ? {
                position: 'absolute',
                [side === 'top' ? 'bottom' : 'top']: '0',
                [isPhysicalLeft ? 'right' : 'left']: '0'
            } : __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$empty$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["EMPTY_OBJECT"];
        }
    }["usePopupAutoResize.useMemo[anchoringStyles]"], [
        side,
        direction
    ]);
    (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useIsoLayoutEffect$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useIsoLayoutEffect"])({
        "usePopupAutoResize.useIsoLayoutEffect": ()=>{
            // Reset the state when the popup is closed.
            if (!mounted || !enabled() || !supportsResizeObserver) {
                restoreAnchoringStylesRef.current = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$empty$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["NOOP"];
                isInitialRenderRef.current = true;
                committedDimensionsRef.current = null;
                liveDimensionsRef.current = null;
                return undefined;
            }
            if (!popupElement || !positionerElement) {
                return undefined;
            }
            restoreAnchoringStylesRef.current = applyElementStyles(popupElement, anchoringStyles);
            const observer = new ResizeObserver({
                "usePopupAutoResize.useIsoLayoutEffect": (entries)=>{
                    const entry = entries[0];
                    if (entry) {
                        liveDimensionsRef.current = {
                            width: Math.ceil(entry.borderBoxSize[0].inlineSize),
                            height: Math.ceil(entry.borderBoxSize[0].blockSize)
                        };
                    }
                }
            }["usePopupAutoResize.useIsoLayoutEffect"]);
            observer.observe(popupElement);
            // Measure the rendered size to enable transitions:
            setPopupCssSize(popupElement, 'auto');
            const restorePopupPosition = overrideElementStyle(popupElement, 'position', 'static');
            const restorePopupTransform = overrideElementStyle(popupElement, 'transform', 'none');
            const restorePopupScale = overrideElementStyle(popupElement, 'scale', '1');
            const restorePositionerAvailableSize = applyElementStyles(positionerElement, {
                '--available-width': 'max-content',
                '--available-height': 'max-content'
            });
            function restoreMeasurementOverrides() {
                restorePopupPosition();
                restorePopupTransform();
                restorePositionerAvailableSize();
            }
            function restoreMeasurementOverridesIncludingScale() {
                restoreMeasurementOverrides();
                restorePopupScale();
            }
            onMeasureLayout?.();
            // Initial render (for each time the popup opens).
            if (isInitialRenderRef.current || committedDimensionsRef.current === null) {
                setPositionerCssSize(positionerElement, 'max-content');
                const dimensions = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$getCssDimensions$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["getCssDimensions"])(popupElement);
                committedDimensionsRef.current = dimensions;
                setPositionerCssSize(positionerElement, dimensions);
                restoreMeasurementOverridesIncludingScale();
                onMeasureLayoutComplete?.(null, dimensions);
                isInitialRenderRef.current = false;
                return ({
                    "usePopupAutoResize.useIsoLayoutEffect": ()=>{
                        observer.disconnect();
                        restoreAnchoringStylesRef.current();
                        restoreAnchoringStylesRef.current = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$empty$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["NOOP"];
                    }
                })["usePopupAutoResize.useIsoLayoutEffect"];
            }
            // Subsequent renders while open (when `content` changes).
            setPopupCssSize(popupElement, 'auto');
            setPositionerCssSize(positionerElement, 'max-content');
            const previousDimensions = committedDimensionsRef.current ?? liveDimensionsRef.current;
            const newDimensions = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$getCssDimensions$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["getCssDimensions"])(popupElement);
            // Commit immediately so future content changes have a stable previous size, even if
            // ResizeObserver runs after this point.
            committedDimensionsRef.current = newDimensions;
            if (!previousDimensions) {
                setPositionerCssSize(positionerElement, newDimensions);
                restoreMeasurementOverridesIncludingScale();
                onMeasureLayoutComplete?.(null, newDimensions);
                return ({
                    "usePopupAutoResize.useIsoLayoutEffect": ()=>{
                        observer.disconnect();
                        animationFrame.cancel();
                        restoreAnchoringStylesRef.current();
                        restoreAnchoringStylesRef.current = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$empty$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["NOOP"];
                    }
                })["usePopupAutoResize.useIsoLayoutEffect"];
            }
            setPopupCssSize(popupElement, previousDimensions);
            restoreMeasurementOverridesIncludingScale();
            onMeasureLayoutComplete?.(previousDimensions, newDimensions);
            setPositionerCssSize(positionerElement, newDimensions);
            const abortController = new AbortController();
            animationFrame.request({
                "usePopupAutoResize.useIsoLayoutEffect": ()=>{
                    setPopupCssSize(popupElement, newDimensions);
                    runOnceAnimationsFinish({
                        "usePopupAutoResize.useIsoLayoutEffect": ()=>{
                            popupElement.style.setProperty('--popup-width', 'auto');
                            popupElement.style.setProperty('--popup-height', 'auto');
                        }
                    }["usePopupAutoResize.useIsoLayoutEffect"], abortController.signal);
                }
            }["usePopupAutoResize.useIsoLayoutEffect"]);
            return ({
                "usePopupAutoResize.useIsoLayoutEffect": ()=>{
                    observer.disconnect();
                    abortController.abort();
                    animationFrame.cancel();
                    restoreAnchoringStylesRef.current();
                    restoreAnchoringStylesRef.current = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$empty$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["NOOP"];
                }
            })["usePopupAutoResize.useIsoLayoutEffect"];
        }
    }["usePopupAutoResize.useIsoLayoutEffect"], [
        content,
        popupElement,
        positionerElement,
        runOnceAnimationsFinish,
        animationFrame,
        enabled,
        mounted,
        onMeasureLayout,
        onMeasureLayoutComplete,
        anchoringStyles
    ]);
}
function overrideElementStyle(element, property, value) {
    const originalValue = element.style.getPropertyValue(property);
    element.style.setProperty(property, value);
    return ()=>{
        element.style.setProperty(property, originalValue);
    };
}
function applyElementStyles(element, styles) {
    const restorers = [];
    for (const [key, value] of Object.entries(styles)){
        restorers.push(overrideElementStyle(element, key, value));
    }
    return restorers.length ? ()=>{
        restorers.forEach((restore)=>restore());
    } : __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$empty$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["NOOP"];
}
function setPopupCssSize(popupElement, size) {
    const width = size === 'auto' ? 'auto' : `${size.width}px`;
    const height = size === 'auto' ? 'auto' : `${size.height}px`;
    popupElement.style.setProperty('--popup-width', width);
    popupElement.style.setProperty('--popup-height', height);
}
function setPositionerCssSize(positionerElement, size) {
    const width = size === 'max-content' ? 'max-content' : `${size.width}px`;
    const height = size === 'max-content' ? 'max-content' : `${size.height}px`;
    positionerElement.style.setProperty('--positioner-width', width);
    positionerElement.style.setProperty('--positioner-height', height);
}
}),
"[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/utils/usePopupViewport.js [app-client] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "usePopupViewport",
    ()=>usePopupViewport
]);
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/compiled/react/index.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$inertValue$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+utils@0.2.5+0ea9ec2a211d4613/node_modules/@base-ui/utils/esm/inertValue.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useAnimationFrame$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+utils@0.2.5+0ea9ec2a211d4613/node_modules/@base-ui/utils/esm/useAnimationFrame.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$usePreviousValue$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+utils@0.2.5+0ea9ec2a211d4613/node_modules/@base-ui/utils/esm/usePreviousValue.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useIsoLayoutEffect$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+utils@0.2.5+0ea9ec2a211d4613/node_modules/@base-ui/utils/esm/useIsoLayoutEffect.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useStableCallback$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+utils@0.2.5+0ea9ec2a211d4613/node_modules/@base-ui/utils/esm/useStableCallback.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$useAnimationsFinished$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/utils/useAnimationsFinished.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$usePopupAutoResize$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/utils/usePopupAutoResize.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$direction$2d$provider$2f$DirectionContext$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/direction-provider/DirectionContext.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/compiled/react/jsx-runtime.js [app-client] (ecmascript)");
'use client';
;
;
;
;
;
;
;
;
;
;
function usePopupViewport(parameters) {
    const { store, side, cssVars, children } = parameters;
    const direction = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$direction$2d$provider$2f$DirectionContext$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useDirection"])();
    const activeTrigger = store.useState('activeTriggerElement');
    const activeTriggerId = store.useState('activeTriggerId');
    const open = store.useState('open');
    const payload = store.useState('payload');
    const mounted = store.useState('mounted');
    const popupElement = store.useState('popupElement');
    const positionerElement = store.useState('positionerElement');
    const previousActiveTrigger = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$usePreviousValue$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["usePreviousValue"])(open ? activeTrigger : null);
    // Remount current content on trigger changes (and once more when payload lags) to avoid DOM reuse flashes.
    // The key bumps immediately on trigger switches, then again if the payload arrives on a later render.
    const currentContentKey = usePopupContentKey(activeTriggerId, payload);
    const capturedNodeRef = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useRef"](null);
    const [previousContentNode, setPreviousContentNode] = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useState"](null);
    const [newTriggerOffset, setNewTriggerOffset] = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useState"](null);
    const currentContainerRef = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useRef"](null);
    const previousContainerRef = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useRef"](null);
    const onAnimationsFinished = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$useAnimationsFinished$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useAnimationsFinished"])(currentContainerRef, true, false);
    const cleanupFrame = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useAnimationFrame$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useAnimationFrame"])();
    const [previousContentDimensions, setPreviousContentDimensions] = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useState"](null);
    const [showStartingStyleAttribute, setShowStartingStyleAttribute] = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useState"](false);
    (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useIsoLayoutEffect$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useIsoLayoutEffect"])({
        "usePopupViewport.useIsoLayoutEffect": ()=>{
            store.set('hasViewport', true);
            return ({
                "usePopupViewport.useIsoLayoutEffect": ()=>{
                    store.set('hasViewport', false);
                }
            })["usePopupViewport.useIsoLayoutEffect"];
        }
    }["usePopupViewport.useIsoLayoutEffect"], [
        store
    ]);
    const handleMeasureLayout = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useStableCallback$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useStableCallback"])({
        "usePopupViewport.useStableCallback[handleMeasureLayout]": ()=>{
            currentContainerRef.current?.style.setProperty('animation', 'none');
            currentContainerRef.current?.style.setProperty('transition', 'none');
            previousContainerRef.current?.style.setProperty('display', 'none');
        }
    }["usePopupViewport.useStableCallback[handleMeasureLayout]"]);
    const handleMeasureLayoutComplete = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useStableCallback$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useStableCallback"])({
        "usePopupViewport.useStableCallback[handleMeasureLayoutComplete]": (previousDimensions)=>{
            currentContainerRef.current?.style.removeProperty('animation');
            currentContainerRef.current?.style.removeProperty('transition');
            previousContainerRef.current?.style.removeProperty('display');
            if (previousDimensions) {
                setPreviousContentDimensions(previousDimensions);
            }
        }
    }["usePopupViewport.useStableCallback[handleMeasureLayoutComplete]"]);
    const lastHandledTriggerRef = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useRef"](null);
    (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useIsoLayoutEffect$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useIsoLayoutEffect"])({
        "usePopupViewport.useIsoLayoutEffect": ()=>{
            // When a trigger changes, set the captured children HTML to state,
            // so we can render both new and old content.
            if (activeTrigger && previousActiveTrigger && activeTrigger !== previousActiveTrigger && lastHandledTriggerRef.current !== activeTrigger && capturedNodeRef.current) {
                setPreviousContentNode(capturedNodeRef.current);
                setShowStartingStyleAttribute(true);
                // Calculate the relative position between the previous and new trigger,
                // so we can pass it to the style hook for animation purposes.
                const offset = calculateRelativePosition(previousActiveTrigger, activeTrigger);
                setNewTriggerOffset(offset);
                cleanupFrame.request({
                    "usePopupViewport.useIsoLayoutEffect": ()=>{
                        cleanupFrame.request({
                            "usePopupViewport.useIsoLayoutEffect": ()=>{
                                setShowStartingStyleAttribute(false);
                                onAnimationsFinished({
                                    "usePopupViewport.useIsoLayoutEffect": ()=>{
                                        setPreviousContentNode(null);
                                        setPreviousContentDimensions(null);
                                        capturedNodeRef.current = null;
                                    }
                                }["usePopupViewport.useIsoLayoutEffect"]);
                            }
                        }["usePopupViewport.useIsoLayoutEffect"]);
                    }
                }["usePopupViewport.useIsoLayoutEffect"]);
                lastHandledTriggerRef.current = activeTrigger;
            }
        }
    }["usePopupViewport.useIsoLayoutEffect"], [
        activeTrigger,
        previousActiveTrigger,
        previousContentNode,
        onAnimationsFinished,
        cleanupFrame
    ]);
    // Capture a clone of the current content DOM subtree when not transitioning.
    // We can't store previous React nodes as they may be stateful; instead we capture DOM clones for visual continuity.
    (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useIsoLayoutEffect$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useIsoLayoutEffect"])({
        "usePopupViewport.useIsoLayoutEffect": ()=>{
            // When a transition is in progress, we store the next content in capturedNodeRef.
            // This handles the case where the trigger changes multiple times before the transition finishes.
            // We want to always capture the latest content for the previous snapshot.
            // So clicking quickly on T1, T2, T3 will result in the following sequence:
            // 1. T1 -> T2: previousContent = T1, currentContent = T2
            // 2. T2 -> T3: previousContent = T2, currentContent = T3
            const source = currentContainerRef.current;
            if (!source) {
                return;
            }
            const wrapper = document.createElement('div');
            for (const child of Array.from(source.childNodes)){
                wrapper.appendChild(child.cloneNode(true));
            }
            capturedNodeRef.current = wrapper;
        }
    }["usePopupViewport.useIsoLayoutEffect"]);
    const isTransitioning = previousContentNode != null;
    let childrenToRender;
    if (!isTransitioning) {
        childrenToRender = /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsx"])("div", {
            "data-current": true,
            ref: currentContainerRef,
            children: children
        }, currentContentKey);
    } else {
        childrenToRender = /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsxs"])(__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["Fragment"], {
            children: [
                /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsx"])("div", {
                    "data-previous": true,
                    inert: (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$inertValue$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["inertValue"])(true),
                    ref: previousContainerRef,
                    style: {
                        [cssVars.popupWidth]: `${previousContentDimensions?.width}px`,
                        [cssVars.popupHeight]: `${previousContentDimensions?.height}px`,
                        position: 'absolute'
                    },
                    "data-ending-style": showStartingStyleAttribute ? undefined : ''
                }, "previous"),
                /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$jsx$2d$runtime$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["jsx"])("div", {
                    "data-current": true,
                    ref: currentContainerRef,
                    "data-starting-style": showStartingStyleAttribute ? '' : undefined,
                    children: children
                }, currentContentKey)
            ]
        });
    }
    // When previousContentNode is present, imperatively populate the previous container with the cloned children.
    (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useIsoLayoutEffect$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useIsoLayoutEffect"])({
        "usePopupViewport.useIsoLayoutEffect": ()=>{
            const container = previousContainerRef.current;
            if (!container || !previousContentNode) {
                return;
            }
            container.replaceChildren(...Array.from(previousContentNode.childNodes));
        }
    }["usePopupViewport.useIsoLayoutEffect"], [
        previousContentNode
    ]);
    (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$usePopupAutoResize$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["usePopupAutoResize"])({
        popupElement,
        positionerElement,
        mounted,
        content: payload,
        onMeasureLayout: handleMeasureLayout,
        onMeasureLayoutComplete: handleMeasureLayoutComplete,
        side,
        direction
    });
    const state = {
        activationDirection: getActivationDirection(newTriggerOffset),
        transitioning: isTransitioning
    };
    return {
        children: childrenToRender,
        state
    };
}
/**
 * Returns a string describing the provided offset.
 * It describes both the horizontal and vertical offset, separated by a space.
 *
 * @param offset
 */ function getActivationDirection(offset) {
    if (!offset) {
        return undefined;
    }
    return `${getValueWithTolerance(offset.horizontal, 5, 'right', 'left')} ${getValueWithTolerance(offset.vertical, 5, 'down', 'up')}`;
}
/**
 * Returns a label describing the value (positive/negative) treating values
 * within tolerance as zero.
 *
 * @param value Value to check
 * @param tolerance Tolerance to treat the value as zero.
 * @param positiveLabel
 * @param negativeLabel
 * @returns If 0 < abs(value) < tolerance, returns an empty string. Otherwise returns positiveLabel or negativeLabel.
 */ function getValueWithTolerance(value, tolerance, positiveLabel, negativeLabel) {
    if (value > tolerance) {
        return positiveLabel;
    }
    if (value < -tolerance) {
        return negativeLabel;
    }
    return '';
}
/**
 * Calculates the relative position between centers of two elements.
 */ function calculateRelativePosition(from, to) {
    const fromRect = from.getBoundingClientRect();
    const toRect = to.getBoundingClientRect();
    const fromCenter = {
        x: fromRect.left + fromRect.width / 2,
        y: fromRect.top + fromRect.height / 2
    };
    const toCenter = {
        x: toRect.left + toRect.width / 2,
        y: toRect.top + toRect.height / 2
    };
    return {
        horizontal: toCenter.x - fromCenter.x,
        vertical: toCenter.y - fromCenter.y
    };
}
/**
 * Returns a key that forces remounting content when triggers change or a payload is updated.
 */ function usePopupContentKey(activeTriggerId, payload) {
    const [contentKey, setContentKey] = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useState"](0);
    const previousActiveTriggerIdRef = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useRef"](activeTriggerId);
    const previousPayloadRef = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useRef"](payload);
    const pendingPayloadUpdateRef = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useRef"](false);
    (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$utils$40$0$2e$2$2e$5$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$utils$2f$esm$2f$useIsoLayoutEffect$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useIsoLayoutEffect"])({
        "usePopupContentKey.useIsoLayoutEffect": ()=>{
            // Compare against the last committed values to decide whether we need a new DOM subtree.
            const previousActiveTriggerId = previousActiveTriggerIdRef.current;
            const previousPayload = previousPayloadRef.current;
            const triggerIdChanged = activeTriggerId !== previousActiveTriggerId;
            const payloadChanged = payload !== previousPayload;
            if (triggerIdChanged) {
                // Remount immediately on trigger change; remember if payload hasn't caught up yet.
                setContentKey({
                    "usePopupContentKey.useIsoLayoutEffect": (value)=>value + 1
                }["usePopupContentKey.useIsoLayoutEffect"]);
                pendingPayloadUpdateRef.current = !payloadChanged;
            } else if (pendingPayloadUpdateRef.current && payloadChanged) {
                // Payload arrived a render later, so remount once more to avoid reusing the old <img>.
                setContentKey({
                    "usePopupContentKey.useIsoLayoutEffect": (value)=>value + 1
                }["usePopupContentKey.useIsoLayoutEffect"]);
                pendingPayloadUpdateRef.current = false;
            }
            // Persist current values for the next render's comparison.
            previousActiveTriggerIdRef.current = activeTriggerId;
            previousPayloadRef.current = payload;
        }
    }["usePopupContentKey.useIsoLayoutEffect"], [
        activeTriggerId,
        payload
    ]);
    return `${activeTriggerId ?? 'current'}-${contentKey}`;
}
}),
"[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/tooltip/viewport/TooltipViewport.js [app-client] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "TooltipViewport",
    ()=>TooltipViewport
]);
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$build$2f$polyfills$2f$process$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = /*#__PURE__*/ __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/build/polyfills/process.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/compiled/react/index.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$tooltip$2f$root$2f$TooltipRootContext$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/tooltip/root/TooltipRootContext.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$tooltip$2f$positioner$2f$TooltipPositionerContext$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/tooltip/positioner/TooltipPositionerContext.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$useRenderElement$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/utils/useRenderElement.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$tooltip$2f$viewport$2f$TooltipViewportCssVars$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/tooltip/viewport/TooltipViewportCssVars.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$usePopupViewport$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/utils/usePopupViewport.js [app-client] (ecmascript)");
'use client';
;
;
;
;
;
;
const stateAttributesMapping = {
    activationDirection: (value)=>value ? {
            'data-activation-direction': value
        } : null
};
const TooltipViewport = /*#__PURE__*/ __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$compiled$2f$react$2f$index$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["forwardRef"](function TooltipViewport(componentProps, forwardedRef) {
    const { render, className, children, ...elementProps } = componentProps;
    const store = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$tooltip$2f$root$2f$TooltipRootContext$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useTooltipRootContext"])();
    const positioner = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$tooltip$2f$positioner$2f$TooltipPositionerContext$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useTooltipPositionerContext"])();
    const instantType = store.useState('instantType');
    const { children: childrenToRender, state: viewportState } = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$usePopupViewport$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["usePopupViewport"])({
        store,
        side: positioner.side,
        cssVars: __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$tooltip$2f$viewport$2f$TooltipViewportCssVars$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["TooltipViewportCssVars"],
        children
    });
    const state = {
        activationDirection: viewportState.activationDirection,
        transitioning: viewportState.transitioning,
        instant: instantType
    };
    return (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$useRenderElement$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["useRenderElement"])('div', componentProps, {
        state,
        ref: forwardedRef,
        props: [
            elementProps,
            {
                children: childrenToRender
            }
        ],
        stateAttributesMapping
    });
});
if ("TURBOPACK compile-time truthy", 1) TooltipViewport.displayName = "TooltipViewport";
}),
"[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/tooltip/store/TooltipHandle.js [app-client] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "TooltipHandle",
    ()=>TooltipHandle,
    "createTooltipHandle",
    ()=>createTooltipHandle
]);
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$build$2f$polyfills$2f$process$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = /*#__PURE__*/ __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/build/polyfills/process.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$tooltip$2f$store$2f$TooltipStore$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/tooltip/store/TooltipStore.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$createBaseUIEventDetails$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/utils/createBaseUIEventDetails.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$reason$2d$parts$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$export__$2a$__as__REASONS$3e$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/utils/reason-parts.js [app-client] (ecmascript) <export * as REASONS>");
;
;
;
;
class TooltipHandle {
    /**
   * Internal store holding the tooltip state.
   * @internal
   */ constructor(){
        this.store = new __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$tooltip$2f$store$2f$TooltipStore$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["TooltipStore"]();
    }
    /**
   * Opens the tooltip and associates it with the trigger with the given ID.
   * The trigger must be a Tooltip.Trigger component with this handle passed as a prop.
   *
   * This method should only be called in an event handler or an effect (not during rendering).
   *
   * @param triggerId ID of the trigger to associate with the tooltip.
   */ open(triggerId) {
        const triggerElement = triggerId ? this.store.context.triggerElements.getById(triggerId) : undefined;
        if (triggerId && !triggerElement) {
            throw new Error(("TURBOPACK compile-time truthy", 1) ? `Base UI: TooltipHandle.open: No trigger found with id "${triggerId}".` : "TURBOPACK unreachable");
        }
        this.store.setOpen(true, (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$createBaseUIEventDetails$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["createChangeEventDetails"])(__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$reason$2d$parts$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$export__$2a$__as__REASONS$3e$__["REASONS"].imperativeAction, undefined, triggerElement));
    }
    /**
   * Closes the tooltip.
   */ close() {
        this.store.setOpen(false, (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$createBaseUIEventDetails$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["createChangeEventDetails"])(__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$utils$2f$reason$2d$parts$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$export__$2a$__as__REASONS$3e$__["REASONS"].imperativeAction, undefined, undefined));
    }
    /**
   * Indicates whether the tooltip is currently open.
   */ get isOpen() {
        return this.store.state.open;
    }
}
function createTooltipHandle() {
    return new TooltipHandle();
}
}),
"[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/tooltip/index.parts.js [app-client] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "Arrow",
    ()=>__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$tooltip$2f$arrow$2f$TooltipArrow$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["TooltipArrow"],
    "Handle",
    ()=>__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$tooltip$2f$store$2f$TooltipHandle$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["TooltipHandle"],
    "Popup",
    ()=>__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$tooltip$2f$popup$2f$TooltipPopup$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["TooltipPopup"],
    "Portal",
    ()=>__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$tooltip$2f$portal$2f$TooltipPortal$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["TooltipPortal"],
    "Positioner",
    ()=>__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$tooltip$2f$positioner$2f$TooltipPositioner$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["TooltipPositioner"],
    "Provider",
    ()=>__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$tooltip$2f$provider$2f$TooltipProvider$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["TooltipProvider"],
    "Root",
    ()=>__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$tooltip$2f$root$2f$TooltipRoot$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["TooltipRoot"],
    "Trigger",
    ()=>__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$tooltip$2f$trigger$2f$TooltipTrigger$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["TooltipTrigger"],
    "Viewport",
    ()=>__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$tooltip$2f$viewport$2f$TooltipViewport$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["TooltipViewport"],
    "createHandle",
    ()=>__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$tooltip$2f$store$2f$TooltipHandle$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__["createTooltipHandle"]
]);
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$tooltip$2f$index$2e$parts$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__$3c$locals$3e$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/tooltip/index.parts.js [app-client] (ecmascript) <locals>");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$tooltip$2f$root$2f$TooltipRoot$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/tooltip/root/TooltipRoot.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$tooltip$2f$trigger$2f$TooltipTrigger$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/tooltip/trigger/TooltipTrigger.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$tooltip$2f$portal$2f$TooltipPortal$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/tooltip/portal/TooltipPortal.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$tooltip$2f$positioner$2f$TooltipPositioner$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/tooltip/positioner/TooltipPositioner.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$tooltip$2f$popup$2f$TooltipPopup$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/tooltip/popup/TooltipPopup.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$tooltip$2f$arrow$2f$TooltipArrow$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/tooltip/arrow/TooltipArrow.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$tooltip$2f$provider$2f$TooltipProvider$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/tooltip/provider/TooltipProvider.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$tooltip$2f$viewport$2f$TooltipViewport$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/tooltip/viewport/TooltipViewport.js [app-client] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$tooltip$2f$store$2f$TooltipHandle$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/tooltip/store/TooltipHandle.js [app-client] (ecmascript)");
}),
"[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/tooltip/index.parts.js [app-client] (ecmascript) <export * as Tooltip>", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "Tooltip",
    ()=>__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$tooltip$2f$index$2e$parts$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__
]);
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f40$base$2d$ui$2b$react$40$1$2e$2$2e$0$2b$0ea9ec2a211d4613$2f$node_modules$2f40$base$2d$ui$2f$react$2f$esm$2f$tooltip$2f$index$2e$parts$2e$js__$5b$app$2d$client$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/@base-ui+react@1.2.0+0ea9ec2a211d4613/node_modules/@base-ui/react/esm/tooltip/index.parts.js [app-client] (ecmascript)");
}),
]);

//# sourceMappingURL=15baa_%40base-ui_react_esm_9af1d739._.js.map