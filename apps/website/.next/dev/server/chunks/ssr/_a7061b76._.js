module.exports = [
"[project]/apps/website/src/app/(landing)/blog/layout.tsx [app-rsc] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "default",
    ()=>__TURBOPACK__default__export__
]);
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$server$2f$route$2d$modules$2f$app$2d$page$2f$vendored$2f$rsc$2f$react$2d$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/server/route-modules/app-page/vendored/rsc/react-jsx-dev-runtime.js [app-rsc] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$basehub$40$9$2e$5$2e$3$2b$7111ff09de72ce5c$2f$node_modules$2f$basehub$2f$dist$2f$next$2f$toolbar$2f$index$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/basehub@9.5.3+7111ff09de72ce5c/node_modules/basehub/dist/next/toolbar/index.js [app-rsc] (ecmascript)");
;
;
const Layout = ({ children })=>/*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$server$2f$route$2d$modules$2f$app$2d$page$2f$vendored$2f$rsc$2f$react$2d$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["jsxDEV"])(__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$server$2f$route$2d$modules$2f$app$2d$page$2f$vendored$2f$rsc$2f$react$2d$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["Fragment"], {
        children: [
            children,
            /*#__PURE__*/ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$server$2f$route$2d$modules$2f$app$2d$page$2f$vendored$2f$rsc$2f$react$2d$jsx$2d$dev$2d$runtime$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["jsxDEV"])(__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$basehub$40$9$2e$5$2e$3$2b$7111ff09de72ce5c$2f$node_modules$2f$basehub$2f$dist$2f$next$2f$toolbar$2f$index$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["Toolbar"], {}, void 0, false, {
                fileName: "[project]/apps/website/src/app/(landing)/blog/layout.tsx",
                lineNumber: 11,
                columnNumber: 5
            }, ("TURBOPACK compile-time value", void 0))
        ]
    }, void 0, true);
const __TURBOPACK__default__export__ = Layout;
}),
"[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/build/webpack/loaders/next-flight-loader/server-reference.js [app-rsc] (ecmascript)", ((__turbopack_context__, module, exports) => {
"use strict";

/* eslint-disable import/no-extraneous-dependencies */ Object.defineProperty(exports, "__esModule", {
    value: true
});
Object.defineProperty(exports, "registerServerReference", {
    enumerable: true,
    get: function() {
        return _server.registerServerReference;
    }
});
const _server = __turbopack_context__.r("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/server/route-modules/app-page/vendored/rsc/react-server-dom-turbopack-server.js [app-rsc] (ecmascript)"); //# sourceMappingURL=server-reference.js.map
}),
"[project]/node_modules/.bun/basehub@9.5.3+7111ff09de72ce5c/node_modules/basehub/dist/chunk-FQNSFYPW.js [app-rsc] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "GenqlError",
    ()=>GenqlError
]);
// src/genql/runtime/_error.ts
var GenqlError = class extends Error {
    constructor(errors, data, extraWarnings){
        let message = Array.isArray(errors) ? errors.map((x)=>x?.message || "").join("\n") : "";
        if (!message) {
            message = "GraphQL error";
        }
        super(message);
        this.errors = [];
        this.errors = errors;
        this.data = data;
        this.errorsStringified = JSON.stringify(errors, null, 2).slice(0, 1e3);
        this.extraWarnings = extraWarnings;
    }
};
;
}),
"[project]/node_modules/.bun/basehub@9.5.3+7111ff09de72ce5c/node_modules/basehub/dist/chunk-LPXQCVN6.js [app-rsc] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "QueryBatcher",
    ()=>QueryBatcher
]);
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$basehub$40$9$2e$5$2e$3$2b$7111ff09de72ce5c$2f$node_modules$2f$basehub$2f$dist$2f$chunk$2d$FQNSFYPW$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/basehub@9.5.3+7111ff09de72ce5c/node_modules/basehub/dist/chunk-FQNSFYPW.js [app-rsc] (ecmascript)");
;
// src/genql/runtime/_batcher.ts
function dispatchQueueBatch(client, queue) {
    let batchedQuery = queue.map((item)=>item.request);
    if (batchedQuery.length === 1) {
        batchedQuery = batchedQuery[0];
    }
    (()=>{
        try {
            return client.fetcher(batchedQuery);
        } catch (e) {
            return Promise.reject(e);
        }
    })().then((responses)=>{
        if (queue.length === 1 && !Array.isArray(responses)) {
            if (responses.errors && responses.errors.length) {
                queue[0]?.reject(new __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$basehub$40$9$2e$5$2e$3$2b$7111ff09de72ce5c$2f$node_modules$2f$basehub$2f$dist$2f$chunk$2d$FQNSFYPW$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["GenqlError"](responses.errors, responses.data));
                return;
            }
            queue[0]?.resolve(responses);
            return;
        } else if (responses.length !== queue.length) {
            throw new Error("response length did not match query length");
        }
        for(let i = 0; i < queue.length; i++){
            if (responses[i].errors && responses[i].errors.length) {
                queue[i]?.reject(new __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$basehub$40$9$2e$5$2e$3$2b$7111ff09de72ce5c$2f$node_modules$2f$basehub$2f$dist$2f$chunk$2d$FQNSFYPW$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["GenqlError"](responses[i].errors, responses[i].data));
            } else {
                queue[i]?.resolve(responses[i]);
            }
        }
    }).catch((e)=>{
        for(let i = 0; i < queue.length; i++){
            queue[i]?.reject(e);
        }
    });
}
function dispatchQueue(client, options) {
    const queue = client._queue;
    const maxBatchSize = options.maxBatchSize || 0;
    client._queue = [];
    if (maxBatchSize > 0 && maxBatchSize < queue.length) {
        for(let i = 0; i < queue.length / maxBatchSize; i++){
            dispatchQueueBatch(client, queue.slice(i * maxBatchSize, (i + 1) * maxBatchSize));
        }
    } else {
        dispatchQueueBatch(client, queue);
    }
}
var QueryBatcher = class _QueryBatcher {
    constructor(fetcher, { batchInterval = 16, shouldBatch = true, maxBatchSize = 0 } = {}){
        this.fetcher = fetcher;
        this._options = {
            batchInterval,
            shouldBatch,
            maxBatchSize
        };
        this._queue = [];
    }
    /**
   * Fetch will send a graphql request and return the parsed json.
   * @param {string}      query          - the graphql query.
   * @param {Variables}   variables      - any variables you wish to inject as key/value pairs.
   * @param {[string]}    operationName  - the graphql operationName.
   * @param {Options}     overrides      - the client options overrides.
   *
   * @return {promise} resolves to parsed json of server response
   *
   * @example
   * client.fetch(`
   *    query getHuman($id: ID!) {
   *      human(id: $id) {
   *        name
   *        height
   *      }
   *    }
   * `, { id: "1001" }, 'getHuman')
   *    .then(human => {
   *      // do something with human
   *      console.log(human);
   *    });
   */ fetch(query, variables, operationName, overrides = {}) {
        const request = {
            query
        };
        const options = Object.assign({}, this._options, overrides);
        if (variables) {
            request.variables = variables;
        }
        if (operationName) {
            request.operationName = operationName;
        }
        const promise = new Promise((resolve, reject)=>{
            this._queue.push({
                request,
                resolve,
                reject
            });
            if (this._queue.length === 1) {
                if (options.shouldBatch) {
                    setTimeout(()=>dispatchQueue(this, options), options.batchInterval);
                } else {
                    dispatchQueue(this, options);
                }
            }
        });
        return promise;
    }
    /**
   * Fetch will send a graphql request and return the parsed json.
   * @param {string}      query          - the graphql query.
   * @param {Variables}   variables      - any variables you wish to inject as key/value pairs.
   * @param {[string]}    operationName  - the graphql operationName.
   * @param {Options}     overrides      - the client options overrides.
   *
   * @return {Promise<Array<Result>>} resolves to parsed json of server response
   *
   * @example
   * client.forceFetch(`
   *    query getHuman($id: ID!) {
   *      human(id: $id) {
   *        name
   *        height
   *      }
   *    }
   * `, { id: "1001" }, 'getHuman')
   *    .then(human => {
   *      // do something with human
   *      console.log(human);
   *    });
   */ forceFetch(query, variables, operationName, overrides = {}) {
        const request = {
            query
        };
        const options = Object.assign({}, this._options, overrides, {
            shouldBatch: false
        });
        if (variables) {
            request.variables = variables;
        }
        if (operationName) {
            request.operationName = operationName;
        }
        const promise = new Promise((resolve, reject)=>{
            const client = new _QueryBatcher(this.fetcher, this._options);
            client._queue = [
                {
                    request,
                    resolve,
                    reject
                }
            ];
            dispatchQueue(client, options);
        });
        return promise;
    }
};
;
}),
"[project]/node_modules/.bun/basehub@9.5.3+7111ff09de72ce5c/node_modules/basehub/dist/chunk-LJ34TIMX.js [app-rsc] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "createFetcher",
    ()=>createFetcher
]);
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$basehub$40$9$2e$5$2e$3$2b$7111ff09de72ce5c$2f$node_modules$2f$basehub$2f$dist$2f$chunk$2d$LPXQCVN6$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/basehub@9.5.3+7111ff09de72ce5c/node_modules/basehub/dist/chunk-LPXQCVN6.js [app-rsc] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$basehub$40$9$2e$5$2e$3$2b$7111ff09de72ce5c$2f$node_modules$2f$basehub$2f$dist$2f$chunk$2d$FQNSFYPW$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/basehub@9.5.3+7111ff09de72ce5c/node_modules/basehub/dist/chunk-FQNSFYPW.js [app-rsc] (ecmascript)");
;
;
// src/genql/runtime/_fetcher.ts
var DEFAULT_BATCH_OPTIONS = {
    maxBatchSize: 10,
    batchInterval: 40
};
var createFetcher = ({ url, headers = {}, fetcher, fetch: _fetch, batch = false, ...rest })=>{
    if (!url && !fetcher) {
        throw new Error("url or fetcher is required");
    }
    fetcher = fetcher || (async (body, extraFetchOptions)=>{
        let headersObject = typeof headers == "function" ? await headers() : headers;
        headersObject = headersObject || {};
        if (typeof fetch === "undefined" && !_fetch) {
            throw new Error("Global `fetch` function is not available, pass a fetch polyfill to Genql `createClient`");
        }
        const fetchImpl = _fetch || fetch;
        if (extraFetchOptions?.headers) {
            headersObject = {
                ...headersObject,
                ...extraFetchOptions.headers
            };
            delete extraFetchOptions.headers;
        }
        const res = await fetchImpl(url, {
            headers: {
                "Content-Type": "application/json",
                ...headersObject
            },
            method: "POST",
            body: JSON.stringify(body),
            ...rest,
            ...extraFetchOptions
        });
        if (!res.ok) {
            throw new Error(`${res.statusText}: ${await res.text()}`);
        }
        const json = await res.json();
        return json;
    });
    if (!batch) {
        return async (body, extraFetchOptions)=>{
            const json = await fetcher(body, extraFetchOptions);
            if (Array.isArray(json)) {
                return json.map((json2)=>{
                    if (json2?.errors?.length) {
                        throw new __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$basehub$40$9$2e$5$2e$3$2b$7111ff09de72ce5c$2f$node_modules$2f$basehub$2f$dist$2f$chunk$2d$FQNSFYPW$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["GenqlError"](json2.errors || [], json2.data);
                    }
                    return json2.data;
                });
            } else {
                if (json?.errors?.length) {
                    throw new __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$basehub$40$9$2e$5$2e$3$2b$7111ff09de72ce5c$2f$node_modules$2f$basehub$2f$dist$2f$chunk$2d$FQNSFYPW$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["GenqlError"](json.errors || [], json.data);
                }
                return json.data;
            }
        };
    }
    const batcher = new __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$basehub$40$9$2e$5$2e$3$2b$7111ff09de72ce5c$2f$node_modules$2f$basehub$2f$dist$2f$chunk$2d$LPXQCVN6$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["QueryBatcher"](async (batchedQuery, extraFetchOptions)=>{
        const json = await fetcher(batchedQuery, extraFetchOptions);
        return json;
    }, batch === true ? DEFAULT_BATCH_OPTIONS : batch);
    return async ({ query, variables })=>{
        const json = await batcher.fetch(query, variables);
        if (json?.data) {
            return json.data;
        }
        throw new Error("Genql batch fetcher returned unexpected result " + JSON.stringify(json));
    };
};
;
}),
"[project]/node_modules/.bun/basehub@9.5.3+7111ff09de72ce5c/node_modules/basehub/dist/chunk-Z6OIZIMQ.js [app-rsc] (ecmascript)", ((__turbopack_context__) => {
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
"[project]/node_modules/.bun/basehub@9.5.3+7111ff09de72ce5c/node_modules/basehub/dist/chunk-XP4IVFBD.js [app-rsc] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "generateGraphqlOperation",
    ()=>generateGraphqlOperation,
    "getFieldFromPath",
    ()=>getFieldFromPath
]);
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$basehub$40$9$2e$5$2e$3$2b$7111ff09de72ce5c$2f$node_modules$2f$basehub$2f$dist$2f$chunk$2d$Z6OIZIMQ$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/basehub@9.5.3+7111ff09de72ce5c/node_modules/basehub/dist/chunk-Z6OIZIMQ.js [app-rsc] (ecmascript)");
;
// src/genql/runtime/_generate-graphql-operation.ts
var parseRequest = (operation, request, ctx, path, options)=>{
    if (typeof request === "object" && "__args" in request) {
        const args = request.__args;
        const fields = {
            ...request
        };
        delete fields.__args;
        const argNames = Object.keys(args);
        if (argNames.length === 0) {
            return parseRequest(operation, fields, ctx, path, options);
        }
        const argsThatShouldNotBeEnums = [
            // image processing args
            "anim",
            "background",
            "border",
            "compression",
            "fit",
            "gamma",
            "gravity",
            "metadata",
            "rotate",
            "sharpen",
            "trim",
            // signedUrl args
            "fileName",
            // transaction args
            "autoCommit",
            "authorId",
            // in _structure it's an enum, in an image it's a string
            ...[
                "_structure"
            ].includes(path?.[0] || "") ? [] : [
                "format"
            ]
        ];
        const objectsThatShouldHoldEnums = [
            "variants"
        ];
        const argStrings = argNames.map((argName)=>{
            let value = args[argName];
            if (typeof value === "object") {
                value = JSON.stringify(value);
                const stringifyObject = operation === "mutation" && [
                    "transaction",
                    "transactionAsync"
                ].includes(path?.[0] || "") && argName === "data";
                if (stringifyObject) {
                    value = JSON.stringify(value);
                } else if (objectsThatShouldHoldEnums.includes(argName)) {
                    value = value.replace(/"([^"]+)":/g, "$1:");
                    value = value.replace(/:"([^"]*)"/g, ":$1");
                } else {
                    value = value.replace(/"([^"]+)":/g, "$1:");
                }
            } else if (typeof value === "string" && argsThatShouldNotBeEnums.includes(argName)) {
                value = JSON.stringify(value);
            }
            return `${argName}:${value}`;
        });
        return `(${argStrings})${parseRequest(operation, fields, ctx, path, options)}`;
    } else if (typeof request === "object" && Object.keys(request).length > 0) {
        const fields = request;
        const fieldNames = Object.keys(fields).filter((k)=>Boolean(fields[k]));
        const fieldsSelection = fieldNames.filter((f)=>![
                "__scalar",
                "__name",
                "__fragmentOn"
            ].includes(f)).map((f)=>{
            if (f.startsWith("on_")) {
                ctx.fragmentCounter++;
                const implementationFragment = `f${ctx.fragmentCounter}`;
                const parsed = parseRequest(operation, fields[f], ctx, [
                    ...path,
                    f
                ], {
                    ...options,
                    aliasPrefix: implementationFragment
                });
                const typeMatch = f.match(/^on_(.+)/);
                if (!typeMatch || !typeMatch[1]) {
                    throw new Error("match failed");
                }
                ctx.fragments.push(`fragment ${implementationFragment} on ${typeMatch[1]}${parsed}`);
                return `...${implementationFragment}`;
            } else {
                const field = fields?.[f];
                if (!field) return "";
                if (typeof field === "boolean" || typeof field === "number" || typeof field === "string") {
                    return `${options?.aliasPrefix ? `${options.aliasPrefix}${__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$basehub$40$9$2e$5$2e$3$2b$7111ff09de72ce5c$2f$node_modules$2f$basehub$2f$dist$2f$chunk$2d$Z6OIZIMQ$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["aliasSeparator"]}${f}: ` : ""}${f}`;
                }
                const parsed = parseRequest(operation, fields[f], ctx, [
                    ...path,
                    f
                ], options);
                if (!parsed) {
                    return `${options?.aliasPrefix ? `${options.aliasPrefix}${__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$basehub$40$9$2e$5$2e$3$2b$7111ff09de72ce5c$2f$node_modules$2f$basehub$2f$dist$2f$chunk$2d$Z6OIZIMQ$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["aliasSeparator"]}${f}: ` : ""}${f}{__typename}`;
                }
                return `${options?.aliasPrefix ? `${options.aliasPrefix}${__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$basehub$40$9$2e$5$2e$3$2b$7111ff09de72ce5c$2f$node_modules$2f$basehub$2f$dist$2f$chunk$2d$Z6OIZIMQ$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["aliasSeparator"]}${f}: ` : ""}${f}${parsed}`;
            }
        }).filter(Boolean).join(",");
        return fieldsSelection ? `{${fieldsSelection}}` : "";
    } else {
        return "";
    }
};
var generateGraphqlOperation = (operation, fields)=>{
    const ctx = {
        fragmentCounter: 0,
        fragments: []
    };
    const result = parseRequest(operation, fields, ctx, []);
    const operationName = fields?.__name || "";
    const q = {
        query: [
            `${operation} ${operationName}${result}`,
            ...ctx.fragments
        ].join(","),
        variables: {},
        ...operationName ? {
            operationName: operationName.toString()
        } : {}
    };
    return q;
};
var getFieldFromPath = (root, path)=>{
    let current;
    if (!root) throw new Error("root type is not provided");
    if (path.length === 0) throw new Error(`path is empty`);
    path.forEach((f)=>{
        const type = current ? current.type : root;
        if (!type.fields) {
            throw new Error(`type \`${type.name}\` does not have fields`);
        }
        const possibleTypes = Object.keys(type.fields).filter((i)=>i.startsWith("on_")).reduce((types, fieldName)=>{
            const field2 = type.fields && type.fields[fieldName];
            if (field2) types.push(field2.type);
            return types;
        }, [
            type
        ]);
        let field = null;
        possibleTypes.forEach((type2)=>{
            const found = type2.fields && type2.fields[f];
            if (found) field = found;
        });
        if (!field) {
            throw new Error(`type \`${type.name}\` does not have a field \`${f}\``);
        }
        current = field;
    });
    return current;
};
;
}),
"[project]/node_modules/.bun/basehub@9.5.3+7111ff09de72ce5c/node_modules/basehub/dist/chunk-VGXQV2T4.js [app-rsc] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "createClient",
    ()=>createClient
]);
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$basehub$40$9$2e$5$2e$3$2b$7111ff09de72ce5c$2f$node_modules$2f$basehub$2f$dist$2f$chunk$2d$LJ34TIMX$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/basehub@9.5.3+7111ff09de72ce5c/node_modules/basehub/dist/chunk-LJ34TIMX.js [app-rsc] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$basehub$40$9$2e$5$2e$3$2b$7111ff09de72ce5c$2f$node_modules$2f$basehub$2f$dist$2f$chunk$2d$XP4IVFBD$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/basehub@9.5.3+7111ff09de72ce5c/node_modules/basehub/dist/chunk-XP4IVFBD.js [app-rsc] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$basehub$40$9$2e$5$2e$3$2b$7111ff09de72ce5c$2f$node_modules$2f$basehub$2f$dist$2f$chunk$2d$Z6OIZIMQ$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/basehub@9.5.3+7111ff09de72ce5c/node_modules/basehub/dist/chunk-Z6OIZIMQ.js [app-rsc] (ecmascript)");
;
;
;
// src/genql/runtime/_create-client.ts
var createClient = ({ getExtraFetchOptions, ...options })=>{
    return {
        query: async (request)=>{
            const body = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$basehub$40$9$2e$5$2e$3$2b$7111ff09de72ce5c$2f$node_modules$2f$basehub$2f$dist$2f$chunk$2d$XP4IVFBD$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["generateGraphqlOperation"])("query", request);
            const extraFetchOptions = await getExtraFetchOptions?.("query", body, request);
            const fetcher = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$basehub$40$9$2e$5$2e$3$2b$7111ff09de72ce5c$2f$node_modules$2f$basehub$2f$dist$2f$chunk$2d$LJ34TIMX$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["createFetcher"])({
                ...options,
                ...extraFetchOptions
            });
            const result = await fetcher(body, extraFetchOptions);
            return (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$basehub$40$9$2e$5$2e$3$2b$7111ff09de72ce5c$2f$node_modules$2f$basehub$2f$dist$2f$chunk$2d$Z6OIZIMQ$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["replaceSystemAliases"])(result);
        },
        mutation: async (request)=>{
            const body = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$basehub$40$9$2e$5$2e$3$2b$7111ff09de72ce5c$2f$node_modules$2f$basehub$2f$dist$2f$chunk$2d$XP4IVFBD$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["generateGraphqlOperation"])("mutation", request);
            const extraFetchOptions = await getExtraFetchOptions?.("mutation", body, request);
            const fetcher = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$basehub$40$9$2e$5$2e$3$2b$7111ff09de72ce5c$2f$node_modules$2f$basehub$2f$dist$2f$chunk$2d$LJ34TIMX$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["createFetcher"])({
                ...options,
                ...extraFetchOptions
            });
            const result = await fetcher(body, extraFetchOptions);
            return (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$basehub$40$9$2e$5$2e$3$2b$7111ff09de72ce5c$2f$node_modules$2f$basehub$2f$dist$2f$chunk$2d$Z6OIZIMQ$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["replaceSystemAliases"])(result);
        }
    };
};
createClient.replaceSystemAliases = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$basehub$40$9$2e$5$2e$3$2b$7111ff09de72ce5c$2f$node_modules$2f$basehub$2f$dist$2f$chunk$2d$Z6OIZIMQ$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["replaceSystemAliases"];
;
}),
"[project]/node_modules/.bun/basehub@9.5.3+7111ff09de72ce5c/node_modules/basehub/dist/chunk-N7COQVBX.js [app-rsc] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "getGitEnv",
    ()=>getGitEnv
]);
// src/bin/util/get-git-env.ts
var getGitEnv = async (_opts)=>{
    const execSyncSafe = async (command)=>{
        try {
            const execSync = await __turbopack_context__.A("[externals]/child_process [external] (child_process, cjs, async loader)").then((m)=>m.execSync);
            return execSync(command, {
                stdio: "pipe"
            }).toString().trim();
        } catch (error) {
            return "";
        }
    };
    const gitBranch = process.env.VERCEL_GIT_COMMIT_REF || process.env.BRANCH || process.env.RENDER_GIT_BRANCH || process.env.GIT_BRANCH || process.env.CF_PAGES_BRANCH || await execSyncSafe("git symbolic-ref --short HEAD") || await execSyncSafe("git rev-parse --abbrev-ref HEAD");
    const gitCommitSHA = process.env.VERCEL_GIT_COMMIT_SHA || process.env.COMMIT_REF || process.env.RENDER_GIT_COMMIT || process.env.COMMIT_SHA || process.env.CF_PAGES_COMMIT_SHA || await execSyncSafe("git rev-parse HEAD");
    const gitBranchDeploymentURL = process.env.VERCEL_BRANCH_URL || process.env.DEPLOY_PRIME_URL || process.env.CF_PAGES_URL || null;
    const productionDeploymentURL = process.env.VERCEL_PROJECT_PRODUCTION_URL || process.env.URL || process.env.RENDER_EXTERNAL_URL || null;
    return {
        gitBranch,
        gitCommitSHA,
        gitBranchDeploymentURL,
        productionDeploymentURL
    };
};
;
}),
"[project]/node_modules/.bun/basehub@9.5.3+7111ff09de72ce5c/node_modules/basehub/dist/chunk-UVZEJLNP.js [app-rsc] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "hashObject",
    ()=>hashObject
]);
// src/bin/util/hash.ts
function hashObject(obj) {
    const sortObjectKeys = (obj2)=>{
        if (!isObjectAsWeCommonlyCallIt(obj2)) return obj2;
        return Object.keys(obj2).sort().reduce((acc, key)=>{
            acc[key] = obj2[key];
            return acc;
        }, {});
    };
    const recursiveSortObjectKeys = (obj2)=>{
        const sortedObj2 = sortObjectKeys(obj2);
        if (!isObjectAsWeCommonlyCallIt(sortedObj2)) return sortedObj2;
        Object.keys(sortedObj2).forEach((key)=>{
            if (isObjectAsWeCommonlyCallIt(sortedObj2[key])) {
                sortedObj2[key] = recursiveSortObjectKeys(sortedObj2[key]);
            } else if (Array.isArray(sortedObj2[key])) {
                sortedObj2[key] = sortedObj2[key].map((item)=>{
                    if (isObjectAsWeCommonlyCallIt(item)) {
                        return recursiveSortObjectKeys(item);
                    } else {
                        return item;
                    }
                });
            }
        });
        return sortedObj2;
    };
    const isObjectAsWeCommonlyCallIt = (obj2)=>{
        return Object.prototype.toString.call(obj2) === "[object Object]";
    };
    const sortedObj = recursiveSortObjectKeys(obj);
    const str = JSON.stringify(sortedObj);
    let hash = 0;
    for(let i = 0, len = str.length; i < len; i++){
        const chr = str.charCodeAt(i);
        hash = (hash << 5) - hash + chr;
        hash |= 0;
    }
    return Math.abs(hash).toString();
}
;
}),
"[project]/node_modules/.bun/basehub@9.5.3+7111ff09de72ce5c/node_modules/basehub/dist/chunk-2DLPZSMQ.js [app-rsc] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "version",
    ()=>version
]);
// src/version.ts
var version = "9.5.3";
;
}),
"[project]/node_modules/.bun/basehub@9.5.3+7111ff09de72ce5c/node_modules/basehub/dist/chunk-A7NXCT6I.js [app-rsc] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "isBolt",
    ()=>isBolt,
    "isV0",
    ()=>isV0,
    "isV0OrBolt",
    ()=>isV0OrBolt
]);
// src/vibe.ts
var isV0 = ()=>{
    try {
        return(// eslint-disable-next-line turbo/no-undeclared-env-vars
        process.env.VERCEL_URL?.includes(".lite.vusercontent.net") || // @ts-ignore
        process.env.NEXT_PUBLIC_VERCEL_URL?.includes(".lite.vusercontent.net"));
    } catch (err) {
        return false;
    }
};
var isBolt = ()=>{
    try {
        return process.env.SHELL === "/bin/jsh";
    } catch (err) {
        return false;
    }
};
var isV0OrBolt = ()=>{
    return isV0() || isBolt();
};
;
}),
"[project]/node_modules/.bun/basehub@9.5.3+7111ff09de72ce5c/node_modules/basehub/dist/chunk-R45FBMZ7.js [app-rsc] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "basehub",
    ()=>basehub,
    "basehubAPIOrigin",
    ()=>basehubAPIOrigin,
    "cacheTagFromQuery",
    ()=>cacheTagFromQuery,
    "generateMutationOp",
    ()=>generateMutationOp,
    "generateQueryOp",
    ()=>generateQueryOp,
    "getGlobalConfig",
    ()=>getGlobalConfig,
    "getStuffFromEnv",
    ()=>getStuffFromEnv,
    "resolveRef",
    ()=>resolveRef,
    "setGlobalConfig",
    ()=>setGlobalConfig
]);
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$basehub$40$9$2e$5$2e$3$2b$7111ff09de72ce5c$2f$node_modules$2f$basehub$2f$dist$2f$chunk$2d$VGXQV2T4$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/basehub@9.5.3+7111ff09de72ce5c/node_modules/basehub/dist/chunk-VGXQV2T4.js [app-rsc] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$basehub$40$9$2e$5$2e$3$2b$7111ff09de72ce5c$2f$node_modules$2f$basehub$2f$dist$2f$chunk$2d$XP4IVFBD$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/basehub@9.5.3+7111ff09de72ce5c/node_modules/basehub/dist/chunk-XP4IVFBD.js [app-rsc] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$basehub$40$9$2e$5$2e$3$2b$7111ff09de72ce5c$2f$node_modules$2f$basehub$2f$dist$2f$chunk$2d$N7COQVBX$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/basehub@9.5.3+7111ff09de72ce5c/node_modules/basehub/dist/chunk-N7COQVBX.js [app-rsc] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$basehub$40$9$2e$5$2e$3$2b$7111ff09de72ce5c$2f$node_modules$2f$basehub$2f$dist$2f$chunk$2d$UVZEJLNP$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/basehub@9.5.3+7111ff09de72ce5c/node_modules/basehub/dist/chunk-UVZEJLNP.js [app-rsc] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$basehub$40$9$2e$5$2e$3$2b$7111ff09de72ce5c$2f$node_modules$2f$basehub$2f$dist$2f$chunk$2d$2DLPZSMQ$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/basehub@9.5.3+7111ff09de72ce5c/node_modules/basehub/dist/chunk-2DLPZSMQ.js [app-rsc] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$basehub$40$9$2e$5$2e$3$2b$7111ff09de72ce5c$2f$node_modules$2f$basehub$2f$dist$2f$chunk$2d$A7NXCT6I$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/basehub@9.5.3+7111ff09de72ce5c/node_modules/basehub/dist/chunk-A7NXCT6I.js [app-rsc] (ecmascript)");
;
;
;
;
;
;
// src/index.ts
function cacheTagFromQuery({ query, sdkBuildId }) {
    const result = "basehub-" + (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$basehub$40$9$2e$5$2e$3$2b$7111ff09de72ce5c$2f$node_modules$2f$basehub$2f$dist$2f$chunk$2d$UVZEJLNP$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["hashObject"])({
        ...query,
        sdkBuildId
    });
    return result;
}
var basehub = (options)=>{
    if (!options) {
        options = {};
    }
    options.getExtraFetchOptions = async (op, _body, originalRequest)=>{
        const { url, headers, sdkBuildId, draft } = await getStuffFromEnv(options);
        const extra = {
            url,
            headers: {
                ...headers
            }
        };
        if (op !== "query") return extra;
        let isNextjs = false;
        if (draft) {
            extra.next = {
                revalidate: void 0
            };
            extra.cache = "no-store";
        }
        if (draft) return extra;
        if (!(0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$basehub$40$9$2e$5$2e$3$2b$7111ff09de72ce5c$2f$node_modules$2f$basehub$2f$dist$2f$chunk$2d$A7NXCT6I$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["isV0OrBolt"])() && typeof options?.next === "undefined") {
            try {
                isNextjs = !!await __turbopack_context__.A("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/headers.js [app-rsc] (ecmascript, async loader)");
            } catch (error) {}
            if (isNextjs) {
                const cacheTag = cacheTagFromQuery({
                    query: originalRequest,
                    sdkBuildId
                });
                extra.next = {
                    tags: [
                        cacheTag
                    ]
                };
                extra.headers = {
                    ...extra.headers,
                    ["x-basehub-cache-tag"]: cacheTag
                };
            }
        }
        return extra;
    };
    return (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$basehub$40$9$2e$5$2e$3$2b$7111ff09de72ce5c$2f$node_modules$2f$basehub$2f$dist$2f$chunk$2d$VGXQV2T4$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["createClient"])(options);
};
var generateQueryOp = function(fields) {
    return (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$basehub$40$9$2e$5$2e$3$2b$7111ff09de72ce5c$2f$node_modules$2f$basehub$2f$dist$2f$chunk$2d$XP4IVFBD$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["generateGraphqlOperation"])("query", fields);
};
var generateMutationOp = function(fields) {
    return (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$basehub$40$9$2e$5$2e$3$2b$7111ff09de72ce5c$2f$node_modules$2f$basehub$2f$dist$2f$chunk$2d$XP4IVFBD$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["generateGraphqlOperation"])("mutation", fields);
};
var BASEHUB_CONFIG = Symbol.for("basehub.config");
function setGlobalConfig(config) {
    globalThis[BASEHUB_CONFIG] = config;
}
function getGlobalConfig() {
    return globalThis[BASEHUB_CONFIG] ?? null;
}
// src/bin/util/get-stuff-from-env.ts
var basehubAPIOrigin = "https://api.basehub.com";
var defaultEnvVarPrefix = "BASEHUB";
var DEFAULT_API_VERSION = "4";
var getStuffFromEnv = async (options)=>{
    if (!options) {
        options = {};
    }
    if (options.cli) {
        await __turbopack_context__.A("[project]/node_modules/.bun/basehub@9.5.3+7111ff09de72ce5c/node_modules/basehub/dist/dist-5VU6BG5H.js [app-rsc] (ecmascript, async loader)").then(({ dotenvLoad })=>{
            dotenvLoad({
                priorities: {
                    ".dev.vars": 1
                }
            });
        });
    }
    const globalConfig = getGlobalConfig();
    let isForcedDraft = false;
    try {
        isForcedDraft = ("TURBOPACK compile-time value", "development") === "development" || (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$basehub$40$9$2e$5$2e$3$2b$7111ff09de72ce5c$2f$node_modules$2f$basehub$2f$dist$2f$chunk$2d$A7NXCT6I$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["isV0OrBolt"])();
    } catch (err) {}
    const buildEnvVarName = (name)=>{
        let prefix = defaultEnvVarPrefix;
        if (options.prefix) {
            if (options.prefix.endsWith("_")) {
                options.prefix = options.prefix.slice(0, -1);
            }
            if (options.prefix.endsWith(name)) {
                options.prefix = options.prefix.slice(0, -name.length);
            }
            if (options.prefix.endsWith(defaultEnvVarPrefix)) {
                prefix = options.prefix;
            } else {
                prefix = `${options.prefix}_${defaultEnvVarPrefix}`;
            }
        }
        return `${prefix}_${name}`;
    };
    const getEnvVar = (name)=>process?.env?.[buildEnvVarName(name)];
    const parsedDebugForcedURL = getEnvVar("DEBUG_FORCED_URL");
    const parsedBackwardsCompatURL = getEnvVar("URL");
    const backwardsCompatURL = parsedBackwardsCompatURL ? new URL(parsedBackwardsCompatURL) : void 0;
    const basehubUrl = new URL(parsedDebugForcedURL ? parsedDebugForcedURL : `${basehubAPIOrigin}/graphql`);
    let tokenNotFoundErrorMessage = `\u{1F534} Token not found. Make sure to include the ${buildEnvVarName("TOKEN")} env var.`;
    const resolveTokenParam = (token2)=>{
        if (!token2) return null;
        const isRaw = token2.startsWith("bshb_");
        if (isRaw) {
            return token2;
        }
        tokenNotFoundErrorMessage = `\u{1F534} Token not found. Make sure to include the ${token2} env var.`;
        const fromEnv = process?.env?.[token2];
        if (fromEnv) return fromEnv;
        return "";
    };
    const resolvedToken = resolveTokenParam(options?.token ?? null);
    const token = resolvedToken ?? basehubUrl.searchParams.get("token") ?? getEnvVar("TOKEN") ?? globalConfig?.token ?? (backwardsCompatURL ? backwardsCompatURL.searchParams.get("token") : void 0) ?? null;
    let fallbackPlayground;
    if (!token) {
        const fallbackPlaygroundTarget = options.fallbackPlayground?.target ?? getEnvVar("FALLBACK_PLAYGROUND_TARGET") ?? globalConfig?.fallbackPlayground?.target;
        const fallbackPlaygroundId = options.fallbackPlayground?.id ?? getEnvVar("FALLBACK_PLAYGROUND_ID") ?? globalConfig?.fallbackPlayground?.id;
        fallbackPlayground = options.fallbackPlayground ?? (fallbackPlaygroundId && fallbackPlaygroundTarget ? {
            target: fallbackPlaygroundTarget,
            id: fallbackPlaygroundId
        } : void 0);
        if (fallbackPlayground) {} else {
            if (options.cli) {
                console.error(tokenNotFoundErrorMessage);
                process.exit(1);
            } else {
                throw new Error(tokenNotFoundErrorMessage);
            }
        }
    }
    let ref = options.ref ?? basehubUrl.searchParams.get("ref") ?? getEnvVar("REF") ?? globalConfig?.ref ?? (backwardsCompatURL ? backwardsCompatURL.searchParams.get("ref") : void 0) ?? null;
    let draft = basehubUrl.searchParams.get("draft") ?? getEnvVar("DRAFT") ?? globalConfig?.draft ?? (backwardsCompatURL ? backwardsCompatURL.searchParams.get("draft") : void 0) ?? false;
    if (isForcedDraft) {
        draft = true;
    }
    if (options?.draft) {
        draft = true;
    }
    let apiVersion = basehubUrl.searchParams.get("api-version") ?? getEnvVar("API_VERSION") ?? (backwardsCompatURL ? backwardsCompatURL.searchParams.get("api-version") : void 0) ?? DEFAULT_API_VERSION;
    if (options?.apiVersion) {
        apiVersion = options.apiVersion;
    }
    if (basehubUrl.pathname.split("/")[1] !== "graphql") {
        const err = `\u{1F534} Invalid URL. The URL needs to point your repo's GraphQL endpoint, so the pathname should end with /graphql.`;
        if (options.cli) {
            console.error(err);
            process.exit(1);
        } else {
            throw new Error(err);
        }
    }
    basehubUrl.searchParams.delete("token");
    basehubUrl.searchParams.delete("ref");
    basehubUrl.searchParams.delete("draft");
    draft = !!draft;
    const { gitBranch, gitCommitSHA, gitBranchDeploymentURL, productionDeploymentURL } = await (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$basehub$40$9$2e$5$2e$3$2b$7111ff09de72ce5c$2f$node_modules$2f$basehub$2f$dist$2f$chunk$2d$N7COQVBX$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["getGitEnv"])(options);
    const resolvedRef = await resolveRef({
        url: basehubUrl,
        token,
        ref,
        gitBranch,
        gitCommitSHA,
        gitBranchDeploymentURL,
        productionDeploymentURL,
        apiVersion,
        revalidate: options.revalidateResolvedRef,
        fallbackPlayground
    });
    let isNextjsDraftMode = false;
    if (!(0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$basehub$40$9$2e$5$2e$3$2b$7111ff09de72ce5c$2f$node_modules$2f$basehub$2f$dist$2f$chunk$2d$A7NXCT6I$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["isV0OrBolt"])() && !draft) {
        try {
            const { draftMode } = await __turbopack_context__.A("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/headers.js [app-rsc] (ecmascript, async loader)");
            isNextjsDraftMode = (await draftMode()).isEnabled;
        } catch (error) {}
    }
    if (isNextjsDraftMode) {
        draft = true;
    }
    let previewRef;
    if (draft && !(0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$basehub$40$9$2e$5$2e$3$2b$7111ff09de72ce5c$2f$node_modules$2f$basehub$2f$dist$2f$chunk$2d$A7NXCT6I$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["isV0OrBolt"])()) {
        try {
            const { cookies } = await __turbopack_context__.A("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/headers.js [app-rsc] (ecmascript, async loader)");
            const cookieStore = await cookies();
            const ref2 = cookieStore.get("bshb-preview-ref-" + resolvedRef.repoHash)?.value;
            if (ref2) {
                previewRef = ref2;
            }
        } catch (error) {}
    }
    const sdkBuildId = `bshb_sdk__${__TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$basehub$40$9$2e$5$2e$3$2b$7111ff09de72ce5c$2f$node_modules$2f$basehub$2f$dist$2f$chunk$2d$2DLPZSMQ$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["version"]}__${resolvedRef.id}${gitBranch ? `__git_branch_${gitBranch}` : ""}${gitCommitSHA ? `__git_commit_sha_${gitCommitSHA}` : ""}`;
    if (!ref && resolvedRef) {
        ref = resolvedRef.ref;
    }
    return {
        draft,
        previewRef,
        isForcedDraft,
        isNextjsDraftMode,
        output: getEnvVar("OUTPUT") ?? options.cli?.output ?? null,
        resolvedRef,
        url: basehubUrl.toString(),
        gitBranch,
        gitCommitSHA,
        token,
        fallbackPlayground,
        gitBranchDeploymentURL,
        productionDeploymentURL,
        sdkBuildId,
        apiVersion,
        headers: {
            "x-basehub-api-version": apiVersion,
            "x-basehub-sdk-build-id": sdkBuildId,
            ...token ? {
                "x-basehub-token": token
            } : {},
            ...ref ? {
                "x-basehub-ref": ref
            } : {},
            // override if present
            ...previewRef ? {
                "x-basehub-ref": previewRef
            } : {},
            ...gitBranch ? {
                "x-basehub-git-branch": gitBranch
            } : {},
            ...gitCommitSHA ? {
                "x-basehub-git-commit-sha": gitCommitSHA
            } : {},
            ...draft ? {
                "x-basehub-draft": "true"
            } : {},
            ...gitBranchDeploymentURL ? {
                "x-basehub-git-branch-deployment-url": gitBranchDeploymentURL
            } : {},
            ...productionDeploymentURL ? {
                "x-basehub-production-deployment-url": productionDeploymentURL
            } : {},
            ...fallbackPlayground ? {
                "x-basehub-fallback-playground-target": fallbackPlayground.target,
                "x-basehub-fallback-playground-id": fallbackPlayground.id
            } : {}
        }
    };
};
var resolvedRefCache = /* @__PURE__ */ new Map();
async function resolveRef({ url, token, ref, gitBranch, gitCommitSHA, gitBranchDeploymentURL, productionDeploymentURL, apiVersion, revalidate, fallbackPlayground }) {
    const headers = {
        ...token ? {
            "x-basehub-token": token
        } : {},
        ...ref ? {
            "x-basehub-ref": ref
        } : {},
        ...gitBranch ? {
            "x-basehub-git-branch": gitBranch
        } : {},
        ...gitCommitSHA ? {
            "x-basehub-git-commit-sha": gitCommitSHA
        } : {},
        ...apiVersion ? {
            "x-basehub-api-version": apiVersion
        } : {},
        ...gitBranchDeploymentURL ? {
            "x-basehub-git-branch-deployment-url": gitBranchDeploymentURL
        } : {},
        ...productionDeploymentURL ? {
            "x-basehub-production-deployment-url": productionDeploymentURL
        } : {},
        ...fallbackPlayground ? {
            "x-basehub-fallback-playground-target": fallbackPlayground.target,
            "x-basehub-fallback-playground-id": fallbackPlayground.id
        } : {}
    };
    const cacheKey = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$basehub$40$9$2e$5$2e$3$2b$7111ff09de72ce5c$2f$node_modules$2f$basehub$2f$dist$2f$chunk$2d$UVZEJLNP$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["hashObject"])({
        headers
    });
    if (!revalidate) {
        const cachedResolvedRef = resolvedRefCache.get(cacheKey);
        if (cachedResolvedRef) {
            return cachedResolvedRef;
        }
    }
    const refResolverEndpoint = getBaseHubAppApiEndpoint(url, "/api/git/resolve-ref");
    const res = await fetch(refResolverEndpoint, {
        method: "POST",
        headers: {
            "Content-Type": "application/json",
            ...headers
        },
        cache: "no-store",
        body: JSON.stringify({})
    });
    if (res.status !== 200) {
        throw new Error(`Failed to resolve ref: ${res.statusText}`);
    }
    const data = await res.json();
    const resolvedRef = data;
    resolvedRefCache.set(cacheKey, resolvedRef);
    return resolvedRef;
}
function getBaseHubAppApiEndpoint(url, pathname) {
    let origin;
    switch(true){
        case url.origin.includes("api.bshb.dev"):
            origin = "https://basehub.dev" + pathname + url.search + url.hash;
            break;
        case url.origin.includes("localhost:3001"):
            origin = "http://localhost:3000" + pathname + url.search + url.hash;
            break;
        case url.origin.includes("api.basehub.com"):
        default:
            origin = "https://basehub.com" + pathname + url.search + url.hash;
    }
    return origin;
}
;
}),
"[project]/node_modules/.bun/basehub@9.5.3+7111ff09de72ce5c/node_modules/basehub/dist/next/toolbar/index.js [app-rsc] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "$$RSC_SERVER_ACTION_0",
    ()=>$$RSC_SERVER_ACTION_0,
    "$$RSC_SERVER_ACTION_1",
    ()=>$$RSC_SERVER_ACTION_1,
    "$$RSC_SERVER_ACTION_2",
    ()=>$$RSC_SERVER_ACTION_2,
    "$$RSC_SERVER_ACTION_3",
    ()=>$$RSC_SERVER_ACTION_3,
    "Toolbar",
    ()=>ServerToolbar
]);
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$build$2f$webpack$2f$loaders$2f$next$2d$flight$2d$loader$2f$server$2d$reference$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/build/webpack/loaders/next-flight-loader/server-reference.js [app-rsc] (ecmascript)");
/* __next_internal_action_entry_do_not_use__ [{"00661ccaf4b7466b09eb583bfe20d0f5854badd224":"$$RSC_SERVER_ACTION_2","601c171143a242d8e2f3fc33775755aa13287ade0d":"$$RSC_SERVER_ACTION_1","6045cbc8eaf7a56a74edcf895d925e12158760861d":"$$RSC_SERVER_ACTION_0","60ea0f965c0bbd2b6e7bd21ad468ff954f246ef2ff":"$$RSC_SERVER_ACTION_3"},"",""] */ var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$basehub$40$9$2e$5$2e$3$2b$7111ff09de72ce5c$2f$node_modules$2f$basehub$2f$dist$2f$chunk$2d$R45FBMZ7$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/basehub@9.5.3+7111ff09de72ce5c/node_modules/basehub/dist/chunk-R45FBMZ7.js [app-rsc] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$basehub$40$9$2e$5$2e$3$2b$7111ff09de72ce5c$2f$node_modules$2f$basehub$2f$dist$2f$chunk$2d$A7NXCT6I$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/basehub@9.5.3+7111ff09de72ce5c/node_modules/basehub/dist/chunk-A7NXCT6I.js [app-rsc] (ecmascript)");
// src/next/toolbar/server-toolbar.tsx
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$server$2f$route$2d$modules$2f$app$2d$page$2f$vendored$2f$rsc$2f$react$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/server/route-modules/app-page/vendored/rsc/react.js [app-rsc] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$server$2f$route$2d$modules$2f$app$2d$page$2f$vendored$2f$rsc$2f$react$2d$jsx$2d$runtime$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/dist/server/route-modules/app-page/vendored/rsc/react-jsx-runtime.js [app-rsc] (ecmascript)");
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
;
;
var LazyClientConditionalRenderer = __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$server$2f$route$2d$modules$2f$app$2d$page$2f$vendored$2f$rsc$2f$react$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["lazy"](()=>__turbopack_context__.A("[project]/node_modules/.bun/basehub@9.5.3+7111ff09de72ce5c/node_modules/basehub/dist/client-conditional-renderer-YBOE2OM5.js [app-rsc] (ecmascript, async loader)").then((mod)=>({
            default: mod.ClientConditionalRenderer
        })));
const $$RSC_SERVER_ACTION_0 = async function enableDraftMode_unbound(basehubProps2, { bshbPreviewToken }) {
    try {
        const { draftMode } = await __turbopack_context__.A("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/headers.js [app-rsc] (ecmascript, async loader)");
        const { headers, url } = await (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$basehub$40$9$2e$5$2e$3$2b$7111ff09de72ce5c$2f$node_modules$2f$basehub$2f$dist$2f$chunk$2d$R45FBMZ7$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["getStuffFromEnv"])(basehubProps2);
        const appApiEndpoint = getBaseHubAppApiEndpoint(new URL(url), "/api/nextjs/preview-auth");
        const res = await fetch(appApiEndpoint, {
            cache: "no-store",
            method: "POST",
            headers: {
                "content-type": "application/json",
                ...headers
            },
            body: JSON.stringify({
                bshbPreview: bshbPreviewToken
            })
        });
        const responseIsJson = res.headers.get("content-type")?.includes("json");
        if (!responseIsJson) {
            return {
                status: 400,
                response: {
                    error: "Bad request"
                }
            };
        }
        const response = await res.json();
        if (res.status === 200) (await draftMode())?.enable();
        return {
            status: res.status,
            response
        };
    } catch (error) {
        return {
            status: 500,
            response: {
                error: "Something went wrong"
            }
        };
    }
};
(0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$build$2f$webpack$2f$loaders$2f$next$2d$flight$2d$loader$2f$server$2d$reference$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["registerServerReference"])($$RSC_SERVER_ACTION_0, "6045cbc8eaf7a56a74edcf895d925e12158760861d", null);
const $$RSC_SERVER_ACTION_1 = async function getLatestBranches_unbound(basehubProps2, { bshbPreviewToken }) {
    try {
        const { headers, url, isForcedDraft: isForcedDraft2 } = await (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$basehub$40$9$2e$5$2e$3$2b$7111ff09de72ce5c$2f$node_modules$2f$basehub$2f$dist$2f$chunk$2d$R45FBMZ7$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["getStuffFromEnv"])(basehubProps2);
        const { draftMode } = await __turbopack_context__.A("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/headers.js [app-rsc] (ecmascript, async loader)");
        if (((await draftMode())?.isEnabled ?? false) === false && !isForcedDraft2 && !bshbPreviewToken) {
            return {
                status: 403,
                response: {
                    error: "Unauthorized"
                }
            };
        }
        const appApiEndpoint = getBaseHubAppApiEndpoint(new URL(url), "/api/nextjs/latest-branches");
        const res = await fetch(appApiEndpoint, {
            cache: "no-store",
            method: "GET",
            headers: {
                "content-type": "application/json",
                ...headers,
                ...bshbPreviewToken && {
                    "x-basehub-preview-token": bshbPreviewToken
                },
                ...isForcedDraft2 && {
                    "x-basehub-forced-draft": "true"
                }
            }
        });
        const responseIsJson = res.headers.get("content-type")?.includes("json");
        if (!responseIsJson) {
            return {
                status: 400,
                response: {
                    error: "Bad request"
                }
            };
        }
        const response = await res.json();
        return {
            status: res.status,
            response
        };
    } catch (error) {
        return {
            status: 500,
            response: {
                error: "Something went wrong"
            }
        };
    }
};
(0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$build$2f$webpack$2f$loaders$2f$next$2d$flight$2d$loader$2f$server$2d$reference$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["registerServerReference"])($$RSC_SERVER_ACTION_1, "601c171143a242d8e2f3fc33775755aa13287ade0d", null);
const $$RSC_SERVER_ACTION_2 = async function disableDraftMode() {
    try {
        const { draftMode } = await __turbopack_context__.A("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/headers.js [app-rsc] (ecmascript, async loader)");
        (await draftMode()).disable();
    } catch (err) {}
};
(0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$build$2f$webpack$2f$loaders$2f$next$2d$flight$2d$loader$2f$server$2d$reference$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["registerServerReference"])($$RSC_SERVER_ACTION_2, "00661ccaf4b7466b09eb583bfe20d0f5854badd224", null);
const $$RSC_SERVER_ACTION_3 = async function revalidateTags_unbound(basehubProps2, { bshbPreviewToken, ref, warmupRun }) {
    try {
        const { headers, url } = await (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$basehub$40$9$2e$5$2e$3$2b$7111ff09de72ce5c$2f$node_modules$2f$basehub$2f$dist$2f$chunk$2d$R45FBMZ7$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["getStuffFromEnv"])(basehubProps2);
        const appApiEndpoint = getBaseHubAppApiEndpoint(new URL(url), "/api/nextjs/pending-tags");
        if (!bshbPreviewToken) {
            return {
                success: false,
                error: "Unauthorized"
            };
        }
        const init = {
            cache: "no-store",
            method: "GET",
            headers: {
                "content-type": "application/json",
                ...headers,
                ...ref && {
                    "x-basehub-ref": ref
                },
                ...bshbPreviewToken && {
                    "x-basehub-preview-token": bshbPreviewToken
                }
            }
        };
        const res = await fetch(appApiEndpoint + (warmupRun ? "?warmupRun=true" : ""), structuredClone(init));
        if (res.status !== 200) {
            return {
                success: false,
                message: `Received status ${res.status} from server`
            };
        }
        if (warmupRun) {
            return {
                success: true,
                message: "ok",
                fetchData: {
                    url: appApiEndpoint,
                    options: init
                }
            };
        }
        const response = await res.json();
        const { tags } = response;
        if (!tags || !Array.isArray(tags) || tags.length === 0) {
            return {
                success: true,
                message: "No tags to revalidate"
            };
        }
        const { revalidateTag } = await __turbopack_context__.A("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/cache.js [app-rsc] (ecmascript, async loader)");
        await Promise.all(tags.map(async (_tag)=>{
            const tag = _tag.startsWith("basehub-") ? _tag : `basehub-${_tag}`;
            await revalidateTag(tag);
        }));
        return {
            success: true,
            message: `Revalidated ${tags.length} tags`
        };
    } catch (error) {
        console.error(error);
        return {
            success: false,
            message: "Something went wrong while revalidating tags"
        };
    }
};
(0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$build$2f$webpack$2f$loaders$2f$next$2d$flight$2d$loader$2f$server$2d$reference$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["registerServerReference"])($$RSC_SERVER_ACTION_3, "60ea0f965c0bbd2b6e7bd21ad468ff954f246ef2ff", null);
var ServerToolbar = async ({ ...basehubProps })=>{
    const { isForcedDraft, resolvedRef } = await (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$basehub$40$9$2e$5$2e$3$2b$7111ff09de72ce5c$2f$node_modules$2f$basehub$2f$dist$2f$chunk$2d$R45FBMZ7$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["getStuffFromEnv"])(basehubProps);
    let isDraftMode = false;
    if (!(0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$basehub$40$9$2e$5$2e$3$2b$7111ff09de72ce5c$2f$node_modules$2f$basehub$2f$dist$2f$chunk$2d$A7NXCT6I$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["isV0OrBolt"])()) {
        try {
            const { draftMode } = await __turbopack_context__.A("[project]/node_modules/.bun/next@16.1.6+6d2d1b167ad600d7/node_modules/next/headers.js [app-rsc] (ecmascript, async loader)");
            isDraftMode = (await draftMode()).isEnabled;
        } catch (err) {}
    }
    const enableDraftMode_unbound = $$RSC_SERVER_ACTION_0;
    const getLatestBranches_unbound = $$RSC_SERVER_ACTION_1;
    const disableDraftMode = $$RSC_SERVER_ACTION_2;
    const revalidateTags_unbound = $$RSC_SERVER_ACTION_3;
    const enableDraftMode = enableDraftMode_unbound.bind(null, basehubProps);
    const getLatestBranches = getLatestBranches_unbound.bind(null, basehubProps);
    const revalidateTags = revalidateTags_unbound.bind(null, basehubProps);
    return /* @__PURE__ */ (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$next$40$16$2e$1$2e$6$2b$6d2d1b167ad600d7$2f$node_modules$2f$next$2f$dist$2f$server$2f$route$2d$modules$2f$app$2d$page$2f$vendored$2f$rsc$2f$react$2d$jsx$2d$runtime$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["jsx"])(LazyClientConditionalRenderer, {
        draft: isDraftMode,
        isForcedDraft,
        enableDraftMode,
        disableDraftMode,
        revalidateTags,
        getLatestBranches,
        resolvedRef
    });
};
function getBaseHubAppApiEndpoint(url, pathname) {
    let origin;
    switch(true){
        case url.origin.includes("api.bshb.dev"):
            origin = "https://basehub.dev" + pathname + url.search + url.hash;
            break;
        case url.origin.includes("localhost:3001"):
            origin = "http://localhost:3000" + pathname + url.search + url.hash;
            break;
        case url.origin.includes("api.basehub.com"):
        default:
            origin = "https://basehub.com" + pathname + url.search + url.hash;
    }
    return origin;
}
;
}),
];

//# sourceMappingURL=_a7061b76._.js.map