module.exports = [
"[project]/node_modules/.bun/basehub@9.5.3+7111ff09de72ce5c/node_modules/basehub/dist/chunk-FPPKRKBX.js [app-rsc] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "__commonJS",
    ()=>__commonJS,
    "__export",
    ()=>__export,
    "__privateAdd",
    ()=>__privateAdd,
    "__privateGet",
    ()=>__privateGet,
    "__privateMethod",
    ()=>__privateMethod,
    "__privateSet",
    ()=>__privateSet,
    "__privateWrapper",
    ()=>__privateWrapper,
    "__publicField",
    ()=>__publicField,
    "__require",
    ()=>__require,
    "__toESM",
    ()=>__toESM
]);
var __create = Object.create;
var __defProp = Object.defineProperty;
var __getOwnPropDesc = Object.getOwnPropertyDescriptor;
var __getOwnPropNames = Object.getOwnPropertyNames;
var __getProtoOf = Object.getPrototypeOf;
var __hasOwnProp = Object.prototype.hasOwnProperty;
var __typeError = (msg)=>{
    throw TypeError(msg);
};
var __defNormalProp = (obj, key, value1)=>key in obj ? __defProp(obj, key, {
        enumerable: true,
        configurable: true,
        writable: true,
        value: value1
    }) : obj[key] = value1;
var __require = /* @__PURE__ */ ((x)=>("TURBOPACK compile-time truthy", 1) ? /*TURBOPACK member replacement*/ __turbopack_context__.z : "TURBOPACK unreachable")(function(x) {
    if ("TURBOPACK compile-time truthy", 1) return /*TURBOPACK member replacement*/ __turbopack_context__.z.apply(this, arguments);
    //TURBOPACK unreachable
    ;
});
var __commonJS = (cb, mod)=>function __require2() {
        return mod || (0, cb[__getOwnPropNames(cb)[0]])((mod = {
            exports: {}
        }).exports, mod), mod.exports;
    };
var __export = (target, all)=>{
    for(var name in all)__defProp(target, name, {
        get: all[name],
        enumerable: true
    });
};
var __copyProps = (to, from, except, desc)=>{
    if (from && typeof from === "object" || typeof from === "function") {
        for (let key of __getOwnPropNames(from))if (!__hasOwnProp.call(to, key) && key !== except) __defProp(to, key, {
            get: ()=>from[key],
            enumerable: !(desc = __getOwnPropDesc(from, key)) || desc.enumerable
        });
    }
    return to;
};
var __toESM = (mod, isNodeMode, target)=>(target = mod != null ? __create(__getProtoOf(mod)) : {}, __copyProps(// If the importer is in node compatibility mode or this is not an ESM
    // file that has been converted to a CommonJS file using a Babel-
    // compatible transform (i.e. "__esModule" has not been set), then set
    // "default" to the CommonJS "module.exports" for node compatibility.
    isNodeMode || !mod || !mod.__esModule ? __defProp(target, "default", {
        value: mod,
        enumerable: true
    }) : target, mod));
var __publicField = (obj, key, value1)=>__defNormalProp(obj, typeof key !== "symbol" ? key + "" : key, value1);
var __accessCheck = (obj, member, msg)=>member.has(obj) || __typeError("Cannot " + msg);
var __privateGet = (obj, member, getter)=>(__accessCheck(obj, member, "read from private field"), getter ? getter.call(obj) : member.get(obj));
var __privateAdd = (obj, member, value1)=>member.has(obj) ? __typeError("Cannot add the same private member more than once") : member instanceof WeakSet ? member.add(obj) : member.set(obj, value1);
var __privateSet = (obj, member, value1, setter)=>(__accessCheck(obj, member, "write to private field"), setter ? setter.call(obj, value1) : member.set(obj, value1), value1);
var __privateMethod = (obj, member, method)=>(__accessCheck(obj, member, "access private method"), method);
var __privateWrapper = (obj, member, setter, getter)=>({
        set _ (value){
            __privateSet(obj, member, value, setter);
        },
        get _ () {
            return __privateGet(obj, member, getter);
        }
    });
;
}),
"[project]/node_modules/.bun/basehub@9.5.3+7111ff09de72ce5c/node_modules/basehub/dist/dist-5VU6BG5H.js [app-rsc] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "default",
    ()=>__TURBOPACK__default__export__
]);
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$basehub$40$9$2e$5$2e$3$2b$7111ff09de72ce5c$2f$node_modules$2f$basehub$2f$dist$2f$chunk$2d$FPPKRKBX$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/.bun/basehub@9.5.3+7111ff09de72ce5c/node_modules/basehub/dist/chunk-FPPKRKBX.js [app-rsc] (ecmascript)");
;
// ../../node_modules/.pnpm/dotenv@16.6.1/node_modules/dotenv/package.json
var require_package = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$basehub$40$9$2e$5$2e$3$2b$7111ff09de72ce5c$2f$node_modules$2f$basehub$2f$dist$2f$chunk$2d$FPPKRKBX$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["__commonJS"])({
    "../../node_modules/.pnpm/dotenv@16.6.1/node_modules/dotenv/package.json" (exports, module) {
        module.exports = {
            name: "dotenv",
            version: "16.6.1",
            description: "Loads environment variables from .env file",
            main: "lib/main.js",
            types: "lib/main.d.ts",
            exports: {
                ".": {
                    types: "./lib/main.d.ts",
                    require: "./lib/main.js",
                    default: "./lib/main.js"
                },
                "./config": "./config.js",
                "./config.js": "./config.js",
                "./lib/env-options": "./lib/env-options.js",
                "./lib/env-options.js": "./lib/env-options.js",
                "./lib/cli-options": "./lib/cli-options.js",
                "./lib/cli-options.js": "./lib/cli-options.js",
                "./package.json": "./package.json"
            },
            scripts: {
                "dts-check": "tsc --project tests/types/tsconfig.json",
                lint: "standard",
                pretest: "npm run lint && npm run dts-check",
                test: "tap run --allow-empty-coverage --disable-coverage --timeout=60000",
                "test:coverage": "tap run --show-full-coverage --timeout=60000 --coverage-report=text --coverage-report=lcov",
                prerelease: "npm test",
                release: "standard-version"
            },
            repository: {
                type: "git",
                url: "git://github.com/motdotla/dotenv.git"
            },
            homepage: "https://github.com/motdotla/dotenv#readme",
            funding: "https://dotenvx.com",
            keywords: [
                "dotenv",
                "env",
                ".env",
                "environment",
                "variables",
                "config",
                "settings"
            ],
            readmeFilename: "README.md",
            license: "BSD-2-Clause",
            devDependencies: {
                "@types/node": "^18.11.3",
                decache: "^4.6.2",
                sinon: "^14.0.1",
                standard: "^17.0.0",
                "standard-version": "^9.5.0",
                tap: "^19.2.0",
                typescript: "^4.8.4"
            },
            engines: {
                node: ">=12"
            },
            browser: {
                fs: false
            }
        };
    }
});
// ../../node_modules/.pnpm/dotenv@16.6.1/node_modules/dotenv/lib/main.js
var require_main = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$basehub$40$9$2e$5$2e$3$2b$7111ff09de72ce5c$2f$node_modules$2f$basehub$2f$dist$2f$chunk$2d$FPPKRKBX$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["__commonJS"])({
    "../../node_modules/.pnpm/dotenv@16.6.1/node_modules/dotenv/lib/main.js" (exports, module) {
        "use strict";
        var fs = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$basehub$40$9$2e$5$2e$3$2b$7111ff09de72ce5c$2f$node_modules$2f$basehub$2f$dist$2f$chunk$2d$FPPKRKBX$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["__require"])("fs");
        var path = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$basehub$40$9$2e$5$2e$3$2b$7111ff09de72ce5c$2f$node_modules$2f$basehub$2f$dist$2f$chunk$2d$FPPKRKBX$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["__require"])("path");
        var os = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$basehub$40$9$2e$5$2e$3$2b$7111ff09de72ce5c$2f$node_modules$2f$basehub$2f$dist$2f$chunk$2d$FPPKRKBX$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["__require"])("os");
        var crypto = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$basehub$40$9$2e$5$2e$3$2b$7111ff09de72ce5c$2f$node_modules$2f$basehub$2f$dist$2f$chunk$2d$FPPKRKBX$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["__require"])("crypto");
        var packageJson = require_package();
        var version = packageJson.version;
        var LINE = /(?:^|^)\s*(?:export\s+)?([\w.-]+)(?:\s*=\s*?|:\s+?)(\s*'(?:\\'|[^'])*'|\s*"(?:\\"|[^"])*"|\s*`(?:\\`|[^`])*`|[^#\r\n]+)?\s*(?:#.*)?(?:$|$)/mg;
        function parse(src) {
            const obj = {};
            let lines = src.toString();
            lines = lines.replace(/\r\n?/mg, "\n");
            let match;
            while((match = LINE.exec(lines)) != null){
                const key = match[1];
                let value = match[2] || "";
                value = value.trim();
                const maybeQuote = value[0];
                value = value.replace(/^(['"`])([\s\S]*)\1$/mg, "$2");
                if (maybeQuote === '"') {
                    value = value.replace(/\\n/g, "\n");
                    value = value.replace(/\\r/g, "\r");
                }
                obj[key] = value;
            }
            return obj;
        }
        function _parseVault(options) {
            options = options || {};
            const vaultPath = _vaultPath(options);
            options.path = vaultPath;
            const result = DotenvModule.configDotenv(options);
            if (!result.parsed) {
                const err = new Error(`MISSING_DATA: Cannot parse ${vaultPath} for an unknown reason`);
                err.code = "MISSING_DATA";
                throw err;
            }
            const keys = _dotenvKey(options).split(",");
            const length = keys.length;
            let decrypted;
            for(let i = 0; i < length; i++){
                try {
                    const key = keys[i].trim();
                    const attrs = _instructions(result, key);
                    decrypted = DotenvModule.decrypt(attrs.ciphertext, attrs.key);
                    break;
                } catch (error) {
                    if (i + 1 >= length) {
                        throw error;
                    }
                }
            }
            return DotenvModule.parse(decrypted);
        }
        function _warn(message) {
            console.log(`[dotenv@${version}][WARN] ${message}`);
        }
        function _debug(message) {
            console.log(`[dotenv@${version}][DEBUG] ${message}`);
        }
        function _log(message) {
            console.log(`[dotenv@${version}] ${message}`);
        }
        function _dotenvKey(options) {
            if (options && options.DOTENV_KEY && options.DOTENV_KEY.length > 0) {
                return options.DOTENV_KEY;
            }
            if (process.env.DOTENV_KEY && process.env.DOTENV_KEY.length > 0) {
                return process.env.DOTENV_KEY;
            }
            return "";
        }
        function _instructions(result, dotenvKey) {
            let uri;
            try {
                uri = new URL(dotenvKey);
            } catch (error) {
                if (error.code === "ERR_INVALID_URL") {
                    const err = new Error("INVALID_DOTENV_KEY: Wrong format. Must be in valid uri format like dotenv://:key_1234@dotenvx.com/vault/.env.vault?environment=development");
                    err.code = "INVALID_DOTENV_KEY";
                    throw err;
                }
                throw error;
            }
            const key = uri.password;
            if (!key) {
                const err = new Error("INVALID_DOTENV_KEY: Missing key part");
                err.code = "INVALID_DOTENV_KEY";
                throw err;
            }
            const environment = uri.searchParams.get("environment");
            if (!environment) {
                const err = new Error("INVALID_DOTENV_KEY: Missing environment part");
                err.code = "INVALID_DOTENV_KEY";
                throw err;
            }
            const environmentKey = `DOTENV_VAULT_${environment.toUpperCase()}`;
            const ciphertext = result.parsed[environmentKey];
            if (!ciphertext) {
                const err = new Error(`NOT_FOUND_DOTENV_ENVIRONMENT: Cannot locate environment ${environmentKey} in your .env.vault file.`);
                err.code = "NOT_FOUND_DOTENV_ENVIRONMENT";
                throw err;
            }
            return {
                ciphertext,
                key
            };
        }
        function _vaultPath(options) {
            let possibleVaultPath = null;
            if (options && options.path && options.path.length > 0) {
                if (Array.isArray(options.path)) {
                    for (const filepath of options.path){
                        if (fs.existsSync(filepath)) {
                            possibleVaultPath = filepath.endsWith(".vault") ? filepath : `${filepath}.vault`;
                        }
                    }
                } else {
                    possibleVaultPath = options.path.endsWith(".vault") ? options.path : `${options.path}.vault`;
                }
            } else {
                possibleVaultPath = path.resolve(process.cwd(), ".env.vault");
            }
            if (fs.existsSync(possibleVaultPath)) {
                return possibleVaultPath;
            }
            return null;
        }
        function _resolveHome(envPath) {
            return envPath[0] === "~" ? path.join(os.homedir(), envPath.slice(1)) : envPath;
        }
        function _configVault(options) {
            const debug = Boolean(options && options.debug);
            const quiet = options && "quiet" in options ? options.quiet : true;
            if (debug || !quiet) {
                _log("Loading env from encrypted .env.vault");
            }
            const parsed = DotenvModule._parseVault(options);
            let processEnv = process.env;
            if (options && options.processEnv != null) {
                processEnv = options.processEnv;
            }
            DotenvModule.populate(processEnv, parsed, options);
            return {
                parsed
            };
        }
        function configDotenv(options) {
            const dotenvPath = path.resolve(process.cwd(), ".env");
            let encoding = "utf8";
            const debug = Boolean(options && options.debug);
            const quiet = options && "quiet" in options ? options.quiet : true;
            if (options && options.encoding) {
                encoding = options.encoding;
            } else {
                if (debug) {
                    _debug("No encoding is specified. UTF-8 is used by default");
                }
            }
            let optionPaths = [
                dotenvPath
            ];
            if (options && options.path) {
                if (!Array.isArray(options.path)) {
                    optionPaths = [
                        _resolveHome(options.path)
                    ];
                } else {
                    optionPaths = [];
                    for (const filepath of options.path){
                        optionPaths.push(_resolveHome(filepath));
                    }
                }
            }
            let lastError;
            const parsedAll = {};
            for (const path2 of optionPaths){
                try {
                    const parsed = DotenvModule.parse(fs.readFileSync(path2, {
                        encoding
                    }));
                    DotenvModule.populate(parsedAll, parsed, options);
                } catch (e) {
                    if (debug) {
                        _debug(`Failed to load ${path2} ${e.message}`);
                    }
                    lastError = e;
                }
            }
            let processEnv = process.env;
            if (options && options.processEnv != null) {
                processEnv = options.processEnv;
            }
            DotenvModule.populate(processEnv, parsedAll, options);
            if (debug || !quiet) {
                const keysCount = Object.keys(parsedAll).length;
                const shortPaths = [];
                for (const filePath of optionPaths){
                    try {
                        const relative = path.relative(process.cwd(), filePath);
                        shortPaths.push(relative);
                    } catch (e) {
                        if (debug) {
                            _debug(`Failed to load ${filePath} ${e.message}`);
                        }
                        lastError = e;
                    }
                }
                _log(`injecting env (${keysCount}) from ${shortPaths.join(",")}`);
            }
            if (lastError) {
                return {
                    parsed: parsedAll,
                    error: lastError
                };
            } else {
                return {
                    parsed: parsedAll
                };
            }
        }
        function config(options) {
            if (_dotenvKey(options).length === 0) {
                return DotenvModule.configDotenv(options);
            }
            const vaultPath = _vaultPath(options);
            if (!vaultPath) {
                _warn(`You set DOTENV_KEY but you are missing a .env.vault file at ${vaultPath}. Did you forget to build it?`);
                return DotenvModule.configDotenv(options);
            }
            return DotenvModule._configVault(options);
        }
        function decrypt(encrypted, keyStr) {
            const key = Buffer.from(keyStr.slice(-64), "hex");
            let ciphertext = Buffer.from(encrypted, "base64");
            const nonce = ciphertext.subarray(0, 12);
            const authTag = ciphertext.subarray(-16);
            ciphertext = ciphertext.subarray(12, -16);
            try {
                const aesgcm = crypto.createDecipheriv("aes-256-gcm", key, nonce);
                aesgcm.setAuthTag(authTag);
                return `${aesgcm.update(ciphertext)}${aesgcm.final()}`;
            } catch (error) {
                const isRange = error instanceof RangeError;
                const invalidKeyLength = error.message === "Invalid key length";
                const decryptionFailed = error.message === "Unsupported state or unable to authenticate data";
                if (isRange || invalidKeyLength) {
                    const err = new Error("INVALID_DOTENV_KEY: It must be 64 characters long (or more)");
                    err.code = "INVALID_DOTENV_KEY";
                    throw err;
                } else if (decryptionFailed) {
                    const err = new Error("DECRYPTION_FAILED: Please check your DOTENV_KEY");
                    err.code = "DECRYPTION_FAILED";
                    throw err;
                } else {
                    throw error;
                }
            }
        }
        function populate(processEnv, parsed, options = {}) {
            const debug = Boolean(options && options.debug);
            const override = Boolean(options && options.override);
            if (typeof parsed !== "object") {
                const err = new Error("OBJECT_REQUIRED: Please check the processEnv argument being passed to populate");
                err.code = "OBJECT_REQUIRED";
                throw err;
            }
            for (const key of Object.keys(parsed)){
                if (Object.prototype.hasOwnProperty.call(processEnv, key)) {
                    if (override === true) {
                        processEnv[key] = parsed[key];
                    }
                    if (debug) {
                        if (override === true) {
                            _debug(`"${key}" is already defined and WAS overwritten`);
                        } else {
                            _debug(`"${key}" is already defined and was NOT overwritten`);
                        }
                    }
                } else {
                    processEnv[key] = parsed[key];
                }
            }
        }
        var DotenvModule = {
            configDotenv,
            _configVault,
            _parseVault,
            config,
            decrypt,
            parse,
            populate
        };
        module.exports.configDotenv = DotenvModule.configDotenv;
        module.exports._configVault = DotenvModule._configVault;
        module.exports._parseVault = DotenvModule._parseVault;
        module.exports.config = DotenvModule.config;
        module.exports.decrypt = DotenvModule.decrypt;
        module.exports.parse = DotenvModule.parse;
        module.exports.populate = DotenvModule.populate;
        module.exports = DotenvModule;
    }
});
// ../../node_modules/.pnpm/dotenv-expand@10.0.0/node_modules/dotenv-expand/lib/main.js
var require_main2 = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$basehub$40$9$2e$5$2e$3$2b$7111ff09de72ce5c$2f$node_modules$2f$basehub$2f$dist$2f$chunk$2d$FPPKRKBX$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["__commonJS"])({
    "../../node_modules/.pnpm/dotenv-expand@10.0.0/node_modules/dotenv-expand/lib/main.js" (exports, module) {
        "use strict";
        function _searchLast(str, rgx) {
            const matches = Array.from(str.matchAll(rgx));
            return matches.length > 0 ? matches.slice(-1)[0].index : -1;
        }
        function _interpolate(envValue, environment, config) {
            const lastUnescapedDollarSignIndex = _searchLast(envValue, /(?!(?<=\\))\$/g);
            if (lastUnescapedDollarSignIndex === -1) return envValue;
            const rightMostGroup = envValue.slice(lastUnescapedDollarSignIndex);
            const matchGroup = /((?!(?<=\\))\${?([\w]+)(?::-([^}\\]*))?}?)/;
            const match = rightMostGroup.match(matchGroup);
            if (match != null) {
                const [, group, variableName, defaultValue] = match;
                return _interpolate(envValue.replace(group, environment[variableName] || defaultValue || config.parsed[variableName] || ""), environment, config);
            }
            return envValue;
        }
        function _resolveEscapeSequences(value) {
            return value.replace(/\\\$/g, "$");
        }
        function expand(config) {
            const environment = config.ignoreProcessEnv ? {} : process.env;
            for(const configKey in config.parsed){
                const value = Object.prototype.hasOwnProperty.call(environment, configKey) ? environment[configKey] : config.parsed[configKey];
                config.parsed[configKey] = _resolveEscapeSequences(_interpolate(value, environment, config));
            }
            for(const processKey in config.parsed){
                environment[processKey] = config.parsed[processKey];
            }
            return config;
        }
        module.exports.expand = expand;
    }
});
// ../../node_modules/.pnpm/dotenv-mono@1.3.10/node_modules/dotenv-mono/dist/index.js
var require_dist = (0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$basehub$40$9$2e$5$2e$3$2b$7111ff09de72ce5c$2f$node_modules$2f$basehub$2f$dist$2f$chunk$2d$FPPKRKBX$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["__commonJS"])({
    "../../node_modules/.pnpm/dotenv-mono@1.3.10/node_modules/dotenv-mono/dist/index.js" (exports) {
        var __classPrivateFieldGet = exports && exports.__classPrivateFieldGet || function(receiver, state, kind, f) {
            if (kind === "a" && !f) throw new TypeError("Private accessor was defined without a getter");
            if (typeof state === "function" ? receiver !== state || !f : !state.has(receiver)) throw new TypeError("Cannot read private member from an object whose class did not declare it");
            return kind === "m" ? f : kind === "a" ? f.call(receiver) : f ? f.value : state.get(receiver);
        };
        var __classPrivateFieldSet = exports && exports.__classPrivateFieldSet || function(receiver, state, value, kind, f) {
            if (kind === "m") throw new TypeError("Private method is not writable");
            if (kind === "a" && !f) throw new TypeError("Private accessor was defined without a setter");
            if (typeof state === "function" ? receiver !== state || !f : !state.has(receiver)) throw new TypeError("Cannot write private member to an object whose class did not declare it");
            return kind === "a" ? f.call(receiver, value) : f ? f.value = value : state.set(receiver, value), value;
        };
        var __importDefault = exports && exports.__importDefault || function(mod) {
            return mod && mod.__esModule ? mod : {
                "default": mod
            };
        };
        var _Dotenv__cwd;
        var _Dotenv__debug;
        var _Dotenv__defaults;
        var _Dotenv__depth;
        var _Dotenv__encoding;
        var _Dotenv__expand;
        var _Dotenv__extension;
        var _Dotenv__override;
        var _Dotenv__path;
        var _Dotenv__priorities;
        Object.defineProperty(exports, "__esModule", {
            value: true
        });
        exports.config = exports.dotenvConfig = exports.load = exports.dotenvLoad = exports.Dotenv = void 0;
        var fs_1 = __importDefault((0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$basehub$40$9$2e$5$2e$3$2b$7111ff09de72ce5c$2f$node_modules$2f$basehub$2f$dist$2f$chunk$2d$FPPKRKBX$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["__require"])("fs"));
        var os_1 = __importDefault((0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$basehub$40$9$2e$5$2e$3$2b$7111ff09de72ce5c$2f$node_modules$2f$basehub$2f$dist$2f$chunk$2d$FPPKRKBX$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["__require"])("os"));
        var path_1 = __importDefault((0, __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f2e$bun$2f$basehub$40$9$2e$5$2e$3$2b$7111ff09de72ce5c$2f$node_modules$2f$basehub$2f$dist$2f$chunk$2d$FPPKRKBX$2e$js__$5b$app$2d$rsc$5d$__$28$ecmascript$29$__["__require"])("path"));
        var dotenv_1 = __importDefault(require_main());
        var dotenv_expand_1 = __importDefault(require_main2());
        var Dotenv = class {
            /**
       * Constructor.
       * @param cwd - current Working Directory
       * @param debug - turn on/off debugging
       * @param depth - max walking up depth
       * @param encoding - file encoding
       * @param expand - turn on/off dotenv-expand plugin
       * @param extension - add dotenv extension
       * @param override - override process variables
       * @param path - dotenv path
       * @param priorities - priorities
       */ constructor({ cwd, debug, defaults, depth, encoding, expand, extension, override, path, priorities } = {}){
                this.config = {};
                this.env = {};
                this.plain = "";
                _Dotenv__cwd.set(this, "");
                _Dotenv__debug.set(this, false);
                _Dotenv__defaults.set(this, ".env.defaults");
                _Dotenv__depth.set(this, 4);
                _Dotenv__encoding.set(this, "utf8");
                _Dotenv__expand.set(this, true);
                _Dotenv__extension.set(this, "");
                _Dotenv__override.set(this, false);
                _Dotenv__path.set(this, "");
                _Dotenv__priorities.set(this, {});
                this.cwd = cwd;
                this.debug = debug;
                this.defaults = defaults;
                this.depth = depth;
                this.encoding = encoding;
                this.expand = expand;
                this.extension = extension;
                this.override = override;
                this.path = path;
                this.priorities = priorities;
                this.dotenvDefaultsMatcher = this.dotenvDefaultsMatcher.bind(this);
                this.dotenvMatcher = this.dotenvMatcher.bind(this);
            }
            /**
       * Get debugging.
       */ get debug() {
                return __classPrivateFieldGet(this, _Dotenv__debug, "f");
            }
            /**
       * Set debugging.
       * @param value
       */ set debug(value) {
                if (value != null) __classPrivateFieldSet(this, _Dotenv__debug, value, "f");
            }
            /**
       * Get defaults filename.
       */ get defaults() {
                return __classPrivateFieldGet(this, _Dotenv__defaults, "f");
            }
            /**
       * Set defaults filename.
       * @param value
       */ set defaults(value) {
                if (value != null) __classPrivateFieldSet(this, _Dotenv__defaults, value, "f");
            }
            /**
       * Get encoding.
       */ get encoding() {
                return __classPrivateFieldGet(this, _Dotenv__encoding, "f");
            }
            /**
       * Set encoding.
       * @param value
       */ set encoding(value) {
                if (value != null) __classPrivateFieldSet(this, _Dotenv__encoding, value, "f");
            }
            /**
       * Get dotenv-expand plugin enabling.
       */ get expand() {
                return __classPrivateFieldGet(this, _Dotenv__expand, "f");
            }
            /**
       * Turn on/off dotenv-expand plugin.
       */ set expand(value) {
                if (value != null) __classPrivateFieldSet(this, _Dotenv__expand, value, "f");
            }
            /**
       * Get extension.
       */ get extension() {
                return __classPrivateFieldGet(this, _Dotenv__extension, "f");
            }
            /**
       * Set extension.
       */ set extension(value) {
                if (value != null) __classPrivateFieldSet(this, _Dotenv__extension, value.replace(/^\.+/, "").replace(/\.+$/, ""), "f");
            }
            /**
       * Get current working directory.
       */ get cwd() {
                var _a;
                if (!__classPrivateFieldGet(this, _Dotenv__cwd, "f")) return (_a = process.cwd()) !== null && _a !== void 0 ? _a : "";
                return __classPrivateFieldGet(this, _Dotenv__cwd, "f");
            }
            /**
       * Set current working directory.
       * @param value
       */ set cwd(value) {
                __classPrivateFieldSet(this, _Dotenv__cwd, value !== null && value !== void 0 ? value : "", "f");
            }
            /**
       * Get depth.
       */ get depth() {
                return __classPrivateFieldGet(this, _Dotenv__depth, "f");
            }
            /**
       * Set depth.
       * @param value
       */ set depth(value) {
                if (value != null) __classPrivateFieldSet(this, _Dotenv__depth, value, "f");
            }
            /**
       * Get override.
       */ get override() {
                return __classPrivateFieldGet(this, _Dotenv__override, "f");
            }
            /**
       * Set override.
       * @param value
       */ set override(value) {
                if (value != null) __classPrivateFieldSet(this, _Dotenv__override, value, "f");
            }
            /**
       * Get path.
       */ get path() {
                return __classPrivateFieldGet(this, _Dotenv__path, "f");
            }
            /**
       * Set path.
       */ set path(value) {
                if (value != null) __classPrivateFieldSet(this, _Dotenv__path, value, "f");
            }
            /**
       * Get priorities.
       */ get priorities() {
                var _a;
                const nodeEnv = (_a = ("TURBOPACK compile-time value", "development")) !== null && _a !== void 0 ? _a : "development";
                const ext = this.extension ? `.${this.extension}` : "";
                const priorities = Object.assign({
                    [`.env${ext}.${nodeEnv}.local`]: 75,
                    [`.env${ext}.local`]: 50,
                    [`.env${ext}.${nodeEnv}`]: 25,
                    [`.env${ext}`]: 1
                }, __classPrivateFieldGet(this, _Dotenv__priorities, "f"));
                return priorities;
            }
            /**
       * Merge priorities specified with default and check NODE_ENV.
       * @param value
       */ set priorities(value) {
                if (value != null) __classPrivateFieldSet(this, _Dotenv__priorities, value, "f");
            }
            /**
       * Parses a string or buffer in the .env file format into an object.
       * @see https://docs.dotenv.org
       * @returns an object with keys and values based on `src`. example: `{ DB_HOST : 'localhost' }`
       */ parse(src) {
                return dotenv_1.default.parse(src);
            }
            /**
       * Loads `.env` and default file contents.
       * @param loadOnProcess - load contents inside process
       * @returns current instance
       */ load(loadOnProcess = true) {
                this.env = {};
                this.config = {};
                const file = this.path || this.find();
                this.loadDotenv(file, loadOnProcess);
                const defaultFile = this.find(this.dotenvDefaultsMatcher);
                this.loadDotenv(defaultFile, loadOnProcess, true);
                return this;
            }
            /**
       * Load with dotenv package and set parsed and plain content into the instance.
       * @private
       * @param file - path to dotenv
       * @param loadOnProcess - load contents inside process
       * @param defaults - is the default dotenv
       */ loadDotenv(file, loadOnProcess, defaults = false) {
                if (!file || !fs_1.default.existsSync(file)) return;
                const plain = fs_1.default.readFileSync(file, {
                    encoding: this.encoding,
                    flag: "r"
                });
                const config = loadOnProcess ? dotenv_1.default.config({
                    path: file,
                    debug: this.debug,
                    encoding: this.encoding,
                    override: !defaults && this.override
                }) : {
                    parsed: this.parse(plain),
                    ignoreProcessEnv: true
                };
                if (this.expand) dotenv_expand_1.default.expand(config);
                this.mergeDotenvConfig(config);
                if (!defaults) this.plain = plain;
            }
            /**
       * Merge dotenv package configs.
       * @private
       * @param config - dotenv config
       */ mergeDotenvConfig(config) {
                var _a, _b, _c, _d, _e;
                this.config = {
                    parsed: Object.assign(Object.assign({}, (_a = this.config.parsed) !== null && _a !== void 0 ? _a : {}), (_b = config.parsed) !== null && _b !== void 0 ? _b : {}),
                    error: (_d = (_c = this.config.error) !== null && _c !== void 0 ? _c : config.error) !== null && _d !== void 0 ? _d : void 0
                };
                this.env = Object.assign(Object.assign({}, this.env), (_e = this.config.parsed) !== null && _e !== void 0 ? _e : {});
            }
            /**
       * Loads `.env` file contents.
       * @returns current instance
       */ loadFile() {
                this.load(false);
                return this;
            }
            /**
       * Find first `.env` file walking up from cwd directory based on priority criteria.
       * @returns file matched with higher priority
       */ find(matcher) {
                if (!matcher) matcher = this.dotenvMatcher;
                let dotenv = null;
                let directory = path_1.default.resolve(this.cwd);
                const { root } = path_1.default.parse(directory);
                let depth = 0;
                let match = false;
                while(depth < this.depth){
                    depth++;
                    const { foundPath, foundDotenv } = matcher(dotenv, directory);
                    dotenv = foundDotenv;
                    if (match) break;
                    if (foundPath) match = true;
                    if (directory === root) break;
                    directory = path_1.default.dirname(directory);
                }
                return dotenv;
            }
            /**
       * Dotenv matcher.
       * @private
       * @param dotenv - dotenv result
       * @param cwd - current working directory
       * @returns paths found
       */ dotenvMatcher(dotenv, cwd) {
                const priority = -1;
                Object.keys(this.priorities).forEach((fileName)=>{
                    if (this.priorities[fileName] > priority && fs_1.default.existsSync(path_1.default.join(cwd, fileName))) {
                        dotenv = path_1.default.join(cwd, fileName);
                    }
                });
                const foundPath = dotenv != null ? cwd : null;
                if (typeof foundPath === "string") {
                    try {
                        const stat = fs_1.default.statSync(path_1.default.resolve(cwd, foundPath));
                        if (stat.isDirectory()) return {
                            foundPath,
                            foundDotenv: dotenv
                        };
                    } catch (_a) {}
                }
                return {
                    foundPath,
                    foundDotenv: dotenv
                };
            }
            /**
       * Defaults dotenv matcher.
       * @private
       * @param dotenv - dotenv result
       * @param cwd - current working directory
       * @returns paths found
       */ dotenvDefaultsMatcher(dotenv, cwd) {
                if (fs_1.default.existsSync(path_1.default.join(cwd, this.defaults))) {
                    dotenv = path_1.default.join(cwd, this.defaults);
                }
                const foundPath = dotenv != null ? cwd : null;
                if (typeof foundPath === "string") {
                    try {
                        const stat = fs_1.default.statSync(path_1.default.resolve(cwd, foundPath));
                        if (stat.isDirectory()) return {
                            foundPath,
                            foundDotenv: dotenv
                        };
                    } catch (_a) {}
                }
                return {
                    foundPath,
                    foundDotenv: dotenv
                };
            }
            /**
       * Save `.env` file contents.
       * @param changes - data to change on the dotenv
       * @returns current instance
       */ save(changes) {
                const file = this.path || this.find();
                if (!file || !fs_1.default.existsSync(file)) return this;
                const EOL = os_1.default.EOL;
                const breakPattern = /\n/g;
                const breakReplacement = "\\n";
                const flags = "gm";
                const groupPattern = /\$/g;
                const groupReplacement = "$$$";
                const h = "[^\\S\\r\\n]";
                const returnPattern = /\r/g;
                const returnReplacement = "\\r";
                const endsWithEOL = (string)=>string.endsWith("\n") || string.endsWith("\r\n");
                let hasAppended = false;
                const data = Object.keys(changes).reduce((result, variable)=>{
                    const value = changes[variable].replace(breakPattern, breakReplacement).replace(returnPattern, returnReplacement).trim();
                    const safeName = this.escapeRegExp(variable);
                    const varPattern = new RegExp(`^(${h}*${safeName}${h}*=${h}*).*?(${h}*)$`, flags);
                    if (varPattern.test(result)) {
                        const safeValue = value.replace(groupPattern, groupReplacement);
                        return result.replace(varPattern, `$1${safeValue}$2`);
                    } else if (result === "") {
                        hasAppended = true;
                        return `${variable}=${value}${EOL}`;
                    } else if (!endsWithEOL(result) && !hasAppended) {
                        hasAppended = true;
                        return `${result}${EOL}${EOL}${variable}=${value}`;
                    } else if (!endsWithEOL(result)) {
                        return `${result}${EOL}${variable}=${value}`;
                    } else if (endsWithEOL(result) && !hasAppended) {
                        hasAppended = true;
                        return `${result}${EOL}${variable}=${value}${EOL}`;
                    } else {
                        return `${result}${variable}=${value}${EOL}`;
                    }
                }, this.plain);
                fs_1.default.writeFileSync(file, data, {
                    encoding: this.encoding
                });
                this.plain = data;
                return this;
            }
            /**
       * Escape regex.
       * @param string - string to escape
       * @returns escaped string
       */ escapeRegExp(string) {
                return string.replace(/[|\\{}()[\]^$+*?.]/g, "\\$&").replace(/-/g, "\\x2d");
            }
        };
        exports.Dotenv = Dotenv;
        _Dotenv__cwd = /* @__PURE__ */ new WeakMap(), _Dotenv__debug = /* @__PURE__ */ new WeakMap(), _Dotenv__defaults = /* @__PURE__ */ new WeakMap(), _Dotenv__depth = /* @__PURE__ */ new WeakMap(), _Dotenv__encoding = /* @__PURE__ */ new WeakMap(), _Dotenv__expand = /* @__PURE__ */ new WeakMap(), _Dotenv__extension = /* @__PURE__ */ new WeakMap(), _Dotenv__override = /* @__PURE__ */ new WeakMap(), _Dotenv__path = /* @__PURE__ */ new WeakMap(), _Dotenv__priorities = /* @__PURE__ */ new WeakMap();
        function dotenvLoad(props) {
            const dotenv = new Dotenv(props);
            return dotenv.load();
        }
        exports.dotenvLoad = dotenvLoad;
        exports.load = dotenvLoad;
        function dotenvConfig(props) {
            const dotenv = new Dotenv(props);
            return dotenv.load().config;
        }
        exports.dotenvConfig = dotenvConfig;
        exports.config = dotenvConfig;
        exports.default = Dotenv;
    }
});
const __TURBOPACK__default__export__ = require_dist();
}),
];

//# sourceMappingURL=02e36_basehub_dist_46c27fdc._.js.map