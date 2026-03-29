var __defProp = Object.defineProperty;
var __name = (target, value) => __defProp(target, "name", { value, configurable: true });
var __export = (target, all5) => {
  for (var name in all5)
    __defProp(target, name, { get: all5[name], enumerable: true });
};

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/Function.js
var isFunction = /* @__PURE__ */ __name((input) => typeof input === "function", "isFunction");
var dual = /* @__PURE__ */ __name(function(arity, body) {
  if (typeof arity === "function") {
    return function() {
      if (arity(arguments)) {
        return body.apply(this, arguments);
      }
      return (self) => body(self, ...arguments);
    };
  }
  switch (arity) {
    case 0:
    case 1:
      throw new RangeError(`Invalid arity ${arity}`);
    case 2:
      return function(a, b) {
        if (arguments.length >= 2) {
          return body(a, b);
        }
        return function(self) {
          return body(self, a);
        };
      };
    case 3:
      return function(a, b, c) {
        if (arguments.length >= 3) {
          return body(a, b, c);
        }
        return function(self) {
          return body(self, a, b);
        };
      };
    case 4:
      return function(a, b, c, d) {
        if (arguments.length >= 4) {
          return body(a, b, c, d);
        }
        return function(self) {
          return body(self, a, b, c);
        };
      };
    case 5:
      return function(a, b, c, d, e) {
        if (arguments.length >= 5) {
          return body(a, b, c, d, e);
        }
        return function(self) {
          return body(self, a, b, c, d);
        };
      };
    default:
      return function() {
        if (arguments.length >= arity) {
          return body.apply(this, arguments);
        }
        const args2 = arguments;
        return function(self) {
          return body(self, ...args2);
        };
      };
  }
}, "dual");
var identity = /* @__PURE__ */ __name((a) => a, "identity");
var constant = /* @__PURE__ */ __name((value) => () => value, "constant");
var constTrue = /* @__PURE__ */ constant(true);
var constFalse = /* @__PURE__ */ constant(false);
var constUndefined = /* @__PURE__ */ constant(void 0);
var constVoid = constUndefined;
function pipe(a, ab, bc, cd, de, ef, fg, gh, hi) {
  switch (arguments.length) {
    case 1:
      return a;
    case 2:
      return ab(a);
    case 3:
      return bc(ab(a));
    case 4:
      return cd(bc(ab(a)));
    case 5:
      return de(cd(bc(ab(a))));
    case 6:
      return ef(de(cd(bc(ab(a)))));
    case 7:
      return fg(ef(de(cd(bc(ab(a))))));
    case 8:
      return gh(fg(ef(de(cd(bc(ab(a)))))));
    case 9:
      return hi(gh(fg(ef(de(cd(bc(ab(a))))))));
    default: {
      let ret = arguments[0];
      for (let i = 1; i < arguments.length; i++) {
        ret = arguments[i](ret);
      }
      return ret;
    }
  }
}
__name(pipe, "pipe");

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/Equivalence.js
var make = /* @__PURE__ */ __name((isEquivalent) => (self, that) => self === that || isEquivalent(self, that), "make");
var mapInput = /* @__PURE__ */ dual(2, (self, f) => make((x, y) => self(f(x), f(y))));
var array = /* @__PURE__ */ __name((item) => make((self, that) => {
  if (self.length !== that.length) {
    return false;
  }
  for (let i = 0; i < self.length; i++) {
    const isEq = item(self[i], that[i]);
    if (!isEq) {
      return false;
    }
  }
  return true;
}), "array");

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/internal/doNotation.js
var let_ = /* @__PURE__ */ __name((map14) => dual(3, (self, name, f) => map14(self, (a) => ({
  ...a,
  [name]: f(a)
}))), "let_");
var bindTo = /* @__PURE__ */ __name((map14) => dual(2, (self, name) => map14(self, (a) => ({
  [name]: a
}))), "bindTo");
var bind = /* @__PURE__ */ __name((map14, flatMap12) => dual(3, (self, name, f) => flatMap12(self, (a) => map14(f(a), (b) => ({
  ...a,
  [name]: b
})))), "bind");

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/GlobalValue.js
var globalStoreId = `effect/GlobalValue`;
var globalStore;
var globalValue = /* @__PURE__ */ __name((id, compute) => {
  if (!globalStore) {
    globalThis[globalStoreId] ??= /* @__PURE__ */ new Map();
    globalStore = globalThis[globalStoreId];
  }
  if (!globalStore.has(id)) {
    globalStore.set(id, compute());
  }
  return globalStore.get(id);
}, "globalValue");

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/Predicate.js
var isString = /* @__PURE__ */ __name((input) => typeof input === "string", "isString");
var isNumber = /* @__PURE__ */ __name((input) => typeof input === "number", "isNumber");
var isBigInt = /* @__PURE__ */ __name((input) => typeof input === "bigint", "isBigInt");
var isFunction2 = isFunction;
var isRecordOrArray = /* @__PURE__ */ __name((input) => typeof input === "object" && input !== null, "isRecordOrArray");
var isObject = /* @__PURE__ */ __name((input) => isRecordOrArray(input) || isFunction2(input), "isObject");
var hasProperty = /* @__PURE__ */ dual(2, (self, property) => isObject(self) && property in self);
var isTagged = /* @__PURE__ */ dual(2, (self, tag) => hasProperty(self, "_tag") && self["_tag"] === tag);
var isNullable = /* @__PURE__ */ __name((input) => input === null || input === void 0, "isNullable");
var isIterable = /* @__PURE__ */ __name((input) => typeof input === "string" || hasProperty(input, Symbol.iterator), "isIterable");
var isPromiseLike = /* @__PURE__ */ __name((input) => hasProperty(input, "then") && isFunction2(input.then), "isPromiseLike");

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/internal/errors.js
var getBugErrorMessage = /* @__PURE__ */ __name((message) => `BUG: ${message} - please report an issue at https://github.com/Effect-TS/effect/issues`, "getBugErrorMessage");

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/Utils.js
var GenKindTypeId = /* @__PURE__ */ Symbol.for("effect/Gen/GenKind");
var GenKindImpl = class {
  static {
    __name(this, "GenKindImpl");
  }
  value;
  constructor(value) {
    this.value = value;
  }
  /**
   * @since 2.0.0
   */
  get _F() {
    return identity;
  }
  /**
   * @since 2.0.0
   */
  get _R() {
    return (_) => _;
  }
  /**
   * @since 2.0.0
   */
  get _O() {
    return (_) => _;
  }
  /**
   * @since 2.0.0
   */
  get _E() {
    return (_) => _;
  }
  /**
   * @since 2.0.0
   */
  [GenKindTypeId] = GenKindTypeId;
  /**
   * @since 2.0.0
   */
  [Symbol.iterator]() {
    return new SingleShotGen(this);
  }
};
var SingleShotGen = class _SingleShotGen {
  static {
    __name(this, "SingleShotGen");
  }
  self;
  called = false;
  constructor(self) {
    this.self = self;
  }
  /**
   * @since 2.0.0
   */
  next(a) {
    return this.called ? {
      value: a,
      done: true
    } : (this.called = true, {
      value: this.self,
      done: false
    });
  }
  /**
   * @since 2.0.0
   */
  return(a) {
    return {
      value: a,
      done: true
    };
  }
  /**
   * @since 2.0.0
   */
  throw(e) {
    throw e;
  }
  /**
   * @since 2.0.0
   */
  [Symbol.iterator]() {
    return new _SingleShotGen(this.self);
  }
};
var defaultIncHi = 335903614;
var defaultIncLo = 4150755663;
var MUL_HI = 1481765933 >>> 0;
var MUL_LO = 1284865837 >>> 0;
var BIT_53 = 9007199254740992;
var BIT_27 = 134217728;
var PCGRandom = class {
  static {
    __name(this, "PCGRandom");
  }
  _state;
  constructor(seedHi, seedLo, incHi, incLo) {
    if (isNullable(seedLo) && isNullable(seedHi)) {
      seedLo = Math.random() * 4294967295 >>> 0;
      seedHi = 0;
    } else if (isNullable(seedLo)) {
      seedLo = seedHi;
      seedHi = 0;
    }
    if (isNullable(incLo) && isNullable(incHi)) {
      incLo = this._state ? this._state[3] : defaultIncLo;
      incHi = this._state ? this._state[2] : defaultIncHi;
    } else if (isNullable(incLo)) {
      incLo = incHi;
      incHi = 0;
    }
    this._state = new Int32Array([0, 0, incHi >>> 0, ((incLo || 0) | 1) >>> 0]);
    this._next();
    add64(this._state, this._state[0], this._state[1], seedHi >>> 0, seedLo >>> 0);
    this._next();
    return this;
  }
  /**
   * Returns a copy of the internal state of this random number generator as a
   * JavaScript Array.
   *
   * @category getters
   * @since 2.0.0
   */
  getState() {
    return [this._state[0], this._state[1], this._state[2], this._state[3]];
  }
  /**
   * Restore state previously retrieved using `getState()`.
   *
   * @since 2.0.0
   */
  setState(state) {
    this._state[0] = state[0];
    this._state[1] = state[1];
    this._state[2] = state[2];
    this._state[3] = state[3] | 1;
  }
  /**
   * Get a uniformly distributed 32 bit integer between [0, max).
   *
   * @category getter
   * @since 2.0.0
   */
  integer(max6) {
    return Math.round(this.number() * Number.MAX_SAFE_INTEGER) % max6;
  }
  /**
   * Get a uniformly distributed IEEE-754 double between 0.0 and 1.0, with
   * 53 bits of precision (every bit of the mantissa is randomized).
   *
   * @category getters
   * @since 2.0.0
   */
  number() {
    const hi = (this._next() & 67108863) * 1;
    const lo = (this._next() & 134217727) * 1;
    return (hi * BIT_27 + lo) / BIT_53;
  }
  /** @internal */
  _next() {
    const oldHi = this._state[0] >>> 0;
    const oldLo = this._state[1] >>> 0;
    mul64(this._state, oldHi, oldLo, MUL_HI, MUL_LO);
    add64(this._state, this._state[0], this._state[1], this._state[2], this._state[3]);
    let xsHi = oldHi >>> 18;
    let xsLo = (oldLo >>> 18 | oldHi << 14) >>> 0;
    xsHi = (xsHi ^ oldHi) >>> 0;
    xsLo = (xsLo ^ oldLo) >>> 0;
    const xorshifted = (xsLo >>> 27 | xsHi << 5) >>> 0;
    const rot = oldHi >>> 27;
    const rot2 = (-rot >>> 0 & 31) >>> 0;
    return (xorshifted >>> rot | xorshifted << rot2) >>> 0;
  }
};
function mul64(out, aHi, aLo, bHi, bLo) {
  let c1 = (aLo >>> 16) * (bLo & 65535) >>> 0;
  let c0 = (aLo & 65535) * (bLo >>> 16) >>> 0;
  let lo = (aLo & 65535) * (bLo & 65535) >>> 0;
  let hi = (aLo >>> 16) * (bLo >>> 16) + ((c0 >>> 16) + (c1 >>> 16)) >>> 0;
  c0 = c0 << 16 >>> 0;
  lo = lo + c0 >>> 0;
  if (lo >>> 0 < c0 >>> 0) {
    hi = hi + 1 >>> 0;
  }
  c1 = c1 << 16 >>> 0;
  lo = lo + c1 >>> 0;
  if (lo >>> 0 < c1 >>> 0) {
    hi = hi + 1 >>> 0;
  }
  hi = hi + Math.imul(aLo, bHi) >>> 0;
  hi = hi + Math.imul(aHi, bLo) >>> 0;
  out[0] = hi;
  out[1] = lo;
}
__name(mul64, "mul64");
function add64(out, aHi, aLo, bHi, bLo) {
  let hi = aHi + bHi >>> 0;
  const lo = aLo + bLo >>> 0;
  if (lo >>> 0 < aLo >>> 0) {
    hi = hi + 1 | 0;
  }
  out[0] = hi;
  out[1] = lo;
}
__name(add64, "add64");
var YieldWrapTypeId = /* @__PURE__ */ Symbol.for("effect/Utils/YieldWrap");
var YieldWrap = class {
  static {
    __name(this, "YieldWrap");
  }
  /**
   * @since 3.0.6
   */
  #value;
  constructor(value) {
    this.#value = value;
  }
  /**
   * @since 3.0.6
   */
  [YieldWrapTypeId]() {
    return this.#value;
  }
};
function yieldWrapGet(self) {
  if (typeof self === "object" && self !== null && YieldWrapTypeId in self) {
    return self[YieldWrapTypeId]();
  }
  throw new Error(getBugErrorMessage("yieldWrapGet"));
}
__name(yieldWrapGet, "yieldWrapGet");
var structuralRegionState = /* @__PURE__ */ globalValue("effect/Utils/isStructuralRegion", () => ({
  enabled: false,
  tester: void 0
}));
var standard = {
  effect_internal_function: /* @__PURE__ */ __name((body) => {
    return body();
  }, "effect_internal_function")
};
var forced = {
  effect_internal_function: /* @__PURE__ */ __name((body) => {
    try {
      return body();
    } finally {
    }
  }, "effect_internal_function")
};
var isNotOptimizedAway = /* @__PURE__ */ standard.effect_internal_function(() => new Error().stack)?.includes("effect_internal_function") === true;
var internalCall = isNotOptimizedAway ? standard.effect_internal_function : forced.effect_internal_function;
var genConstructor = function* () {
}.constructor;
var isGeneratorFunction = /* @__PURE__ */ __name((u) => isObject(u) && u.constructor === genConstructor, "isGeneratorFunction");

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/Hash.js
var randomHashCache = /* @__PURE__ */ globalValue(/* @__PURE__ */ Symbol.for("effect/Hash/randomHashCache"), () => /* @__PURE__ */ new WeakMap());
var symbol = /* @__PURE__ */ Symbol.for("effect/Hash");
var hash = /* @__PURE__ */ __name((self) => {
  if (structuralRegionState.enabled === true) {
    return 0;
  }
  switch (typeof self) {
    case "number":
      return number(self);
    case "bigint":
      return string(self.toString(10));
    case "boolean":
      return string(String(self));
    case "symbol":
      return string(String(self));
    case "string":
      return string(self);
    case "undefined":
      return string("undefined");
    case "function":
    case "object": {
      if (self === null) {
        return string("null");
      } else if (self instanceof Date) {
        if (Number.isNaN(self.getTime())) {
          return string("Invalid Date");
        }
        return hash(self.toISOString());
      } else if (self instanceof URL) {
        return hash(self.href);
      } else if (isHash(self)) {
        return self[symbol]();
      } else {
        return random(self);
      }
    }
    default:
      throw new Error(`BUG: unhandled typeof ${typeof self} - please report an issue at https://github.com/Effect-TS/effect/issues`);
  }
}, "hash");
var random = /* @__PURE__ */ __name((self) => {
  if (!randomHashCache.has(self)) {
    randomHashCache.set(self, number(Math.floor(Math.random() * Number.MAX_SAFE_INTEGER)));
  }
  return randomHashCache.get(self);
}, "random");
var combine = /* @__PURE__ */ __name((b) => (self) => self * 53 ^ b, "combine");
var optimize = /* @__PURE__ */ __name((n) => n & 3221225471 | n >>> 1 & 1073741824, "optimize");
var isHash = /* @__PURE__ */ __name((u) => hasProperty(u, symbol), "isHash");
var number = /* @__PURE__ */ __name((n) => {
  if (n !== n || n === Infinity) {
    return 0;
  }
  let h = n | 0;
  if (h !== n) {
    h ^= n * 4294967295;
  }
  while (n > 4294967295) {
    h ^= n /= 4294967295;
  }
  return optimize(h);
}, "number");
var string = /* @__PURE__ */ __name((str) => {
  let h = 5381, i = str.length;
  while (i) {
    h = h * 33 ^ str.charCodeAt(--i);
  }
  return optimize(h);
}, "string");
var structureKeys = /* @__PURE__ */ __name((o, keys5) => {
  let h = 12289;
  for (let i = 0; i < keys5.length; i++) {
    h ^= pipe(string(keys5[i]), combine(hash(o[keys5[i]])));
  }
  return optimize(h);
}, "structureKeys");
var structure = /* @__PURE__ */ __name((o) => structureKeys(o, Object.keys(o)), "structure");
var array2 = /* @__PURE__ */ __name((arr) => {
  let h = 6151;
  for (let i = 0; i < arr.length; i++) {
    h = pipe(h, combine(hash(arr[i])));
  }
  return optimize(h);
}, "array");
var cached = /* @__PURE__ */ __name(function() {
  if (arguments.length === 1) {
    const self2 = arguments[0];
    return function(hash3) {
      Object.defineProperty(self2, symbol, {
        value() {
          return hash3;
        },
        enumerable: false
      });
      return hash3;
    };
  }
  const self = arguments[0];
  const hash2 = arguments[1];
  Object.defineProperty(self, symbol, {
    value() {
      return hash2;
    },
    enumerable: false
  });
  return hash2;
}, "cached");

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/Equal.js
var symbol2 = /* @__PURE__ */ Symbol.for("effect/Equal");
function equals() {
  if (arguments.length === 1) {
    return (self) => compareBoth(self, arguments[0]);
  }
  return compareBoth(arguments[0], arguments[1]);
}
__name(equals, "equals");
function compareBoth(self, that) {
  if (self === that) {
    return true;
  }
  const selfType = typeof self;
  if (selfType !== typeof that) {
    return false;
  }
  if (selfType === "object" || selfType === "function") {
    if (self !== null && that !== null) {
      if (isEqual(self) && isEqual(that)) {
        if (hash(self) === hash(that) && self[symbol2](that)) {
          return true;
        } else {
          return structuralRegionState.enabled && structuralRegionState.tester ? structuralRegionState.tester(self, that) : false;
        }
      } else if (self instanceof Date && that instanceof Date) {
        const t1 = self.getTime();
        const t2 = that.getTime();
        return t1 === t2 || Number.isNaN(t1) && Number.isNaN(t2);
      } else if (self instanceof URL && that instanceof URL) {
        return self.href === that.href;
      }
    }
    if (structuralRegionState.enabled) {
      if (self === null || that === null) {
        return false;
      }
      if (Array.isArray(self) && Array.isArray(that)) {
        return self.length === that.length && self.every((v, i) => compareBoth(v, that[i]));
      }
      if (Object.getPrototypeOf(self) === Object.prototype && Object.getPrototypeOf(that) === Object.prototype) {
        const keysSelf = Object.keys(self);
        const keysThat = Object.keys(that);
        if (keysSelf.length === keysThat.length) {
          for (const key of keysSelf) {
            if (!(key in that && compareBoth(self[key], that[key]))) {
              return structuralRegionState.tester ? structuralRegionState.tester(self, that) : false;
            }
          }
          return true;
        }
      }
      return structuralRegionState.tester ? structuralRegionState.tester(self, that) : false;
    }
  }
  return structuralRegionState.enabled && structuralRegionState.tester ? structuralRegionState.tester(self, that) : false;
}
__name(compareBoth, "compareBoth");
var isEqual = /* @__PURE__ */ __name((u) => hasProperty(u, symbol2), "isEqual");
var equivalence = /* @__PURE__ */ __name(() => equals, "equivalence");

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/Inspectable.js
var NodeInspectSymbol = /* @__PURE__ */ Symbol.for("nodejs.util.inspect.custom");
var toJSON = /* @__PURE__ */ __name((x) => {
  try {
    if (hasProperty(x, "toJSON") && isFunction2(x["toJSON"]) && x["toJSON"].length === 0) {
      return x.toJSON();
    } else if (Array.isArray(x)) {
      return x.map(toJSON);
    }
  } catch {
    return {};
  }
  return redact(x);
}, "toJSON");
var format = /* @__PURE__ */ __name((x) => JSON.stringify(x, null, 2), "format");
var BaseProto = {
  toJSON() {
    return toJSON(this);
  },
  [NodeInspectSymbol]() {
    return this.toJSON();
  },
  toString() {
    return format(this.toJSON());
  }
};
var Class = class {
  static {
    __name(this, "Class");
  }
  /**
   * @since 2.0.0
   */
  [NodeInspectSymbol]() {
    return this.toJSON();
  }
  /**
   * @since 2.0.0
   */
  toString() {
    return format(this.toJSON());
  }
};
var toStringUnknown = /* @__PURE__ */ __name((u, whitespace = 2) => {
  if (typeof u === "string") {
    return u;
  }
  try {
    return typeof u === "object" ? stringifyCircular(u, whitespace) : String(u);
  } catch {
    return String(u);
  }
}, "toStringUnknown");
var stringifyCircular = /* @__PURE__ */ __name((obj, whitespace) => {
  let cache = [];
  const retVal = JSON.stringify(obj, (_key, value) => typeof value === "object" && value !== null ? cache.includes(value) ? void 0 : cache.push(value) && (redactableState.fiberRefs !== void 0 && isRedactable(value) ? value[symbolRedactable](redactableState.fiberRefs) : value) : value, whitespace);
  cache = void 0;
  return retVal;
}, "stringifyCircular");
var symbolRedactable = /* @__PURE__ */ Symbol.for("effect/Inspectable/Redactable");
var isRedactable = /* @__PURE__ */ __name((u) => typeof u === "object" && u !== null && symbolRedactable in u, "isRedactable");
var redactableState = /* @__PURE__ */ globalValue("effect/Inspectable/redactableState", () => ({
  fiberRefs: void 0
}));
var withRedactableContext = /* @__PURE__ */ __name((context4, f) => {
  const prev = redactableState.fiberRefs;
  redactableState.fiberRefs = context4;
  try {
    return f();
  } finally {
    redactableState.fiberRefs = prev;
  }
}, "withRedactableContext");
var redact = /* @__PURE__ */ __name((u) => {
  if (isRedactable(u) && redactableState.fiberRefs !== void 0) {
    return u[symbolRedactable](redactableState.fiberRefs);
  }
  return u;
}, "redact");

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/Pipeable.js
var pipeArguments = /* @__PURE__ */ __name((self, args2) => {
  switch (args2.length) {
    case 0:
      return self;
    case 1:
      return args2[0](self);
    case 2:
      return args2[1](args2[0](self));
    case 3:
      return args2[2](args2[1](args2[0](self)));
    case 4:
      return args2[3](args2[2](args2[1](args2[0](self))));
    case 5:
      return args2[4](args2[3](args2[2](args2[1](args2[0](self)))));
    case 6:
      return args2[5](args2[4](args2[3](args2[2](args2[1](args2[0](self))))));
    case 7:
      return args2[6](args2[5](args2[4](args2[3](args2[2](args2[1](args2[0](self)))))));
    case 8:
      return args2[7](args2[6](args2[5](args2[4](args2[3](args2[2](args2[1](args2[0](self))))))));
    case 9:
      return args2[8](args2[7](args2[6](args2[5](args2[4](args2[3](args2[2](args2[1](args2[0](self)))))))));
    default: {
      let ret = self;
      for (let i = 0, len = args2.length; i < len; i++) {
        ret = args2[i](ret);
      }
      return ret;
    }
  }
}, "pipeArguments");

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/internal/opCodes/effect.js
var OP_ASYNC = "Async";
var OP_COMMIT = "Commit";
var OP_FAILURE = "Failure";
var OP_ON_FAILURE = "OnFailure";
var OP_ON_SUCCESS = "OnSuccess";
var OP_ON_SUCCESS_AND_FAILURE = "OnSuccessAndFailure";
var OP_SUCCESS = "Success";
var OP_SYNC = "Sync";
var OP_TAG = "Tag";
var OP_UPDATE_RUNTIME_FLAGS = "UpdateRuntimeFlags";
var OP_WHILE = "While";
var OP_ITERATOR = "Iterator";
var OP_WITH_RUNTIME = "WithRuntime";
var OP_YIELD = "Yield";
var OP_REVERT_FLAGS = "RevertFlags";

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/internal/version.js
var moduleVersion = "3.21.0";
var getCurrentVersion = /* @__PURE__ */ __name(() => moduleVersion, "getCurrentVersion");

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/internal/effectable.js
var EffectTypeId = /* @__PURE__ */ Symbol.for("effect/Effect");
var StreamTypeId = /* @__PURE__ */ Symbol.for("effect/Stream");
var SinkTypeId = /* @__PURE__ */ Symbol.for("effect/Sink");
var ChannelTypeId = /* @__PURE__ */ Symbol.for("effect/Channel");
var effectVariance = {
  /* c8 ignore next */
  _R: /* @__PURE__ */ __name((_) => _, "_R"),
  /* c8 ignore next */
  _E: /* @__PURE__ */ __name((_) => _, "_E"),
  /* c8 ignore next */
  _A: /* @__PURE__ */ __name((_) => _, "_A"),
  _V: /* @__PURE__ */ getCurrentVersion()
};
var sinkVariance = {
  /* c8 ignore next */
  _A: /* @__PURE__ */ __name((_) => _, "_A"),
  /* c8 ignore next */
  _In: /* @__PURE__ */ __name((_) => _, "_In"),
  /* c8 ignore next */
  _L: /* @__PURE__ */ __name((_) => _, "_L"),
  /* c8 ignore next */
  _E: /* @__PURE__ */ __name((_) => _, "_E"),
  /* c8 ignore next */
  _R: /* @__PURE__ */ __name((_) => _, "_R")
};
var channelVariance = {
  /* c8 ignore next */
  _Env: /* @__PURE__ */ __name((_) => _, "_Env"),
  /* c8 ignore next */
  _InErr: /* @__PURE__ */ __name((_) => _, "_InErr"),
  /* c8 ignore next */
  _InElem: /* @__PURE__ */ __name((_) => _, "_InElem"),
  /* c8 ignore next */
  _InDone: /* @__PURE__ */ __name((_) => _, "_InDone"),
  /* c8 ignore next */
  _OutErr: /* @__PURE__ */ __name((_) => _, "_OutErr"),
  /* c8 ignore next */
  _OutElem: /* @__PURE__ */ __name((_) => _, "_OutElem"),
  /* c8 ignore next */
  _OutDone: /* @__PURE__ */ __name((_) => _, "_OutDone")
};
var EffectPrototype = {
  [EffectTypeId]: effectVariance,
  [StreamTypeId]: effectVariance,
  [SinkTypeId]: sinkVariance,
  [ChannelTypeId]: channelVariance,
  [symbol2](that) {
    return this === that;
  },
  [symbol]() {
    return cached(this, random(this));
  },
  [Symbol.iterator]() {
    return new SingleShotGen(new YieldWrap(this));
  },
  pipe() {
    return pipeArguments(this, arguments);
  }
};
var StructuralPrototype = {
  [symbol]() {
    return cached(this, structure(this));
  },
  [symbol2](that) {
    const selfKeys = Object.keys(this);
    const thatKeys = Object.keys(that);
    if (selfKeys.length !== thatKeys.length) {
      return false;
    }
    for (const key of selfKeys) {
      if (!(key in that && equals(this[key], that[key]))) {
        return false;
      }
    }
    return true;
  }
};
var CommitPrototype = {
  ...EffectPrototype,
  _op: OP_COMMIT
};
var StructuralCommitPrototype = {
  ...CommitPrototype,
  ...StructuralPrototype
};
var Base = /* @__PURE__ */ (function() {
  function Base3() {
  }
  __name(Base3, "Base");
  Base3.prototype = CommitPrototype;
  return Base3;
})();

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/internal/option.js
var TypeId = /* @__PURE__ */ Symbol.for("effect/Option");
var CommonProto = {
  ...EffectPrototype,
  [TypeId]: {
    _A: /* @__PURE__ */ __name((_) => _, "_A")
  },
  [NodeInspectSymbol]() {
    return this.toJSON();
  },
  toString() {
    return format(this.toJSON());
  }
};
var SomeProto = /* @__PURE__ */ Object.assign(/* @__PURE__ */ Object.create(CommonProto), {
  _tag: "Some",
  _op: "Some",
  [symbol2](that) {
    return isOption(that) && isSome(that) && equals(this.value, that.value);
  },
  [symbol]() {
    return cached(this, combine(hash(this._tag))(hash(this.value)));
  },
  toJSON() {
    return {
      _id: "Option",
      _tag: this._tag,
      value: toJSON(this.value)
    };
  }
});
var NoneHash = /* @__PURE__ */ hash("None");
var NoneProto = /* @__PURE__ */ Object.assign(/* @__PURE__ */ Object.create(CommonProto), {
  _tag: "None",
  _op: "None",
  [symbol2](that) {
    return isOption(that) && isNone(that);
  },
  [symbol]() {
    return NoneHash;
  },
  toJSON() {
    return {
      _id: "Option",
      _tag: this._tag
    };
  }
});
var isOption = /* @__PURE__ */ __name((input) => hasProperty(input, TypeId), "isOption");
var isNone = /* @__PURE__ */ __name((fa) => fa._tag === "None", "isNone");
var isSome = /* @__PURE__ */ __name((fa) => fa._tag === "Some", "isSome");
var none = /* @__PURE__ */ Object.create(NoneProto);
var some = /* @__PURE__ */ __name((value) => {
  const a = Object.create(SomeProto);
  a.value = value;
  return a;
}, "some");

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/internal/either.js
var TypeId2 = /* @__PURE__ */ Symbol.for("effect/Either");
var CommonProto2 = {
  ...EffectPrototype,
  [TypeId2]: {
    _R: /* @__PURE__ */ __name((_) => _, "_R")
  },
  [NodeInspectSymbol]() {
    return this.toJSON();
  },
  toString() {
    return format(this.toJSON());
  }
};
var RightProto = /* @__PURE__ */ Object.assign(/* @__PURE__ */ Object.create(CommonProto2), {
  _tag: "Right",
  _op: "Right",
  [symbol2](that) {
    return isEither(that) && isRight(that) && equals(this.right, that.right);
  },
  [symbol]() {
    return combine(hash(this._tag))(hash(this.right));
  },
  toJSON() {
    return {
      _id: "Either",
      _tag: this._tag,
      right: toJSON(this.right)
    };
  }
});
var LeftProto = /* @__PURE__ */ Object.assign(/* @__PURE__ */ Object.create(CommonProto2), {
  _tag: "Left",
  _op: "Left",
  [symbol2](that) {
    return isEither(that) && isLeft(that) && equals(this.left, that.left);
  },
  [symbol]() {
    return combine(hash(this._tag))(hash(this.left));
  },
  toJSON() {
    return {
      _id: "Either",
      _tag: this._tag,
      left: toJSON(this.left)
    };
  }
});
var isEither = /* @__PURE__ */ __name((input) => hasProperty(input, TypeId2), "isEither");
var isLeft = /* @__PURE__ */ __name((ma) => ma._tag === "Left", "isLeft");
var isRight = /* @__PURE__ */ __name((ma) => ma._tag === "Right", "isRight");
var left = /* @__PURE__ */ __name((left3) => {
  const a = Object.create(LeftProto);
  a.left = left3;
  return a;
}, "left");
var right = /* @__PURE__ */ __name((right3) => {
  const a = Object.create(RightProto);
  a.right = right3;
  return a;
}, "right");

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/Either.js
var right2 = right;
var left2 = left;
var isLeft2 = isLeft;
var isRight2 = isRight;
var match = /* @__PURE__ */ dual(2, (self, {
  onLeft,
  onRight
}) => isLeft2(self) ? onLeft(self.left) : onRight(self.right));
var merge = /* @__PURE__ */ match({
  onLeft: identity,
  onRight: identity
});

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/internal/array.js
var isNonEmptyArray = /* @__PURE__ */ __name((self) => self.length > 0, "isNonEmptyArray");

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/Order.js
var make2 = /* @__PURE__ */ __name((compare) => (self, that) => self === that ? 0 : compare(self, that), "make");
var number2 = /* @__PURE__ */ make2((self, that) => self < that ? -1 : 1);
var mapInput2 = /* @__PURE__ */ dual(2, (self, f) => make2((b1, b2) => self(f(b1), f(b2))));
var lessThan = /* @__PURE__ */ __name((O) => dual(2, (self, that) => O(self, that) === -1), "lessThan");
var greaterThan = /* @__PURE__ */ __name((O) => dual(2, (self, that) => O(self, that) === 1), "greaterThan");
var min = /* @__PURE__ */ __name((O) => dual(2, (self, that) => self === that || O(self, that) < 1 ? self : that), "min");
var max = /* @__PURE__ */ __name((O) => dual(2, (self, that) => self === that || O(self, that) > -1 ? self : that), "max");
var clamp = /* @__PURE__ */ __name((O) => dual(2, (self, options) => min(O)(options.maximum, max(O)(options.minimum, self))), "clamp");
var between = /* @__PURE__ */ __name((O) => dual(2, (self, options) => !lessThan(O)(self, options.minimum) && !greaterThan(O)(self, options.maximum)), "between");

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/Option.js
var none2 = /* @__PURE__ */ __name(() => none, "none");
var some2 = some;
var isNone2 = isNone;
var isSome2 = isSome;
var match2 = /* @__PURE__ */ dual(2, (self, {
  onNone,
  onSome
}) => isNone2(self) ? onNone() : onSome(self.value));
var getOrElse = /* @__PURE__ */ dual(2, (self, onNone) => isNone2(self) ? onNone() : self.value);
var orElseSome = /* @__PURE__ */ dual(2, (self, onNone) => isNone2(self) ? some2(onNone()) : self);
var fromNullable = /* @__PURE__ */ __name((nullableValue) => nullableValue == null ? none2() : some2(nullableValue), "fromNullable");
var getOrUndefined = /* @__PURE__ */ getOrElse(constUndefined);
var liftThrowable = /* @__PURE__ */ __name((f) => (...a) => {
  try {
    return some2(f(...a));
  } catch {
    return none2();
  }
}, "liftThrowable");
var getOrThrowWith = /* @__PURE__ */ dual(2, (self, onNone) => {
  if (isSome2(self)) {
    return self.value;
  }
  throw onNone();
});
var getOrThrow = /* @__PURE__ */ getOrThrowWith(() => new Error("getOrThrow called on a None"));
var map = /* @__PURE__ */ dual(2, (self, f) => isNone2(self) ? none2() : some2(f(self.value)));
var flatMap = /* @__PURE__ */ dual(2, (self, f) => isNone2(self) ? none2() : f(self.value));
var containsWith = /* @__PURE__ */ __name((isEquivalent) => dual(2, (self, a) => isNone2(self) ? false : isEquivalent(self.value, a)), "containsWith");
var _equivalence = /* @__PURE__ */ equivalence();
var contains = /* @__PURE__ */ containsWith(_equivalence);
var mergeWith = /* @__PURE__ */ __name((f) => (o1, o2) => {
  if (isNone2(o1)) {
    return o2;
  } else if (isNone2(o2)) {
    return o1;
  }
  return some2(f(o1.value, o2.value));
}, "mergeWith");

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/Tuple.js
var make3 = /* @__PURE__ */ __name((...elements) => elements, "make");

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/Array.js
var allocate = /* @__PURE__ */ __name((n) => new Array(n), "allocate");
var makeBy = /* @__PURE__ */ dual(2, (n, f) => {
  const max6 = Math.max(1, Math.floor(n));
  const out = new Array(max6);
  for (let i = 0; i < max6; i++) {
    out[i] = f(i);
  }
  return out;
});
var fromIterable = /* @__PURE__ */ __name((collection) => Array.isArray(collection) ? collection : Array.from(collection), "fromIterable");
var ensure = /* @__PURE__ */ __name((self) => Array.isArray(self) ? self : [self], "ensure");
var prepend = /* @__PURE__ */ dual(2, (self, head5) => [head5, ...self]);
var append = /* @__PURE__ */ dual(2, (self, last3) => [...self, last3]);
var appendAll = /* @__PURE__ */ dual(2, (self, that) => fromIterable(self).concat(fromIterable(that)));
var isEmptyArray = /* @__PURE__ */ __name((self) => self.length === 0, "isEmptyArray");
var isEmptyReadonlyArray = isEmptyArray;
var isNonEmptyArray2 = isNonEmptyArray;
var isNonEmptyReadonlyArray = isNonEmptyArray;
var isOutOfBounds = /* @__PURE__ */ __name((i, as7) => i < 0 || i >= as7.length, "isOutOfBounds");
var clamp2 = /* @__PURE__ */ __name((i, as7) => Math.floor(Math.min(Math.max(0, i), as7.length)), "clamp");
var get = /* @__PURE__ */ dual(2, (self, index) => {
  const i = Math.floor(index);
  return isOutOfBounds(i, self) ? none2() : some2(self[i]);
});
var unsafeGet = /* @__PURE__ */ dual(2, (self, index) => {
  const i = Math.floor(index);
  if (isOutOfBounds(i, self)) {
    throw new Error(`Index ${i} out of bounds`);
  }
  return self[i];
});
var head = /* @__PURE__ */ get(0);
var headNonEmpty = /* @__PURE__ */ unsafeGet(0);
var last = /* @__PURE__ */ __name((self) => isNonEmptyReadonlyArray(self) ? some2(lastNonEmpty(self)) : none2(), "last");
var lastNonEmpty = /* @__PURE__ */ __name((self) => self[self.length - 1], "lastNonEmpty");
var tailNonEmpty = /* @__PURE__ */ __name((self) => self.slice(1), "tailNonEmpty");
var spanIndex = /* @__PURE__ */ __name((self, predicate) => {
  let i = 0;
  for (const a of self) {
    if (!predicate(a, i)) {
      break;
    }
    i++;
  }
  return i;
}, "spanIndex");
var span = /* @__PURE__ */ dual(2, (self, predicate) => splitAt(self, spanIndex(self, predicate)));
var drop = /* @__PURE__ */ dual(2, (self, n) => {
  const input = fromIterable(self);
  return input.slice(clamp2(n, input), input.length);
});
var reverse = /* @__PURE__ */ __name((self) => Array.from(self).reverse(), "reverse");
var sort = /* @__PURE__ */ dual(2, (self, O) => {
  const out = Array.from(self);
  out.sort(O);
  return out;
});
var zip = /* @__PURE__ */ dual(2, (self, that) => zipWith(self, that, make3));
var zipWith = /* @__PURE__ */ dual(3, (self, that, f) => {
  const as7 = fromIterable(self);
  const bs = fromIterable(that);
  if (isNonEmptyReadonlyArray(as7) && isNonEmptyReadonlyArray(bs)) {
    const out = [f(headNonEmpty(as7), headNonEmpty(bs))];
    const len = Math.min(as7.length, bs.length);
    for (let i = 1; i < len; i++) {
      out[i] = f(as7[i], bs[i]);
    }
    return out;
  }
  return [];
});
var _equivalence2 = /* @__PURE__ */ equivalence();
var splitAt = /* @__PURE__ */ dual(2, (self, n) => {
  const input = Array.from(self);
  const _n = Math.floor(n);
  if (isNonEmptyReadonlyArray(input)) {
    if (_n >= 1) {
      return splitNonEmptyAt(input, _n);
    }
    return [[], input];
  }
  return [input, []];
});
var splitNonEmptyAt = /* @__PURE__ */ dual(2, (self, n) => {
  const _n = Math.max(1, Math.floor(n));
  return _n >= self.length ? [copy(self), []] : [prepend(self.slice(1, _n), headNonEmpty(self)), self.slice(_n)];
});
var copy = /* @__PURE__ */ __name((self) => self.slice(), "copy");
var unionWith = /* @__PURE__ */ dual(3, (self, that, isEquivalent) => {
  const a = fromIterable(self);
  const b = fromIterable(that);
  if (isNonEmptyReadonlyArray(a)) {
    if (isNonEmptyReadonlyArray(b)) {
      const dedupe2 = dedupeWith(isEquivalent);
      return dedupe2(appendAll(a, b));
    }
    return a;
  }
  return b;
});
var union = /* @__PURE__ */ dual(2, (self, that) => unionWith(self, that, _equivalence2));
var empty = /* @__PURE__ */ __name(() => [], "empty");
var of = /* @__PURE__ */ __name((a) => [a], "of");
var map2 = /* @__PURE__ */ dual(2, (self, f) => self.map(f));
var flatMap2 = /* @__PURE__ */ dual(2, (self, f) => {
  if (isEmptyReadonlyArray(self)) {
    return [];
  }
  const out = [];
  for (let i = 0; i < self.length; i++) {
    const inner = f(self[i], i);
    for (let j = 0; j < inner.length; j++) {
      out.push(inner[j]);
    }
  }
  return out;
});
var flatten = /* @__PURE__ */ flatMap2(identity);
var filterMap = /* @__PURE__ */ dual(2, (self, f) => {
  const as7 = fromIterable(self);
  const out = [];
  for (let i = 0; i < as7.length; i++) {
    const o = f(as7[i], i);
    if (isSome2(o)) {
      out.push(o.value);
    }
  }
  return out;
});
var partitionMap = /* @__PURE__ */ dual(2, (self, f) => {
  const left3 = [];
  const right3 = [];
  const as7 = fromIterable(self);
  for (let i = 0; i < as7.length; i++) {
    const e = f(as7[i], i);
    if (isLeft2(e)) {
      left3.push(e.left);
    } else {
      right3.push(e.right);
    }
  }
  return [left3, right3];
});
var getSomes = /* @__PURE__ */ filterMap(identity);
var reduce = /* @__PURE__ */ dual(3, (self, b, f) => fromIterable(self).reduce((b2, a, i) => f(b2, a, i), b));
var reduceRight = /* @__PURE__ */ dual(3, (self, b, f) => fromIterable(self).reduceRight((b2, a, i) => f(b2, a, i), b));
var unfold = /* @__PURE__ */ __name((b, f) => {
  const out = [];
  let next = b;
  let o;
  while (isSome2(o = f(next))) {
    const [a, b2] = o.value;
    out.push(a);
    next = b2;
  }
  return out;
}, "unfold");
var getEquivalence = array;
var dedupeWith = /* @__PURE__ */ dual(2, (self, isEquivalent) => {
  const input = fromIterable(self);
  if (isNonEmptyReadonlyArray(input)) {
    const out = [headNonEmpty(input)];
    const rest = tailNonEmpty(input);
    for (const r of rest) {
      if (out.every((a) => !isEquivalent(r, a))) {
        out.push(r);
      }
    }
    return out;
  }
  return [];
});
var dedupe = /* @__PURE__ */ __name((self) => dedupeWith(self, equivalence()), "dedupe");
var join = /* @__PURE__ */ dual(2, (self, sep) => fromIterable(self).join(sep));

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/Number.js
var Order = number2;

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/RegExp.js
var escape = /* @__PURE__ */ __name((string2) => string2.replace(/[/\\^$*+?.()|[\]{}]/g, "\\$&"), "escape");

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/Boolean.js
var not = /* @__PURE__ */ __name((self) => !self, "not");

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/internal/context.js
var TagTypeId = /* @__PURE__ */ Symbol.for("effect/Context/Tag");
var ReferenceTypeId = /* @__PURE__ */ Symbol.for("effect/Context/Reference");
var STMSymbolKey = "effect/STM";
var STMTypeId = /* @__PURE__ */ Symbol.for(STMSymbolKey);
var TagProto = {
  ...EffectPrototype,
  _op: "Tag",
  [STMTypeId]: effectVariance,
  [TagTypeId]: {
    _Service: /* @__PURE__ */ __name((_) => _, "_Service"),
    _Identifier: /* @__PURE__ */ __name((_) => _, "_Identifier")
  },
  toString() {
    return format(this.toJSON());
  },
  toJSON() {
    return {
      _id: "Tag",
      key: this.key,
      stack: this.stack
    };
  },
  [NodeInspectSymbol]() {
    return this.toJSON();
  },
  of(self) {
    return self;
  },
  context(self) {
    return make4(this, self);
  }
};
var ReferenceProto = {
  ...TagProto,
  [ReferenceTypeId]: ReferenceTypeId
};
var makeGenericTag = /* @__PURE__ */ __name((key) => {
  const limit = Error.stackTraceLimit;
  Error.stackTraceLimit = 2;
  const creationError = new Error();
  Error.stackTraceLimit = limit;
  const tag = Object.create(TagProto);
  Object.defineProperty(tag, "stack", {
    get() {
      return creationError.stack;
    }
  });
  tag.key = key;
  return tag;
}, "makeGenericTag");
var Reference = /* @__PURE__ */ __name(() => (id, options) => {
  const limit = Error.stackTraceLimit;
  Error.stackTraceLimit = 2;
  const creationError = new Error();
  Error.stackTraceLimit = limit;
  function ReferenceClass() {
  }
  __name(ReferenceClass, "ReferenceClass");
  Object.setPrototypeOf(ReferenceClass, ReferenceProto);
  ReferenceClass.key = id;
  ReferenceClass.defaultValue = options.defaultValue;
  Object.defineProperty(ReferenceClass, "stack", {
    get() {
      return creationError.stack;
    }
  });
  return ReferenceClass;
}, "Reference");
var TypeId3 = /* @__PURE__ */ Symbol.for("effect/Context");
var ContextProto = {
  [TypeId3]: {
    _Services: /* @__PURE__ */ __name((_) => _, "_Services")
  },
  [symbol2](that) {
    if (isContext(that)) {
      if (this.unsafeMap.size === that.unsafeMap.size) {
        for (const k of this.unsafeMap.keys()) {
          if (!that.unsafeMap.has(k) || !equals(this.unsafeMap.get(k), that.unsafeMap.get(k))) {
            return false;
          }
        }
        return true;
      }
    }
    return false;
  },
  [symbol]() {
    return cached(this, number(this.unsafeMap.size));
  },
  pipe() {
    return pipeArguments(this, arguments);
  },
  toString() {
    return format(this.toJSON());
  },
  toJSON() {
    return {
      _id: "Context",
      services: Array.from(this.unsafeMap).map(toJSON)
    };
  },
  [NodeInspectSymbol]() {
    return this.toJSON();
  }
};
var makeContext = /* @__PURE__ */ __name((unsafeMap) => {
  const context4 = Object.create(ContextProto);
  context4.unsafeMap = unsafeMap;
  return context4;
}, "makeContext");
var serviceNotFoundError = /* @__PURE__ */ __name((tag) => {
  const error = new Error(`Service not found${tag.key ? `: ${String(tag.key)}` : ""}`);
  if (tag.stack) {
    const lines = tag.stack.split("\n");
    if (lines.length > 2) {
      const afterAt = lines[2].match(/at (.*)/);
      if (afterAt) {
        error.message = error.message + ` (defined at ${afterAt[1]})`;
      }
    }
  }
  if (error.stack) {
    const lines = error.stack.split("\n");
    lines.splice(1, 3);
    error.stack = lines.join("\n");
  }
  return error;
}, "serviceNotFoundError");
var isContext = /* @__PURE__ */ __name((u) => hasProperty(u, TypeId3), "isContext");
var isTag = /* @__PURE__ */ __name((u) => hasProperty(u, TagTypeId), "isTag");
var isReference = /* @__PURE__ */ __name((u) => hasProperty(u, ReferenceTypeId), "isReference");
var _empty = /* @__PURE__ */ makeContext(/* @__PURE__ */ new Map());
var empty2 = /* @__PURE__ */ __name(() => _empty, "empty");
var make4 = /* @__PURE__ */ __name((tag, service) => makeContext(/* @__PURE__ */ new Map([[tag.key, service]])), "make");
var add = /* @__PURE__ */ dual(3, (self, tag, service) => {
  const map14 = new Map(self.unsafeMap);
  map14.set(tag.key, service);
  return makeContext(map14);
});
var defaultValueCache = /* @__PURE__ */ globalValue("effect/Context/defaultValueCache", () => /* @__PURE__ */ new Map());
var getDefaultValue = /* @__PURE__ */ __name((tag) => {
  if (defaultValueCache.has(tag.key)) {
    return defaultValueCache.get(tag.key);
  }
  const value = tag.defaultValue();
  defaultValueCache.set(tag.key, value);
  return value;
}, "getDefaultValue");
var unsafeGetReference = /* @__PURE__ */ __name((self, tag) => {
  return self.unsafeMap.has(tag.key) ? self.unsafeMap.get(tag.key) : getDefaultValue(tag);
}, "unsafeGetReference");
var unsafeGet2 = /* @__PURE__ */ dual(2, (self, tag) => {
  if (!self.unsafeMap.has(tag.key)) {
    if (ReferenceTypeId in tag) return getDefaultValue(tag);
    throw serviceNotFoundError(tag);
  }
  return self.unsafeMap.get(tag.key);
});
var get2 = unsafeGet2;
var getOption = /* @__PURE__ */ dual(2, (self, tag) => {
  if (!self.unsafeMap.has(tag.key)) {
    return isReference(tag) ? some(getDefaultValue(tag)) : none;
  }
  return some(self.unsafeMap.get(tag.key));
});
var merge2 = /* @__PURE__ */ dual(2, (self, that) => {
  const map14 = new Map(self.unsafeMap);
  for (const [tag, s] of that.unsafeMap) {
    map14.set(tag, s);
  }
  return makeContext(map14);
});
var mergeAll = /* @__PURE__ */ __name((...ctxs) => {
  const map14 = /* @__PURE__ */ new Map();
  for (let i = 0; i < ctxs.length; i++) {
    ctxs[i].unsafeMap.forEach((value, key) => {
      map14.set(key, value);
    });
  }
  return makeContext(map14);
}, "mergeAll");

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/Context.js
var GenericTag = makeGenericTag;
var isContext2 = isContext;
var isTag2 = isTag;
var empty3 = empty2;
var make5 = make4;
var add2 = add;
var get3 = get2;
var unsafeGet3 = unsafeGet2;
var getOption2 = getOption;
var merge3 = merge2;
var mergeAll2 = mergeAll;
var Reference2 = Reference;

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/Chunk.js
var TypeId4 = /* @__PURE__ */ Symbol.for("effect/Chunk");
function copy2(src, srcPos, dest, destPos, len) {
  for (let i = srcPos; i < Math.min(src.length, srcPos + len); i++) {
    dest[destPos + i - srcPos] = src[i];
  }
  return dest;
}
__name(copy2, "copy");
var emptyArray = [];
var getEquivalence2 = /* @__PURE__ */ __name((isEquivalent) => make((self, that) => self.length === that.length && toReadonlyArray(self).every((value, i) => isEquivalent(value, unsafeGet4(that, i)))), "getEquivalence");
var _equivalence3 = /* @__PURE__ */ getEquivalence2(equals);
var ChunkProto = {
  [TypeId4]: {
    _A: /* @__PURE__ */ __name((_) => _, "_A")
  },
  toString() {
    return format(this.toJSON());
  },
  toJSON() {
    return {
      _id: "Chunk",
      values: toReadonlyArray(this).map(toJSON)
    };
  },
  [NodeInspectSymbol]() {
    return this.toJSON();
  },
  [symbol2](that) {
    return isChunk(that) && _equivalence3(this, that);
  },
  [symbol]() {
    return cached(this, array2(toReadonlyArray(this)));
  },
  [Symbol.iterator]() {
    switch (this.backing._tag) {
      case "IArray": {
        return this.backing.array[Symbol.iterator]();
      }
      case "IEmpty": {
        return emptyArray[Symbol.iterator]();
      }
      default: {
        return toReadonlyArray(this)[Symbol.iterator]();
      }
    }
  },
  pipe() {
    return pipeArguments(this, arguments);
  }
};
var makeChunk = /* @__PURE__ */ __name((backing) => {
  const chunk2 = Object.create(ChunkProto);
  chunk2.backing = backing;
  switch (backing._tag) {
    case "IEmpty": {
      chunk2.length = 0;
      chunk2.depth = 0;
      chunk2.left = chunk2;
      chunk2.right = chunk2;
      break;
    }
    case "IConcat": {
      chunk2.length = backing.left.length + backing.right.length;
      chunk2.depth = 1 + Math.max(backing.left.depth, backing.right.depth);
      chunk2.left = backing.left;
      chunk2.right = backing.right;
      break;
    }
    case "IArray": {
      chunk2.length = backing.array.length;
      chunk2.depth = 0;
      chunk2.left = _empty2;
      chunk2.right = _empty2;
      break;
    }
    case "ISingleton": {
      chunk2.length = 1;
      chunk2.depth = 0;
      chunk2.left = _empty2;
      chunk2.right = _empty2;
      break;
    }
    case "ISlice": {
      chunk2.length = backing.length;
      chunk2.depth = backing.chunk.depth + 1;
      chunk2.left = _empty2;
      chunk2.right = _empty2;
      break;
    }
  }
  return chunk2;
}, "makeChunk");
var isChunk = /* @__PURE__ */ __name((u) => hasProperty(u, TypeId4), "isChunk");
var _empty2 = /* @__PURE__ */ makeChunk({
  _tag: "IEmpty"
});
var empty4 = /* @__PURE__ */ __name(() => _empty2, "empty");
var make6 = /* @__PURE__ */ __name((...as7) => unsafeFromNonEmptyArray(as7), "make");
var of2 = /* @__PURE__ */ __name((a) => makeChunk({
  _tag: "ISingleton",
  a
}), "of");
var fromIterable2 = /* @__PURE__ */ __name((self) => isChunk(self) ? self : unsafeFromArray(fromIterable(self)), "fromIterable");
var copyToArray = /* @__PURE__ */ __name((self, array3, initial) => {
  switch (self.backing._tag) {
    case "IArray": {
      copy2(self.backing.array, 0, array3, initial, self.length);
      break;
    }
    case "IConcat": {
      copyToArray(self.left, array3, initial);
      copyToArray(self.right, array3, initial + self.left.length);
      break;
    }
    case "ISingleton": {
      array3[initial] = self.backing.a;
      break;
    }
    case "ISlice": {
      let i = 0;
      let j = initial;
      while (i < self.length) {
        array3[j] = unsafeGet4(self, i);
        i += 1;
        j += 1;
      }
      break;
    }
  }
}, "copyToArray");
var toReadonlyArray_ = /* @__PURE__ */ __name((self) => {
  switch (self.backing._tag) {
    case "IEmpty": {
      return emptyArray;
    }
    case "IArray": {
      return self.backing.array;
    }
    default: {
      const arr = new Array(self.length);
      copyToArray(self, arr, 0);
      self.backing = {
        _tag: "IArray",
        array: arr
      };
      self.left = _empty2;
      self.right = _empty2;
      self.depth = 0;
      return arr;
    }
  }
}, "toReadonlyArray_");
var toReadonlyArray = toReadonlyArray_;
var reverseChunk = /* @__PURE__ */ __name((self) => {
  switch (self.backing._tag) {
    case "IEmpty":
    case "ISingleton":
      return self;
    case "IArray": {
      return makeChunk({
        _tag: "IArray",
        array: reverse(self.backing.array)
      });
    }
    case "IConcat": {
      return makeChunk({
        _tag: "IConcat",
        left: reverse2(self.backing.right),
        right: reverse2(self.backing.left)
      });
    }
    case "ISlice":
      return unsafeFromArray(reverse(toReadonlyArray(self)));
  }
}, "reverseChunk");
var reverse2 = reverseChunk;
var get4 = /* @__PURE__ */ dual(2, (self, index) => index < 0 || index >= self.length ? none2() : some2(unsafeGet4(self, index)));
var unsafeFromArray = /* @__PURE__ */ __name((self) => self.length === 0 ? empty4() : self.length === 1 ? of2(self[0]) : makeChunk({
  _tag: "IArray",
  array: self
}), "unsafeFromArray");
var unsafeFromNonEmptyArray = /* @__PURE__ */ __name((self) => unsafeFromArray(self), "unsafeFromNonEmptyArray");
var unsafeGet4 = /* @__PURE__ */ dual(2, (self, index) => {
  switch (self.backing._tag) {
    case "IEmpty": {
      throw new Error(`Index out of bounds`);
    }
    case "ISingleton": {
      if (index !== 0) {
        throw new Error(`Index out of bounds`);
      }
      return self.backing.a;
    }
    case "IArray": {
      if (index >= self.length || index < 0) {
        throw new Error(`Index out of bounds`);
      }
      return self.backing.array[index];
    }
    case "IConcat": {
      return index < self.left.length ? unsafeGet4(self.left, index) : unsafeGet4(self.right, index - self.left.length);
    }
    case "ISlice": {
      return unsafeGet4(self.backing.chunk, index + self.backing.offset);
    }
  }
});
var append2 = /* @__PURE__ */ dual(2, (self, a) => appendAll2(self, of2(a)));
var prepend2 = /* @__PURE__ */ dual(2, (self, elem) => appendAll2(of2(elem), self));
var drop2 = /* @__PURE__ */ dual(2, (self, n) => {
  if (n <= 0) {
    return self;
  } else if (n >= self.length) {
    return _empty2;
  } else {
    switch (self.backing._tag) {
      case "ISlice": {
        return makeChunk({
          _tag: "ISlice",
          chunk: self.backing.chunk,
          offset: self.backing.offset + n,
          length: self.backing.length - n
        });
      }
      case "IConcat": {
        if (n > self.left.length) {
          return drop2(self.right, n - self.left.length);
        }
        return makeChunk({
          _tag: "IConcat",
          left: drop2(self.left, n),
          right: self.right
        });
      }
      default: {
        return makeChunk({
          _tag: "ISlice",
          chunk: self,
          offset: n,
          length: self.length - n
        });
      }
    }
  }
});
var appendAll2 = /* @__PURE__ */ dual(2, (self, that) => {
  if (self.backing._tag === "IEmpty") {
    return that;
  }
  if (that.backing._tag === "IEmpty") {
    return self;
  }
  const diff8 = that.depth - self.depth;
  if (Math.abs(diff8) <= 1) {
    return makeChunk({
      _tag: "IConcat",
      left: self,
      right: that
    });
  } else if (diff8 < -1) {
    if (self.left.depth >= self.right.depth) {
      const nr = appendAll2(self.right, that);
      return makeChunk({
        _tag: "IConcat",
        left: self.left,
        right: nr
      });
    } else {
      const nrr = appendAll2(self.right.right, that);
      if (nrr.depth === self.depth - 3) {
        const nr = makeChunk({
          _tag: "IConcat",
          left: self.right.left,
          right: nrr
        });
        return makeChunk({
          _tag: "IConcat",
          left: self.left,
          right: nr
        });
      } else {
        const nl = makeChunk({
          _tag: "IConcat",
          left: self.left,
          right: self.right.left
        });
        return makeChunk({
          _tag: "IConcat",
          left: nl,
          right: nrr
        });
      }
    }
  } else {
    if (that.right.depth >= that.left.depth) {
      const nl = appendAll2(self, that.left);
      return makeChunk({
        _tag: "IConcat",
        left: nl,
        right: that.right
      });
    } else {
      const nll = appendAll2(self, that.left.left);
      if (nll.depth === that.depth - 3) {
        const nl = makeChunk({
          _tag: "IConcat",
          left: nll,
          right: that.left.right
        });
        return makeChunk({
          _tag: "IConcat",
          left: nl,
          right: that.right
        });
      } else {
        const nr = makeChunk({
          _tag: "IConcat",
          left: that.left.right,
          right: that.right
        });
        return makeChunk({
          _tag: "IConcat",
          left: nll,
          right: nr
        });
      }
    }
  }
});
var isEmpty = /* @__PURE__ */ __name((self) => self.length === 0, "isEmpty");
var isNonEmpty = /* @__PURE__ */ __name((self) => self.length > 0, "isNonEmpty");
var head2 = /* @__PURE__ */ get4(0);
var unsafeHead = /* @__PURE__ */ __name((self) => unsafeGet4(self, 0), "unsafeHead");
var headNonEmpty2 = unsafeHead;
var tailNonEmpty2 = /* @__PURE__ */ __name((self) => drop2(self, 1), "tailNonEmpty");

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/Duration.js
var Duration_exports = {};
__export(Duration_exports, {
  Equivalence: () => Equivalence,
  Order: () => Order2,
  between: () => between2,
  clamp: () => clamp3,
  days: () => days,
  decode: () => decode,
  decodeUnknown: () => decodeUnknown,
  divide: () => divide,
  equals: () => equals2,
  format: () => format2,
  formatIso: () => formatIso,
  fromIso: () => fromIso,
  greaterThan: () => greaterThan2,
  greaterThanOrEqualTo: () => greaterThanOrEqualTo2,
  hours: () => hours,
  infinity: () => infinity,
  isDuration: () => isDuration,
  isFinite: () => isFinite,
  isZero: () => isZero,
  lessThan: () => lessThan2,
  lessThanOrEqualTo: () => lessThanOrEqualTo2,
  match: () => match3,
  matchWith: () => matchWith,
  max: () => max2,
  micros: () => micros,
  millis: () => millis,
  min: () => min2,
  minutes: () => minutes,
  nanos: () => nanos,
  parts: () => parts,
  seconds: () => seconds,
  subtract: () => subtract,
  sum: () => sum,
  times: () => times,
  toDays: () => toDays,
  toHours: () => toHours,
  toHrTime: () => toHrTime,
  toMillis: () => toMillis,
  toMinutes: () => toMinutes,
  toNanos: () => toNanos,
  toSeconds: () => toSeconds,
  toWeeks: () => toWeeks,
  unsafeDivide: () => unsafeDivide,
  unsafeFormatIso: () => unsafeFormatIso,
  unsafeToNanos: () => unsafeToNanos,
  weeks: () => weeks,
  zero: () => zero
});
var TypeId5 = /* @__PURE__ */ Symbol.for("effect/Duration");
var bigint0 = /* @__PURE__ */ BigInt(0);
var bigint24 = /* @__PURE__ */ BigInt(24);
var bigint60 = /* @__PURE__ */ BigInt(60);
var bigint1e3 = /* @__PURE__ */ BigInt(1e3);
var bigint1e6 = /* @__PURE__ */ BigInt(1e6);
var bigint1e9 = /* @__PURE__ */ BigInt(1e9);
var DURATION_REGEX = /^(-?\d+(?:\.\d+)?)\s+(nanos?|micros?|millis?|seconds?|minutes?|hours?|days?|weeks?)$/;
var decode = /* @__PURE__ */ __name((input) => {
  if (isDuration(input)) {
    return input;
  } else if (isNumber(input)) {
    return millis(input);
  } else if (isBigInt(input)) {
    return nanos(input);
  } else if (Array.isArray(input) && input.length === 2 && input.every(isNumber)) {
    if (input[0] === -Infinity || input[1] === -Infinity || Number.isNaN(input[0]) || Number.isNaN(input[1])) {
      return zero;
    }
    if (input[0] === Infinity || input[1] === Infinity) {
      return infinity;
    }
    return nanos(BigInt(Math.round(input[0] * 1e9)) + BigInt(Math.round(input[1])));
  } else if (isString(input)) {
    const match12 = DURATION_REGEX.exec(input);
    if (match12) {
      const [_, valueStr, unit] = match12;
      const value = Number(valueStr);
      switch (unit) {
        case "nano":
        case "nanos":
          return nanos(BigInt(valueStr));
        case "micro":
        case "micros":
          return micros(BigInt(valueStr));
        case "milli":
        case "millis":
          return millis(value);
        case "second":
        case "seconds":
          return seconds(value);
        case "minute":
        case "minutes":
          return minutes(value);
        case "hour":
        case "hours":
          return hours(value);
        case "day":
        case "days":
          return days(value);
        case "week":
        case "weeks":
          return weeks(value);
      }
    }
  }
  throw new Error("Invalid DurationInput");
}, "decode");
var decodeUnknown = /* @__PURE__ */ liftThrowable(decode);
var zeroValue = {
  _tag: "Millis",
  millis: 0
};
var infinityValue = {
  _tag: "Infinity"
};
var DurationProto = {
  [TypeId5]: TypeId5,
  [symbol]() {
    return cached(this, structure(this.value));
  },
  [symbol2](that) {
    return isDuration(that) && equals2(this, that);
  },
  toString() {
    return `Duration(${format2(this)})`;
  },
  toJSON() {
    switch (this.value._tag) {
      case "Millis":
        return {
          _id: "Duration",
          _tag: "Millis",
          millis: this.value.millis
        };
      case "Nanos":
        return {
          _id: "Duration",
          _tag: "Nanos",
          hrtime: toHrTime(this)
        };
      case "Infinity":
        return {
          _id: "Duration",
          _tag: "Infinity"
        };
    }
  },
  [NodeInspectSymbol]() {
    return this.toJSON();
  },
  pipe() {
    return pipeArguments(this, arguments);
  }
};
var make7 = /* @__PURE__ */ __name((input) => {
  const duration = Object.create(DurationProto);
  if (isNumber(input)) {
    if (isNaN(input) || input <= 0) {
      duration.value = zeroValue;
    } else if (!Number.isFinite(input)) {
      duration.value = infinityValue;
    } else if (!Number.isInteger(input)) {
      duration.value = {
        _tag: "Nanos",
        nanos: BigInt(Math.round(input * 1e6))
      };
    } else {
      duration.value = {
        _tag: "Millis",
        millis: input
      };
    }
  } else if (input <= bigint0) {
    duration.value = zeroValue;
  } else {
    duration.value = {
      _tag: "Nanos",
      nanos: input
    };
  }
  return duration;
}, "make");
var isDuration = /* @__PURE__ */ __name((u) => hasProperty(u, TypeId5), "isDuration");
var isFinite = /* @__PURE__ */ __name((self) => self.value._tag !== "Infinity", "isFinite");
var isZero = /* @__PURE__ */ __name((self) => {
  switch (self.value._tag) {
    case "Millis": {
      return self.value.millis === 0;
    }
    case "Nanos": {
      return self.value.nanos === bigint0;
    }
    case "Infinity": {
      return false;
    }
  }
}, "isZero");
var zero = /* @__PURE__ */ make7(0);
var infinity = /* @__PURE__ */ make7(Infinity);
var nanos = /* @__PURE__ */ __name((nanos2) => make7(nanos2), "nanos");
var micros = /* @__PURE__ */ __name((micros2) => make7(micros2 * bigint1e3), "micros");
var millis = /* @__PURE__ */ __name((millis2) => make7(millis2), "millis");
var seconds = /* @__PURE__ */ __name((seconds2) => make7(seconds2 * 1e3), "seconds");
var minutes = /* @__PURE__ */ __name((minutes2) => make7(minutes2 * 6e4), "minutes");
var hours = /* @__PURE__ */ __name((hours2) => make7(hours2 * 36e5), "hours");
var days = /* @__PURE__ */ __name((days2) => make7(days2 * 864e5), "days");
var weeks = /* @__PURE__ */ __name((weeks2) => make7(weeks2 * 6048e5), "weeks");
var toMillis = /* @__PURE__ */ __name((self) => match3(self, {
  onMillis: /* @__PURE__ */ __name((millis2) => millis2, "onMillis"),
  onNanos: /* @__PURE__ */ __name((nanos2) => Number(nanos2) / 1e6, "onNanos")
}), "toMillis");
var toSeconds = /* @__PURE__ */ __name((self) => match3(self, {
  onMillis: /* @__PURE__ */ __name((millis2) => millis2 / 1e3, "onMillis"),
  onNanos: /* @__PURE__ */ __name((nanos2) => Number(nanos2) / 1e9, "onNanos")
}), "toSeconds");
var toMinutes = /* @__PURE__ */ __name((self) => match3(self, {
  onMillis: /* @__PURE__ */ __name((millis2) => millis2 / 6e4, "onMillis"),
  onNanos: /* @__PURE__ */ __name((nanos2) => Number(nanos2) / 6e10, "onNanos")
}), "toMinutes");
var toHours = /* @__PURE__ */ __name((self) => match3(self, {
  onMillis: /* @__PURE__ */ __name((millis2) => millis2 / 36e5, "onMillis"),
  onNanos: /* @__PURE__ */ __name((nanos2) => Number(nanos2) / 36e11, "onNanos")
}), "toHours");
var toDays = /* @__PURE__ */ __name((self) => match3(self, {
  onMillis: /* @__PURE__ */ __name((millis2) => millis2 / 864e5, "onMillis"),
  onNanos: /* @__PURE__ */ __name((nanos2) => Number(nanos2) / 864e11, "onNanos")
}), "toDays");
var toWeeks = /* @__PURE__ */ __name((self) => match3(self, {
  onMillis: /* @__PURE__ */ __name((millis2) => millis2 / 6048e5, "onMillis"),
  onNanos: /* @__PURE__ */ __name((nanos2) => Number(nanos2) / 6048e11, "onNanos")
}), "toWeeks");
var toNanos = /* @__PURE__ */ __name((self) => {
  const _self = decode(self);
  switch (_self.value._tag) {
    case "Infinity":
      return none2();
    case "Nanos":
      return some2(_self.value.nanos);
    case "Millis":
      return some2(BigInt(Math.round(_self.value.millis * 1e6)));
  }
}, "toNanos");
var unsafeToNanos = /* @__PURE__ */ __name((self) => {
  const _self = decode(self);
  switch (_self.value._tag) {
    case "Infinity":
      throw new Error("Cannot convert infinite duration to nanos");
    case "Nanos":
      return _self.value.nanos;
    case "Millis":
      return BigInt(Math.round(_self.value.millis * 1e6));
  }
}, "unsafeToNanos");
var toHrTime = /* @__PURE__ */ __name((self) => {
  const _self = decode(self);
  switch (_self.value._tag) {
    case "Infinity":
      return [Infinity, 0];
    case "Nanos":
      return [Number(_self.value.nanos / bigint1e9), Number(_self.value.nanos % bigint1e9)];
    case "Millis":
      return [Math.floor(_self.value.millis / 1e3), Math.round(_self.value.millis % 1e3 * 1e6)];
  }
}, "toHrTime");
var match3 = /* @__PURE__ */ dual(2, (self, options) => {
  const _self = decode(self);
  switch (_self.value._tag) {
    case "Nanos":
      return options.onNanos(_self.value.nanos);
    case "Infinity":
      return options.onMillis(Infinity);
    case "Millis":
      return options.onMillis(_self.value.millis);
  }
});
var matchWith = /* @__PURE__ */ dual(3, (self, that, options) => {
  const _self = decode(self);
  const _that = decode(that);
  if (_self.value._tag === "Infinity" || _that.value._tag === "Infinity") {
    return options.onMillis(toMillis(_self), toMillis(_that));
  } else if (_self.value._tag === "Nanos" || _that.value._tag === "Nanos") {
    const selfNanos = _self.value._tag === "Nanos" ? _self.value.nanos : BigInt(Math.round(_self.value.millis * 1e6));
    const thatNanos = _that.value._tag === "Nanos" ? _that.value.nanos : BigInt(Math.round(_that.value.millis * 1e6));
    return options.onNanos(selfNanos, thatNanos);
  }
  return options.onMillis(_self.value.millis, _that.value.millis);
});
var Order2 = /* @__PURE__ */ make2((self, that) => matchWith(self, that, {
  onMillis: /* @__PURE__ */ __name((self2, that2) => self2 < that2 ? -1 : self2 > that2 ? 1 : 0, "onMillis"),
  onNanos: /* @__PURE__ */ __name((self2, that2) => self2 < that2 ? -1 : self2 > that2 ? 1 : 0, "onNanos")
}));
var between2 = /* @__PURE__ */ between(/* @__PURE__ */ mapInput2(Order2, decode));
var Equivalence = /* @__PURE__ */ __name((self, that) => matchWith(self, that, {
  onMillis: /* @__PURE__ */ __name((self2, that2) => self2 === that2, "onMillis"),
  onNanos: /* @__PURE__ */ __name((self2, that2) => self2 === that2, "onNanos")
}), "Equivalence");
var _min = /* @__PURE__ */ min(Order2);
var min2 = /* @__PURE__ */ dual(2, (self, that) => _min(decode(self), decode(that)));
var _max = /* @__PURE__ */ max(Order2);
var max2 = /* @__PURE__ */ dual(2, (self, that) => _max(decode(self), decode(that)));
var _clamp = /* @__PURE__ */ clamp(Order2);
var clamp3 = /* @__PURE__ */ dual(2, (self, options) => _clamp(decode(self), {
  minimum: decode(options.minimum),
  maximum: decode(options.maximum)
}));
var divide = /* @__PURE__ */ dual(2, (self, by) => match3(self, {
  onMillis: /* @__PURE__ */ __name((millis2) => {
    if (by === 0 || isNaN(by) || !Number.isFinite(by)) {
      return none2();
    }
    return some2(make7(millis2 / by));
  }, "onMillis"),
  onNanos: /* @__PURE__ */ __name((nanos2) => {
    if (isNaN(by) || by <= 0 || !Number.isFinite(by)) {
      return none2();
    }
    try {
      return some2(make7(nanos2 / BigInt(by)));
    } catch {
      return none2();
    }
  }, "onNanos")
}));
var unsafeDivide = /* @__PURE__ */ dual(2, (self, by) => match3(self, {
  onMillis: /* @__PURE__ */ __name((millis2) => make7(millis2 / by), "onMillis"),
  onNanos: /* @__PURE__ */ __name((nanos2) => {
    if (isNaN(by) || by < 0 || Object.is(by, -0)) {
      return zero;
    } else if (Object.is(by, 0) || !Number.isFinite(by)) {
      return infinity;
    }
    return make7(nanos2 / BigInt(by));
  }, "onNanos")
}));
var times = /* @__PURE__ */ dual(2, (self, times2) => match3(self, {
  onMillis: /* @__PURE__ */ __name((millis2) => make7(millis2 * times2), "onMillis"),
  onNanos: /* @__PURE__ */ __name((nanos2) => make7(nanos2 * BigInt(times2)), "onNanos")
}));
var subtract = /* @__PURE__ */ dual(2, (self, that) => matchWith(self, that, {
  onMillis: /* @__PURE__ */ __name((self2, that2) => make7(self2 - that2), "onMillis"),
  onNanos: /* @__PURE__ */ __name((self2, that2) => make7(self2 - that2), "onNanos")
}));
var sum = /* @__PURE__ */ dual(2, (self, that) => matchWith(self, that, {
  onMillis: /* @__PURE__ */ __name((self2, that2) => make7(self2 + that2), "onMillis"),
  onNanos: /* @__PURE__ */ __name((self2, that2) => make7(self2 + that2), "onNanos")
}));
var lessThan2 = /* @__PURE__ */ dual(2, (self, that) => matchWith(self, that, {
  onMillis: /* @__PURE__ */ __name((self2, that2) => self2 < that2, "onMillis"),
  onNanos: /* @__PURE__ */ __name((self2, that2) => self2 < that2, "onNanos")
}));
var lessThanOrEqualTo2 = /* @__PURE__ */ dual(2, (self, that) => matchWith(self, that, {
  onMillis: /* @__PURE__ */ __name((self2, that2) => self2 <= that2, "onMillis"),
  onNanos: /* @__PURE__ */ __name((self2, that2) => self2 <= that2, "onNanos")
}));
var greaterThan2 = /* @__PURE__ */ dual(2, (self, that) => matchWith(self, that, {
  onMillis: /* @__PURE__ */ __name((self2, that2) => self2 > that2, "onMillis"),
  onNanos: /* @__PURE__ */ __name((self2, that2) => self2 > that2, "onNanos")
}));
var greaterThanOrEqualTo2 = /* @__PURE__ */ dual(2, (self, that) => matchWith(self, that, {
  onMillis: /* @__PURE__ */ __name((self2, that2) => self2 >= that2, "onMillis"),
  onNanos: /* @__PURE__ */ __name((self2, that2) => self2 >= that2, "onNanos")
}));
var equals2 = /* @__PURE__ */ dual(2, (self, that) => Equivalence(decode(self), decode(that)));
var parts = /* @__PURE__ */ __name((self) => {
  const duration = decode(self);
  if (duration.value._tag === "Infinity") {
    return {
      days: Infinity,
      hours: Infinity,
      minutes: Infinity,
      seconds: Infinity,
      millis: Infinity,
      nanos: Infinity
    };
  }
  const nanos2 = unsafeToNanos(duration);
  const ms = nanos2 / bigint1e6;
  const sec = ms / bigint1e3;
  const min4 = sec / bigint60;
  const hr = min4 / bigint60;
  const days2 = hr / bigint24;
  return {
    days: Number(days2),
    hours: Number(hr % bigint24),
    minutes: Number(min4 % bigint60),
    seconds: Number(sec % bigint60),
    millis: Number(ms % bigint1e3),
    nanos: Number(nanos2 % bigint1e6)
  };
}, "parts");
var format2 = /* @__PURE__ */ __name((self) => {
  const duration = decode(self);
  if (duration.value._tag === "Infinity") {
    return "Infinity";
  }
  if (isZero(duration)) {
    return "0";
  }
  const fragments = parts(duration);
  const pieces = [];
  if (fragments.days !== 0) {
    pieces.push(`${fragments.days}d`);
  }
  if (fragments.hours !== 0) {
    pieces.push(`${fragments.hours}h`);
  }
  if (fragments.minutes !== 0) {
    pieces.push(`${fragments.minutes}m`);
  }
  if (fragments.seconds !== 0) {
    pieces.push(`${fragments.seconds}s`);
  }
  if (fragments.millis !== 0) {
    pieces.push(`${fragments.millis}ms`);
  }
  if (fragments.nanos !== 0) {
    pieces.push(`${fragments.nanos}ns`);
  }
  return pieces.join(" ");
}, "format");
var unsafeFormatIso = /* @__PURE__ */ __name((self) => {
  const duration = decode(self);
  if (!isFinite(duration)) {
    throw new RangeError("Cannot format infinite duration");
  }
  const fragments = [];
  const {
    days: days2,
    hours: hours2,
    millis: millis2,
    minutes: minutes2,
    nanos: nanos2,
    seconds: seconds2
  } = parts(duration);
  let rest = days2;
  if (rest >= 365) {
    const years = Math.floor(rest / 365);
    rest %= 365;
    fragments.push(`${years}Y`);
  }
  if (rest >= 30) {
    const months = Math.floor(rest / 30);
    rest %= 30;
    fragments.push(`${months}M`);
  }
  if (rest >= 7) {
    const weeks2 = Math.floor(rest / 7);
    rest %= 7;
    fragments.push(`${weeks2}W`);
  }
  if (rest > 0) {
    fragments.push(`${rest}D`);
  }
  if (hours2 !== 0 || minutes2 !== 0 || seconds2 !== 0 || millis2 !== 0 || nanos2 !== 0) {
    fragments.push("T");
    if (hours2 !== 0) {
      fragments.push(`${hours2}H`);
    }
    if (minutes2 !== 0) {
      fragments.push(`${minutes2}M`);
    }
    if (seconds2 !== 0 || millis2 !== 0 || nanos2 !== 0) {
      const total = BigInt(seconds2) * bigint1e9 + BigInt(millis2) * bigint1e6 + BigInt(nanos2);
      const str = (Number(total) / 1e9).toFixed(9).replace(/\.?0+$/, "");
      fragments.push(`${str}S`);
    }
  }
  return `P${fragments.join("") || "T0S"}`;
}, "unsafeFormatIso");
var formatIso = /* @__PURE__ */ __name((self) => {
  const duration = decode(self);
  return isFinite(duration) ? some2(unsafeFormatIso(duration)) : none2();
}, "formatIso");
var fromIso = /* @__PURE__ */ __name((iso) => {
  const result = DURATION_ISO_REGEX.exec(iso);
  if (result == null) {
    return none2();
  }
  const [years, months, weeks2, days2, hours2, mins, secs] = result.slice(1, 8).map((_) => _ ? Number(_) : 0);
  const value = years * 365 * 24 * 60 * 60 + months * 30 * 24 * 60 * 60 + weeks2 * 7 * 24 * 60 * 60 + days2 * 24 * 60 * 60 + hours2 * 60 * 60 + mins * 60 + secs;
  return some2(seconds(value));
}, "fromIso");
var DURATION_ISO_REGEX = /^P(?!$)(?:(\d+)Y)?(?:(\d+)M)?(?:(\d+)W)?(?:(\d+)D)?(?:T(?!$)(?:(\d+)H)?(?:(\d+)M)?(?:(\d+(?:\.\d+)?)S)?)?$/;

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/internal/hashMap/config.js
var SIZE = 5;
var BUCKET_SIZE = /* @__PURE__ */ Math.pow(2, SIZE);
var MASK = BUCKET_SIZE - 1;
var MAX_INDEX_NODE = BUCKET_SIZE / 2;
var MIN_ARRAY_NODE = BUCKET_SIZE / 4;

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/internal/hashMap/bitwise.js
function popcount(x) {
  x -= x >> 1 & 1431655765;
  x = (x & 858993459) + (x >> 2 & 858993459);
  x = x + (x >> 4) & 252645135;
  x += x >> 8;
  x += x >> 16;
  return x & 127;
}
__name(popcount, "popcount");
function hashFragment(shift2, h) {
  return h >>> shift2 & MASK;
}
__name(hashFragment, "hashFragment");
function toBitmap(x) {
  return 1 << x;
}
__name(toBitmap, "toBitmap");
function fromBitmap(bitmap, bit) {
  return popcount(bitmap & bit - 1);
}
__name(fromBitmap, "fromBitmap");

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/internal/stack.js
var make8 = /* @__PURE__ */ __name((value, previous) => ({
  value,
  previous
}), "make");

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/internal/hashMap/array.js
function arrayUpdate(mutate4, at, v, arr) {
  let out = arr;
  if (!mutate4) {
    const len = arr.length;
    out = new Array(len);
    for (let i = 0; i < len; ++i) out[i] = arr[i];
  }
  out[at] = v;
  return out;
}
__name(arrayUpdate, "arrayUpdate");
function arraySpliceOut(mutate4, at, arr) {
  const newLen = arr.length - 1;
  let i = 0;
  let g = 0;
  let out = arr;
  if (mutate4) {
    i = g = at;
  } else {
    out = new Array(newLen);
    while (i < at) out[g++] = arr[i++];
  }
  ++i;
  while (i <= newLen) out[g++] = arr[i++];
  if (mutate4) {
    out.length = newLen;
  }
  return out;
}
__name(arraySpliceOut, "arraySpliceOut");
function arraySpliceIn(mutate4, at, v, arr) {
  const len = arr.length;
  if (mutate4) {
    let i2 = len;
    while (i2 >= at) arr[i2--] = arr[i2];
    arr[at] = v;
    return arr;
  }
  let i = 0, g = 0;
  const out = new Array(len + 1);
  while (i < at) out[g++] = arr[i++];
  out[at] = v;
  while (i < len) out[++g] = arr[i++];
  return out;
}
__name(arraySpliceIn, "arraySpliceIn");

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/internal/hashMap/node.js
var EmptyNode = class _EmptyNode {
  static {
    __name(this, "EmptyNode");
  }
  _tag = "EmptyNode";
  modify(edit, _shift, f, hash2, key, size11) {
    const v = f(none2());
    if (isNone2(v)) return new _EmptyNode();
    ++size11.value;
    return new LeafNode(edit, hash2, key, v);
  }
};
function isEmptyNode(a) {
  return isTagged(a, "EmptyNode");
}
__name(isEmptyNode, "isEmptyNode");
function isLeafNode(node) {
  return isEmptyNode(node) || node._tag === "LeafNode" || node._tag === "CollisionNode";
}
__name(isLeafNode, "isLeafNode");
function canEditNode(node, edit) {
  return isEmptyNode(node) ? false : edit === node.edit;
}
__name(canEditNode, "canEditNode");
var LeafNode = class _LeafNode {
  static {
    __name(this, "LeafNode");
  }
  edit;
  hash;
  key;
  value;
  _tag = "LeafNode";
  constructor(edit, hash2, key, value) {
    this.edit = edit;
    this.hash = hash2;
    this.key = key;
    this.value = value;
  }
  modify(edit, shift2, f, hash2, key, size11) {
    if (equals(key, this.key)) {
      const v2 = f(this.value);
      if (v2 === this.value) return this;
      else if (isNone2(v2)) {
        --size11.value;
        return new EmptyNode();
      }
      if (canEditNode(this, edit)) {
        this.value = v2;
        return this;
      }
      return new _LeafNode(edit, hash2, key, v2);
    }
    const v = f(none2());
    if (isNone2(v)) return this;
    ++size11.value;
    return mergeLeaves(edit, shift2, this.hash, this, hash2, new _LeafNode(edit, hash2, key, v));
  }
};
var CollisionNode = class _CollisionNode {
  static {
    __name(this, "CollisionNode");
  }
  edit;
  hash;
  children;
  _tag = "CollisionNode";
  constructor(edit, hash2, children) {
    this.edit = edit;
    this.hash = hash2;
    this.children = children;
  }
  modify(edit, shift2, f, hash2, key, size11) {
    if (hash2 === this.hash) {
      const canEdit = canEditNode(this, edit);
      const list = this.updateCollisionList(canEdit, edit, this.hash, this.children, f, key, size11);
      if (list === this.children) return this;
      return list.length > 1 ? new _CollisionNode(edit, this.hash, list) : list[0];
    }
    const v = f(none2());
    if (isNone2(v)) return this;
    ++size11.value;
    return mergeLeaves(edit, shift2, this.hash, this, hash2, new LeafNode(edit, hash2, key, v));
  }
  updateCollisionList(mutate4, edit, hash2, list, f, key, size11) {
    const len = list.length;
    for (let i = 0; i < len; ++i) {
      const child = list[i];
      if ("key" in child && equals(key, child.key)) {
        const value = child.value;
        const newValue2 = f(value);
        if (newValue2 === value) return list;
        if (isNone2(newValue2)) {
          --size11.value;
          return arraySpliceOut(mutate4, i, list);
        }
        return arrayUpdate(mutate4, i, new LeafNode(edit, hash2, key, newValue2), list);
      }
    }
    const newValue = f(none2());
    if (isNone2(newValue)) return list;
    ++size11.value;
    return arrayUpdate(mutate4, len, new LeafNode(edit, hash2, key, newValue), list);
  }
};
var IndexedNode = class _IndexedNode {
  static {
    __name(this, "IndexedNode");
  }
  edit;
  mask;
  children;
  _tag = "IndexedNode";
  constructor(edit, mask, children) {
    this.edit = edit;
    this.mask = mask;
    this.children = children;
  }
  modify(edit, shift2, f, hash2, key, size11) {
    const mask = this.mask;
    const children = this.children;
    const frag = hashFragment(shift2, hash2);
    const bit = toBitmap(frag);
    const indx = fromBitmap(mask, bit);
    const exists4 = mask & bit;
    const canEdit = canEditNode(this, edit);
    if (!exists4) {
      const _newChild = new EmptyNode().modify(edit, shift2 + SIZE, f, hash2, key, size11);
      if (!_newChild) return this;
      return children.length >= MAX_INDEX_NODE ? expand(edit, frag, _newChild, mask, children) : new _IndexedNode(edit, mask | bit, arraySpliceIn(canEdit, indx, _newChild, children));
    }
    const current = children[indx];
    const child = current.modify(edit, shift2 + SIZE, f, hash2, key, size11);
    if (current === child) return this;
    let bitmap = mask;
    let newChildren;
    if (isEmptyNode(child)) {
      bitmap &= ~bit;
      if (!bitmap) return new EmptyNode();
      if (children.length <= 2 && isLeafNode(children[indx ^ 1])) {
        return children[indx ^ 1];
      }
      newChildren = arraySpliceOut(canEdit, indx, children);
    } else {
      newChildren = arrayUpdate(canEdit, indx, child, children);
    }
    if (canEdit) {
      this.mask = bitmap;
      this.children = newChildren;
      return this;
    }
    return new _IndexedNode(edit, bitmap, newChildren);
  }
};
var ArrayNode = class _ArrayNode {
  static {
    __name(this, "ArrayNode");
  }
  edit;
  size;
  children;
  _tag = "ArrayNode";
  constructor(edit, size11, children) {
    this.edit = edit;
    this.size = size11;
    this.children = children;
  }
  modify(edit, shift2, f, hash2, key, size11) {
    let count = this.size;
    const children = this.children;
    const frag = hashFragment(shift2, hash2);
    const child = children[frag];
    const newChild = (child || new EmptyNode()).modify(edit, shift2 + SIZE, f, hash2, key, size11);
    if (child === newChild) return this;
    const canEdit = canEditNode(this, edit);
    let newChildren;
    if (isEmptyNode(child) && !isEmptyNode(newChild)) {
      ++count;
      newChildren = arrayUpdate(canEdit, frag, newChild, children);
    } else if (!isEmptyNode(child) && isEmptyNode(newChild)) {
      --count;
      if (count <= MIN_ARRAY_NODE) {
        return pack(edit, count, frag, children);
      }
      newChildren = arrayUpdate(canEdit, frag, new EmptyNode(), children);
    } else {
      newChildren = arrayUpdate(canEdit, frag, newChild, children);
    }
    if (canEdit) {
      this.size = count;
      this.children = newChildren;
      return this;
    }
    return new _ArrayNode(edit, count, newChildren);
  }
};
function pack(edit, count, removed, elements) {
  const children = new Array(count - 1);
  let g = 0;
  let bitmap = 0;
  for (let i = 0, len = elements.length; i < len; ++i) {
    if (i !== removed) {
      const elem = elements[i];
      if (elem && !isEmptyNode(elem)) {
        children[g++] = elem;
        bitmap |= 1 << i;
      }
    }
  }
  return new IndexedNode(edit, bitmap, children);
}
__name(pack, "pack");
function expand(edit, frag, child, bitmap, subNodes) {
  const arr = [];
  let bit = bitmap;
  let count = 0;
  for (let i = 0; bit; ++i) {
    if (bit & 1) arr[i] = subNodes[count++];
    bit >>>= 1;
  }
  arr[frag] = child;
  return new ArrayNode(edit, count + 1, arr);
}
__name(expand, "expand");
function mergeLeavesInner(edit, shift2, h1, n1, h2, n2) {
  if (h1 === h2) return new CollisionNode(edit, h1, [n2, n1]);
  const subH1 = hashFragment(shift2, h1);
  const subH2 = hashFragment(shift2, h2);
  if (subH1 === subH2) {
    return (child) => new IndexedNode(edit, toBitmap(subH1) | toBitmap(subH2), [child]);
  } else {
    const children = subH1 < subH2 ? [n1, n2] : [n2, n1];
    return new IndexedNode(edit, toBitmap(subH1) | toBitmap(subH2), children);
  }
}
__name(mergeLeavesInner, "mergeLeavesInner");
function mergeLeaves(edit, shift2, h1, n1, h2, n2) {
  let stack = void 0;
  let currentShift = shift2;
  while (true) {
    const res = mergeLeavesInner(edit, currentShift, h1, n1, h2, n2);
    if (typeof res === "function") {
      stack = make8(res, stack);
      currentShift = currentShift + SIZE;
    } else {
      let final = res;
      while (stack != null) {
        final = stack.value(final);
        stack = stack.previous;
      }
      return final;
    }
  }
}
__name(mergeLeaves, "mergeLeaves");

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/internal/hashMap.js
var HashMapSymbolKey = "effect/HashMap";
var HashMapTypeId = /* @__PURE__ */ Symbol.for(HashMapSymbolKey);
var HashMapProto = {
  [HashMapTypeId]: HashMapTypeId,
  [Symbol.iterator]() {
    return new HashMapIterator(this, (k, v) => [k, v]);
  },
  [symbol]() {
    let hash2 = hash(HashMapSymbolKey);
    for (const item of this) {
      hash2 ^= pipe(hash(item[0]), combine(hash(item[1])));
    }
    return cached(this, hash2);
  },
  [symbol2](that) {
    if (isHashMap(that)) {
      if (that._size !== this._size) {
        return false;
      }
      for (const item of this) {
        const elem = pipe(that, getHash(item[0], hash(item[0])));
        if (isNone2(elem)) {
          return false;
        } else {
          if (!equals(item[1], elem.value)) {
            return false;
          }
        }
      }
      return true;
    }
    return false;
  },
  toString() {
    return format(this.toJSON());
  },
  toJSON() {
    return {
      _id: "HashMap",
      values: Array.from(this).map(toJSON)
    };
  },
  [NodeInspectSymbol]() {
    return this.toJSON();
  },
  pipe() {
    return pipeArguments(this, arguments);
  }
};
var makeImpl = /* @__PURE__ */ __name((editable, edit, root, size11) => {
  const map14 = Object.create(HashMapProto);
  map14._editable = editable;
  map14._edit = edit;
  map14._root = root;
  map14._size = size11;
  return map14;
}, "makeImpl");
var HashMapIterator = class _HashMapIterator {
  static {
    __name(this, "HashMapIterator");
  }
  map;
  f;
  v;
  constructor(map14, f) {
    this.map = map14;
    this.f = f;
    this.v = visitLazy(this.map._root, this.f, void 0);
  }
  next() {
    if (isNone2(this.v)) {
      return {
        done: true,
        value: void 0
      };
    }
    const v0 = this.v.value;
    this.v = applyCont(v0.cont);
    return {
      done: false,
      value: v0.value
    };
  }
  [Symbol.iterator]() {
    return new _HashMapIterator(this.map, this.f);
  }
};
var applyCont = /* @__PURE__ */ __name((cont) => cont ? visitLazyChildren(cont[0], cont[1], cont[2], cont[3], cont[4]) : none2(), "applyCont");
var visitLazy = /* @__PURE__ */ __name((node, f, cont = void 0) => {
  switch (node._tag) {
    case "LeafNode": {
      if (isSome2(node.value)) {
        return some2({
          value: f(node.key, node.value.value),
          cont
        });
      }
      return applyCont(cont);
    }
    case "CollisionNode":
    case "ArrayNode":
    case "IndexedNode": {
      const children = node.children;
      return visitLazyChildren(children.length, children, 0, f, cont);
    }
    default: {
      return applyCont(cont);
    }
  }
}, "visitLazy");
var visitLazyChildren = /* @__PURE__ */ __name((len, children, i, f, cont) => {
  while (i < len) {
    const child = children[i++];
    if (child && !isEmptyNode(child)) {
      return visitLazy(child, f, [len, children, i, f, cont]);
    }
  }
  return applyCont(cont);
}, "visitLazyChildren");
var _empty3 = /* @__PURE__ */ makeImpl(false, 0, /* @__PURE__ */ new EmptyNode(), 0);
var empty5 = /* @__PURE__ */ __name(() => _empty3, "empty");
var fromIterable3 = /* @__PURE__ */ __name((entries2) => {
  const map14 = beginMutation(empty5());
  for (const entry of entries2) {
    set(map14, entry[0], entry[1]);
  }
  return endMutation(map14);
}, "fromIterable");
var isHashMap = /* @__PURE__ */ __name((u) => hasProperty(u, HashMapTypeId), "isHashMap");
var isEmpty2 = /* @__PURE__ */ __name((self) => self && isEmptyNode(self._root), "isEmpty");
var get5 = /* @__PURE__ */ dual(2, (self, key) => getHash(self, key, hash(key)));
var getHash = /* @__PURE__ */ dual(3, (self, key, hash2) => {
  let node = self._root;
  let shift2 = 0;
  while (true) {
    switch (node._tag) {
      case "LeafNode": {
        return equals(key, node.key) ? node.value : none2();
      }
      case "CollisionNode": {
        if (hash2 === node.hash) {
          const children = node.children;
          for (let i = 0, len = children.length; i < len; ++i) {
            const child = children[i];
            if ("key" in child && equals(key, child.key)) {
              return child.value;
            }
          }
        }
        return none2();
      }
      case "IndexedNode": {
        const frag = hashFragment(shift2, hash2);
        const bit = toBitmap(frag);
        if (node.mask & bit) {
          node = node.children[fromBitmap(node.mask, bit)];
          shift2 += SIZE;
          break;
        }
        return none2();
      }
      case "ArrayNode": {
        node = node.children[hashFragment(shift2, hash2)];
        if (node) {
          shift2 += SIZE;
          break;
        }
        return none2();
      }
      default:
        return none2();
    }
  }
});
var has = /* @__PURE__ */ dual(2, (self, key) => isSome2(getHash(self, key, hash(key))));
var set = /* @__PURE__ */ dual(3, (self, key, value) => modifyAt(self, key, () => some2(value)));
var setTree = /* @__PURE__ */ dual(3, (self, newRoot, newSize) => {
  if (self._editable) {
    ;
    self._root = newRoot;
    self._size = newSize;
    return self;
  }
  return newRoot === self._root ? self : makeImpl(self._editable, self._edit, newRoot, newSize);
});
var keys = /* @__PURE__ */ __name((self) => new HashMapIterator(self, (key) => key), "keys");
var size = /* @__PURE__ */ __name((self) => self._size, "size");
var beginMutation = /* @__PURE__ */ __name((self) => makeImpl(true, self._edit + 1, self._root, self._size), "beginMutation");
var endMutation = /* @__PURE__ */ __name((self) => {
  ;
  self._editable = false;
  return self;
}, "endMutation");
var mutate = /* @__PURE__ */ dual(2, (self, f) => {
  const transient = beginMutation(self);
  f(transient);
  return endMutation(transient);
});
var modifyAt = /* @__PURE__ */ dual(3, (self, key, f) => modifyHash(self, key, hash(key), f));
var modifyHash = /* @__PURE__ */ dual(4, (self, key, hash2, f) => {
  const size11 = {
    value: self._size
  };
  const newRoot = self._root.modify(self._editable ? self._edit : NaN, 0, f, hash2, key, size11);
  return pipe(self, setTree(newRoot, size11.value));
});
var remove2 = /* @__PURE__ */ dual(2, (self, key) => modifyAt(self, key, none2));
var map3 = /* @__PURE__ */ dual(2, (self, f) => reduce2(self, empty5(), (map14, value, key) => set(map14, key, f(value, key))));
var forEach = /* @__PURE__ */ dual(2, (self, f) => reduce2(self, void 0, (_, value, key) => f(value, key)));
var reduce2 = /* @__PURE__ */ dual(3, (self, zero2, f) => {
  const root = self._root;
  if (root._tag === "LeafNode") {
    return isSome2(root.value) ? f(zero2, root.value.value, root.key) : zero2;
  }
  if (root._tag === "EmptyNode") {
    return zero2;
  }
  const toVisit = [root.children];
  let children;
  while (children = toVisit.pop()) {
    for (let i = 0, len = children.length; i < len; ) {
      const child = children[i++];
      if (child && !isEmptyNode(child)) {
        if (child._tag === "LeafNode") {
          if (isSome2(child.value)) {
            zero2 = f(zero2, child.value.value, child.key);
          }
        } else {
          toVisit.push(child.children);
        }
      }
    }
  }
  return zero2;
});

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/internal/hashSet.js
var HashSetSymbolKey = "effect/HashSet";
var HashSetTypeId = /* @__PURE__ */ Symbol.for(HashSetSymbolKey);
var HashSetProto = {
  [HashSetTypeId]: HashSetTypeId,
  [Symbol.iterator]() {
    return keys(this._keyMap);
  },
  [symbol]() {
    return cached(this, combine(hash(this._keyMap))(hash(HashSetSymbolKey)));
  },
  [symbol2](that) {
    if (isHashSet(that)) {
      return size(this._keyMap) === size(that._keyMap) && equals(this._keyMap, that._keyMap);
    }
    return false;
  },
  toString() {
    return format(this.toJSON());
  },
  toJSON() {
    return {
      _id: "HashSet",
      values: Array.from(this).map(toJSON)
    };
  },
  [NodeInspectSymbol]() {
    return this.toJSON();
  },
  pipe() {
    return pipeArguments(this, arguments);
  }
};
var makeImpl2 = /* @__PURE__ */ __name((keyMap) => {
  const set6 = Object.create(HashSetProto);
  set6._keyMap = keyMap;
  return set6;
}, "makeImpl");
var isHashSet = /* @__PURE__ */ __name((u) => hasProperty(u, HashSetTypeId), "isHashSet");
var _empty4 = /* @__PURE__ */ makeImpl2(/* @__PURE__ */ empty5());
var empty6 = /* @__PURE__ */ __name(() => _empty4, "empty");
var fromIterable4 = /* @__PURE__ */ __name((elements) => {
  const set6 = beginMutation2(empty6());
  for (const value of elements) {
    add3(set6, value);
  }
  return endMutation2(set6);
}, "fromIterable");
var make9 = /* @__PURE__ */ __name((...elements) => {
  const set6 = beginMutation2(empty6());
  for (const value of elements) {
    add3(set6, value);
  }
  return endMutation2(set6);
}, "make");
var has2 = /* @__PURE__ */ dual(2, (self, value) => has(self._keyMap, value));
var size2 = /* @__PURE__ */ __name((self) => size(self._keyMap), "size");
var beginMutation2 = /* @__PURE__ */ __name((self) => makeImpl2(beginMutation(self._keyMap)), "beginMutation");
var endMutation2 = /* @__PURE__ */ __name((self) => {
  ;
  self._keyMap._editable = false;
  return self;
}, "endMutation");
var mutate2 = /* @__PURE__ */ dual(2, (self, f) => {
  const transient = beginMutation2(self);
  f(transient);
  return endMutation2(transient);
});
var add3 = /* @__PURE__ */ dual(2, (self, value) => self._keyMap._editable ? (set(value, true)(self._keyMap), self) : makeImpl2(set(value, true)(self._keyMap)));
var remove3 = /* @__PURE__ */ dual(2, (self, value) => self._keyMap._editable ? (remove2(value)(self._keyMap), self) : makeImpl2(remove2(value)(self._keyMap)));
var difference2 = /* @__PURE__ */ dual(2, (self, that) => mutate2(self, (set6) => {
  for (const value of that) {
    remove3(set6, value);
  }
}));
var union2 = /* @__PURE__ */ dual(2, (self, that) => mutate2(empty6(), (set6) => {
  forEach2(self, (value) => add3(set6, value));
  for (const value of that) {
    add3(set6, value);
  }
}));
var map4 = /* @__PURE__ */ dual(2, (self, f) => mutate2(empty6(), (set6) => {
  forEach2(self, (a) => {
    const b = f(a);
    if (!has2(set6, b)) {
      add3(set6, b);
    }
  });
}));
var flatMap3 = /* @__PURE__ */ dual(2, (self, f) => mutate2(empty6(), (set6) => {
  forEach2(self, (a) => {
    for (const b of f(a)) {
      if (!has2(set6, b)) {
        add3(set6, b);
      }
    }
  });
}));
var forEach2 = /* @__PURE__ */ dual(2, (self, f) => forEach(self._keyMap, (_, k) => f(k)));
var reduce3 = /* @__PURE__ */ dual(3, (self, zero2, f) => reduce2(self._keyMap, zero2, (z, _, a) => f(z, a)));

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/HashSet.js
var empty7 = empty6;
var fromIterable5 = fromIterable4;
var make10 = make9;
var has3 = has2;
var size3 = size2;
var add4 = add3;
var remove4 = remove3;
var difference3 = difference2;
var union3 = union2;
var map5 = map4;
var flatMap4 = flatMap3;
var reduce4 = reduce3;

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/MutableRef.js
var TypeId6 = /* @__PURE__ */ Symbol.for("effect/MutableRef");
var MutableRefProto = {
  [TypeId6]: TypeId6,
  toString() {
    return format(this.toJSON());
  },
  toJSON() {
    return {
      _id: "MutableRef",
      current: toJSON(this.current)
    };
  },
  [NodeInspectSymbol]() {
    return this.toJSON();
  },
  pipe() {
    return pipeArguments(this, arguments);
  }
};
var make11 = /* @__PURE__ */ __name((value) => {
  const ref = Object.create(MutableRefProto);
  ref.current = value;
  return ref;
}, "make");
var compareAndSet = /* @__PURE__ */ dual(3, (self, oldValue, newValue) => {
  if (equals(oldValue, self.current)) {
    self.current = newValue;
    return true;
  }
  return false;
});
var get6 = /* @__PURE__ */ __name((self) => self.current, "get");
var set2 = /* @__PURE__ */ dual(2, (self, value) => {
  self.current = value;
  return self;
});

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/internal/fiberId.js
var FiberIdSymbolKey = "effect/FiberId";
var FiberIdTypeId = /* @__PURE__ */ Symbol.for(FiberIdSymbolKey);
var OP_NONE = "None";
var OP_RUNTIME = "Runtime";
var OP_COMPOSITE = "Composite";
var emptyHash = /* @__PURE__ */ string(`${FiberIdSymbolKey}-${OP_NONE}`);
var None = class {
  static {
    __name(this, "None");
  }
  [FiberIdTypeId] = FiberIdTypeId;
  _tag = OP_NONE;
  id = -1;
  startTimeMillis = -1;
  [symbol]() {
    return emptyHash;
  }
  [symbol2](that) {
    return isFiberId(that) && that._tag === OP_NONE;
  }
  toString() {
    return format(this.toJSON());
  }
  toJSON() {
    return {
      _id: "FiberId",
      _tag: this._tag
    };
  }
  [NodeInspectSymbol]() {
    return this.toJSON();
  }
};
var Runtime = class {
  static {
    __name(this, "Runtime");
  }
  id;
  startTimeMillis;
  [FiberIdTypeId] = FiberIdTypeId;
  _tag = OP_RUNTIME;
  constructor(id, startTimeMillis) {
    this.id = id;
    this.startTimeMillis = startTimeMillis;
  }
  [symbol]() {
    return cached(this, string(`${FiberIdSymbolKey}-${this._tag}-${this.id}-${this.startTimeMillis}`));
  }
  [symbol2](that) {
    return isFiberId(that) && that._tag === OP_RUNTIME && this.id === that.id && this.startTimeMillis === that.startTimeMillis;
  }
  toString() {
    return format(this.toJSON());
  }
  toJSON() {
    return {
      _id: "FiberId",
      _tag: this._tag,
      id: this.id,
      startTimeMillis: this.startTimeMillis
    };
  }
  [NodeInspectSymbol]() {
    return this.toJSON();
  }
};
var Composite = class {
  static {
    __name(this, "Composite");
  }
  left;
  right;
  [FiberIdTypeId] = FiberIdTypeId;
  _tag = OP_COMPOSITE;
  constructor(left3, right3) {
    this.left = left3;
    this.right = right3;
  }
  _hash;
  [symbol]() {
    return pipe(string(`${FiberIdSymbolKey}-${this._tag}`), combine(hash(this.left)), combine(hash(this.right)), cached(this));
  }
  [symbol2](that) {
    return isFiberId(that) && that._tag === OP_COMPOSITE && equals(this.left, that.left) && equals(this.right, that.right);
  }
  toString() {
    return format(this.toJSON());
  }
  toJSON() {
    return {
      _id: "FiberId",
      _tag: this._tag,
      left: toJSON(this.left),
      right: toJSON(this.right)
    };
  }
  [NodeInspectSymbol]() {
    return this.toJSON();
  }
};
var none3 = /* @__PURE__ */ new None();
var isFiberId = /* @__PURE__ */ __name((self) => hasProperty(self, FiberIdTypeId), "isFiberId");
var combine2 = /* @__PURE__ */ dual(2, (self, that) => {
  if (self._tag === OP_NONE) {
    return that;
  }
  if (that._tag === OP_NONE) {
    return self;
  }
  return new Composite(self, that);
});
var ids = /* @__PURE__ */ __name((self) => {
  switch (self._tag) {
    case OP_NONE: {
      return empty7();
    }
    case OP_RUNTIME: {
      return make10(self.id);
    }
    case OP_COMPOSITE: {
      return pipe(ids(self.left), union3(ids(self.right)));
    }
  }
}, "ids");
var _fiberCounter = /* @__PURE__ */ globalValue(/* @__PURE__ */ Symbol.for("effect/Fiber/Id/_fiberCounter"), () => make11(0));
var threadName = /* @__PURE__ */ __name((self) => {
  const identifiers = Array.from(ids(self)).map((n) => `#${n}`).join(",");
  return identifiers;
}, "threadName");
var unsafeMake = /* @__PURE__ */ __name(() => {
  const id = get6(_fiberCounter);
  pipe(_fiberCounter, set2(id + 1));
  return new Runtime(id, Date.now());
}, "unsafeMake");

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/FiberId.js
var none4 = none3;
var combine3 = combine2;
var ids2 = ids;
var threadName2 = threadName;
var unsafeMake2 = unsafeMake;

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/HashMap.js
var empty8 = empty5;
var fromIterable6 = fromIterable3;
var isEmpty3 = isEmpty2;
var get7 = get5;
var set3 = set;
var keys2 = keys;
var mutate3 = mutate;
var modifyAt2 = modifyAt;
var map6 = map3;
var forEach3 = forEach;
var reduce5 = reduce2;

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/List.js
var TypeId7 = /* @__PURE__ */ Symbol.for("effect/List");
var toArray2 = /* @__PURE__ */ __name((self) => fromIterable(self), "toArray");
var getEquivalence3 = /* @__PURE__ */ __name((isEquivalent) => mapInput(getEquivalence(isEquivalent), toArray2), "getEquivalence");
var _equivalence4 = /* @__PURE__ */ getEquivalence3(equals);
var ConsProto = {
  [TypeId7]: TypeId7,
  _tag: "Cons",
  toString() {
    return format(this.toJSON());
  },
  toJSON() {
    return {
      _id: "List",
      _tag: "Cons",
      values: toArray2(this).map(toJSON)
    };
  },
  [NodeInspectSymbol]() {
    return this.toJSON();
  },
  [symbol2](that) {
    return isList(that) && this._tag === that._tag && _equivalence4(this, that);
  },
  [symbol]() {
    return cached(this, array2(toArray2(this)));
  },
  [Symbol.iterator]() {
    let done7 = false;
    let self = this;
    return {
      next() {
        if (done7) {
          return this.return();
        }
        if (self._tag === "Nil") {
          done7 = true;
          return this.return();
        }
        const value = self.head;
        self = self.tail;
        return {
          done: done7,
          value
        };
      },
      return(value) {
        if (!done7) {
          done7 = true;
        }
        return {
          done: true,
          value
        };
      }
    };
  },
  pipe() {
    return pipeArguments(this, arguments);
  }
};
var makeCons = /* @__PURE__ */ __name((head5, tail) => {
  const cons2 = Object.create(ConsProto);
  cons2.head = head5;
  cons2.tail = tail;
  return cons2;
}, "makeCons");
var NilHash = /* @__PURE__ */ string("Nil");
var NilProto = {
  [TypeId7]: TypeId7,
  _tag: "Nil",
  toString() {
    return format(this.toJSON());
  },
  toJSON() {
    return {
      _id: "List",
      _tag: "Nil"
    };
  },
  [NodeInspectSymbol]() {
    return this.toJSON();
  },
  [symbol]() {
    return NilHash;
  },
  [symbol2](that) {
    return isList(that) && this._tag === that._tag;
  },
  [Symbol.iterator]() {
    return {
      next() {
        return {
          done: true,
          value: void 0
        };
      }
    };
  },
  pipe() {
    return pipeArguments(this, arguments);
  }
};
var _Nil = /* @__PURE__ */ Object.create(NilProto);
var isList = /* @__PURE__ */ __name((u) => hasProperty(u, TypeId7), "isList");
var isNil = /* @__PURE__ */ __name((self) => self._tag === "Nil", "isNil");
var isCons = /* @__PURE__ */ __name((self) => self._tag === "Cons", "isCons");
var nil = /* @__PURE__ */ __name(() => _Nil, "nil");
var cons = /* @__PURE__ */ __name((head5, tail) => makeCons(head5, tail), "cons");
var empty9 = nil;
var of3 = /* @__PURE__ */ __name((value) => makeCons(value, _Nil), "of");
var appendAll3 = /* @__PURE__ */ dual(2, (self, that) => prependAll(that, self));
var prepend3 = /* @__PURE__ */ dual(2, (self, element) => cons(element, self));
var prependAll = /* @__PURE__ */ dual(2, (self, prefix) => {
  if (isNil(self)) {
    return prefix;
  } else if (isNil(prefix)) {
    return self;
  } else {
    const result = makeCons(prefix.head, self);
    let curr = result;
    let that = prefix.tail;
    while (!isNil(that)) {
      const temp = makeCons(that.head, self);
      curr.tail = temp;
      curr = temp;
      that = that.tail;
    }
    return result;
  }
});
var reduce6 = /* @__PURE__ */ dual(3, (self, zero2, f) => {
  let acc = zero2;
  let these = self;
  while (!isNil(these)) {
    acc = f(acc, these.head);
    these = these.tail;
  }
  return acc;
});
var reverse3 = /* @__PURE__ */ __name((self) => {
  let result = empty9();
  let these = self;
  while (!isNil(these)) {
    result = prepend3(result, these.head);
    these = these.tail;
  }
  return result;
}, "reverse");

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/internal/data.js
var ArrayProto = /* @__PURE__ */ Object.assign(/* @__PURE__ */ Object.create(Array.prototype), {
  [symbol]() {
    return cached(this, array2(this));
  },
  [symbol2](that) {
    if (Array.isArray(that) && this.length === that.length) {
      return this.every((v, i) => equals(v, that[i]));
    } else {
      return false;
    }
  }
});
var Structural = /* @__PURE__ */ (function() {
  function Structural2(args2) {
    if (args2) {
      Object.assign(this, args2);
    }
  }
  __name(Structural2, "Structural");
  Structural2.prototype = StructuralPrototype;
  return Structural2;
})();
var struct = /* @__PURE__ */ __name((as7) => Object.assign(Object.create(StructuralPrototype), as7), "struct");

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/internal/differ/contextPatch.js
var ContextPatchTypeId = /* @__PURE__ */ Symbol.for("effect/DifferContextPatch");
function variance(a) {
  return a;
}
__name(variance, "variance");
var PatchProto = {
  ...Structural.prototype,
  [ContextPatchTypeId]: {
    _Value: variance,
    _Patch: variance
  }
};
var EmptyProto = /* @__PURE__ */ Object.assign(/* @__PURE__ */ Object.create(PatchProto), {
  _tag: "Empty"
});
var _empty5 = /* @__PURE__ */ Object.create(EmptyProto);
var empty10 = /* @__PURE__ */ __name(() => _empty5, "empty");
var AndThenProto = /* @__PURE__ */ Object.assign(/* @__PURE__ */ Object.create(PatchProto), {
  _tag: "AndThen"
});
var makeAndThen = /* @__PURE__ */ __name((first2, second) => {
  const o = Object.create(AndThenProto);
  o.first = first2;
  o.second = second;
  return o;
}, "makeAndThen");
var AddServiceProto = /* @__PURE__ */ Object.assign(/* @__PURE__ */ Object.create(PatchProto), {
  _tag: "AddService"
});
var makeAddService = /* @__PURE__ */ __name((key, service) => {
  const o = Object.create(AddServiceProto);
  o.key = key;
  o.service = service;
  return o;
}, "makeAddService");
var RemoveServiceProto = /* @__PURE__ */ Object.assign(/* @__PURE__ */ Object.create(PatchProto), {
  _tag: "RemoveService"
});
var makeRemoveService = /* @__PURE__ */ __name((key) => {
  const o = Object.create(RemoveServiceProto);
  o.key = key;
  return o;
}, "makeRemoveService");
var UpdateServiceProto = /* @__PURE__ */ Object.assign(/* @__PURE__ */ Object.create(PatchProto), {
  _tag: "UpdateService"
});
var makeUpdateService = /* @__PURE__ */ __name((key, update5) => {
  const o = Object.create(UpdateServiceProto);
  o.key = key;
  o.update = update5;
  return o;
}, "makeUpdateService");
var diff = /* @__PURE__ */ __name((oldValue, newValue) => {
  const missingServices = new Map(oldValue.unsafeMap);
  let patch9 = empty10();
  for (const [tag, newService] of newValue.unsafeMap.entries()) {
    if (missingServices.has(tag)) {
      const old = missingServices.get(tag);
      missingServices.delete(tag);
      if (!equals(old, newService)) {
        patch9 = combine4(makeUpdateService(tag, () => newService))(patch9);
      }
    } else {
      missingServices.delete(tag);
      patch9 = combine4(makeAddService(tag, newService))(patch9);
    }
  }
  for (const [tag] of missingServices.entries()) {
    patch9 = combine4(makeRemoveService(tag))(patch9);
  }
  return patch9;
}, "diff");
var combine4 = /* @__PURE__ */ dual(2, (self, that) => makeAndThen(self, that));
var patch = /* @__PURE__ */ dual(2, (self, context4) => {
  if (self._tag === "Empty") {
    return context4;
  }
  let wasServiceUpdated = false;
  let patches = of2(self);
  const updatedContext = new Map(context4.unsafeMap);
  while (isNonEmpty(patches)) {
    const head5 = headNonEmpty2(patches);
    const tail = tailNonEmpty2(patches);
    switch (head5._tag) {
      case "Empty": {
        patches = tail;
        break;
      }
      case "AddService": {
        updatedContext.set(head5.key, head5.service);
        patches = tail;
        break;
      }
      case "AndThen": {
        patches = prepend2(prepend2(tail, head5.second), head5.first);
        break;
      }
      case "RemoveService": {
        updatedContext.delete(head5.key);
        patches = tail;
        break;
      }
      case "UpdateService": {
        updatedContext.set(head5.key, head5.update(updatedContext.get(head5.key)));
        wasServiceUpdated = true;
        patches = tail;
        break;
      }
    }
  }
  if (!wasServiceUpdated) {
    return makeContext(updatedContext);
  }
  const map14 = /* @__PURE__ */ new Map();
  for (const [tag] of context4.unsafeMap) {
    if (updatedContext.has(tag)) {
      map14.set(tag, updatedContext.get(tag));
      updatedContext.delete(tag);
    }
  }
  for (const [tag, s] of updatedContext) {
    map14.set(tag, s);
  }
  return makeContext(map14);
});

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/internal/differ/hashSetPatch.js
var HashSetPatchTypeId = /* @__PURE__ */ Symbol.for("effect/DifferHashSetPatch");
function variance2(a) {
  return a;
}
__name(variance2, "variance");
var PatchProto2 = {
  ...Structural.prototype,
  [HashSetPatchTypeId]: {
    _Value: variance2,
    _Key: variance2,
    _Patch: variance2
  }
};
var EmptyProto2 = /* @__PURE__ */ Object.assign(/* @__PURE__ */ Object.create(PatchProto2), {
  _tag: "Empty"
});
var _empty6 = /* @__PURE__ */ Object.create(EmptyProto2);
var empty11 = /* @__PURE__ */ __name(() => _empty6, "empty");
var AndThenProto2 = /* @__PURE__ */ Object.assign(/* @__PURE__ */ Object.create(PatchProto2), {
  _tag: "AndThen"
});
var makeAndThen2 = /* @__PURE__ */ __name((first2, second) => {
  const o = Object.create(AndThenProto2);
  o.first = first2;
  o.second = second;
  return o;
}, "makeAndThen");
var AddProto = /* @__PURE__ */ Object.assign(/* @__PURE__ */ Object.create(PatchProto2), {
  _tag: "Add"
});
var makeAdd = /* @__PURE__ */ __name((value) => {
  const o = Object.create(AddProto);
  o.value = value;
  return o;
}, "makeAdd");
var RemoveProto = /* @__PURE__ */ Object.assign(/* @__PURE__ */ Object.create(PatchProto2), {
  _tag: "Remove"
});
var makeRemove = /* @__PURE__ */ __name((value) => {
  const o = Object.create(RemoveProto);
  o.value = value;
  return o;
}, "makeRemove");
var diff2 = /* @__PURE__ */ __name((oldValue, newValue) => {
  const [removed, patch9] = reduce4([oldValue, empty11()], ([set6, patch10], value) => {
    if (has3(value)(set6)) {
      return [remove4(value)(set6), patch10];
    }
    return [set6, combine5(makeAdd(value))(patch10)];
  })(newValue);
  return reduce4(patch9, (patch10, value) => combine5(makeRemove(value))(patch10))(removed);
}, "diff");
var combine5 = /* @__PURE__ */ dual(2, (self, that) => makeAndThen2(self, that));
var patch2 = /* @__PURE__ */ dual(2, (self, oldValue) => {
  if (self._tag === "Empty") {
    return oldValue;
  }
  let set6 = oldValue;
  let patches = of2(self);
  while (isNonEmpty(patches)) {
    const head5 = headNonEmpty2(patches);
    const tail = tailNonEmpty2(patches);
    switch (head5._tag) {
      case "Empty": {
        patches = tail;
        break;
      }
      case "AndThen": {
        patches = prepend2(head5.first)(prepend2(head5.second)(tail));
        break;
      }
      case "Add": {
        set6 = add4(head5.value)(set6);
        patches = tail;
        break;
      }
      case "Remove": {
        set6 = remove4(head5.value)(set6);
        patches = tail;
      }
    }
  }
  return set6;
});

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/internal/differ/readonlyArrayPatch.js
var ReadonlyArrayPatchTypeId = /* @__PURE__ */ Symbol.for("effect/DifferReadonlyArrayPatch");
function variance3(a) {
  return a;
}
__name(variance3, "variance");
var PatchProto3 = {
  ...Structural.prototype,
  [ReadonlyArrayPatchTypeId]: {
    _Value: variance3,
    _Patch: variance3
  }
};
var EmptyProto3 = /* @__PURE__ */ Object.assign(/* @__PURE__ */ Object.create(PatchProto3), {
  _tag: "Empty"
});
var _empty7 = /* @__PURE__ */ Object.create(EmptyProto3);
var empty12 = /* @__PURE__ */ __name(() => _empty7, "empty");
var AndThenProto3 = /* @__PURE__ */ Object.assign(/* @__PURE__ */ Object.create(PatchProto3), {
  _tag: "AndThen"
});
var makeAndThen3 = /* @__PURE__ */ __name((first2, second) => {
  const o = Object.create(AndThenProto3);
  o.first = first2;
  o.second = second;
  return o;
}, "makeAndThen");
var AppendProto = /* @__PURE__ */ Object.assign(/* @__PURE__ */ Object.create(PatchProto3), {
  _tag: "Append"
});
var makeAppend = /* @__PURE__ */ __name((values3) => {
  const o = Object.create(AppendProto);
  o.values = values3;
  return o;
}, "makeAppend");
var SliceProto = /* @__PURE__ */ Object.assign(/* @__PURE__ */ Object.create(PatchProto3), {
  _tag: "Slice"
});
var makeSlice = /* @__PURE__ */ __name((from, until) => {
  const o = Object.create(SliceProto);
  o.from = from;
  o.until = until;
  return o;
}, "makeSlice");
var UpdateProto = /* @__PURE__ */ Object.assign(/* @__PURE__ */ Object.create(PatchProto3), {
  _tag: "Update"
});
var makeUpdate = /* @__PURE__ */ __name((index, patch9) => {
  const o = Object.create(UpdateProto);
  o.index = index;
  o.patch = patch9;
  return o;
}, "makeUpdate");
var diff3 = /* @__PURE__ */ __name((options) => {
  let i = 0;
  let patch9 = empty12();
  while (i < options.oldValue.length && i < options.newValue.length) {
    const oldElement = options.oldValue[i];
    const newElement = options.newValue[i];
    const valuePatch = options.differ.diff(oldElement, newElement);
    if (!equals(valuePatch, options.differ.empty)) {
      patch9 = combine6(patch9, makeUpdate(i, valuePatch));
    }
    i = i + 1;
  }
  if (i < options.oldValue.length) {
    patch9 = combine6(patch9, makeSlice(0, i));
  }
  if (i < options.newValue.length) {
    patch9 = combine6(patch9, makeAppend(drop(i)(options.newValue)));
  }
  return patch9;
}, "diff");
var combine6 = /* @__PURE__ */ dual(2, (self, that) => makeAndThen3(self, that));
var patch3 = /* @__PURE__ */ dual(3, (self, oldValue, differ3) => {
  if (self._tag === "Empty") {
    return oldValue;
  }
  let readonlyArray2 = oldValue.slice();
  let patches = of(self);
  while (isNonEmptyArray2(patches)) {
    const head5 = headNonEmpty(patches);
    const tail = tailNonEmpty(patches);
    switch (head5._tag) {
      case "Empty": {
        patches = tail;
        break;
      }
      case "AndThen": {
        tail.unshift(head5.first, head5.second);
        patches = tail;
        break;
      }
      case "Append": {
        for (const value of head5.values) {
          readonlyArray2.push(value);
        }
        patches = tail;
        break;
      }
      case "Slice": {
        readonlyArray2 = readonlyArray2.slice(head5.from, head5.until);
        patches = tail;
        break;
      }
      case "Update": {
        readonlyArray2[head5.index] = differ3.patch(head5.patch, readonlyArray2[head5.index]);
        patches = tail;
        break;
      }
    }
  }
  return readonlyArray2;
});

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/internal/differ.js
var DifferTypeId = /* @__PURE__ */ Symbol.for("effect/Differ");
var DifferProto = {
  [DifferTypeId]: {
    _P: identity,
    _V: identity
  },
  pipe() {
    return pipeArguments(this, arguments);
  }
};
var make14 = /* @__PURE__ */ __name((params) => {
  const differ3 = Object.create(DifferProto);
  differ3.empty = params.empty;
  differ3.diff = params.diff;
  differ3.combine = params.combine;
  differ3.patch = params.patch;
  return differ3;
}, "make");
var environment = /* @__PURE__ */ __name(() => make14({
  empty: empty10(),
  combine: /* @__PURE__ */ __name((first2, second) => combine4(second)(first2), "combine"),
  diff: /* @__PURE__ */ __name((oldValue, newValue) => diff(oldValue, newValue), "diff"),
  patch: /* @__PURE__ */ __name((patch9, oldValue) => patch(oldValue)(patch9), "patch")
}), "environment");
var hashSet = /* @__PURE__ */ __name(() => make14({
  empty: empty11(),
  combine: /* @__PURE__ */ __name((first2, second) => combine5(second)(first2), "combine"),
  diff: /* @__PURE__ */ __name((oldValue, newValue) => diff2(oldValue, newValue), "diff"),
  patch: /* @__PURE__ */ __name((patch9, oldValue) => patch2(oldValue)(patch9), "patch")
}), "hashSet");
var readonlyArray = /* @__PURE__ */ __name((differ3) => make14({
  empty: empty12(),
  combine: /* @__PURE__ */ __name((first2, second) => combine6(first2, second), "combine"),
  diff: /* @__PURE__ */ __name((oldValue, newValue) => diff3({
    oldValue,
    newValue,
    differ: differ3
  }), "diff"),
  patch: /* @__PURE__ */ __name((patch9, oldValue) => patch3(patch9, oldValue, differ3), "patch")
}), "readonlyArray");
var update = /* @__PURE__ */ __name(() => updateWith((_, a) => a), "update");
var updateWith = /* @__PURE__ */ __name((f) => make14({
  empty: identity,
  combine: /* @__PURE__ */ __name((first2, second) => {
    if (first2 === identity) {
      return second;
    }
    if (second === identity) {
      return first2;
    }
    return (a) => second(first2(a));
  }, "combine"),
  diff: /* @__PURE__ */ __name((oldValue, newValue) => {
    if (equals(oldValue, newValue)) {
      return identity;
    }
    return constant(newValue);
  }, "diff"),
  patch: /* @__PURE__ */ __name((patch9, oldValue) => f(oldValue, patch9(oldValue)), "patch")
}), "updateWith");

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/internal/runtimeFlagsPatch.js
var BIT_MASK = 255;
var BIT_SHIFT = 8;
var active = /* @__PURE__ */ __name((patch9) => patch9 & BIT_MASK, "active");
var enabled = /* @__PURE__ */ __name((patch9) => patch9 >> BIT_SHIFT & BIT_MASK, "enabled");
var make15 = /* @__PURE__ */ __name((active2, enabled2) => (active2 & BIT_MASK) + ((enabled2 & active2 & BIT_MASK) << BIT_SHIFT), "make");
var empty13 = /* @__PURE__ */ make15(0, 0);
var enable = /* @__PURE__ */ __name((flag) => make15(flag, flag), "enable");
var disable = /* @__PURE__ */ __name((flag) => make15(flag, 0), "disable");
var exclude = /* @__PURE__ */ dual(2, (self, flag) => make15(active(self) & ~flag, enabled(self)));
var andThen = /* @__PURE__ */ dual(2, (self, that) => self | that);
var invert = /* @__PURE__ */ __name((n) => ~n >>> 0 & BIT_MASK, "invert");

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/internal/runtimeFlags.js
var None2 = 0;
var Interruption = 1 << 0;
var OpSupervision = 1 << 1;
var RuntimeMetrics = 1 << 2;
var WindDown = 1 << 4;
var CooperativeYielding = 1 << 5;
var cooperativeYielding = /* @__PURE__ */ __name((self) => isEnabled(self, CooperativeYielding), "cooperativeYielding");
var disable2 = /* @__PURE__ */ dual(2, (self, flag) => self & ~flag);
var enable2 = /* @__PURE__ */ dual(2, (self, flag) => self | flag);
var interruptible = /* @__PURE__ */ __name((self) => interruption(self) && !windDown(self), "interruptible");
var interruption = /* @__PURE__ */ __name((self) => isEnabled(self, Interruption), "interruption");
var isEnabled = /* @__PURE__ */ dual(2, (self, flag) => (self & flag) !== 0);
var make16 = /* @__PURE__ */ __name((...flags) => flags.reduce((a, b) => a | b, 0), "make");
var none5 = /* @__PURE__ */ make16(None2);
var runtimeMetrics = /* @__PURE__ */ __name((self) => isEnabled(self, RuntimeMetrics), "runtimeMetrics");
var windDown = /* @__PURE__ */ __name((self) => isEnabled(self, WindDown), "windDown");
var diff4 = /* @__PURE__ */ dual(2, (self, that) => make15(self ^ that, that));
var patch4 = /* @__PURE__ */ dual(2, (self, patch9) => self & (invert(active(patch9)) | enabled(patch9)) | active(patch9) & enabled(patch9));
var differ = /* @__PURE__ */ make14({
  empty: empty13,
  diff: /* @__PURE__ */ __name((oldValue, newValue) => diff4(oldValue, newValue), "diff"),
  combine: /* @__PURE__ */ __name((first2, second) => andThen(second)(first2), "combine"),
  patch: /* @__PURE__ */ __name((_patch, oldValue) => patch4(oldValue, _patch), "patch")
});

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/RuntimeFlagsPatch.js
var empty14 = empty13;
var enable3 = enable;
var disable3 = disable;
var exclude2 = exclude;

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/internal/blockedRequests.js
var empty15 = {
  _tag: "Empty"
};
var par = /* @__PURE__ */ __name((self, that) => ({
  _tag: "Par",
  left: self,
  right: that
}), "par");
var seq = /* @__PURE__ */ __name((self, that) => ({
  _tag: "Seq",
  left: self,
  right: that
}), "seq");
var single = /* @__PURE__ */ __name((dataSource, blockedRequest) => ({
  _tag: "Single",
  dataSource,
  blockedRequest
}), "single");
var flatten2 = /* @__PURE__ */ __name((self) => {
  let current = of3(self);
  let updated = empty9();
  while (1) {
    const [parallel5, sequential5] = reduce6(current, [parallelCollectionEmpty(), empty9()], ([parallel6, sequential6], blockedRequest) => {
      const [par2, seq2] = step(blockedRequest);
      return [parallelCollectionCombine(parallel6, par2), appendAll3(sequential6, seq2)];
    });
    updated = merge4(updated, parallel5);
    if (isNil(sequential5)) {
      return reverse3(updated);
    }
    current = sequential5;
  }
  throw new Error("BUG: BlockedRequests.flatten - please report an issue at https://github.com/Effect-TS/effect/issues");
}, "flatten");
var step = /* @__PURE__ */ __name((requests) => {
  let current = requests;
  let parallel5 = parallelCollectionEmpty();
  let stack = empty9();
  let sequential5 = empty9();
  while (1) {
    switch (current._tag) {
      case "Empty": {
        if (isNil(stack)) {
          return [parallel5, sequential5];
        }
        current = stack.head;
        stack = stack.tail;
        break;
      }
      case "Par": {
        stack = cons(current.right, stack);
        current = current.left;
        break;
      }
      case "Seq": {
        const left3 = current.left;
        const right3 = current.right;
        switch (left3._tag) {
          case "Empty": {
            current = right3;
            break;
          }
          case "Par": {
            const l = left3.left;
            const r = left3.right;
            current = par(seq(l, right3), seq(r, right3));
            break;
          }
          case "Seq": {
            const l = left3.left;
            const r = left3.right;
            current = seq(l, seq(r, right3));
            break;
          }
          case "Single": {
            current = left3;
            sequential5 = cons(right3, sequential5);
            break;
          }
        }
        break;
      }
      case "Single": {
        parallel5 = parallelCollectionAdd(parallel5, current);
        if (isNil(stack)) {
          return [parallel5, sequential5];
        }
        current = stack.head;
        stack = stack.tail;
        break;
      }
    }
  }
  throw new Error("BUG: BlockedRequests.step - please report an issue at https://github.com/Effect-TS/effect/issues");
}, "step");
var merge4 = /* @__PURE__ */ __name((sequential5, parallel5) => {
  if (isNil(sequential5)) {
    return of3(parallelCollectionToSequentialCollection(parallel5));
  }
  if (parallelCollectionIsEmpty(parallel5)) {
    return sequential5;
  }
  const seqHeadKeys = sequentialCollectionKeys(sequential5.head);
  const parKeys = parallelCollectionKeys(parallel5);
  if (seqHeadKeys.length === 1 && parKeys.length === 1 && equals(seqHeadKeys[0], parKeys[0])) {
    return cons(sequentialCollectionCombine(sequential5.head, parallelCollectionToSequentialCollection(parallel5)), sequential5.tail);
  }
  return cons(parallelCollectionToSequentialCollection(parallel5), sequential5);
}, "merge");
var EntryTypeId = /* @__PURE__ */ Symbol.for("effect/RequestBlock/Entry");
var EntryImpl = class {
  static {
    __name(this, "EntryImpl");
  }
  request;
  result;
  listeners;
  ownerId;
  state;
  [EntryTypeId] = blockedRequestVariance;
  constructor(request2, result, listeners, ownerId, state) {
    this.request = request2;
    this.result = result;
    this.listeners = listeners;
    this.ownerId = ownerId;
    this.state = state;
  }
};
var blockedRequestVariance = {
  /* c8 ignore next */
  _R: /* @__PURE__ */ __name((_) => _, "_R")
};
var makeEntry = /* @__PURE__ */ __name((options) => new EntryImpl(options.request, options.result, options.listeners, options.ownerId, options.state), "makeEntry");
var RequestBlockParallelTypeId = /* @__PURE__ */ Symbol.for("effect/RequestBlock/RequestBlockParallel");
var parallelVariance = {
  /* c8 ignore next */
  _R: /* @__PURE__ */ __name((_) => _, "_R")
};
var ParallelImpl = class {
  static {
    __name(this, "ParallelImpl");
  }
  map;
  [RequestBlockParallelTypeId] = parallelVariance;
  constructor(map14) {
    this.map = map14;
  }
};
var parallelCollectionEmpty = /* @__PURE__ */ __name(() => new ParallelImpl(empty8()), "parallelCollectionEmpty");
var parallelCollectionAdd = /* @__PURE__ */ __name((self, blockedRequest) => new ParallelImpl(modifyAt2(self.map, blockedRequest.dataSource, (_) => orElseSome(map(_, append2(blockedRequest.blockedRequest)), () => of2(blockedRequest.blockedRequest)))), "parallelCollectionAdd");
var parallelCollectionCombine = /* @__PURE__ */ __name((self, that) => new ParallelImpl(reduce5(self.map, that.map, (map14, value, key) => set3(map14, key, match2(get7(map14, key), {
  onNone: /* @__PURE__ */ __name(() => value, "onNone"),
  onSome: /* @__PURE__ */ __name((other) => appendAll2(value, other), "onSome")
})))), "parallelCollectionCombine");
var parallelCollectionIsEmpty = /* @__PURE__ */ __name((self) => isEmpty3(self.map), "parallelCollectionIsEmpty");
var parallelCollectionKeys = /* @__PURE__ */ __name((self) => Array.from(keys2(self.map)), "parallelCollectionKeys");
var parallelCollectionToSequentialCollection = /* @__PURE__ */ __name((self) => sequentialCollectionMake(map6(self.map, (x) => of2(x))), "parallelCollectionToSequentialCollection");
var SequentialCollectionTypeId = /* @__PURE__ */ Symbol.for("effect/RequestBlock/RequestBlockSequential");
var sequentialVariance = {
  /* c8 ignore next */
  _R: /* @__PURE__ */ __name((_) => _, "_R")
};
var SequentialImpl = class {
  static {
    __name(this, "SequentialImpl");
  }
  map;
  [SequentialCollectionTypeId] = sequentialVariance;
  constructor(map14) {
    this.map = map14;
  }
};
var sequentialCollectionMake = /* @__PURE__ */ __name((map14) => new SequentialImpl(map14), "sequentialCollectionMake");
var sequentialCollectionCombine = /* @__PURE__ */ __name((self, that) => new SequentialImpl(reduce5(that.map, self.map, (map14, value, key) => set3(map14, key, match2(get7(map14, key), {
  onNone: /* @__PURE__ */ __name(() => empty4(), "onNone"),
  onSome: /* @__PURE__ */ __name((a) => appendAll2(a, value), "onSome")
})))), "sequentialCollectionCombine");
var sequentialCollectionKeys = /* @__PURE__ */ __name((self) => Array.from(keys2(self.map)), "sequentialCollectionKeys");
var sequentialCollectionToChunk = /* @__PURE__ */ __name((self) => Array.from(self.map), "sequentialCollectionToChunk");

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/internal/opCodes/cause.js
var OP_DIE = "Die";
var OP_EMPTY = "Empty";
var OP_FAIL = "Fail";
var OP_INTERRUPT = "Interrupt";
var OP_PARALLEL = "Parallel";
var OP_SEQUENTIAL = "Sequential";

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/internal/cause.js
var CauseSymbolKey = "effect/Cause";
var CauseTypeId = /* @__PURE__ */ Symbol.for(CauseSymbolKey);
var variance4 = {
  /* c8 ignore next */
  _E: /* @__PURE__ */ __name((_) => _, "_E")
};
var proto = {
  [CauseTypeId]: variance4,
  [symbol]() {
    return pipe(hash(CauseSymbolKey), combine(hash(flattenCause(this))), cached(this));
  },
  [symbol2](that) {
    return isCause(that) && causeEquals(this, that);
  },
  pipe() {
    return pipeArguments(this, arguments);
  },
  toJSON() {
    switch (this._tag) {
      case "Empty":
        return {
          _id: "Cause",
          _tag: this._tag
        };
      case "Die":
        return {
          _id: "Cause",
          _tag: this._tag,
          defect: toJSON(this.defect)
        };
      case "Interrupt":
        return {
          _id: "Cause",
          _tag: this._tag,
          fiberId: this.fiberId.toJSON()
        };
      case "Fail":
        return {
          _id: "Cause",
          _tag: this._tag,
          failure: toJSON(this.error)
        };
      case "Sequential":
      case "Parallel":
        return {
          _id: "Cause",
          _tag: this._tag,
          left: toJSON(this.left),
          right: toJSON(this.right)
        };
    }
  },
  toString() {
    return pretty(this);
  },
  [NodeInspectSymbol]() {
    return this.toJSON();
  }
};
var empty16 = /* @__PURE__ */ (() => {
  const o = /* @__PURE__ */ Object.create(proto);
  o._tag = OP_EMPTY;
  return o;
})();
var fail = /* @__PURE__ */ __name((error) => {
  const o = Object.create(proto);
  o._tag = OP_FAIL;
  o.error = error;
  return o;
}, "fail");
var die = /* @__PURE__ */ __name((defect) => {
  const o = Object.create(proto);
  o._tag = OP_DIE;
  o.defect = defect;
  return o;
}, "die");
var interrupt = /* @__PURE__ */ __name((fiberId3) => {
  const o = Object.create(proto);
  o._tag = OP_INTERRUPT;
  o.fiberId = fiberId3;
  return o;
}, "interrupt");
var parallel = /* @__PURE__ */ __name((left3, right3) => {
  const o = Object.create(proto);
  o._tag = OP_PARALLEL;
  o.left = left3;
  o.right = right3;
  return o;
}, "parallel");
var sequential = /* @__PURE__ */ __name((left3, right3) => {
  const o = Object.create(proto);
  o._tag = OP_SEQUENTIAL;
  o.left = left3;
  o.right = right3;
  return o;
}, "sequential");
var isCause = /* @__PURE__ */ __name((u) => hasProperty(u, CauseTypeId), "isCause");
var isEmptyType = /* @__PURE__ */ __name((self) => self._tag === OP_EMPTY, "isEmptyType");
var isFailType = /* @__PURE__ */ __name((self) => self._tag === OP_FAIL, "isFailType");
var isDieType = /* @__PURE__ */ __name((self) => self._tag === OP_DIE, "isDieType");
var isInterruptType = /* @__PURE__ */ __name((self) => self._tag === OP_INTERRUPT, "isInterruptType");
var isSequentialType = /* @__PURE__ */ __name((self) => self._tag === OP_SEQUENTIAL, "isSequentialType");
var isParallelType = /* @__PURE__ */ __name((self) => self._tag === OP_PARALLEL, "isParallelType");
var size4 = /* @__PURE__ */ __name((self) => reduceWithContext(self, void 0, SizeCauseReducer), "size");
var isEmpty5 = /* @__PURE__ */ __name((self) => {
  if (self._tag === OP_EMPTY) {
    return true;
  }
  return reduce7(self, true, (acc, cause3) => {
    switch (cause3._tag) {
      case OP_EMPTY: {
        return some2(acc);
      }
      case OP_DIE:
      case OP_FAIL:
      case OP_INTERRUPT: {
        return some2(false);
      }
      default: {
        return none2();
      }
    }
  });
}, "isEmpty");
var isFailure = /* @__PURE__ */ __name((self) => isSome2(failureOption(self)), "isFailure");
var isDie = /* @__PURE__ */ __name((self) => isSome2(dieOption(self)), "isDie");
var isInterrupted = /* @__PURE__ */ __name((self) => isSome2(interruptOption(self)), "isInterrupted");
var isInterruptedOnly = /* @__PURE__ */ __name((self) => reduceWithContext(void 0, IsInterruptedOnlyCauseReducer)(self), "isInterruptedOnly");
var failures = /* @__PURE__ */ __name((self) => reverse2(reduce7(self, empty4(), (list, cause3) => cause3._tag === OP_FAIL ? some2(pipe(list, prepend2(cause3.error))) : none2())), "failures");
var defects = /* @__PURE__ */ __name((self) => reverse2(reduce7(self, empty4(), (list, cause3) => cause3._tag === OP_DIE ? some2(pipe(list, prepend2(cause3.defect))) : none2())), "defects");
var interruptors = /* @__PURE__ */ __name((self) => reduce7(self, empty7(), (set6, cause3) => cause3._tag === OP_INTERRUPT ? some2(pipe(set6, add4(cause3.fiberId))) : none2()), "interruptors");
var failureOption = /* @__PURE__ */ __name((self) => find(self, (cause3) => cause3._tag === OP_FAIL ? some2(cause3.error) : none2()), "failureOption");
var failureOrCause = /* @__PURE__ */ __name((self) => {
  const option3 = failureOption(self);
  switch (option3._tag) {
    case "None": {
      return right2(self);
    }
    case "Some": {
      return left2(option3.value);
    }
  }
}, "failureOrCause");
var dieOption = /* @__PURE__ */ __name((self) => find(self, (cause3) => cause3._tag === OP_DIE ? some2(cause3.defect) : none2()), "dieOption");
var flipCauseOption = /* @__PURE__ */ __name((self) => match4(self, {
  onEmpty: some2(empty16),
  onFail: map(fail),
  onDie: /* @__PURE__ */ __name((defect) => some2(die(defect)), "onDie"),
  onInterrupt: /* @__PURE__ */ __name((fiberId3) => some2(interrupt(fiberId3)), "onInterrupt"),
  onSequential: mergeWith(sequential),
  onParallel: mergeWith(parallel)
}), "flipCauseOption");
var interruptOption = /* @__PURE__ */ __name((self) => find(self, (cause3) => cause3._tag === OP_INTERRUPT ? some2(cause3.fiberId) : none2()), "interruptOption");
var keepDefects = /* @__PURE__ */ __name((self) => match4(self, {
  onEmpty: none2(),
  onFail: /* @__PURE__ */ __name(() => none2(), "onFail"),
  onDie: /* @__PURE__ */ __name((defect) => some2(die(defect)), "onDie"),
  onInterrupt: /* @__PURE__ */ __name(() => none2(), "onInterrupt"),
  onSequential: mergeWith(sequential),
  onParallel: mergeWith(parallel)
}), "keepDefects");
var keepDefectsAndElectFailures = /* @__PURE__ */ __name((self) => match4(self, {
  onEmpty: none2(),
  onFail: /* @__PURE__ */ __name((failure) => some2(die(failure)), "onFail"),
  onDie: /* @__PURE__ */ __name((defect) => some2(die(defect)), "onDie"),
  onInterrupt: /* @__PURE__ */ __name(() => none2(), "onInterrupt"),
  onSequential: mergeWith(sequential),
  onParallel: mergeWith(parallel)
}), "keepDefectsAndElectFailures");
var linearize = /* @__PURE__ */ __name((self) => match4(self, {
  onEmpty: empty7(),
  onFail: /* @__PURE__ */ __name((error) => make10(fail(error)), "onFail"),
  onDie: /* @__PURE__ */ __name((defect) => make10(die(defect)), "onDie"),
  onInterrupt: /* @__PURE__ */ __name((fiberId3) => make10(interrupt(fiberId3)), "onInterrupt"),
  onSequential: /* @__PURE__ */ __name((leftSet, rightSet) => flatMap4(leftSet, (leftCause) => map5(rightSet, (rightCause) => sequential(leftCause, rightCause))), "onSequential"),
  onParallel: /* @__PURE__ */ __name((leftSet, rightSet) => flatMap4(leftSet, (leftCause) => map5(rightSet, (rightCause) => parallel(leftCause, rightCause))), "onParallel")
}), "linearize");
var stripFailures = /* @__PURE__ */ __name((self) => match4(self, {
  onEmpty: empty16,
  onFail: /* @__PURE__ */ __name(() => empty16, "onFail"),
  onDie: die,
  onInterrupt: interrupt,
  onSequential: sequential,
  onParallel: parallel
}), "stripFailures");
var electFailures = /* @__PURE__ */ __name((self) => match4(self, {
  onEmpty: empty16,
  onFail: die,
  onDie: die,
  onInterrupt: interrupt,
  onSequential: sequential,
  onParallel: parallel
}), "electFailures");
var stripSomeDefects = /* @__PURE__ */ dual(2, (self, pf) => match4(self, {
  onEmpty: some2(empty16),
  onFail: /* @__PURE__ */ __name((error) => some2(fail(error)), "onFail"),
  onDie: /* @__PURE__ */ __name((defect) => {
    const option3 = pf(defect);
    return isSome2(option3) ? none2() : some2(die(defect));
  }, "onDie"),
  onInterrupt: /* @__PURE__ */ __name((fiberId3) => some2(interrupt(fiberId3)), "onInterrupt"),
  onSequential: mergeWith(sequential),
  onParallel: mergeWith(parallel)
}));
var as = /* @__PURE__ */ dual(2, (self, error) => map7(self, () => error));
var map7 = /* @__PURE__ */ dual(2, (self, f) => flatMap6(self, (e) => fail(f(e))));
var flatMap6 = /* @__PURE__ */ dual(2, (self, f) => match4(self, {
  onEmpty: empty16,
  onFail: /* @__PURE__ */ __name((error) => f(error), "onFail"),
  onDie: /* @__PURE__ */ __name((defect) => die(defect), "onDie"),
  onInterrupt: /* @__PURE__ */ __name((fiberId3) => interrupt(fiberId3), "onInterrupt"),
  onSequential: /* @__PURE__ */ __name((left3, right3) => sequential(left3, right3), "onSequential"),
  onParallel: /* @__PURE__ */ __name((left3, right3) => parallel(left3, right3), "onParallel")
}));
var flatten3 = /* @__PURE__ */ __name((self) => flatMap6(self, identity), "flatten");
var andThen2 = /* @__PURE__ */ dual(2, (self, f) => isFunction2(f) ? flatMap6(self, f) : flatMap6(self, () => f));
var contains3 = /* @__PURE__ */ dual(2, (self, that) => {
  if (that._tag === OP_EMPTY || self === that) {
    return true;
  }
  return reduce7(self, false, (accumulator, cause3) => {
    return some2(accumulator || causeEquals(cause3, that));
  });
});
var causeEquals = /* @__PURE__ */ __name((left3, right3) => {
  let leftStack = of2(left3);
  let rightStack = of2(right3);
  while (isNonEmpty(leftStack) && isNonEmpty(rightStack)) {
    const [leftParallel, leftSequential] = pipe(headNonEmpty2(leftStack), reduce7([empty7(), empty4()], ([parallel5, sequential5], cause3) => {
      const [par2, seq2] = evaluateCause(cause3);
      return some2([pipe(parallel5, union3(par2)), pipe(sequential5, appendAll2(seq2))]);
    }));
    const [rightParallel, rightSequential] = pipe(headNonEmpty2(rightStack), reduce7([empty7(), empty4()], ([parallel5, sequential5], cause3) => {
      const [par2, seq2] = evaluateCause(cause3);
      return some2([pipe(parallel5, union3(par2)), pipe(sequential5, appendAll2(seq2))]);
    }));
    if (!equals(leftParallel, rightParallel)) {
      return false;
    }
    leftStack = leftSequential;
    rightStack = rightSequential;
  }
  return true;
}, "causeEquals");
var flattenCause = /* @__PURE__ */ __name((cause3) => {
  return flattenCauseLoop(of2(cause3), empty4());
}, "flattenCause");
var flattenCauseLoop = /* @__PURE__ */ __name((causes, flattened) => {
  while (1) {
    const [parallel5, sequential5] = pipe(causes, reduce([empty7(), empty4()], ([parallel6, sequential6], cause3) => {
      const [par2, seq2] = evaluateCause(cause3);
      return [pipe(parallel6, union3(par2)), pipe(sequential6, appendAll2(seq2))];
    }));
    const updated = size3(parallel5) > 0 ? pipe(flattened, prepend2(parallel5)) : flattened;
    if (isEmpty(sequential5)) {
      return reverse2(updated);
    }
    causes = sequential5;
    flattened = updated;
  }
  throw new Error(getBugErrorMessage("Cause.flattenCauseLoop"));
}, "flattenCauseLoop");
var find = /* @__PURE__ */ dual(2, (self, pf) => {
  const stack = [self];
  while (stack.length > 0) {
    const item = stack.pop();
    const option3 = pf(item);
    switch (option3._tag) {
      case "None": {
        switch (item._tag) {
          case OP_SEQUENTIAL:
          case OP_PARALLEL: {
            stack.push(item.right);
            stack.push(item.left);
            break;
          }
        }
        break;
      }
      case "Some": {
        return option3;
      }
    }
  }
  return none2();
});
var filter4 = /* @__PURE__ */ dual(2, (self, predicate) => reduceWithContext(self, void 0, FilterCauseReducer(predicate)));
var evaluateCause = /* @__PURE__ */ __name((self) => {
  let cause3 = self;
  const stack = [];
  let _parallel = empty7();
  let _sequential = empty4();
  while (cause3 !== void 0) {
    switch (cause3._tag) {
      case OP_EMPTY: {
        if (stack.length === 0) {
          return [_parallel, _sequential];
        }
        cause3 = stack.pop();
        break;
      }
      case OP_FAIL: {
        _parallel = add4(_parallel, make6(cause3._tag, cause3.error));
        if (stack.length === 0) {
          return [_parallel, _sequential];
        }
        cause3 = stack.pop();
        break;
      }
      case OP_DIE: {
        _parallel = add4(_parallel, make6(cause3._tag, cause3.defect));
        if (stack.length === 0) {
          return [_parallel, _sequential];
        }
        cause3 = stack.pop();
        break;
      }
      case OP_INTERRUPT: {
        _parallel = add4(_parallel, make6(cause3._tag, cause3.fiberId));
        if (stack.length === 0) {
          return [_parallel, _sequential];
        }
        cause3 = stack.pop();
        break;
      }
      case OP_SEQUENTIAL: {
        switch (cause3.left._tag) {
          case OP_EMPTY: {
            cause3 = cause3.right;
            break;
          }
          case OP_SEQUENTIAL: {
            cause3 = sequential(cause3.left.left, sequential(cause3.left.right, cause3.right));
            break;
          }
          case OP_PARALLEL: {
            cause3 = parallel(sequential(cause3.left.left, cause3.right), sequential(cause3.left.right, cause3.right));
            break;
          }
          default: {
            _sequential = prepend2(_sequential, cause3.right);
            cause3 = cause3.left;
            break;
          }
        }
        break;
      }
      case OP_PARALLEL: {
        stack.push(cause3.right);
        cause3 = cause3.left;
        break;
      }
    }
  }
  throw new Error(getBugErrorMessage("Cause.evaluateCauseLoop"));
}, "evaluateCause");
var SizeCauseReducer = {
  emptyCase: /* @__PURE__ */ __name(() => 0, "emptyCase"),
  failCase: /* @__PURE__ */ __name(() => 1, "failCase"),
  dieCase: /* @__PURE__ */ __name(() => 1, "dieCase"),
  interruptCase: /* @__PURE__ */ __name(() => 1, "interruptCase"),
  sequentialCase: /* @__PURE__ */ __name((_, left3, right3) => left3 + right3, "sequentialCase"),
  parallelCase: /* @__PURE__ */ __name((_, left3, right3) => left3 + right3, "parallelCase")
};
var IsInterruptedOnlyCauseReducer = {
  emptyCase: constTrue,
  failCase: constFalse,
  dieCase: constFalse,
  interruptCase: constTrue,
  sequentialCase: /* @__PURE__ */ __name((_, left3, right3) => left3 && right3, "sequentialCase"),
  parallelCase: /* @__PURE__ */ __name((_, left3, right3) => left3 && right3, "parallelCase")
};
var FilterCauseReducer = /* @__PURE__ */ __name((predicate) => ({
  emptyCase: /* @__PURE__ */ __name(() => empty16, "emptyCase"),
  failCase: /* @__PURE__ */ __name((_, error) => fail(error), "failCase"),
  dieCase: /* @__PURE__ */ __name((_, defect) => die(defect), "dieCase"),
  interruptCase: /* @__PURE__ */ __name((_, fiberId3) => interrupt(fiberId3), "interruptCase"),
  sequentialCase: /* @__PURE__ */ __name((_, left3, right3) => {
    if (predicate(left3)) {
      if (predicate(right3)) {
        return sequential(left3, right3);
      }
      return left3;
    }
    if (predicate(right3)) {
      return right3;
    }
    return empty16;
  }, "sequentialCase"),
  parallelCase: /* @__PURE__ */ __name((_, left3, right3) => {
    if (predicate(left3)) {
      if (predicate(right3)) {
        return parallel(left3, right3);
      }
      return left3;
    }
    if (predicate(right3)) {
      return right3;
    }
    return empty16;
  }, "parallelCase")
}), "FilterCauseReducer");
var OP_SEQUENTIAL_CASE = "SequentialCase";
var OP_PARALLEL_CASE = "ParallelCase";
var match4 = /* @__PURE__ */ dual(2, (self, {
  onDie,
  onEmpty,
  onFail,
  onInterrupt: onInterrupt3,
  onParallel,
  onSequential
}) => {
  return reduceWithContext(self, void 0, {
    emptyCase: /* @__PURE__ */ __name(() => onEmpty, "emptyCase"),
    failCase: /* @__PURE__ */ __name((_, error) => onFail(error), "failCase"),
    dieCase: /* @__PURE__ */ __name((_, defect) => onDie(defect), "dieCase"),
    interruptCase: /* @__PURE__ */ __name((_, fiberId3) => onInterrupt3(fiberId3), "interruptCase"),
    sequentialCase: /* @__PURE__ */ __name((_, left3, right3) => onSequential(left3, right3), "sequentialCase"),
    parallelCase: /* @__PURE__ */ __name((_, left3, right3) => onParallel(left3, right3), "parallelCase")
  });
});
var reduce7 = /* @__PURE__ */ dual(3, (self, zero2, pf) => {
  let accumulator = zero2;
  let cause3 = self;
  const causes = [];
  while (cause3 !== void 0) {
    const option3 = pf(accumulator, cause3);
    accumulator = isSome2(option3) ? option3.value : accumulator;
    switch (cause3._tag) {
      case OP_SEQUENTIAL: {
        causes.push(cause3.right);
        cause3 = cause3.left;
        break;
      }
      case OP_PARALLEL: {
        causes.push(cause3.right);
        cause3 = cause3.left;
        break;
      }
      default: {
        cause3 = void 0;
        break;
      }
    }
    if (cause3 === void 0 && causes.length > 0) {
      cause3 = causes.pop();
    }
  }
  return accumulator;
});
var reduceWithContext = /* @__PURE__ */ dual(3, (self, context4, reducer) => {
  const input = [self];
  const output = [];
  while (input.length > 0) {
    const cause3 = input.pop();
    switch (cause3._tag) {
      case OP_EMPTY: {
        output.push(right2(reducer.emptyCase(context4)));
        break;
      }
      case OP_FAIL: {
        output.push(right2(reducer.failCase(context4, cause3.error)));
        break;
      }
      case OP_DIE: {
        output.push(right2(reducer.dieCase(context4, cause3.defect)));
        break;
      }
      case OP_INTERRUPT: {
        output.push(right2(reducer.interruptCase(context4, cause3.fiberId)));
        break;
      }
      case OP_SEQUENTIAL: {
        input.push(cause3.right);
        input.push(cause3.left);
        output.push(left2({
          _tag: OP_SEQUENTIAL_CASE
        }));
        break;
      }
      case OP_PARALLEL: {
        input.push(cause3.right);
        input.push(cause3.left);
        output.push(left2({
          _tag: OP_PARALLEL_CASE
        }));
        break;
      }
    }
  }
  const accumulator = [];
  while (output.length > 0) {
    const either4 = output.pop();
    switch (either4._tag) {
      case "Left": {
        switch (either4.left._tag) {
          case OP_SEQUENTIAL_CASE: {
            const left3 = accumulator.pop();
            const right3 = accumulator.pop();
            const value = reducer.sequentialCase(context4, left3, right3);
            accumulator.push(value);
            break;
          }
          case OP_PARALLEL_CASE: {
            const left3 = accumulator.pop();
            const right3 = accumulator.pop();
            const value = reducer.parallelCase(context4, left3, right3);
            accumulator.push(value);
            break;
          }
        }
        break;
      }
      case "Right": {
        accumulator.push(either4.right);
        break;
      }
    }
  }
  if (accumulator.length === 0) {
    throw new Error("BUG: Cause.reduceWithContext - please report an issue at https://github.com/Effect-TS/effect/issues");
  }
  return accumulator.pop();
});
var pretty = /* @__PURE__ */ __name((cause3, options) => {
  if (isInterruptedOnly(cause3)) {
    return "All fibers interrupted without errors.";
  }
  return prettyErrors(cause3).map(function(e) {
    if (options?.renderErrorCause !== true || e.cause === void 0) {
      return e.stack;
    }
    return `${e.stack} {
${renderErrorCause(e.cause, "  ")}
}`;
  }).join("\n");
}, "pretty");
var renderErrorCause = /* @__PURE__ */ __name((cause3, prefix) => {
  const lines = cause3.stack.split("\n");
  let stack = `${prefix}[cause]: ${lines[0]}`;
  for (let i = 1, len = lines.length; i < len; i++) {
    stack += `
${prefix}${lines[i]}`;
  }
  if (cause3.cause) {
    stack += ` {
${renderErrorCause(cause3.cause, `${prefix}  `)}
${prefix}}`;
  }
  return stack;
}, "renderErrorCause");
var makePrettyError = /* @__PURE__ */ __name((originalError2) => {
  const originalErrorIsObject = typeof originalError2 === "object" && originalError2 !== null;
  const prevLimit = Error.stackTraceLimit;
  Error.stackTraceLimit = 1;
  const error = new Error(prettyErrorMessage(originalError2), originalErrorIsObject && "cause" in originalError2 && typeof originalError2.cause !== "undefined" ? {
    cause: makePrettyError(originalError2.cause)
  } : void 0);
  Error.stackTraceLimit = prevLimit;
  if (error.message === "") {
    error.message = "An error has occurred";
  }
  Error.stackTraceLimit = prevLimit;
  error.name = originalError2 instanceof Error ? originalError2.name : "Error";
  if (originalErrorIsObject) {
    if (spanSymbol in originalError2) {
      error.span = originalError2[spanSymbol];
    }
    Object.keys(originalError2).forEach((key) => {
      if (!(key in error)) {
        error[key] = originalError2[key];
      }
    });
  }
  error.stack = prettyErrorStack(`${error.name}: ${error.message}`, originalError2 instanceof Error && originalError2.stack ? originalError2.stack : "", error.span);
  return error;
}, "makePrettyError");
var prettyErrorMessage = /* @__PURE__ */ __name((u) => {
  if (typeof u === "string") {
    return u;
  }
  if (typeof u === "object" && u !== null && u instanceof Error) {
    return u.message;
  }
  try {
    if (hasProperty(u, "toString") && isFunction2(u["toString"]) && u["toString"] !== Object.prototype.toString && u["toString"] !== globalThis.Array.prototype.toString) {
      return u["toString"]();
    }
  } catch {
  }
  return stringifyCircular(u);
}, "prettyErrorMessage");
var locationRegex = /\((.*)\)/g;
var spanToTrace = /* @__PURE__ */ globalValue("effect/Tracer/spanToTrace", () => /* @__PURE__ */ new WeakMap());
var prettyErrorStack = /* @__PURE__ */ __name((message, stack, span2) => {
  const out = [message];
  const lines = stack.startsWith(message) ? stack.slice(message.length).split("\n") : stack.split("\n");
  for (let i = 1; i < lines.length; i++) {
    if (lines[i].includes(" at new BaseEffectError") || lines[i].includes(" at new YieldableError")) {
      i++;
      continue;
    }
    if (lines[i].includes("Generator.next")) {
      break;
    }
    if (lines[i].includes("effect_internal_function")) {
      break;
    }
    out.push(lines[i].replace(/at .*effect_instruction_i.*\((.*)\)/, "at $1").replace(/EffectPrimitive\.\w+/, "<anonymous>"));
  }
  if (span2) {
    let current = span2;
    let i = 0;
    while (current && current._tag === "Span" && i < 10) {
      const stackFn = spanToTrace.get(current);
      if (typeof stackFn === "function") {
        const stack2 = stackFn();
        if (typeof stack2 === "string") {
          const locationMatchAll = stack2.matchAll(locationRegex);
          let match12 = false;
          for (const [, location] of locationMatchAll) {
            match12 = true;
            out.push(`    at ${current.name} (${location})`);
          }
          if (!match12) {
            out.push(`    at ${current.name} (${stack2.replace(/^at /, "")})`);
          }
        } else {
          out.push(`    at ${current.name}`);
        }
      } else {
        out.push(`    at ${current.name}`);
      }
      current = getOrUndefined(current.parent);
      i++;
    }
  }
  return out.join("\n");
}, "prettyErrorStack");
var spanSymbol = /* @__PURE__ */ Symbol.for("effect/SpanAnnotation");
var prettyErrors = /* @__PURE__ */ __name((cause3) => reduceWithContext(cause3, void 0, {
  emptyCase: /* @__PURE__ */ __name(() => [], "emptyCase"),
  dieCase: /* @__PURE__ */ __name((_, unknownError) => {
    return [makePrettyError(unknownError)];
  }, "dieCase"),
  failCase: /* @__PURE__ */ __name((_, error) => {
    return [makePrettyError(error)];
  }, "failCase"),
  interruptCase: /* @__PURE__ */ __name(() => [], "interruptCase"),
  parallelCase: /* @__PURE__ */ __name((_, l, r) => [...l, ...r], "parallelCase"),
  sequentialCase: /* @__PURE__ */ __name((_, l, r) => [...l, ...r], "sequentialCase")
}), "prettyErrors");

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/internal/opCodes/deferred.js
var OP_STATE_PENDING = "Pending";
var OP_STATE_DONE = "Done";

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/internal/deferred.js
var DeferredSymbolKey = "effect/Deferred";
var DeferredTypeId = /* @__PURE__ */ Symbol.for(DeferredSymbolKey);
var deferredVariance = {
  /* c8 ignore next */
  _E: /* @__PURE__ */ __name((_) => _, "_E"),
  /* c8 ignore next */
  _A: /* @__PURE__ */ __name((_) => _, "_A")
};
var pending = /* @__PURE__ */ __name((joiners) => {
  return {
    _tag: OP_STATE_PENDING,
    joiners
  };
}, "pending");
var done = /* @__PURE__ */ __name((effect) => {
  return {
    _tag: OP_STATE_DONE,
    effect
  };
}, "done");

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/internal/singleShotGen.js
var SingleShotGen2 = class _SingleShotGen {
  static {
    __name(this, "SingleShotGen");
  }
  self;
  called = false;
  constructor(self) {
    this.self = self;
  }
  next(a) {
    return this.called ? {
      value: a,
      done: true
    } : (this.called = true, {
      value: this.self,
      done: false
    });
  }
  return(a) {
    return {
      value: a,
      done: true
    };
  }
  throw(e) {
    throw e;
  }
  [Symbol.iterator]() {
    return new _SingleShotGen(this.self);
  }
};

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/internal/core.js
var blocked = /* @__PURE__ */ __name((blockedRequests, _continue3) => {
  const effect = new EffectPrimitive("Blocked");
  effect.effect_instruction_i0 = blockedRequests;
  effect.effect_instruction_i1 = _continue3;
  return effect;
}, "blocked");
var runRequestBlock = /* @__PURE__ */ __name((blockedRequests) => {
  const effect = new EffectPrimitive("RunBlocked");
  effect.effect_instruction_i0 = blockedRequests;
  return effect;
}, "runRequestBlock");
var EffectTypeId2 = /* @__PURE__ */ Symbol.for("effect/Effect");
var RevertFlags = class {
  static {
    __name(this, "RevertFlags");
  }
  patch;
  op;
  _op = OP_REVERT_FLAGS;
  constructor(patch9, op) {
    this.patch = patch9;
    this.op = op;
  }
};
var EffectPrimitive = class {
  static {
    __name(this, "EffectPrimitive");
  }
  _op;
  effect_instruction_i0 = void 0;
  effect_instruction_i1 = void 0;
  effect_instruction_i2 = void 0;
  trace = void 0;
  [EffectTypeId2] = effectVariance;
  constructor(_op) {
    this._op = _op;
  }
  [symbol2](that) {
    return this === that;
  }
  [symbol]() {
    return cached(this, random(this));
  }
  pipe() {
    return pipeArguments(this, arguments);
  }
  toJSON() {
    return {
      _id: "Effect",
      _op: this._op,
      effect_instruction_i0: toJSON(this.effect_instruction_i0),
      effect_instruction_i1: toJSON(this.effect_instruction_i1),
      effect_instruction_i2: toJSON(this.effect_instruction_i2)
    };
  }
  toString() {
    return format(this.toJSON());
  }
  [NodeInspectSymbol]() {
    return this.toJSON();
  }
  [Symbol.iterator]() {
    return new SingleShotGen2(new YieldWrap(this));
  }
};
var EffectPrimitiveFailure = class {
  static {
    __name(this, "EffectPrimitiveFailure");
  }
  _op;
  effect_instruction_i0 = void 0;
  effect_instruction_i1 = void 0;
  effect_instruction_i2 = void 0;
  trace = void 0;
  [EffectTypeId2] = effectVariance;
  constructor(_op) {
    this._op = _op;
    this._tag = _op;
  }
  [symbol2](that) {
    return exitIsExit(that) && that._op === "Failure" && // @ts-expect-error
    equals(this.effect_instruction_i0, that.effect_instruction_i0);
  }
  [symbol]() {
    return pipe(
      // @ts-expect-error
      string(this._tag),
      // @ts-expect-error
      combine(hash(this.effect_instruction_i0)),
      cached(this)
    );
  }
  get cause() {
    return this.effect_instruction_i0;
  }
  pipe() {
    return pipeArguments(this, arguments);
  }
  toJSON() {
    return {
      _id: "Exit",
      _tag: this._op,
      cause: this.cause.toJSON()
    };
  }
  toString() {
    return format(this.toJSON());
  }
  [NodeInspectSymbol]() {
    return this.toJSON();
  }
  [Symbol.iterator]() {
    return new SingleShotGen2(new YieldWrap(this));
  }
};
var EffectPrimitiveSuccess = class {
  static {
    __name(this, "EffectPrimitiveSuccess");
  }
  _op;
  effect_instruction_i0 = void 0;
  effect_instruction_i1 = void 0;
  effect_instruction_i2 = void 0;
  trace = void 0;
  [EffectTypeId2] = effectVariance;
  constructor(_op) {
    this._op = _op;
    this._tag = _op;
  }
  [symbol2](that) {
    return exitIsExit(that) && that._op === "Success" && // @ts-expect-error
    equals(this.effect_instruction_i0, that.effect_instruction_i0);
  }
  [symbol]() {
    return pipe(
      // @ts-expect-error
      string(this._tag),
      // @ts-expect-error
      combine(hash(this.effect_instruction_i0)),
      cached(this)
    );
  }
  get value() {
    return this.effect_instruction_i0;
  }
  pipe() {
    return pipeArguments(this, arguments);
  }
  toJSON() {
    return {
      _id: "Exit",
      _tag: this._op,
      value: toJSON(this.value)
    };
  }
  toString() {
    return format(this.toJSON());
  }
  [NodeInspectSymbol]() {
    return this.toJSON();
  }
  [Symbol.iterator]() {
    return new SingleShotGen2(new YieldWrap(this));
  }
};
var isEffect = /* @__PURE__ */ __name((u) => hasProperty(u, EffectTypeId2), "isEffect");
var withFiberRuntime = /* @__PURE__ */ __name((withRuntime) => {
  const effect = new EffectPrimitive(OP_WITH_RUNTIME);
  effect.effect_instruction_i0 = withRuntime;
  return effect;
}, "withFiberRuntime");
var acquireUseRelease = /* @__PURE__ */ dual(3, (acquire, use, release) => uninterruptibleMask((restore) => flatMap7(acquire, (a) => flatMap7(exit(suspend(() => restore(use(a)))), (exit4) => {
  return suspend(() => release(a, exit4)).pipe(matchCauseEffect({
    onFailure: /* @__PURE__ */ __name((cause3) => {
      switch (exit4._tag) {
        case OP_FAILURE:
          return failCause(sequential(exit4.effect_instruction_i0, cause3));
        case OP_SUCCESS:
          return failCause(cause3);
      }
    }, "onFailure"),
    onSuccess: /* @__PURE__ */ __name(() => exit4, "onSuccess")
  }));
}))));
var as2 = /* @__PURE__ */ dual(2, (self, value) => flatMap7(self, () => succeed(value)));
var asVoid = /* @__PURE__ */ __name((self) => as2(self, void 0), "asVoid");
var custom = /* @__PURE__ */ __name(function() {
  const wrapper = new EffectPrimitive(OP_COMMIT);
  switch (arguments.length) {
    case 2: {
      wrapper.effect_instruction_i0 = arguments[0];
      wrapper.commit = arguments[1];
      break;
    }
    case 3: {
      wrapper.effect_instruction_i0 = arguments[0];
      wrapper.effect_instruction_i1 = arguments[1];
      wrapper.commit = arguments[2];
      break;
    }
    case 4: {
      wrapper.effect_instruction_i0 = arguments[0];
      wrapper.effect_instruction_i1 = arguments[1];
      wrapper.effect_instruction_i2 = arguments[2];
      wrapper.commit = arguments[3];
      break;
    }
    default: {
      throw new Error(getBugErrorMessage("you're not supposed to end up here"));
    }
  }
  return wrapper;
}, "custom");
var unsafeAsync = /* @__PURE__ */ __name((register, blockingOn = none4) => {
  const effect = new EffectPrimitive(OP_ASYNC);
  let cancelerRef = void 0;
  effect.effect_instruction_i0 = (resume2) => {
    cancelerRef = register(resume2);
  };
  effect.effect_instruction_i1 = blockingOn;
  return onInterrupt(effect, (_) => isEffect(cancelerRef) ? cancelerRef : void_);
}, "unsafeAsync");
var asyncInterrupt = /* @__PURE__ */ __name((register, blockingOn = none4) => suspend(() => unsafeAsync(register, blockingOn)), "asyncInterrupt");
var async_ = /* @__PURE__ */ __name((resume2, blockingOn = none4) => {
  return custom(resume2, function() {
    let backingResume = void 0;
    let pendingEffect = void 0;
    function proxyResume(effect2) {
      if (backingResume) {
        backingResume(effect2);
      } else if (pendingEffect === void 0) {
        pendingEffect = effect2;
      }
    }
    __name(proxyResume, "proxyResume");
    const effect = new EffectPrimitive(OP_ASYNC);
    effect.effect_instruction_i0 = (resume3) => {
      backingResume = resume3;
      if (pendingEffect) {
        resume3(pendingEffect);
      }
    };
    effect.effect_instruction_i1 = blockingOn;
    let cancelerRef = void 0;
    let controllerRef = void 0;
    if (this.effect_instruction_i0.length !== 1) {
      controllerRef = new AbortController();
      cancelerRef = internalCall(() => this.effect_instruction_i0(proxyResume, controllerRef.signal));
    } else {
      cancelerRef = internalCall(() => this.effect_instruction_i0(proxyResume));
    }
    return cancelerRef || controllerRef ? onInterrupt(effect, (_) => {
      if (controllerRef) {
        controllerRef.abort();
      }
      return cancelerRef ?? void_;
    }) : effect;
  });
}, "async_");
var catchAllCause = /* @__PURE__ */ dual(2, (self, f) => {
  const effect = new EffectPrimitive(OP_ON_FAILURE);
  effect.effect_instruction_i0 = self;
  effect.effect_instruction_i1 = f;
  return effect;
});
var catchAll = /* @__PURE__ */ dual(2, (self, f) => matchEffect(self, {
  onFailure: f,
  onSuccess: succeed
}));
var catchIf = /* @__PURE__ */ dual(3, (self, predicate, f) => catchAllCause(self, (cause3) => {
  const either4 = failureOrCause(cause3);
  switch (either4._tag) {
    case "Left":
      return predicate(either4.left) ? f(either4.left) : failCause(cause3);
    case "Right":
      return failCause(either4.right);
  }
}));
var catchSome = /* @__PURE__ */ dual(2, (self, pf) => catchAllCause(self, (cause3) => {
  const either4 = failureOrCause(cause3);
  switch (either4._tag) {
    case "Left":
      return pipe(pf(either4.left), getOrElse(() => failCause(cause3)));
    case "Right":
      return failCause(either4.right);
  }
}));
var checkInterruptible = /* @__PURE__ */ __name((f) => withFiberRuntime((_, status) => f(interruption(status.runtimeFlags))), "checkInterruptible");
var originalSymbol = /* @__PURE__ */ Symbol.for("effect/OriginalAnnotation");
var originalInstance = /* @__PURE__ */ __name((obj) => {
  if (hasProperty(obj, originalSymbol)) {
    return obj[originalSymbol];
  }
  return obj;
}, "originalInstance");
var capture = /* @__PURE__ */ __name((obj, span2) => {
  if (isSome2(span2)) {
    return new Proxy(obj, {
      has(target, p) {
        return p === spanSymbol || p === originalSymbol || p in target;
      },
      get(target, p) {
        if (p === spanSymbol) {
          return span2.value;
        }
        if (p === originalSymbol) {
          return obj;
        }
        return target[p];
      }
    });
  }
  return obj;
}, "capture");
var die2 = /* @__PURE__ */ __name((defect) => isObject(defect) && !(spanSymbol in defect) ? withFiberRuntime((fiber) => failCause(die(capture(defect, currentSpanFromFiber(fiber))))) : failCause(die(defect)), "die");
var dieMessage = /* @__PURE__ */ __name((message) => failCauseSync(() => die(new RuntimeException(message))), "dieMessage");
var dieSync = /* @__PURE__ */ __name((evaluate2) => flatMap7(sync(evaluate2), die2), "dieSync");
var either2 = /* @__PURE__ */ __name((self) => matchEffect(self, {
  onFailure: /* @__PURE__ */ __name((e) => succeed(left2(e)), "onFailure"),
  onSuccess: /* @__PURE__ */ __name((a) => succeed(right2(a)), "onSuccess")
}), "either");
var exit = /* @__PURE__ */ __name((self) => matchCause(self, {
  onFailure: exitFailCause,
  onSuccess: exitSucceed
}), "exit");
var fail2 = /* @__PURE__ */ __name((error) => isObject(error) && !(spanSymbol in error) ? withFiberRuntime((fiber) => failCause(fail(capture(error, currentSpanFromFiber(fiber))))) : failCause(fail(error)), "fail");
var failSync = /* @__PURE__ */ __name((evaluate2) => flatMap7(sync(evaluate2), fail2), "failSync");
var failCause = /* @__PURE__ */ __name((cause3) => {
  const effect = new EffectPrimitiveFailure(OP_FAILURE);
  effect.effect_instruction_i0 = cause3;
  return effect;
}, "failCause");
var failCauseSync = /* @__PURE__ */ __name((evaluate2) => flatMap7(sync(evaluate2), failCause), "failCauseSync");
var fiberId = /* @__PURE__ */ withFiberRuntime((state) => succeed(state.id()));
var fiberIdWith = /* @__PURE__ */ __name((f) => withFiberRuntime((state) => f(state.id())), "fiberIdWith");
var flatMap7 = /* @__PURE__ */ dual(2, (self, f) => {
  const effect = new EffectPrimitive(OP_ON_SUCCESS);
  effect.effect_instruction_i0 = self;
  effect.effect_instruction_i1 = f;
  return effect;
});
var andThen3 = /* @__PURE__ */ dual(2, (self, f) => flatMap7(self, (a) => {
  const b = typeof f === "function" ? f(a) : f;
  if (isEffect(b)) {
    return b;
  } else if (isPromiseLike(b)) {
    return unsafeAsync((resume2) => {
      b.then((a2) => resume2(succeed(a2)), (e) => resume2(fail2(new UnknownException(e, "An unknown error occurred in Effect.andThen"))));
    });
  }
  return succeed(b);
}));
var step2 = /* @__PURE__ */ __name((self) => {
  const effect = new EffectPrimitive("OnStep");
  effect.effect_instruction_i0 = self;
  return effect;
}, "step");
var flatten4 = /* @__PURE__ */ __name((self) => flatMap7(self, identity), "flatten");
var flip = /* @__PURE__ */ __name((self) => matchEffect(self, {
  onFailure: succeed,
  onSuccess: fail2
}), "flip");
var matchCause = /* @__PURE__ */ dual(2, (self, options) => matchCauseEffect(self, {
  onFailure: /* @__PURE__ */ __name((cause3) => succeed(options.onFailure(cause3)), "onFailure"),
  onSuccess: /* @__PURE__ */ __name((a) => succeed(options.onSuccess(a)), "onSuccess")
}));
var matchCauseEffect = /* @__PURE__ */ dual(2, (self, options) => {
  const effect = new EffectPrimitive(OP_ON_SUCCESS_AND_FAILURE);
  effect.effect_instruction_i0 = self;
  effect.effect_instruction_i1 = options.onFailure;
  effect.effect_instruction_i2 = options.onSuccess;
  return effect;
});
var matchEffect = /* @__PURE__ */ dual(2, (self, options) => matchCauseEffect(self, {
  onFailure: /* @__PURE__ */ __name((cause3) => {
    const defects3 = defects(cause3);
    if (defects3.length > 0) {
      return failCause(electFailures(cause3));
    }
    const failures3 = failures(cause3);
    if (failures3.length > 0) {
      return options.onFailure(unsafeHead(failures3));
    }
    return failCause(cause3);
  }, "onFailure"),
  onSuccess: options.onSuccess
}));
var forEachSequential = /* @__PURE__ */ dual(2, (self, f) => suspend(() => {
  const arr = fromIterable(self);
  const ret = allocate(arr.length);
  let i = 0;
  return as2(whileLoop({
    while: /* @__PURE__ */ __name(() => i < arr.length, "while"),
    body: /* @__PURE__ */ __name(() => f(arr[i], i), "body"),
    step: /* @__PURE__ */ __name((b) => {
      ret[i++] = b;
    }, "step")
  }), ret);
}));
var forEachSequentialDiscard = /* @__PURE__ */ dual(2, (self, f) => suspend(() => {
  const arr = fromIterable(self);
  let i = 0;
  return whileLoop({
    while: /* @__PURE__ */ __name(() => i < arr.length, "while"),
    body: /* @__PURE__ */ __name(() => f(arr[i], i), "body"),
    step: /* @__PURE__ */ __name(() => {
      i++;
    }, "step")
  });
}));
var if_ = /* @__PURE__ */ dual((args2) => typeof args2[0] === "boolean" || isEffect(args2[0]), (self, options) => isEffect(self) ? flatMap7(self, (b) => b ? options.onTrue() : options.onFalse()) : self ? options.onTrue() : options.onFalse());
var interrupt2 = /* @__PURE__ */ flatMap7(fiberId, (fiberId3) => interruptWith(fiberId3));
var interruptWith = /* @__PURE__ */ __name((fiberId3) => failCause(interrupt(fiberId3)), "interruptWith");
var interruptible2 = /* @__PURE__ */ __name((self) => {
  const effect = new EffectPrimitive(OP_UPDATE_RUNTIME_FLAGS);
  effect.effect_instruction_i0 = enable3(Interruption);
  effect.effect_instruction_i1 = () => self;
  return effect;
}, "interruptible");
var interruptibleMask = /* @__PURE__ */ __name((f) => custom(f, function() {
  const effect = new EffectPrimitive(OP_UPDATE_RUNTIME_FLAGS);
  effect.effect_instruction_i0 = enable3(Interruption);
  effect.effect_instruction_i1 = (oldFlags) => interruption(oldFlags) ? internalCall(() => this.effect_instruction_i0(interruptible2)) : internalCall(() => this.effect_instruction_i0(uninterruptible));
  return effect;
}), "interruptibleMask");
var intoDeferred = /* @__PURE__ */ dual(2, (self, deferred) => uninterruptibleMask((restore) => flatMap7(exit(restore(self)), (exit4) => deferredDone(deferred, exit4))));
var map8 = /* @__PURE__ */ dual(2, (self, f) => flatMap7(self, (a) => sync(() => f(a))));
var mapBoth = /* @__PURE__ */ dual(2, (self, options) => matchEffect(self, {
  onFailure: /* @__PURE__ */ __name((e) => failSync(() => options.onFailure(e)), "onFailure"),
  onSuccess: /* @__PURE__ */ __name((a) => sync(() => options.onSuccess(a)), "onSuccess")
}));
var mapError = /* @__PURE__ */ dual(2, (self, f) => matchCauseEffect(self, {
  onFailure: /* @__PURE__ */ __name((cause3) => {
    const either4 = failureOrCause(cause3);
    switch (either4._tag) {
      case "Left": {
        return failSync(() => f(either4.left));
      }
      case "Right": {
        return failCause(either4.right);
      }
    }
  }, "onFailure"),
  onSuccess: succeed
}));
var onError = /* @__PURE__ */ dual(2, (self, cleanup) => onExit(self, (exit4) => exitIsSuccess(exit4) ? void_ : cleanup(exit4.effect_instruction_i0)));
var onExit = /* @__PURE__ */ dual(2, (self, cleanup) => uninterruptibleMask((restore) => matchCauseEffect(restore(self), {
  onFailure: /* @__PURE__ */ __name((cause1) => {
    const result = exitFailCause(cause1);
    return matchCauseEffect(cleanup(result), {
      onFailure: /* @__PURE__ */ __name((cause22) => exitFailCause(sequential(cause1, cause22)), "onFailure"),
      onSuccess: /* @__PURE__ */ __name(() => result, "onSuccess")
    });
  }, "onFailure"),
  onSuccess: /* @__PURE__ */ __name((success) => {
    const result = exitSucceed(success);
    return zipRight(cleanup(result), result);
  }, "onSuccess")
})));
var onInterrupt = /* @__PURE__ */ dual(2, (self, cleanup) => onExit(self, exitMatch({
  onFailure: /* @__PURE__ */ __name((cause3) => isInterruptedOnly(cause3) ? asVoid(cleanup(interruptors(cause3))) : void_, "onFailure"),
  onSuccess: /* @__PURE__ */ __name(() => void_, "onSuccess")
})));
var orElse = /* @__PURE__ */ dual(2, (self, that) => attemptOrElse(self, that, succeed));
var orDie = /* @__PURE__ */ __name((self) => orDieWith(self, identity), "orDie");
var orDieWith = /* @__PURE__ */ dual(2, (self, f) => matchEffect(self, {
  onFailure: /* @__PURE__ */ __name((e) => die2(f(e)), "onFailure"),
  onSuccess: succeed
}));
var partitionMap2 = partitionMap;
var runtimeFlags = /* @__PURE__ */ withFiberRuntime((_, status) => succeed(status.runtimeFlags));
var succeed = /* @__PURE__ */ __name((value) => {
  const effect = new EffectPrimitiveSuccess(OP_SUCCESS);
  effect.effect_instruction_i0 = value;
  return effect;
}, "succeed");
var suspend = /* @__PURE__ */ __name((evaluate2) => {
  const effect = new EffectPrimitive(OP_COMMIT);
  effect.commit = evaluate2;
  return effect;
}, "suspend");
var sync = /* @__PURE__ */ __name((thunk) => {
  const effect = new EffectPrimitive(OP_SYNC);
  effect.effect_instruction_i0 = thunk;
  return effect;
}, "sync");
var tap = /* @__PURE__ */ dual((args2) => args2.length === 3 || args2.length === 2 && !(isObject(args2[1]) && "onlyEffect" in args2[1]), (self, f) => flatMap7(self, (a) => {
  const b = typeof f === "function" ? f(a) : f;
  if (isEffect(b)) {
    return as2(b, a);
  } else if (isPromiseLike(b)) {
    return unsafeAsync((resume2) => {
      b.then((_) => resume2(succeed(a)), (e) => resume2(fail2(new UnknownException(e, "An unknown error occurred in Effect.tap"))));
    });
  }
  return succeed(a);
}));
var transplant = /* @__PURE__ */ __name((f) => withFiberRuntime((state) => {
  const scopeOverride = state.getFiberRef(currentForkScopeOverride);
  const scope3 = pipe(scopeOverride, getOrElse(() => state.scope()));
  return f(fiberRefLocally(currentForkScopeOverride, some2(scope3)));
}), "transplant");
var attemptOrElse = /* @__PURE__ */ dual(3, (self, that, onSuccess) => matchCauseEffect(self, {
  onFailure: /* @__PURE__ */ __name((cause3) => {
    const defects3 = defects(cause3);
    if (defects3.length > 0) {
      return failCause(getOrThrow(keepDefectsAndElectFailures(cause3)));
    }
    return that();
  }, "onFailure"),
  onSuccess
}));
var uninterruptible = /* @__PURE__ */ __name((self) => {
  const effect = new EffectPrimitive(OP_UPDATE_RUNTIME_FLAGS);
  effect.effect_instruction_i0 = disable3(Interruption);
  effect.effect_instruction_i1 = () => self;
  return effect;
}, "uninterruptible");
var uninterruptibleMask = /* @__PURE__ */ __name((f) => custom(f, function() {
  const effect = new EffectPrimitive(OP_UPDATE_RUNTIME_FLAGS);
  effect.effect_instruction_i0 = disable3(Interruption);
  effect.effect_instruction_i1 = (oldFlags) => interruption(oldFlags) ? internalCall(() => this.effect_instruction_i0(interruptible2)) : internalCall(() => this.effect_instruction_i0(uninterruptible));
  return effect;
}), "uninterruptibleMask");
var void_ = /* @__PURE__ */ succeed(void 0);
var updateRuntimeFlags = /* @__PURE__ */ __name((patch9) => {
  const effect = new EffectPrimitive(OP_UPDATE_RUNTIME_FLAGS);
  effect.effect_instruction_i0 = patch9;
  effect.effect_instruction_i1 = void 0;
  return effect;
}, "updateRuntimeFlags");
var whenEffect = /* @__PURE__ */ dual(2, (self, condition) => flatMap7(condition, (b) => {
  if (b) {
    return pipe(self, map8(some2));
  }
  return succeed(none2());
}));
var whileLoop = /* @__PURE__ */ __name((options) => {
  const effect = new EffectPrimitive(OP_WHILE);
  effect.effect_instruction_i0 = options.while;
  effect.effect_instruction_i1 = options.body;
  effect.effect_instruction_i2 = options.step;
  return effect;
}, "whileLoop");
var fromIterator = /* @__PURE__ */ __name((iterator) => suspend(() => {
  const effect = new EffectPrimitive(OP_ITERATOR);
  effect.effect_instruction_i0 = iterator();
  return effect;
}), "fromIterator");
var gen = /* @__PURE__ */ __name(function() {
  const f = arguments.length === 1 ? arguments[0] : arguments[1].bind(arguments[0]);
  return fromIterator(() => f(pipe));
}, "gen");
var fnUntraced = /* @__PURE__ */ __name((body, ...pipeables) => Object.defineProperty(pipeables.length === 0 ? function(...args2) {
  return fromIterator(() => body.apply(this, args2));
} : function(...args2) {
  let effect = fromIterator(() => body.apply(this, args2));
  for (const x of pipeables) {
    effect = x(effect, ...args2);
  }
  return effect;
}, "length", {
  value: body.length,
  configurable: true
}), "fnUntraced");
var withConcurrency = /* @__PURE__ */ dual(2, (self, concurrency) => fiberRefLocally(self, currentConcurrency, concurrency));
var withRequestBatching = /* @__PURE__ */ dual(2, (self, requestBatching) => fiberRefLocally(self, currentRequestBatching, requestBatching));
var withRuntimeFlags = /* @__PURE__ */ dual(2, (self, update5) => {
  const effect = new EffectPrimitive(OP_UPDATE_RUNTIME_FLAGS);
  effect.effect_instruction_i0 = update5;
  effect.effect_instruction_i1 = () => self;
  return effect;
});
var withTracerEnabled = /* @__PURE__ */ dual(2, (effect, enabled2) => fiberRefLocally(effect, currentTracerEnabled, enabled2));
var withTracerTiming = /* @__PURE__ */ dual(2, (effect, enabled2) => fiberRefLocally(effect, currentTracerTimingEnabled, enabled2));
var yieldNow = /* @__PURE__ */ __name((options) => {
  const effect = new EffectPrimitive(OP_YIELD);
  return typeof options?.priority !== "undefined" ? withSchedulingPriority(effect, options.priority) : effect;
}, "yieldNow");
var zip2 = /* @__PURE__ */ dual(2, (self, that) => flatMap7(self, (a) => map8(that, (b) => [a, b])));
var zipLeft = /* @__PURE__ */ dual(2, (self, that) => flatMap7(self, (a) => as2(that, a)));
var zipRight = /* @__PURE__ */ dual(2, (self, that) => flatMap7(self, () => that));
var zipWith2 = /* @__PURE__ */ dual(3, (self, that, f) => flatMap7(self, (a) => map8(that, (b) => f(a, b))));
var never = /* @__PURE__ */ asyncInterrupt(() => {
  const interval = setInterval(() => {
  }, 2 ** 31 - 1);
  return sync(() => clearInterval(interval));
});
var interruptFiber = /* @__PURE__ */ __name((self) => flatMap7(fiberId, (fiberId3) => pipe(self, interruptAsFiber(fiberId3))), "interruptFiber");
var interruptAsFiber = /* @__PURE__ */ dual(2, (self, fiberId3) => flatMap7(self.interruptAsFork(fiberId3), () => self.await));
var logLevelAll = {
  _tag: "All",
  syslog: 0,
  label: "ALL",
  ordinal: Number.MIN_SAFE_INTEGER,
  pipe() {
    return pipeArguments(this, arguments);
  }
};
var logLevelFatal = {
  _tag: "Fatal",
  syslog: 2,
  label: "FATAL",
  ordinal: 5e4,
  pipe() {
    return pipeArguments(this, arguments);
  }
};
var logLevelError = {
  _tag: "Error",
  syslog: 3,
  label: "ERROR",
  ordinal: 4e4,
  pipe() {
    return pipeArguments(this, arguments);
  }
};
var logLevelWarning = {
  _tag: "Warning",
  syslog: 4,
  label: "WARN",
  ordinal: 3e4,
  pipe() {
    return pipeArguments(this, arguments);
  }
};
var logLevelInfo = {
  _tag: "Info",
  syslog: 6,
  label: "INFO",
  ordinal: 2e4,
  pipe() {
    return pipeArguments(this, arguments);
  }
};
var logLevelDebug = {
  _tag: "Debug",
  syslog: 7,
  label: "DEBUG",
  ordinal: 1e4,
  pipe() {
    return pipeArguments(this, arguments);
  }
};
var logLevelTrace = {
  _tag: "Trace",
  syslog: 7,
  label: "TRACE",
  ordinal: 0,
  pipe() {
    return pipeArguments(this, arguments);
  }
};
var logLevelNone = {
  _tag: "None",
  syslog: 7,
  label: "OFF",
  ordinal: Number.MAX_SAFE_INTEGER,
  pipe() {
    return pipeArguments(this, arguments);
  }
};
var FiberRefSymbolKey = "effect/FiberRef";
var FiberRefTypeId = /* @__PURE__ */ Symbol.for(FiberRefSymbolKey);
var fiberRefVariance = {
  /* c8 ignore next */
  _A: /* @__PURE__ */ __name((_) => _, "_A")
};
var fiberRefGet = /* @__PURE__ */ __name((self) => withFiberRuntime((fiber) => exitSucceed(fiber.getFiberRef(self))), "fiberRefGet");
var fiberRefGetWith = /* @__PURE__ */ dual(2, (self, f) => flatMap7(fiberRefGet(self), f));
var fiberRefSet = /* @__PURE__ */ dual(2, (self, value) => fiberRefModify(self, () => [void 0, value]));
var fiberRefModify = /* @__PURE__ */ dual(2, (self, f) => withFiberRuntime((state) => {
  const [b, a] = f(state.getFiberRef(self));
  state.setFiberRef(self, a);
  return succeed(b);
}));
var RequestResolverSymbolKey = "effect/RequestResolver";
var RequestResolverTypeId = /* @__PURE__ */ Symbol.for(RequestResolverSymbolKey);
var requestResolverVariance = {
  /* c8 ignore next */
  _A: /* @__PURE__ */ __name((_) => _, "_A"),
  /* c8 ignore next */
  _R: /* @__PURE__ */ __name((_) => _, "_R")
};
var RequestResolverImpl = class _RequestResolverImpl {
  static {
    __name(this, "RequestResolverImpl");
  }
  runAll;
  target;
  [RequestResolverTypeId] = requestResolverVariance;
  constructor(runAll, target) {
    this.runAll = runAll;
    this.target = target;
  }
  [symbol]() {
    return cached(this, this.target ? hash(this.target) : random(this));
  }
  [symbol2](that) {
    return this.target ? isRequestResolver(that) && equals(this.target, that.target) : this === that;
  }
  identified(...ids3) {
    return new _RequestResolverImpl(this.runAll, fromIterable2(ids3));
  }
  pipe() {
    return pipeArguments(this, arguments);
  }
};
var isRequestResolver = /* @__PURE__ */ __name((u) => hasProperty(u, RequestResolverTypeId), "isRequestResolver");
var fiberRefLocally = /* @__PURE__ */ dual(3, (use, self, value) => acquireUseRelease(zipLeft(fiberRefGet(self), fiberRefSet(self, value)), () => use, (oldValue) => fiberRefSet(self, oldValue)));
var fiberRefLocallyWith = /* @__PURE__ */ dual(3, (use, self, f) => fiberRefGetWith(self, (a) => fiberRefLocally(use, self, f(a))));
var fiberRefUnsafeMake = /* @__PURE__ */ __name((initial, options) => fiberRefUnsafeMakePatch(initial, {
  differ: update(),
  fork: options?.fork ?? identity,
  join: options?.join
}), "fiberRefUnsafeMake");
var fiberRefUnsafeMakeHashSet = /* @__PURE__ */ __name((initial) => {
  const differ3 = hashSet();
  return fiberRefUnsafeMakePatch(initial, {
    differ: differ3,
    fork: differ3.empty
  });
}, "fiberRefUnsafeMakeHashSet");
var fiberRefUnsafeMakeReadonlyArray = /* @__PURE__ */ __name((initial) => {
  const differ3 = readonlyArray(update());
  return fiberRefUnsafeMakePatch(initial, {
    differ: differ3,
    fork: differ3.empty
  });
}, "fiberRefUnsafeMakeReadonlyArray");
var fiberRefUnsafeMakeContext = /* @__PURE__ */ __name((initial) => {
  const differ3 = environment();
  return fiberRefUnsafeMakePatch(initial, {
    differ: differ3,
    fork: differ3.empty
  });
}, "fiberRefUnsafeMakeContext");
var fiberRefUnsafeMakePatch = /* @__PURE__ */ __name((initial, options) => {
  const _fiberRef = {
    ...CommitPrototype,
    [FiberRefTypeId]: fiberRefVariance,
    initial,
    commit() {
      return fiberRefGet(this);
    },
    diff: /* @__PURE__ */ __name((oldValue, newValue) => options.differ.diff(oldValue, newValue), "diff"),
    combine: /* @__PURE__ */ __name((first2, second) => options.differ.combine(first2, second), "combine"),
    patch: /* @__PURE__ */ __name((patch9) => (oldValue) => options.differ.patch(patch9, oldValue), "patch"),
    fork: options.fork,
    join: options.join ?? ((_, n) => n)
  };
  return _fiberRef;
}, "fiberRefUnsafeMakePatch");
var fiberRefUnsafeMakeRuntimeFlags = /* @__PURE__ */ __name((initial) => fiberRefUnsafeMakePatch(initial, {
  differ,
  fork: differ.empty
}), "fiberRefUnsafeMakeRuntimeFlags");
var currentContext = /* @__PURE__ */ globalValue(/* @__PURE__ */ Symbol.for("effect/FiberRef/currentContext"), () => fiberRefUnsafeMakeContext(empty3()));
var currentSchedulingPriority = /* @__PURE__ */ globalValue(/* @__PURE__ */ Symbol.for("effect/FiberRef/currentSchedulingPriority"), () => fiberRefUnsafeMake(0));
var currentMaxOpsBeforeYield = /* @__PURE__ */ globalValue(/* @__PURE__ */ Symbol.for("effect/FiberRef/currentMaxOpsBeforeYield"), () => fiberRefUnsafeMake(2048));
var currentLogAnnotations = /* @__PURE__ */ globalValue(/* @__PURE__ */ Symbol.for("effect/FiberRef/currentLogAnnotation"), () => fiberRefUnsafeMake(empty8()));
var currentLogLevel = /* @__PURE__ */ globalValue(/* @__PURE__ */ Symbol.for("effect/FiberRef/currentLogLevel"), () => fiberRefUnsafeMake(logLevelInfo));
var currentLogSpan = /* @__PURE__ */ globalValue(/* @__PURE__ */ Symbol.for("effect/FiberRef/currentLogSpan"), () => fiberRefUnsafeMake(empty9()));
var withSchedulingPriority = /* @__PURE__ */ dual(2, (self, scheduler) => fiberRefLocally(self, currentSchedulingPriority, scheduler));
var withMaxOpsBeforeYield = /* @__PURE__ */ dual(2, (self, scheduler) => fiberRefLocally(self, currentMaxOpsBeforeYield, scheduler));
var currentConcurrency = /* @__PURE__ */ globalValue(/* @__PURE__ */ Symbol.for("effect/FiberRef/currentConcurrency"), () => fiberRefUnsafeMake("unbounded"));
var currentRequestBatching = /* @__PURE__ */ globalValue(/* @__PURE__ */ Symbol.for("effect/FiberRef/currentRequestBatching"), () => fiberRefUnsafeMake(true));
var currentUnhandledErrorLogLevel = /* @__PURE__ */ globalValue(/* @__PURE__ */ Symbol.for("effect/FiberRef/currentUnhandledErrorLogLevel"), () => fiberRefUnsafeMake(some2(logLevelDebug)));
var currentVersionMismatchErrorLogLevel = /* @__PURE__ */ globalValue(/* @__PURE__ */ Symbol.for("effect/FiberRef/versionMismatchErrorLogLevel"), () => fiberRefUnsafeMake(some2(logLevelWarning)));
var withUnhandledErrorLogLevel = /* @__PURE__ */ dual(2, (self, level) => fiberRefLocally(self, currentUnhandledErrorLogLevel, level));
var currentMetricLabels = /* @__PURE__ */ globalValue(/* @__PURE__ */ Symbol.for("effect/FiberRef/currentMetricLabels"), () => fiberRefUnsafeMakeReadonlyArray(empty()));
var metricLabels = /* @__PURE__ */ fiberRefGet(currentMetricLabels);
var currentForkScopeOverride = /* @__PURE__ */ globalValue(/* @__PURE__ */ Symbol.for("effect/FiberRef/currentForkScopeOverride"), () => fiberRefUnsafeMake(none2(), {
  fork: /* @__PURE__ */ __name(() => none2(), "fork"),
  join: /* @__PURE__ */ __name((parent, _) => parent, "join")
}));
var currentInterruptedCause = /* @__PURE__ */ globalValue(/* @__PURE__ */ Symbol.for("effect/FiberRef/currentInterruptedCause"), () => fiberRefUnsafeMake(empty16, {
  fork: /* @__PURE__ */ __name(() => empty16, "fork"),
  join: /* @__PURE__ */ __name((parent, _) => parent, "join")
}));
var currentTracerEnabled = /* @__PURE__ */ globalValue(/* @__PURE__ */ Symbol.for("effect/FiberRef/currentTracerEnabled"), () => fiberRefUnsafeMake(true));
var currentTracerTimingEnabled = /* @__PURE__ */ globalValue(/* @__PURE__ */ Symbol.for("effect/FiberRef/currentTracerTiming"), () => fiberRefUnsafeMake(true));
var currentTracerSpanAnnotations = /* @__PURE__ */ globalValue(/* @__PURE__ */ Symbol.for("effect/FiberRef/currentTracerSpanAnnotations"), () => fiberRefUnsafeMake(empty8()));
var currentTracerSpanLinks = /* @__PURE__ */ globalValue(/* @__PURE__ */ Symbol.for("effect/FiberRef/currentTracerSpanLinks"), () => fiberRefUnsafeMake(empty4()));
var ScopeTypeId = /* @__PURE__ */ Symbol.for("effect/Scope");
var CloseableScopeTypeId = /* @__PURE__ */ Symbol.for("effect/CloseableScope");
var scopeAddFinalizer = /* @__PURE__ */ __name((self, finalizer) => self.addFinalizer(() => asVoid(finalizer)), "scopeAddFinalizer");
var scopeAddFinalizerExit = /* @__PURE__ */ __name((self, finalizer) => self.addFinalizer(finalizer), "scopeAddFinalizerExit");
var scopeClose = /* @__PURE__ */ __name((self, exit4) => self.close(exit4), "scopeClose");
var scopeFork = /* @__PURE__ */ __name((self, strategy) => self.fork(strategy), "scopeFork");
var causeSquash = /* @__PURE__ */ __name((self) => {
  return causeSquashWith(identity)(self);
}, "causeSquash");
var causeSquashWith = /* @__PURE__ */ dual(2, (self, f) => {
  const option3 = pipe(self, failureOption, map(f));
  switch (option3._tag) {
    case "None": {
      return pipe(defects(self), head2, match2({
        onNone: /* @__PURE__ */ __name(() => {
          const interrupts = fromIterable(interruptors(self)).flatMap((fiberId3) => fromIterable(ids2(fiberId3)).map((id) => `#${id}`));
          return new InterruptedException(interrupts ? `Interrupted by fibers: ${interrupts.join(", ")}` : void 0);
        }, "onNone"),
        onSome: identity
      }));
    }
    case "Some": {
      return option3.value;
    }
  }
});
var YieldableError = /* @__PURE__ */ (function() {
  class YieldableError3 extends globalThis.Error {
    static {
      __name(this, "YieldableError");
    }
    commit() {
      return fail2(this);
    }
    toJSON() {
      const obj = {
        ...this
      };
      if (this.message) obj.message = this.message;
      if (this.cause) obj.cause = this.cause;
      return obj;
    }
    [NodeInspectSymbol]() {
      if (this.toString !== globalThis.Error.prototype.toString) {
        return this.stack ? `${this.toString()}
${this.stack.split("\n").slice(1).join("\n")}` : this.toString();
      } else if ("Bun" in globalThis) {
        return pretty(fail(this), {
          renderErrorCause: true
        });
      }
      return this;
    }
  }
  Object.assign(YieldableError3.prototype, StructuralCommitPrototype);
  return YieldableError3;
})();
var makeException = /* @__PURE__ */ __name((proto4, tag) => {
  class Base3 extends YieldableError {
    static {
      __name(this, "Base");
    }
    _tag = tag;
  }
  Object.assign(Base3.prototype, proto4);
  Base3.prototype.name = tag;
  return Base3;
}, "makeException");
var RuntimeExceptionTypeId = /* @__PURE__ */ Symbol.for("effect/Cause/errors/RuntimeException");
var RuntimeException = /* @__PURE__ */ makeException({
  [RuntimeExceptionTypeId]: RuntimeExceptionTypeId
}, "RuntimeException");
var isRuntimeException = /* @__PURE__ */ __name((u) => hasProperty(u, RuntimeExceptionTypeId), "isRuntimeException");
var InterruptedExceptionTypeId = /* @__PURE__ */ Symbol.for("effect/Cause/errors/InterruptedException");
var InterruptedException = /* @__PURE__ */ makeException({
  [InterruptedExceptionTypeId]: InterruptedExceptionTypeId
}, "InterruptedException");
var isInterruptedException = /* @__PURE__ */ __name((u) => hasProperty(u, InterruptedExceptionTypeId), "isInterruptedException");
var IllegalArgumentExceptionTypeId = /* @__PURE__ */ Symbol.for("effect/Cause/errors/IllegalArgument");
var IllegalArgumentException = /* @__PURE__ */ makeException({
  [IllegalArgumentExceptionTypeId]: IllegalArgumentExceptionTypeId
}, "IllegalArgumentException");
var isIllegalArgumentException = /* @__PURE__ */ __name((u) => hasProperty(u, IllegalArgumentExceptionTypeId), "isIllegalArgumentException");
var NoSuchElementExceptionTypeId = /* @__PURE__ */ Symbol.for("effect/Cause/errors/NoSuchElement");
var NoSuchElementException = /* @__PURE__ */ makeException({
  [NoSuchElementExceptionTypeId]: NoSuchElementExceptionTypeId
}, "NoSuchElementException");
var isNoSuchElementException = /* @__PURE__ */ __name((u) => hasProperty(u, NoSuchElementExceptionTypeId), "isNoSuchElementException");
var InvalidPubSubCapacityExceptionTypeId = /* @__PURE__ */ Symbol.for("effect/Cause/errors/InvalidPubSubCapacityException");
var InvalidPubSubCapacityException = /* @__PURE__ */ makeException({
  [InvalidPubSubCapacityExceptionTypeId]: InvalidPubSubCapacityExceptionTypeId
}, "InvalidPubSubCapacityException");
var ExceededCapacityExceptionTypeId = /* @__PURE__ */ Symbol.for("effect/Cause/errors/ExceededCapacityException");
var ExceededCapacityException = /* @__PURE__ */ makeException({
  [ExceededCapacityExceptionTypeId]: ExceededCapacityExceptionTypeId
}, "ExceededCapacityException");
var isExceededCapacityException = /* @__PURE__ */ __name((u) => hasProperty(u, ExceededCapacityExceptionTypeId), "isExceededCapacityException");
var TimeoutExceptionTypeId = /* @__PURE__ */ Symbol.for("effect/Cause/errors/Timeout");
var TimeoutException = /* @__PURE__ */ makeException({
  [TimeoutExceptionTypeId]: TimeoutExceptionTypeId
}, "TimeoutException");
var timeoutExceptionFromDuration = /* @__PURE__ */ __name((duration) => new TimeoutException(`Operation timed out after '${format2(duration)}'`), "timeoutExceptionFromDuration");
var isTimeoutException = /* @__PURE__ */ __name((u) => hasProperty(u, TimeoutExceptionTypeId), "isTimeoutException");
var UnknownExceptionTypeId = /* @__PURE__ */ Symbol.for("effect/Cause/errors/UnknownException");
var UnknownException = /* @__PURE__ */ (function() {
  class UnknownException3 extends YieldableError {
    static {
      __name(this, "UnknownException");
    }
    _tag = "UnknownException";
    error;
    constructor(cause3, message) {
      super(message ?? "An unknown error occurred", {
        cause: cause3
      });
      this.error = cause3;
    }
  }
  Object.assign(UnknownException3.prototype, {
    [UnknownExceptionTypeId]: UnknownExceptionTypeId,
    name: "UnknownException"
  });
  return UnknownException3;
})();
var isUnknownException = /* @__PURE__ */ __name((u) => hasProperty(u, UnknownExceptionTypeId), "isUnknownException");
var exitIsExit = /* @__PURE__ */ __name((u) => isEffect(u) && "_tag" in u && (u._tag === "Success" || u._tag === "Failure"), "exitIsExit");
var exitIsFailure = /* @__PURE__ */ __name((self) => self._tag === "Failure", "exitIsFailure");
var exitIsSuccess = /* @__PURE__ */ __name((self) => self._tag === "Success", "exitIsSuccess");
var exitIsInterrupted = /* @__PURE__ */ __name((self) => {
  switch (self._tag) {
    case OP_FAILURE:
      return isInterrupted(self.effect_instruction_i0);
    case OP_SUCCESS:
      return false;
  }
}, "exitIsInterrupted");
var exitAs = /* @__PURE__ */ dual(2, (self, value) => {
  switch (self._tag) {
    case OP_FAILURE: {
      return exitFailCause(self.effect_instruction_i0);
    }
    case OP_SUCCESS: {
      return exitSucceed(value);
    }
  }
});
var exitAsVoid = /* @__PURE__ */ __name((self) => exitAs(self, void 0), "exitAsVoid");
var exitCauseOption = /* @__PURE__ */ __name((self) => {
  switch (self._tag) {
    case OP_FAILURE:
      return some2(self.effect_instruction_i0);
    case OP_SUCCESS:
      return none2();
  }
}, "exitCauseOption");
var exitCollectAll = /* @__PURE__ */ __name((exits, options) => exitCollectAllInternal(exits, options?.parallel ? parallel : sequential), "exitCollectAll");
var exitDie = /* @__PURE__ */ __name((defect) => exitFailCause(die(defect)), "exitDie");
var exitExists = /* @__PURE__ */ dual(2, (self, refinement) => {
  switch (self._tag) {
    case OP_FAILURE:
      return false;
    case OP_SUCCESS:
      return refinement(self.effect_instruction_i0);
  }
});
var exitFail = /* @__PURE__ */ __name((error) => exitFailCause(fail(error)), "exitFail");
var exitFailCause = /* @__PURE__ */ __name((cause3) => {
  const effect = new EffectPrimitiveFailure(OP_FAILURE);
  effect.effect_instruction_i0 = cause3;
  return effect;
}, "exitFailCause");
var exitFlatMap = /* @__PURE__ */ dual(2, (self, f) => {
  switch (self._tag) {
    case OP_FAILURE: {
      return exitFailCause(self.effect_instruction_i0);
    }
    case OP_SUCCESS: {
      return f(self.effect_instruction_i0);
    }
  }
});
var exitFlatMapEffect = /* @__PURE__ */ dual(2, (self, f) => {
  switch (self._tag) {
    case OP_FAILURE: {
      return succeed(exitFailCause(self.effect_instruction_i0));
    }
    case OP_SUCCESS: {
      return f(self.effect_instruction_i0);
    }
  }
});
var exitFlatten = /* @__PURE__ */ __name((self) => pipe(self, exitFlatMap(identity)), "exitFlatten");
var exitForEachEffect = /* @__PURE__ */ dual(2, (self, f) => {
  switch (self._tag) {
    case OP_FAILURE: {
      return succeed(exitFailCause(self.effect_instruction_i0));
    }
    case OP_SUCCESS: {
      return exit(f(self.effect_instruction_i0));
    }
  }
});
var exitFromEither = /* @__PURE__ */ __name((either4) => {
  switch (either4._tag) {
    case "Left":
      return exitFail(either4.left);
    case "Right":
      return exitSucceed(either4.right);
  }
}, "exitFromEither");
var exitFromOption = /* @__PURE__ */ __name((option3) => {
  switch (option3._tag) {
    case "None":
      return exitFail(void 0);
    case "Some":
      return exitSucceed(option3.value);
  }
}, "exitFromOption");
var exitGetOrElse = /* @__PURE__ */ dual(2, (self, orElse3) => {
  switch (self._tag) {
    case OP_FAILURE:
      return orElse3(self.effect_instruction_i0);
    case OP_SUCCESS:
      return self.effect_instruction_i0;
  }
});
var exitInterrupt = /* @__PURE__ */ __name((fiberId3) => exitFailCause(interrupt(fiberId3)), "exitInterrupt");
var exitMap = /* @__PURE__ */ dual(2, (self, f) => {
  switch (self._tag) {
    case OP_FAILURE:
      return exitFailCause(self.effect_instruction_i0);
    case OP_SUCCESS:
      return exitSucceed(f(self.effect_instruction_i0));
  }
});
var exitMapBoth = /* @__PURE__ */ dual(2, (self, {
  onFailure,
  onSuccess
}) => {
  switch (self._tag) {
    case OP_FAILURE:
      return exitFailCause(pipe(self.effect_instruction_i0, map7(onFailure)));
    case OP_SUCCESS:
      return exitSucceed(onSuccess(self.effect_instruction_i0));
  }
});
var exitMapError = /* @__PURE__ */ dual(2, (self, f) => {
  switch (self._tag) {
    case OP_FAILURE:
      return exitFailCause(pipe(self.effect_instruction_i0, map7(f)));
    case OP_SUCCESS:
      return exitSucceed(self.effect_instruction_i0);
  }
});
var exitMapErrorCause = /* @__PURE__ */ dual(2, (self, f) => {
  switch (self._tag) {
    case OP_FAILURE:
      return exitFailCause(f(self.effect_instruction_i0));
    case OP_SUCCESS:
      return exitSucceed(self.effect_instruction_i0);
  }
});
var exitMatch = /* @__PURE__ */ dual(2, (self, {
  onFailure,
  onSuccess
}) => {
  switch (self._tag) {
    case OP_FAILURE:
      return onFailure(self.effect_instruction_i0);
    case OP_SUCCESS:
      return onSuccess(self.effect_instruction_i0);
  }
});
var exitMatchEffect = /* @__PURE__ */ dual(2, (self, {
  onFailure,
  onSuccess
}) => {
  switch (self._tag) {
    case OP_FAILURE:
      return onFailure(self.effect_instruction_i0);
    case OP_SUCCESS:
      return onSuccess(self.effect_instruction_i0);
  }
});
var exitSucceed = /* @__PURE__ */ __name((value) => {
  const effect = new EffectPrimitiveSuccess(OP_SUCCESS);
  effect.effect_instruction_i0 = value;
  return effect;
}, "exitSucceed");
var exitVoid = /* @__PURE__ */ exitSucceed(void 0);
var exitZip = /* @__PURE__ */ dual(2, (self, that) => exitZipWith(self, that, {
  onSuccess: /* @__PURE__ */ __name((a, a2) => [a, a2], "onSuccess"),
  onFailure: sequential
}));
var exitZipLeft = /* @__PURE__ */ dual(2, (self, that) => exitZipWith(self, that, {
  onSuccess: /* @__PURE__ */ __name((a, _) => a, "onSuccess"),
  onFailure: sequential
}));
var exitZipRight = /* @__PURE__ */ dual(2, (self, that) => exitZipWith(self, that, {
  onSuccess: /* @__PURE__ */ __name((_, a2) => a2, "onSuccess"),
  onFailure: sequential
}));
var exitZipPar = /* @__PURE__ */ dual(2, (self, that) => exitZipWith(self, that, {
  onSuccess: /* @__PURE__ */ __name((a, a2) => [a, a2], "onSuccess"),
  onFailure: parallel
}));
var exitZipParLeft = /* @__PURE__ */ dual(2, (self, that) => exitZipWith(self, that, {
  onSuccess: /* @__PURE__ */ __name((a, _) => a, "onSuccess"),
  onFailure: parallel
}));
var exitZipParRight = /* @__PURE__ */ dual(2, (self, that) => exitZipWith(self, that, {
  onSuccess: /* @__PURE__ */ __name((_, a2) => a2, "onSuccess"),
  onFailure: parallel
}));
var exitZipWith = /* @__PURE__ */ dual(3, (self, that, {
  onFailure,
  onSuccess
}) => {
  switch (self._tag) {
    case OP_FAILURE: {
      switch (that._tag) {
        case OP_SUCCESS:
          return exitFailCause(self.effect_instruction_i0);
        case OP_FAILURE: {
          return exitFailCause(onFailure(self.effect_instruction_i0, that.effect_instruction_i0));
        }
      }
    }
    case OP_SUCCESS: {
      switch (that._tag) {
        case OP_SUCCESS:
          return exitSucceed(onSuccess(self.effect_instruction_i0, that.effect_instruction_i0));
        case OP_FAILURE:
          return exitFailCause(that.effect_instruction_i0);
      }
    }
  }
});
var exitCollectAllInternal = /* @__PURE__ */ __name((exits, combineCauses) => {
  const list = fromIterable2(exits);
  if (!isNonEmpty(list)) {
    return none2();
  }
  return pipe(tailNonEmpty2(list), reduce(pipe(headNonEmpty2(list), exitMap(of2)), (accumulator, current) => pipe(accumulator, exitZipWith(current, {
    onSuccess: /* @__PURE__ */ __name((list2, value) => pipe(list2, prepend2(value)), "onSuccess"),
    onFailure: combineCauses
  }))), exitMap(reverse2), exitMap((chunk2) => toReadonlyArray(chunk2)), some2);
}, "exitCollectAllInternal");
var deferredUnsafeMake = /* @__PURE__ */ __name((fiberId3) => {
  const _deferred = {
    ...CommitPrototype,
    [DeferredTypeId]: deferredVariance,
    state: make11(pending([])),
    commit() {
      return deferredAwait(this);
    },
    blockingOn: fiberId3
  };
  return _deferred;
}, "deferredUnsafeMake");
var deferredMake = /* @__PURE__ */ __name(() => flatMap7(fiberId, (id) => deferredMakeAs(id)), "deferredMake");
var deferredMakeAs = /* @__PURE__ */ __name((fiberId3) => sync(() => deferredUnsafeMake(fiberId3)), "deferredMakeAs");
var deferredAwait = /* @__PURE__ */ __name((self) => asyncInterrupt((resume2) => {
  const state = get6(self.state);
  switch (state._tag) {
    case OP_STATE_DONE: {
      return resume2(state.effect);
    }
    case OP_STATE_PENDING: {
      state.joiners.push(resume2);
      return deferredInterruptJoiner(self, resume2);
    }
  }
}, self.blockingOn), "deferredAwait");
var deferredComplete = /* @__PURE__ */ dual(2, (self, effect) => intoDeferred(effect, self));
var deferredCompleteWith = /* @__PURE__ */ dual(2, (self, effect) => sync(() => {
  const state = get6(self.state);
  switch (state._tag) {
    case OP_STATE_DONE: {
      return false;
    }
    case OP_STATE_PENDING: {
      set2(self.state, done(effect));
      for (let i = 0, len = state.joiners.length; i < len; i++) {
        state.joiners[i](effect);
      }
      return true;
    }
  }
}));
var deferredDone = /* @__PURE__ */ dual(2, (self, exit4) => deferredCompleteWith(self, exit4));
var deferredFailCause = /* @__PURE__ */ dual(2, (self, cause3) => deferredCompleteWith(self, failCause(cause3)));
var deferredInterrupt = /* @__PURE__ */ __name((self) => flatMap7(fiberId, (fiberId3) => deferredCompleteWith(self, interruptWith(fiberId3))), "deferredInterrupt");
var deferredSucceed = /* @__PURE__ */ dual(2, (self, value) => deferredCompleteWith(self, succeed(value)));
var deferredUnsafeDone = /* @__PURE__ */ __name((self, effect) => {
  const state = get6(self.state);
  if (state._tag === OP_STATE_PENDING) {
    set2(self.state, done(effect));
    for (let i = 0, len = state.joiners.length; i < len; i++) {
      state.joiners[i](effect);
    }
  }
}, "deferredUnsafeDone");
var deferredInterruptJoiner = /* @__PURE__ */ __name((self, joiner) => sync(() => {
  const state = get6(self.state);
  if (state._tag === OP_STATE_PENDING) {
    const index = state.joiners.indexOf(joiner);
    if (index >= 0) {
      state.joiners.splice(index, 1);
    }
  }
}), "deferredInterruptJoiner");
var constContext = /* @__PURE__ */ withFiberRuntime((fiber) => exitSucceed(fiber.currentContext));
var context = /* @__PURE__ */ __name(() => constContext, "context");
var contextWithEffect = /* @__PURE__ */ __name((f) => flatMap7(context(), f), "contextWithEffect");
var provideContext = /* @__PURE__ */ dual(2, (self, context4) => fiberRefLocally(currentContext, context4)(self));
var provideSomeContext = /* @__PURE__ */ dual(2, (self, context4) => fiberRefLocallyWith(currentContext, (parent) => merge3(parent, context4))(self));
var mapInputContext = /* @__PURE__ */ dual(2, (self, f) => contextWithEffect((context4) => provideContext(self, f(context4))));
var filterEffectOrElse = /* @__PURE__ */ dual(2, (self, options) => flatMap7(self, (a) => flatMap7(options.predicate(a), (pass) => pass ? succeed(a) : options.orElse(a))));
var filterEffectOrFail = /* @__PURE__ */ dual(2, (self, options) => filterEffectOrElse(self, {
  predicate: options.predicate,
  orElse: /* @__PURE__ */ __name((a) => fail2(options.orFailWith(a)), "orElse")
}));
var currentSpanFromFiber = /* @__PURE__ */ __name((fiber) => {
  const span2 = fiber.currentSpan;
  return span2 !== void 0 && span2._tag === "Span" ? some2(span2) : none2();
}, "currentSpanFromFiber");
var NoopSpanProto = {
  _tag: "Span",
  spanId: "noop",
  traceId: "noop",
  sampled: false,
  status: {
    _tag: "Ended",
    startTime: /* @__PURE__ */ BigInt(0),
    endTime: /* @__PURE__ */ BigInt(0),
    exit: exitVoid
  },
  attributes: /* @__PURE__ */ new Map(),
  links: [],
  kind: "internal",
  attribute() {
  },
  event() {
  },
  end() {
  },
  addLinks() {
  }
};
var noopSpan = /* @__PURE__ */ __name((options) => Object.assign(Object.create(NoopSpanProto), options), "noopSpan");

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/Deferred.js
var _await = deferredAwait;
var done2 = deferredDone;
var interrupt3 = deferredInterrupt;
var unsafeMake3 = deferredUnsafeMake;

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/Exit.js
var Exit_exports = {};
__export(Exit_exports, {
  all: () => all,
  as: () => as3,
  asVoid: () => asVoid2,
  causeOption: () => causeOption,
  die: () => die3,
  exists: () => exists,
  fail: () => fail3,
  failCause: () => failCause2,
  flatMap: () => flatMap8,
  flatMapEffect: () => flatMapEffect,
  flatten: () => flatten5,
  forEachEffect: () => forEachEffect,
  fromEither: () => fromEither,
  fromOption: () => fromOption2,
  getOrElse: () => getOrElse4,
  interrupt: () => interrupt4,
  isExit: () => isExit,
  isFailure: () => isFailure2,
  isInterrupted: () => isInterrupted2,
  isSuccess: () => isSuccess,
  map: () => map9,
  mapBoth: () => mapBoth2,
  mapError: () => mapError2,
  mapErrorCause: () => mapErrorCause,
  match: () => match5,
  matchEffect: () => matchEffect2,
  succeed: () => succeed2,
  void: () => void_2,
  zip: () => zip3,
  zipLeft: () => zipLeft2,
  zipPar: () => zipPar,
  zipParLeft: () => zipParLeft,
  zipParRight: () => zipParRight,
  zipRight: () => zipRight2,
  zipWith: () => zipWith3
});
var isExit = exitIsExit;
var isFailure2 = exitIsFailure;
var isSuccess = exitIsSuccess;
var isInterrupted2 = exitIsInterrupted;
var as3 = exitAs;
var asVoid2 = exitAsVoid;
var causeOption = exitCauseOption;
var all = exitCollectAll;
var die3 = exitDie;
var exists = exitExists;
var fail3 = exitFail;
var failCause2 = exitFailCause;
var flatMap8 = exitFlatMap;
var flatMapEffect = exitFlatMapEffect;
var flatten5 = exitFlatten;
var forEachEffect = exitForEachEffect;
var fromEither = exitFromEither;
var fromOption2 = exitFromOption;
var getOrElse4 = exitGetOrElse;
var interrupt4 = exitInterrupt;
var map9 = exitMap;
var mapBoth2 = exitMapBoth;
var mapError2 = exitMapError;
var mapErrorCause = exitMapErrorCause;
var match5 = exitMatch;
var matchEffect2 = exitMatchEffect;
var succeed2 = exitSucceed;
var void_2 = exitVoid;
var zip3 = exitZip;
var zipLeft2 = exitZipLeft;
var zipRight2 = exitZipRight;
var zipPar = exitZipPar;
var zipParLeft = exitZipParLeft;
var zipParRight = exitZipParRight;
var zipWith3 = exitZipWith;

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/MutableHashMap.js
var TypeId8 = /* @__PURE__ */ Symbol.for("effect/MutableHashMap");
var MutableHashMapProto = {
  [TypeId8]: TypeId8,
  [Symbol.iterator]() {
    return new MutableHashMapIterator(this);
  },
  toString() {
    return format(this.toJSON());
  },
  toJSON() {
    return {
      _id: "MutableHashMap",
      values: Array.from(this).map(toJSON)
    };
  },
  [NodeInspectSymbol]() {
    return this.toJSON();
  },
  pipe() {
    return pipeArguments(this, arguments);
  }
};
var MutableHashMapIterator = class _MutableHashMapIterator {
  static {
    __name(this, "MutableHashMapIterator");
  }
  self;
  referentialIterator;
  bucketIterator;
  constructor(self) {
    this.self = self;
    this.referentialIterator = self.referential[Symbol.iterator]();
  }
  next() {
    if (this.bucketIterator !== void 0) {
      return this.bucketIterator.next();
    }
    const result = this.referentialIterator.next();
    if (result.done) {
      this.bucketIterator = new BucketIterator(this.self.buckets.values());
      return this.next();
    }
    return result;
  }
  [Symbol.iterator]() {
    return new _MutableHashMapIterator(this.self);
  }
};
var BucketIterator = class {
  static {
    __name(this, "BucketIterator");
  }
  backing;
  constructor(backing) {
    this.backing = backing;
  }
  currentBucket;
  next() {
    if (this.currentBucket === void 0) {
      const result2 = this.backing.next();
      if (result2.done) {
        return result2;
      }
      this.currentBucket = result2.value[Symbol.iterator]();
    }
    const result = this.currentBucket.next();
    if (result.done) {
      this.currentBucket = void 0;
      return this.next();
    }
    return result;
  }
};
var empty17 = /* @__PURE__ */ __name(() => {
  const self = Object.create(MutableHashMapProto);
  self.referential = /* @__PURE__ */ new Map();
  self.buckets = /* @__PURE__ */ new Map();
  self.bucketsSize = 0;
  return self;
}, "empty");
var get8 = /* @__PURE__ */ dual(2, (self, key) => {
  if (isEqual(key) === false) {
    return self.referential.has(key) ? some2(self.referential.get(key)) : none2();
  }
  const hash2 = key[symbol]();
  const bucket = self.buckets.get(hash2);
  if (bucket === void 0) {
    return none2();
  }
  return getFromBucket(self, bucket, key);
});
var getFromBucket = /* @__PURE__ */ __name((self, bucket, key, remove8 = false) => {
  for (let i = 0, len = bucket.length; i < len; i++) {
    if (key[symbol2](bucket[i][0])) {
      const value = bucket[i][1];
      if (remove8) {
        bucket.splice(i, 1);
        self.bucketsSize--;
      }
      return some2(value);
    }
  }
  return none2();
}, "getFromBucket");
var has4 = /* @__PURE__ */ dual(2, (self, key) => isSome2(get8(self, key)));
var set4 = /* @__PURE__ */ dual(3, (self, key, value) => {
  if (isEqual(key) === false) {
    self.referential.set(key, value);
    return self;
  }
  const hash2 = key[symbol]();
  const bucket = self.buckets.get(hash2);
  if (bucket === void 0) {
    self.buckets.set(hash2, [[key, value]]);
    self.bucketsSize++;
    return self;
  }
  removeFromBucket(self, bucket, key);
  bucket.push([key, value]);
  self.bucketsSize++;
  return self;
});
var removeFromBucket = /* @__PURE__ */ __name((self, bucket, key) => {
  for (let i = 0, len = bucket.length; i < len; i++) {
    if (key[symbol2](bucket[i][0])) {
      bucket.splice(i, 1);
      self.bucketsSize--;
      return;
    }
  }
}, "removeFromBucket");
var remove5 = /* @__PURE__ */ dual(2, (self, key) => {
  if (isEqual(key) === false) {
    self.referential.delete(key);
    return self;
  }
  const hash2 = key[symbol]();
  const bucket = self.buckets.get(hash2);
  if (bucket === void 0) {
    return self;
  }
  removeFromBucket(self, bucket, key);
  if (bucket.length === 0) {
    self.buckets.delete(hash2);
  }
  return self;
});
var size5 = /* @__PURE__ */ __name((self) => {
  return self.referential.size + self.bucketsSize;
}, "size");

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/MutableList.js
var TypeId9 = /* @__PURE__ */ Symbol.for("effect/MutableList");
var MutableListProto = {
  [TypeId9]: TypeId9,
  [Symbol.iterator]() {
    let done7 = false;
    let head5 = this.head;
    return {
      next() {
        if (done7) {
          return this.return();
        }
        if (head5 == null) {
          done7 = true;
          return this.return();
        }
        const value = head5.value;
        head5 = head5.next;
        return {
          done: done7,
          value
        };
      },
      return(value) {
        if (!done7) {
          done7 = true;
        }
        return {
          done: true,
          value
        };
      }
    };
  },
  toString() {
    return format(this.toJSON());
  },
  toJSON() {
    return {
      _id: "MutableList",
      values: Array.from(this).map(toJSON)
    };
  },
  [NodeInspectSymbol]() {
    return this.toJSON();
  },
  pipe() {
    return pipeArguments(this, arguments);
  }
};
var makeNode = /* @__PURE__ */ __name((value) => ({
  value,
  removed: false,
  prev: void 0,
  next: void 0
}), "makeNode");
var empty18 = /* @__PURE__ */ __name(() => {
  const list = Object.create(MutableListProto);
  list.head = void 0;
  list.tail = void 0;
  list._length = 0;
  return list;
}, "empty");
var isEmpty6 = /* @__PURE__ */ __name((self) => length(self) === 0, "isEmpty");
var length = /* @__PURE__ */ __name((self) => self._length, "length");
var append3 = /* @__PURE__ */ dual(2, (self, value) => {
  const node = makeNode(value);
  if (self.head === void 0) {
    self.head = node;
  }
  if (self.tail === void 0) {
    self.tail = node;
  } else {
    self.tail.next = node;
    node.prev = self.tail;
    self.tail = node;
  }
  ;
  self._length += 1;
  return self;
});
var shift = /* @__PURE__ */ __name((self) => {
  const head5 = self.head;
  if (head5 !== void 0) {
    remove6(self, head5);
    return head5.value;
  }
  return void 0;
}, "shift");
var remove6 = /* @__PURE__ */ __name((self, node) => {
  if (node.removed) {
    return;
  }
  node.removed = true;
  if (node.prev !== void 0 && node.next !== void 0) {
    node.prev.next = node.next;
    node.next.prev = node.prev;
  } else if (node.prev !== void 0) {
    self.tail = node.prev;
    node.prev.next = void 0;
  } else if (node.next !== void 0) {
    self.head = node.next;
    node.next.prev = void 0;
  } else {
    self.tail = void 0;
    self.head = void 0;
  }
  if (self._length > 0) {
    ;
    self._length -= 1;
  }
}, "remove");

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/MutableQueue.js
var TypeId10 = /* @__PURE__ */ Symbol.for("effect/MutableQueue");
var EmptyMutableQueue = /* @__PURE__ */ Symbol.for("effect/mutable/MutableQueue/Empty");
var MutableQueueProto = {
  [TypeId10]: TypeId10,
  [Symbol.iterator]() {
    return Array.from(this.queue)[Symbol.iterator]();
  },
  toString() {
    return format(this.toJSON());
  },
  toJSON() {
    return {
      _id: "MutableQueue",
      values: Array.from(this).map(toJSON)
    };
  },
  [NodeInspectSymbol]() {
    return this.toJSON();
  },
  pipe() {
    return pipeArguments(this, arguments);
  }
};
var make18 = /* @__PURE__ */ __name((capacity) => {
  const queue = Object.create(MutableQueueProto);
  queue.queue = empty18();
  queue.capacity = capacity;
  return queue;
}, "make");
var unbounded = /* @__PURE__ */ __name(() => make18(void 0), "unbounded");
var offer = /* @__PURE__ */ dual(2, (self, value) => {
  const queueLength = length(self.queue);
  if (self.capacity !== void 0 && queueLength === self.capacity) {
    return false;
  }
  append3(value)(self.queue);
  return true;
});
var poll = /* @__PURE__ */ dual(2, (self, def) => {
  if (isEmpty6(self.queue)) {
    return def;
  }
  return shift(self.queue);
});

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/internal/clock.js
var ClockSymbolKey = "effect/Clock";
var ClockTypeId = /* @__PURE__ */ Symbol.for(ClockSymbolKey);
var clockTag = /* @__PURE__ */ GenericTag("effect/Clock");
var MAX_TIMER_MILLIS = 2 ** 31 - 1;
var globalClockScheduler = {
  unsafeSchedule(task, duration) {
    const millis2 = toMillis(duration);
    if (millis2 > MAX_TIMER_MILLIS) {
      return constFalse;
    }
    let completed = false;
    const handle = setTimeout(() => {
      completed = true;
      task();
    }, millis2);
    return () => {
      clearTimeout(handle);
      return !completed;
    };
  }
};
var performanceNowNanos = /* @__PURE__ */ (function() {
  const bigint1e62 = /* @__PURE__ */ BigInt(1e6);
  if (typeof performance === "undefined" || typeof performance.now !== "function") {
    return () => BigInt(Date.now()) * bigint1e62;
  }
  let origin;
  return () => {
    if (origin === void 0) {
      origin = BigInt(Date.now()) * bigint1e62 - BigInt(Math.round(performance.now() * 1e6));
    }
    return origin + BigInt(Math.round(performance.now() * 1e6));
  };
})();
var processOrPerformanceNow = /* @__PURE__ */ (function() {
  const processHrtime = typeof process === "object" && "hrtime" in process && typeof process.hrtime.bigint === "function" ? process.hrtime : void 0;
  if (!processHrtime) {
    return performanceNowNanos;
  }
  const origin = /* @__PURE__ */ performanceNowNanos() - /* @__PURE__ */ processHrtime.bigint();
  return () => origin + processHrtime.bigint();
})();
var ClockImpl = class {
  static {
    __name(this, "ClockImpl");
  }
  [ClockTypeId] = ClockTypeId;
  unsafeCurrentTimeMillis() {
    return Date.now();
  }
  unsafeCurrentTimeNanos() {
    return processOrPerformanceNow();
  }
  currentTimeMillis = /* @__PURE__ */ sync(() => this.unsafeCurrentTimeMillis());
  currentTimeNanos = /* @__PURE__ */ sync(() => this.unsafeCurrentTimeNanos());
  scheduler() {
    return succeed(globalClockScheduler);
  }
  sleep(duration) {
    return async_((resume2) => {
      const canceler = globalClockScheduler.unsafeSchedule(() => resume2(void_), duration);
      return asVoid(sync(canceler));
    });
  }
};
var make19 = /* @__PURE__ */ __name(() => new ClockImpl(), "make");

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/internal/opCodes/configError.js
var OP_AND = "And";
var OP_OR = "Or";
var OP_INVALID_DATA = "InvalidData";
var OP_MISSING_DATA = "MissingData";
var OP_SOURCE_UNAVAILABLE = "SourceUnavailable";
var OP_UNSUPPORTED = "Unsupported";

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/internal/configError.js
var ConfigErrorSymbolKey = "effect/ConfigError";
var ConfigErrorTypeId = /* @__PURE__ */ Symbol.for(ConfigErrorSymbolKey);
var proto2 = {
  _tag: "ConfigError",
  [ConfigErrorTypeId]: ConfigErrorTypeId
};
var And = /* @__PURE__ */ __name((self, that) => {
  const error = Object.create(proto2);
  error._op = OP_AND;
  error.left = self;
  error.right = that;
  Object.defineProperty(error, "toString", {
    enumerable: false,
    value() {
      return `${this.left} and ${this.right}`;
    }
  });
  Object.defineProperty(error, "message", {
    enumerable: false,
    get() {
      return this.toString();
    }
  });
  return error;
}, "And");
var Or = /* @__PURE__ */ __name((self, that) => {
  const error = Object.create(proto2);
  error._op = OP_OR;
  error.left = self;
  error.right = that;
  Object.defineProperty(error, "toString", {
    enumerable: false,
    value() {
      return `${this.left} or ${this.right}`;
    }
  });
  Object.defineProperty(error, "message", {
    enumerable: false,
    get() {
      return this.toString();
    }
  });
  return error;
}, "Or");
var InvalidData = /* @__PURE__ */ __name((path, message, options = {
  pathDelim: "."
}) => {
  const error = Object.create(proto2);
  error._op = OP_INVALID_DATA;
  error.path = path;
  error.message = message;
  Object.defineProperty(error, "toString", {
    enumerable: false,
    value() {
      const path2 = pipe(this.path, join(options.pathDelim));
      return `(Invalid data at ${path2}: "${this.message}")`;
    }
  });
  return error;
}, "InvalidData");
var MissingData = /* @__PURE__ */ __name((path, message, options = {
  pathDelim: "."
}) => {
  const error = Object.create(proto2);
  error._op = OP_MISSING_DATA;
  error.path = path;
  error.message = message;
  Object.defineProperty(error, "toString", {
    enumerable: false,
    value() {
      const path2 = pipe(this.path, join(options.pathDelim));
      return `(Missing data at ${path2}: "${this.message}")`;
    }
  });
  return error;
}, "MissingData");
var SourceUnavailable = /* @__PURE__ */ __name((path, message, cause3, options = {
  pathDelim: "."
}) => {
  const error = Object.create(proto2);
  error._op = OP_SOURCE_UNAVAILABLE;
  error.path = path;
  error.message = message;
  error.cause = cause3;
  Object.defineProperty(error, "toString", {
    enumerable: false,
    value() {
      const path2 = pipe(this.path, join(options.pathDelim));
      return `(Source unavailable at ${path2}: "${this.message}")`;
    }
  });
  return error;
}, "SourceUnavailable");
var Unsupported = /* @__PURE__ */ __name((path, message, options = {
  pathDelim: "."
}) => {
  const error = Object.create(proto2);
  error._op = OP_UNSUPPORTED;
  error.path = path;
  error.message = message;
  Object.defineProperty(error, "toString", {
    enumerable: false,
    value() {
      const path2 = pipe(this.path, join(options.pathDelim));
      return `(Unsupported operation at ${path2}: "${this.message}")`;
    }
  });
  return error;
}, "Unsupported");
var prefixed = /* @__PURE__ */ dual(2, (self, prefix) => {
  switch (self._op) {
    case OP_AND: {
      return And(prefixed(self.left, prefix), prefixed(self.right, prefix));
    }
    case OP_OR: {
      return Or(prefixed(self.left, prefix), prefixed(self.right, prefix));
    }
    case OP_INVALID_DATA: {
      return InvalidData([...prefix, ...self.path], self.message);
    }
    case OP_MISSING_DATA: {
      return MissingData([...prefix, ...self.path], self.message);
    }
    case OP_SOURCE_UNAVAILABLE: {
      return SourceUnavailable([...prefix, ...self.path], self.message, self.cause);
    }
    case OP_UNSUPPORTED: {
      return Unsupported([...prefix, ...self.path], self.message);
    }
  }
});

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/internal/configProvider/pathPatch.js
var empty19 = {
  _tag: "Empty"
};
var patch5 = /* @__PURE__ */ dual(2, (path, patch9) => {
  let input = of3(patch9);
  let output = path;
  while (isCons(input)) {
    const patch10 = input.head;
    switch (patch10._tag) {
      case "Empty": {
        input = input.tail;
        break;
      }
      case "AndThen": {
        input = cons(patch10.first, cons(patch10.second, input.tail));
        break;
      }
      case "MapName": {
        output = map2(output, patch10.f);
        input = input.tail;
        break;
      }
      case "Nested": {
        output = prepend(output, patch10.name);
        input = input.tail;
        break;
      }
      case "Unnested": {
        const containsName = pipe(head(output), contains(patch10.name));
        if (containsName) {
          output = tailNonEmpty(output);
          input = input.tail;
        } else {
          return left2(MissingData(output, `Expected ${patch10.name} to be in path in ConfigProvider#unnested`));
        }
        break;
      }
    }
  }
  return right2(output);
});

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/internal/opCodes/config.js
var OP_CONSTANT = "Constant";
var OP_FAIL2 = "Fail";
var OP_FALLBACK = "Fallback";
var OP_DESCRIBED = "Described";
var OP_LAZY = "Lazy";
var OP_MAP_OR_FAIL = "MapOrFail";
var OP_NESTED = "Nested";
var OP_PRIMITIVE = "Primitive";
var OP_SEQUENCE = "Sequence";
var OP_HASHMAP = "HashMap";
var OP_ZIP_WITH = "ZipWith";

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/internal/configProvider.js
var concat = /* @__PURE__ */ __name((l, r) => [...l, ...r], "concat");
var ConfigProviderSymbolKey = "effect/ConfigProvider";
var ConfigProviderTypeId = /* @__PURE__ */ Symbol.for(ConfigProviderSymbolKey);
var configProviderTag = /* @__PURE__ */ GenericTag("effect/ConfigProvider");
var FlatConfigProviderSymbolKey = "effect/ConfigProviderFlat";
var FlatConfigProviderTypeId = /* @__PURE__ */ Symbol.for(FlatConfigProviderSymbolKey);
var make21 = /* @__PURE__ */ __name((options) => ({
  [ConfigProviderTypeId]: ConfigProviderTypeId,
  pipe() {
    return pipeArguments(this, arguments);
  },
  ...options
}), "make");
var makeFlat = /* @__PURE__ */ __name((options) => ({
  [FlatConfigProviderTypeId]: FlatConfigProviderTypeId,
  patch: options.patch,
  load: /* @__PURE__ */ __name((path, config, split = true) => options.load(path, config, split), "load"),
  enumerateChildren: options.enumerateChildren
}), "makeFlat");
var fromFlat = /* @__PURE__ */ __name((flat) => make21({
  load: /* @__PURE__ */ __name((config) => flatMap7(fromFlatLoop(flat, empty(), config, false), (chunk2) => match2(head(chunk2), {
    onNone: /* @__PURE__ */ __name(() => fail2(MissingData(empty(), `Expected a single value having structure: ${config}`)), "onNone"),
    onSome: succeed
  })), "load"),
  flattened: flat
}), "fromFlat");
var fromEnv = /* @__PURE__ */ __name((options) => {
  const {
    pathDelim,
    seqDelim
  } = Object.assign({}, {
    pathDelim: "_",
    seqDelim: ","
  }, options);
  const makePathString = /* @__PURE__ */ __name((path) => pipe(path, join(pathDelim)), "makePathString");
  const unmakePathString = /* @__PURE__ */ __name((pathString) => pathString.split(pathDelim), "unmakePathString");
  const getEnv = /* @__PURE__ */ __name(() => typeof process !== "undefined" && "env" in process && typeof process.env === "object" ? process.env : {}, "getEnv");
  const load = /* @__PURE__ */ __name((path, primitive, split = true) => {
    const pathString = makePathString(path);
    const current = getEnv();
    const valueOpt = pathString in current ? some2(current[pathString]) : none2();
    return pipe(valueOpt, mapError(() => MissingData(path, `Expected ${pathString} to exist in the process context`)), flatMap7((value) => parsePrimitive(value, path, primitive, seqDelim, split)));
  }, "load");
  const enumerateChildren = /* @__PURE__ */ __name((path) => sync(() => {
    const current = getEnv();
    const keys5 = Object.keys(current);
    const keyPaths = keys5.map((value) => unmakePathString(value.toUpperCase()));
    const filteredKeyPaths = keyPaths.filter((keyPath) => {
      for (let i = 0; i < path.length; i++) {
        const pathComponent = pipe(path, unsafeGet(i));
        const currentElement = keyPath[i];
        if (currentElement === void 0 || pathComponent !== currentElement) {
          return false;
        }
      }
      return true;
    }).flatMap((keyPath) => keyPath.slice(path.length, path.length + 1));
    return fromIterable5(filteredKeyPaths);
  }), "enumerateChildren");
  return fromFlat(makeFlat({
    load,
    enumerateChildren,
    patch: empty19
  }));
}, "fromEnv");
var extend = /* @__PURE__ */ __name((leftDef, rightDef, left3, right3) => {
  const leftPad = unfold(left3.length, (index) => index >= right3.length ? none2() : some2([leftDef(index), index + 1]));
  const rightPad = unfold(right3.length, (index) => index >= left3.length ? none2() : some2([rightDef(index), index + 1]));
  const leftExtension = concat(left3, leftPad);
  const rightExtension = concat(right3, rightPad);
  return [leftExtension, rightExtension];
}, "extend");
var appendConfigPath = /* @__PURE__ */ __name((path, config) => {
  let op = config;
  if (op._tag === "Nested") {
    const out = path.slice();
    while (op._tag === "Nested") {
      out.push(op.name);
      op = op.config;
    }
    return out;
  }
  return path;
}, "appendConfigPath");
var fromFlatLoop = /* @__PURE__ */ __name((flat, prefix, config, split) => {
  const op = config;
  switch (op._tag) {
    case OP_CONSTANT: {
      return succeed(of(op.value));
    }
    case OP_DESCRIBED: {
      return suspend(() => fromFlatLoop(flat, prefix, op.config, split));
    }
    case OP_FAIL2: {
      return fail2(MissingData(prefix, op.message));
    }
    case OP_FALLBACK: {
      return pipe(suspend(() => fromFlatLoop(flat, prefix, op.first, split)), catchAll((error1) => {
        if (op.condition(error1)) {
          return pipe(fromFlatLoop(flat, prefix, op.second, split), catchAll((error2) => fail2(Or(error1, error2))));
        }
        return fail2(error1);
      }));
    }
    case OP_LAZY: {
      return suspend(() => fromFlatLoop(flat, prefix, op.config(), split));
    }
    case OP_MAP_OR_FAIL: {
      return suspend(() => pipe(fromFlatLoop(flat, prefix, op.original, split), flatMap7(forEachSequential((a) => pipe(op.mapOrFail(a), mapError(prefixed(appendConfigPath(prefix, op.original))))))));
    }
    case OP_NESTED: {
      return suspend(() => fromFlatLoop(flat, concat(prefix, of(op.name)), op.config, split));
    }
    case OP_PRIMITIVE: {
      return pipe(patch5(prefix, flat.patch), flatMap7((prefix2) => pipe(flat.load(prefix2, op, split), flatMap7((values3) => {
        if (values3.length === 0) {
          const name = pipe(last(prefix2), getOrElse(() => "<n/a>"));
          return fail2(MissingData([], `Expected ${op.description} with name ${name}`));
        }
        return succeed(values3);
      }))));
    }
    case OP_SEQUENCE: {
      return pipe(patch5(prefix, flat.patch), flatMap7((patchedPrefix) => pipe(flat.enumerateChildren(patchedPrefix), flatMap7(indicesFrom), flatMap7((indices) => {
        if (indices.length === 0) {
          return suspend(() => map8(fromFlatLoop(flat, prefix, op.config, true), of));
        }
        return pipe(forEachSequential(indices, (index) => fromFlatLoop(flat, append(prefix, `[${index}]`), op.config, true)), map8((chunkChunk) => {
          const flattened = flatten(chunkChunk);
          if (flattened.length === 0) {
            return of(empty());
          }
          return of(flattened);
        }));
      }))));
    }
    case OP_HASHMAP: {
      return suspend(() => pipe(patch5(prefix, flat.patch), flatMap7((prefix2) => pipe(flat.enumerateChildren(prefix2), flatMap7((keys5) => {
        return pipe(keys5, forEachSequential((key) => fromFlatLoop(flat, concat(prefix2, of(key)), op.valueConfig, split)), map8((matrix) => {
          if (matrix.length === 0) {
            return of(empty8());
          }
          return pipe(transpose(matrix), map2((values3) => fromIterable6(zip(fromIterable(keys5), values3))));
        }));
      })))));
    }
    case OP_ZIP_WITH: {
      return suspend(() => pipe(fromFlatLoop(flat, prefix, op.left, split), either2, flatMap7((left3) => pipe(fromFlatLoop(flat, prefix, op.right, split), either2, flatMap7((right3) => {
        if (isLeft2(left3) && isLeft2(right3)) {
          return fail2(And(left3.left, right3.left));
        }
        if (isLeft2(left3) && isRight2(right3)) {
          return fail2(left3.left);
        }
        if (isRight2(left3) && isLeft2(right3)) {
          return fail2(right3.left);
        }
        if (isRight2(left3) && isRight2(right3)) {
          const path = pipe(prefix, join("."));
          const fail7 = fromFlatLoopFail(prefix, path);
          const [lefts, rights] = extend(fail7, fail7, pipe(left3.right, map2(right2)), pipe(right3.right, map2(right2)));
          return pipe(lefts, zip(rights), forEachSequential(([left4, right4]) => pipe(zip2(left4, right4), map8(([left5, right5]) => op.zip(left5, right5)))));
        }
        throw new Error("BUG: ConfigProvider.fromFlatLoop - please report an issue at https://github.com/Effect-TS/effect/issues");
      })))));
    }
  }
}, "fromFlatLoop");
var fromFlatLoopFail = /* @__PURE__ */ __name((prefix, path) => (index) => left2(MissingData(prefix, `The element at index ${index} in a sequence at path "${path}" was missing`)), "fromFlatLoopFail");
var splitPathString = /* @__PURE__ */ __name((text, delim) => {
  const split = text.split(new RegExp(`\\s*${escape(delim)}\\s*`));
  return split;
}, "splitPathString");
var parsePrimitive = /* @__PURE__ */ __name((text, path, primitive, delimiter, split) => {
  if (!split) {
    return pipe(primitive.parse(text), mapBoth({
      onFailure: prefixed(path),
      onSuccess: of
    }));
  }
  return pipe(splitPathString(text, delimiter), forEachSequential((char) => primitive.parse(char.trim())), mapError(prefixed(path)));
}, "parsePrimitive");
var transpose = /* @__PURE__ */ __name((array3) => {
  return Object.keys(array3[0]).map((column) => array3.map((row) => row[column]));
}, "transpose");
var indicesFrom = /* @__PURE__ */ __name((quotedIndices) => pipe(forEachSequential(quotedIndices, parseQuotedIndex), mapBoth({
  onFailure: /* @__PURE__ */ __name(() => empty(), "onFailure"),
  onSuccess: sort(Order)
}), either2, map8(merge)), "indicesFrom");
var QUOTED_INDEX_REGEX = /^(\[(\d+)\])$/;
var parseQuotedIndex = /* @__PURE__ */ __name((str) => {
  const match12 = str.match(QUOTED_INDEX_REGEX);
  if (match12 !== null) {
    const matchedIndex = match12[2];
    return pipe(matchedIndex !== void 0 && matchedIndex.length > 0 ? some2(matchedIndex) : none2(), flatMap(parseInteger));
  }
  return none2();
}, "parseQuotedIndex");
var parseInteger = /* @__PURE__ */ __name((str) => {
  const parsedIndex = Number.parseInt(str);
  return Number.isNaN(parsedIndex) ? none2() : some2(parsedIndex);
}, "parseInteger");

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/internal/defaultServices/console.js
var TypeId11 = /* @__PURE__ */ Symbol.for("effect/Console");
var consoleTag = /* @__PURE__ */ GenericTag("effect/Console");
var defaultConsole = {
  [TypeId11]: TypeId11,
  assert(condition, ...args2) {
    return sync(() => {
      console.assert(condition, ...args2);
    });
  },
  clear: /* @__PURE__ */ sync(() => {
    console.clear();
  }),
  count(label) {
    return sync(() => {
      console.count(label);
    });
  },
  countReset(label) {
    return sync(() => {
      console.countReset(label);
    });
  },
  debug(...args2) {
    return sync(() => {
      console.debug(...args2);
    });
  },
  dir(item, options) {
    return sync(() => {
      console.dir(item, options);
    });
  },
  dirxml(...args2) {
    return sync(() => {
      console.dirxml(...args2);
    });
  },
  error(...args2) {
    return sync(() => {
      console.error(...args2);
    });
  },
  group(options) {
    return options?.collapsed ? sync(() => console.groupCollapsed(options?.label)) : sync(() => console.group(options?.label));
  },
  groupEnd: /* @__PURE__ */ sync(() => {
    console.groupEnd();
  }),
  info(...args2) {
    return sync(() => {
      console.info(...args2);
    });
  },
  log(...args2) {
    return sync(() => {
      console.log(...args2);
    });
  },
  table(tabularData, properties) {
    return sync(() => {
      console.table(tabularData, properties);
    });
  },
  time(label) {
    return sync(() => console.time(label));
  },
  timeEnd(label) {
    return sync(() => console.timeEnd(label));
  },
  timeLog(label, ...args2) {
    return sync(() => {
      console.timeLog(label, ...args2);
    });
  },
  trace(...args2) {
    return sync(() => {
      console.trace(...args2);
    });
  },
  warn(...args2) {
    return sync(() => {
      console.warn(...args2);
    });
  },
  unsafe: console
};

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/internal/random.js
var RandomSymbolKey = "effect/Random";
var RandomTypeId = /* @__PURE__ */ Symbol.for(RandomSymbolKey);
var randomTag = /* @__PURE__ */ GenericTag("effect/Random");
var RandomImpl = class {
  static {
    __name(this, "RandomImpl");
  }
  seed;
  [RandomTypeId] = RandomTypeId;
  PRNG;
  constructor(seed) {
    this.seed = seed;
    this.PRNG = new PCGRandom(seed);
  }
  get next() {
    return sync(() => this.PRNG.number());
  }
  get nextBoolean() {
    return map8(this.next, (n) => n > 0.5);
  }
  get nextInt() {
    return sync(() => this.PRNG.integer(Number.MAX_SAFE_INTEGER));
  }
  nextRange(min4, max6) {
    return map8(this.next, (n) => (max6 - min4) * n + min4);
  }
  nextIntBetween(min4, max6) {
    return sync(() => this.PRNG.integer(max6 - min4) + min4);
  }
  shuffle(elements) {
    return shuffleWith(elements, (n) => this.nextIntBetween(0, n));
  }
};
var shuffleWith = /* @__PURE__ */ __name((elements, nextIntBounded) => {
  return suspend(() => pipe(sync(() => Array.from(elements)), flatMap7((buffer) => {
    const numbers = [];
    for (let i = buffer.length; i >= 2; i = i - 1) {
      numbers.push(i);
    }
    return pipe(numbers, forEachSequentialDiscard((n) => pipe(nextIntBounded(n), map8((k) => swap(buffer, n - 1, k)))), as2(fromIterable2(buffer)));
  })));
}, "shuffleWith");
var swap = /* @__PURE__ */ __name((buffer, index1, index2) => {
  const tmp = buffer[index1];
  buffer[index1] = buffer[index2];
  buffer[index2] = tmp;
  return buffer;
}, "swap");
var make22 = /* @__PURE__ */ __name((seed) => new RandomImpl(hash(seed)), "make");
var FixedRandomImpl = class {
  static {
    __name(this, "FixedRandomImpl");
  }
  values;
  [RandomTypeId] = RandomTypeId;
  index = 0;
  constructor(values3) {
    this.values = values3;
    if (values3.length === 0) {
      throw new Error("Requires at least one value");
    }
  }
  getNextValue() {
    const value = this.values[this.index];
    this.index = (this.index + 1) % this.values.length;
    return value;
  }
  get next() {
    return sync(() => {
      const value = this.getNextValue();
      if (typeof value === "number") {
        return Math.max(0, Math.min(1, value));
      }
      return hash(value) / 2147483647;
    });
  }
  get nextBoolean() {
    return sync(() => {
      const value = this.getNextValue();
      if (typeof value === "boolean") {
        return value;
      }
      return hash(value) % 2 === 0;
    });
  }
  get nextInt() {
    return sync(() => {
      const value = this.getNextValue();
      if (typeof value === "number" && Number.isFinite(value)) {
        return Math.round(value);
      }
      return Math.abs(hash(value));
    });
  }
  nextRange(min4, max6) {
    return map8(this.next, (n) => (max6 - min4) * n + min4);
  }
  nextIntBetween(min4, max6) {
    return sync(() => {
      const value = this.getNextValue();
      if (typeof value === "number" && Number.isFinite(value)) {
        return Math.max(min4, Math.min(max6 - 1, Math.round(value)));
      }
      const hash2 = Math.abs(hash(value));
      return min4 + hash2 % (max6 - min4);
    });
  }
  shuffle(elements) {
    return shuffleWith(elements, (n) => this.nextIntBetween(0, n));
  }
};
var fixed = /* @__PURE__ */ __name((values3) => new FixedRandomImpl(values3), "fixed");

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/internal/tracer.js
var TracerTypeId = /* @__PURE__ */ Symbol.for("effect/Tracer");
var make23 = /* @__PURE__ */ __name((options) => ({
  [TracerTypeId]: TracerTypeId,
  ...options
}), "make");
var tracerTag = /* @__PURE__ */ GenericTag("effect/Tracer");
var spanTag = /* @__PURE__ */ GenericTag("effect/ParentSpan");
var randomHexString = /* @__PURE__ */ (function() {
  const characters = "abcdef0123456789";
  const charactersLength = characters.length;
  return function(length2) {
    let result = "";
    for (let i = 0; i < length2; i++) {
      result += characters.charAt(Math.floor(Math.random() * charactersLength));
    }
    return result;
  };
})();
var NativeSpan = class {
  static {
    __name(this, "NativeSpan");
  }
  name;
  parent;
  context;
  startTime;
  kind;
  _tag = "Span";
  spanId;
  traceId = "native";
  sampled = true;
  status;
  attributes;
  events = [];
  links;
  constructor(name, parent, context4, links, startTime, kind) {
    this.name = name;
    this.parent = parent;
    this.context = context4;
    this.startTime = startTime;
    this.kind = kind;
    this.status = {
      _tag: "Started",
      startTime
    };
    this.attributes = /* @__PURE__ */ new Map();
    this.traceId = parent._tag === "Some" ? parent.value.traceId : randomHexString(32);
    this.spanId = randomHexString(16);
    this.links = Array.from(links);
  }
  end(endTime, exit4) {
    this.status = {
      _tag: "Ended",
      endTime,
      exit: exit4,
      startTime: this.status.startTime
    };
  }
  attribute(key, value) {
    this.attributes.set(key, value);
  }
  event(name, startTime, attributes) {
    this.events.push([name, startTime, attributes ?? {}]);
  }
  addLinks(links) {
    this.links.push(...links);
  }
};
var nativeTracer = /* @__PURE__ */ make23({
  span: /* @__PURE__ */ __name((name, parent, context4, links, startTime, kind) => new NativeSpan(name, parent, context4, links, startTime, kind), "span"),
  context: /* @__PURE__ */ __name((f) => f(), "context")
});
var addSpanStackTrace = /* @__PURE__ */ __name((options) => {
  if (options?.captureStackTrace === false) {
    return options;
  } else if (options?.captureStackTrace !== void 0 && typeof options.captureStackTrace !== "boolean") {
    return options;
  }
  const limit = Error.stackTraceLimit;
  Error.stackTraceLimit = 3;
  const traceError = new Error();
  Error.stackTraceLimit = limit;
  let cache = false;
  return {
    ...options,
    captureStackTrace: /* @__PURE__ */ __name(() => {
      if (cache !== false) {
        return cache;
      }
      if (traceError.stack !== void 0) {
        const stack = traceError.stack.split("\n");
        if (stack[3] !== void 0) {
          cache = stack[3].trim();
          return cache;
        }
      }
    }, "captureStackTrace")
  };
}, "addSpanStackTrace");
var DisablePropagation = /* @__PURE__ */ Reference2()("effect/Tracer/DisablePropagation", {
  defaultValue: constFalse
});

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/internal/defaultServices.js
var liveServices = /* @__PURE__ */ pipe(/* @__PURE__ */ empty3(), /* @__PURE__ */ add2(clockTag, /* @__PURE__ */ make19()), /* @__PURE__ */ add2(consoleTag, defaultConsole), /* @__PURE__ */ add2(randomTag, /* @__PURE__ */ make22(/* @__PURE__ */ Math.random())), /* @__PURE__ */ add2(configProviderTag, /* @__PURE__ */ fromEnv()), /* @__PURE__ */ add2(tracerTag, nativeTracer));
var currentServices = /* @__PURE__ */ globalValue(/* @__PURE__ */ Symbol.for("effect/DefaultServices/currentServices"), () => fiberRefUnsafeMakeContext(liveServices));
var sleep = /* @__PURE__ */ __name((duration) => {
  const decodedDuration = decode(duration);
  return clockWith((clock3) => clock3.sleep(decodedDuration));
}, "sleep");
var defaultServicesWith = /* @__PURE__ */ __name((f) => withFiberRuntime((fiber) => f(fiber.currentDefaultServices)), "defaultServicesWith");
var clockWith = /* @__PURE__ */ __name((f) => defaultServicesWith((services) => f(services.unsafeMap.get(clockTag.key))), "clockWith");
var currentTimeMillis = /* @__PURE__ */ clockWith((clock3) => clock3.currentTimeMillis);
var currentTimeNanos = /* @__PURE__ */ clockWith((clock3) => clock3.currentTimeNanos);
var withClock = /* @__PURE__ */ dual(2, (effect, c) => fiberRefLocallyWith(currentServices, add2(clockTag, c))(effect));
var withConfigProvider = /* @__PURE__ */ dual(2, (self, provider) => fiberRefLocallyWith(currentServices, add2(configProviderTag, provider))(self));
var configProviderWith = /* @__PURE__ */ __name((f) => defaultServicesWith((services) => f(services.unsafeMap.get(configProviderTag.key))), "configProviderWith");
var randomWith = /* @__PURE__ */ __name((f) => defaultServicesWith((services) => f(services.unsafeMap.get(randomTag.key))), "randomWith");
var withRandom = /* @__PURE__ */ dual(2, (effect, value) => fiberRefLocallyWith(currentServices, add2(randomTag, value))(effect));
var tracerWith = /* @__PURE__ */ __name((f) => defaultServicesWith((services) => f(services.unsafeMap.get(tracerTag.key))), "tracerWith");
var withTracer = /* @__PURE__ */ dual(2, (effect, value) => fiberRefLocallyWith(currentServices, add2(tracerTag, value))(effect));

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/Clock.js
var sleep2 = sleep;
var currentTimeMillis2 = currentTimeMillis;
var currentTimeNanos2 = currentTimeNanos;
var clockWith2 = clockWith;
var Clock = clockTag;

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/internal/fiberRefs.js
function unsafeMake4(fiberRefLocals) {
  return new FiberRefsImpl(fiberRefLocals);
}
__name(unsafeMake4, "unsafeMake");
function empty20() {
  return unsafeMake4(/* @__PURE__ */ new Map());
}
__name(empty20, "empty");
var FiberRefsSym = /* @__PURE__ */ Symbol.for("effect/FiberRefs");
var FiberRefsImpl = class {
  static {
    __name(this, "FiberRefsImpl");
  }
  locals;
  [FiberRefsSym] = FiberRefsSym;
  constructor(locals) {
    this.locals = locals;
  }
  pipe() {
    return pipeArguments(this, arguments);
  }
};
var findAncestor = /* @__PURE__ */ __name((_ref, _parentStack, _childStack, _childModified = false) => {
  const ref = _ref;
  let parentStack = _parentStack;
  let childStack = _childStack;
  let childModified = _childModified;
  let ret = void 0;
  while (ret === void 0) {
    if (isNonEmptyReadonlyArray(parentStack) && isNonEmptyReadonlyArray(childStack)) {
      const parentFiberId = headNonEmpty(parentStack)[0];
      const parentAncestors = tailNonEmpty(parentStack);
      const childFiberId = headNonEmpty(childStack)[0];
      const childRefValue = headNonEmpty(childStack)[1];
      const childAncestors = tailNonEmpty(childStack);
      if (parentFiberId.startTimeMillis < childFiberId.startTimeMillis) {
        childStack = childAncestors;
        childModified = true;
      } else if (parentFiberId.startTimeMillis > childFiberId.startTimeMillis) {
        parentStack = parentAncestors;
      } else {
        if (parentFiberId.id < childFiberId.id) {
          childStack = childAncestors;
          childModified = true;
        } else if (parentFiberId.id > childFiberId.id) {
          parentStack = parentAncestors;
        } else {
          ret = [childRefValue, childModified];
        }
      }
    } else {
      ret = [ref.initial, true];
    }
  }
  return ret;
}, "findAncestor");
var joinAs = /* @__PURE__ */ dual(3, (self, fiberId3, that) => {
  const parentFiberRefs = new Map(self.locals);
  that.locals.forEach((childStack, fiberRef) => {
    const childValue = childStack[0][1];
    if (!childStack[0][0][symbol2](fiberId3)) {
      if (!parentFiberRefs.has(fiberRef)) {
        if (equals(childValue, fiberRef.initial)) {
          return;
        }
        parentFiberRefs.set(fiberRef, [[fiberId3, fiberRef.join(fiberRef.initial, childValue)]]);
        return;
      }
      const parentStack = parentFiberRefs.get(fiberRef);
      const [ancestor, wasModified] = findAncestor(fiberRef, parentStack, childStack);
      if (wasModified) {
        const patch9 = fiberRef.diff(ancestor, childValue);
        const oldValue = parentStack[0][1];
        const newValue = fiberRef.join(oldValue, fiberRef.patch(patch9)(oldValue));
        if (!equals(oldValue, newValue)) {
          let newStack;
          const parentFiberId = parentStack[0][0];
          if (parentFiberId[symbol2](fiberId3)) {
            newStack = [[parentFiberId, newValue], ...parentStack.slice(1)];
          } else {
            newStack = [[fiberId3, newValue], ...parentStack];
          }
          parentFiberRefs.set(fiberRef, newStack);
        }
      }
    }
  });
  return new FiberRefsImpl(parentFiberRefs);
});
var forkAs = /* @__PURE__ */ dual(2, (self, childId) => {
  const map14 = /* @__PURE__ */ new Map();
  unsafeForkAs(self, map14, childId);
  return new FiberRefsImpl(map14);
});
var unsafeForkAs = /* @__PURE__ */ __name((self, map14, fiberId3) => {
  self.locals.forEach((stack, fiberRef) => {
    const oldValue = stack[0][1];
    const newValue = fiberRef.patch(fiberRef.fork)(oldValue);
    if (equals(oldValue, newValue)) {
      map14.set(fiberRef, stack);
    } else {
      map14.set(fiberRef, [[fiberId3, newValue], ...stack]);
    }
  });
}, "unsafeForkAs");
var fiberRefs = /* @__PURE__ */ __name((self) => fromIterable5(self.locals.keys()), "fiberRefs");
var setAll = /* @__PURE__ */ __name((self) => forEachSequentialDiscard(fiberRefs(self), (fiberRef) => fiberRefSet(fiberRef, getOrDefault(self, fiberRef))), "setAll");
var delete_ = /* @__PURE__ */ dual(2, (self, fiberRef) => {
  const locals = new Map(self.locals);
  locals.delete(fiberRef);
  return new FiberRefsImpl(locals);
});
var get9 = /* @__PURE__ */ dual(2, (self, fiberRef) => {
  if (!self.locals.has(fiberRef)) {
    return none2();
  }
  return some2(headNonEmpty(self.locals.get(fiberRef))[1]);
});
var getOrDefault = /* @__PURE__ */ dual(2, (self, fiberRef) => pipe(get9(self, fiberRef), getOrElse(() => fiberRef.initial)));
var updateAs = /* @__PURE__ */ dual(2, (self, {
  fiberId: fiberId3,
  fiberRef,
  value
}) => {
  if (self.locals.size === 0) {
    return new FiberRefsImpl(/* @__PURE__ */ new Map([[fiberRef, [[fiberId3, value]]]]));
  }
  const locals = new Map(self.locals);
  unsafeUpdateAs(locals, fiberId3, fiberRef, value);
  return new FiberRefsImpl(locals);
});
var unsafeUpdateAs = /* @__PURE__ */ __name((locals, fiberId3, fiberRef, value) => {
  const oldStack = locals.get(fiberRef) ?? [];
  let newStack;
  if (isNonEmptyReadonlyArray(oldStack)) {
    const [currentId, currentValue] = headNonEmpty(oldStack);
    if (currentId[symbol2](fiberId3)) {
      if (equals(currentValue, value)) {
        return;
      } else {
        newStack = [[fiberId3, value], ...oldStack.slice(1)];
      }
    } else {
      newStack = [[fiberId3, value], ...oldStack];
    }
  } else {
    newStack = [[fiberId3, value]];
  }
  locals.set(fiberRef, newStack);
}, "unsafeUpdateAs");
var updateManyAs = /* @__PURE__ */ dual(2, (self, {
  entries: entries2,
  forkAs: forkAs2
}) => {
  if (self.locals.size === 0) {
    return new FiberRefsImpl(new Map(entries2));
  }
  const locals = new Map(self.locals);
  if (forkAs2 !== void 0) {
    unsafeForkAs(self, locals, forkAs2);
  }
  entries2.forEach(([fiberRef, values3]) => {
    if (values3.length === 1) {
      unsafeUpdateAs(locals, values3[0][0], fiberRef, values3[0][1]);
    } else {
      values3.forEach(([fiberId3, value]) => {
        unsafeUpdateAs(locals, fiberId3, fiberRef, value);
      });
    }
  });
  return new FiberRefsImpl(locals);
});

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/FiberRefs.js
var get10 = get9;
var getOrDefault2 = getOrDefault;
var joinAs2 = joinAs;
var setAll2 = setAll;
var updateManyAs2 = updateManyAs;
var empty21 = empty20;

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/LogLevel.js
var All = logLevelAll;
var Fatal = logLevelFatal;
var Error2 = logLevelError;
var Warning = logLevelWarning;
var Info = logLevelInfo;
var Debug = logLevelDebug;
var Trace = logLevelTrace;
var None3 = logLevelNone;
var Order3 = /* @__PURE__ */ pipe(Order, /* @__PURE__ */ mapInput2((level) => level.ordinal));
var greaterThan3 = /* @__PURE__ */ greaterThan(Order3);
var fromLiteral = /* @__PURE__ */ __name((literal) => {
  switch (literal) {
    case "All":
      return All;
    case "Debug":
      return Debug;
    case "Error":
      return Error2;
    case "Fatal":
      return Fatal;
    case "Info":
      return Info;
    case "Trace":
      return Trace;
    case "None":
      return None3;
    case "Warning":
      return Warning;
  }
}, "fromLiteral");

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/internal/logSpan.js
var make24 = /* @__PURE__ */ __name((label, startTime) => ({
  label,
  startTime
}), "make");
var formatLabel = /* @__PURE__ */ __name((key) => key.replace(/[\s="]/g, "_"), "formatLabel");
var render = /* @__PURE__ */ __name((now) => (self) => {
  const label = formatLabel(self.label);
  return `${label}=${now - self.startTime}ms`;
}, "render");

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/LogSpan.js
var make25 = make24;

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/Effectable.js
var EffectPrototype2 = EffectPrototype;
var CommitPrototype2 = CommitPrototype;
var Base2 = Base;
var Class2 = class extends Base2 {
  static {
    __name(this, "Class");
  }
};

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/Readable.js
var TypeId12 = /* @__PURE__ */ Symbol.for("effect/Readable");
var Proto = {
  [TypeId12]: TypeId12,
  pipe() {
    return pipeArguments(this, arguments);
  }
};

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/internal/ref.js
var RefTypeId = /* @__PURE__ */ Symbol.for("effect/Ref");
var refVariance = {
  /* c8 ignore next */
  _A: /* @__PURE__ */ __name((_) => _, "_A")
};
var RefImpl = class extends Class2 {
  static {
    __name(this, "RefImpl");
  }
  ref;
  commit() {
    return this.get;
  }
  [RefTypeId] = refVariance;
  [TypeId12] = TypeId12;
  constructor(ref) {
    super();
    this.ref = ref;
    this.get = sync(() => get6(this.ref));
  }
  get;
  modify(f) {
    return sync(() => {
      const current = get6(this.ref);
      const [b, a] = f(current);
      if (current !== a) {
        set2(a)(this.ref);
      }
      return b;
    });
  }
};
var unsafeMake5 = /* @__PURE__ */ __name((value) => new RefImpl(make11(value)), "unsafeMake");
var make26 = /* @__PURE__ */ __name((value) => sync(() => unsafeMake5(value)), "make");
var get11 = /* @__PURE__ */ __name((self) => self.get, "get");
var set5 = /* @__PURE__ */ dual(2, (self, value) => self.modify(() => [void 0, value]));
var getAndSet = /* @__PURE__ */ dual(2, (self, value) => self.modify((a) => [a, value]));
var modify3 = /* @__PURE__ */ dual(2, (self, f) => self.modify(f));
var update2 = /* @__PURE__ */ dual(2, (self, f) => self.modify((a) => [void 0, f(a)]));

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/Ref.js
var make27 = make26;
var get12 = get11;
var getAndSet2 = getAndSet;
var update3 = update2;

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/Tracer.js
var tracerWith2 = tracerWith;

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/internal/fiberRefs/patch.js
var OP_EMPTY2 = "Empty";
var OP_ADD = "Add";
var OP_REMOVE = "Remove";
var OP_UPDATE = "Update";
var OP_AND_THEN = "AndThen";
var empty22 = {
  _tag: OP_EMPTY2
};
var diff5 = /* @__PURE__ */ __name((oldValue, newValue) => {
  const missingLocals = new Map(oldValue.locals);
  let patch9 = empty22;
  for (const [fiberRef, pairs] of newValue.locals.entries()) {
    const newValue2 = headNonEmpty(pairs)[1];
    const old = missingLocals.get(fiberRef);
    if (old !== void 0) {
      const oldValue2 = headNonEmpty(old)[1];
      if (!equals(oldValue2, newValue2)) {
        patch9 = combine7({
          _tag: OP_UPDATE,
          fiberRef,
          patch: fiberRef.diff(oldValue2, newValue2)
        })(patch9);
      }
    } else {
      patch9 = combine7({
        _tag: OP_ADD,
        fiberRef,
        value: newValue2
      })(patch9);
    }
    missingLocals.delete(fiberRef);
  }
  for (const [fiberRef] of missingLocals.entries()) {
    patch9 = combine7({
      _tag: OP_REMOVE,
      fiberRef
    })(patch9);
  }
  return patch9;
}, "diff");
var combine7 = /* @__PURE__ */ dual(2, (self, that) => ({
  _tag: OP_AND_THEN,
  first: self,
  second: that
}));
var patch6 = /* @__PURE__ */ dual(3, (self, fiberId3, oldValue) => {
  let fiberRefs3 = oldValue;
  let patches = of(self);
  while (isNonEmptyReadonlyArray(patches)) {
    const head5 = headNonEmpty(patches);
    const tail = tailNonEmpty(patches);
    switch (head5._tag) {
      case OP_EMPTY2: {
        patches = tail;
        break;
      }
      case OP_ADD: {
        fiberRefs3 = updateAs(fiberRefs3, {
          fiberId: fiberId3,
          fiberRef: head5.fiberRef,
          value: head5.value
        });
        patches = tail;
        break;
      }
      case OP_REMOVE: {
        fiberRefs3 = delete_(fiberRefs3, head5.fiberRef);
        patches = tail;
        break;
      }
      case OP_UPDATE: {
        const value = getOrDefault(fiberRefs3, head5.fiberRef);
        fiberRefs3 = updateAs(fiberRefs3, {
          fiberId: fiberId3,
          fiberRef: head5.fiberRef,
          value: head5.fiberRef.patch(head5.patch)(value)
        });
        patches = tail;
        break;
      }
      case OP_AND_THEN: {
        patches = prepend(head5.first)(prepend(head5.second)(tail));
        break;
      }
    }
  }
  return fiberRefs3;
});

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/internal/metric/label.js
var MetricLabelSymbolKey = "effect/MetricLabel";
var MetricLabelTypeId = /* @__PURE__ */ Symbol.for(MetricLabelSymbolKey);
var MetricLabelImpl = class {
  static {
    __name(this, "MetricLabelImpl");
  }
  key;
  value;
  [MetricLabelTypeId] = MetricLabelTypeId;
  _hash;
  constructor(key, value) {
    this.key = key;
    this.value = value;
    this._hash = string(MetricLabelSymbolKey + this.key + this.value);
  }
  [symbol]() {
    return this._hash;
  }
  [symbol2](that) {
    return isMetricLabel(that) && this.key === that.key && this.value === that.value;
  }
  pipe() {
    return pipeArguments(this, arguments);
  }
};
var make28 = /* @__PURE__ */ __name((key, value) => {
  return new MetricLabelImpl(key, value);
}, "make");
var isMetricLabel = /* @__PURE__ */ __name((u) => hasProperty(u, MetricLabelTypeId), "isMetricLabel");

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/internal/core-effect.js
var annotateLogs = /* @__PURE__ */ dual((args2) => isEffect(args2[0]), function() {
  const args2 = arguments;
  return fiberRefLocallyWith(args2[0], currentLogAnnotations, typeof args2[1] === "string" ? set3(args2[1], args2[2]) : (annotations) => Object.entries(args2[1]).reduce((acc, [key, value]) => set3(acc, key, value), annotations));
});
var asSome = /* @__PURE__ */ __name((self) => map8(self, some2), "asSome");
var asSomeError = /* @__PURE__ */ __name((self) => mapError(self, some2), "asSomeError");
var try_ = /* @__PURE__ */ __name((arg) => {
  let evaluate2;
  let onFailure = void 0;
  if (typeof arg === "function") {
    evaluate2 = arg;
  } else {
    evaluate2 = arg.try;
    onFailure = arg.catch;
  }
  return suspend(() => {
    try {
      return succeed(internalCall(evaluate2));
    } catch (error) {
      return fail2(onFailure ? internalCall(() => onFailure(error)) : new UnknownException(error, "An unknown error occurred in Effect.try"));
    }
  });
}, "try_");
var _catch = /* @__PURE__ */ dual(3, (self, tag, options) => catchAll(self, (e) => {
  if (hasProperty(e, tag) && e[tag] === options.failure) {
    return options.onFailure(e);
  }
  return fail2(e);
}));
var catchAllDefect = /* @__PURE__ */ dual(2, (self, f) => catchAllCause(self, (cause3) => {
  const option3 = find(cause3, (_) => isDieType(_) ? some2(_) : none2());
  switch (option3._tag) {
    case "None": {
      return failCause(cause3);
    }
    case "Some": {
      return f(option3.value.defect);
    }
  }
}));
var catchSomeCause = /* @__PURE__ */ dual(2, (self, f) => matchCauseEffect(self, {
  onFailure: /* @__PURE__ */ __name((cause3) => {
    const option3 = f(cause3);
    switch (option3._tag) {
      case "None": {
        return failCause(cause3);
      }
      case "Some": {
        return option3.value;
      }
    }
  }, "onFailure"),
  onSuccess: succeed
}));
var catchSomeDefect = /* @__PURE__ */ dual(2, (self, pf) => catchAllCause(self, (cause3) => {
  const option3 = find(cause3, (_) => isDieType(_) ? some2(_) : none2());
  switch (option3._tag) {
    case "None": {
      return failCause(cause3);
    }
    case "Some": {
      const optionEffect = pf(option3.value.defect);
      return optionEffect._tag === "Some" ? optionEffect.value : failCause(cause3);
    }
  }
}));
var catchTag = /* @__PURE__ */ dual((args2) => isEffect(args2[0]), (self, ...args2) => {
  const f = args2[args2.length - 1];
  let predicate;
  if (args2.length === 2) {
    predicate = isTagged(args2[0]);
  } else {
    predicate = /* @__PURE__ */ __name((e) => {
      const tag = hasProperty(e, "_tag") ? e["_tag"] : void 0;
      if (!tag) return false;
      for (let i = 0; i < args2.length - 1; i++) {
        if (args2[i] === tag) return true;
      }
      return false;
    }, "predicate");
  }
  return catchIf(self, predicate, f);
});
var catchTags = /* @__PURE__ */ dual(2, (self, cases) => {
  let keys5;
  return catchIf(self, (e) => {
    keys5 ??= Object.keys(cases);
    return hasProperty(e, "_tag") && isString(e["_tag"]) && keys5.includes(e["_tag"]);
  }, (e) => cases[e["_tag"]](e));
});
var cause = /* @__PURE__ */ __name((self) => matchCause(self, {
  onFailure: identity,
  onSuccess: /* @__PURE__ */ __name(() => empty16, "onSuccess")
}), "cause");
var clockWith3 = clockWith2;
var clock = /* @__PURE__ */ clockWith3(succeed);
var delay = /* @__PURE__ */ dual(2, (self, duration) => zipRight(sleep2(duration), self));
var descriptorWith = /* @__PURE__ */ __name((f) => withFiberRuntime((state, status) => f({
  id: state.id(),
  status,
  interruptors: interruptors(state.getFiberRef(currentInterruptedCause))
})), "descriptorWith");
var allowInterrupt = /* @__PURE__ */ descriptorWith((descriptor3) => size3(descriptor3.interruptors) > 0 ? interrupt2 : void_);
var descriptor = /* @__PURE__ */ descriptorWith(succeed);
var diffFiberRefs = /* @__PURE__ */ __name((self) => summarized(self, fiberRefs2, diff5), "diffFiberRefs");
var diffFiberRefsAndRuntimeFlags = /* @__PURE__ */ __name((self) => summarized(self, zip2(fiberRefs2, runtimeFlags), ([refs, flags], [refsNew, flagsNew]) => [diff5(refs, refsNew), diff4(flags, flagsNew)]), "diffFiberRefsAndRuntimeFlags");
var Do = /* @__PURE__ */ succeed({});
var bind2 = /* @__PURE__ */ bind(map8, flatMap7);
var bindTo2 = /* @__PURE__ */ bindTo(map8);
var let_2 = /* @__PURE__ */ let_(map8);
var dropUntil = /* @__PURE__ */ dual(2, (elements, predicate) => suspend(() => {
  const iterator = elements[Symbol.iterator]();
  const builder = [];
  let next;
  let dropping = succeed(false);
  let i = 0;
  while ((next = iterator.next()) && !next.done) {
    const a = next.value;
    const index = i++;
    dropping = flatMap7(dropping, (bool) => {
      if (bool) {
        builder.push(a);
        return succeed(true);
      }
      return predicate(a, index);
    });
  }
  return map8(dropping, () => builder);
}));
var dropWhile = /* @__PURE__ */ dual(2, (elements, predicate) => suspend(() => {
  const iterator = elements[Symbol.iterator]();
  const builder = [];
  let next;
  let dropping = succeed(true);
  let i = 0;
  while ((next = iterator.next()) && !next.done) {
    const a = next.value;
    const index = i++;
    dropping = flatMap7(dropping, (d) => map8(d ? predicate(a, index) : succeed(false), (b) => {
      if (!b) {
        builder.push(a);
      }
      return b;
    }));
  }
  return map8(dropping, () => builder);
}));
var contextWith = /* @__PURE__ */ __name((f) => map8(context(), f), "contextWith");
var eventually = /* @__PURE__ */ __name((self) => orElse(self, () => flatMap7(yieldNow(), () => eventually(self))), "eventually");
var filterMap3 = /* @__PURE__ */ dual(2, (elements, pf) => map8(forEachSequential(elements, identity), filterMap(pf)));
var filterOrDie = /* @__PURE__ */ dual(3, (self, predicate, orDieWith3) => filterOrElse(self, predicate, (a) => dieSync(() => orDieWith3(a))));
var filterOrDieMessage = /* @__PURE__ */ dual(3, (self, predicate, message) => filterOrElse(self, predicate, () => dieMessage(message)));
var filterOrElse = /* @__PURE__ */ dual(3, (self, predicate, orElse3) => flatMap7(self, (a) => predicate(a) ? succeed(a) : orElse3(a)));
var liftPredicate = /* @__PURE__ */ dual(3, (self, predicate, orFailWith) => suspend(() => predicate(self) ? succeed(self) : fail2(orFailWith(self))));
var filterOrFail = /* @__PURE__ */ dual((args2) => isEffect(args2[0]), (self, predicate, orFailWith) => filterOrElse(self, predicate, (a) => orFailWith === void 0 ? fail2(new NoSuchElementException()) : failSync(() => orFailWith(a))));
var findFirst3 = /* @__PURE__ */ dual(2, (elements, predicate) => suspend(() => {
  const iterator = elements[Symbol.iterator]();
  const next = iterator.next();
  if (!next.done) {
    return findLoop(iterator, 0, predicate, next.value);
  }
  return succeed(none2());
}));
var findLoop = /* @__PURE__ */ __name((iterator, index, f, value) => flatMap7(f(value, index), (result) => {
  if (result) {
    return succeed(some2(value));
  }
  const next = iterator.next();
  if (!next.done) {
    return findLoop(iterator, index + 1, f, next.value);
  }
  return succeed(none2());
}), "findLoop");
var firstSuccessOf = /* @__PURE__ */ __name((effects) => suspend(() => {
  const list = fromIterable2(effects);
  if (!isNonEmpty(list)) {
    return dieSync(() => new IllegalArgumentException(`Received an empty collection of effects`));
  }
  return pipe(tailNonEmpty2(list), reduce(headNonEmpty2(list), (left3, right3) => orElse(left3, () => right3)));
}), "firstSuccessOf");
var flipWith = /* @__PURE__ */ dual(2, (self, f) => flip(f(flip(self))));
var match7 = /* @__PURE__ */ dual(2, (self, options) => matchEffect(self, {
  onFailure: /* @__PURE__ */ __name((e) => succeed(options.onFailure(e)), "onFailure"),
  onSuccess: /* @__PURE__ */ __name((a) => succeed(options.onSuccess(a)), "onSuccess")
}));
var every4 = /* @__PURE__ */ dual(2, (elements, predicate) => suspend(() => forAllLoop(elements[Symbol.iterator](), 0, predicate)));
var forAllLoop = /* @__PURE__ */ __name((iterator, index, f) => {
  const next = iterator.next();
  return next.done ? succeed(true) : flatMap7(f(next.value, index), (b) => b ? forAllLoop(iterator, index + 1, f) : succeed(b));
}, "forAllLoop");
var forever = /* @__PURE__ */ __name((self) => {
  const loop3 = flatMap7(flatMap7(self, () => yieldNow()), () => loop3);
  return loop3;
}, "forever");
var fiberRefs2 = /* @__PURE__ */ withFiberRuntime((state) => succeed(state.getFiberRefs()));
var head3 = /* @__PURE__ */ __name((self) => flatMap7(self, (as7) => {
  const iterator = as7[Symbol.iterator]();
  const next = iterator.next();
  if (next.done) {
    return fail2(new NoSuchElementException());
  }
  return succeed(next.value);
}), "head");
var ignore = /* @__PURE__ */ __name((self) => match7(self, {
  onFailure: constVoid,
  onSuccess: constVoid
}), "ignore");
var ignoreLogged = /* @__PURE__ */ __name((self) => matchCauseEffect(self, {
  onFailure: /* @__PURE__ */ __name((cause3) => logDebug(cause3, "An error was silently ignored because it is not anticipated to be useful"), "onFailure"),
  onSuccess: /* @__PURE__ */ __name(() => void_, "onSuccess")
}), "ignoreLogged");
var inheritFiberRefs = /* @__PURE__ */ __name((childFiberRefs) => updateFiberRefs((parentFiberId, parentFiberRefs) => joinAs2(parentFiberRefs, parentFiberId, childFiberRefs)), "inheritFiberRefs");
var isFailure3 = /* @__PURE__ */ __name((self) => match7(self, {
  onFailure: constTrue,
  onSuccess: constFalse
}), "isFailure");
var isSuccess2 = /* @__PURE__ */ __name((self) => match7(self, {
  onFailure: constFalse,
  onSuccess: constTrue
}), "isSuccess");
var iterate = /* @__PURE__ */ __name((initial, options) => suspend(() => {
  if (options.while(initial)) {
    return flatMap7(options.body(initial), (z2) => iterate(z2, options));
  }
  return succeed(initial);
}), "iterate");
var logWithLevel = /* @__PURE__ */ __name((level) => (...message) => {
  const levelOption = fromNullable(level);
  let cause3 = void 0;
  for (let i = 0, len = message.length; i < len; i++) {
    const msg = message[i];
    if (isCause(msg)) {
      if (cause3 !== void 0) {
        cause3 = sequential(cause3, msg);
      } else {
        cause3 = msg;
      }
      message = [...message.slice(0, i), ...message.slice(i + 1)];
      i--;
    }
  }
  if (cause3 === void 0) {
    cause3 = empty16;
  }
  return withFiberRuntime((fiberState) => {
    fiberState.log(message, cause3, levelOption);
    return void_;
  });
}, "logWithLevel");
var log = /* @__PURE__ */ logWithLevel();
var logTrace = /* @__PURE__ */ logWithLevel(Trace);
var logDebug = /* @__PURE__ */ logWithLevel(Debug);
var logInfo = /* @__PURE__ */ logWithLevel(Info);
var logWarning = /* @__PURE__ */ logWithLevel(Warning);
var logError = /* @__PURE__ */ logWithLevel(Error2);
var logFatal = /* @__PURE__ */ logWithLevel(Fatal);
var withLogSpan = /* @__PURE__ */ dual(2, (effect, label) => flatMap7(currentTimeMillis2, (now) => fiberRefLocallyWith(effect, currentLogSpan, prepend3(make25(label, now)))));
var logAnnotations = /* @__PURE__ */ fiberRefGet(currentLogAnnotations);
var loop = /* @__PURE__ */ __name((initial, options) => options.discard ? loopDiscard(initial, options.while, options.step, options.body) : map8(loopInternal(initial, options.while, options.step, options.body), fromIterable), "loop");
var loopInternal = /* @__PURE__ */ __name((initial, cont, inc, body) => suspend(() => cont(initial) ? flatMap7(body(initial), (a) => map8(loopInternal(inc(initial), cont, inc, body), prepend3(a))) : sync(() => empty9())), "loopInternal");
var loopDiscard = /* @__PURE__ */ __name((initial, cont, inc, body) => suspend(() => cont(initial) ? flatMap7(body(initial), () => loopDiscard(inc(initial), cont, inc, body)) : void_), "loopDiscard");
var mapAccum2 = /* @__PURE__ */ dual(3, (elements, initial, f) => suspend(() => {
  const iterator = elements[Symbol.iterator]();
  const builder = [];
  let result = succeed(initial);
  let next;
  let i = 0;
  while (!(next = iterator.next()).done) {
    const index = i++;
    const value = next.value;
    result = flatMap7(result, (state) => map8(f(state, value, index), ([z, b]) => {
      builder.push(b);
      return z;
    }));
  }
  return map8(result, (z) => [z, builder]);
}));
var mapErrorCause2 = /* @__PURE__ */ dual(2, (self, f) => matchCauseEffect(self, {
  onFailure: /* @__PURE__ */ __name((c) => failCauseSync(() => f(c)), "onFailure"),
  onSuccess: succeed
}));
var memoize = /* @__PURE__ */ __name((self) => pipe(deferredMake(), flatMap7((deferred) => pipe(diffFiberRefsAndRuntimeFlags(self), intoDeferred(deferred), once, map8((complete3) => zipRight(complete3, pipe(deferredAwait(deferred), flatMap7(([patch9, a]) => as2(zip2(patchFiberRefs(patch9[0]), updateRuntimeFlags(patch9[1])), a)))))))), "memoize");
var merge5 = /* @__PURE__ */ __name((self) => matchEffect(self, {
  onFailure: /* @__PURE__ */ __name((e) => succeed(e), "onFailure"),
  onSuccess: succeed
}), "merge");
var negate = /* @__PURE__ */ __name((self) => map8(self, (b) => !b), "negate");
var none6 = /* @__PURE__ */ __name((self) => flatMap7(self, (option3) => {
  switch (option3._tag) {
    case "None":
      return void_;
    case "Some":
      return fail2(new NoSuchElementException());
  }
}), "none");
var once = /* @__PURE__ */ __name((self) => map8(make27(true), (ref) => asVoid(whenEffect(self, getAndSet2(ref, false)))), "once");
var option = /* @__PURE__ */ __name((self) => matchEffect(self, {
  onFailure: /* @__PURE__ */ __name(() => succeed(none2()), "onFailure"),
  onSuccess: /* @__PURE__ */ __name((a) => succeed(some2(a)), "onSuccess")
}), "option");
var orElseFail = /* @__PURE__ */ dual(2, (self, evaluate2) => orElse(self, () => failSync(evaluate2)));
var orElseSucceed = /* @__PURE__ */ dual(2, (self, evaluate2) => orElse(self, () => sync(evaluate2)));
var parallelErrors = /* @__PURE__ */ __name((self) => matchCauseEffect(self, {
  onFailure: /* @__PURE__ */ __name((cause3) => {
    const errors = fromIterable(failures(cause3));
    return errors.length === 0 ? failCause(cause3) : fail2(errors);
  }, "onFailure"),
  onSuccess: succeed
}), "parallelErrors");
var patchFiberRefs = /* @__PURE__ */ __name((patch9) => updateFiberRefs((fiberId3, fiberRefs3) => pipe(patch9, patch6(fiberId3, fiberRefs3))), "patchFiberRefs");
var promise = /* @__PURE__ */ __name((evaluate2) => evaluate2.length >= 1 ? async_((resolve, signal) => {
  try {
    evaluate2(signal).then((a) => resolve(succeed(a)), (e) => resolve(die2(e)));
  } catch (e) {
    resolve(die2(e));
  }
}) : async_((resolve) => {
  try {
    ;
    evaluate2().then((a) => resolve(succeed(a)), (e) => resolve(die2(e)));
  } catch (e) {
    resolve(die2(e));
  }
}), "promise");
var provideService = /* @__PURE__ */ dual(3, (self, tag, service) => contextWithEffect((env) => provideContext(self, add2(env, tag, service))));
var provideServiceEffect = /* @__PURE__ */ dual(3, (self, tag, effect) => contextWithEffect((env) => flatMap7(effect, (service) => provideContext(self, pipe(env, add2(tag, service))))));
var random2 = /* @__PURE__ */ randomWith(succeed);
var reduce8 = /* @__PURE__ */ dual(3, (elements, zero2, f) => fromIterable(elements).reduce((acc, el, i) => flatMap7(acc, (a) => f(a, el, i)), succeed(zero2)));
var reduceRight2 = /* @__PURE__ */ dual(3, (elements, zero2, f) => fromIterable(elements).reduceRight((acc, el, i) => flatMap7(acc, (a) => f(el, a, i)), succeed(zero2)));
var reduceWhile = /* @__PURE__ */ dual(3, (elements, zero2, options) => flatMap7(sync(() => elements[Symbol.iterator]()), (iterator) => reduceWhileLoop(iterator, 0, zero2, options.while, options.body)));
var reduceWhileLoop = /* @__PURE__ */ __name((iterator, index, state, predicate, f) => {
  const next = iterator.next();
  if (!next.done && predicate(state)) {
    return flatMap7(f(state, next.value, index), (nextState) => reduceWhileLoop(iterator, index + 1, nextState, predicate, f));
  }
  return succeed(state);
}, "reduceWhileLoop");
var repeatN = /* @__PURE__ */ dual(2, (self, n) => suspend(() => repeatNLoop(self, n)));
var repeatNLoop = /* @__PURE__ */ __name((self, n) => flatMap7(self, (a) => n <= 0 ? succeed(a) : zipRight(yieldNow(), repeatNLoop(self, n - 1))), "repeatNLoop");
var sandbox = /* @__PURE__ */ __name((self) => matchCauseEffect(self, {
  onFailure: fail2,
  onSuccess: succeed
}), "sandbox");
var setFiberRefs = /* @__PURE__ */ __name((fiberRefs3) => suspend(() => setAll2(fiberRefs3)), "setFiberRefs");
var sleep3 = sleep2;
var succeedNone = /* @__PURE__ */ succeed(/* @__PURE__ */ none2());
var succeedSome = /* @__PURE__ */ __name((value) => succeed(some2(value)), "succeedSome");
var summarized = /* @__PURE__ */ dual(3, (self, summary5, f) => flatMap7(summary5, (start3) => flatMap7(self, (value) => map8(summary5, (end3) => [f(start3, end3), value]))));
var tagMetrics = /* @__PURE__ */ dual((args2) => isEffect(args2[0]), function() {
  return labelMetrics(arguments[0], typeof arguments[1] === "string" ? [make28(arguments[1], arguments[2])] : Object.entries(arguments[1]).map(([k, v]) => make28(k, v)));
});
var labelMetrics = /* @__PURE__ */ dual(2, (self, labels) => fiberRefLocallyWith(self, currentMetricLabels, (old) => union(old, labels)));
var takeUntil = /* @__PURE__ */ dual(2, (elements, predicate) => suspend(() => {
  const iterator = elements[Symbol.iterator]();
  const builder = [];
  let next;
  let effect = succeed(false);
  let i = 0;
  while ((next = iterator.next()) && !next.done) {
    const a = next.value;
    const index = i++;
    effect = flatMap7(effect, (bool) => {
      if (bool) {
        return succeed(true);
      }
      builder.push(a);
      return predicate(a, index);
    });
  }
  return map8(effect, () => builder);
}));
var takeWhile = /* @__PURE__ */ dual(2, (elements, predicate) => suspend(() => {
  const iterator = elements[Symbol.iterator]();
  const builder = [];
  let next;
  let taking = succeed(true);
  let i = 0;
  while ((next = iterator.next()) && !next.done) {
    const a = next.value;
    const index = i++;
    taking = flatMap7(taking, (taking2) => pipe(taking2 ? predicate(a, index) : succeed(false), map8((bool) => {
      if (bool) {
        builder.push(a);
      }
      return bool;
    })));
  }
  return map8(taking, () => builder);
}));
var tapBoth = /* @__PURE__ */ dual(2, (self, {
  onFailure,
  onSuccess
}) => matchCauseEffect(self, {
  onFailure: /* @__PURE__ */ __name((cause3) => {
    const either4 = failureOrCause(cause3);
    switch (either4._tag) {
      case "Left": {
        return zipRight(onFailure(either4.left), failCause(cause3));
      }
      case "Right": {
        return failCause(cause3);
      }
    }
  }, "onFailure"),
  onSuccess: /* @__PURE__ */ __name((a) => as2(onSuccess(a), a), "onSuccess")
}));
var tapDefect = /* @__PURE__ */ dual(2, (self, f) => catchAllCause(self, (cause3) => match2(keepDefects(cause3), {
  onNone: /* @__PURE__ */ __name(() => failCause(cause3), "onNone"),
  onSome: /* @__PURE__ */ __name((a) => zipRight(f(a), failCause(cause3)), "onSome")
})));
var tapError = /* @__PURE__ */ dual(2, (self, f) => matchCauseEffect(self, {
  onFailure: /* @__PURE__ */ __name((cause3) => {
    const either4 = failureOrCause(cause3);
    switch (either4._tag) {
      case "Left":
        return zipRight(f(either4.left), failCause(cause3));
      case "Right":
        return failCause(cause3);
    }
  }, "onFailure"),
  onSuccess: succeed
}));
var tapErrorTag = /* @__PURE__ */ dual(3, (self, k, f) => tapError(self, (e) => {
  if (isTagged(e, k)) {
    return f(e);
  }
  return void_;
}));
var tapErrorCause = /* @__PURE__ */ dual(2, (self, f) => matchCauseEffect(self, {
  onFailure: /* @__PURE__ */ __name((cause3) => zipRight(f(cause3), failCause(cause3)), "onFailure"),
  onSuccess: succeed
}));
var timed = /* @__PURE__ */ __name((self) => timedWith(self, currentTimeNanos2), "timed");
var timedWith = /* @__PURE__ */ dual(2, (self, nanos2) => summarized(self, nanos2, (start3, end3) => nanos(end3 - start3)));
var tracerWith3 = tracerWith2;
var tracer = /* @__PURE__ */ tracerWith3(succeed);
var tryPromise = /* @__PURE__ */ __name((arg) => {
  let evaluate2;
  let catcher = void 0;
  if (typeof arg === "function") {
    evaluate2 = arg;
  } else {
    evaluate2 = arg.try;
    catcher = arg.catch;
  }
  const fail7 = /* @__PURE__ */ __name((e) => catcher ? failSync(() => catcher(e)) : fail2(new UnknownException(e, "An unknown error occurred in Effect.tryPromise")), "fail");
  if (evaluate2.length >= 1) {
    return async_((resolve, signal) => {
      try {
        evaluate2(signal).then((a) => resolve(succeed(a)), (e) => resolve(fail7(e)));
      } catch (e) {
        resolve(fail7(e));
      }
    });
  }
  return async_((resolve) => {
    try {
      evaluate2().then((a) => resolve(succeed(a)), (e) => resolve(fail7(e)));
    } catch (e) {
      resolve(fail7(e));
    }
  });
}, "tryPromise");
var tryMap = /* @__PURE__ */ dual(2, (self, options) => flatMap7(self, (a) => try_({
  try: /* @__PURE__ */ __name(() => options.try(a), "try"),
  catch: options.catch
})));
var tryMapPromise = /* @__PURE__ */ dual(2, (self, options) => flatMap7(self, (a) => tryPromise({
  try: options.try.length >= 1 ? (signal) => options.try(a, signal) : () => options.try(a),
  catch: options.catch
})));
var unless = /* @__PURE__ */ dual(2, (self, condition) => suspend(() => condition() ? succeedNone : asSome(self)));
var unlessEffect = /* @__PURE__ */ dual(2, (self, condition) => flatMap7(condition, (b) => b ? succeedNone : asSome(self)));
var unsandbox = /* @__PURE__ */ __name((self) => mapErrorCause2(self, flatten3), "unsandbox");
var updateFiberRefs = /* @__PURE__ */ __name((f) => withFiberRuntime((state) => {
  state.setFiberRefs(f(state.id(), state.getFiberRefs()));
  return void_;
}), "updateFiberRefs");
var updateService = /* @__PURE__ */ dual(3, (self, tag, f) => mapInputContext(self, (context4) => add2(context4, tag, f(unsafeGet3(context4, tag)))));
var when = /* @__PURE__ */ dual(2, (self, condition) => suspend(() => condition() ? map8(self, some2) : succeed(none2())));
var whenFiberRef = /* @__PURE__ */ dual(3, (self, fiberRef, predicate) => flatMap7(fiberRefGet(fiberRef), (s) => predicate(s) ? map8(self, (a) => [s, some2(a)]) : succeed([s, none2()])));
var whenRef = /* @__PURE__ */ dual(3, (self, ref, predicate) => flatMap7(get12(ref), (s) => predicate(s) ? map8(self, (a) => [s, some2(a)]) : succeed([s, none2()])));
var withMetric = /* @__PURE__ */ dual(2, (self, metric) => metric(self));
var serviceFunctionEffect = /* @__PURE__ */ __name((getService, f) => (...args2) => flatMap7(getService, (a) => f(a)(...args2)), "serviceFunctionEffect");
var serviceFunction = /* @__PURE__ */ __name((getService, f) => (...args2) => map8(getService, (a) => f(a)(...args2)), "serviceFunction");
var serviceFunctions = /* @__PURE__ */ __name((getService) => new Proxy({}, {
  get(_target, prop, _receiver) {
    return (...args2) => flatMap7(getService, (s) => s[prop](...args2));
  }
}), "serviceFunctions");
var serviceConstants = /* @__PURE__ */ __name((getService) => new Proxy({}, {
  get(_target, prop, _receiver) {
    return flatMap7(getService, (s) => isEffect(s[prop]) ? s[prop] : succeed(s[prop]));
  }
}), "serviceConstants");
var serviceMembers = /* @__PURE__ */ __name((getService) => ({
  functions: serviceFunctions(getService),
  constants: serviceConstants(getService)
}), "serviceMembers");
var serviceOption = /* @__PURE__ */ __name((tag) => map8(context(), getOption2(tag)), "serviceOption");
var serviceOptional = /* @__PURE__ */ __name((tag) => flatMap7(context(), getOption2(tag)), "serviceOptional");
var annotateCurrentSpan = /* @__PURE__ */ __name(function() {
  const args2 = arguments;
  return ignore(flatMap7(currentPropagatedSpan, (span2) => sync(() => {
    if (typeof args2[0] === "string") {
      span2.attribute(args2[0], args2[1]);
    } else {
      for (const key in args2[0]) {
        span2.attribute(key, args2[0][key]);
      }
    }
  })));
}, "annotateCurrentSpan");
var linkSpanCurrent = /* @__PURE__ */ __name(function() {
  const args2 = arguments;
  const links = Array.isArray(args2[0]) ? args2[0] : [{
    _tag: "SpanLink",
    span: args2[0],
    attributes: args2[1] ?? {}
  }];
  return ignore(flatMap7(currentSpan, (span2) => sync(() => span2.addLinks(links))));
}, "linkSpanCurrent");
var annotateSpans = /* @__PURE__ */ dual((args2) => isEffect(args2[0]), function() {
  const args2 = arguments;
  return fiberRefLocallyWith(args2[0], currentTracerSpanAnnotations, typeof args2[1] === "string" ? set3(args2[1], args2[2]) : (annotations) => Object.entries(args2[1]).reduce((acc, [key, value]) => set3(acc, key, value), annotations));
});
var currentParentSpan = /* @__PURE__ */ serviceOptional(spanTag);
var currentSpan = /* @__PURE__ */ flatMap7(/* @__PURE__ */ context(), (context4) => {
  const span2 = context4.unsafeMap.get(spanTag.key);
  return span2 !== void 0 && span2._tag === "Span" ? succeed(span2) : fail2(new NoSuchElementException());
});
var currentPropagatedSpan = /* @__PURE__ */ flatMap7(/* @__PURE__ */ context(), (context4) => {
  const span2 = filterDisablePropagation(getOption2(context4, spanTag));
  return span2._tag === "Some" && span2.value._tag === "Span" ? succeed(span2.value) : fail2(new NoSuchElementException());
});
var linkSpans = /* @__PURE__ */ dual((args2) => isEffect(args2[0]), (self, span2, attributes) => fiberRefLocallyWith(self, currentTracerSpanLinks, append2({
  _tag: "SpanLink",
  span: span2,
  attributes: attributes ?? {}
})));
var bigint02 = /* @__PURE__ */ BigInt(0);
var filterDisablePropagation = /* @__PURE__ */ flatMap((span2) => get3(span2.context, DisablePropagation) ? span2._tag === "Span" ? filterDisablePropagation(span2.parent) : none2() : some2(span2));
var unsafeMakeSpan = /* @__PURE__ */ __name((fiber, name, options) => {
  const disablePropagation = !fiber.getFiberRef(currentTracerEnabled) || options.context && get3(options.context, DisablePropagation);
  const context4 = fiber.getFiberRef(currentContext);
  const parent = options.parent ? some2(options.parent) : options.root ? none2() : filterDisablePropagation(getOption2(context4, spanTag));
  let span2;
  if (disablePropagation) {
    span2 = noopSpan({
      name,
      parent,
      context: add2(options.context ?? empty3(), DisablePropagation, true)
    });
  } else {
    const services = fiber.getFiberRef(currentServices);
    const tracer3 = get3(services, tracerTag);
    const clock3 = get3(services, Clock);
    const timingEnabled = fiber.getFiberRef(currentTracerTimingEnabled);
    const fiberRefs3 = fiber.getFiberRefs();
    const annotationsFromEnv = get10(fiberRefs3, currentTracerSpanAnnotations);
    const linksFromEnv = get10(fiberRefs3, currentTracerSpanLinks);
    const links = linksFromEnv._tag === "Some" ? options.links !== void 0 ? [...toReadonlyArray(linksFromEnv.value), ...options.links ?? []] : toReadonlyArray(linksFromEnv.value) : options.links ?? empty();
    span2 = tracer3.span(name, parent, options.context ?? empty3(), links, timingEnabled ? clock3.unsafeCurrentTimeNanos() : bigint02, options.kind ?? "internal", options);
    if (annotationsFromEnv._tag === "Some") {
      forEach3(annotationsFromEnv.value, (value, key) => span2.attribute(key, value));
    }
    if (options.attributes !== void 0) {
      Object.entries(options.attributes).forEach(([k, v]) => span2.attribute(k, v));
    }
  }
  if (typeof options.captureStackTrace === "function") {
    spanToTrace.set(span2, options.captureStackTrace);
  }
  return span2;
}, "unsafeMakeSpan");
var makeSpan = /* @__PURE__ */ __name((name, options) => {
  options = addSpanStackTrace(options);
  return withFiberRuntime((fiber) => succeed(unsafeMakeSpan(fiber, name, options)));
}, "makeSpan");
var spanAnnotations = /* @__PURE__ */ fiberRefGet(currentTracerSpanAnnotations);
var spanLinks = /* @__PURE__ */ fiberRefGet(currentTracerSpanLinks);
var endSpan = /* @__PURE__ */ __name((span2, exit4, clock3, timingEnabled) => sync(() => {
  if (span2.status._tag === "Ended") {
    return;
  }
  if (exitIsFailure(exit4) && spanToTrace.has(span2)) {
    span2.attribute("code.stacktrace", spanToTrace.get(span2)());
  }
  span2.end(timingEnabled ? clock3.unsafeCurrentTimeNanos() : bigint02, exit4);
}), "endSpan");
var useSpan = /* @__PURE__ */ __name((name, ...args2) => {
  const options = addSpanStackTrace(args2.length === 1 ? void 0 : args2[0]);
  const evaluate2 = args2[args2.length - 1];
  return withFiberRuntime((fiber) => {
    const span2 = unsafeMakeSpan(fiber, name, options);
    const timingEnabled = fiber.getFiberRef(currentTracerTimingEnabled);
    const clock3 = get3(fiber.getFiberRef(currentServices), clockTag);
    return onExit(evaluate2(span2), (exit4) => endSpan(span2, exit4, clock3, timingEnabled));
  });
}, "useSpan");
var withParentSpan = /* @__PURE__ */ dual(2, (self, span2) => provideService(self, spanTag, span2));
var withSpan = /* @__PURE__ */ __name(function() {
  const dataFirst = typeof arguments[0] !== "string";
  const name = dataFirst ? arguments[1] : arguments[0];
  const options = addSpanStackTrace(dataFirst ? arguments[2] : arguments[1]);
  if (dataFirst) {
    const self = arguments[0];
    return useSpan(name, options, (span2) => withParentSpan(self, span2));
  }
  return (self) => useSpan(name, options, (span2) => withParentSpan(self, span2));
}, "withSpan");
var functionWithSpan = /* @__PURE__ */ __name((options) => function() {
  let captureStackTrace = options.captureStackTrace ?? false;
  if (options.captureStackTrace !== false) {
    const limit = Error.stackTraceLimit;
    Error.stackTraceLimit = 2;
    const error = new Error();
    Error.stackTraceLimit = limit;
    let cache = false;
    captureStackTrace = /* @__PURE__ */ __name(() => {
      if (cache !== false) {
        return cache;
      }
      if (error.stack) {
        const stack = error.stack.trim().split("\n");
        cache = stack.slice(2).join("\n").trim();
        return cache;
      }
    }, "captureStackTrace");
  }
  return suspend(() => {
    const opts = typeof options.options === "function" ? options.options.apply(null, arguments) : options.options;
    return withSpan(suspend(() => internalCall(() => options.body.apply(this, arguments))), opts.name, {
      ...opts,
      captureStackTrace
    });
  });
}, "functionWithSpan");
var fromNullable2 = /* @__PURE__ */ __name((value) => value == null ? fail2(new NoSuchElementException()) : succeed(value), "fromNullable");
var optionFromOptional = /* @__PURE__ */ __name((self) => catchAll(map8(self, some2), (error) => isNoSuchElementException(error) ? succeedNone : fail2(error)), "optionFromOptional");

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/internal/executionStrategy.js
var OP_SEQUENTIAL2 = "Sequential";
var OP_PARALLEL2 = "Parallel";
var OP_PARALLEL_N = "ParallelN";
var sequential2 = {
  _tag: OP_SEQUENTIAL2
};
var parallel2 = {
  _tag: OP_PARALLEL2
};
var parallelN = /* @__PURE__ */ __name((parallelism) => ({
  _tag: OP_PARALLEL_N,
  parallelism
}), "parallelN");
var isSequential = /* @__PURE__ */ __name((self) => self._tag === OP_SEQUENTIAL2, "isSequential");
var isParallel = /* @__PURE__ */ __name((self) => self._tag === OP_PARALLEL2, "isParallel");

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/ExecutionStrategy.js
var sequential3 = sequential2;
var parallel3 = parallel2;
var parallelN2 = parallelN;

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/FiberRefsPatch.js
var diff6 = diff5;
var patch7 = patch6;

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/internal/fiberStatus.js
var FiberStatusSymbolKey = "effect/FiberStatus";
var FiberStatusTypeId = /* @__PURE__ */ Symbol.for(FiberStatusSymbolKey);
var OP_DONE = "Done";
var OP_RUNNING = "Running";
var OP_SUSPENDED = "Suspended";
var DoneHash = /* @__PURE__ */ string(`${FiberStatusSymbolKey}-${OP_DONE}`);
var Done = class {
  static {
    __name(this, "Done");
  }
  [FiberStatusTypeId] = FiberStatusTypeId;
  _tag = OP_DONE;
  [symbol]() {
    return DoneHash;
  }
  [symbol2](that) {
    return isFiberStatus(that) && that._tag === OP_DONE;
  }
};
var Running = class {
  static {
    __name(this, "Running");
  }
  runtimeFlags;
  [FiberStatusTypeId] = FiberStatusTypeId;
  _tag = OP_RUNNING;
  constructor(runtimeFlags2) {
    this.runtimeFlags = runtimeFlags2;
  }
  [symbol]() {
    return pipe(hash(FiberStatusSymbolKey), combine(hash(this._tag)), combine(hash(this.runtimeFlags)), cached(this));
  }
  [symbol2](that) {
    return isFiberStatus(that) && that._tag === OP_RUNNING && this.runtimeFlags === that.runtimeFlags;
  }
};
var Suspended = class {
  static {
    __name(this, "Suspended");
  }
  runtimeFlags;
  blockingOn;
  [FiberStatusTypeId] = FiberStatusTypeId;
  _tag = OP_SUSPENDED;
  constructor(runtimeFlags2, blockingOn) {
    this.runtimeFlags = runtimeFlags2;
    this.blockingOn = blockingOn;
  }
  [symbol]() {
    return pipe(hash(FiberStatusSymbolKey), combine(hash(this._tag)), combine(hash(this.runtimeFlags)), combine(hash(this.blockingOn)), cached(this));
  }
  [symbol2](that) {
    return isFiberStatus(that) && that._tag === OP_SUSPENDED && this.runtimeFlags === that.runtimeFlags && equals(this.blockingOn, that.blockingOn);
  }
};
var done3 = /* @__PURE__ */ new Done();
var running = /* @__PURE__ */ __name((runtimeFlags2) => new Running(runtimeFlags2), "running");
var suspended = /* @__PURE__ */ __name((runtimeFlags2, blockingOn) => new Suspended(runtimeFlags2, blockingOn), "suspended");
var isFiberStatus = /* @__PURE__ */ __name((u) => hasProperty(u, FiberStatusTypeId), "isFiberStatus");
var isDone = /* @__PURE__ */ __name((self) => self._tag === OP_DONE, "isDone");

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/FiberStatus.js
var done4 = done3;
var running2 = running;
var suspended2 = suspended;
var isDone2 = isDone;

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/Micro.js
var TypeId13 = /* @__PURE__ */ Symbol.for("effect/Micro");
var MicroExitTypeId = /* @__PURE__ */ Symbol.for("effect/Micro/MicroExit");
var MicroCauseTypeId = /* @__PURE__ */ Symbol.for("effect/Micro/MicroCause");
var microCauseVariance = {
  _E: identity
};
var MicroCauseImpl = class extends globalThis.Error {
  static {
    __name(this, "MicroCauseImpl");
  }
  _tag;
  traces;
  [MicroCauseTypeId];
  constructor(_tag, originalError2, traces) {
    const causeName = `MicroCause.${_tag}`;
    let name;
    let message;
    let stack;
    if (originalError2 instanceof globalThis.Error) {
      name = `(${causeName}) ${originalError2.name}`;
      message = originalError2.message;
      const messageLines = message.split("\n").length;
      stack = originalError2.stack ? `(${causeName}) ${originalError2.stack.split("\n").slice(0, messageLines + 3).join("\n")}` : `${name}: ${message}`;
    } else {
      name = causeName;
      message = toStringUnknown(originalError2, 0);
      stack = `${name}: ${message}`;
    }
    if (traces.length > 0) {
      stack += `
    ${traces.join("\n    ")}`;
    }
    super(message);
    this._tag = _tag;
    this.traces = traces;
    this[MicroCauseTypeId] = microCauseVariance;
    this.name = name;
    this.stack = stack;
  }
  pipe() {
    return pipeArguments(this, arguments);
  }
  toString() {
    return this.stack;
  }
  [NodeInspectSymbol]() {
    return this.stack;
  }
};
var Die = class extends MicroCauseImpl {
  static {
    __name(this, "Die");
  }
  defect;
  constructor(defect, traces = []) {
    super("Die", defect, traces);
    this.defect = defect;
  }
};
var causeDie = /* @__PURE__ */ __name((defect, traces = []) => new Die(defect, traces), "causeDie");
var Interrupt = class extends MicroCauseImpl {
  static {
    __name(this, "Interrupt");
  }
  constructor(traces = []) {
    super("Interrupt", "interrupted", traces);
  }
};
var causeInterrupt = /* @__PURE__ */ __name((traces = []) => new Interrupt(traces), "causeInterrupt");
var causeIsInterrupt = /* @__PURE__ */ __name((self) => self._tag === "Interrupt", "causeIsInterrupt");
var MicroFiberTypeId = /* @__PURE__ */ Symbol.for("effect/Micro/MicroFiber");
var fiberVariance = {
  _A: identity,
  _E: identity
};
var MicroFiberImpl = class {
  static {
    __name(this, "MicroFiberImpl");
  }
  context;
  interruptible;
  [MicroFiberTypeId];
  _stack = [];
  _observers = [];
  _exit;
  _children;
  currentOpCount = 0;
  constructor(context4, interruptible5 = true) {
    this.context = context4;
    this.interruptible = interruptible5;
    this[MicroFiberTypeId] = fiberVariance;
  }
  getRef(ref) {
    return unsafeGetReference(this.context, ref);
  }
  addObserver(cb) {
    if (this._exit) {
      cb(this._exit);
      return constVoid;
    }
    this._observers.push(cb);
    return () => {
      const index = this._observers.indexOf(cb);
      if (index >= 0) {
        this._observers.splice(index, 1);
      }
    };
  }
  _interrupted = false;
  unsafeInterrupt() {
    if (this._exit) {
      return;
    }
    this._interrupted = true;
    if (this.interruptible) {
      this.evaluate(exitInterrupt2);
    }
  }
  unsafePoll() {
    return this._exit;
  }
  evaluate(effect) {
    if (this._exit) {
      return;
    } else if (this._yielded !== void 0) {
      const yielded = this._yielded;
      this._yielded = void 0;
      yielded();
    }
    const exit4 = this.runLoop(effect);
    if (exit4 === Yield) {
      return;
    }
    const interruptChildren = fiberMiddleware.interruptChildren && fiberMiddleware.interruptChildren(this);
    if (interruptChildren !== void 0) {
      return this.evaluate(flatMap9(interruptChildren, () => exit4));
    }
    this._exit = exit4;
    for (let i = 0; i < this._observers.length; i++) {
      this._observers[i](exit4);
    }
    this._observers.length = 0;
  }
  runLoop(effect) {
    let yielding = false;
    let current = effect;
    this.currentOpCount = 0;
    try {
      while (true) {
        this.currentOpCount++;
        if (!yielding && this.getRef(CurrentScheduler).shouldYield(this)) {
          yielding = true;
          const prev = current;
          current = flatMap9(yieldNow2, () => prev);
        }
        current = current[evaluate](this);
        if (current === Yield) {
          const yielded = this._yielded;
          if (MicroExitTypeId in yielded) {
            this._yielded = void 0;
            return yielded;
          }
          return Yield;
        }
      }
    } catch (error) {
      if (!hasProperty(current, evaluate)) {
        return exitDie2(`MicroFiber.runLoop: Not a valid effect: ${String(current)}`);
      }
      return exitDie2(error);
    }
  }
  getCont(symbol3) {
    while (true) {
      const op = this._stack.pop();
      if (!op) return void 0;
      const cont = op[ensureCont] && op[ensureCont](this);
      if (cont) return {
        [symbol3]: cont
      };
      if (op[symbol3]) return op;
    }
  }
  // cancel the yielded operation, or for the yielded exit value
  _yielded = void 0;
  yieldWith(value) {
    this._yielded = value;
    return Yield;
  }
  children() {
    return this._children ??= /* @__PURE__ */ new Set();
  }
};
var fiberMiddleware = /* @__PURE__ */ globalValue("effect/Micro/fiberMiddleware", () => ({
  interruptChildren: void 0
}));
var fiberInterruptAll = /* @__PURE__ */ __name((fibers) => suspend2(() => {
  for (const fiber of fibers) fiber.unsafeInterrupt();
  const iter = fibers[Symbol.iterator]();
  const wait = suspend2(() => {
    let result = iter.next();
    while (!result.done) {
      if (result.value.unsafePoll()) {
        result = iter.next();
        continue;
      }
      const fiber = result.value;
      return async((resume2) => {
        fiber.addObserver((_) => {
          resume2(wait);
        });
      });
    }
    return exitVoid2;
  });
  return wait;
}), "fiberInterruptAll");
var identifier = /* @__PURE__ */ Symbol.for("effect/Micro/identifier");
var args = /* @__PURE__ */ Symbol.for("effect/Micro/args");
var evaluate = /* @__PURE__ */ Symbol.for("effect/Micro/evaluate");
var successCont = /* @__PURE__ */ Symbol.for("effect/Micro/successCont");
var failureCont = /* @__PURE__ */ Symbol.for("effect/Micro/failureCont");
var ensureCont = /* @__PURE__ */ Symbol.for("effect/Micro/ensureCont");
var Yield = /* @__PURE__ */ Symbol.for("effect/Micro/Yield");
var microVariance = {
  _A: identity,
  _E: identity,
  _R: identity
};
var MicroProto = {
  ...EffectPrototype2,
  _op: "Micro",
  [TypeId13]: microVariance,
  pipe() {
    return pipeArguments(this, arguments);
  },
  [Symbol.iterator]() {
    return new SingleShotGen(new YieldWrap(this));
  },
  toJSON() {
    return {
      _id: "Micro",
      op: this[identifier],
      ...args in this ? {
        args: this[args]
      } : void 0
    };
  },
  toString() {
    return format(this);
  },
  [NodeInspectSymbol]() {
    return format(this);
  }
};
function defaultEvaluate(_fiber) {
  return exitDie2(`Micro.evaluate: Not implemented`);
}
__name(defaultEvaluate, "defaultEvaluate");
var makePrimitiveProto = /* @__PURE__ */ __name((options) => ({
  ...MicroProto,
  [identifier]: options.op,
  [evaluate]: options.eval ?? defaultEvaluate,
  [successCont]: options.contA,
  [failureCont]: options.contE,
  [ensureCont]: options.ensure
}), "makePrimitiveProto");
var makePrimitive = /* @__PURE__ */ __name((options) => {
  const Proto2 = makePrimitiveProto(options);
  return function() {
    const self = Object.create(Proto2);
    self[args] = options.single === false ? arguments : arguments[0];
    return self;
  };
}, "makePrimitive");
var makeExit = /* @__PURE__ */ __name((options) => {
  const Proto2 = {
    ...makePrimitiveProto(options),
    [MicroExitTypeId]: MicroExitTypeId,
    _tag: options.op,
    get [options.prop]() {
      return this[args];
    },
    toJSON() {
      return {
        _id: "MicroExit",
        _tag: options.op,
        [options.prop]: this[args]
      };
    },
    [symbol2](that) {
      return isMicroExit(that) && that._tag === options.op && equals(this[args], that[args]);
    },
    [symbol]() {
      return cached(this, combine(string(options.op))(hash(this[args])));
    }
  };
  return function(value) {
    const self = Object.create(Proto2);
    self[args] = value;
    self[successCont] = void 0;
    self[failureCont] = void 0;
    self[ensureCont] = void 0;
    return self;
  };
}, "makeExit");
var succeed3 = /* @__PURE__ */ makeExit({
  op: "Success",
  prop: "value",
  eval(fiber) {
    const cont = fiber.getCont(successCont);
    return cont ? cont[successCont](this[args], fiber) : fiber.yieldWith(this);
  }
});
var failCause3 = /* @__PURE__ */ makeExit({
  op: "Failure",
  prop: "cause",
  eval(fiber) {
    let cont = fiber.getCont(failureCont);
    while (causeIsInterrupt(this[args]) && cont && fiber.interruptible) {
      cont = fiber.getCont(failureCont);
    }
    return cont ? cont[failureCont](this[args], fiber) : fiber.yieldWith(this);
  }
});
var sync2 = /* @__PURE__ */ makePrimitive({
  op: "Sync",
  eval(fiber) {
    const value = this[args]();
    const cont = fiber.getCont(successCont);
    return cont ? cont[successCont](value, fiber) : fiber.yieldWith(exitSucceed2(value));
  }
});
var suspend2 = /* @__PURE__ */ makePrimitive({
  op: "Suspend",
  eval(_fiber) {
    return this[args]();
  }
});
var yieldNowWith = /* @__PURE__ */ makePrimitive({
  op: "Yield",
  eval(fiber) {
    let resumed = false;
    fiber.getRef(CurrentScheduler).scheduleTask(() => {
      if (resumed) return;
      fiber.evaluate(exitVoid2);
    }, this[args] ?? 0);
    return fiber.yieldWith(() => {
      resumed = true;
    });
  }
});
var yieldNow2 = /* @__PURE__ */ yieldNowWith(0);
var void_3 = /* @__PURE__ */ succeed3(void 0);
var withMicroFiber = /* @__PURE__ */ makePrimitive({
  op: "WithMicroFiber",
  eval(fiber) {
    return this[args](fiber);
  }
});
var asyncOptions = /* @__PURE__ */ makePrimitive({
  op: "Async",
  single: false,
  eval(fiber) {
    const register = this[args][0];
    let resumed = false;
    let yielded = false;
    const controller = this[args][1] ? new AbortController() : void 0;
    const onCancel = register((effect) => {
      if (resumed) return;
      resumed = true;
      if (yielded) {
        fiber.evaluate(effect);
      } else {
        yielded = effect;
      }
    }, controller?.signal);
    if (yielded !== false) return yielded;
    yielded = true;
    fiber._yielded = () => {
      resumed = true;
    };
    if (controller === void 0 && onCancel === void 0) {
      return Yield;
    }
    fiber._stack.push(asyncFinalizer(() => {
      resumed = true;
      controller?.abort();
      return onCancel ?? exitVoid2;
    }));
    return Yield;
  }
});
var asyncFinalizer = /* @__PURE__ */ makePrimitive({
  op: "AsyncFinalizer",
  ensure(fiber) {
    if (fiber.interruptible) {
      fiber.interruptible = false;
      fiber._stack.push(setInterruptible(true));
    }
  },
  contE(cause3, _fiber) {
    return causeIsInterrupt(cause3) ? flatMap9(this[args](), () => failCause3(cause3)) : failCause3(cause3);
  }
});
var async = /* @__PURE__ */ __name((register) => asyncOptions(register, register.length >= 2), "async");
var as4 = /* @__PURE__ */ dual(2, (self, value) => map10(self, (_) => value));
var exit2 = /* @__PURE__ */ __name((self) => matchCause2(self, {
  onFailure: exitFailCause2,
  onSuccess: exitSucceed2
}), "exit");
var flatMap9 = /* @__PURE__ */ dual(2, (self, f) => {
  const onSuccess = Object.create(OnSuccessProto);
  onSuccess[args] = self;
  onSuccess[successCont] = f;
  return onSuccess;
});
var OnSuccessProto = /* @__PURE__ */ makePrimitiveProto({
  op: "OnSuccess",
  eval(fiber) {
    fiber._stack.push(this);
    return this[args];
  }
});
var map10 = /* @__PURE__ */ dual(2, (self, f) => flatMap9(self, (a) => succeed3(f(a))));
var isMicroExit = /* @__PURE__ */ __name((u) => hasProperty(u, MicroExitTypeId), "isMicroExit");
var exitSucceed2 = succeed3;
var exitFailCause2 = failCause3;
var exitInterrupt2 = /* @__PURE__ */ exitFailCause2(/* @__PURE__ */ causeInterrupt());
var exitDie2 = /* @__PURE__ */ __name((defect) => exitFailCause2(causeDie(defect)), "exitDie");
var exitVoid2 = /* @__PURE__ */ exitSucceed2(void 0);
var exitVoidAll = /* @__PURE__ */ __name((exits) => {
  for (const exit4 of exits) {
    if (exit4._tag === "Failure") {
      return exit4;
    }
  }
  return exitVoid2;
}, "exitVoidAll");
var setImmediate = "setImmediate" in globalThis ? globalThis.setImmediate : (f) => setTimeout(f, 0);
var MicroSchedulerDefault = class {
  static {
    __name(this, "MicroSchedulerDefault");
  }
  tasks = [];
  running = false;
  /**
   * @since 3.5.9
   */
  scheduleTask(task, _priority) {
    this.tasks.push(task);
    if (!this.running) {
      this.running = true;
      setImmediate(this.afterScheduled);
    }
  }
  /**
   * @since 3.5.9
   */
  afterScheduled = /* @__PURE__ */ __name(() => {
    this.running = false;
    this.runTasks();
  }, "afterScheduled");
  /**
   * @since 3.5.9
   */
  runTasks() {
    const tasks = this.tasks;
    this.tasks = [];
    for (let i = 0, len = tasks.length; i < len; i++) {
      tasks[i]();
    }
  }
  /**
   * @since 3.5.9
   */
  shouldYield(fiber) {
    return fiber.currentOpCount >= fiber.getRef(MaxOpsBeforeYield);
  }
  /**
   * @since 3.5.9
   */
  flush() {
    while (this.tasks.length > 0) {
      this.runTasks();
    }
  }
};
var updateContext = /* @__PURE__ */ dual(2, (self, f) => withMicroFiber((fiber) => {
  const prev = fiber.context;
  fiber.context = f(prev);
  return onExit2(self, () => {
    fiber.context = prev;
    return void_3;
  });
}));
var provideContext2 = /* @__PURE__ */ dual(2, (self, provided) => updateContext(self, merge3(provided)));
var MaxOpsBeforeYield = class extends (/* @__PURE__ */ Reference2()("effect/Micro/currentMaxOpsBeforeYield", {
  defaultValue: /* @__PURE__ */ __name(() => 2048, "defaultValue")
})) {
  static {
    __name(this, "MaxOpsBeforeYield");
  }
};
var CurrentConcurrency = class extends (/* @__PURE__ */ Reference2()("effect/Micro/currentConcurrency", {
  defaultValue: /* @__PURE__ */ __name(() => "unbounded", "defaultValue")
})) {
  static {
    __name(this, "CurrentConcurrency");
  }
};
var CurrentScheduler = class extends (/* @__PURE__ */ Reference2()("effect/Micro/currentScheduler", {
  defaultValue: /* @__PURE__ */ __name(() => new MicroSchedulerDefault(), "defaultValue")
})) {
  static {
    __name(this, "CurrentScheduler");
  }
};
var matchCauseEffect2 = /* @__PURE__ */ dual(2, (self, options) => {
  const primitive = Object.create(OnSuccessAndFailureProto);
  primitive[args] = self;
  primitive[successCont] = options.onSuccess;
  primitive[failureCont] = options.onFailure;
  return primitive;
});
var OnSuccessAndFailureProto = /* @__PURE__ */ makePrimitiveProto({
  op: "OnSuccessAndFailure",
  eval(fiber) {
    fiber._stack.push(this);
    return this[args];
  }
});
var matchCause2 = /* @__PURE__ */ dual(2, (self, options) => matchCauseEffect2(self, {
  onFailure: /* @__PURE__ */ __name((cause3) => sync2(() => options.onFailure(cause3)), "onFailure"),
  onSuccess: /* @__PURE__ */ __name((value) => sync2(() => options.onSuccess(value)), "onSuccess")
}));
var MicroScopeTypeId = /* @__PURE__ */ Symbol.for("effect/Micro/MicroScope");
var MicroScopeImpl = class _MicroScopeImpl {
  static {
    __name(this, "MicroScopeImpl");
  }
  [MicroScopeTypeId];
  state = {
    _tag: "Open",
    finalizers: /* @__PURE__ */ new Set()
  };
  constructor() {
    this[MicroScopeTypeId] = MicroScopeTypeId;
  }
  unsafeAddFinalizer(finalizer) {
    if (this.state._tag === "Open") {
      this.state.finalizers.add(finalizer);
    }
  }
  addFinalizer(finalizer) {
    return suspend2(() => {
      if (this.state._tag === "Open") {
        this.state.finalizers.add(finalizer);
        return void_3;
      }
      return finalizer(this.state.exit);
    });
  }
  unsafeRemoveFinalizer(finalizer) {
    if (this.state._tag === "Open") {
      this.state.finalizers.delete(finalizer);
    }
  }
  close(microExit) {
    return suspend2(() => {
      if (this.state._tag === "Open") {
        const finalizers = Array.from(this.state.finalizers).reverse();
        this.state = {
          _tag: "Closed",
          exit: microExit
        };
        return flatMap9(forEach4(finalizers, (finalizer) => exit2(finalizer(microExit))), exitVoidAll);
      }
      return void_3;
    });
  }
  get fork() {
    return sync2(() => {
      const newScope = new _MicroScopeImpl();
      if (this.state._tag === "Closed") {
        newScope.state = this.state;
        return newScope;
      }
      function fin(exit4) {
        return newScope.close(exit4);
      }
      __name(fin, "fin");
      this.state.finalizers.add(fin);
      newScope.unsafeAddFinalizer((_) => sync2(() => this.unsafeRemoveFinalizer(fin)));
      return newScope;
    });
  }
};
var onExit2 = /* @__PURE__ */ dual(2, (self, f) => uninterruptibleMask2((restore) => matchCauseEffect2(restore(self), {
  onFailure: /* @__PURE__ */ __name((cause3) => flatMap9(f(exitFailCause2(cause3)), () => failCause3(cause3)), "onFailure"),
  onSuccess: /* @__PURE__ */ __name((a) => flatMap9(f(exitSucceed2(a)), () => succeed3(a)), "onSuccess")
})));
var setInterruptible = /* @__PURE__ */ makePrimitive({
  op: "SetInterruptible",
  ensure(fiber) {
    fiber.interruptible = this[args];
    if (fiber._interrupted && fiber.interruptible) {
      return () => exitInterrupt2;
    }
  }
});
var interruptible3 = /* @__PURE__ */ __name((self) => withMicroFiber((fiber) => {
  if (fiber.interruptible) return self;
  fiber.interruptible = true;
  fiber._stack.push(setInterruptible(false));
  if (fiber._interrupted) return exitInterrupt2;
  return self;
}), "interruptible");
var uninterruptibleMask2 = /* @__PURE__ */ __name((f) => withMicroFiber((fiber) => {
  if (!fiber.interruptible) return f(identity);
  fiber.interruptible = false;
  fiber._stack.push(setInterruptible(true));
  return f(interruptible3);
}), "uninterruptibleMask");
var whileLoop2 = /* @__PURE__ */ makePrimitive({
  op: "While",
  contA(value, fiber) {
    this[args].step(value);
    if (this[args].while()) {
      fiber._stack.push(this);
      return this[args].body();
    }
    return exitVoid2;
  },
  eval(fiber) {
    if (this[args].while()) {
      fiber._stack.push(this);
      return this[args].body();
    }
    return exitVoid2;
  }
});
var forEach4 = /* @__PURE__ */ __name((iterable, f, options) => withMicroFiber((parent) => {
  const concurrencyOption = options?.concurrency === "inherit" ? parent.getRef(CurrentConcurrency) : options?.concurrency ?? 1;
  const concurrency = concurrencyOption === "unbounded" ? Number.POSITIVE_INFINITY : Math.max(1, concurrencyOption);
  const items = fromIterable(iterable);
  let length2 = items.length;
  if (length2 === 0) {
    return options?.discard ? void_3 : succeed3([]);
  }
  const out = options?.discard ? void 0 : new Array(length2);
  let index = 0;
  if (concurrency === 1) {
    return as4(whileLoop2({
      while: /* @__PURE__ */ __name(() => index < items.length, "while"),
      body: /* @__PURE__ */ __name(() => f(items[index], index), "body"),
      step: out ? (b) => out[index++] = b : (_) => index++
    }), out);
  }
  return async((resume2) => {
    const fibers = /* @__PURE__ */ new Set();
    let result = void 0;
    let inProgress = 0;
    let doneCount = 0;
    let pumping = false;
    let interrupted = false;
    function pump() {
      pumping = true;
      while (inProgress < concurrency && index < length2) {
        const currentIndex = index;
        const item = items[currentIndex];
        index++;
        inProgress++;
        try {
          const child = unsafeFork(parent, f(item, currentIndex), true, true);
          fibers.add(child);
          child.addObserver((exit4) => {
            fibers.delete(child);
            if (interrupted) {
              return;
            } else if (exit4._tag === "Failure") {
              if (result === void 0) {
                result = exit4;
                length2 = index;
                fibers.forEach((fiber) => fiber.unsafeInterrupt());
              }
            } else if (out !== void 0) {
              out[currentIndex] = exit4.value;
            }
            doneCount++;
            inProgress--;
            if (doneCount === length2) {
              resume2(result ?? succeed3(out));
            } else if (!pumping && inProgress < concurrency) {
              pump();
            }
          });
        } catch (err) {
          result = exitDie2(err);
          length2 = index;
          fibers.forEach((fiber) => fiber.unsafeInterrupt());
        }
      }
      pumping = false;
    }
    __name(pump, "pump");
    pump();
    return suspend2(() => {
      interrupted = true;
      index = length2;
      return fiberInterruptAll(fibers);
    });
  });
}), "forEach");
var unsafeFork = /* @__PURE__ */ __name((parent, effect, immediate = false, daemon = false) => {
  const child = new MicroFiberImpl(parent.context, parent.interruptible);
  if (!daemon) {
    parent.children().add(child);
    child.addObserver(() => parent.children().delete(child));
  }
  if (immediate) {
    child.evaluate(effect);
  } else {
    parent.getRef(CurrentScheduler).scheduleTask(() => child.evaluate(effect), 0);
  }
  return child;
}, "unsafeFork");
var runFork = /* @__PURE__ */ __name((effect, options) => {
  const fiber = new MicroFiberImpl(CurrentScheduler.context(options?.scheduler ?? new MicroSchedulerDefault()));
  fiber.evaluate(effect);
  if (options?.signal) {
    if (options.signal.aborted) {
      fiber.unsafeInterrupt();
    } else {
      const abort = /* @__PURE__ */ __name(() => fiber.unsafeInterrupt(), "abort");
      options.signal.addEventListener("abort", abort, {
        once: true
      });
      fiber.addObserver(() => options.signal.removeEventListener("abort", abort));
    }
  }
  return fiber;
}, "runFork");

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/Scheduler.js
var SchedulerRunner = class _SchedulerRunner {
  static {
    __name(this, "SchedulerRunner");
  }
  scheduleDrain;
  running = false;
  tasks = /* @__PURE__ */ new PriorityBuckets();
  constructor(scheduleDrain) {
    this.scheduleDrain = scheduleDrain;
  }
  starveInternal = /* @__PURE__ */ __name((depth) => {
    const tasks = this.tasks.buckets;
    this.tasks.buckets = [];
    for (const [_, toRun] of tasks) {
      for (let i = 0; i < toRun.length; i++) {
        toRun[i]();
      }
    }
    if (this.tasks.buckets.length === 0) {
      this.running = false;
    } else {
      this.starve(depth);
    }
  }, "starveInternal");
  starve(depth = 0) {
    this.scheduleDrain(depth, this.starveInternal);
  }
  scheduleTask(task, priority) {
    this.tasks.scheduleTask(task, priority);
    if (!this.running) {
      this.running = true;
      this.starve();
    }
  }
  /**
   * @since 3.20.0
   * @category constructors
   */
  static cached(scheduleDrain) {
    const fallback = new _SchedulerRunner(scheduleDrain);
    const runners = /* @__PURE__ */ new WeakMap();
    return (fiber) => {
      if (fiber === void 0) {
        return fallback;
      }
      let runner = runners.get(fiber);
      if (runner === void 0) {
        runner = new _SchedulerRunner(scheduleDrain);
        runners.set(fiber, runner);
      }
      return runner;
    };
  }
};
var PriorityBuckets = class {
  static {
    __name(this, "PriorityBuckets");
  }
  /**
   * @since 2.0.0
   */
  buckets = [];
  /**
   * @since 2.0.0
   */
  scheduleTask(task, priority) {
    const length2 = this.buckets.length;
    let bucket = void 0;
    let index = 0;
    for (; index < length2; index++) {
      if (this.buckets[index][0] <= priority) {
        bucket = this.buckets[index];
      } else {
        break;
      }
    }
    if (bucket && bucket[0] === priority) {
      bucket[1].push(task);
    } else if (index === length2) {
      this.buckets.push([priority, [task]]);
    } else {
      this.buckets.splice(index, 0, [priority, [task]]);
    }
  }
};
var MixedScheduler = class {
  static {
    __name(this, "MixedScheduler");
  }
  maxNextTickBeforeTimer;
  getRunner = /* @__PURE__ */ SchedulerRunner.cached((depth, drain) => {
    if (depth >= this.maxNextTickBeforeTimer) {
      setTimeout(() => drain(0), 0);
    } else {
      Promise.resolve(void 0).then(() => drain(depth + 1));
    }
  });
  constructor(maxNextTickBeforeTimer) {
    this.maxNextTickBeforeTimer = maxNextTickBeforeTimer;
  }
  /**
   * @since 2.0.0
   */
  shouldYield(fiber) {
    return fiber.currentOpCount > fiber.getFiberRef(currentMaxOpsBeforeYield) ? fiber.getFiberRef(currentSchedulingPriority) : false;
  }
  /**
   * @since 2.0.0
   */
  scheduleTask(task, priority, fiber) {
    this.getRunner(fiber).scheduleTask(task, priority);
  }
};
var defaultScheduler = /* @__PURE__ */ globalValue(/* @__PURE__ */ Symbol.for("effect/Scheduler/defaultScheduler"), () => new MixedScheduler(2048));
var SyncScheduler = class {
  static {
    __name(this, "SyncScheduler");
  }
  /**
   * @since 2.0.0
   */
  tasks = /* @__PURE__ */ new PriorityBuckets();
  /**
   * @since 2.0.0
   */
  deferred = false;
  /**
   * @since 2.0.0
   */
  scheduleTask(task, priority, fiber) {
    if (this.deferred) {
      defaultScheduler.scheduleTask(task, priority, fiber);
    } else {
      this.tasks.scheduleTask(task, priority);
    }
  }
  /**
   * @since 2.0.0
   */
  shouldYield(fiber) {
    return fiber.currentOpCount > fiber.getFiberRef(currentMaxOpsBeforeYield) ? fiber.getFiberRef(currentSchedulingPriority) : false;
  }
  /**
   * @since 2.0.0
   */
  flush() {
    while (this.tasks.buckets.length > 0) {
      const tasks = this.tasks.buckets;
      this.tasks.buckets = [];
      for (const [_, toRun] of tasks) {
        for (let i = 0; i < toRun.length; i++) {
          toRun[i]();
        }
      }
    }
    this.deferred = true;
  }
};
var currentScheduler = /* @__PURE__ */ globalValue(/* @__PURE__ */ Symbol.for("effect/FiberRef/currentScheduler"), () => fiberRefUnsafeMake(defaultScheduler));
var withScheduler = /* @__PURE__ */ dual(2, (self, scheduler) => fiberRefLocally(self, currentScheduler, scheduler));

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/internal/completedRequestMap.js
var currentRequestMap = /* @__PURE__ */ globalValue(/* @__PURE__ */ Symbol.for("effect/FiberRef/currentRequestMap"), () => fiberRefUnsafeMake(/* @__PURE__ */ new Map()));

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/internal/concurrency.js
var match9 = /* @__PURE__ */ __name((concurrency, sequential5, unbounded2, bounded) => {
  switch (concurrency) {
    case void 0:
      return sequential5();
    case "unbounded":
      return unbounded2();
    case "inherit":
      return fiberRefGetWith(currentConcurrency, (concurrency2) => concurrency2 === "unbounded" ? unbounded2() : concurrency2 > 1 ? bounded(concurrency2) : sequential5());
    default:
      return concurrency > 1 ? bounded(concurrency) : sequential5();
  }
}, "match");
var matchSimple = /* @__PURE__ */ __name((concurrency, sequential5, concurrent) => {
  switch (concurrency) {
    case void 0:
      return sequential5();
    case "unbounded":
      return concurrent();
    case "inherit":
      return fiberRefGetWith(currentConcurrency, (concurrency2) => concurrency2 === "unbounded" || concurrency2 > 1 ? concurrent() : sequential5());
    default:
      return concurrency > 1 ? concurrent() : sequential5();
  }
}, "matchSimple");

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/internal/fiberMessage.js
var OP_INTERRUPT_SIGNAL = "InterruptSignal";
var OP_STATEFUL = "Stateful";
var OP_RESUME = "Resume";
var OP_YIELD_NOW = "YieldNow";
var interruptSignal = /* @__PURE__ */ __name((cause3) => ({
  _tag: OP_INTERRUPT_SIGNAL,
  cause: cause3
}), "interruptSignal");
var stateful = /* @__PURE__ */ __name((onFiber) => ({
  _tag: OP_STATEFUL,
  onFiber
}), "stateful");
var resume = /* @__PURE__ */ __name((effect) => ({
  _tag: OP_RESUME,
  effect
}), "resume");
var yieldNow3 = /* @__PURE__ */ __name(() => ({
  _tag: OP_YIELD_NOW
}), "yieldNow");

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/internal/fiberScope.js
var FiberScopeSymbolKey = "effect/FiberScope";
var FiberScopeTypeId = /* @__PURE__ */ Symbol.for(FiberScopeSymbolKey);
var Global = class {
  static {
    __name(this, "Global");
  }
  [FiberScopeTypeId] = FiberScopeTypeId;
  fiberId = none4;
  roots = /* @__PURE__ */ new Set();
  add(_runtimeFlags, child) {
    this.roots.add(child);
    child.addObserver(() => {
      this.roots.delete(child);
    });
  }
};
var Local = class {
  static {
    __name(this, "Local");
  }
  fiberId;
  parent;
  [FiberScopeTypeId] = FiberScopeTypeId;
  constructor(fiberId3, parent) {
    this.fiberId = fiberId3;
    this.parent = parent;
  }
  add(_runtimeFlags, child) {
    this.parent.tell(stateful((parentFiber) => {
      parentFiber.addChild(child);
      child.addObserver(() => {
        parentFiber.removeChild(child);
      });
    }));
  }
};
var unsafeMake6 = /* @__PURE__ */ __name((fiber) => {
  return new Local(fiber.id(), fiber);
}, "unsafeMake");
var globalScope = /* @__PURE__ */ globalValue(/* @__PURE__ */ Symbol.for("effect/FiberScope/Global"), () => new Global());

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/internal/fiber.js
var FiberSymbolKey = "effect/Fiber";
var FiberTypeId = /* @__PURE__ */ Symbol.for(FiberSymbolKey);
var fiberVariance2 = {
  /* c8 ignore next */
  _E: /* @__PURE__ */ __name((_) => _, "_E"),
  /* c8 ignore next */
  _A: /* @__PURE__ */ __name((_) => _, "_A")
};
var fiberProto = {
  [FiberTypeId]: fiberVariance2,
  pipe() {
    return pipeArguments(this, arguments);
  }
};
var RuntimeFiberSymbolKey = "effect/Fiber";
var RuntimeFiberTypeId = /* @__PURE__ */ Symbol.for(RuntimeFiberSymbolKey);
var isRuntimeFiber = /* @__PURE__ */ __name((self) => RuntimeFiberTypeId in self, "isRuntimeFiber");
var _await2 = /* @__PURE__ */ __name((self) => self.await, "_await");
var inheritAll = /* @__PURE__ */ __name((self) => self.inheritAll, "inheritAll");
var interruptAllAs = /* @__PURE__ */ dual(2, /* @__PURE__ */ fnUntraced(function* (fibers, fiberId3) {
  for (const fiber of fibers) {
    if (isRuntimeFiber(fiber)) {
      fiber.unsafeInterruptAsFork(fiberId3);
      continue;
    }
    yield* fiber.interruptAsFork(fiberId3);
  }
  for (const fiber of fibers) {
    if (isRuntimeFiber(fiber) && fiber.unsafePoll()) {
      continue;
    }
    yield* fiber.await;
  }
}));
var interruptAsFork = /* @__PURE__ */ dual(2, (self, fiberId3) => self.interruptAsFork(fiberId3));
var join2 = /* @__PURE__ */ __name((self) => zipLeft(flatten4(self.await), self.inheritAll), "join");
var _never = {
  ...CommitPrototype,
  commit() {
    return join2(this);
  },
  ...fiberProto,
  id: /* @__PURE__ */ __name(() => none4, "id"),
  await: never,
  children: /* @__PURE__ */ succeed([]),
  inheritAll: never,
  poll: /* @__PURE__ */ succeed(/* @__PURE__ */ none2()),
  interruptAsFork: /* @__PURE__ */ __name(() => never, "interruptAsFork")
};
var currentFiberURI = "effect/FiberCurrent";

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/internal/logger.js
var LoggerSymbolKey = "effect/Logger";
var LoggerTypeId = /* @__PURE__ */ Symbol.for(LoggerSymbolKey);
var loggerVariance = {
  /* c8 ignore next */
  _Message: /* @__PURE__ */ __name((_) => _, "_Message"),
  /* c8 ignore next */
  _Output: /* @__PURE__ */ __name((_) => _, "_Output")
};
var makeLogger = /* @__PURE__ */ __name((log3) => ({
  [LoggerTypeId]: loggerVariance,
  log: log3,
  pipe() {
    return pipeArguments(this, arguments);
  }
}), "makeLogger");
var none7 = {
  [LoggerTypeId]: loggerVariance,
  log: constVoid,
  pipe() {
    return pipeArguments(this, arguments);
  }
};
var textOnly = /^[^\s"=]*$/;
var format3 = /* @__PURE__ */ __name((quoteValue, whitespace) => ({
  annotations,
  cause: cause3,
  date,
  fiberId: fiberId3,
  logLevel,
  message,
  spans
}) => {
  const formatValue = /* @__PURE__ */ __name((value) => value.match(textOnly) ? value : quoteValue(value), "formatValue");
  const format4 = /* @__PURE__ */ __name((label, value) => `${formatLabel(label)}=${formatValue(value)}`, "format");
  const append4 = /* @__PURE__ */ __name((label, value) => " " + format4(label, value), "append");
  let out = format4("timestamp", date.toISOString());
  out += append4("level", logLevel.label);
  out += append4("fiber", threadName(fiberId3));
  const messages = ensure(message);
  for (let i = 0; i < messages.length; i++) {
    out += append4("message", toStringUnknown(messages[i], whitespace));
  }
  if (!isEmptyType(cause3)) {
    out += append4("cause", pretty(cause3, {
      renderErrorCause: true
    }));
  }
  for (const span2 of spans) {
    out += " " + render(date.getTime())(span2);
  }
  for (const [label, value] of annotations) {
    out += append4(label, toStringUnknown(value, whitespace));
  }
  return out;
}, "format");
var escapeDoubleQuotes = /* @__PURE__ */ __name((s) => `"${s.replace(/\\([\s\S])|(")/g, "\\$1$2")}"`, "escapeDoubleQuotes");
var stringLogger = /* @__PURE__ */ makeLogger(/* @__PURE__ */ format3(escapeDoubleQuotes));
var colors = {
  bold: "1",
  red: "31",
  green: "32",
  yellow: "33",
  blue: "34",
  cyan: "36",
  white: "37",
  gray: "90",
  black: "30",
  bgBrightRed: "101"
};
var logLevelColors = {
  None: [],
  All: [],
  Trace: [colors.gray],
  Debug: [colors.blue],
  Info: [colors.green],
  Warning: [colors.yellow],
  Error: [colors.red],
  Fatal: [colors.bgBrightRed, colors.black]
};
var hasProcessStdout = typeof process === "object" && process !== null && typeof process.stdout === "object" && process.stdout !== null;
var processStdoutIsTTY = hasProcessStdout && process.stdout.isTTY === true;
var hasProcessStdoutOrDeno = hasProcessStdout || "Deno" in globalThis;

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/internal/metric/boundaries.js
var MetricBoundariesSymbolKey = "effect/MetricBoundaries";
var MetricBoundariesTypeId = /* @__PURE__ */ Symbol.for(MetricBoundariesSymbolKey);
var MetricBoundariesImpl = class {
  static {
    __name(this, "MetricBoundariesImpl");
  }
  values;
  [MetricBoundariesTypeId] = MetricBoundariesTypeId;
  constructor(values3) {
    this.values = values3;
    this._hash = pipe(string(MetricBoundariesSymbolKey), combine(array2(this.values)));
  }
  _hash;
  [symbol]() {
    return this._hash;
  }
  [symbol2](u) {
    return isMetricBoundaries(u) && equals(this.values, u.values);
  }
  pipe() {
    return pipeArguments(this, arguments);
  }
};
var isMetricBoundaries = /* @__PURE__ */ __name((u) => hasProperty(u, MetricBoundariesTypeId), "isMetricBoundaries");
var fromIterable7 = /* @__PURE__ */ __name((iterable) => {
  const values3 = pipe(iterable, appendAll(of2(Number.POSITIVE_INFINITY)), dedupe);
  return new MetricBoundariesImpl(values3);
}, "fromIterable");
var exponential = /* @__PURE__ */ __name((options) => pipe(makeBy(options.count - 1, (i) => options.start * Math.pow(options.factor, i)), unsafeFromArray, fromIterable7), "exponential");

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/internal/metric/keyType.js
var MetricKeyTypeSymbolKey = "effect/MetricKeyType";
var MetricKeyTypeTypeId = /* @__PURE__ */ Symbol.for(MetricKeyTypeSymbolKey);
var CounterKeyTypeSymbolKey = "effect/MetricKeyType/Counter";
var CounterKeyTypeTypeId = /* @__PURE__ */ Symbol.for(CounterKeyTypeSymbolKey);
var FrequencyKeyTypeSymbolKey = "effect/MetricKeyType/Frequency";
var FrequencyKeyTypeTypeId = /* @__PURE__ */ Symbol.for(FrequencyKeyTypeSymbolKey);
var GaugeKeyTypeSymbolKey = "effect/MetricKeyType/Gauge";
var GaugeKeyTypeTypeId = /* @__PURE__ */ Symbol.for(GaugeKeyTypeSymbolKey);
var HistogramKeyTypeSymbolKey = "effect/MetricKeyType/Histogram";
var HistogramKeyTypeTypeId = /* @__PURE__ */ Symbol.for(HistogramKeyTypeSymbolKey);
var SummaryKeyTypeSymbolKey = "effect/MetricKeyType/Summary";
var SummaryKeyTypeTypeId = /* @__PURE__ */ Symbol.for(SummaryKeyTypeSymbolKey);
var metricKeyTypeVariance = {
  /* c8 ignore next */
  _In: /* @__PURE__ */ __name((_) => _, "_In"),
  /* c8 ignore next */
  _Out: /* @__PURE__ */ __name((_) => _, "_Out")
};
var CounterKeyType = class {
  static {
    __name(this, "CounterKeyType");
  }
  incremental;
  bigint;
  [MetricKeyTypeTypeId] = metricKeyTypeVariance;
  [CounterKeyTypeTypeId] = CounterKeyTypeTypeId;
  constructor(incremental, bigint) {
    this.incremental = incremental;
    this.bigint = bigint;
    this._hash = string(CounterKeyTypeSymbolKey);
  }
  _hash;
  [symbol]() {
    return this._hash;
  }
  [symbol2](that) {
    return isCounterKey(that);
  }
  pipe() {
    return pipeArguments(this, arguments);
  }
};
var FrequencyKeyTypeHash = /* @__PURE__ */ string(FrequencyKeyTypeSymbolKey);
var FrequencyKeyType = class {
  static {
    __name(this, "FrequencyKeyType");
  }
  preregisteredWords;
  [MetricKeyTypeTypeId] = metricKeyTypeVariance;
  [FrequencyKeyTypeTypeId] = FrequencyKeyTypeTypeId;
  constructor(preregisteredWords) {
    this.preregisteredWords = preregisteredWords;
  }
  [symbol]() {
    return FrequencyKeyTypeHash;
  }
  [symbol2](that) {
    return isFrequencyKey(that);
  }
  pipe() {
    return pipeArguments(this, arguments);
  }
};
var GaugeKeyTypeHash = /* @__PURE__ */ string(GaugeKeyTypeSymbolKey);
var GaugeKeyType = class {
  static {
    __name(this, "GaugeKeyType");
  }
  bigint;
  [MetricKeyTypeTypeId] = metricKeyTypeVariance;
  [GaugeKeyTypeTypeId] = GaugeKeyTypeTypeId;
  constructor(bigint) {
    this.bigint = bigint;
  }
  [symbol]() {
    return GaugeKeyTypeHash;
  }
  [symbol2](that) {
    return isGaugeKey(that);
  }
  pipe() {
    return pipeArguments(this, arguments);
  }
};
var HistogramKeyType = class {
  static {
    __name(this, "HistogramKeyType");
  }
  boundaries;
  [MetricKeyTypeTypeId] = metricKeyTypeVariance;
  [HistogramKeyTypeTypeId] = HistogramKeyTypeTypeId;
  constructor(boundaries) {
    this.boundaries = boundaries;
    this._hash = pipe(string(HistogramKeyTypeSymbolKey), combine(hash(this.boundaries)));
  }
  _hash;
  [symbol]() {
    return this._hash;
  }
  [symbol2](that) {
    return isHistogramKey(that) && equals(this.boundaries, that.boundaries);
  }
  pipe() {
    return pipeArguments(this, arguments);
  }
};
var SummaryKeyType = class {
  static {
    __name(this, "SummaryKeyType");
  }
  maxAge;
  maxSize;
  error;
  quantiles;
  [MetricKeyTypeTypeId] = metricKeyTypeVariance;
  [SummaryKeyTypeTypeId] = SummaryKeyTypeTypeId;
  constructor(maxAge, maxSize, error, quantiles) {
    this.maxAge = maxAge;
    this.maxSize = maxSize;
    this.error = error;
    this.quantiles = quantiles;
    this._hash = pipe(string(SummaryKeyTypeSymbolKey), combine(hash(this.maxAge)), combine(hash(this.maxSize)), combine(hash(this.error)), combine(array2(this.quantiles)));
  }
  _hash;
  [symbol]() {
    return this._hash;
  }
  [symbol2](that) {
    return isSummaryKey(that) && equals(this.maxAge, that.maxAge) && this.maxSize === that.maxSize && this.error === that.error && equals(this.quantiles, that.quantiles);
  }
  pipe() {
    return pipeArguments(this, arguments);
  }
};
var counter = /* @__PURE__ */ __name((options) => new CounterKeyType(options?.incremental ?? false, options?.bigint ?? false), "counter");
var histogram = /* @__PURE__ */ __name((boundaries) => {
  return new HistogramKeyType(boundaries);
}, "histogram");
var isCounterKey = /* @__PURE__ */ __name((u) => hasProperty(u, CounterKeyTypeTypeId), "isCounterKey");
var isFrequencyKey = /* @__PURE__ */ __name((u) => hasProperty(u, FrequencyKeyTypeTypeId), "isFrequencyKey");
var isGaugeKey = /* @__PURE__ */ __name((u) => hasProperty(u, GaugeKeyTypeTypeId), "isGaugeKey");
var isHistogramKey = /* @__PURE__ */ __name((u) => hasProperty(u, HistogramKeyTypeTypeId), "isHistogramKey");
var isSummaryKey = /* @__PURE__ */ __name((u) => hasProperty(u, SummaryKeyTypeTypeId), "isSummaryKey");

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/internal/metric/key.js
var MetricKeySymbolKey = "effect/MetricKey";
var MetricKeyTypeId = /* @__PURE__ */ Symbol.for(MetricKeySymbolKey);
var metricKeyVariance = {
  /* c8 ignore next */
  _Type: /* @__PURE__ */ __name((_) => _, "_Type")
};
var arrayEquivilence = /* @__PURE__ */ getEquivalence(equals);
var MetricKeyImpl = class {
  static {
    __name(this, "MetricKeyImpl");
  }
  name;
  keyType;
  description;
  tags;
  [MetricKeyTypeId] = metricKeyVariance;
  constructor(name, keyType, description, tags = []) {
    this.name = name;
    this.keyType = keyType;
    this.description = description;
    this.tags = tags;
    this._hash = pipe(string(this.name + this.description), combine(hash(this.keyType)), combine(array2(this.tags)));
  }
  _hash;
  [symbol]() {
    return this._hash;
  }
  [symbol2](u) {
    return isMetricKey(u) && this.name === u.name && equals(this.keyType, u.keyType) && equals(this.description, u.description) && arrayEquivilence(this.tags, u.tags);
  }
  pipe() {
    return pipeArguments(this, arguments);
  }
};
var isMetricKey = /* @__PURE__ */ __name((u) => hasProperty(u, MetricKeyTypeId), "isMetricKey");
var counter2 = /* @__PURE__ */ __name((name, options) => new MetricKeyImpl(name, counter(options), fromNullable(options?.description)), "counter");
var histogram2 = /* @__PURE__ */ __name((name, boundaries, description) => new MetricKeyImpl(name, histogram(boundaries), fromNullable(description)), "histogram");
var taggedWithLabels = /* @__PURE__ */ dual(2, (self, extraTags) => extraTags.length === 0 ? self : new MetricKeyImpl(self.name, self.keyType, self.description, union(self.tags, extraTags)));

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/internal/metric/state.js
var MetricStateSymbolKey = "effect/MetricState";
var MetricStateTypeId = /* @__PURE__ */ Symbol.for(MetricStateSymbolKey);
var CounterStateSymbolKey = "effect/MetricState/Counter";
var CounterStateTypeId = /* @__PURE__ */ Symbol.for(CounterStateSymbolKey);
var FrequencyStateSymbolKey = "effect/MetricState/Frequency";
var FrequencyStateTypeId = /* @__PURE__ */ Symbol.for(FrequencyStateSymbolKey);
var GaugeStateSymbolKey = "effect/MetricState/Gauge";
var GaugeStateTypeId = /* @__PURE__ */ Symbol.for(GaugeStateSymbolKey);
var HistogramStateSymbolKey = "effect/MetricState/Histogram";
var HistogramStateTypeId = /* @__PURE__ */ Symbol.for(HistogramStateSymbolKey);
var SummaryStateSymbolKey = "effect/MetricState/Summary";
var SummaryStateTypeId = /* @__PURE__ */ Symbol.for(SummaryStateSymbolKey);
var metricStateVariance = {
  /* c8 ignore next */
  _A: /* @__PURE__ */ __name((_) => _, "_A")
};
var CounterState = class {
  static {
    __name(this, "CounterState");
  }
  count;
  [MetricStateTypeId] = metricStateVariance;
  [CounterStateTypeId] = CounterStateTypeId;
  constructor(count) {
    this.count = count;
  }
  [symbol]() {
    return pipe(hash(CounterStateSymbolKey), combine(hash(this.count)), cached(this));
  }
  [symbol2](that) {
    return isCounterState(that) && this.count === that.count;
  }
  pipe() {
    return pipeArguments(this, arguments);
  }
};
var arrayEquals = /* @__PURE__ */ getEquivalence(equals);
var FrequencyState = class {
  static {
    __name(this, "FrequencyState");
  }
  occurrences;
  [MetricStateTypeId] = metricStateVariance;
  [FrequencyStateTypeId] = FrequencyStateTypeId;
  constructor(occurrences) {
    this.occurrences = occurrences;
  }
  _hash;
  [symbol]() {
    return pipe(string(FrequencyStateSymbolKey), combine(array2(fromIterable(this.occurrences.entries()))), cached(this));
  }
  [symbol2](that) {
    return isFrequencyState(that) && arrayEquals(fromIterable(this.occurrences.entries()), fromIterable(that.occurrences.entries()));
  }
  pipe() {
    return pipeArguments(this, arguments);
  }
};
var GaugeState = class {
  static {
    __name(this, "GaugeState");
  }
  value;
  [MetricStateTypeId] = metricStateVariance;
  [GaugeStateTypeId] = GaugeStateTypeId;
  constructor(value) {
    this.value = value;
  }
  [symbol]() {
    return pipe(hash(GaugeStateSymbolKey), combine(hash(this.value)), cached(this));
  }
  [symbol2](u) {
    return isGaugeState(u) && this.value === u.value;
  }
  pipe() {
    return pipeArguments(this, arguments);
  }
};
var HistogramState = class {
  static {
    __name(this, "HistogramState");
  }
  buckets;
  count;
  min;
  max;
  sum;
  [MetricStateTypeId] = metricStateVariance;
  [HistogramStateTypeId] = HistogramStateTypeId;
  constructor(buckets, count, min4, max6, sum2) {
    this.buckets = buckets;
    this.count = count;
    this.min = min4;
    this.max = max6;
    this.sum = sum2;
  }
  [symbol]() {
    return pipe(hash(HistogramStateSymbolKey), combine(hash(this.buckets)), combine(hash(this.count)), combine(hash(this.min)), combine(hash(this.max)), combine(hash(this.sum)), cached(this));
  }
  [symbol2](that) {
    return isHistogramState(that) && equals(this.buckets, that.buckets) && this.count === that.count && this.min === that.min && this.max === that.max && this.sum === that.sum;
  }
  pipe() {
    return pipeArguments(this, arguments);
  }
};
var SummaryState = class {
  static {
    __name(this, "SummaryState");
  }
  error;
  quantiles;
  count;
  min;
  max;
  sum;
  [MetricStateTypeId] = metricStateVariance;
  [SummaryStateTypeId] = SummaryStateTypeId;
  constructor(error, quantiles, count, min4, max6, sum2) {
    this.error = error;
    this.quantiles = quantiles;
    this.count = count;
    this.min = min4;
    this.max = max6;
    this.sum = sum2;
  }
  [symbol]() {
    return pipe(hash(SummaryStateSymbolKey), combine(hash(this.error)), combine(hash(this.quantiles)), combine(hash(this.count)), combine(hash(this.min)), combine(hash(this.max)), combine(hash(this.sum)), cached(this));
  }
  [symbol2](that) {
    return isSummaryState(that) && this.error === that.error && equals(this.quantiles, that.quantiles) && this.count === that.count && this.min === that.min && this.max === that.max && this.sum === that.sum;
  }
  pipe() {
    return pipeArguments(this, arguments);
  }
};
var counter3 = /* @__PURE__ */ __name((count) => new CounterState(count), "counter");
var frequency2 = /* @__PURE__ */ __name((occurrences) => {
  return new FrequencyState(occurrences);
}, "frequency");
var gauge2 = /* @__PURE__ */ __name((count) => new GaugeState(count), "gauge");
var histogram3 = /* @__PURE__ */ __name((options) => new HistogramState(options.buckets, options.count, options.min, options.max, options.sum), "histogram");
var summary2 = /* @__PURE__ */ __name((options) => new SummaryState(options.error, options.quantiles, options.count, options.min, options.max, options.sum), "summary");
var isCounterState = /* @__PURE__ */ __name((u) => hasProperty(u, CounterStateTypeId), "isCounterState");
var isFrequencyState = /* @__PURE__ */ __name((u) => hasProperty(u, FrequencyStateTypeId), "isFrequencyState");
var isGaugeState = /* @__PURE__ */ __name((u) => hasProperty(u, GaugeStateTypeId), "isGaugeState");
var isHistogramState = /* @__PURE__ */ __name((u) => hasProperty(u, HistogramStateTypeId), "isHistogramState");
var isSummaryState = /* @__PURE__ */ __name((u) => hasProperty(u, SummaryStateTypeId), "isSummaryState");

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/internal/metric/hook.js
var MetricHookSymbolKey = "effect/MetricHook";
var MetricHookTypeId = /* @__PURE__ */ Symbol.for(MetricHookSymbolKey);
var metricHookVariance = {
  /* c8 ignore next */
  _In: /* @__PURE__ */ __name((_) => _, "_In"),
  /* c8 ignore next */
  _Out: /* @__PURE__ */ __name((_) => _, "_Out")
};
var make29 = /* @__PURE__ */ __name((options) => ({
  [MetricHookTypeId]: metricHookVariance,
  pipe() {
    return pipeArguments(this, arguments);
  },
  ...options
}), "make");
var bigint03 = /* @__PURE__ */ BigInt(0);
var counter4 = /* @__PURE__ */ __name((key) => {
  let sum2 = key.keyType.bigint ? bigint03 : 0;
  const canUpdate = key.keyType.incremental ? key.keyType.bigint ? (value) => value >= bigint03 : (value) => value >= 0 : (_value) => true;
  const update5 = /* @__PURE__ */ __name((value) => {
    if (canUpdate(value)) {
      sum2 = sum2 + value;
    }
  }, "update");
  return make29({
    get: /* @__PURE__ */ __name(() => counter3(sum2), "get"),
    update: update5,
    modify: update5
  });
}, "counter");
var frequency3 = /* @__PURE__ */ __name((key) => {
  const values3 = /* @__PURE__ */ new Map();
  for (const word of key.keyType.preregisteredWords) {
    values3.set(word, 0);
  }
  const update5 = /* @__PURE__ */ __name((word) => {
    const slotCount = values3.get(word) ?? 0;
    values3.set(word, slotCount + 1);
  }, "update");
  return make29({
    get: /* @__PURE__ */ __name(() => frequency2(values3), "get"),
    update: update5,
    modify: update5
  });
}, "frequency");
var gauge3 = /* @__PURE__ */ __name((_key, startAt) => {
  let value = startAt;
  return make29({
    get: /* @__PURE__ */ __name(() => gauge2(value), "get"),
    update: /* @__PURE__ */ __name((v) => {
      value = v;
    }, "update"),
    modify: /* @__PURE__ */ __name((v) => {
      value = value + v;
    }, "modify")
  });
}, "gauge");
var histogram4 = /* @__PURE__ */ __name((key) => {
  const bounds = key.keyType.boundaries.values;
  const size11 = bounds.length;
  const values3 = new Uint32Array(size11 + 1);
  const boundaries = new Float64Array(size11);
  let count = 0;
  let sum2 = 0;
  let min4 = Number.MAX_VALUE;
  let max6 = Number.MIN_VALUE;
  pipe(bounds, sort(Order), map2((n, i) => {
    boundaries[i] = n;
  }));
  const update5 = /* @__PURE__ */ __name((value) => {
    let from = 0;
    let to = size11;
    while (from !== to) {
      const mid = Math.floor(from + (to - from) / 2);
      const boundary = boundaries[mid];
      if (value <= boundary) {
        to = mid;
      } else {
        from = mid;
      }
      if (to === from + 1) {
        if (value <= boundaries[from]) {
          to = from;
        } else {
          from = to;
        }
      }
    }
    values3[from] = values3[from] + 1;
    count = count + 1;
    sum2 = sum2 + value;
    if (value < min4) {
      min4 = value;
    }
    if (value > max6) {
      max6 = value;
    }
  }, "update");
  const getBuckets = /* @__PURE__ */ __name(() => {
    const builder = allocate(size11);
    let cumulated = 0;
    for (let i = 0; i < size11; i++) {
      const boundary = boundaries[i];
      const value = values3[i];
      cumulated = cumulated + value;
      builder[i] = [boundary, cumulated];
    }
    return builder;
  }, "getBuckets");
  return make29({
    get: /* @__PURE__ */ __name(() => histogram3({
      buckets: getBuckets(),
      count,
      min: min4,
      max: max6,
      sum: sum2
    }), "get"),
    update: update5,
    modify: update5
  });
}, "histogram");
var summary3 = /* @__PURE__ */ __name((key) => {
  const {
    error,
    maxAge,
    maxSize,
    quantiles
  } = key.keyType;
  const sortedQuantiles = pipe(quantiles, sort(Order));
  const values3 = allocate(maxSize);
  let head5 = 0;
  let count = 0;
  let sum2 = 0;
  let min4 = 0;
  let max6 = 0;
  const snapshot = /* @__PURE__ */ __name((now) => {
    const builder = [];
    let i = 0;
    while (i !== maxSize - 1) {
      const item = values3[i];
      if (item != null) {
        const [t, v] = item;
        const age = millis(now - t);
        if (greaterThanOrEqualTo2(age, zero) && lessThanOrEqualTo2(age, maxAge)) {
          builder.push(v);
        }
      }
      i = i + 1;
    }
    return calculateQuantiles(error, sortedQuantiles, sort(builder, Order));
  }, "snapshot");
  const observe = /* @__PURE__ */ __name((value, timestamp) => {
    if (maxSize > 0) {
      head5 = head5 + 1;
      const target = head5 % maxSize;
      values3[target] = [timestamp, value];
    }
    min4 = count === 0 ? value : Math.min(min4, value);
    max6 = count === 0 ? value : Math.max(max6, value);
    count = count + 1;
    sum2 = sum2 + value;
  }, "observe");
  return make29({
    get: /* @__PURE__ */ __name(() => summary2({
      error,
      quantiles: snapshot(Date.now()),
      count,
      min: min4,
      max: max6,
      sum: sum2
    }), "get"),
    update: /* @__PURE__ */ __name(([value, timestamp]) => observe(value, timestamp), "update"),
    modify: /* @__PURE__ */ __name(([value, timestamp]) => observe(value, timestamp), "modify")
  });
}, "summary");
var calculateQuantiles = /* @__PURE__ */ __name((error, sortedQuantiles, sortedSamples) => {
  const sampleCount = sortedSamples.length;
  if (!isNonEmptyReadonlyArray(sortedQuantiles)) {
    return empty();
  }
  const head5 = sortedQuantiles[0];
  const tail = sortedQuantiles.slice(1);
  const resolvedHead = resolveQuantile(error, sampleCount, none2(), 0, head5, sortedSamples);
  const resolved = of(resolvedHead);
  tail.forEach((quantile) => {
    resolved.push(resolveQuantile(error, sampleCount, resolvedHead.value, resolvedHead.consumed, quantile, resolvedHead.rest));
  });
  return map2(resolved, (rq) => [rq.quantile, rq.value]);
}, "calculateQuantiles");
var resolveQuantile = /* @__PURE__ */ __name((error, sampleCount, current, consumed, quantile, rest) => {
  let error_1 = error;
  let sampleCount_1 = sampleCount;
  let current_1 = current;
  let consumed_1 = consumed;
  let quantile_1 = quantile;
  let rest_1 = rest;
  let error_2 = error;
  let sampleCount_2 = sampleCount;
  let current_2 = current;
  let consumed_2 = consumed;
  let quantile_2 = quantile;
  let rest_2 = rest;
  while (1) {
    if (!isNonEmptyReadonlyArray(rest_1)) {
      return {
        quantile: quantile_1,
        value: none2(),
        consumed: consumed_1,
        rest: []
      };
    }
    if (quantile_1 === 1) {
      return {
        quantile: quantile_1,
        value: some2(lastNonEmpty(rest_1)),
        consumed: consumed_1 + rest_1.length,
        rest: []
      };
    }
    const headValue = headNonEmpty(rest_1);
    const sameHead = span(rest_1, (n) => n === headValue);
    const desired = quantile_1 * sampleCount_1;
    const allowedError = error_1 / 2 * desired;
    const candConsumed = consumed_1 + sameHead[0].length;
    const candError = Math.abs(candConsumed - desired);
    if (candConsumed < desired - allowedError) {
      error_2 = error_1;
      sampleCount_2 = sampleCount_1;
      current_2 = head(rest_1);
      consumed_2 = candConsumed;
      quantile_2 = quantile_1;
      rest_2 = sameHead[1];
      error_1 = error_2;
      sampleCount_1 = sampleCount_2;
      current_1 = current_2;
      consumed_1 = consumed_2;
      quantile_1 = quantile_2;
      rest_1 = rest_2;
      continue;
    }
    if (candConsumed > desired + allowedError) {
      const valueToReturn = isNone2(current_1) ? some2(headValue) : current_1;
      return {
        quantile: quantile_1,
        value: valueToReturn,
        consumed: consumed_1,
        rest: rest_1
      };
    }
    switch (current_1._tag) {
      case "None": {
        error_2 = error_1;
        sampleCount_2 = sampleCount_1;
        current_2 = head(rest_1);
        consumed_2 = candConsumed;
        quantile_2 = quantile_1;
        rest_2 = sameHead[1];
        error_1 = error_2;
        sampleCount_1 = sampleCount_2;
        current_1 = current_2;
        consumed_1 = consumed_2;
        quantile_1 = quantile_2;
        rest_1 = rest_2;
        continue;
      }
      case "Some": {
        const prevError = Math.abs(desired - current_1.value);
        if (candError < prevError) {
          error_2 = error_1;
          sampleCount_2 = sampleCount_1;
          current_2 = head(rest_1);
          consumed_2 = candConsumed;
          quantile_2 = quantile_1;
          rest_2 = sameHead[1];
          error_1 = error_2;
          sampleCount_1 = sampleCount_2;
          current_1 = current_2;
          consumed_1 = consumed_2;
          quantile_1 = quantile_2;
          rest_1 = rest_2;
          continue;
        }
        return {
          quantile: quantile_1,
          value: some2(current_1.value),
          consumed: consumed_1,
          rest: rest_1
        };
      }
    }
  }
  throw new Error("BUG: MetricHook.resolveQuantiles - please report an issue at https://github.com/Effect-TS/effect/issues");
}, "resolveQuantile");

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/internal/metric/pair.js
var MetricPairSymbolKey = "effect/MetricPair";
var MetricPairTypeId = /* @__PURE__ */ Symbol.for(MetricPairSymbolKey);
var metricPairVariance = {
  /* c8 ignore next */
  _Type: /* @__PURE__ */ __name((_) => _, "_Type")
};
var unsafeMake7 = /* @__PURE__ */ __name((metricKey, metricState) => {
  return {
    [MetricPairTypeId]: metricPairVariance,
    metricKey,
    metricState,
    pipe() {
      return pipeArguments(this, arguments);
    }
  };
}, "unsafeMake");

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/internal/metric/registry.js
var MetricRegistrySymbolKey = "effect/MetricRegistry";
var MetricRegistryTypeId = /* @__PURE__ */ Symbol.for(MetricRegistrySymbolKey);
var MetricRegistryImpl = class {
  static {
    __name(this, "MetricRegistryImpl");
  }
  [MetricRegistryTypeId] = MetricRegistryTypeId;
  map = /* @__PURE__ */ empty17();
  snapshot() {
    const result = [];
    for (const [key, hook] of this.map) {
      result.push(unsafeMake7(key, hook.get()));
    }
    return result;
  }
  get(key) {
    const hook = pipe(this.map, get8(key), getOrUndefined);
    if (hook == null) {
      if (isCounterKey(key.keyType)) {
        return this.getCounter(key);
      }
      if (isGaugeKey(key.keyType)) {
        return this.getGauge(key);
      }
      if (isFrequencyKey(key.keyType)) {
        return this.getFrequency(key);
      }
      if (isHistogramKey(key.keyType)) {
        return this.getHistogram(key);
      }
      if (isSummaryKey(key.keyType)) {
        return this.getSummary(key);
      }
      throw new Error("BUG: MetricRegistry.get - unknown MetricKeyType - please report an issue at https://github.com/Effect-TS/effect/issues");
    } else {
      return hook;
    }
  }
  getCounter(key) {
    let value = pipe(this.map, get8(key), getOrUndefined);
    if (value == null) {
      const counter6 = counter4(key);
      if (!pipe(this.map, has4(key))) {
        pipe(this.map, set4(key, counter6));
      }
      value = counter6;
    }
    return value;
  }
  getFrequency(key) {
    let value = pipe(this.map, get8(key), getOrUndefined);
    if (value == null) {
      const frequency5 = frequency3(key);
      if (!pipe(this.map, has4(key))) {
        pipe(this.map, set4(key, frequency5));
      }
      value = frequency5;
    }
    return value;
  }
  getGauge(key) {
    let value = pipe(this.map, get8(key), getOrUndefined);
    if (value == null) {
      const gauge5 = gauge3(key, key.keyType.bigint ? BigInt(0) : 0);
      if (!pipe(this.map, has4(key))) {
        pipe(this.map, set4(key, gauge5));
      }
      value = gauge5;
    }
    return value;
  }
  getHistogram(key) {
    let value = pipe(this.map, get8(key), getOrUndefined);
    if (value == null) {
      const histogram6 = histogram4(key);
      if (!pipe(this.map, has4(key))) {
        pipe(this.map, set4(key, histogram6));
      }
      value = histogram6;
    }
    return value;
  }
  getSummary(key) {
    let value = pipe(this.map, get8(key), getOrUndefined);
    if (value == null) {
      const summary5 = summary3(key);
      if (!pipe(this.map, has4(key))) {
        pipe(this.map, set4(key, summary5));
      }
      value = summary5;
    }
    return value;
  }
};
var make30 = /* @__PURE__ */ __name(() => {
  return new MetricRegistryImpl();
}, "make");

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/internal/metric.js
var MetricSymbolKey = "effect/Metric";
var MetricTypeId = /* @__PURE__ */ Symbol.for(MetricSymbolKey);
var metricVariance = {
  /* c8 ignore next */
  _Type: /* @__PURE__ */ __name((_) => _, "_Type"),
  /* c8 ignore next */
  _In: /* @__PURE__ */ __name((_) => _, "_In"),
  /* c8 ignore next */
  _Out: /* @__PURE__ */ __name((_) => _, "_Out")
};
var globalMetricRegistry = /* @__PURE__ */ globalValue(/* @__PURE__ */ Symbol.for("effect/Metric/globalMetricRegistry"), () => make30());
var make31 = /* @__PURE__ */ __name(function(keyType, unsafeUpdate, unsafeValue, unsafeModify) {
  const metric = Object.assign((effect) => tap(effect, (a) => update4(metric, a)), {
    [MetricTypeId]: metricVariance,
    keyType,
    unsafeUpdate,
    unsafeValue,
    unsafeModify,
    register() {
      this.unsafeValue([]);
      return this;
    },
    pipe() {
      return pipeArguments(this, arguments);
    }
  });
  return metric;
}, "make");
var counter5 = /* @__PURE__ */ __name((name, options) => fromMetricKey(counter2(name, options)), "counter");
var fromMetricKey = /* @__PURE__ */ __name((key) => {
  let untaggedHook;
  const hookCache = /* @__PURE__ */ new WeakMap();
  const hook = /* @__PURE__ */ __name((extraTags) => {
    if (extraTags.length === 0) {
      if (untaggedHook !== void 0) {
        return untaggedHook;
      }
      untaggedHook = globalMetricRegistry.get(key);
      return untaggedHook;
    }
    let hook2 = hookCache.get(extraTags);
    if (hook2 !== void 0) {
      return hook2;
    }
    hook2 = globalMetricRegistry.get(taggedWithLabels(key, extraTags));
    hookCache.set(extraTags, hook2);
    return hook2;
  }, "hook");
  return make31(key.keyType, (input, extraTags) => hook(extraTags).update(input), (extraTags) => hook(extraTags).get(), (input, extraTags) => hook(extraTags).modify(input));
}, "fromMetricKey");
var histogram5 = /* @__PURE__ */ __name((name, boundaries, description) => fromMetricKey(histogram2(name, boundaries, description)), "histogram");
var tagged = /* @__PURE__ */ dual(3, (self, key, value) => taggedWithLabels2(self, [make28(key, value)]));
var taggedWithLabels2 = /* @__PURE__ */ dual(2, (self, extraTags) => {
  return make31(self.keyType, (input, extraTags1) => self.unsafeUpdate(input, union(extraTags, extraTags1)), (extraTags1) => self.unsafeValue(union(extraTags, extraTags1)), (input, extraTags1) => self.unsafeModify(input, union(extraTags, extraTags1)));
});
var update4 = /* @__PURE__ */ dual(2, (self, input) => fiberRefGetWith(currentMetricLabels, (tags) => sync(() => self.unsafeUpdate(input, tags))));

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/internal/request.js
var RequestSymbolKey = "effect/Request";
var RequestTypeId = /* @__PURE__ */ Symbol.for(RequestSymbolKey);
var requestVariance = {
  /* c8 ignore next */
  _E: /* @__PURE__ */ __name((_) => _, "_E"),
  /* c8 ignore next */
  _A: /* @__PURE__ */ __name((_) => _, "_A")
};
var RequestPrototype = {
  ...StructuralPrototype,
  [RequestTypeId]: requestVariance
};
var isRequest = /* @__PURE__ */ __name((u) => hasProperty(u, RequestTypeId), "isRequest");
var complete = /* @__PURE__ */ dual(2, (self, result) => fiberRefGetWith(currentRequestMap, (map14) => sync(() => {
  if (map14.has(self)) {
    const entry = map14.get(self);
    if (!entry.state.completed) {
      entry.state.completed = true;
      deferredUnsafeDone(entry.result, result);
    }
  }
})));
var Listeners = class {
  static {
    __name(this, "Listeners");
  }
  count = 0;
  observers = /* @__PURE__ */ new Set();
  interrupted = false;
  addObserver(f) {
    this.observers.add(f);
  }
  removeObserver(f) {
    this.observers.delete(f);
  }
  increment() {
    this.count++;
    this.observers.forEach((f) => f(this.count));
  }
  decrement() {
    this.count--;
    this.observers.forEach((f) => f(this.count));
  }
};

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/internal/redBlackTree/iterator.js
var Direction = {
  Forward: 0,
  Backward: 1 << 0
};
var RedBlackTreeIterator = class _RedBlackTreeIterator {
  static {
    __name(this, "RedBlackTreeIterator");
  }
  self;
  stack;
  direction;
  count = 0;
  constructor(self, stack, direction) {
    this.self = self;
    this.stack = stack;
    this.direction = direction;
  }
  /**
   * Clones the iterator
   */
  clone() {
    return new _RedBlackTreeIterator(this.self, this.stack.slice(), this.direction);
  }
  /**
   * Reverse the traversal direction
   */
  reversed() {
    return new _RedBlackTreeIterator(this.self, this.stack.slice(), this.direction === Direction.Forward ? Direction.Backward : Direction.Forward);
  }
  /**
   * Iterator next
   */
  next() {
    const entry = this.entry;
    this.count++;
    if (this.direction === Direction.Forward) {
      this.moveNext();
    } else {
      this.movePrev();
    }
    switch (entry._tag) {
      case "None": {
        return {
          done: true,
          value: this.count
        };
      }
      case "Some": {
        return {
          done: false,
          value: entry.value
        };
      }
    }
  }
  /**
   * Returns the key
   */
  get key() {
    if (this.stack.length > 0) {
      return some2(this.stack[this.stack.length - 1].key);
    }
    return none2();
  }
  /**
   * Returns the value
   */
  get value() {
    if (this.stack.length > 0) {
      return some2(this.stack[this.stack.length - 1].value);
    }
    return none2();
  }
  /**
   * Returns the key
   */
  get entry() {
    return map(last(this.stack), (node) => [node.key, node.value]);
  }
  /**
   * Returns the position of this iterator in the sorted list
   */
  get index() {
    let idx = 0;
    const stack = this.stack;
    if (stack.length === 0) {
      const r = this.self._root;
      if (r != null) {
        return r.count;
      }
      return 0;
    } else if (stack[stack.length - 1].left != null) {
      idx = stack[stack.length - 1].left.count;
    }
    for (let s = stack.length - 2; s >= 0; --s) {
      if (stack[s + 1] === stack[s].right) {
        ++idx;
        if (stack[s].left != null) {
          idx += stack[s].left.count;
        }
      }
    }
    return idx;
  }
  /**
   * Advances iterator to next element in list
   */
  moveNext() {
    const stack = this.stack;
    if (stack.length === 0) {
      return;
    }
    let n = stack[stack.length - 1];
    if (n.right != null) {
      n = n.right;
      while (n != null) {
        stack.push(n);
        n = n.left;
      }
    } else {
      stack.pop();
      while (stack.length > 0 && stack[stack.length - 1].right === n) {
        n = stack[stack.length - 1];
        stack.pop();
      }
    }
  }
  /**
   * Checks if there is a next element
   */
  get hasNext() {
    const stack = this.stack;
    if (stack.length === 0) {
      return false;
    }
    if (stack[stack.length - 1].right != null) {
      return true;
    }
    for (let s = stack.length - 1; s > 0; --s) {
      if (stack[s - 1].left === stack[s]) {
        return true;
      }
    }
    return false;
  }
  /**
   * Advances iterator to previous element in list
   */
  movePrev() {
    const stack = this.stack;
    if (stack.length === 0) {
      return;
    }
    let n = stack[stack.length - 1];
    if (n != null && n.left != null) {
      n = n.left;
      while (n != null) {
        stack.push(n);
        n = n.right;
      }
    } else {
      stack.pop();
      while (stack.length > 0 && stack[stack.length - 1].left === n) {
        n = stack[stack.length - 1];
        stack.pop();
      }
    }
  }
  /**
   * Checks if there is a previous element
   */
  get hasPrev() {
    const stack = this.stack;
    if (stack.length === 0) {
      return false;
    }
    if (stack[stack.length - 1].left != null) {
      return true;
    }
    for (let s = stack.length - 1; s > 0; --s) {
      if (stack[s - 1].right === stack[s]) {
        return true;
      }
    }
    return false;
  }
};

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/internal/redBlackTree/node.js
var Color = {
  Red: 0,
  Black: 1 << 0
};
var clone = /* @__PURE__ */ __name(({
  color,
  count,
  key,
  left: left3,
  right: right3,
  value
}) => ({
  color,
  key,
  value,
  left: left3,
  right: right3,
  count
}), "clone");
function swap2(n, v) {
  n.key = v.key;
  n.value = v.value;
  n.left = v.left;
  n.right = v.right;
  n.color = v.color;
  n.count = v.count;
}
__name(swap2, "swap");
var repaint = /* @__PURE__ */ __name(({
  count,
  key,
  left: left3,
  right: right3,
  value
}, color) => ({
  color,
  key,
  value,
  left: left3,
  right: right3,
  count
}), "repaint");
var recount = /* @__PURE__ */ __name((node) => {
  node.count = 1 + (node.left?.count ?? 0) + (node.right?.count ?? 0);
}, "recount");

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/internal/redBlackTree.js
var RedBlackTreeSymbolKey = "effect/RedBlackTree";
var RedBlackTreeTypeId = /* @__PURE__ */ Symbol.for(RedBlackTreeSymbolKey);
var redBlackTreeVariance = {
  /* c8 ignore next */
  _Key: /* @__PURE__ */ __name((_) => _, "_Key"),
  /* c8 ignore next */
  _Value: /* @__PURE__ */ __name((_) => _, "_Value")
};
var RedBlackTreeProto = {
  [RedBlackTreeTypeId]: redBlackTreeVariance,
  [symbol]() {
    let hash2 = hash(RedBlackTreeSymbolKey);
    for (const item of this) {
      hash2 ^= pipe(hash(item[0]), combine(hash(item[1])));
    }
    return cached(this, hash2);
  },
  [symbol2](that) {
    if (isRedBlackTree(that)) {
      if ((this._root?.count ?? 0) !== (that._root?.count ?? 0)) {
        return false;
      }
      const entries2 = Array.from(that);
      return Array.from(this).every((itemSelf, i) => {
        const itemThat = entries2[i];
        return equals(itemSelf[0], itemThat[0]) && equals(itemSelf[1], itemThat[1]);
      });
    }
    return false;
  },
  [Symbol.iterator]() {
    const stack = [];
    let n = this._root;
    while (n != null) {
      stack.push(n);
      n = n.left;
    }
    return new RedBlackTreeIterator(this, stack, Direction.Forward);
  },
  toString() {
    return format(this.toJSON());
  },
  toJSON() {
    return {
      _id: "RedBlackTree",
      values: Array.from(this).map(toJSON)
    };
  },
  [NodeInspectSymbol]() {
    return this.toJSON();
  },
  pipe() {
    return pipeArguments(this, arguments);
  }
};
var makeImpl3 = /* @__PURE__ */ __name((ord, root) => {
  const tree = Object.create(RedBlackTreeProto);
  tree._ord = ord;
  tree._root = root;
  return tree;
}, "makeImpl");
var isRedBlackTree = /* @__PURE__ */ __name((u) => hasProperty(u, RedBlackTreeTypeId), "isRedBlackTree");
var findFirst4 = /* @__PURE__ */ dual(2, (self, key) => {
  const cmp = self._ord;
  let node = self._root;
  while (node !== void 0) {
    const d = cmp(key, node.key);
    if (equals(key, node.key)) {
      return some2(node.value);
    }
    if (d <= 0) {
      node = node.left;
    } else {
      node = node.right;
    }
  }
  return none2();
});
var has5 = /* @__PURE__ */ dual(2, (self, key) => isSome2(findFirst4(self, key)));
var insert = /* @__PURE__ */ dual(3, (self, key, value) => {
  const cmp = self._ord;
  let n = self._root;
  const n_stack = [];
  const d_stack = [];
  while (n != null) {
    const d = cmp(key, n.key);
    n_stack.push(n);
    d_stack.push(d);
    if (d <= 0) {
      n = n.left;
    } else {
      n = n.right;
    }
  }
  n_stack.push({
    color: Color.Red,
    key,
    value,
    left: void 0,
    right: void 0,
    count: 1
  });
  for (let s = n_stack.length - 2; s >= 0; --s) {
    const n2 = n_stack[s];
    if (d_stack[s] <= 0) {
      n_stack[s] = {
        color: n2.color,
        key: n2.key,
        value: n2.value,
        left: n_stack[s + 1],
        right: n2.right,
        count: n2.count + 1
      };
    } else {
      n_stack[s] = {
        color: n2.color,
        key: n2.key,
        value: n2.value,
        left: n2.left,
        right: n_stack[s + 1],
        count: n2.count + 1
      };
    }
  }
  for (let s = n_stack.length - 1; s > 1; --s) {
    const p = n_stack[s - 1];
    const n3 = n_stack[s];
    if (p.color === Color.Black || n3.color === Color.Black) {
      break;
    }
    const pp = n_stack[s - 2];
    if (pp.left === p) {
      if (p.left === n3) {
        const y = pp.right;
        if (y && y.color === Color.Red) {
          p.color = Color.Black;
          pp.right = repaint(y, Color.Black);
          pp.color = Color.Red;
          s -= 1;
        } else {
          pp.color = Color.Red;
          pp.left = p.right;
          p.color = Color.Black;
          p.right = pp;
          n_stack[s - 2] = p;
          n_stack[s - 1] = n3;
          recount(pp);
          recount(p);
          if (s >= 3) {
            const ppp = n_stack[s - 3];
            if (ppp.left === pp) {
              ppp.left = p;
            } else {
              ppp.right = p;
            }
          }
          break;
        }
      } else {
        const y = pp.right;
        if (y && y.color === Color.Red) {
          p.color = Color.Black;
          pp.right = repaint(y, Color.Black);
          pp.color = Color.Red;
          s -= 1;
        } else {
          p.right = n3.left;
          pp.color = Color.Red;
          pp.left = n3.right;
          n3.color = Color.Black;
          n3.left = p;
          n3.right = pp;
          n_stack[s - 2] = n3;
          n_stack[s - 1] = p;
          recount(pp);
          recount(p);
          recount(n3);
          if (s >= 3) {
            const ppp = n_stack[s - 3];
            if (ppp.left === pp) {
              ppp.left = n3;
            } else {
              ppp.right = n3;
            }
          }
          break;
        }
      }
    } else {
      if (p.right === n3) {
        const y = pp.left;
        if (y && y.color === Color.Red) {
          p.color = Color.Black;
          pp.left = repaint(y, Color.Black);
          pp.color = Color.Red;
          s -= 1;
        } else {
          pp.color = Color.Red;
          pp.right = p.left;
          p.color = Color.Black;
          p.left = pp;
          n_stack[s - 2] = p;
          n_stack[s - 1] = n3;
          recount(pp);
          recount(p);
          if (s >= 3) {
            const ppp = n_stack[s - 3];
            if (ppp.right === pp) {
              ppp.right = p;
            } else {
              ppp.left = p;
            }
          }
          break;
        }
      } else {
        const y = pp.left;
        if (y && y.color === Color.Red) {
          p.color = Color.Black;
          pp.left = repaint(y, Color.Black);
          pp.color = Color.Red;
          s -= 1;
        } else {
          p.left = n3.right;
          pp.color = Color.Red;
          pp.right = n3.left;
          n3.color = Color.Black;
          n3.right = p;
          n3.left = pp;
          n_stack[s - 2] = n3;
          n_stack[s - 1] = p;
          recount(pp);
          recount(p);
          recount(n3);
          if (s >= 3) {
            const ppp = n_stack[s - 3];
            if (ppp.right === pp) {
              ppp.right = n3;
            } else {
              ppp.left = n3;
            }
          }
          break;
        }
      }
    }
  }
  n_stack[0].color = Color.Black;
  return makeImpl3(self._ord, n_stack[0]);
});
var keysForward = /* @__PURE__ */ __name((self) => keys3(self, Direction.Forward), "keysForward");
var keys3 = /* @__PURE__ */ __name((self, direction) => {
  const begin = self[Symbol.iterator]();
  let count = 0;
  return {
    [Symbol.iterator]: () => keys3(self, direction),
    next: /* @__PURE__ */ __name(() => {
      count++;
      const entry = begin.key;
      if (direction === Direction.Forward) {
        begin.moveNext();
      } else {
        begin.movePrev();
      }
      switch (entry._tag) {
        case "None": {
          return {
            done: true,
            value: count
          };
        }
        case "Some": {
          return {
            done: false,
            value: entry.value
          };
        }
      }
    }, "next")
  };
}, "keys");
var removeFirst = /* @__PURE__ */ dual(2, (self, key) => {
  if (!has5(self, key)) {
    return self;
  }
  const ord = self._ord;
  const cmp = ord;
  let node = self._root;
  const stack = [];
  while (node !== void 0) {
    const d = cmp(key, node.key);
    stack.push(node);
    if (equals(key, node.key)) {
      node = void 0;
    } else if (d <= 0) {
      node = node.left;
    } else {
      node = node.right;
    }
  }
  if (stack.length === 0) {
    return self;
  }
  const cstack = new Array(stack.length);
  let n = stack[stack.length - 1];
  cstack[cstack.length - 1] = {
    color: n.color,
    key: n.key,
    value: n.value,
    left: n.left,
    right: n.right,
    count: n.count
  };
  for (let i = stack.length - 2; i >= 0; --i) {
    n = stack[i];
    if (n.left === stack[i + 1]) {
      cstack[i] = {
        color: n.color,
        key: n.key,
        value: n.value,
        left: cstack[i + 1],
        right: n.right,
        count: n.count
      };
    } else {
      cstack[i] = {
        color: n.color,
        key: n.key,
        value: n.value,
        left: n.left,
        right: cstack[i + 1],
        count: n.count
      };
    }
  }
  n = cstack[cstack.length - 1];
  if (n.left !== void 0 && n.right !== void 0) {
    const split = cstack.length;
    n = n.left;
    while (n.right != null) {
      cstack.push(n);
      n = n.right;
    }
    const v = cstack[split - 1];
    cstack.push({
      color: n.color,
      key: v.key,
      value: v.value,
      left: n.left,
      right: n.right,
      count: n.count
    });
    cstack[split - 1].key = n.key;
    cstack[split - 1].value = n.value;
    for (let i = cstack.length - 2; i >= split; --i) {
      n = cstack[i];
      cstack[i] = {
        color: n.color,
        key: n.key,
        value: n.value,
        left: n.left,
        right: cstack[i + 1],
        count: n.count
      };
    }
    cstack[split - 1].left = cstack[split];
  }
  n = cstack[cstack.length - 1];
  if (n.color === Color.Red) {
    const p = cstack[cstack.length - 2];
    if (p.left === n) {
      p.left = void 0;
    } else if (p.right === n) {
      p.right = void 0;
    }
    cstack.pop();
    for (let i = 0; i < cstack.length; ++i) {
      cstack[i].count--;
    }
    return makeImpl3(ord, cstack[0]);
  } else {
    if (n.left !== void 0 || n.right !== void 0) {
      if (n.left !== void 0) {
        swap2(n, n.left);
      } else if (n.right !== void 0) {
        swap2(n, n.right);
      }
      n.color = Color.Black;
      for (let i = 0; i < cstack.length - 1; ++i) {
        cstack[i].count--;
      }
      return makeImpl3(ord, cstack[0]);
    } else if (cstack.length === 1) {
      return makeImpl3(ord, void 0);
    } else {
      for (let i = 0; i < cstack.length; ++i) {
        cstack[i].count--;
      }
      const parent = cstack[cstack.length - 2];
      fixDoubleBlack(cstack);
      if (parent.left === n) {
        parent.left = void 0;
      } else {
        parent.right = void 0;
      }
    }
  }
  return makeImpl3(ord, cstack[0]);
});
var fixDoubleBlack = /* @__PURE__ */ __name((stack) => {
  let n, p, s, z;
  for (let i = stack.length - 1; i >= 0; --i) {
    n = stack[i];
    if (i === 0) {
      n.color = Color.Black;
      return;
    }
    p = stack[i - 1];
    if (p.left === n) {
      s = p.right;
      if (s !== void 0 && s.right !== void 0 && s.right.color === Color.Red) {
        s = p.right = clone(s);
        z = s.right = clone(s.right);
        p.right = s.left;
        s.left = p;
        s.right = z;
        s.color = p.color;
        n.color = Color.Black;
        p.color = Color.Black;
        z.color = Color.Black;
        recount(p);
        recount(s);
        if (i > 1) {
          const pp = stack[i - 2];
          if (pp.left === p) {
            pp.left = s;
          } else {
            pp.right = s;
          }
        }
        stack[i - 1] = s;
        return;
      } else if (s !== void 0 && s.left !== void 0 && s.left.color === Color.Red) {
        s = p.right = clone(s);
        z = s.left = clone(s.left);
        p.right = z.left;
        s.left = z.right;
        z.left = p;
        z.right = s;
        z.color = p.color;
        p.color = Color.Black;
        s.color = Color.Black;
        n.color = Color.Black;
        recount(p);
        recount(s);
        recount(z);
        if (i > 1) {
          const pp = stack[i - 2];
          if (pp.left === p) {
            pp.left = z;
          } else {
            pp.right = z;
          }
        }
        stack[i - 1] = z;
        return;
      }
      if (s !== void 0 && s.color === Color.Black) {
        if (p.color === Color.Red) {
          p.color = Color.Black;
          p.right = repaint(s, Color.Red);
          return;
        } else {
          p.right = repaint(s, Color.Red);
          continue;
        }
      } else if (s !== void 0) {
        s = clone(s);
        p.right = s.left;
        s.left = p;
        s.color = p.color;
        p.color = Color.Red;
        recount(p);
        recount(s);
        if (i > 1) {
          const pp = stack[i - 2];
          if (pp.left === p) {
            pp.left = s;
          } else {
            pp.right = s;
          }
        }
        stack[i - 1] = s;
        stack[i] = p;
        if (i + 1 < stack.length) {
          stack[i + 1] = n;
        } else {
          stack.push(n);
        }
        i = i + 2;
      }
    } else {
      s = p.left;
      if (s !== void 0 && s.left !== void 0 && s.left.color === Color.Red) {
        s = p.left = clone(s);
        z = s.left = clone(s.left);
        p.left = s.right;
        s.right = p;
        s.left = z;
        s.color = p.color;
        n.color = Color.Black;
        p.color = Color.Black;
        z.color = Color.Black;
        recount(p);
        recount(s);
        if (i > 1) {
          const pp = stack[i - 2];
          if (pp.right === p) {
            pp.right = s;
          } else {
            pp.left = s;
          }
        }
        stack[i - 1] = s;
        return;
      } else if (s !== void 0 && s.right !== void 0 && s.right.color === Color.Red) {
        s = p.left = clone(s);
        z = s.right = clone(s.right);
        p.left = z.right;
        s.right = z.left;
        z.right = p;
        z.left = s;
        z.color = p.color;
        p.color = Color.Black;
        s.color = Color.Black;
        n.color = Color.Black;
        recount(p);
        recount(s);
        recount(z);
        if (i > 1) {
          const pp = stack[i - 2];
          if (pp.right === p) {
            pp.right = z;
          } else {
            pp.left = z;
          }
        }
        stack[i - 1] = z;
        return;
      }
      if (s !== void 0 && s.color === Color.Black) {
        if (p.color === Color.Red) {
          p.color = Color.Black;
          p.left = repaint(s, Color.Red);
          return;
        } else {
          p.left = repaint(s, Color.Red);
          continue;
        }
      } else if (s !== void 0) {
        s = clone(s);
        p.left = s.right;
        s.right = p;
        s.color = p.color;
        p.color = Color.Red;
        recount(p);
        recount(s);
        if (i > 1) {
          const pp = stack[i - 2];
          if (pp.right === p) {
            pp.right = s;
          } else {
            pp.left = s;
          }
        }
        stack[i - 1] = s;
        stack[i] = p;
        if (i + 1 < stack.length) {
          stack[i + 1] = n;
        } else {
          stack.push(n);
        }
        i = i + 2;
      }
    }
  }
}, "fixDoubleBlack");

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/RedBlackTree.js
var has6 = has5;
var insert2 = insert;
var keys4 = keysForward;
var removeFirst2 = removeFirst;

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/SortedSet.js
var TypeId14 = /* @__PURE__ */ Symbol.for("effect/SortedSet");
var SortedSetProto = {
  [TypeId14]: {
    _A: /* @__PURE__ */ __name((_) => _, "_A")
  },
  [symbol]() {
    return pipe(hash(this.keyTree), combine(hash(TypeId14)), cached(this));
  },
  [symbol2](that) {
    return isSortedSet(that) && equals(this.keyTree, that.keyTree);
  },
  [Symbol.iterator]() {
    return keys4(this.keyTree);
  },
  toString() {
    return format(this.toJSON());
  },
  toJSON() {
    return {
      _id: "SortedSet",
      values: Array.from(this).map(toJSON)
    };
  },
  [NodeInspectSymbol]() {
    return this.toJSON();
  },
  pipe() {
    return pipeArguments(this, arguments);
  }
};
var fromTree = /* @__PURE__ */ __name((keyTree) => {
  const a = Object.create(SortedSetProto);
  a.keyTree = keyTree;
  return a;
}, "fromTree");
var isSortedSet = /* @__PURE__ */ __name((u) => hasProperty(u, TypeId14), "isSortedSet");
var add5 = /* @__PURE__ */ dual(2, (self, value) => has6(self.keyTree, value) ? self : fromTree(insert2(self.keyTree, value, true)));
var remove7 = /* @__PURE__ */ dual(2, (self, value) => fromTree(removeFirst2(self.keyTree, value)));

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/internal/supervisor.js
var SupervisorSymbolKey = "effect/Supervisor";
var SupervisorTypeId = /* @__PURE__ */ Symbol.for(SupervisorSymbolKey);
var supervisorVariance = {
  /* c8 ignore next */
  _T: /* @__PURE__ */ __name((_) => _, "_T")
};
var ProxySupervisor = class _ProxySupervisor {
  static {
    __name(this, "ProxySupervisor");
  }
  underlying;
  value0;
  [SupervisorTypeId] = supervisorVariance;
  constructor(underlying, value0) {
    this.underlying = underlying;
    this.value0 = value0;
  }
  get value() {
    return this.value0;
  }
  onStart(context4, effect, parent, fiber) {
    this.underlying.onStart(context4, effect, parent, fiber);
  }
  onEnd(value, fiber) {
    this.underlying.onEnd(value, fiber);
  }
  onEffect(fiber, effect) {
    this.underlying.onEffect(fiber, effect);
  }
  onSuspend(fiber) {
    this.underlying.onSuspend(fiber);
  }
  onResume(fiber) {
    this.underlying.onResume(fiber);
  }
  map(f) {
    return new _ProxySupervisor(this, pipe(this.value, map8(f)));
  }
  zip(right3) {
    return new Zip(this, right3);
  }
};
var Zip = class _Zip {
  static {
    __name(this, "Zip");
  }
  left;
  right;
  _tag = "Zip";
  [SupervisorTypeId] = supervisorVariance;
  constructor(left3, right3) {
    this.left = left3;
    this.right = right3;
  }
  get value() {
    return zip2(this.left.value, this.right.value);
  }
  onStart(context4, effect, parent, fiber) {
    this.left.onStart(context4, effect, parent, fiber);
    this.right.onStart(context4, effect, parent, fiber);
  }
  onEnd(value, fiber) {
    this.left.onEnd(value, fiber);
    this.right.onEnd(value, fiber);
  }
  onEffect(fiber, effect) {
    this.left.onEffect(fiber, effect);
    this.right.onEffect(fiber, effect);
  }
  onSuspend(fiber) {
    this.left.onSuspend(fiber);
    this.right.onSuspend(fiber);
  }
  onResume(fiber) {
    this.left.onResume(fiber);
    this.right.onResume(fiber);
  }
  map(f) {
    return new ProxySupervisor(this, pipe(this.value, map8(f)));
  }
  zip(right3) {
    return new _Zip(this, right3);
  }
};
var isZip = /* @__PURE__ */ __name((self) => hasProperty(self, SupervisorTypeId) && isTagged(self, "Zip"), "isZip");
var Track = class {
  static {
    __name(this, "Track");
  }
  [SupervisorTypeId] = supervisorVariance;
  fibers = /* @__PURE__ */ new Set();
  get value() {
    return sync(() => Array.from(this.fibers));
  }
  onStart(_context, _effect, _parent, fiber) {
    this.fibers.add(fiber);
  }
  onEnd(_value, fiber) {
    this.fibers.delete(fiber);
  }
  onEffect(_fiber, _effect) {
  }
  onSuspend(_fiber) {
  }
  onResume(_fiber) {
  }
  map(f) {
    return new ProxySupervisor(this, pipe(this.value, map8(f)));
  }
  zip(right3) {
    return new Zip(this, right3);
  }
  onRun(execution, _fiber) {
    return execution();
  }
};
var Const = class {
  static {
    __name(this, "Const");
  }
  effect;
  [SupervisorTypeId] = supervisorVariance;
  constructor(effect) {
    this.effect = effect;
  }
  get value() {
    return this.effect;
  }
  onStart(_context, _effect, _parent, _fiber) {
  }
  onEnd(_value, _fiber) {
  }
  onEffect(_fiber, _effect) {
  }
  onSuspend(_fiber) {
  }
  onResume(_fiber) {
  }
  map(f) {
    return new ProxySupervisor(this, pipe(this.value, map8(f)));
  }
  zip(right3) {
    return new Zip(this, right3);
  }
  onRun(execution, _fiber) {
    return execution();
  }
};
var FibersIn = class {
  static {
    __name(this, "FibersIn");
  }
  ref;
  [SupervisorTypeId] = supervisorVariance;
  constructor(ref) {
    this.ref = ref;
  }
  get value() {
    return sync(() => get6(this.ref));
  }
  onStart(_context, _effect, _parent, fiber) {
    pipe(this.ref, set2(pipe(get6(this.ref), add5(fiber))));
  }
  onEnd(_value, fiber) {
    pipe(this.ref, set2(pipe(get6(this.ref), remove7(fiber))));
  }
  onEffect(_fiber, _effect) {
  }
  onSuspend(_fiber) {
  }
  onResume(_fiber) {
  }
  map(f) {
    return new ProxySupervisor(this, pipe(this.value, map8(f)));
  }
  zip(right3) {
    return new Zip(this, right3);
  }
  onRun(execution, _fiber) {
    return execution();
  }
};
var unsafeTrack = /* @__PURE__ */ __name(() => {
  return new Track();
}, "unsafeTrack");
var track = /* @__PURE__ */ sync(unsafeTrack);
var fromEffect = /* @__PURE__ */ __name((effect) => {
  return new Const(effect);
}, "fromEffect");
var none8 = /* @__PURE__ */ globalValue("effect/Supervisor/none", () => fromEffect(void_));

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/Differ.js
var make33 = make14;

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/internal/supervisor/patch.js
var OP_EMPTY3 = "Empty";
var OP_ADD_SUPERVISOR = "AddSupervisor";
var OP_REMOVE_SUPERVISOR = "RemoveSupervisor";
var OP_AND_THEN2 = "AndThen";
var empty25 = {
  _tag: OP_EMPTY3
};
var combine8 = /* @__PURE__ */ __name((self, that) => {
  return {
    _tag: OP_AND_THEN2,
    first: self,
    second: that
  };
}, "combine");
var patch8 = /* @__PURE__ */ __name((self, supervisor) => {
  return patchLoop(supervisor, of2(self));
}, "patch");
var patchLoop = /* @__PURE__ */ __name((_supervisor, _patches) => {
  let supervisor = _supervisor;
  let patches = _patches;
  while (isNonEmpty(patches)) {
    const head5 = headNonEmpty2(patches);
    switch (head5._tag) {
      case OP_EMPTY3: {
        patches = tailNonEmpty2(patches);
        break;
      }
      case OP_ADD_SUPERVISOR: {
        supervisor = supervisor.zip(head5.supervisor);
        patches = tailNonEmpty2(patches);
        break;
      }
      case OP_REMOVE_SUPERVISOR: {
        supervisor = removeSupervisor(supervisor, head5.supervisor);
        patches = tailNonEmpty2(patches);
        break;
      }
      case OP_AND_THEN2: {
        patches = prepend2(head5.first)(prepend2(head5.second)(tailNonEmpty2(patches)));
        break;
      }
    }
  }
  return supervisor;
}, "patchLoop");
var removeSupervisor = /* @__PURE__ */ __name((self, that) => {
  if (equals(self, that)) {
    return none8;
  } else {
    if (isZip(self)) {
      return removeSupervisor(self.left, that).zip(removeSupervisor(self.right, that));
    } else {
      return self;
    }
  }
}, "removeSupervisor");
var toSet2 = /* @__PURE__ */ __name((self) => {
  if (equals(self, none8)) {
    return empty7();
  } else {
    if (isZip(self)) {
      return pipe(toSet2(self.left), union3(toSet2(self.right)));
    } else {
      return make10(self);
    }
  }
}, "toSet");
var diff7 = /* @__PURE__ */ __name((oldValue, newValue) => {
  if (equals(oldValue, newValue)) {
    return empty25;
  }
  const oldSupervisors = toSet2(oldValue);
  const newSupervisors = toSet2(newValue);
  const added = pipe(newSupervisors, difference3(oldSupervisors), reduce4(empty25, (patch9, supervisor) => combine8(patch9, {
    _tag: OP_ADD_SUPERVISOR,
    supervisor
  })));
  const removed = pipe(oldSupervisors, difference3(newSupervisors), reduce4(empty25, (patch9, supervisor) => combine8(patch9, {
    _tag: OP_REMOVE_SUPERVISOR,
    supervisor
  })));
  return combine8(added, removed);
}, "diff");
var differ2 = /* @__PURE__ */ make33({
  empty: empty25,
  patch: patch8,
  combine: combine8,
  diff: diff7
});

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/internal/fiberRuntime.js
var fiberStarted = /* @__PURE__ */ counter5("effect_fiber_started", {
  incremental: true
});
var fiberActive = /* @__PURE__ */ counter5("effect_fiber_active");
var fiberSuccesses = /* @__PURE__ */ counter5("effect_fiber_successes", {
  incremental: true
});
var fiberFailures = /* @__PURE__ */ counter5("effect_fiber_failures", {
  incremental: true
});
var fiberLifetimes = /* @__PURE__ */ tagged(/* @__PURE__ */ histogram5("effect_fiber_lifetimes", /* @__PURE__ */ exponential({
  start: 0.5,
  factor: 2,
  count: 35
})), "time_unit", "milliseconds");
var EvaluationSignalContinue = "Continue";
var EvaluationSignalDone = "Done";
var EvaluationSignalYieldNow = "Yield";
var runtimeFiberVariance = {
  /* c8 ignore next */
  _E: /* @__PURE__ */ __name((_) => _, "_E"),
  /* c8 ignore next */
  _A: /* @__PURE__ */ __name((_) => _, "_A")
};
var absurd = /* @__PURE__ */ __name((_) => {
  throw new Error(`BUG: FiberRuntime - ${toStringUnknown(_)} - please report an issue at https://github.com/Effect-TS/effect/issues`);
}, "absurd");
var YieldedOp = /* @__PURE__ */ Symbol.for("effect/internal/fiberRuntime/YieldedOp");
var yieldedOpChannel = /* @__PURE__ */ globalValue("effect/internal/fiberRuntime/yieldedOpChannel", () => ({
  currentOp: null
}));
var contOpSuccess = {
  [OP_ON_SUCCESS]: (_, cont, value) => {
    return internalCall(() => cont.effect_instruction_i1(value));
  },
  ["OnStep"]: /* @__PURE__ */ __name((_, _cont, value) => {
    return exitSucceed(exitSucceed(value));
  }, "OnStep"),
  [OP_ON_SUCCESS_AND_FAILURE]: (_, cont, value) => {
    return internalCall(() => cont.effect_instruction_i2(value));
  },
  [OP_REVERT_FLAGS]: (self, cont, value) => {
    self.patchRuntimeFlags(self.currentRuntimeFlags, cont.patch);
    if (interruptible(self.currentRuntimeFlags) && self.isInterrupted()) {
      return exitFailCause(self.getInterruptedCause());
    } else {
      return exitSucceed(value);
    }
  },
  [OP_WHILE]: (self, cont, value) => {
    internalCall(() => cont.effect_instruction_i2(value));
    if (internalCall(() => cont.effect_instruction_i0())) {
      self.pushStack(cont);
      return internalCall(() => cont.effect_instruction_i1());
    } else {
      return void_;
    }
  },
  [OP_ITERATOR]: (self, cont, value) => {
    while (true) {
      const state = internalCall(() => cont.effect_instruction_i0.next(value));
      if (state.done) {
        return exitSucceed(state.value);
      }
      const primitive = yieldWrapGet(state.value);
      if (!exitIsExit(primitive)) {
        self.pushStack(cont);
        return primitive;
      } else if (primitive._tag === "Failure") {
        return primitive;
      }
      value = primitive.value;
    }
  }
};
var drainQueueWhileRunningTable = {
  [OP_INTERRUPT_SIGNAL]: (self, runtimeFlags2, cur, message) => {
    self.processNewInterruptSignal(message.cause);
    return interruptible(runtimeFlags2) ? exitFailCause(message.cause) : cur;
  },
  [OP_RESUME]: (_self, _runtimeFlags, _cur, _message) => {
    throw new Error("It is illegal to have multiple concurrent run loops in a single fiber");
  },
  [OP_STATEFUL]: (self, runtimeFlags2, cur, message) => {
    message.onFiber(self, running2(runtimeFlags2));
    return cur;
  },
  [OP_YIELD_NOW]: (_self, _runtimeFlags, cur, _message) => {
    return flatMap7(yieldNow(), () => cur);
  }
};
var runBlockedRequests = /* @__PURE__ */ __name((self) => forEachSequentialDiscard(flatten2(self), (requestsByRequestResolver) => forEachConcurrentDiscard(sequentialCollectionToChunk(requestsByRequestResolver), ([dataSource, sequential5]) => {
  const map14 = /* @__PURE__ */ new Map();
  const arr = [];
  for (const block of sequential5) {
    arr.push(toReadonlyArray(block));
    for (const entry of block) {
      map14.set(entry.request, entry);
    }
  }
  const flat = arr.flat();
  return fiberRefLocally(invokeWithInterrupt(dataSource.runAll(arr), flat, () => flat.forEach((entry) => {
    entry.listeners.interrupted = true;
  })), currentRequestMap, map14);
}, false, false)), "runBlockedRequests");
var _version = /* @__PURE__ */ getCurrentVersion();
var FiberRuntime = class extends Class2 {
  static {
    __name(this, "FiberRuntime");
  }
  [FiberTypeId] = fiberVariance2;
  [RuntimeFiberTypeId] = runtimeFiberVariance;
  _fiberRefs;
  _fiberId;
  _queue = /* @__PURE__ */ new Array();
  _children = null;
  _observers = /* @__PURE__ */ new Array();
  _running = false;
  _stack = [];
  _asyncInterruptor = null;
  _asyncBlockingOn = null;
  _exitValue = null;
  _steps = [];
  _isYielding = false;
  currentRuntimeFlags;
  currentOpCount = 0;
  currentSupervisor;
  currentScheduler;
  currentTracer;
  currentSpan;
  currentContext;
  currentDefaultServices;
  constructor(fiberId3, fiberRefs0, runtimeFlags0) {
    super();
    this.currentRuntimeFlags = runtimeFlags0;
    this._fiberId = fiberId3;
    this._fiberRefs = fiberRefs0;
    if (runtimeMetrics(runtimeFlags0)) {
      const tags = this.getFiberRef(currentMetricLabels);
      fiberStarted.unsafeUpdate(1, tags);
      fiberActive.unsafeUpdate(1, tags);
    }
    this.refreshRefCache();
  }
  commit() {
    return join2(this);
  }
  /**
   * The identity of the fiber.
   */
  id() {
    return this._fiberId;
  }
  /**
   * Begins execution of the effect associated with this fiber on in the
   * background. This can be called to "kick off" execution of a fiber after
   * it has been created.
   */
  resume(effect) {
    this.tell(resume(effect));
  }
  /**
   * The status of the fiber.
   */
  get status() {
    return this.ask((_, status) => status);
  }
  /**
   * Gets the fiber runtime flags.
   */
  get runtimeFlags() {
    return this.ask((state, status) => {
      if (isDone2(status)) {
        return state.currentRuntimeFlags;
      }
      return status.runtimeFlags;
    });
  }
  /**
   * Returns the current `FiberScope` for the fiber.
   */
  scope() {
    return unsafeMake6(this);
  }
  /**
   * Retrieves the immediate children of the fiber.
   */
  get children() {
    return this.ask((fiber) => Array.from(fiber.getChildren()));
  }
  /**
   * Gets the fiber's set of children.
   */
  getChildren() {
    if (this._children === null) {
      this._children = /* @__PURE__ */ new Set();
    }
    return this._children;
  }
  /**
   * Retrieves the interrupted cause of the fiber, which will be `Cause.empty`
   * if the fiber has not been interrupted.
   *
   * **NOTE**: This method is safe to invoke on any fiber, but if not invoked
   * on this fiber, then values derived from the fiber's state (including the
   * log annotations and log level) may not be up-to-date.
   */
  getInterruptedCause() {
    return this.getFiberRef(currentInterruptedCause);
  }
  /**
   * Retrieves the whole set of fiber refs.
   */
  fiberRefs() {
    return this.ask((fiber) => fiber.getFiberRefs());
  }
  /**
   * Returns an effect that will contain information computed from the fiber
   * state and status while running on the fiber.
   *
   * This allows the outside world to interact safely with mutable fiber state
   * without locks or immutable data.
   */
  ask(f) {
    return suspend(() => {
      const deferred = deferredUnsafeMake(this._fiberId);
      this.tell(stateful((fiber, status) => {
        deferredUnsafeDone(deferred, sync(() => f(fiber, status)));
      }));
      return deferredAwait(deferred);
    });
  }
  /**
   * Adds a message to be processed by the fiber on the fiber.
   */
  tell(message) {
    this._queue.push(message);
    if (!this._running) {
      this._running = true;
      this.drainQueueLaterOnExecutor();
    }
  }
  get await() {
    return async_((resume2) => {
      const cb = /* @__PURE__ */ __name((exit4) => resume2(succeed(exit4)), "cb");
      if (this._exitValue !== null) {
        cb(this._exitValue);
        return;
      }
      this.tell(stateful((fiber, _) => {
        if (fiber._exitValue !== null) {
          cb(this._exitValue);
        } else {
          fiber.addObserver(cb);
        }
      }));
      return sync(() => this.tell(stateful((fiber, _) => {
        fiber.removeObserver(cb);
      })));
    }, this.id());
  }
  get inheritAll() {
    return withFiberRuntime((parentFiber, parentStatus) => {
      const parentFiberId = parentFiber.id();
      const parentFiberRefs = parentFiber.getFiberRefs();
      const parentRuntimeFlags = parentStatus.runtimeFlags;
      const childFiberRefs = this.getFiberRefs();
      const updatedFiberRefs = joinAs(parentFiberRefs, parentFiberId, childFiberRefs);
      parentFiber.setFiberRefs(updatedFiberRefs);
      const updatedRuntimeFlags = parentFiber.getFiberRef(currentRuntimeFlags);
      const patch9 = pipe(
        diff4(parentRuntimeFlags, updatedRuntimeFlags),
        // Do not inherit WindDown or Interruption!
        exclude2(Interruption),
        exclude2(WindDown)
      );
      return updateRuntimeFlags(patch9);
    });
  }
  /**
   * Tentatively observes the fiber, but returns immediately if it is not
   * already done.
   */
  get poll() {
    return sync(() => fromNullable(this._exitValue));
  }
  /**
   * Unsafely observes the fiber, but returns immediately if it is not
   * already done.
   */
  unsafePoll() {
    return this._exitValue;
  }
  /**
   * In the background, interrupts the fiber as if interrupted from the specified fiber.
   */
  interruptAsFork(fiberId3) {
    return sync(() => this.tell(interruptSignal(interrupt(fiberId3))));
  }
  /**
   * In the background, interrupts the fiber as if interrupted from the specified fiber.
   */
  unsafeInterruptAsFork(fiberId3) {
    this.tell(interruptSignal(interrupt(fiberId3)));
  }
  /**
   * Adds an observer to the list of observers.
   *
   * **NOTE**: This method must be invoked by the fiber itself.
   */
  addObserver(observer) {
    if (this._exitValue !== null) {
      observer(this._exitValue);
    } else {
      this._observers.push(observer);
    }
  }
  /**
   * Removes the specified observer from the list of observers that will be
   * notified when the fiber exits.
   *
   * **NOTE**: This method must be invoked by the fiber itself.
   */
  removeObserver(observer) {
    this._observers = this._observers.filter((o) => o !== observer);
  }
  /**
   * Retrieves all fiber refs of the fiber.
   *
   * **NOTE**: This method is safe to invoke on any fiber, but if not invoked
   * on this fiber, then values derived from the fiber's state (including the
   * log annotations and log level) may not be up-to-date.
   */
  getFiberRefs() {
    this.setFiberRef(currentRuntimeFlags, this.currentRuntimeFlags);
    return this._fiberRefs;
  }
  /**
   * Deletes the specified fiber ref.
   *
   * **NOTE**: This method must be invoked by the fiber itself.
   */
  unsafeDeleteFiberRef(fiberRef) {
    this._fiberRefs = delete_(this._fiberRefs, fiberRef);
  }
  /**
   * Retrieves the state of the fiber ref, or else its initial value.
   *
   * **NOTE**: This method is safe to invoke on any fiber, but if not invoked
   * on this fiber, then values derived from the fiber's state (including the
   * log annotations and log level) may not be up-to-date.
   */
  getFiberRef(fiberRef) {
    if (this._fiberRefs.locals.has(fiberRef)) {
      return this._fiberRefs.locals.get(fiberRef)[0][1];
    }
    return fiberRef.initial;
  }
  /**
   * Sets the fiber ref to the specified value.
   *
   * **NOTE**: This method must be invoked by the fiber itself.
   */
  setFiberRef(fiberRef, value) {
    this._fiberRefs = updateAs(this._fiberRefs, {
      fiberId: this._fiberId,
      fiberRef,
      value
    });
    this.refreshRefCache();
  }
  refreshRefCache() {
    this.currentDefaultServices = this.getFiberRef(currentServices);
    this.currentTracer = this.currentDefaultServices.unsafeMap.get(tracerTag.key);
    this.currentSupervisor = this.getFiberRef(currentSupervisor);
    this.currentScheduler = this.getFiberRef(currentScheduler);
    this.currentContext = this.getFiberRef(currentContext);
    this.currentSpan = this.currentContext.unsafeMap.get(spanTag.key);
  }
  /**
   * Wholesale replaces all fiber refs of this fiber.
   *
   * **NOTE**: This method must be invoked by the fiber itself.
   */
  setFiberRefs(fiberRefs3) {
    this._fiberRefs = fiberRefs3;
    this.refreshRefCache();
  }
  /**
   * Adds a reference to the specified fiber inside the children set.
   *
   * **NOTE**: This method must be invoked by the fiber itself.
   */
  addChild(child) {
    this.getChildren().add(child);
  }
  /**
   * Removes a reference to the specified fiber inside the children set.
   *
   * **NOTE**: This method must be invoked by the fiber itself.
   */
  removeChild(child) {
    this.getChildren().delete(child);
  }
  /**
   * Transfers all children of this fiber that are currently running to the
   * specified fiber scope.
   *
   * **NOTE**: This method must be invoked by the fiber itself after it has
   * evaluated the effects but prior to exiting.
   */
  transferChildren(scope3) {
    const children = this._children;
    this._children = null;
    if (children !== null && children.size > 0) {
      for (const child of children) {
        if (child._exitValue === null) {
          scope3.add(this.currentRuntimeFlags, child);
        }
      }
    }
  }
  /**
   * On the current thread, executes all messages in the fiber's inbox. This
   * method may return before all work is done, in the event the fiber executes
   * an asynchronous operation.
   *
   * **NOTE**: This method must be invoked by the fiber itself.
   */
  drainQueueOnCurrentThread() {
    let recurse = true;
    while (recurse) {
      let evaluationSignal = EvaluationSignalContinue;
      const prev = globalThis[currentFiberURI];
      globalThis[currentFiberURI] = this;
      try {
        while (evaluationSignal === EvaluationSignalContinue) {
          evaluationSignal = this._queue.length === 0 ? EvaluationSignalDone : this.evaluateMessageWhileSuspended(this._queue.splice(0, 1)[0]);
        }
      } finally {
        this._running = false;
        globalThis[currentFiberURI] = prev;
      }
      if (this._queue.length > 0 && !this._running) {
        this._running = true;
        if (evaluationSignal === EvaluationSignalYieldNow) {
          this.drainQueueLaterOnExecutor();
          recurse = false;
        } else {
          recurse = true;
        }
      } else {
        recurse = false;
      }
    }
  }
  /**
   * Schedules the execution of all messages in the fiber's inbox.
   *
   * This method will return immediately after the scheduling
   * operation is completed, but potentially before such messages have been
   * executed.
   *
   * **NOTE**: This method must be invoked by the fiber itself.
   */
  drainQueueLaterOnExecutor() {
    this.currentScheduler.scheduleTask(this.run, this.getFiberRef(currentSchedulingPriority), this);
  }
  /**
   * Drains the fiber's message queue while the fiber is actively running,
   * returning the next effect to execute, which may be the input effect if no
   * additional effect needs to be executed.
   *
   * **NOTE**: This method must be invoked by the fiber itself.
   */
  drainQueueWhileRunning(runtimeFlags2, cur0) {
    let cur = cur0;
    while (this._queue.length > 0) {
      const message = this._queue.splice(0, 1)[0];
      cur = drainQueueWhileRunningTable[message._tag](this, runtimeFlags2, cur, message);
    }
    return cur;
  }
  /**
   * Determines if the fiber is interrupted.
   *
   * **NOTE**: This method is safe to invoke on any fiber, but if not invoked
   * on this fiber, then values derived from the fiber's state (including the
   * log annotations and log level) may not be up-to-date.
   */
  isInterrupted() {
    return !isEmpty5(this.getFiberRef(currentInterruptedCause));
  }
  /**
   * Adds an interruptor to the set of interruptors that are interrupting this
   * fiber.
   *
   * **NOTE**: This method must be invoked by the fiber itself.
   */
  addInterruptedCause(cause3) {
    const oldSC = this.getFiberRef(currentInterruptedCause);
    this.setFiberRef(currentInterruptedCause, sequential(oldSC, cause3));
  }
  /**
   * Processes a new incoming interrupt signal.
   *
   * **NOTE**: This method must be invoked by the fiber itself.
   */
  processNewInterruptSignal(cause3) {
    this.addInterruptedCause(cause3);
    this.sendInterruptSignalToAllChildren();
  }
  /**
   * Interrupts all children of the current fiber, returning an effect that will
   * await the exit of the children. This method will return null if the fiber
   * has no children.
   *
   * **NOTE**: This method must be invoked by the fiber itself.
   */
  sendInterruptSignalToAllChildren() {
    if (this._children === null || this._children.size === 0) {
      return false;
    }
    let told = false;
    for (const child of this._children) {
      child.tell(interruptSignal(interrupt(this.id())));
      told = true;
    }
    return told;
  }
  /**
   * Interrupts all children of the current fiber, returning an effect that will
   * await the exit of the children. This method will return null if the fiber
   * has no children.
   *
   * **NOTE**: This method must be invoked by the fiber itself.
   */
  interruptAllChildren() {
    if (this.sendInterruptSignalToAllChildren()) {
      const it = this._children.values();
      this._children = null;
      let isDone5 = false;
      const body = /* @__PURE__ */ __name(() => {
        const next = it.next();
        if (!next.done) {
          return asVoid(next.value.await);
        } else {
          return sync(() => {
            isDone5 = true;
          });
        }
      }, "body");
      return whileLoop({
        while: /* @__PURE__ */ __name(() => !isDone5, "while"),
        body,
        step: /* @__PURE__ */ __name(() => {
        }, "step")
      });
    }
    return null;
  }
  reportExitValue(exit4) {
    if (runtimeMetrics(this.currentRuntimeFlags)) {
      const tags = this.getFiberRef(currentMetricLabels);
      const startTimeMillis = this.id().startTimeMillis;
      const endTimeMillis = Date.now();
      fiberLifetimes.unsafeUpdate(endTimeMillis - startTimeMillis, tags);
      fiberActive.unsafeUpdate(-1, tags);
      switch (exit4._tag) {
        case OP_SUCCESS: {
          fiberSuccesses.unsafeUpdate(1, tags);
          break;
        }
        case OP_FAILURE: {
          fiberFailures.unsafeUpdate(1, tags);
          break;
        }
      }
    }
    if (exit4._tag === "Failure") {
      const level = this.getFiberRef(currentUnhandledErrorLogLevel);
      if (!isInterruptedOnly(exit4.cause) && level._tag === "Some") {
        this.log("Fiber terminated with an unhandled error", exit4.cause, level);
      }
    }
  }
  setExitValue(exit4) {
    this._exitValue = exit4;
    this.reportExitValue(exit4);
    for (let i = this._observers.length - 1; i >= 0; i--) {
      this._observers[i](exit4);
    }
    this._observers = [];
  }
  getLoggers() {
    return this.getFiberRef(currentLoggers);
  }
  log(message, cause3, overrideLogLevel) {
    const logLevel = isSome2(overrideLogLevel) ? overrideLogLevel.value : this.getFiberRef(currentLogLevel);
    const minimumLogLevel = this.getFiberRef(currentMinimumLogLevel);
    if (greaterThan3(minimumLogLevel, logLevel)) {
      return;
    }
    const spans = this.getFiberRef(currentLogSpan);
    const annotations = this.getFiberRef(currentLogAnnotations);
    const loggers = this.getLoggers();
    const contextMap = this.getFiberRefs();
    if (size3(loggers) > 0) {
      const clockService = get3(this.getFiberRef(currentServices), clockTag);
      const date = new Date(clockService.unsafeCurrentTimeMillis());
      withRedactableContext(contextMap, () => {
        for (const logger of loggers) {
          logger.log({
            fiberId: this.id(),
            logLevel,
            message,
            cause: cause3,
            context: contextMap,
            spans,
            annotations,
            date
          });
        }
      });
    }
  }
  /**
   * Evaluates a single message on the current thread, while the fiber is
   * suspended. This method should only be called while evaluation of the
   * fiber's effect is suspended due to an asynchronous operation.
   *
   * **NOTE**: This method must be invoked by the fiber itself.
   */
  evaluateMessageWhileSuspended(message) {
    switch (message._tag) {
      case OP_YIELD_NOW: {
        return EvaluationSignalYieldNow;
      }
      case OP_INTERRUPT_SIGNAL: {
        this.processNewInterruptSignal(message.cause);
        if (this._asyncInterruptor !== null) {
          this._asyncInterruptor(exitFailCause(message.cause));
          this._asyncInterruptor = null;
        }
        return EvaluationSignalContinue;
      }
      case OP_RESUME: {
        this._asyncInterruptor = null;
        this._asyncBlockingOn = null;
        this.evaluateEffect(message.effect);
        return EvaluationSignalContinue;
      }
      case OP_STATEFUL: {
        message.onFiber(this, this._exitValue !== null ? done4 : suspended2(this.currentRuntimeFlags, this._asyncBlockingOn));
        return EvaluationSignalContinue;
      }
      default: {
        return absurd(message);
      }
    }
  }
  /**
   * Evaluates an effect until completion, potentially asynchronously.
   *
   * **NOTE**: This method must be invoked by the fiber itself.
   */
  evaluateEffect(effect0) {
    this.currentSupervisor.onResume(this);
    try {
      let effect = interruptible(this.currentRuntimeFlags) && this.isInterrupted() ? exitFailCause(this.getInterruptedCause()) : effect0;
      while (effect !== null) {
        const eff = effect;
        const exit4 = this.runLoop(eff);
        if (exit4 === YieldedOp) {
          const op = yieldedOpChannel.currentOp;
          yieldedOpChannel.currentOp = null;
          if (op._op === OP_YIELD) {
            if (cooperativeYielding(this.currentRuntimeFlags)) {
              this.tell(yieldNow3());
              this.tell(resume(exitVoid));
              effect = null;
            } else {
              effect = exitVoid;
            }
          } else if (op._op === OP_ASYNC) {
            effect = null;
          }
        } else {
          this.currentRuntimeFlags = pipe(this.currentRuntimeFlags, enable2(WindDown));
          const interruption2 = this.interruptAllChildren();
          if (interruption2 !== null) {
            effect = flatMap7(interruption2, () => exit4);
          } else {
            if (this._queue.length === 0) {
              this.setExitValue(exit4);
            } else {
              this.tell(resume(exit4));
            }
            effect = null;
          }
        }
      }
    } finally {
      this.currentSupervisor.onSuspend(this);
    }
  }
  /**
   * Begins execution of the effect associated with this fiber on the current
   * thread. This can be called to "kick off" execution of a fiber after it has
   * been created, in hopes that the effect can be executed synchronously.
   *
   * This is not the normal way of starting a fiber, but it is useful when the
   * express goal of executing the fiber is to synchronously produce its exit.
   */
  start(effect) {
    if (!this._running) {
      this._running = true;
      const prev = globalThis[currentFiberURI];
      globalThis[currentFiberURI] = this;
      try {
        this.evaluateEffect(effect);
      } finally {
        this._running = false;
        globalThis[currentFiberURI] = prev;
        if (this._queue.length > 0) {
          this.drainQueueLaterOnExecutor();
        }
      }
    } else {
      this.tell(resume(effect));
    }
  }
  /**
   * Begins execution of the effect associated with this fiber on in the
   * background, and on the correct thread pool. This can be called to "kick
   * off" execution of a fiber after it has been created, in hopes that the
   * effect can be executed synchronously.
   */
  startFork(effect) {
    this.tell(resume(effect));
  }
  /**
   * Takes the current runtime flags, patches them to return the new runtime
   * flags, and then makes any changes necessary to fiber state based on the
   * specified patch.
   *
   * **NOTE**: This method must be invoked by the fiber itself.
   */
  patchRuntimeFlags(oldRuntimeFlags, patch9) {
    const newRuntimeFlags = patch4(oldRuntimeFlags, patch9);
    globalThis[currentFiberURI] = this;
    this.currentRuntimeFlags = newRuntimeFlags;
    return newRuntimeFlags;
  }
  /**
   * Initiates an asynchronous operation, by building a callback that will
   * resume execution, and then feeding that callback to the registration
   * function, handling error cases and repeated resumptions appropriately.
   *
   * **NOTE**: This method must be invoked by the fiber itself.
   */
  initiateAsync(runtimeFlags2, asyncRegister) {
    let alreadyCalled = false;
    const callback = /* @__PURE__ */ __name((effect) => {
      if (!alreadyCalled) {
        alreadyCalled = true;
        this.tell(resume(effect));
      }
    }, "callback");
    if (interruptible(runtimeFlags2)) {
      this._asyncInterruptor = callback;
    }
    try {
      asyncRegister(callback);
    } catch (e) {
      callback(failCause(die(e)));
    }
  }
  pushStack(cont) {
    this._stack.push(cont);
    if (cont._op === "OnStep") {
      this._steps.push({
        refs: this.getFiberRefs(),
        flags: this.currentRuntimeFlags
      });
    }
  }
  popStack() {
    const item = this._stack.pop();
    if (item) {
      if (item._op === "OnStep") {
        this._steps.pop();
      }
      return item;
    }
    return;
  }
  getNextSuccessCont() {
    let frame = this.popStack();
    while (frame) {
      if (frame._op !== OP_ON_FAILURE) {
        return frame;
      }
      frame = this.popStack();
    }
  }
  getNextFailCont() {
    let frame = this.popStack();
    while (frame) {
      if (frame._op !== OP_ON_SUCCESS && frame._op !== OP_WHILE && frame._op !== OP_ITERATOR) {
        return frame;
      }
      frame = this.popStack();
    }
  }
  [OP_TAG](op) {
    return sync(() => unsafeGet3(this.currentContext, op));
  }
  ["Left"](op) {
    return fail2(op.left);
  }
  ["None"](_) {
    return fail2(new NoSuchElementException());
  }
  ["Right"](op) {
    return exitSucceed(op.right);
  }
  ["Some"](op) {
    return exitSucceed(op.value);
  }
  ["Micro"](op) {
    return unsafeAsync((microResume) => {
      let resume2 = microResume;
      const fiber = runFork(provideContext2(op, this.currentContext));
      fiber.addObserver((exit4) => {
        if (exit4._tag === "Success") {
          return resume2(exitSucceed(exit4.value));
        }
        switch (exit4.cause._tag) {
          case "Interrupt": {
            return resume2(exitFailCause(interrupt(none4)));
          }
          case "Fail": {
            return resume2(fail2(exit4.cause.error));
          }
          case "Die": {
            return resume2(die2(exit4.cause.defect));
          }
        }
      });
      return unsafeAsync((abortResume) => {
        resume2 = /* @__PURE__ */ __name((_) => {
          abortResume(void_);
        }, "resume");
        fiber.unsafeInterrupt();
      });
    });
  }
  [OP_SYNC](op) {
    const value = internalCall(() => op.effect_instruction_i0());
    const cont = this.getNextSuccessCont();
    if (cont !== void 0) {
      if (!(cont._op in contOpSuccess)) {
        absurd(cont);
      }
      return contOpSuccess[cont._op](this, cont, value);
    } else {
      yieldedOpChannel.currentOp = exitSucceed(value);
      return YieldedOp;
    }
  }
  [OP_SUCCESS](op) {
    const oldCur = op;
    const cont = this.getNextSuccessCont();
    if (cont !== void 0) {
      if (!(cont._op in contOpSuccess)) {
        absurd(cont);
      }
      return contOpSuccess[cont._op](this, cont, oldCur.effect_instruction_i0);
    } else {
      yieldedOpChannel.currentOp = oldCur;
      return YieldedOp;
    }
  }
  [OP_FAILURE](op) {
    const cause3 = op.effect_instruction_i0;
    const cont = this.getNextFailCont();
    if (cont !== void 0) {
      switch (cont._op) {
        case OP_ON_FAILURE:
        case OP_ON_SUCCESS_AND_FAILURE: {
          if (!(interruptible(this.currentRuntimeFlags) && this.isInterrupted())) {
            return internalCall(() => cont.effect_instruction_i1(cause3));
          } else {
            return exitFailCause(stripFailures(cause3));
          }
        }
        case "OnStep": {
          if (!(interruptible(this.currentRuntimeFlags) && this.isInterrupted())) {
            return exitSucceed(exitFailCause(cause3));
          } else {
            return exitFailCause(stripFailures(cause3));
          }
        }
        case OP_REVERT_FLAGS: {
          this.patchRuntimeFlags(this.currentRuntimeFlags, cont.patch);
          if (interruptible(this.currentRuntimeFlags) && this.isInterrupted()) {
            return exitFailCause(sequential(cause3, this.getInterruptedCause()));
          } else {
            return exitFailCause(cause3);
          }
        }
        default: {
          absurd(cont);
        }
      }
    } else {
      yieldedOpChannel.currentOp = exitFailCause(cause3);
      return YieldedOp;
    }
  }
  [OP_WITH_RUNTIME](op) {
    return internalCall(() => op.effect_instruction_i0(this, running2(this.currentRuntimeFlags)));
  }
  ["Blocked"](op) {
    const refs = this.getFiberRefs();
    const flags = this.currentRuntimeFlags;
    if (this._steps.length > 0) {
      const frames = [];
      const snap = this._steps[this._steps.length - 1];
      let frame = this.popStack();
      while (frame && frame._op !== "OnStep") {
        frames.push(frame);
        frame = this.popStack();
      }
      this.setFiberRefs(snap.refs);
      this.currentRuntimeFlags = snap.flags;
      const patchRefs = diff6(snap.refs, refs);
      const patchFlags = diff4(snap.flags, flags);
      return exitSucceed(blocked(op.effect_instruction_i0, withFiberRuntime((newFiber) => {
        while (frames.length > 0) {
          newFiber.pushStack(frames.pop());
        }
        newFiber.setFiberRefs(patch7(newFiber.id(), newFiber.getFiberRefs())(patchRefs));
        newFiber.currentRuntimeFlags = patch4(patchFlags)(newFiber.currentRuntimeFlags);
        return op.effect_instruction_i1;
      })));
    }
    return uninterruptibleMask((restore) => flatMap7(forkDaemon(runRequestBlock(op.effect_instruction_i0)), () => restore(op.effect_instruction_i1)));
  }
  ["RunBlocked"](op) {
    return runBlockedRequests(op.effect_instruction_i0);
  }
  [OP_UPDATE_RUNTIME_FLAGS](op) {
    const updateFlags = op.effect_instruction_i0;
    const oldRuntimeFlags = this.currentRuntimeFlags;
    const newRuntimeFlags = patch4(oldRuntimeFlags, updateFlags);
    if (interruptible(newRuntimeFlags) && this.isInterrupted()) {
      return exitFailCause(this.getInterruptedCause());
    } else {
      this.patchRuntimeFlags(this.currentRuntimeFlags, updateFlags);
      if (op.effect_instruction_i1) {
        const revertFlags = diff4(newRuntimeFlags, oldRuntimeFlags);
        this.pushStack(new RevertFlags(revertFlags, op));
        return internalCall(() => op.effect_instruction_i1(oldRuntimeFlags));
      } else {
        return exitVoid;
      }
    }
  }
  [OP_ON_SUCCESS](op) {
    this.pushStack(op);
    return op.effect_instruction_i0;
  }
  ["OnStep"](op) {
    this.pushStack(op);
    return op.effect_instruction_i0;
  }
  [OP_ON_FAILURE](op) {
    this.pushStack(op);
    return op.effect_instruction_i0;
  }
  [OP_ON_SUCCESS_AND_FAILURE](op) {
    this.pushStack(op);
    return op.effect_instruction_i0;
  }
  [OP_ASYNC](op) {
    this._asyncBlockingOn = op.effect_instruction_i1;
    this.initiateAsync(this.currentRuntimeFlags, op.effect_instruction_i0);
    yieldedOpChannel.currentOp = op;
    return YieldedOp;
  }
  [OP_YIELD](op) {
    this._isYielding = false;
    yieldedOpChannel.currentOp = op;
    return YieldedOp;
  }
  [OP_WHILE](op) {
    const check2 = op.effect_instruction_i0;
    const body = op.effect_instruction_i1;
    if (check2()) {
      this.pushStack(op);
      return body();
    } else {
      return exitVoid;
    }
  }
  [OP_ITERATOR](op) {
    return contOpSuccess[OP_ITERATOR](this, op, void 0);
  }
  [OP_COMMIT](op) {
    return internalCall(() => op.commit());
  }
  /**
   * The main run-loop for evaluating effects.
   *
   * **NOTE**: This method must be invoked by the fiber itself.
   */
  runLoop(effect0) {
    let cur = effect0;
    this.currentOpCount = 0;
    while (true) {
      if ((this.currentRuntimeFlags & OpSupervision) !== 0) {
        this.currentSupervisor.onEffect(this, cur);
      }
      if (this._queue.length > 0) {
        cur = this.drainQueueWhileRunning(this.currentRuntimeFlags, cur);
      }
      if (!this._isYielding) {
        this.currentOpCount += 1;
        const shouldYield = this.currentScheduler.shouldYield(this);
        if (shouldYield !== false) {
          this._isYielding = true;
          this.currentOpCount = 0;
          const oldCur = cur;
          cur = flatMap7(yieldNow({
            priority: shouldYield
          }), () => oldCur);
        }
      }
      try {
        cur = this.currentTracer.context(() => {
          if (_version !== cur[EffectTypeId2]._V) {
            const level = this.getFiberRef(currentVersionMismatchErrorLogLevel);
            if (level._tag === "Some") {
              const effectVersion = cur[EffectTypeId2]._V;
              this.log(`Executing an Effect versioned ${effectVersion} with a Runtime of version ${getCurrentVersion()}, you may want to dedupe the effect dependencies, you can use the language service plugin to detect this at compile time: https://github.com/Effect-TS/language-service`, empty16, level);
            }
          }
          return this[cur._op](cur);
        }, this);
        if (cur === YieldedOp) {
          const op = yieldedOpChannel.currentOp;
          if (op._op === OP_YIELD || op._op === OP_ASYNC) {
            return YieldedOp;
          }
          yieldedOpChannel.currentOp = null;
          return op._op === OP_SUCCESS || op._op === OP_FAILURE ? op : exitFailCause(die(op));
        }
      } catch (e) {
        if (cur !== YieldedOp && !hasProperty(cur, "_op") || !(cur._op in this)) {
          cur = dieMessage(`Not a valid effect: ${toStringUnknown(cur)}`);
        } else if (isInterruptedException(e)) {
          cur = exitFailCause(sequential(die(e), interrupt(none4)));
        } else {
          cur = die2(e);
        }
      }
    }
  }
  run = /* @__PURE__ */ __name(() => {
    this.drainQueueOnCurrentThread();
  }, "run");
};
var currentMinimumLogLevel = /* @__PURE__ */ globalValue("effect/FiberRef/currentMinimumLogLevel", () => fiberRefUnsafeMake(fromLiteral("Info")));
var loggerWithConsoleLog = /* @__PURE__ */ __name((self) => makeLogger((opts) => {
  const services = getOrDefault2(opts.context, currentServices);
  get3(services, consoleTag).unsafe.log(self.log(opts));
}), "loggerWithConsoleLog");
var defaultLogger = /* @__PURE__ */ globalValue(/* @__PURE__ */ Symbol.for("effect/Logger/defaultLogger"), () => loggerWithConsoleLog(stringLogger));
var tracerLogger = /* @__PURE__ */ globalValue(/* @__PURE__ */ Symbol.for("effect/Logger/tracerLogger"), () => makeLogger(({
  annotations,
  cause: cause3,
  context: context4,
  fiberId: fiberId3,
  logLevel,
  message
}) => {
  const span2 = filterDisablePropagation(getOption2(getOrDefault(context4, currentContext), spanTag));
  if (span2._tag === "None" || span2.value._tag === "ExternalSpan") {
    return;
  }
  const clockService = unsafeGet3(getOrDefault(context4, currentServices), clockTag);
  const attributes = {};
  for (const [key, value] of annotations) {
    attributes[key] = value;
  }
  attributes["effect.fiberId"] = threadName2(fiberId3);
  attributes["effect.logLevel"] = logLevel.label;
  if (cause3 !== null && cause3._tag !== "Empty") {
    attributes["effect.cause"] = pretty(cause3, {
      renderErrorCause: true
    });
  }
  span2.value.event(toStringUnknown(Array.isArray(message) && message.length === 1 ? message[0] : message), clockService.unsafeCurrentTimeNanos(), attributes);
}));
var currentLoggers = /* @__PURE__ */ globalValue(/* @__PURE__ */ Symbol.for("effect/FiberRef/currentLoggers"), () => fiberRefUnsafeMakeHashSet(make10(defaultLogger, tracerLogger)));
var annotateLogsScoped = /* @__PURE__ */ __name(function() {
  if (typeof arguments[0] === "string") {
    return fiberRefLocallyScopedWith(currentLogAnnotations, set3(arguments[0], arguments[1]));
  }
  const entries2 = Object.entries(arguments[0]);
  return fiberRefLocallyScopedWith(currentLogAnnotations, mutate3((annotations) => {
    for (let i = 0; i < entries2.length; i++) {
      const [key, value] = entries2[i];
      set3(annotations, key, value);
    }
    return annotations;
  }));
}, "annotateLogsScoped");
var whenLogLevel = /* @__PURE__ */ dual(2, (effect, level) => {
  const requiredLogLevel = typeof level === "string" ? fromLiteral(level) : level;
  return withFiberRuntime((fiberState) => {
    const minimumLogLevel = fiberState.getFiberRef(currentMinimumLogLevel);
    if (greaterThan3(minimumLogLevel, requiredLogLevel)) {
      return succeed(none2());
    }
    return map8(effect, some2);
  });
});
var acquireRelease = /* @__PURE__ */ dual((args2) => isEffect(args2[0]), (acquire, release) => uninterruptible(tap(acquire, (a) => addFinalizer((exit4) => release(a, exit4)))));
var acquireReleaseInterruptible = /* @__PURE__ */ dual((args2) => isEffect(args2[0]), (acquire, release) => ensuring(acquire, addFinalizer((exit4) => release(exit4))));
var addFinalizer = /* @__PURE__ */ __name((finalizer) => withFiberRuntime((runtime4) => {
  const acquireRefs = runtime4.getFiberRefs();
  const acquireFlags = disable2(runtime4.currentRuntimeFlags, Interruption);
  return flatMap7(scope, (scope3) => scopeAddFinalizerExit(scope3, (exit4) => withFiberRuntime((runtimeFinalizer) => {
    const preRefs = runtimeFinalizer.getFiberRefs();
    const preFlags = runtimeFinalizer.currentRuntimeFlags;
    const patchRefs = diff6(preRefs, acquireRefs);
    const patchFlags = diff4(preFlags, acquireFlags);
    const inverseRefs = diff6(acquireRefs, preRefs);
    runtimeFinalizer.setFiberRefs(patch7(patchRefs, runtimeFinalizer.id(), acquireRefs));
    return ensuring(withRuntimeFlags(finalizer(exit4), patchFlags), sync(() => {
      runtimeFinalizer.setFiberRefs(patch7(inverseRefs, runtimeFinalizer.id(), runtimeFinalizer.getFiberRefs()));
    }));
  })));
}), "addFinalizer");
var daemonChildren = /* @__PURE__ */ __name((self) => {
  const forkScope = fiberRefLocally(currentForkScopeOverride, some2(globalScope));
  return forkScope(self);
}, "daemonChildren");
var _existsParFound = /* @__PURE__ */ Symbol.for("effect/Effect/existsPar/found");
var exists2 = /* @__PURE__ */ dual((args2) => isIterable(args2[0]) && !isEffect(args2[0]), (elements, predicate, options) => matchSimple(options?.concurrency, () => suspend(() => existsLoop(elements[Symbol.iterator](), 0, predicate)), () => matchEffect(forEach7(elements, (a, i) => if_(predicate(a, i), {
  onTrue: /* @__PURE__ */ __name(() => fail2(_existsParFound), "onTrue"),
  onFalse: /* @__PURE__ */ __name(() => void_, "onFalse")
}), options), {
  onFailure: /* @__PURE__ */ __name((e) => e === _existsParFound ? succeed(true) : fail2(e), "onFailure"),
  onSuccess: /* @__PURE__ */ __name(() => succeed(false), "onSuccess")
})));
var existsLoop = /* @__PURE__ */ __name((iterator, index, f) => {
  const next = iterator.next();
  if (next.done) {
    return succeed(false);
  }
  return flatMap7(f(next.value, index), (b) => b ? succeed(b) : existsLoop(iterator, index + 1, f));
}, "existsLoop");
var filter5 = /* @__PURE__ */ dual((args2) => isIterable(args2[0]) && !isEffect(args2[0]), (elements, predicate, options) => {
  const predicate_ = options?.negate ? (a, i) => map8(predicate(a, i), not) : predicate;
  return matchSimple(options?.concurrency, () => suspend(() => fromIterable(elements).reduceRight((effect, a, i) => zipWith2(effect, suspend(() => predicate_(a, i)), (list, b) => b ? [a, ...list] : list), sync(() => new Array()))), () => map8(forEach7(elements, (a, i) => map8(predicate_(a, i), (b) => b ? some2(a) : none2()), options), getSomes));
});
var allResolveInput = /* @__PURE__ */ __name((input) => {
  if (Array.isArray(input) || isIterable(input)) {
    return [input, none2()];
  }
  const keys5 = Object.keys(input);
  const size11 = keys5.length;
  return [keys5.map((k) => input[k]), some2((values3) => {
    const res = {};
    for (let i = 0; i < size11; i++) {
      ;
      res[keys5[i]] = values3[i];
    }
    return res;
  })];
}, "allResolveInput");
var allValidate = /* @__PURE__ */ __name((effects, reconcile, options) => {
  const eitherEffects = [];
  for (const effect of effects) {
    eitherEffects.push(either2(effect));
  }
  return flatMap7(forEach7(eitherEffects, identity, {
    concurrency: options?.concurrency,
    batching: options?.batching,
    concurrentFinalizers: options?.concurrentFinalizers
  }), (eithers) => {
    const none10 = none2();
    const size11 = eithers.length;
    const errors = new Array(size11);
    const successes = new Array(size11);
    let errored = false;
    for (let i = 0; i < size11; i++) {
      const either4 = eithers[i];
      if (either4._tag === "Left") {
        errors[i] = some2(either4.left);
        errored = true;
      } else {
        successes[i] = either4.right;
        errors[i] = none10;
      }
    }
    if (errored) {
      return reconcile._tag === "Some" ? fail2(reconcile.value(errors)) : fail2(errors);
    } else if (options?.discard) {
      return void_;
    }
    return reconcile._tag === "Some" ? succeed(reconcile.value(successes)) : succeed(successes);
  });
}, "allValidate");
var allEither = /* @__PURE__ */ __name((effects, reconcile, options) => {
  const eitherEffects = [];
  for (const effect of effects) {
    eitherEffects.push(either2(effect));
  }
  if (options?.discard) {
    return forEach7(eitherEffects, identity, {
      concurrency: options?.concurrency,
      batching: options?.batching,
      discard: true,
      concurrentFinalizers: options?.concurrentFinalizers
    });
  }
  return map8(forEach7(eitherEffects, identity, {
    concurrency: options?.concurrency,
    batching: options?.batching,
    concurrentFinalizers: options?.concurrentFinalizers
  }), (eithers) => reconcile._tag === "Some" ? reconcile.value(eithers) : eithers);
}, "allEither");
var all3 = /* @__PURE__ */ __name((arg, options) => {
  const [effects, reconcile] = allResolveInput(arg);
  if (options?.mode === "validate") {
    return allValidate(effects, reconcile, options);
  } else if (options?.mode === "either") {
    return allEither(effects, reconcile, options);
  }
  return options?.discard !== true && reconcile._tag === "Some" ? map8(forEach7(effects, identity, options), reconcile.value) : forEach7(effects, identity, options);
}, "all");
var allWith = /* @__PURE__ */ __name((options) => (arg) => all3(arg, options), "allWith");
var allSuccesses = /* @__PURE__ */ __name((elements, options) => map8(all3(fromIterable(elements).map(exit), options), filterMap((exit4) => exitIsSuccess(exit4) ? some2(exit4.effect_instruction_i0) : none2())), "allSuccesses");
var replicate = /* @__PURE__ */ dual(2, (self, n) => Array.from({
  length: n
}, () => self));
var replicateEffect = /* @__PURE__ */ dual((args2) => isEffect(args2[0]), (self, n, options) => all3(replicate(self, n), options));
var forEach7 = /* @__PURE__ */ dual((args2) => isIterable(args2[0]), (self, f, options) => withFiberRuntime((r) => {
  const isRequestBatchingEnabled = options?.batching === true || options?.batching === "inherit" && r.getFiberRef(currentRequestBatching);
  if (options?.discard) {
    return match9(options.concurrency, () => finalizersMaskInternal(sequential3, options?.concurrentFinalizers)((restore) => isRequestBatchingEnabled ? forEachConcurrentDiscard(self, (a, i) => restore(f(a, i)), true, false, 1) : forEachSequentialDiscard(self, (a, i) => restore(f(a, i)))), () => finalizersMaskInternal(parallel3, options?.concurrentFinalizers)((restore) => forEachConcurrentDiscard(self, (a, i) => restore(f(a, i)), isRequestBatchingEnabled, false)), (n) => finalizersMaskInternal(parallelN2(n), options?.concurrentFinalizers)((restore) => forEachConcurrentDiscard(self, (a, i) => restore(f(a, i)), isRequestBatchingEnabled, false, n)));
  }
  return match9(options?.concurrency, () => finalizersMaskInternal(sequential3, options?.concurrentFinalizers)((restore) => isRequestBatchingEnabled ? forEachParN(self, 1, (a, i) => restore(f(a, i)), true) : forEachSequential(self, (a, i) => restore(f(a, i)))), () => finalizersMaskInternal(parallel3, options?.concurrentFinalizers)((restore) => forEachParUnbounded(self, (a, i) => restore(f(a, i)), isRequestBatchingEnabled)), (n) => finalizersMaskInternal(parallelN2(n), options?.concurrentFinalizers)((restore) => forEachParN(self, n, (a, i) => restore(f(a, i)), isRequestBatchingEnabled)));
}));
var forEachParUnbounded = /* @__PURE__ */ __name((self, f, batching) => suspend(() => {
  const as7 = fromIterable(self);
  const array3 = new Array(as7.length);
  const fn2 = /* @__PURE__ */ __name((a, i) => flatMap7(f(a, i), (b) => sync(() => array3[i] = b)), "fn");
  return zipRight(forEachConcurrentDiscard(as7, fn2, batching, false), succeed(array3));
}), "forEachParUnbounded");
var forEachConcurrentDiscard = /* @__PURE__ */ __name((self, f, batching, processAll, n) => uninterruptibleMask((restore) => transplant((graft) => withFiberRuntime((parent) => {
  let todos = Array.from(self).reverse();
  let target = todos.length;
  if (target === 0) {
    return void_;
  }
  let counter6 = 0;
  let interrupted = false;
  const fibersCount = n ? Math.min(todos.length, n) : todos.length;
  const fibers = /* @__PURE__ */ new Set();
  const results = new Array();
  const interruptAll = /* @__PURE__ */ __name(() => fibers.forEach((fiber) => {
    fiber.currentScheduler.scheduleTask(() => {
      fiber.unsafeInterruptAsFork(parent.id());
    }, 0, fiber);
  }), "interruptAll");
  const startOrder = new Array();
  const joinOrder = new Array();
  const residual = new Array();
  const collectExits = /* @__PURE__ */ __name(() => {
    const exits = results.filter(({
      exit: exit4
    }) => exit4._tag === "Failure").sort((a, b) => a.index < b.index ? -1 : a.index === b.index ? 0 : 1).map(({
      exit: exit4
    }) => exit4);
    if (exits.length === 0) {
      exits.push(exitVoid);
    }
    return exits;
  }, "collectExits");
  const runFiber = /* @__PURE__ */ __name((eff, interruptImmediately = false) => {
    const runnable = uninterruptible(graft(eff));
    const fiber = unsafeForkUnstarted(runnable, parent, parent.currentRuntimeFlags, globalScope);
    parent.currentScheduler.scheduleTask(() => {
      if (interruptImmediately) {
        fiber.unsafeInterruptAsFork(parent.id());
      }
      fiber.resume(runnable);
    }, 0, fiber);
    return fiber;
  }, "runFiber");
  const onInterruptSignal = /* @__PURE__ */ __name(() => {
    if (!processAll) {
      target -= todos.length;
      todos = [];
    }
    interrupted = true;
    interruptAll();
  }, "onInterruptSignal");
  const stepOrExit = batching ? step2 : exit;
  const processingFiber = runFiber(async_((resume2) => {
    const pushResult = /* @__PURE__ */ __name((res, index) => {
      if (res._op === "Blocked") {
        residual.push(res);
      } else {
        results.push({
          index,
          exit: res
        });
        if (res._op === "Failure" && !interrupted) {
          onInterruptSignal();
        }
      }
    }, "pushResult");
    const next = /* @__PURE__ */ __name(() => {
      if (todos.length > 0) {
        const a = todos.pop();
        let index = counter6++;
        const returnNextElement = /* @__PURE__ */ __name(() => {
          const a2 = todos.pop();
          index = counter6++;
          return flatMap7(yieldNow(), () => flatMap7(stepOrExit(restore(f(a2, index))), onRes));
        }, "returnNextElement");
        const onRes = /* @__PURE__ */ __name((res) => {
          if (todos.length > 0) {
            pushResult(res, index);
            if (todos.length > 0) {
              return returnNextElement();
            }
          }
          return succeed(res);
        }, "onRes");
        const todo = flatMap7(stepOrExit(restore(f(a, index))), onRes);
        const fiber = runFiber(todo);
        startOrder.push(fiber);
        fibers.add(fiber);
        if (interrupted) {
          fiber.currentScheduler.scheduleTask(() => {
            fiber.unsafeInterruptAsFork(parent.id());
          }, 0, fiber);
        }
        fiber.addObserver((wrapped) => {
          let exit4;
          if (wrapped._op === "Failure") {
            exit4 = wrapped;
          } else {
            exit4 = wrapped.effect_instruction_i0;
          }
          joinOrder.push(fiber);
          fibers.delete(fiber);
          pushResult(exit4, index);
          if (results.length === target) {
            resume2(succeed(getOrElse(exitCollectAll(collectExits(), {
              parallel: true
            }), () => exitVoid)));
          } else if (residual.length + results.length === target) {
            const exits = collectExits();
            const requests = residual.map((blocked3) => blocked3.effect_instruction_i0).reduce(par);
            resume2(succeed(blocked(requests, forEachConcurrentDiscard([getOrElse(exitCollectAll(exits, {
              parallel: true
            }), () => exitVoid), ...residual.map((blocked3) => blocked3.effect_instruction_i1)], (i) => i, batching, true, n))));
          } else {
            next();
          }
        });
      }
    }, "next");
    for (let i = 0; i < fibersCount; i++) {
      next();
    }
  }));
  return asVoid(onExit(flatten4(restore(join2(processingFiber))), exitMatch({
    onFailure: /* @__PURE__ */ __name((cause3) => {
      onInterruptSignal();
      const target2 = residual.length + 1;
      const concurrency = Math.min(typeof n === "number" ? n : residual.length, residual.length);
      const toPop = Array.from(residual);
      return async_((cb) => {
        const exits = [];
        let count = 0;
        let index = 0;
        const check2 = /* @__PURE__ */ __name((index2, hitNext) => (exit4) => {
          exits[index2] = exit4;
          count++;
          if (count === target2) {
            cb(exitSucceed(exitFailCause(cause3)));
          }
          if (toPop.length > 0 && hitNext) {
            next();
          }
        }, "check");
        const next = /* @__PURE__ */ __name(() => {
          runFiber(toPop.pop(), true).addObserver(check2(index, true));
          index++;
        }, "next");
        processingFiber.addObserver(check2(index, false));
        index++;
        for (let i = 0; i < concurrency; i++) {
          next();
        }
      });
    }, "onFailure"),
    onSuccess: /* @__PURE__ */ __name(() => forEachSequential(joinOrder, (f2) => f2.inheritAll), "onSuccess")
  })));
}))), "forEachConcurrentDiscard");
var forEachParN = /* @__PURE__ */ __name((self, n, f, batching) => suspend(() => {
  const as7 = fromIterable(self);
  const array3 = new Array(as7.length);
  const fn2 = /* @__PURE__ */ __name((a, i) => map8(f(a, i), (b) => array3[i] = b), "fn");
  return zipRight(forEachConcurrentDiscard(as7, fn2, batching, false, n), succeed(array3));
}), "forEachParN");
var fork = /* @__PURE__ */ __name((self) => withFiberRuntime((state, status) => succeed(unsafeFork2(self, state, status.runtimeFlags))), "fork");
var forkDaemon = /* @__PURE__ */ __name((self) => forkWithScopeOverride(self, globalScope), "forkDaemon");
var forkWithErrorHandler = /* @__PURE__ */ dual(2, (self, handler) => fork(onError(self, (cause3) => {
  const either4 = failureOrCause(cause3);
  switch (either4._tag) {
    case "Left":
      return handler(either4.left);
    case "Right":
      return failCause(either4.right);
  }
})));
var unsafeFork2 = /* @__PURE__ */ __name((effect, parentFiber, parentRuntimeFlags, overrideScope = null) => {
  const childFiber = unsafeMakeChildFiber(effect, parentFiber, parentRuntimeFlags, overrideScope);
  childFiber.resume(effect);
  return childFiber;
}, "unsafeFork");
var unsafeForkUnstarted = /* @__PURE__ */ __name((effect, parentFiber, parentRuntimeFlags, overrideScope = null) => {
  const childFiber = unsafeMakeChildFiber(effect, parentFiber, parentRuntimeFlags, overrideScope);
  return childFiber;
}, "unsafeForkUnstarted");
var unsafeMakeChildFiber = /* @__PURE__ */ __name((effect, parentFiber, parentRuntimeFlags, overrideScope = null) => {
  const childId = unsafeMake2();
  const parentFiberRefs = parentFiber.getFiberRefs();
  const childFiberRefs = forkAs(parentFiberRefs, childId);
  const childFiber = new FiberRuntime(childId, childFiberRefs, parentRuntimeFlags);
  const childContext = getOrDefault(childFiberRefs, currentContext);
  const supervisor = childFiber.currentSupervisor;
  supervisor.onStart(childContext, effect, some2(parentFiber), childFiber);
  childFiber.addObserver((exit4) => supervisor.onEnd(exit4, childFiber));
  const parentScope = overrideScope !== null ? overrideScope : pipe(parentFiber.getFiberRef(currentForkScopeOverride), getOrElse(() => parentFiber.scope()));
  parentScope.add(parentRuntimeFlags, childFiber);
  return childFiber;
}, "unsafeMakeChildFiber");
var forkWithScopeOverride = /* @__PURE__ */ __name((self, scopeOverride) => withFiberRuntime((parentFiber, parentStatus) => succeed(unsafeFork2(self, parentFiber, parentStatus.runtimeFlags, scopeOverride))), "forkWithScopeOverride");
var mergeAll3 = /* @__PURE__ */ dual((args2) => isFunction2(args2[2]), (elements, zero2, f, options) => matchSimple(options?.concurrency, () => fromIterable(elements).reduce((acc, a, i) => zipWith2(acc, a, (acc2, a2) => f(acc2, a2, i)), succeed(zero2)), () => flatMap7(make27(zero2), (acc) => flatMap7(forEach7(elements, (effect, i) => flatMap7(effect, (a) => update3(acc, (b) => f(b, a, i))), options), () => get12(acc)))));
var partition3 = /* @__PURE__ */ dual((args2) => isIterable(args2[0]), (elements, f, options) => pipe(forEach7(elements, (a, i) => either2(f(a, i)), options), map8((chunk2) => partitionMap2(chunk2, identity))));
var validateAll = /* @__PURE__ */ dual((args2) => isIterable(args2[0]), (elements, f, options) => flatMap7(partition3(elements, f, {
  concurrency: options?.concurrency,
  batching: options?.batching,
  concurrentFinalizers: options?.concurrentFinalizers
}), ([es, bs]) => isNonEmptyArray2(es) ? fail2(es) : options?.discard ? void_ : succeed(bs)));
var raceAll = /* @__PURE__ */ __name((all5) => withFiberRuntime((state, status) => async_((resume2) => {
  const fibers = /* @__PURE__ */ new Set();
  let winner;
  let failures3 = empty16;
  const interruptAll = /* @__PURE__ */ __name(() => {
    for (const fiber of fibers) {
      fiber.unsafeInterruptAsFork(state.id());
    }
  }, "interruptAll");
  let latch = false;
  let empty30 = true;
  for (const self of all5) {
    empty30 = false;
    const fiber = unsafeFork2(interruptible2(self), state, status.runtimeFlags);
    fibers.add(fiber);
    fiber.addObserver((exit4) => {
      fibers.delete(fiber);
      if (!winner) {
        if (exit4._tag === "Success") {
          latch = true;
          winner = fiber;
          failures3 = empty16;
          interruptAll();
        } else {
          failures3 = parallel(exit4.cause, failures3);
        }
      }
      if (latch && fibers.size === 0) {
        resume2(winner ? zipRight(inheritAll(winner), winner.unsafePoll()) : failCause(failures3));
      }
    });
    if (winner) break;
  }
  if (empty30) {
    return resume2(dieSync(() => new IllegalArgumentException(`Received an empty collection of effects`)));
  }
  latch = true;
  return interruptAllAs(fibers, state.id());
})), "raceAll");
var reduceEffect = /* @__PURE__ */ dual((args2) => isIterable(args2[0]) && !isEffect(args2[0]), (elements, zero2, f, options) => matchSimple(options?.concurrency, () => fromIterable(elements).reduce((acc, a, i) => zipWith2(acc, a, (acc2, a2) => f(acc2, a2, i)), zero2), () => suspend(() => pipe(mergeAll3([zero2, ...elements], none2(), (acc, elem, i) => {
  switch (acc._tag) {
    case "None": {
      return some2(elem);
    }
    case "Some": {
      return some2(f(acc.value, elem, i));
    }
  }
}, options), map8((option3) => {
  switch (option3._tag) {
    case "None": {
      throw new Error("BUG: Effect.reduceEffect - please report an issue at https://github.com/Effect-TS/effect/issues");
    }
    case "Some": {
      return option3.value;
    }
  }
})))));
var parallelFinalizers = /* @__PURE__ */ __name((self) => contextWithEffect((context4) => match2(getOption2(context4, scopeTag), {
  onNone: /* @__PURE__ */ __name(() => self, "onNone"),
  onSome: /* @__PURE__ */ __name((scope3) => {
    switch (scope3.strategy._tag) {
      case "Parallel":
        return self;
      case "Sequential":
      case "ParallelN":
        return flatMap7(scopeFork(scope3, parallel3), (inner) => scopeExtend(self, inner));
    }
  }, "onSome")
})), "parallelFinalizers");
var parallelNFinalizers = /* @__PURE__ */ __name((parallelism) => (self) => contextWithEffect((context4) => match2(getOption2(context4, scopeTag), {
  onNone: /* @__PURE__ */ __name(() => self, "onNone"),
  onSome: /* @__PURE__ */ __name((scope3) => {
    if (scope3.strategy._tag === "ParallelN" && scope3.strategy.parallelism === parallelism) {
      return self;
    }
    return flatMap7(scopeFork(scope3, parallelN2(parallelism)), (inner) => scopeExtend(self, inner));
  }, "onSome")
})), "parallelNFinalizers");
var finalizersMask = /* @__PURE__ */ __name((strategy) => (self) => finalizersMaskInternal(strategy, true)(self), "finalizersMask");
var finalizersMaskInternal = /* @__PURE__ */ __name((strategy, concurrentFinalizers) => (self) => contextWithEffect((context4) => match2(getOption2(context4, scopeTag), {
  onNone: /* @__PURE__ */ __name(() => self(identity), "onNone"),
  onSome: /* @__PURE__ */ __name((scope3) => {
    if (concurrentFinalizers === true) {
      const patch9 = strategy._tag === "Parallel" ? parallelFinalizers : strategy._tag === "Sequential" ? sequentialFinalizers : parallelNFinalizers(strategy.parallelism);
      switch (scope3.strategy._tag) {
        case "Parallel":
          return patch9(self(parallelFinalizers));
        case "Sequential":
          return patch9(self(sequentialFinalizers));
        case "ParallelN":
          return patch9(self(parallelNFinalizers(scope3.strategy.parallelism)));
      }
    } else {
      return self(identity);
    }
  }, "onSome")
})), "finalizersMaskInternal");
var scopeWith = /* @__PURE__ */ __name((f) => flatMap7(scopeTag, f), "scopeWith");
var scopedWith = /* @__PURE__ */ __name((f) => flatMap7(scopeMake(), (scope3) => onExit(f(scope3), (exit4) => scope3.close(exit4))), "scopedWith");
var scopedEffect = /* @__PURE__ */ __name((effect) => flatMap7(scopeMake(), (scope3) => scopeUse(effect, scope3)), "scopedEffect");
var sequentialFinalizers = /* @__PURE__ */ __name((self) => contextWithEffect((context4) => match2(getOption2(context4, scopeTag), {
  onNone: /* @__PURE__ */ __name(() => self, "onNone"),
  onSome: /* @__PURE__ */ __name((scope3) => {
    switch (scope3.strategy._tag) {
      case "Sequential":
        return self;
      case "Parallel":
      case "ParallelN":
        return flatMap7(scopeFork(scope3, sequential3), (inner) => scopeExtend(self, inner));
    }
  }, "onSome")
})), "sequentialFinalizers");
var tagMetricsScoped = /* @__PURE__ */ __name((key, value) => labelMetricsScoped([make28(key, value)]), "tagMetricsScoped");
var labelMetricsScoped = /* @__PURE__ */ __name((labels) => fiberRefLocallyScopedWith(currentMetricLabels, (old) => union(old, labels)), "labelMetricsScoped");
var using = /* @__PURE__ */ dual(2, (self, use) => scopedWith((scope3) => flatMap7(scopeExtend(self, scope3), use)));
var validate = /* @__PURE__ */ dual((args2) => isEffect(args2[1]), (self, that, options) => validateWith(self, that, (a, b) => [a, b], options));
var validateWith = /* @__PURE__ */ dual((args2) => isEffect(args2[1]), (self, that, f, options) => flatten4(zipWithOptions(exit(self), exit(that), (ea, eb) => exitZipWith(ea, eb, {
  onSuccess: f,
  onFailure: /* @__PURE__ */ __name((ca, cb) => options?.concurrent ? parallel(ca, cb) : sequential(ca, cb), "onFailure")
}), options)));
var validateFirst = /* @__PURE__ */ dual((args2) => isIterable(args2[0]), (elements, f, options) => flip(forEach7(elements, (a, i) => flip(f(a, i)), options)));
var withClockScoped = /* @__PURE__ */ __name((c) => fiberRefLocallyScopedWith(currentServices, add2(clockTag, c)), "withClockScoped");
var withRandomScoped = /* @__PURE__ */ __name((value) => fiberRefLocallyScopedWith(currentServices, add2(randomTag, value)), "withRandomScoped");
var withConfigProviderScoped = /* @__PURE__ */ __name((provider) => fiberRefLocallyScopedWith(currentServices, add2(configProviderTag, provider)), "withConfigProviderScoped");
var withEarlyRelease = /* @__PURE__ */ __name((self) => scopeWith((parent) => flatMap7(scopeFork(parent, sequential2), (child) => pipe(self, scopeExtend(child), map8((value) => [fiberIdWith((fiberId3) => scopeClose(child, exitInterrupt(fiberId3))), value])))), "withEarlyRelease");
var zipOptions = /* @__PURE__ */ dual((args2) => isEffect(args2[1]), (self, that, options) => zipWithOptions(self, that, (a, b) => [a, b], options));
var zipLeftOptions = /* @__PURE__ */ dual((args2) => isEffect(args2[1]), (self, that, options) => {
  if (options?.concurrent !== true && (options?.batching === void 0 || options.batching === false)) {
    return zipLeft(self, that);
  }
  return zipWithOptions(self, that, (a, _) => a, options);
});
var zipRightOptions = /* @__PURE__ */ dual((args2) => isEffect(args2[1]), (self, that, options) => {
  if (options?.concurrent !== true && (options?.batching === void 0 || options.batching === false)) {
    return zipRight(self, that);
  }
  return zipWithOptions(self, that, (_, b) => b, options);
});
var zipWithOptions = /* @__PURE__ */ dual((args2) => isEffect(args2[1]), (self, that, f, options) => map8(all3([self, that], {
  concurrency: options?.concurrent ? 2 : 1,
  batching: options?.batching,
  concurrentFinalizers: options?.concurrentFinalizers
}), ([a, a2]) => f(a, a2)));
var withRuntimeFlagsScoped = /* @__PURE__ */ __name((update5) => {
  if (update5 === empty14) {
    return void_;
  }
  return pipe(runtimeFlags, flatMap7((runtimeFlags2) => {
    const updatedRuntimeFlags = patch4(runtimeFlags2, update5);
    const revertRuntimeFlags = diff4(updatedRuntimeFlags, runtimeFlags2);
    return pipe(updateRuntimeFlags(update5), zipRight(addFinalizer(() => updateRuntimeFlags(revertRuntimeFlags))), asVoid);
  }), uninterruptible);
}, "withRuntimeFlagsScoped");
var scopeTag = /* @__PURE__ */ GenericTag("effect/Scope");
var scope = scopeTag;
var scopeUnsafeAddFinalizer = /* @__PURE__ */ __name((scope3, fin) => {
  if (scope3.state._tag === "Open") {
    scope3.state.finalizers.set({}, fin);
  }
}, "scopeUnsafeAddFinalizer");
var ScopeImplProto = {
  [ScopeTypeId]: ScopeTypeId,
  [CloseableScopeTypeId]: CloseableScopeTypeId,
  pipe() {
    return pipeArguments(this, arguments);
  },
  fork(strategy) {
    return sync(() => {
      const newScope = scopeUnsafeMake(strategy);
      if (this.state._tag === "Closed") {
        newScope.state = this.state;
        return newScope;
      }
      const key = {};
      const fin = /* @__PURE__ */ __name((exit4) => newScope.close(exit4), "fin");
      this.state.finalizers.set(key, fin);
      scopeUnsafeAddFinalizer(newScope, (_) => sync(() => {
        if (this.state._tag === "Open") {
          this.state.finalizers.delete(key);
        }
      }));
      return newScope;
    });
  },
  close(exit4) {
    return suspend(() => {
      if (this.state._tag === "Closed") {
        return void_;
      }
      const finalizers = Array.from(this.state.finalizers.values()).reverse();
      this.state = {
        _tag: "Closed",
        exit: exit4
      };
      if (finalizers.length === 0) {
        return void_;
      }
      return isSequential(this.strategy) ? pipe(forEachSequential(finalizers, (fin) => exit(fin(exit4))), flatMap7((results) => pipe(exitCollectAll(results), map(exitAsVoid), getOrElse(() => exitVoid)))) : isParallel(this.strategy) ? pipe(forEachParUnbounded(finalizers, (fin) => exit(fin(exit4)), false), flatMap7((results) => pipe(exitCollectAll(results, {
        parallel: true
      }), map(exitAsVoid), getOrElse(() => exitVoid)))) : pipe(forEachParN(finalizers, this.strategy.parallelism, (fin) => exit(fin(exit4)), false), flatMap7((results) => pipe(exitCollectAll(results, {
        parallel: true
      }), map(exitAsVoid), getOrElse(() => exitVoid))));
    });
  },
  addFinalizer(fin) {
    return suspend(() => {
      if (this.state._tag === "Closed") {
        return fin(this.state.exit);
      }
      this.state.finalizers.set({}, fin);
      return void_;
    });
  }
};
var scopeUnsafeMake = /* @__PURE__ */ __name((strategy = sequential2) => {
  const scope3 = Object.create(ScopeImplProto);
  scope3.strategy = strategy;
  scope3.state = {
    _tag: "Open",
    finalizers: /* @__PURE__ */ new Map()
  };
  return scope3;
}, "scopeUnsafeMake");
var scopeMake = /* @__PURE__ */ __name((strategy = sequential2) => sync(() => scopeUnsafeMake(strategy)), "scopeMake");
var scopeExtend = /* @__PURE__ */ dual(2, (effect, scope3) => mapInputContext(
  effect,
  // @ts-expect-error
  merge3(make5(scopeTag, scope3))
));
var scopeUse = /* @__PURE__ */ dual(2, (effect, scope3) => pipe(effect, scopeExtend(scope3), onExit((exit4) => scope3.close(exit4))));
var fiberRefUnsafeMakeSupervisor = /* @__PURE__ */ __name((initial) => fiberRefUnsafeMakePatch(initial, {
  differ: differ2,
  fork: empty25
}), "fiberRefUnsafeMakeSupervisor");
var fiberRefLocallyScoped = /* @__PURE__ */ dual(2, (self, value) => asVoid(acquireRelease(flatMap7(fiberRefGet(self), (oldValue) => as2(fiberRefSet(self, value), oldValue)), (oldValue) => fiberRefSet(self, oldValue))));
var fiberRefLocallyScopedWith = /* @__PURE__ */ dual(2, (self, f) => fiberRefGetWith(self, (a) => fiberRefLocallyScoped(self, f(a))));
var currentRuntimeFlags = /* @__PURE__ */ fiberRefUnsafeMakeRuntimeFlags(none5);
var currentSupervisor = /* @__PURE__ */ fiberRefUnsafeMakeSupervisor(none8);
var fiberAwaitAll = /* @__PURE__ */ __name((fibers) => forEach7(fibers, _await2), "fiberAwaitAll");
var fiberAll = /* @__PURE__ */ __name((fibers) => {
  const _fiberAll = {
    ...CommitPrototype2,
    commit() {
      return join2(this);
    },
    [FiberTypeId]: fiberVariance2,
    id: /* @__PURE__ */ __name(() => fromIterable(fibers).reduce((id, fiber) => combine3(id, fiber.id()), none4), "id"),
    await: exit(forEachParUnbounded(fibers, (fiber) => flatten4(fiber.await), false)),
    children: map8(forEachParUnbounded(fibers, (fiber) => fiber.children, false), flatten),
    inheritAll: forEachSequentialDiscard(fibers, (fiber) => fiber.inheritAll),
    poll: map8(forEachSequential(fibers, (fiber) => fiber.poll), reduceRight(some2(exitSucceed(new Array())), (optionB, optionA) => {
      switch (optionA._tag) {
        case "None": {
          return none2();
        }
        case "Some": {
          switch (optionB._tag) {
            case "None": {
              return none2();
            }
            case "Some": {
              return some2(exitZipWith(optionA.value, optionB.value, {
                onSuccess: /* @__PURE__ */ __name((a, chunk2) => [a, ...chunk2], "onSuccess"),
                onFailure: parallel
              }));
            }
          }
        }
      }
    })),
    interruptAsFork: /* @__PURE__ */ __name((fiberId3) => forEachSequentialDiscard(fibers, (fiber) => fiber.interruptAsFork(fiberId3)), "interruptAsFork")
  };
  return _fiberAll;
}, "fiberAll");
var raceWith = /* @__PURE__ */ dual(3, (self, other, options) => raceFibersWith(self, other, {
  onSelfWin: /* @__PURE__ */ __name((winner, loser) => flatMap7(winner.await, (exit4) => {
    switch (exit4._tag) {
      case OP_SUCCESS: {
        return flatMap7(winner.inheritAll, () => options.onSelfDone(exit4, loser));
      }
      case OP_FAILURE: {
        return options.onSelfDone(exit4, loser);
      }
    }
  }), "onSelfWin"),
  onOtherWin: /* @__PURE__ */ __name((winner, loser) => flatMap7(winner.await, (exit4) => {
    switch (exit4._tag) {
      case OP_SUCCESS: {
        return flatMap7(winner.inheritAll, () => options.onOtherDone(exit4, loser));
      }
      case OP_FAILURE: {
        return options.onOtherDone(exit4, loser);
      }
    }
  }), "onOtherWin")
}));
var disconnect = /* @__PURE__ */ __name((self) => uninterruptibleMask((restore) => fiberIdWith((fiberId3) => flatMap7(forkDaemon(restore(self)), (fiber) => pipe(restore(join2(fiber)), onInterrupt(() => pipe(fiber, interruptAsFork(fiberId3))))))), "disconnect");
var race = /* @__PURE__ */ dual(2, (self, that) => fiberIdWith((parentFiberId) => raceWith(self, that, {
  onSelfDone: /* @__PURE__ */ __name((exit4, right3) => exitMatchEffect(exit4, {
    onFailure: /* @__PURE__ */ __name((cause3) => pipe(join2(right3), mapErrorCause2((cause22) => parallel(cause3, cause22))), "onFailure"),
    onSuccess: /* @__PURE__ */ __name((value) => pipe(right3, interruptAsFiber(parentFiberId), as2(value)), "onSuccess")
  }), "onSelfDone"),
  onOtherDone: /* @__PURE__ */ __name((exit4, left3) => exitMatchEffect(exit4, {
    onFailure: /* @__PURE__ */ __name((cause3) => pipe(join2(left3), mapErrorCause2((cause22) => parallel(cause22, cause3))), "onFailure"),
    onSuccess: /* @__PURE__ */ __name((value) => pipe(left3, interruptAsFiber(parentFiberId), as2(value)), "onSuccess")
  }), "onOtherDone")
})));
var raceFibersWith = /* @__PURE__ */ dual(3, (self, other, options) => withFiberRuntime((parentFiber, parentStatus) => {
  const parentRuntimeFlags = parentStatus.runtimeFlags;
  const raceIndicator = make11(true);
  const leftFiber = unsafeMakeChildFiber(self, parentFiber, parentRuntimeFlags, options.selfScope);
  const rightFiber = unsafeMakeChildFiber(other, parentFiber, parentRuntimeFlags, options.otherScope);
  return async_((cb) => {
    leftFiber.addObserver(() => completeRace(leftFiber, rightFiber, options.onSelfWin, raceIndicator, cb));
    rightFiber.addObserver(() => completeRace(rightFiber, leftFiber, options.onOtherWin, raceIndicator, cb));
    leftFiber.startFork(self);
    rightFiber.startFork(other);
  }, combine3(leftFiber.id(), rightFiber.id()));
}));
var completeRace = /* @__PURE__ */ __name((winner, loser, cont, ab, cb) => {
  if (compareAndSet(true, false)(ab)) {
    cb(cont(winner, loser));
  }
}, "completeRace");
var ensuring = /* @__PURE__ */ dual(2, (self, finalizer) => uninterruptibleMask((restore) => matchCauseEffect(restore(self), {
  onFailure: /* @__PURE__ */ __name((cause1) => matchCauseEffect(finalizer, {
    onFailure: /* @__PURE__ */ __name((cause22) => failCause(sequential(cause1, cause22)), "onFailure"),
    onSuccess: /* @__PURE__ */ __name(() => failCause(cause1), "onSuccess")
  }), "onFailure"),
  onSuccess: /* @__PURE__ */ __name((a) => as2(finalizer, a), "onSuccess")
})));
var invokeWithInterrupt = /* @__PURE__ */ __name((self, entries2, onInterrupt3) => fiberIdWith((id) => flatMap7(flatMap7(forkDaemon(interruptible2(self)), (processing) => async_((cb) => {
  const counts = entries2.map((_) => _.listeners.count);
  const checkDone = /* @__PURE__ */ __name(() => {
    if (counts.every((count) => count === 0)) {
      if (entries2.every((_) => {
        if (_.result.state.current._tag === "Pending") {
          return true;
        } else if (_.result.state.current._tag === "Done" && exitIsExit(_.result.state.current.effect) && _.result.state.current.effect._tag === "Failure" && isInterrupted(_.result.state.current.effect.cause)) {
          return true;
        } else {
          return false;
        }
      })) {
        cleanup.forEach((f) => f());
        onInterrupt3?.();
        cb(interruptFiber(processing));
      }
    }
  }, "checkDone");
  processing.addObserver((exit4) => {
    cleanup.forEach((f) => f());
    cb(exit4);
  });
  const cleanup = entries2.map((r, i) => {
    const observer = /* @__PURE__ */ __name((count) => {
      counts[i] = count;
      checkDone();
    }, "observer");
    r.listeners.addObserver(observer);
    return () => r.listeners.removeObserver(observer);
  });
  checkDone();
  return sync(() => {
    cleanup.forEach((f) => f());
  });
})), () => suspend(() => {
  const residual = entries2.flatMap((entry) => {
    if (!entry.state.completed) {
      return [entry];
    }
    return [];
  });
  return forEachSequentialDiscard(residual, (entry) => complete(entry.request, exitInterrupt(id)));
}))), "invokeWithInterrupt");
var makeSpanScoped = /* @__PURE__ */ __name((name, options) => {
  options = addSpanStackTrace(options);
  return uninterruptible(withFiberRuntime((fiber) => {
    const scope3 = unsafeGet3(fiber.getFiberRef(currentContext), scopeTag);
    const span2 = unsafeMakeSpan(fiber, name, options);
    const timingEnabled = fiber.getFiberRef(currentTracerTimingEnabled);
    const clock_ = get3(fiber.getFiberRef(currentServices), clockTag);
    return as2(scopeAddFinalizerExit(scope3, (exit4) => endSpan(span2, exit4, clock_, timingEnabled)), span2);
  }));
}, "makeSpanScoped");
var withTracerScoped = /* @__PURE__ */ __name((value) => fiberRefLocallyScopedWith(currentServices, add2(tracerTag, value)), "withTracerScoped");
var withSpanScoped = /* @__PURE__ */ __name(function() {
  const dataFirst = typeof arguments[0] !== "string";
  const name = dataFirst ? arguments[1] : arguments[0];
  const options = addSpanStackTrace(dataFirst ? arguments[2] : arguments[1]);
  if (dataFirst) {
    const self = arguments[0];
    return flatMap7(makeSpanScoped(name, addSpanStackTrace(options)), (span2) => provideService(self, spanTag, span2));
  }
  return (self) => flatMap7(makeSpanScoped(name, addSpanStackTrace(options)), (span2) => provideService(self, spanTag, span2));
}, "withSpanScoped");

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/internal/cache.js
var complete2 = /* @__PURE__ */ __name((key, exit4, entryStats, timeToLiveMillis) => struct({
  _tag: "Complete",
  key,
  exit: exit4,
  entryStats,
  timeToLiveMillis
}), "complete");
var pending2 = /* @__PURE__ */ __name((key, deferred) => struct({
  _tag: "Pending",
  key,
  deferred
}), "pending");
var refreshing = /* @__PURE__ */ __name((deferred, complete3) => struct({
  _tag: "Refreshing",
  deferred,
  complete: complete3
}), "refreshing");
var MapKeyTypeId = /* @__PURE__ */ Symbol.for("effect/Cache/MapKey");
var MapKeyImpl = class {
  static {
    __name(this, "MapKeyImpl");
  }
  current;
  [MapKeyTypeId] = MapKeyTypeId;
  previous = void 0;
  next = void 0;
  constructor(current) {
    this.current = current;
  }
  [symbol]() {
    return pipe(hash(this.current), combine(hash(this.previous)), combine(hash(this.next)), cached(this));
  }
  [symbol2](that) {
    if (this === that) {
      return true;
    }
    return isMapKey(that) && equals(this.current, that.current) && equals(this.previous, that.previous) && equals(this.next, that.next);
  }
};
var makeMapKey = /* @__PURE__ */ __name((current) => new MapKeyImpl(current), "makeMapKey");
var isMapKey = /* @__PURE__ */ __name((u) => hasProperty(u, MapKeyTypeId), "isMapKey");
var KeySetImpl = class {
  static {
    __name(this, "KeySetImpl");
  }
  head = void 0;
  tail = void 0;
  add(key) {
    if (key !== this.tail) {
      if (this.tail === void 0) {
        this.head = key;
        this.tail = key;
      } else {
        const previous = key.previous;
        const next = key.next;
        if (next !== void 0) {
          key.next = void 0;
          if (previous !== void 0) {
            previous.next = next;
            next.previous = previous;
          } else {
            this.head = next;
            this.head.previous = void 0;
          }
        }
        this.tail.next = key;
        key.previous = this.tail;
        this.tail = key;
      }
    }
  }
  remove() {
    const key = this.head;
    if (key !== void 0) {
      const next = key.next;
      if (next !== void 0) {
        key.next = void 0;
        this.head = next;
        this.head.previous = void 0;
      } else {
        this.head = void 0;
        this.tail = void 0;
      }
    }
    return key;
  }
};
var makeKeySet = /* @__PURE__ */ __name(() => new KeySetImpl(), "makeKeySet");
var makeCacheState = /* @__PURE__ */ __name((map14, keys5, accesses, updating, hits, misses) => ({
  map: map14,
  keys: keys5,
  accesses,
  updating,
  hits,
  misses
}), "makeCacheState");
var initialCacheState = /* @__PURE__ */ __name(() => makeCacheState(empty17(), makeKeySet(), unbounded(), make11(false), 0, 0), "initialCacheState");
var CacheSymbolKey = "effect/Cache";
var CacheTypeId = /* @__PURE__ */ Symbol.for(CacheSymbolKey);
var cacheVariance = {
  /* c8 ignore next */
  _Key: /* @__PURE__ */ __name((_) => _, "_Key"),
  /* c8 ignore next */
  _Error: /* @__PURE__ */ __name((_) => _, "_Error"),
  /* c8 ignore next */
  _Value: /* @__PURE__ */ __name((_) => _, "_Value")
};
var ConsumerCacheSymbolKey = "effect/ConsumerCache";
var ConsumerCacheTypeId = /* @__PURE__ */ Symbol.for(ConsumerCacheSymbolKey);
var consumerCacheVariance = {
  /* c8 ignore next */
  _Key: /* @__PURE__ */ __name((_) => _, "_Key"),
  /* c8 ignore next */
  _Error: /* @__PURE__ */ __name((_) => _, "_Error"),
  /* c8 ignore next */
  _Value: /* @__PURE__ */ __name((_) => _, "_Value")
};
var makeCacheStats = /* @__PURE__ */ __name((options) => options, "makeCacheStats");
var makeEntryStats = /* @__PURE__ */ __name((loadedMillis) => ({
  loadedMillis
}), "makeEntryStats");
var CacheImpl = class {
  static {
    __name(this, "CacheImpl");
  }
  capacity;
  context;
  fiberId;
  lookup;
  timeToLive;
  [CacheTypeId] = cacheVariance;
  [ConsumerCacheTypeId] = consumerCacheVariance;
  cacheState;
  constructor(capacity, context4, fiberId3, lookup, timeToLive) {
    this.capacity = capacity;
    this.context = context4;
    this.fiberId = fiberId3;
    this.lookup = lookup;
    this.timeToLive = timeToLive;
    this.cacheState = initialCacheState();
  }
  get(key) {
    return map8(this.getEither(key), merge);
  }
  get cacheStats() {
    return sync(() => makeCacheStats({
      hits: this.cacheState.hits,
      misses: this.cacheState.misses,
      size: size5(this.cacheState.map)
    }));
  }
  getOption(key) {
    return suspend(() => match2(get8(this.cacheState.map, key), {
      onNone: /* @__PURE__ */ __name(() => {
        const mapKey = makeMapKey(key);
        this.trackAccess(mapKey);
        this.trackMiss();
        return succeed(none2());
      }, "onNone"),
      onSome: /* @__PURE__ */ __name((value) => this.resolveMapValue(value), "onSome")
    }));
  }
  getOptionComplete(key) {
    return suspend(() => match2(get8(this.cacheState.map, key), {
      onNone: /* @__PURE__ */ __name(() => {
        const mapKey = makeMapKey(key);
        this.trackAccess(mapKey);
        this.trackMiss();
        return succeed(none2());
      }, "onNone"),
      onSome: /* @__PURE__ */ __name((value) => this.resolveMapValue(value, true), "onSome")
    }));
  }
  contains(key) {
    return sync(() => has4(this.cacheState.map, key));
  }
  entryStats(key) {
    return sync(() => {
      const option3 = get8(this.cacheState.map, key);
      if (isSome2(option3)) {
        switch (option3.value._tag) {
          case "Complete": {
            const loaded = option3.value.entryStats.loadedMillis;
            return some2(makeEntryStats(loaded));
          }
          case "Pending": {
            return none2();
          }
          case "Refreshing": {
            const loaded = option3.value.complete.entryStats.loadedMillis;
            return some2(makeEntryStats(loaded));
          }
        }
      }
      return none2();
    });
  }
  getEither(key) {
    return suspend(() => {
      const k = key;
      let mapKey = void 0;
      let deferred = void 0;
      let value = getOrUndefined(get8(this.cacheState.map, k));
      if (value === void 0) {
        deferred = unsafeMake3(this.fiberId);
        mapKey = makeMapKey(k);
        if (has4(this.cacheState.map, k)) {
          value = getOrUndefined(get8(this.cacheState.map, k));
        } else {
          set4(this.cacheState.map, k, pending2(mapKey, deferred));
        }
      }
      if (value === void 0) {
        this.trackAccess(mapKey);
        this.trackMiss();
        return map8(this.lookupValueOf(key, deferred), right2);
      } else {
        return flatMap7(this.resolveMapValue(value), match2({
          onNone: /* @__PURE__ */ __name(() => this.getEither(key), "onNone"),
          onSome: /* @__PURE__ */ __name((value2) => succeed(left2(value2)), "onSome")
        }));
      }
    });
  }
  invalidate(key) {
    return sync(() => {
      remove5(this.cacheState.map, key);
    });
  }
  invalidateWhen(key, when3) {
    return sync(() => {
      const value = get8(this.cacheState.map, key);
      if (isSome2(value) && value.value._tag === "Complete") {
        if (value.value.exit._tag === "Success") {
          if (when3(value.value.exit.value)) {
            remove5(this.cacheState.map, key);
          }
        }
      }
    });
  }
  get invalidateAll() {
    return sync(() => {
      this.cacheState.map = empty17();
    });
  }
  refresh(key) {
    return clockWith3((clock3) => suspend(() => {
      const k = key;
      const deferred = unsafeMake3(this.fiberId);
      let value = getOrUndefined(get8(this.cacheState.map, k));
      if (value === void 0) {
        if (has4(this.cacheState.map, k)) {
          value = getOrUndefined(get8(this.cacheState.map, k));
        } else {
          set4(this.cacheState.map, k, pending2(makeMapKey(k), deferred));
        }
      }
      if (value === void 0) {
        return asVoid(this.lookupValueOf(key, deferred));
      } else {
        switch (value._tag) {
          case "Complete": {
            if (this.hasExpired(clock3, value.timeToLiveMillis)) {
              const found = getOrUndefined(get8(this.cacheState.map, k));
              if (equals(found, value)) {
                remove5(this.cacheState.map, k);
              }
              return asVoid(this.get(key));
            }
            return pipe(this.lookupValueOf(key, deferred), when(() => {
              const current = getOrUndefined(get8(this.cacheState.map, k));
              if (equals(current, value)) {
                const mapValue = refreshing(deferred, value);
                set4(this.cacheState.map, k, mapValue);
                return true;
              }
              return false;
            }), asVoid);
          }
          case "Pending": {
            return _await(value.deferred);
          }
          case "Refreshing": {
            return _await(value.deferred);
          }
        }
      }
    }));
  }
  set(key, value) {
    return clockWith3((clock3) => sync(() => {
      const now = clock3.unsafeCurrentTimeMillis();
      const k = key;
      const lookupResult = succeed2(value);
      const mapValue = complete2(makeMapKey(k), lookupResult, makeEntryStats(now), now + toMillis(decode(this.timeToLive(lookupResult))));
      set4(this.cacheState.map, k, mapValue);
    }));
  }
  get size() {
    return sync(() => {
      return size5(this.cacheState.map);
    });
  }
  get values() {
    return sync(() => {
      const values3 = [];
      for (const entry of this.cacheState.map) {
        if (entry[1]._tag === "Complete" && entry[1].exit._tag === "Success") {
          values3.push(entry[1].exit.value);
        }
      }
      return values3;
    });
  }
  get entries() {
    return sync(() => {
      const values3 = [];
      for (const entry of this.cacheState.map) {
        if (entry[1]._tag === "Complete" && entry[1].exit._tag === "Success") {
          values3.push([entry[0], entry[1].exit.value]);
        }
      }
      return values3;
    });
  }
  get keys() {
    return sync(() => {
      const keys5 = [];
      for (const entry of this.cacheState.map) {
        if (entry[1]._tag === "Complete" && entry[1].exit._tag === "Success") {
          keys5.push(entry[0]);
        }
      }
      return keys5;
    });
  }
  resolveMapValue(value, ignorePending = false) {
    return clockWith3((clock3) => {
      switch (value._tag) {
        case "Complete": {
          this.trackAccess(value.key);
          if (this.hasExpired(clock3, value.timeToLiveMillis)) {
            remove5(this.cacheState.map, value.key.current);
            return succeed(none2());
          }
          this.trackHit();
          return map8(value.exit, some2);
        }
        case "Pending": {
          this.trackAccess(value.key);
          this.trackHit();
          if (ignorePending) {
            return succeed(none2());
          }
          return map8(_await(value.deferred), some2);
        }
        case "Refreshing": {
          this.trackAccess(value.complete.key);
          this.trackHit();
          if (this.hasExpired(clock3, value.complete.timeToLiveMillis)) {
            if (ignorePending) {
              return succeed(none2());
            }
            return map8(_await(value.deferred), some2);
          }
          return map8(value.complete.exit, some2);
        }
      }
    });
  }
  trackHit() {
    this.cacheState.hits = this.cacheState.hits + 1;
  }
  trackMiss() {
    this.cacheState.misses = this.cacheState.misses + 1;
  }
  trackAccess(key) {
    offer(this.cacheState.accesses, key);
    if (compareAndSet(this.cacheState.updating, false, true)) {
      let loop3 = true;
      while (loop3) {
        const key2 = poll(this.cacheState.accesses, EmptyMutableQueue);
        if (key2 === EmptyMutableQueue) {
          loop3 = false;
        } else {
          this.cacheState.keys.add(key2);
        }
      }
      let size11 = size5(this.cacheState.map);
      loop3 = size11 > this.capacity;
      while (loop3) {
        const key2 = this.cacheState.keys.remove();
        if (key2 !== void 0) {
          if (has4(this.cacheState.map, key2.current)) {
            remove5(this.cacheState.map, key2.current);
            size11 = size11 - 1;
            loop3 = size11 > this.capacity;
          }
        } else {
          loop3 = false;
        }
      }
      set2(this.cacheState.updating, false);
    }
  }
  hasExpired(clock3, timeToLiveMillis) {
    return clock3.unsafeCurrentTimeMillis() > timeToLiveMillis;
  }
  lookupValueOf(input, deferred) {
    return clockWith3((clock3) => suspend(() => {
      const key = input;
      return pipe(this.lookup(input), provideContext(this.context), exit, flatMap7((exit4) => {
        const now = clock3.unsafeCurrentTimeMillis();
        const stats = makeEntryStats(now);
        const value = complete2(makeMapKey(key), exit4, stats, now + toMillis(decode(this.timeToLive(exit4))));
        set4(this.cacheState.map, key, value);
        return zipRight(done2(deferred, exit4), exit4);
      }), onInterrupt(() => zipRight(interrupt3(deferred), sync(() => {
        remove5(this.cacheState.map, key);
      }))));
    }));
  }
};
var unsafeMakeWith = /* @__PURE__ */ __name((capacity, lookup, timeToLive) => new CacheImpl(capacity, empty3(), none3, lookup, (exit4) => decode(timeToLive(exit4))), "unsafeMakeWith");

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/Cause.js
var Cause_exports = {};
__export(Cause_exports, {
  CauseTypeId: () => CauseTypeId2,
  ExceededCapacityException: () => ExceededCapacityException2,
  ExceededCapacityExceptionTypeId: () => ExceededCapacityExceptionTypeId2,
  IllegalArgumentException: () => IllegalArgumentException2,
  IllegalArgumentExceptionTypeId: () => IllegalArgumentExceptionTypeId2,
  InterruptedException: () => InterruptedException2,
  InterruptedExceptionTypeId: () => InterruptedExceptionTypeId2,
  InvalidPubSubCapacityExceptionTypeId: () => InvalidPubSubCapacityExceptionTypeId2,
  NoSuchElementException: () => NoSuchElementException2,
  NoSuchElementExceptionTypeId: () => NoSuchElementExceptionTypeId2,
  RuntimeException: () => RuntimeException2,
  RuntimeExceptionTypeId: () => RuntimeExceptionTypeId2,
  TimeoutException: () => TimeoutException2,
  TimeoutExceptionTypeId: () => TimeoutExceptionTypeId2,
  UnknownException: () => UnknownException2,
  UnknownExceptionTypeId: () => UnknownExceptionTypeId2,
  YieldableError: () => YieldableError2,
  andThen: () => andThen4,
  as: () => as5,
  contains: () => contains4,
  defects: () => defects2,
  die: () => die4,
  dieOption: () => dieOption2,
  empty: () => empty26,
  fail: () => fail4,
  failureOption: () => failureOption2,
  failureOrCause: () => failureOrCause2,
  failures: () => failures2,
  filter: () => filter6,
  find: () => find2,
  flatMap: () => flatMap10,
  flatten: () => flatten6,
  flipCauseOption: () => flipCauseOption2,
  interrupt: () => interrupt5,
  interruptOption: () => interruptOption2,
  interruptors: () => interruptors2,
  isCause: () => isCause2,
  isDie: () => isDie2,
  isDieType: () => isDieType2,
  isEmpty: () => isEmpty7,
  isEmptyType: () => isEmptyType2,
  isExceededCapacityException: () => isExceededCapacityException2,
  isFailType: () => isFailType2,
  isFailure: () => isFailure4,
  isIllegalArgumentException: () => isIllegalArgumentException2,
  isInterruptType: () => isInterruptType2,
  isInterrupted: () => isInterrupted3,
  isInterruptedException: () => isInterruptedException2,
  isInterruptedOnly: () => isInterruptedOnly2,
  isNoSuchElementException: () => isNoSuchElementException2,
  isParallelType: () => isParallelType2,
  isRuntimeException: () => isRuntimeException2,
  isSequentialType: () => isSequentialType2,
  isTimeoutException: () => isTimeoutException2,
  isUnknownException: () => isUnknownException2,
  keepDefects: () => keepDefects2,
  linearize: () => linearize2,
  map: () => map11,
  match: () => match10,
  originalError: () => originalError,
  parallel: () => parallel4,
  pretty: () => pretty2,
  prettyErrors: () => prettyErrors2,
  reduce: () => reduce10,
  reduceWithContext: () => reduceWithContext2,
  sequential: () => sequential4,
  size: () => size8,
  squash: () => squash,
  squashWith: () => squashWith,
  stripFailures: () => stripFailures2,
  stripSomeDefects: () => stripSomeDefects2
});
var CauseTypeId2 = CauseTypeId;
var RuntimeExceptionTypeId2 = RuntimeExceptionTypeId;
var InterruptedExceptionTypeId2 = InterruptedExceptionTypeId;
var IllegalArgumentExceptionTypeId2 = IllegalArgumentExceptionTypeId;
var NoSuchElementExceptionTypeId2 = NoSuchElementExceptionTypeId;
var InvalidPubSubCapacityExceptionTypeId2 = InvalidPubSubCapacityExceptionTypeId;
var ExceededCapacityExceptionTypeId2 = ExceededCapacityExceptionTypeId;
var TimeoutExceptionTypeId2 = TimeoutExceptionTypeId;
var UnknownExceptionTypeId2 = UnknownExceptionTypeId;
var YieldableError2 = YieldableError;
var empty26 = empty16;
var fail4 = fail;
var die4 = die;
var interrupt5 = interrupt;
var parallel4 = parallel;
var sequential4 = sequential;
var isCause2 = isCause;
var isEmptyType2 = isEmptyType;
var isFailType2 = isFailType;
var isDieType2 = isDieType;
var isInterruptType2 = isInterruptType;
var isSequentialType2 = isSequentialType;
var isParallelType2 = isParallelType;
var size8 = size4;
var isEmpty7 = isEmpty5;
var isFailure4 = isFailure;
var isDie2 = isDie;
var isInterrupted3 = isInterrupted;
var isInterruptedOnly2 = isInterruptedOnly;
var failures2 = failures;
var defects2 = defects;
var interruptors2 = interruptors;
var failureOption2 = failureOption;
var failureOrCause2 = failureOrCause;
var flipCauseOption2 = flipCauseOption;
var dieOption2 = dieOption;
var interruptOption2 = interruptOption;
var keepDefects2 = keepDefects;
var linearize2 = linearize;
var stripFailures2 = stripFailures;
var stripSomeDefects2 = stripSomeDefects;
var as5 = as;
var map11 = map7;
var flatMap10 = flatMap6;
var andThen4 = andThen2;
var flatten6 = flatten3;
var contains4 = contains3;
var squash = causeSquash;
var squashWith = causeSquashWith;
var find2 = find;
var filter6 = filter4;
var match10 = match4;
var reduce10 = reduce7;
var reduceWithContext2 = reduceWithContext;
var InterruptedException2 = InterruptedException;
var isInterruptedException2 = isInterruptedException;
var IllegalArgumentException2 = IllegalArgumentException;
var isIllegalArgumentException2 = isIllegalArgumentException;
var NoSuchElementException2 = NoSuchElementException;
var isNoSuchElementException2 = isNoSuchElementException;
var RuntimeException2 = RuntimeException;
var isRuntimeException2 = isRuntimeException;
var TimeoutException2 = TimeoutException;
var isTimeoutException2 = isTimeoutException;
var UnknownException2 = UnknownException;
var isUnknownException2 = isUnknownException;
var ExceededCapacityException2 = ExceededCapacityException;
var isExceededCapacityException2 = isExceededCapacityException;
var pretty2 = pretty;
var prettyErrors2 = prettyErrors;
var originalError = originalInstance;

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/Effect.js
var Effect_exports = {};
__export(Effect_exports, {
  Do: () => Do2,
  EffectTypeId: () => EffectTypeId3,
  Service: () => Service,
  Tag: () => Tag2,
  acquireRelease: () => acquireRelease2,
  acquireReleaseInterruptible: () => acquireReleaseInterruptible2,
  acquireUseRelease: () => acquireUseRelease2,
  addFinalizer: () => addFinalizer2,
  all: () => all4,
  allSuccesses: () => allSuccesses2,
  allWith: () => allWith2,
  allowInterrupt: () => allowInterrupt2,
  andThen: () => andThen5,
  annotateCurrentSpan: () => annotateCurrentSpan2,
  annotateLogs: () => annotateLogs2,
  annotateLogsScoped: () => annotateLogsScoped2,
  annotateSpans: () => annotateSpans2,
  ap: () => ap,
  as: () => as6,
  asSome: () => asSome2,
  asSomeError: () => asSomeError2,
  asVoid: () => asVoid4,
  async: () => async2,
  asyncEffect: () => asyncEffect2,
  awaitAllChildren: () => awaitAllChildren2,
  bind: () => bind3,
  bindAll: () => bindAll2,
  bindTo: () => bindTo3,
  blocked: () => blocked2,
  cacheRequestResult: () => cacheRequestResult,
  cached: () => cached3,
  cachedFunction: () => cachedFunction2,
  cachedInvalidateWithTTL: () => cachedInvalidateWithTTL2,
  cachedWithTTL: () => cachedWithTTL,
  catch: () => _catch2,
  catchAll: () => catchAll2,
  catchAllCause: () => catchAllCause2,
  catchAllDefect: () => catchAllDefect2,
  catchIf: () => catchIf2,
  catchSome: () => catchSome2,
  catchSomeCause: () => catchSomeCause2,
  catchSomeDefect: () => catchSomeDefect2,
  catchTag: () => catchTag2,
  catchTags: () => catchTags2,
  cause: () => cause2,
  checkInterruptible: () => checkInterruptible2,
  clock: () => clock2,
  clockWith: () => clockWith4,
  configProviderWith: () => configProviderWith2,
  console: () => console3,
  consoleWith: () => consoleWith2,
  context: () => context3,
  contextWith: () => contextWith2,
  contextWithEffect: () => contextWithEffect2,
  currentParentSpan: () => currentParentSpan2,
  currentPropagatedSpan: () => currentPropagatedSpan2,
  currentSpan: () => currentSpan2,
  custom: () => custom2,
  daemonChildren: () => daemonChildren2,
  delay: () => delay2,
  descriptor: () => descriptor2,
  descriptorWith: () => descriptorWith2,
  die: () => die5,
  dieMessage: () => dieMessage2,
  dieSync: () => dieSync2,
  diffFiberRefs: () => diffFiberRefs2,
  disconnect: () => disconnect2,
  dropUntil: () => dropUntil2,
  dropWhile: () => dropWhile2,
  either: () => either3,
  ensureErrorType: () => ensureErrorType,
  ensureRequirementsType: () => ensureRequirementsType,
  ensureSuccessType: () => ensureSuccessType,
  ensuring: () => ensuring2,
  ensuringChild: () => ensuringChild2,
  ensuringChildren: () => ensuringChildren2,
  eventually: () => eventually2,
  every: () => every5,
  exists: () => exists3,
  exit: () => exit3,
  fail: () => fail6,
  failCause: () => failCause5,
  failCauseSync: () => failCauseSync2,
  failSync: () => failSync2,
  fiberId: () => fiberId2,
  fiberIdWith: () => fiberIdWith2,
  filter: () => filter7,
  filterEffectOrElse: () => filterEffectOrElse2,
  filterEffectOrFail: () => filterEffectOrFail2,
  filterMap: () => filterMap4,
  filterOrDie: () => filterOrDie2,
  filterOrDieMessage: () => filterOrDieMessage2,
  filterOrElse: () => filterOrElse2,
  filterOrFail: () => filterOrFail2,
  finalizersMask: () => finalizersMask2,
  findFirst: () => findFirst5,
  firstSuccessOf: () => firstSuccessOf2,
  flatMap: () => flatMap11,
  flatten: () => flatten7,
  flip: () => flip2,
  flipWith: () => flipWith2,
  fn: () => fn,
  fnUntraced: () => fnUntraced2,
  forEach: () => forEach8,
  forever: () => forever3,
  fork: () => fork3,
  forkAll: () => forkAll2,
  forkDaemon: () => forkDaemon2,
  forkIn: () => forkIn2,
  forkScoped: () => forkScoped2,
  forkWithErrorHandler: () => forkWithErrorHandler2,
  fromFiber: () => fromFiber2,
  fromFiberEffect: () => fromFiberEffect2,
  fromNullable: () => fromNullable3,
  functionWithSpan: () => functionWithSpan2,
  gen: () => gen2,
  getFiberRefs: () => getFiberRefs,
  getRuntimeFlags: () => getRuntimeFlags,
  head: () => head4,
  if: () => if_2,
  ignore: () => ignore2,
  ignoreLogged: () => ignoreLogged2,
  inheritFiberRefs: () => inheritFiberRefs2,
  interrupt: () => interrupt6,
  interruptWith: () => interruptWith2,
  interruptible: () => interruptible4,
  interruptibleMask: () => interruptibleMask2,
  intoDeferred: () => intoDeferred2,
  isEffect: () => isEffect2,
  isFailure: () => isFailure5,
  isSuccess: () => isSuccess3,
  iterate: () => iterate2,
  labelMetrics: () => labelMetrics2,
  labelMetricsScoped: () => labelMetricsScoped2,
  let: () => let_3,
  liftPredicate: () => liftPredicate2,
  linkSpanCurrent: () => linkSpanCurrent2,
  linkSpans: () => linkSpans2,
  locally: () => locally,
  locallyScoped: () => locallyScoped,
  locallyScopedWith: () => locallyScopedWith,
  locallyWith: () => locallyWith,
  log: () => log2,
  logAnnotations: () => logAnnotations2,
  logDebug: () => logDebug2,
  logError: () => logError2,
  logFatal: () => logFatal2,
  logInfo: () => logInfo2,
  logTrace: () => logTrace2,
  logWarning: () => logWarning2,
  logWithLevel: () => logWithLevel2,
  loop: () => loop2,
  makeLatch: () => makeLatch2,
  makeSemaphore: () => makeSemaphore2,
  makeSpan: () => makeSpan2,
  makeSpanScoped: () => makeSpanScoped2,
  map: () => map13,
  mapAccum: () => mapAccum3,
  mapBoth: () => mapBoth3,
  mapError: () => mapError3,
  mapErrorCause: () => mapErrorCause3,
  mapInputContext: () => mapInputContext2,
  match: () => match11,
  matchCause: () => matchCause3,
  matchCauseEffect: () => matchCauseEffect3,
  matchEffect: () => matchEffect3,
  merge: () => merge6,
  mergeAll: () => mergeAll5,
  metricLabels: () => metricLabels2,
  negate: () => negate2,
  never: () => never2,
  none: () => none9,
  onError: () => onError2,
  onExit: () => onExit3,
  onInterrupt: () => onInterrupt2,
  once: () => once3,
  option: () => option2,
  optionFromOptional: () => optionFromOptional2,
  orDie: () => orDie2,
  orDieWith: () => orDieWith2,
  orElse: () => orElse2,
  orElseFail: () => orElseFail2,
  orElseSucceed: () => orElseSucceed2,
  parallelErrors: () => parallelErrors2,
  parallelFinalizers: () => parallelFinalizers2,
  partition: () => partition4,
  patchFiberRefs: () => patchFiberRefs2,
  patchRuntimeFlags: () => patchRuntimeFlags,
  promise: () => promise2,
  provide: () => provide2,
  provideService: () => provideService2,
  provideServiceEffect: () => provideServiceEffect2,
  race: () => race2,
  raceAll: () => raceAll2,
  raceFirst: () => raceFirst2,
  raceWith: () => raceWith2,
  random: () => random3,
  randomWith: () => randomWith2,
  reduce: () => reduce11,
  reduceEffect: () => reduceEffect2,
  reduceRight: () => reduceRight3,
  reduceWhile: () => reduceWhile2,
  repeat: () => repeat,
  repeatN: () => repeatN2,
  repeatOrElse: () => repeatOrElse,
  replicate: () => replicate2,
  replicateEffect: () => replicateEffect2,
  request: () => request,
  retry: () => retry,
  retryOrElse: () => retryOrElse,
  runCallback: () => runCallback,
  runFork: () => runFork2,
  runPromise: () => runPromise,
  runPromiseExit: () => runPromiseExit,
  runRequestBlock: () => runRequestBlock2,
  runSync: () => runSync,
  runSyncExit: () => runSyncExit,
  runtime: () => runtime3,
  sandbox: () => sandbox2,
  schedule: () => schedule,
  scheduleForked: () => scheduleForked2,
  scheduleFrom: () => scheduleFrom,
  scope: () => scope2,
  scopeWith: () => scopeWith2,
  scoped: () => scoped2,
  scopedWith: () => scopedWith2,
  sequentialFinalizers: () => sequentialFinalizers2,
  serviceConstants: () => serviceConstants2,
  serviceFunction: () => serviceFunction2,
  serviceFunctionEffect: () => serviceFunctionEffect2,
  serviceFunctions: () => serviceFunctions2,
  serviceMembers: () => serviceMembers2,
  serviceOption: () => serviceOption2,
  serviceOptional: () => serviceOptional2,
  setFiberRefs: () => setFiberRefs2,
  sleep: () => sleep4,
  spanAnnotations: () => spanAnnotations2,
  spanLinks: () => spanLinks2,
  step: () => step3,
  succeed: () => succeed6,
  succeedNone: () => succeedNone2,
  succeedSome: () => succeedSome2,
  summarized: () => summarized2,
  supervised: () => supervised2,
  suspend: () => suspend4,
  sync: () => sync4,
  tagMetrics: () => tagMetrics2,
  tagMetricsScoped: () => tagMetricsScoped2,
  takeUntil: () => takeUntil2,
  takeWhile: () => takeWhile2,
  tap: () => tap2,
  tapBoth: () => tapBoth2,
  tapDefect: () => tapDefect2,
  tapError: () => tapError2,
  tapErrorCause: () => tapErrorCause2,
  tapErrorTag: () => tapErrorTag2,
  timed: () => timed2,
  timedWith: () => timedWith2,
  timeout: () => timeout2,
  timeoutFail: () => timeoutFail2,
  timeoutFailCause: () => timeoutFailCause2,
  timeoutOption: () => timeoutOption2,
  timeoutTo: () => timeoutTo2,
  tracer: () => tracer2,
  tracerWith: () => tracerWith4,
  transplant: () => transplant2,
  transposeMapOption: () => transposeMapOption,
  transposeOption: () => transposeOption,
  try: () => try_2,
  tryMap: () => tryMap2,
  tryMapPromise: () => tryMapPromise2,
  tryPromise: () => tryPromise2,
  uninterruptible: () => uninterruptible2,
  uninterruptibleMask: () => uninterruptibleMask3,
  unless: () => unless2,
  unlessEffect: () => unlessEffect2,
  unsafeMakeLatch: () => unsafeMakeLatch2,
  unsafeMakeSemaphore: () => unsafeMakeSemaphore2,
  unsandbox: () => unsandbox2,
  updateFiberRefs: () => updateFiberRefs2,
  updateService: () => updateService2,
  useSpan: () => useSpan2,
  using: () => using2,
  validate: () => validate2,
  validateAll: () => validateAll2,
  validateFirst: () => validateFirst2,
  validateWith: () => validateWith2,
  void: () => _void,
  when: () => when2,
  whenEffect: () => whenEffect2,
  whenFiberRef: () => whenFiberRef2,
  whenLogLevel: () => whenLogLevel2,
  whenRef: () => whenRef2,
  whileLoop: () => whileLoop3,
  withClock: () => withClock2,
  withClockScoped: () => withClockScoped2,
  withConcurrency: () => withConcurrency2,
  withConfigProvider: () => withConfigProvider2,
  withConfigProviderScoped: () => withConfigProviderScoped2,
  withConsole: () => withConsole2,
  withConsoleScoped: () => withConsoleScoped2,
  withEarlyRelease: () => withEarlyRelease2,
  withExecutionPlan: () => withExecutionPlan2,
  withFiberRuntime: () => withFiberRuntime2,
  withLogSpan: () => withLogSpan2,
  withMaxOpsBeforeYield: () => withMaxOpsBeforeYield2,
  withMetric: () => withMetric2,
  withParentSpan: () => withParentSpan2,
  withRandom: () => withRandom2,
  withRandomFixed: () => withRandomFixed,
  withRandomScoped: () => withRandomScoped2,
  withRequestBatching: () => withRequestBatching2,
  withRequestCache: () => withRequestCache2,
  withRequestCaching: () => withRequestCaching2,
  withRuntimeFlagsPatch: () => withRuntimeFlagsPatch,
  withRuntimeFlagsPatchScoped: () => withRuntimeFlagsPatchScoped,
  withScheduler: () => withScheduler2,
  withSchedulingPriority: () => withSchedulingPriority2,
  withSpan: () => withSpan2,
  withSpanScoped: () => withSpanScoped2,
  withTracer: () => withTracer2,
  withTracerEnabled: () => withTracerEnabled2,
  withTracerScoped: () => withTracerScoped2,
  withTracerTiming: () => withTracerTiming2,
  withUnhandledErrorLogLevel: () => withUnhandledErrorLogLevel2,
  yieldNow: () => yieldNow4,
  zip: () => zip5,
  zipLeft: () => zipLeft3,
  zipRight: () => zipRight3,
  zipWith: () => zipWith4
});

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/internal/schedule/interval.js
var IntervalSymbolKey = "effect/ScheduleInterval";
var IntervalTypeId = /* @__PURE__ */ Symbol.for(IntervalSymbolKey);
var empty27 = {
  [IntervalTypeId]: IntervalTypeId,
  startMillis: 0,
  endMillis: 0
};
var make34 = /* @__PURE__ */ __name((startMillis, endMillis) => {
  if (startMillis > endMillis) {
    return empty27;
  }
  return {
    [IntervalTypeId]: IntervalTypeId,
    startMillis,
    endMillis
  };
}, "make");
var lessThan3 = /* @__PURE__ */ dual(2, (self, that) => min3(self, that) === self);
var min3 = /* @__PURE__ */ dual(2, (self, that) => {
  if (self.endMillis <= that.startMillis) return self;
  if (that.endMillis <= self.startMillis) return that;
  if (self.startMillis < that.startMillis) return self;
  if (that.startMillis < self.startMillis) return that;
  if (self.endMillis <= that.endMillis) return self;
  return that;
});
var isEmpty8 = /* @__PURE__ */ __name((self) => {
  return self.startMillis >= self.endMillis;
}, "isEmpty");
var intersect = /* @__PURE__ */ dual(2, (self, that) => {
  const start3 = Math.max(self.startMillis, that.startMillis);
  const end3 = Math.min(self.endMillis, that.endMillis);
  return make34(start3, end3);
});
var after = /* @__PURE__ */ __name((startMilliseconds) => {
  return make34(startMilliseconds, Number.POSITIVE_INFINITY);
}, "after");

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/ScheduleInterval.js
var empty28 = empty27;
var lessThan4 = lessThan3;
var isEmpty9 = isEmpty8;
var intersect2 = intersect;
var after2 = after;

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/internal/schedule/intervals.js
var IntervalsSymbolKey = "effect/ScheduleIntervals";
var IntervalsTypeId = /* @__PURE__ */ Symbol.for(IntervalsSymbolKey);
var make36 = /* @__PURE__ */ __name((intervals) => {
  return {
    [IntervalsTypeId]: IntervalsTypeId,
    intervals
  };
}, "make");
var intersect3 = /* @__PURE__ */ dual(2, (self, that) => intersectLoop(self.intervals, that.intervals, empty4()));
var intersectLoop = /* @__PURE__ */ __name((_left, _right, _acc) => {
  let left3 = _left;
  let right3 = _right;
  let acc = _acc;
  while (isNonEmpty(left3) && isNonEmpty(right3)) {
    const interval = pipe(headNonEmpty2(left3), intersect2(headNonEmpty2(right3)));
    const intervals = isEmpty9(interval) ? acc : pipe(acc, prepend2(interval));
    if (pipe(headNonEmpty2(left3), lessThan4(headNonEmpty2(right3)))) {
      left3 = tailNonEmpty2(left3);
    } else {
      right3 = tailNonEmpty2(right3);
    }
    acc = intervals;
  }
  return make36(reverse2(acc));
}, "intersectLoop");
var start = /* @__PURE__ */ __name((self) => {
  return pipe(self.intervals, head2, getOrElse(() => empty28)).startMillis;
}, "start");
var end = /* @__PURE__ */ __name((self) => {
  return pipe(self.intervals, head2, getOrElse(() => empty28)).endMillis;
}, "end");
var lessThan5 = /* @__PURE__ */ dual(2, (self, that) => start(self) < start(that));
var isNonEmpty3 = /* @__PURE__ */ __name((self) => {
  return isNonEmpty(self.intervals);
}, "isNonEmpty");

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/ScheduleIntervals.js
var make37 = make36;
var intersect4 = intersect3;
var start2 = start;
var end2 = end;
var lessThan6 = lessThan5;
var isNonEmpty4 = isNonEmpty3;

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/internal/schedule/decision.js
var OP_CONTINUE = "Continue";
var OP_DONE2 = "Done";
var _continue = /* @__PURE__ */ __name((intervals) => {
  return {
    _tag: OP_CONTINUE,
    intervals
  };
}, "_continue");
var continueWith = /* @__PURE__ */ __name((interval) => {
  return {
    _tag: OP_CONTINUE,
    intervals: make37(of2(interval))
  };
}, "continueWith");
var done5 = {
  _tag: OP_DONE2
};
var isContinue = /* @__PURE__ */ __name((self) => {
  return self._tag === OP_CONTINUE;
}, "isContinue");
var isDone3 = /* @__PURE__ */ __name((self) => {
  return self._tag === OP_DONE2;
}, "isDone");

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/ScheduleDecision.js
var _continue2 = _continue;
var continueWith2 = continueWith;
var done6 = done5;
var isContinue2 = isContinue;
var isDone4 = isDone3;

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/Scope.js
var close = scopeClose;
var fork2 = scopeFork;

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/internal/effect/circular.js
var Semaphore = class {
  static {
    __name(this, "Semaphore");
  }
  permits;
  waiters = /* @__PURE__ */ new Set();
  taken = 0;
  constructor(permits) {
    this.permits = permits;
  }
  get free() {
    return this.permits - this.taken;
  }
  take = /* @__PURE__ */ __name((n) => asyncInterrupt((resume2) => {
    if (this.free < n) {
      const observer = /* @__PURE__ */ __name(() => {
        if (this.free < n) return;
        this.waiters.delete(observer);
        resume2(suspend(() => {
          if (this.free < n) return this.take(n);
          this.taken += n;
          return succeed(n);
        }));
      }, "observer");
      this.waiters.add(observer);
      return sync(() => {
        this.waiters.delete(observer);
      });
    }
    resume2(suspend(() => {
      if (this.free < n) return this.take(n);
      this.taken += n;
      return succeed(n);
    }));
  }), "take");
  updateTakenUnsafe(fiber, f) {
    this.taken = f(this.taken);
    if (this.waiters.size > 0) {
      fiber.getFiberRef(currentScheduler).scheduleTask(() => {
        const iter = this.waiters.values();
        let item = iter.next();
        while (item.done === false && this.free > 0) {
          item.value();
          item = iter.next();
        }
      }, fiber.getFiberRef(currentSchedulingPriority), fiber);
    }
    return succeed(this.free);
  }
  updateTaken(f) {
    return withFiberRuntime((fiber) => this.updateTakenUnsafe(fiber, f));
  }
  resize = /* @__PURE__ */ __name((permits) => asVoid(withFiberRuntime((fiber) => {
    this.permits = permits;
    if (this.free < 0) {
      return void_;
    }
    return this.updateTakenUnsafe(fiber, (taken) => taken);
  })), "resize");
  release = /* @__PURE__ */ __name((n) => this.updateTaken((taken) => taken - n), "release");
  releaseAll = /* @__PURE__ */ this.updateTaken((_) => 0);
  withPermits = /* @__PURE__ */ __name((n) => (self) => uninterruptibleMask((restore) => flatMap7(restore(this.take(n)), (permits) => ensuring(restore(self), this.release(permits)))), "withPermits");
  withPermitsIfAvailable = /* @__PURE__ */ __name((n) => (self) => uninterruptibleMask((restore) => suspend(() => {
    if (this.free < n) {
      return succeedNone;
    }
    this.taken += n;
    return ensuring(restore(asSome(self)), this.release(n));
  })), "withPermitsIfAvailable");
};
var unsafeMakeSemaphore = /* @__PURE__ */ __name((permits) => new Semaphore(permits), "unsafeMakeSemaphore");
var makeSemaphore = /* @__PURE__ */ __name((permits) => sync(() => unsafeMakeSemaphore(permits)), "makeSemaphore");
var Latch = class extends Class2 {
  static {
    __name(this, "Latch");
  }
  isOpen;
  waiters = [];
  scheduled = false;
  constructor(isOpen) {
    super();
    this.isOpen = isOpen;
  }
  commit() {
    return this.await;
  }
  unsafeSchedule(fiber) {
    if (this.scheduled || this.waiters.length === 0) {
      return void_;
    }
    this.scheduled = true;
    fiber.currentScheduler.scheduleTask(this.flushWaiters, fiber.getFiberRef(currentSchedulingPriority), fiber);
    return void_;
  }
  flushWaiters = /* @__PURE__ */ __name(() => {
    this.scheduled = false;
    const waiters = this.waiters;
    this.waiters = [];
    for (let i = 0; i < waiters.length; i++) {
      waiters[i](exitVoid);
    }
  }, "flushWaiters");
  open = /* @__PURE__ */ withFiberRuntime((fiber) => {
    if (this.isOpen) {
      return void_;
    }
    this.isOpen = true;
    return this.unsafeSchedule(fiber);
  });
  unsafeOpen() {
    if (this.isOpen) return;
    this.isOpen = true;
    this.flushWaiters();
  }
  release = /* @__PURE__ */ withFiberRuntime((fiber) => {
    if (this.isOpen) {
      return void_;
    }
    return this.unsafeSchedule(fiber);
  });
  await = /* @__PURE__ */ asyncInterrupt((resume2) => {
    if (this.isOpen) {
      return resume2(void_);
    }
    this.waiters.push(resume2);
    return sync(() => {
      const index = this.waiters.indexOf(resume2);
      if (index !== -1) {
        this.waiters.splice(index, 1);
      }
    });
  });
  unsafeClose() {
    this.isOpen = false;
  }
  close = /* @__PURE__ */ sync(() => {
    this.isOpen = false;
  });
  whenOpen = /* @__PURE__ */ __name((self) => {
    return zipRight(this.await, self);
  }, "whenOpen");
};
var unsafeMakeLatch = /* @__PURE__ */ __name((open) => new Latch(open ?? false), "unsafeMakeLatch");
var makeLatch = /* @__PURE__ */ __name((open) => sync(() => unsafeMakeLatch(open)), "makeLatch");
var awaitAllChildren = /* @__PURE__ */ __name((self) => ensuringChildren(self, fiberAwaitAll), "awaitAllChildren");
var cached2 = /* @__PURE__ */ dual(2, (self, timeToLive) => map8(cachedInvalidateWithTTL(self, timeToLive), (tuple) => tuple[0]));
var cachedInvalidateWithTTL = /* @__PURE__ */ dual(2, (self, timeToLive) => {
  const duration = decode(timeToLive);
  return flatMap7(context(), (env) => map8(makeSynchronized(none2()), (cache) => [provideContext(getCachedValue(self, duration, cache), env), invalidateCache(cache)]));
});
var computeCachedValue = /* @__PURE__ */ __name((self, timeToLive, start3) => {
  const timeToLiveMillis = toMillis(decode(timeToLive));
  return pipe(deferredMake(), tap((deferred) => intoDeferred(self, deferred)), map8((deferred) => some2([start3 + timeToLiveMillis, deferred])));
}, "computeCachedValue");
var getCachedValue = /* @__PURE__ */ __name((self, timeToLive, cache) => uninterruptibleMask((restore) => pipe(clockWith3((clock3) => clock3.currentTimeMillis), flatMap7((time) => updateSomeAndGetEffectSynchronized(cache, (option3) => {
  switch (option3._tag) {
    case "None": {
      return some2(computeCachedValue(self, timeToLive, time));
    }
    case "Some": {
      const [end3] = option3.value;
      return end3 - time <= 0 ? some2(computeCachedValue(self, timeToLive, time)) : none2();
    }
  }
})), flatMap7((option3) => isNone2(option3) ? dieMessage("BUG: Effect.cachedInvalidate - please report an issue at https://github.com/Effect-TS/effect/issues") : restore(deferredAwait(option3.value[1]))))), "getCachedValue");
var invalidateCache = /* @__PURE__ */ __name((cache) => set5(cache, none2()), "invalidateCache");
var ensuringChild = /* @__PURE__ */ dual(2, (self, f) => ensuringChildren(self, (children) => f(fiberAll(children))));
var ensuringChildren = /* @__PURE__ */ dual(2, (self, children) => flatMap7(track, (supervisor) => pipe(supervised(self, supervisor), ensuring(flatMap7(supervisor.value, children)))));
var forkAll = /* @__PURE__ */ dual((args2) => isIterable(args2[0]), (effects, options) => options?.discard ? forEachSequentialDiscard(effects, fork) : map8(forEachSequential(effects, fork), fiberAll));
var forkIn = /* @__PURE__ */ dual(2, (self, scope3) => withFiberRuntime((parent, parentStatus) => {
  const scopeImpl = scope3;
  const fiber = unsafeFork2(self, parent, parentStatus.runtimeFlags, globalScope);
  if (scopeImpl.state._tag === "Open") {
    const finalizer = /* @__PURE__ */ __name(() => fiberIdWith((fiberId3) => equals(fiberId3, fiber.id()) ? void_ : asVoid(interruptFiber(fiber))), "finalizer");
    const key = {};
    scopeImpl.state.finalizers.set(key, finalizer);
    fiber.addObserver(() => {
      if (scopeImpl.state._tag === "Closed") return;
      scopeImpl.state.finalizers.delete(key);
    });
  } else {
    fiber.unsafeInterruptAsFork(parent.id());
  }
  return succeed(fiber);
}));
var forkScoped = /* @__PURE__ */ __name((self) => scopeWith((scope3) => forkIn(self, scope3)), "forkScoped");
var fromFiber = /* @__PURE__ */ __name((fiber) => join2(fiber), "fromFiber");
var fromFiberEffect = /* @__PURE__ */ __name((fiber) => suspend(() => flatMap7(fiber, join2)), "fromFiberEffect");
var memoKeySymbol = /* @__PURE__ */ Symbol.for("effect/Effect/memoizeFunction.key");
var Key = class {
  static {
    __name(this, "Key");
  }
  a;
  eq;
  [memoKeySymbol] = memoKeySymbol;
  constructor(a, eq) {
    this.a = a;
    this.eq = eq;
  }
  [symbol2](that) {
    if (hasProperty(that, memoKeySymbol)) {
      if (this.eq) {
        return this.eq(this.a, that.a);
      } else {
        return equals(this.a, that.a);
      }
    }
    return false;
  }
  [symbol]() {
    return this.eq ? 0 : cached(this, hash(this.a));
  }
};
var cachedFunction = /* @__PURE__ */ __name((f, eq) => {
  return pipe(sync(() => empty17()), flatMap7(makeSynchronized), map8((ref) => (a) => pipe(ref.modifyEffect((map14) => {
    const result = pipe(map14, get8(new Key(a, eq)));
    if (isNone2(result)) {
      return pipe(deferredMake(), tap((deferred) => pipe(diffFiberRefs(f(a)), intoDeferred(deferred), fork)), map8((deferred) => [deferred, pipe(map14, set4(new Key(a, eq), deferred))]));
    }
    return succeed([result.value, map14]);
  }), flatMap7(deferredAwait), flatMap7(([patch9, b]) => pipe(patchFiberRefs(patch9), as2(b))))));
}, "cachedFunction");
var raceFirst = /* @__PURE__ */ dual(2, (self, that) => pipe(exit(self), race(exit(that)), (effect) => flatten4(effect)));
var supervised = /* @__PURE__ */ dual(2, (self, supervisor) => {
  const supervise = fiberRefLocallyWith(currentSupervisor, (s) => s.zip(supervisor));
  return supervise(self);
});
var timeout = /* @__PURE__ */ dual(2, (self, duration) => timeoutFail(self, {
  onTimeout: /* @__PURE__ */ __name(() => timeoutExceptionFromDuration(duration), "onTimeout"),
  duration
}));
var timeoutFail = /* @__PURE__ */ dual(2, (self, {
  duration,
  onTimeout
}) => flatten4(timeoutTo(self, {
  onTimeout: /* @__PURE__ */ __name(() => failSync(onTimeout), "onTimeout"),
  onSuccess: succeed,
  duration
})));
var timeoutFailCause = /* @__PURE__ */ dual(2, (self, {
  duration,
  onTimeout
}) => flatten4(timeoutTo(self, {
  onTimeout: /* @__PURE__ */ __name(() => failCauseSync(onTimeout), "onTimeout"),
  onSuccess: succeed,
  duration
})));
var timeoutOption = /* @__PURE__ */ dual(2, (self, duration) => timeoutTo(self, {
  duration,
  onSuccess: some2,
  onTimeout: none2
}));
var timeoutTo = /* @__PURE__ */ dual(2, (self, {
  duration,
  onSuccess,
  onTimeout
}) => fiberIdWith((parentFiberId) => uninterruptibleMask((restore) => raceFibersWith(restore(self), interruptible2(sleep3(duration)), {
  onSelfWin: /* @__PURE__ */ __name((winner, loser) => flatMap7(winner.await, (exit4) => {
    if (exit4._tag === "Success") {
      return flatMap7(winner.inheritAll, () => as2(interruptAsFiber(loser, parentFiberId), onSuccess(exit4.value)));
    } else {
      return flatMap7(interruptAsFiber(loser, parentFiberId), () => exitFailCause(exit4.cause));
    }
  }), "onSelfWin"),
  onOtherWin: /* @__PURE__ */ __name((winner, loser) => flatMap7(winner.await, (exit4) => {
    if (exit4._tag === "Success") {
      return flatMap7(winner.inheritAll, () => as2(interruptAsFiber(loser, parentFiberId), onTimeout()));
    } else {
      return flatMap7(interruptAsFiber(loser, parentFiberId), () => exitFailCause(exit4.cause));
    }
  }), "onOtherWin"),
  otherScope: globalScope
}))));
var SynchronizedSymbolKey = "effect/Ref/SynchronizedRef";
var SynchronizedTypeId = /* @__PURE__ */ Symbol.for(SynchronizedSymbolKey);
var synchronizedVariance = {
  /* c8 ignore next */
  _A: /* @__PURE__ */ __name((_) => _, "_A")
};
var SynchronizedImpl = class extends Class2 {
  static {
    __name(this, "SynchronizedImpl");
  }
  ref;
  withLock;
  [SynchronizedTypeId] = synchronizedVariance;
  [RefTypeId] = refVariance;
  [TypeId12] = TypeId12;
  constructor(ref, withLock) {
    super();
    this.ref = ref;
    this.withLock = withLock;
    this.get = get11(this.ref);
  }
  get;
  commit() {
    return this.get;
  }
  modify(f) {
    return this.modifyEffect((a) => succeed(f(a)));
  }
  modifyEffect(f) {
    return this.withLock(pipe(flatMap7(get11(this.ref), f), flatMap7(([b, a]) => as2(set5(this.ref, a), b))));
  }
};
var makeSynchronized = /* @__PURE__ */ __name((value) => sync(() => unsafeMakeSynchronized(value)), "makeSynchronized");
var unsafeMakeSynchronized = /* @__PURE__ */ __name((value) => {
  const ref = unsafeMake5(value);
  const sem = unsafeMakeSemaphore(1);
  return new SynchronizedImpl(ref, sem.withPermits(1));
}, "unsafeMakeSynchronized");
var updateSomeAndGetEffectSynchronized = /* @__PURE__ */ dual(2, (self, pf) => self.modifyEffect((value) => {
  const result = pf(value);
  switch (result._tag) {
    case "None": {
      return succeed([value, value]);
    }
    case "Some": {
      return map8(result.value, (a) => [a, a]);
    }
  }
}));
var bindAll = /* @__PURE__ */ dual((args2) => isEffect(args2[0]), (self, f, options) => flatMap7(self, (a) => all3(f(a), options).pipe(map8((record) => Object.assign({}, a, record)))));

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/internal/managedRuntime/circular.js
var TypeId15 = /* @__PURE__ */ Symbol.for("effect/ManagedRuntime");

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/internal/opCodes/layer.js
var OP_FRESH = "Fresh";
var OP_FROM_EFFECT = "FromEffect";
var OP_SCOPED = "Scoped";
var OP_SUSPEND = "Suspend";
var OP_PROVIDE = "Provide";
var OP_PROVIDE_MERGE = "ProvideMerge";
var OP_MERGE_ALL = "MergeAll";

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/Fiber.js
var interruptAs = interruptAsFiber;

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/internal/runtime.js
var makeDual = /* @__PURE__ */ __name((f) => function() {
  if (arguments.length === 1) {
    const runtime4 = arguments[0];
    return (effect, ...args2) => f(runtime4, effect, ...args2);
  }
  return f.apply(this, arguments);
}, "makeDual");
var unsafeFork3 = /* @__PURE__ */ makeDual((runtime4, self, options) => {
  const fiberId3 = unsafeMake2();
  const fiberRefUpdates = [[currentContext, [[fiberId3, runtime4.context]]]];
  if (options?.scheduler) {
    fiberRefUpdates.push([currentScheduler, [[fiberId3, options.scheduler]]]);
  }
  let fiberRefs3 = updateManyAs2(runtime4.fiberRefs, {
    entries: fiberRefUpdates,
    forkAs: fiberId3
  });
  if (options?.updateRefs) {
    fiberRefs3 = options.updateRefs(fiberRefs3, fiberId3);
  }
  const fiberRuntime = new FiberRuntime(fiberId3, fiberRefs3, runtime4.runtimeFlags);
  let effect = self;
  if (options?.scope) {
    effect = flatMap7(fork2(options.scope, sequential2), (closeableScope) => zipRight(scopeAddFinalizer(closeableScope, fiberIdWith((id) => equals(id, fiberRuntime.id()) ? void_ : interruptAsFiber(fiberRuntime, id))), onExit(self, (exit4) => close(closeableScope, exit4))));
  }
  const supervisor = fiberRuntime.currentSupervisor;
  if (supervisor !== none8) {
    supervisor.onStart(runtime4.context, effect, none2(), fiberRuntime);
    fiberRuntime.addObserver((exit4) => supervisor.onEnd(exit4, fiberRuntime));
  }
  globalScope.add(runtime4.runtimeFlags, fiberRuntime);
  if (options?.immediate === false) {
    fiberRuntime.resume(effect);
  } else {
    fiberRuntime.start(effect);
  }
  return fiberRuntime;
});
var unsafeRunCallback = /* @__PURE__ */ makeDual((runtime4, effect, options = {}) => {
  const fiberRuntime = unsafeFork3(runtime4, effect, options);
  if (options.onExit) {
    fiberRuntime.addObserver((exit4) => {
      options.onExit(exit4);
    });
  }
  return (id, cancelOptions) => unsafeRunCallback(runtime4)(pipe(fiberRuntime, interruptAs(id ?? none4)), {
    ...cancelOptions,
    onExit: cancelOptions?.onExit ? (exit4) => cancelOptions.onExit(flatten5(exit4)) : void 0
  });
});
var unsafeRunSync = /* @__PURE__ */ makeDual((runtime4, effect) => {
  const result = unsafeRunSyncExit(runtime4)(effect);
  if (result._tag === "Failure") {
    throw fiberFailure(result.effect_instruction_i0);
  }
  return result.effect_instruction_i0;
});
var AsyncFiberExceptionImpl = class extends Error {
  static {
    __name(this, "AsyncFiberExceptionImpl");
  }
  fiber;
  _tag = "AsyncFiberException";
  constructor(fiber) {
    super(`Fiber #${fiber.id().id} cannot be resolved synchronously. This is caused by using runSync on an effect that performs async work`);
    this.fiber = fiber;
    this.name = this._tag;
    this.stack = this.message;
  }
};
var asyncFiberException = /* @__PURE__ */ __name((fiber) => {
  const limit = Error.stackTraceLimit;
  Error.stackTraceLimit = 0;
  const error = new AsyncFiberExceptionImpl(fiber);
  Error.stackTraceLimit = limit;
  return error;
}, "asyncFiberException");
var FiberFailureId = /* @__PURE__ */ Symbol.for("effect/Runtime/FiberFailure");
var FiberFailureCauseId = /* @__PURE__ */ Symbol.for("effect/Runtime/FiberFailure/Cause");
var FiberFailureImpl = class extends Error {
  static {
    __name(this, "FiberFailureImpl");
  }
  [FiberFailureId];
  [FiberFailureCauseId];
  constructor(cause3) {
    const head5 = prettyErrors(cause3)[0];
    super(head5?.message || "An error has occurred");
    this[FiberFailureId] = FiberFailureId;
    this[FiberFailureCauseId] = cause3;
    this.name = head5 ? `(FiberFailure) ${head5.name}` : "FiberFailure";
    if (head5?.stack) {
      this.stack = head5.stack;
    }
  }
  toJSON() {
    return {
      _id: "FiberFailure",
      cause: this[FiberFailureCauseId].toJSON()
    };
  }
  toString() {
    return "(FiberFailure) " + pretty(this[FiberFailureCauseId], {
      renderErrorCause: true
    });
  }
  [NodeInspectSymbol]() {
    return this.toString();
  }
};
var fiberFailure = /* @__PURE__ */ __name((cause3) => {
  const limit = Error.stackTraceLimit;
  Error.stackTraceLimit = 0;
  const error = new FiberFailureImpl(cause3);
  Error.stackTraceLimit = limit;
  return error;
}, "fiberFailure");
var fastPath = /* @__PURE__ */ __name((effect) => {
  const op = effect;
  switch (op._op) {
    case "Failure":
    case "Success": {
      return op;
    }
    case "Left": {
      return exitFail(op.left);
    }
    case "Right": {
      return exitSucceed(op.right);
    }
    case "Some": {
      return exitSucceed(op.value);
    }
    case "None": {
      return exitFail(new NoSuchElementException());
    }
  }
}, "fastPath");
var unsafeRunSyncExit = /* @__PURE__ */ makeDual((runtime4, effect) => {
  const op = fastPath(effect);
  if (op) {
    return op;
  }
  const scheduler = new SyncScheduler();
  const fiberRuntime = unsafeFork3(runtime4)(effect, {
    scheduler
  });
  scheduler.flush();
  const result = fiberRuntime.unsafePoll();
  if (result) {
    return result;
  }
  return exitDie(capture(asyncFiberException(fiberRuntime), currentSpanFromFiber(fiberRuntime)));
});
var unsafeRunPromise = /* @__PURE__ */ makeDual((runtime4, effect, options) => unsafeRunPromiseExit(runtime4, effect, options).then((result) => {
  switch (result._tag) {
    case OP_SUCCESS: {
      return result.effect_instruction_i0;
    }
    case OP_FAILURE: {
      throw fiberFailure(result.effect_instruction_i0);
    }
  }
}));
var unsafeRunPromiseExit = /* @__PURE__ */ makeDual((runtime4, effect, options) => new Promise((resolve) => {
  const op = fastPath(effect);
  if (op) {
    resolve(op);
  }
  const fiber = unsafeFork3(runtime4)(effect);
  fiber.addObserver((exit4) => {
    resolve(exit4);
  });
  if (options?.signal !== void 0) {
    if (options.signal.aborted) {
      fiber.unsafeInterruptAsFork(fiber.id());
    } else {
      options.signal.addEventListener("abort", () => {
        fiber.unsafeInterruptAsFork(fiber.id());
      }, {
        once: true
      });
    }
  }
}));
var RuntimeImpl = class {
  static {
    __name(this, "RuntimeImpl");
  }
  context;
  runtimeFlags;
  fiberRefs;
  constructor(context4, runtimeFlags2, fiberRefs3) {
    this.context = context4;
    this.runtimeFlags = runtimeFlags2;
    this.fiberRefs = fiberRefs3;
  }
  pipe() {
    return pipeArguments(this, arguments);
  }
};
var make38 = /* @__PURE__ */ __name((options) => new RuntimeImpl(options.context, options.runtimeFlags, options.fiberRefs), "make");
var runtime2 = /* @__PURE__ */ __name(() => withFiberRuntime((state, status) => succeed(new RuntimeImpl(state.getFiberRef(currentContext), status.runtimeFlags, state.getFiberRefs()))), "runtime");
var defaultRuntimeFlags = /* @__PURE__ */ make16(Interruption, CooperativeYielding, RuntimeMetrics);
var defaultRuntime = /* @__PURE__ */ make38({
  context: /* @__PURE__ */ empty3(),
  runtimeFlags: defaultRuntimeFlags,
  fiberRefs: /* @__PURE__ */ empty21()
});
var unsafeRunEffect = /* @__PURE__ */ unsafeRunCallback(defaultRuntime);
var unsafeForkEffect = /* @__PURE__ */ unsafeFork3(defaultRuntime);
var unsafeRunPromiseEffect = /* @__PURE__ */ unsafeRunPromise(defaultRuntime);
var unsafeRunPromiseExitEffect = /* @__PURE__ */ unsafeRunPromiseExit(defaultRuntime);
var unsafeRunSyncEffect = /* @__PURE__ */ unsafeRunSync(defaultRuntime);
var unsafeRunSyncExitEffect = /* @__PURE__ */ unsafeRunSyncExit(defaultRuntime);
var asyncEffect = /* @__PURE__ */ __name((register) => suspend(() => {
  let cleanup = void 0;
  return flatMap7(deferredMake(), (deferred) => flatMap7(runtime2(), (runtime4) => uninterruptibleMask((restore) => zipRight(fork(restore(matchCauseEffect(register((cb) => unsafeRunCallback(runtime4)(intoDeferred(cb, deferred))), {
    onFailure: /* @__PURE__ */ __name((cause3) => deferredFailCause(deferred, cause3), "onFailure"),
    onSuccess: /* @__PURE__ */ __name((cleanup_) => {
      cleanup = cleanup_;
      return void_;
    }, "onSuccess")
  }))), restore(onInterrupt(deferredAwait(deferred), () => cleanup ?? void_))))));
}), "asyncEffect");

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/internal/synchronizedRef.js
var modifyEffect = /* @__PURE__ */ dual(2, (self, f) => self.modifyEffect(f));

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/internal/layer.js
var LayerSymbolKey = "effect/Layer";
var LayerTypeId = /* @__PURE__ */ Symbol.for(LayerSymbolKey);
var layerVariance = {
  /* c8 ignore next */
  _RIn: /* @__PURE__ */ __name((_) => _, "_RIn"),
  /* c8 ignore next */
  _E: /* @__PURE__ */ __name((_) => _, "_E"),
  /* c8 ignore next */
  _ROut: /* @__PURE__ */ __name((_) => _, "_ROut")
};
var proto3 = {
  [LayerTypeId]: layerVariance,
  pipe() {
    return pipeArguments(this, arguments);
  }
};
var MemoMapTypeIdKey = "effect/Layer/MemoMap";
var MemoMapTypeId = /* @__PURE__ */ Symbol.for(MemoMapTypeIdKey);
var CurrentMemoMap = /* @__PURE__ */ Reference2()("effect/Layer/CurrentMemoMap", {
  defaultValue: /* @__PURE__ */ __name(() => unsafeMakeMemoMap(), "defaultValue")
});
var isLayer = /* @__PURE__ */ __name((u) => hasProperty(u, LayerTypeId), "isLayer");
var isFresh = /* @__PURE__ */ __name((self) => {
  return self._op_layer === OP_FRESH;
}, "isFresh");
var MemoMapImpl = class {
  static {
    __name(this, "MemoMapImpl");
  }
  ref;
  [MemoMapTypeId];
  constructor(ref) {
    this.ref = ref;
    this[MemoMapTypeId] = MemoMapTypeId;
  }
  /**
   * Checks the memo map to see if a layer exists. If it is, immediately
   * returns it. Otherwise, obtains the layer, stores it in the memo map,
   * and adds a finalizer to the `Scope`.
   */
  getOrElseMemoize(layer, scope3) {
    return pipe(modifyEffect(this.ref, (map14) => {
      const inMap = map14.get(layer);
      if (inMap !== void 0) {
        const [acquire, release] = inMap;
        const cached4 = pipe(acquire, flatMap7(([patch9, b]) => pipe(patchFiberRefs(patch9), as2(b))), onExit(exitMatch({
          onFailure: /* @__PURE__ */ __name(() => void_, "onFailure"),
          onSuccess: /* @__PURE__ */ __name(() => scopeAddFinalizerExit(scope3, release), "onSuccess")
        })));
        return succeed([cached4, map14]);
      }
      return pipe(make26(0), flatMap7((observers) => pipe(deferredMake(), flatMap7((deferred) => pipe(make26(() => void_), map8((finalizerRef) => {
        const resource = uninterruptibleMask((restore) => pipe(scopeMake(), flatMap7((innerScope) => pipe(restore(flatMap7(makeBuilder(layer, innerScope, true), (f) => diffFiberRefs(f(this)))), exit, flatMap7((exit4) => {
          switch (exit4._tag) {
            case OP_FAILURE: {
              return pipe(deferredFailCause(deferred, exit4.effect_instruction_i0), zipRight(scopeClose(innerScope, exit4)), zipRight(failCause(exit4.effect_instruction_i0)));
            }
            case OP_SUCCESS: {
              return pipe(set5(finalizerRef, (exit5) => pipe(scopeClose(innerScope, exit5), whenEffect(modify3(observers, (n) => [n === 1, n - 1])), asVoid)), zipRight(update2(observers, (n) => n + 1)), zipRight(scopeAddFinalizerExit(scope3, (exit5) => pipe(sync(() => map14.delete(layer)), zipRight(get11(finalizerRef)), flatMap7((finalizer) => finalizer(exit5))))), zipRight(deferredSucceed(deferred, exit4.effect_instruction_i0)), as2(exit4.effect_instruction_i0[1]));
            }
          }
        })))));
        const memoized = [pipe(deferredAwait(deferred), onExit(exitMatchEffect({
          onFailure: /* @__PURE__ */ __name(() => void_, "onFailure"),
          onSuccess: /* @__PURE__ */ __name(() => update2(observers, (n) => n + 1), "onSuccess")
        }))), (exit4) => pipe(get11(finalizerRef), flatMap7((finalizer) => finalizer(exit4)))];
        return [resource, isFresh(layer) ? map14 : map14.set(layer, memoized)];
      }))))));
    }), flatten4);
  }
};
var makeMemoMap = /* @__PURE__ */ suspend(() => map8(makeSynchronized(/* @__PURE__ */ new Map()), (ref) => new MemoMapImpl(ref)));
var unsafeMakeMemoMap = /* @__PURE__ */ __name(() => new MemoMapImpl(unsafeMakeSynchronized(/* @__PURE__ */ new Map())), "unsafeMakeMemoMap");
var buildWithScope = /* @__PURE__ */ dual(2, (self, scope3) => flatMap7(makeMemoMap, (memoMap) => buildWithMemoMap(self, memoMap, scope3)));
var buildWithMemoMap = /* @__PURE__ */ dual(3, (self, memoMap, scope3) => flatMap7(makeBuilder(self, scope3), (run) => provideService(run(memoMap), CurrentMemoMap, memoMap)));
var makeBuilder = /* @__PURE__ */ __name((self, scope3, inMemoMap = false) => {
  const op = self;
  switch (op._op_layer) {
    case "Locally": {
      return sync(() => (memoMap) => op.f(memoMap.getOrElseMemoize(op.self, scope3)));
    }
    case "ExtendScope": {
      return sync(() => (memoMap) => scopeWith((scope4) => memoMap.getOrElseMemoize(op.layer, scope4)));
    }
    case "Fold": {
      return sync(() => (memoMap) => pipe(memoMap.getOrElseMemoize(op.layer, scope3), matchCauseEffect({
        onFailure: /* @__PURE__ */ __name((cause3) => memoMap.getOrElseMemoize(op.failureK(cause3), scope3), "onFailure"),
        onSuccess: /* @__PURE__ */ __name((value) => memoMap.getOrElseMemoize(op.successK(value), scope3), "onSuccess")
      })));
    }
    case "Fresh": {
      return sync(() => (_) => pipe(op.layer, buildWithScope(scope3)));
    }
    case "FromEffect": {
      return inMemoMap ? sync(() => (_) => op.effect) : sync(() => (memoMap) => memoMap.getOrElseMemoize(self, scope3));
    }
    case "Provide": {
      return sync(() => (memoMap) => pipe(memoMap.getOrElseMemoize(op.first, scope3), flatMap7((env) => pipe(memoMap.getOrElseMemoize(op.second, scope3), provideContext(env)))));
    }
    case "Scoped": {
      return inMemoMap ? sync(() => (_) => scopeExtend(op.effect, scope3)) : sync(() => (memoMap) => memoMap.getOrElseMemoize(self, scope3));
    }
    case "Suspend": {
      return sync(() => (memoMap) => memoMap.getOrElseMemoize(op.evaluate(), scope3));
    }
    case "ProvideMerge": {
      return sync(() => (memoMap) => pipe(memoMap.getOrElseMemoize(op.first, scope3), zipWith2(memoMap.getOrElseMemoize(op.second, scope3), op.zipK)));
    }
    case "ZipWith": {
      return gen(function* () {
        const parallelScope = yield* scopeFork(scope3, parallel2);
        const firstScope = yield* scopeFork(parallelScope, sequential2);
        const secondScope = yield* scopeFork(parallelScope, sequential2);
        return (memoMap) => pipe(memoMap.getOrElseMemoize(op.first, firstScope), zipWithOptions(memoMap.getOrElseMemoize(op.second, secondScope), op.zipK, {
          concurrent: true
        }));
      });
    }
    case "MergeAll": {
      const layers = op.layers;
      return map8(scopeFork(scope3, parallel2), (parallelScope) => (memoMap) => {
        const contexts = new Array(layers.length);
        return map8(forEachConcurrentDiscard(layers, fnUntraced(function* (layer, i) {
          const scope4 = yield* scopeFork(parallelScope, sequential2);
          const context4 = yield* memoMap.getOrElseMemoize(layer, scope4);
          contexts[i] = context4;
        }), false, false), () => mergeAll2(...contexts));
      });
    }
  }
}, "makeBuilder");
var context2 = /* @__PURE__ */ __name(() => fromEffectContext(context()), "context");
var fromEffect2 = /* @__PURE__ */ dual(2, (a, b) => {
  const tagFirst = isTag2(a);
  const tag = tagFirst ? a : b;
  const effect = tagFirst ? b : a;
  return fromEffectContext(map8(effect, (service) => make5(tag, service)));
});
function fromEffectContext(effect) {
  const fromEffect3 = Object.create(proto3);
  fromEffect3._op_layer = OP_FROM_EFFECT;
  fromEffect3.effect = effect;
  return fromEffect3;
}
__name(fromEffectContext, "fromEffectContext");
var mergeAll4 = /* @__PURE__ */ __name((...layers) => {
  const mergeAll6 = Object.create(proto3);
  mergeAll6._op_layer = OP_MERGE_ALL;
  mergeAll6.layers = layers;
  return mergeAll6;
}, "mergeAll");
var scoped = /* @__PURE__ */ dual(2, (a, b) => {
  const tagFirst = isTag2(a);
  const tag = tagFirst ? a : b;
  const effect = tagFirst ? b : a;
  return scopedContext(map8(effect, (service) => make5(tag, service)));
});
var scopedContext = /* @__PURE__ */ __name((effect) => {
  const scoped3 = Object.create(proto3);
  scoped3._op_layer = OP_SCOPED;
  scoped3.effect = effect;
  return scoped3;
}, "scopedContext");
var succeed4 = /* @__PURE__ */ dual(2, (a, b) => {
  const tagFirst = isTag2(a);
  const tag = tagFirst ? a : b;
  const resource = tagFirst ? b : a;
  return fromEffectContext(succeed(make5(tag, resource)));
});
var suspend3 = /* @__PURE__ */ __name((evaluate2) => {
  const suspend5 = Object.create(proto3);
  suspend5._op_layer = OP_SUSPEND;
  suspend5.evaluate = evaluate2;
  return suspend5;
}, "suspend");
var sync3 = /* @__PURE__ */ dual(2, (a, b) => {
  const tagFirst = isTag2(a);
  const tag = tagFirst ? a : b;
  const evaluate2 = tagFirst ? b : a;
  return fromEffectContext(sync(() => make5(tag, evaluate2())));
});
var provide = /* @__PURE__ */ dual(2, (self, that) => suspend3(() => {
  const provideTo = Object.create(proto3);
  provideTo._op_layer = OP_PROVIDE;
  provideTo.first = Object.create(proto3, {
    _op_layer: {
      value: OP_PROVIDE_MERGE,
      enumerable: true
    },
    first: {
      value: context2(),
      enumerable: true
    },
    second: {
      value: Array.isArray(that) ? mergeAll4(...that) : that
    },
    zipK: {
      value: /* @__PURE__ */ __name((a, b) => pipe(a, merge3(b)), "value")
    }
  });
  provideTo.second = self;
  return provideTo;
}));
var provideSomeLayer = /* @__PURE__ */ dual(2, (self, layer) => scopedWith((scope3) => flatMap7(buildWithScope(layer, scope3), (context4) => provideSomeContext(self, context4))));
var provideSomeRuntime = /* @__PURE__ */ dual(2, (self, rt) => {
  const patchRefs = diff6(defaultRuntime.fiberRefs, rt.fiberRefs);
  const patchFlags = diff4(defaultRuntime.runtimeFlags, rt.runtimeFlags);
  return uninterruptibleMask((restore) => withFiberRuntime((fiber) => {
    const oldContext = fiber.getFiberRef(currentContext);
    const oldRefs = fiber.getFiberRefs();
    const newRefs = patch7(fiber.id(), oldRefs)(patchRefs);
    const oldFlags = fiber.currentRuntimeFlags;
    const newFlags = patch4(patchFlags)(oldFlags);
    const rollbackRefs = diff6(newRefs, oldRefs);
    const rollbackFlags = diff4(newFlags, oldFlags);
    fiber.setFiberRefs(newRefs);
    fiber.currentRuntimeFlags = newFlags;
    return ensuring(provideSomeContext(restore(self), merge3(oldContext, rt.context)), withFiberRuntime((fiber2) => {
      fiber2.setFiberRefs(patch7(fiber2.id(), fiber2.getFiberRefs())(rollbackRefs));
      fiber2.currentRuntimeFlags = patch4(rollbackFlags)(fiber2.currentRuntimeFlags);
      return void_;
    }));
  }));
});
var effect_provide = /* @__PURE__ */ dual(2, (self, source) => {
  if (Array.isArray(source)) {
    return provideSomeLayer(self, mergeAll4(...source));
  } else if (isLayer(source)) {
    return provideSomeLayer(self, source);
  } else if (isContext2(source)) {
    return provideSomeContext(self, source);
  } else if (TypeId15 in source) {
    return flatMap7(source.runtimeEffect, (rt) => provideSomeRuntime(self, rt));
  } else {
    return provideSomeRuntime(self, source);
  }
});

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/internal/console.js
var console2 = /* @__PURE__ */ map8(/* @__PURE__ */ fiberRefGet(currentServices), /* @__PURE__ */ get3(consoleTag));
var consoleWith = /* @__PURE__ */ __name((f) => fiberRefGetWith(currentServices, (services) => f(get3(services, consoleTag))), "consoleWith");
var withConsole = /* @__PURE__ */ dual(2, (effect, value) => fiberRefLocallyWith(effect, currentServices, add2(consoleTag, value)));
var withConsoleScoped = /* @__PURE__ */ __name((console4) => fiberRefLocallyScopedWith(currentServices, add2(consoleTag, console4)), "withConsoleScoped");

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/Random.js
var fixed2 = fixed;

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/internal/schedule.js
var ScheduleSymbolKey = "effect/Schedule";
var ScheduleTypeId = /* @__PURE__ */ Symbol.for(ScheduleSymbolKey);
var isSchedule = /* @__PURE__ */ __name((u) => hasProperty(u, ScheduleTypeId), "isSchedule");
var ScheduleDriverSymbolKey = "effect/ScheduleDriver";
var ScheduleDriverTypeId = /* @__PURE__ */ Symbol.for(ScheduleDriverSymbolKey);
var defaultIterationMetadata = {
  start: 0,
  now: 0,
  input: void 0,
  output: void 0,
  elapsed: zero,
  elapsedSincePrevious: zero,
  recurrence: 0
};
var CurrentIterationMetadata = /* @__PURE__ */ Reference2()("effect/Schedule/CurrentIterationMetadata", {
  defaultValue: /* @__PURE__ */ __name(() => defaultIterationMetadata, "defaultValue")
});
var scheduleVariance = {
  /* c8 ignore next */
  _Out: /* @__PURE__ */ __name((_) => _, "_Out"),
  /* c8 ignore next */
  _In: /* @__PURE__ */ __name((_) => _, "_In"),
  /* c8 ignore next */
  _R: /* @__PURE__ */ __name((_) => _, "_R")
};
var scheduleDriverVariance = {
  /* c8 ignore next */
  _Out: /* @__PURE__ */ __name((_) => _, "_Out"),
  /* c8 ignore next */
  _In: /* @__PURE__ */ __name((_) => _, "_In"),
  /* c8 ignore next */
  _R: /* @__PURE__ */ __name((_) => _, "_R")
};
var ScheduleImpl = class {
  static {
    __name(this, "ScheduleImpl");
  }
  initial;
  step;
  [ScheduleTypeId] = scheduleVariance;
  constructor(initial, step4) {
    this.initial = initial;
    this.step = step4;
  }
  pipe() {
    return pipeArguments(this, arguments);
  }
};
var updateInfo = /* @__PURE__ */ __name((iterationMetaRef, now, input, output) => update2(iterationMetaRef, (prev) => prev.recurrence === 0 ? {
  now,
  input,
  output,
  recurrence: prev.recurrence + 1,
  elapsed: zero,
  elapsedSincePrevious: zero,
  start: now
} : {
  now,
  input,
  output,
  recurrence: prev.recurrence + 1,
  elapsed: millis(now - prev.start),
  elapsedSincePrevious: millis(now - prev.now),
  start: prev.start
}), "updateInfo");
var ScheduleDriverImpl = class {
  static {
    __name(this, "ScheduleDriverImpl");
  }
  schedule;
  ref;
  [ScheduleDriverTypeId] = scheduleDriverVariance;
  constructor(schedule2, ref) {
    this.schedule = schedule2;
    this.ref = ref;
  }
  get state() {
    return map8(get11(this.ref), (tuple) => tuple[1]);
  }
  get last() {
    return flatMap7(get11(this.ref), ([element, _]) => {
      switch (element._tag) {
        case "None": {
          return failSync(() => new NoSuchElementException());
        }
        case "Some": {
          return succeed(element.value);
        }
      }
    });
  }
  iterationMeta = /* @__PURE__ */ unsafeMake5(defaultIterationMetadata);
  get reset() {
    return set5(this.ref, [none2(), this.schedule.initial]).pipe(zipLeft(set5(this.iterationMeta, defaultIterationMetadata)));
  }
  next(input) {
    return pipe(map8(get11(this.ref), (tuple) => tuple[1]), flatMap7((state) => pipe(currentTimeMillis2, flatMap7((now) => pipe(suspend(() => this.schedule.step(now, input, state)), flatMap7(([state2, out, decision]) => {
      const setState = set5(this.ref, [some2(out), state2]);
      if (isDone4(decision)) {
        return setState.pipe(zipRight(fail2(none2())));
      }
      const millis2 = start2(decision.intervals) - now;
      if (millis2 <= 0) {
        return setState.pipe(zipRight(updateInfo(this.iterationMeta, now, input, out)), as2(out));
      }
      const duration = millis(millis2);
      return pipe(setState, zipRight(updateInfo(this.iterationMeta, now, input, out)), zipRight(sleep3(duration)), as2(out));
    }))))));
  }
};
var makeWithState = /* @__PURE__ */ __name((initial, step4) => new ScheduleImpl(initial, step4), "makeWithState");
var asVoid3 = /* @__PURE__ */ __name((self) => map12(self, constVoid), "asVoid");
var check = /* @__PURE__ */ dual(2, (self, test) => checkEffect(self, (input, out) => sync(() => test(input, out))));
var checkEffect = /* @__PURE__ */ dual(2, (self, test) => makeWithState(self.initial, (now, input, state) => flatMap7(self.step(now, input, state), ([state2, out, decision]) => {
  if (isDone4(decision)) {
    return succeed([state2, out, done6]);
  }
  return map8(test(input, out), (cont) => cont ? [state2, out, decision] : [state2, out, done6]);
})));
var driver = /* @__PURE__ */ __name((self) => pipe(make26([none2(), self.initial]), map8((ref) => new ScheduleDriverImpl(self, ref))), "driver");
var intersect5 = /* @__PURE__ */ dual(2, (self, that) => intersectWith(self, that, intersect4));
var intersectWith = /* @__PURE__ */ dual(3, (self, that, f) => makeWithState([self.initial, that.initial], (now, input, state) => pipe(zipWith2(self.step(now, input, state[0]), that.step(now, input, state[1]), (a, b) => [a, b]), flatMap7(([[lState, out, lDecision], [rState, out2, rDecision]]) => {
  if (isContinue2(lDecision) && isContinue2(rDecision)) {
    return intersectWithLoop(self, that, input, lState, out, lDecision.intervals, rState, out2, rDecision.intervals, f);
  }
  return succeed([[lState, rState], [out, out2], done6]);
}))));
var intersectWithLoop = /* @__PURE__ */ __name((self, that, input, lState, out, lInterval, rState, out2, rInterval, f) => {
  const combined = f(lInterval, rInterval);
  if (isNonEmpty4(combined)) {
    return succeed([[lState, rState], [out, out2], _continue2(combined)]);
  }
  if (pipe(lInterval, lessThan6(rInterval))) {
    return flatMap7(self.step(end2(lInterval), input, lState), ([lState2, out3, decision]) => {
      if (isDone4(decision)) {
        return succeed([[lState2, rState], [out3, out2], done6]);
      }
      return intersectWithLoop(self, that, input, lState2, out3, decision.intervals, rState, out2, rInterval, f);
    });
  }
  return flatMap7(that.step(end2(rInterval), input, rState), ([rState2, out22, decision]) => {
    if (isDone4(decision)) {
      return succeed([[lState, rState2], [out, out22], done6]);
    }
    return intersectWithLoop(self, that, input, lState, out, lInterval, rState2, out22, decision.intervals, f);
  });
}, "intersectWithLoop");
var map12 = /* @__PURE__ */ dual(2, (self, f) => mapEffect(self, (out) => sync(() => f(out))));
var mapEffect = /* @__PURE__ */ dual(2, (self, f) => makeWithState(self.initial, (now, input, state) => flatMap7(self.step(now, input, state), ([state2, out, decision]) => map8(f(out), (out2) => [state2, out2, decision]))));
var passthrough = /* @__PURE__ */ __name((self) => makeWithState(self.initial, (now, input, state) => pipe(self.step(now, input, state), map8(([state2, _, decision]) => [state2, input, decision]))), "passthrough");
var recurs = /* @__PURE__ */ __name((n) => whileOutput(forever2, (out) => out < n), "recurs");
var unfold2 = /* @__PURE__ */ __name((initial, f) => makeWithState(initial, (now, _, state) => sync(() => [f(state), state, continueWith2(after2(now))])), "unfold");
var untilInputEffect = /* @__PURE__ */ dual(2, (self, f) => checkEffect(self, (input, _) => negate(f(input))));
var whileInputEffect = /* @__PURE__ */ dual(2, (self, f) => checkEffect(self, (input, _) => f(input)));
var whileOutput = /* @__PURE__ */ dual(2, (self, f) => check(self, (_, out) => f(out)));
var ScheduleDefectTypeId = /* @__PURE__ */ Symbol.for("effect/Schedule/ScheduleDefect");
var ScheduleDefect = class {
  static {
    __name(this, "ScheduleDefect");
  }
  error;
  [ScheduleDefectTypeId];
  constructor(error) {
    this.error = error;
    this[ScheduleDefectTypeId] = ScheduleDefectTypeId;
  }
};
var isScheduleDefect = /* @__PURE__ */ __name((u) => hasProperty(u, ScheduleDefectTypeId), "isScheduleDefect");
var scheduleDefectWrap = /* @__PURE__ */ __name((self) => catchAll(self, (e) => die2(new ScheduleDefect(e))), "scheduleDefectWrap");
var scheduleDefectRefailCause = /* @__PURE__ */ __name((cause3) => match2(find(cause3, (_) => isDieType(_) && isScheduleDefect(_.defect) ? some2(_.defect) : none2()), {
  onNone: /* @__PURE__ */ __name(() => cause3, "onNone"),
  onSome: /* @__PURE__ */ __name((error) => fail(error.error), "onSome")
}), "scheduleDefectRefailCause");
var scheduleDefectRefail = /* @__PURE__ */ __name((effect) => catchAllCause(effect, (cause3) => failCause(scheduleDefectRefailCause(cause3))), "scheduleDefectRefail");
var repeat_Effect = /* @__PURE__ */ dual(2, (self, schedule2) => repeatOrElse_Effect(self, schedule2, (e, _) => fail2(e)));
var repeat_combined = /* @__PURE__ */ dual(2, (self, options) => {
  if (isSchedule(options)) {
    return repeat_Effect(self, options);
  }
  const base = options.schedule ?? passthrough(forever2);
  const withWhile = options.while ? whileInputEffect(base, (a) => {
    const applied = options.while(a);
    if (typeof applied === "boolean") {
      return succeed(applied);
    }
    return scheduleDefectWrap(applied);
  }) : base;
  const withUntil = options.until ? untilInputEffect(withWhile, (a) => {
    const applied = options.until(a);
    if (typeof applied === "boolean") {
      return succeed(applied);
    }
    return scheduleDefectWrap(applied);
  }) : withWhile;
  const withTimes = options.times ? intersect5(withUntil, recurs(options.times)).pipe(map12((intersectionPair) => intersectionPair[0])) : withUntil;
  return scheduleDefectRefail(repeat_Effect(self, withTimes));
});
var repeatOrElse_Effect = /* @__PURE__ */ dual(3, (self, schedule2, orElse3) => flatMap7(driver(schedule2), (driver2) => matchEffect(self, {
  onFailure: /* @__PURE__ */ __name((error) => orElse3(error, none2()), "onFailure"),
  onSuccess: /* @__PURE__ */ __name((value) => repeatOrElseEffectLoop(provideServiceEffect(self, CurrentIterationMetadata, get11(driver2.iterationMeta)), driver2, (error, option3) => provideServiceEffect(orElse3(error, option3), CurrentIterationMetadata, get11(driver2.iterationMeta)), value), "onSuccess")
})));
var repeatOrElseEffectLoop = /* @__PURE__ */ __name((self, driver2, orElse3, value) => matchEffect(driver2.next(value), {
  onFailure: /* @__PURE__ */ __name(() => orDie(driver2.last), "onFailure"),
  onSuccess: /* @__PURE__ */ __name((b) => matchEffect(self, {
    onFailure: /* @__PURE__ */ __name((error) => orElse3(error, some2(b)), "onFailure"),
    onSuccess: /* @__PURE__ */ __name((value2) => repeatOrElseEffectLoop(self, driver2, orElse3, value2), "onSuccess")
  }), "onSuccess")
}), "repeatOrElseEffectLoop");
var retry_Effect = /* @__PURE__ */ dual(2, (self, policy) => retryOrElse_Effect(self, policy, (e, _) => fail2(e)));
var retry_combined = /* @__PURE__ */ dual(2, (self, options) => {
  if (isSchedule(options)) {
    return retry_Effect(self, options);
  }
  return scheduleDefectRefail(retry_Effect(self, fromRetryOptions(options)));
});
var fromRetryOptions = /* @__PURE__ */ __name((options) => {
  const base = options.schedule ?? forever2;
  const withWhile = options.while ? whileInputEffect(base, (e) => {
    const applied = options.while(e);
    if (typeof applied === "boolean") {
      return succeed(applied);
    }
    return scheduleDefectWrap(applied);
  }) : base;
  const withUntil = options.until ? untilInputEffect(withWhile, (e) => {
    const applied = options.until(e);
    if (typeof applied === "boolean") {
      return succeed(applied);
    }
    return scheduleDefectWrap(applied);
  }) : withWhile;
  return options.times !== void 0 ? intersect5(withUntil, recurs(options.times)) : withUntil;
}, "fromRetryOptions");
var retryOrElse_Effect = /* @__PURE__ */ dual(3, (self, policy, orElse3) => flatMap7(driver(policy), (driver2) => retryOrElse_EffectLoop(provideServiceEffect(self, CurrentIterationMetadata, get11(driver2.iterationMeta)), driver2, (e, out) => provideServiceEffect(orElse3(e, out), CurrentIterationMetadata, get11(driver2.iterationMeta)))));
var retryOrElse_EffectLoop = /* @__PURE__ */ __name((self, driver2, orElse3) => {
  return catchAll(self, (e) => matchEffect(driver2.next(e), {
    onFailure: /* @__PURE__ */ __name(() => pipe(driver2.last, orDie, flatMap7((out) => orElse3(e, out))), "onFailure"),
    onSuccess: /* @__PURE__ */ __name(() => retryOrElse_EffectLoop(self, driver2, orElse3), "onSuccess")
  }));
}, "retryOrElse_EffectLoop");
var schedule_Effect = /* @__PURE__ */ dual(2, (self, schedule2) => scheduleFrom_Effect(self, void 0, schedule2));
var scheduleFrom_Effect = /* @__PURE__ */ dual(3, (self, initial, schedule2) => flatMap7(driver(schedule2), (driver2) => scheduleFrom_EffectLoop(provideServiceEffect(self, CurrentIterationMetadata, get11(driver2.iterationMeta)), initial, driver2)));
var scheduleFrom_EffectLoop = /* @__PURE__ */ __name((self, initial, driver2) => matchEffect(driver2.next(initial), {
  onFailure: /* @__PURE__ */ __name(() => orDie(driver2.last), "onFailure"),
  onSuccess: /* @__PURE__ */ __name(() => flatMap7(self, (a) => scheduleFrom_EffectLoop(self, a, driver2)), "onSuccess")
}), "scheduleFrom_EffectLoop");
var forever2 = /* @__PURE__ */ unfold2(0, (n) => n + 1);
var once2 = /* @__PURE__ */ asVoid3(/* @__PURE__ */ recurs(1));
var scheduleForked = /* @__PURE__ */ dual(2, (self, schedule2) => forkScoped(schedule_Effect(self, schedule2)));

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/internal/executionPlan.js
var withExecutionPlan = /* @__PURE__ */ dual(2, (effect, plan) => suspend(() => {
  let i = 0;
  let result;
  return flatMap7(whileLoop({
    while: /* @__PURE__ */ __name(() => i < plan.steps.length && (result === void 0 || isLeft2(result)), "while"),
    body: /* @__PURE__ */ __name(() => {
      const step4 = plan.steps[i];
      let nextEffect = effect_provide(effect, step4.provide);
      if (result) {
        let attempted = false;
        const wrapped = nextEffect;
        nextEffect = suspend(() => {
          if (attempted) return wrapped;
          attempted = true;
          return result;
        });
        nextEffect = scheduleDefectRefail(retry_Effect(nextEffect, scheduleFromStep(step4, false)));
      } else {
        const schedule2 = scheduleFromStep(step4, true);
        nextEffect = schedule2 ? scheduleDefectRefail(retry_Effect(nextEffect, schedule2)) : nextEffect;
      }
      return either2(nextEffect);
    }, "body"),
    step: /* @__PURE__ */ __name((either4) => {
      result = either4;
      i++;
    }, "step")
  }), () => result);
}));
var scheduleFromStep = /* @__PURE__ */ __name((step4, first2) => {
  if (!first2) {
    return fromRetryOptions({
      schedule: step4.schedule ? step4.schedule : step4.attempts ? void 0 : once2,
      times: step4.attempts,
      while: step4.while
    });
  } else if (step4.attempts === 1 || !(step4.schedule || step4.attempts)) {
    return void 0;
  }
  return fromRetryOptions({
    schedule: step4.schedule,
    while: step4.while,
    times: step4.attempts ? step4.attempts - 1 : void 0
  });
}, "scheduleFromStep");

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/internal/query.js
var currentCache = /* @__PURE__ */ globalValue(/* @__PURE__ */ Symbol.for("effect/FiberRef/currentCache"), () => fiberRefUnsafeMake(unsafeMakeWith(65536, () => map8(deferredMake(), (handle) => ({
  listeners: new Listeners(),
  handle
})), () => seconds(60))));
var currentCacheEnabled = /* @__PURE__ */ globalValue(/* @__PURE__ */ Symbol.for("effect/FiberRef/currentCacheEnabled"), () => fiberRefUnsafeMake(false));
var fromRequest = /* @__PURE__ */ __name((request2, dataSource) => flatMap7(isEffect(dataSource) ? dataSource : succeed(dataSource), (ds) => fiberIdWith((id) => {
  const proxy = new Proxy(request2, {});
  return fiberRefGetWith(currentCacheEnabled, (cacheEnabled) => {
    if (cacheEnabled) {
      const cached4 = fiberRefGetWith(currentCache, (cache) => flatMap7(cache.getEither(proxy), (orNew) => {
        switch (orNew._tag) {
          case "Left": {
            if (orNew.left.listeners.interrupted) {
              return flatMap7(cache.invalidateWhen(proxy, (entry) => entry.handle === orNew.left.handle), () => cached4);
            }
            orNew.left.listeners.increment();
            return uninterruptibleMask((restore) => flatMap7(exit(blocked(empty15, restore(deferredAwait(orNew.left.handle)))), (exit4) => {
              orNew.left.listeners.decrement();
              return exit4;
            }));
          }
          case "Right": {
            orNew.right.listeners.increment();
            return uninterruptibleMask((restore) => flatMap7(exit(blocked(single(ds, makeEntry({
              request: proxy,
              result: orNew.right.handle,
              listeners: orNew.right.listeners,
              ownerId: id,
              state: {
                completed: false
              }
            })), restore(deferredAwait(orNew.right.handle)))), () => {
              orNew.right.listeners.decrement();
              return deferredAwait(orNew.right.handle);
            }));
          }
        }
      }));
      return cached4;
    }
    const listeners = new Listeners();
    listeners.increment();
    return flatMap7(deferredMake(), (ref) => ensuring(blocked(single(ds, makeEntry({
      request: proxy,
      result: ref,
      listeners,
      ownerId: id,
      state: {
        completed: false
      }
    })), deferredAwait(ref)), sync(() => listeners.decrement())));
  });
})), "fromRequest");
var cacheRequest = /* @__PURE__ */ __name((request2, result) => {
  return fiberRefGetWith(currentCacheEnabled, (cacheEnabled) => {
    if (cacheEnabled) {
      return fiberRefGetWith(currentCache, (cache) => flatMap7(cache.getEither(request2), (orNew) => {
        switch (orNew._tag) {
          case "Left": {
            return void_;
          }
          case "Right": {
            return deferredComplete(orNew.right.handle, result);
          }
        }
      }));
    }
    return void_;
  });
}, "cacheRequest");
var withRequestCaching = /* @__PURE__ */ dual(2, (self, strategy) => fiberRefLocally(self, currentCacheEnabled, strategy));
var withRequestCache = /* @__PURE__ */ dual(
  2,
  // @ts-expect-error
  (self, cache) => fiberRefLocally(self, currentCache, cache)
);

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/Request.js
var isRequest2 = isRequest;

// ../../node_modules/.bun/effect@3.21.0/node_modules/effect/dist/esm/Effect.js
var EffectTypeId3 = EffectTypeId2;
var isEffect2 = isEffect;
var cachedWithTTL = cached2;
var cachedInvalidateWithTTL2 = cachedInvalidateWithTTL;
var cached3 = memoize;
var cachedFunction2 = cachedFunction;
var once3 = once;
var all4 = all3;
var allWith2 = allWith;
var allSuccesses2 = allSuccesses;
var dropUntil2 = dropUntil;
var dropWhile2 = dropWhile;
var takeUntil2 = takeUntil;
var takeWhile2 = takeWhile;
var every5 = every4;
var exists3 = exists2;
var filter7 = filter5;
var filterMap4 = filterMap3;
var findFirst5 = findFirst3;
var forEach8 = forEach7;
var head4 = head3;
var mergeAll5 = mergeAll3;
var partition4 = partition3;
var reduce11 = reduce8;
var reduceWhile2 = reduceWhile;
var reduceRight3 = reduceRight2;
var reduceEffect2 = reduceEffect;
var replicate2 = replicate;
var replicateEffect2 = replicateEffect;
var validateAll2 = validateAll;
var validateFirst2 = validateFirst;
var async2 = async_;
var asyncEffect2 = asyncEffect;
var custom2 = custom;
var withFiberRuntime2 = withFiberRuntime;
var fail6 = fail2;
var failSync2 = failSync;
var failCause5 = failCause;
var failCauseSync2 = failCauseSync;
var die5 = die2;
var dieMessage2 = dieMessage;
var dieSync2 = dieSync;
var gen2 = gen;
var never2 = never;
var none9 = none6;
var promise2 = promise;
var succeed6 = succeed;
var succeedNone2 = succeedNone;
var succeedSome2 = succeedSome;
var suspend4 = suspend;
var sync4 = sync;
var _void = void_;
var yieldNow4 = yieldNow;
var _catch2 = _catch;
var catchAll2 = catchAll;
var catchAllCause2 = catchAllCause;
var catchAllDefect2 = catchAllDefect;
var catchIf2 = catchIf;
var catchSome2 = catchSome;
var catchSomeCause2 = catchSomeCause;
var catchSomeDefect2 = catchSomeDefect;
var catchTag2 = catchTag;
var catchTags2 = catchTags;
var cause2 = cause;
var eventually2 = eventually;
var ignore2 = ignore;
var ignoreLogged2 = ignoreLogged;
var parallelErrors2 = parallelErrors;
var sandbox2 = sandbox;
var retry = retry_combined;
var withExecutionPlan2 = withExecutionPlan;
var retryOrElse = retryOrElse_Effect;
var try_2 = try_;
var tryMap2 = tryMap;
var tryMapPromise2 = tryMapPromise;
var tryPromise2 = tryPromise;
var unsandbox2 = unsandbox;
var allowInterrupt2 = allowInterrupt;
var checkInterruptible2 = checkInterruptible;
var disconnect2 = disconnect;
var interrupt6 = interrupt2;
var interruptWith2 = interruptWith;
var interruptible4 = interruptible2;
var interruptibleMask2 = interruptibleMask;
var onInterrupt2 = onInterrupt;
var uninterruptible2 = uninterruptible;
var uninterruptibleMask3 = uninterruptibleMask;
var liftPredicate2 = liftPredicate;
var as6 = as2;
var asSome2 = asSome;
var asSomeError2 = asSomeError;
var asVoid4 = asVoid;
var flip2 = flip;
var flipWith2 = flipWith;
var map13 = map8;
var mapAccum3 = mapAccum2;
var mapBoth3 = mapBoth;
var mapError3 = mapError;
var mapErrorCause3 = mapErrorCause2;
var merge6 = merge5;
var negate2 = negate;
var acquireRelease2 = acquireRelease;
var acquireReleaseInterruptible2 = acquireReleaseInterruptible;
var acquireUseRelease2 = acquireUseRelease;
var addFinalizer2 = addFinalizer;
var ensuring2 = ensuring;
var onError2 = onError;
var onExit3 = onExit;
var parallelFinalizers2 = parallelFinalizers;
var sequentialFinalizers2 = sequentialFinalizers;
var finalizersMask2 = finalizersMask;
var scope2 = scope;
var scopeWith2 = scopeWith;
var scopedWith2 = scopedWith;
var scoped2 = scopedEffect;
var using2 = using;
var withEarlyRelease2 = withEarlyRelease;
var awaitAllChildren2 = awaitAllChildren;
var daemonChildren2 = daemonChildren;
var descriptor2 = descriptor;
var descriptorWith2 = descriptorWith;
var diffFiberRefs2 = diffFiberRefs;
var ensuringChild2 = ensuringChild;
var ensuringChildren2 = ensuringChildren;
var fiberId2 = fiberId;
var fiberIdWith2 = fiberIdWith;
var fork3 = fork;
var forkDaemon2 = forkDaemon;
var forkAll2 = forkAll;
var forkIn2 = forkIn;
var forkScoped2 = forkScoped;
var forkWithErrorHandler2 = forkWithErrorHandler;
var fromFiber2 = fromFiber;
var fromFiberEffect2 = fromFiberEffect;
var supervised2 = supervised;
var transplant2 = transplant;
var withConcurrency2 = withConcurrency;
var withScheduler2 = withScheduler;
var withSchedulingPriority2 = withSchedulingPriority;
var withMaxOpsBeforeYield2 = withMaxOpsBeforeYield;
var clock2 = clock;
var clockWith4 = clockWith3;
var withClockScoped2 = withClockScoped;
var withClock2 = withClock;
var console3 = console2;
var consoleWith2 = consoleWith;
var withConsoleScoped2 = withConsoleScoped;
var withConsole2 = withConsole;
var delay2 = delay;
var sleep4 = sleep3;
var timed2 = timed;
var timedWith2 = timedWith;
var timeout2 = timeout;
var timeoutOption2 = timeoutOption;
var timeoutFail2 = timeoutFail;
var timeoutFailCause2 = timeoutFailCause;
var timeoutTo2 = timeoutTo;
var configProviderWith2 = configProviderWith;
var withConfigProvider2 = withConfigProvider;
var withConfigProviderScoped2 = withConfigProviderScoped;
var context3 = context;
var contextWith2 = contextWith;
var contextWithEffect2 = contextWithEffect;
var mapInputContext2 = mapInputContext;
var provide2 = effect_provide;
var provideService2 = provideService;
var provideServiceEffect2 = provideServiceEffect;
var serviceFunction2 = serviceFunction;
var serviceFunctionEffect2 = serviceFunctionEffect;
var serviceFunctions2 = serviceFunctions;
var serviceConstants2 = serviceConstants;
var serviceMembers2 = serviceMembers;
var serviceOption2 = serviceOption;
var serviceOptional2 = serviceOptional;
var updateService2 = updateService;
var Do2 = Do;
var bind3 = bind2;
var bindAll2 = bindAll;
var bindTo3 = bindTo2;
var let_3 = let_2;
var option2 = option;
var either3 = either2;
var exit3 = exit;
var intoDeferred2 = intoDeferred;
var if_2 = if_;
var filterOrDie2 = filterOrDie;
var filterOrDieMessage2 = filterOrDieMessage;
var filterOrElse2 = filterOrElse;
var filterOrFail2 = filterOrFail;
var filterEffectOrElse2 = filterEffectOrElse;
var filterEffectOrFail2 = filterEffectOrFail;
var unless2 = unless;
var unlessEffect2 = unlessEffect;
var when2 = when;
var whenEffect2 = whenEffect;
var whenFiberRef2 = whenFiberRef;
var whenRef2 = whenRef;
var flatMap11 = flatMap7;
var andThen5 = andThen3;
var flatten7 = flatten4;
var race2 = race;
var raceAll2 = raceAll;
var raceFirst2 = raceFirst;
var raceWith2 = raceWith;
var summarized2 = summarized;
var tap2 = tap;
var tapBoth2 = tapBoth;
var tapDefect2 = tapDefect;
var tapError2 = tapError;
var tapErrorTag2 = tapErrorTag;
var tapErrorCause2 = tapErrorCause;
var forever3 = forever;
var iterate2 = iterate;
var loop2 = loop;
var repeat = repeat_combined;
var repeatN2 = repeatN;
var repeatOrElse = repeatOrElse_Effect;
var schedule = schedule_Effect;
var scheduleForked2 = scheduleForked;
var scheduleFrom = scheduleFrom_Effect;
var whileLoop3 = whileLoop;
var getFiberRefs = fiberRefs2;
var inheritFiberRefs2 = inheritFiberRefs;
var locally = fiberRefLocally;
var locallyWith = fiberRefLocallyWith;
var locallyScoped = fiberRefLocallyScoped;
var locallyScopedWith = fiberRefLocallyScopedWith;
var patchFiberRefs2 = patchFiberRefs;
var setFiberRefs2 = setFiberRefs;
var updateFiberRefs2 = updateFiberRefs;
var isFailure5 = isFailure3;
var isSuccess3 = isSuccess2;
var match11 = match7;
var matchCause3 = matchCause;
var matchCauseEffect3 = matchCauseEffect;
var matchEffect3 = matchEffect;
var log2 = log;
var logWithLevel2 = /* @__PURE__ */ __name((level, ...message) => logWithLevel(level)(...message), "logWithLevel");
var logTrace2 = logTrace;
var logDebug2 = logDebug;
var logInfo2 = logInfo;
var logWarning2 = logWarning;
var logError2 = logError;
var logFatal2 = logFatal;
var withLogSpan2 = withLogSpan;
var annotateLogs2 = annotateLogs;
var annotateLogsScoped2 = annotateLogsScoped;
var logAnnotations2 = logAnnotations;
var withUnhandledErrorLogLevel2 = withUnhandledErrorLogLevel;
var whenLogLevel2 = whenLogLevel;
var orDie2 = orDie;
var orDieWith2 = orDieWith;
var orElse2 = orElse;
var orElseFail2 = orElseFail;
var orElseSucceed2 = orElseSucceed;
var firstSuccessOf2 = firstSuccessOf;
var random3 = random2;
var randomWith2 = randomWith;
var withRandom2 = withRandom;
var withRandomFixed = /* @__PURE__ */ dual(2, (effect, values3) => withRandom2(effect, fixed2(values3)));
var withRandomScoped2 = withRandomScoped;
var runtime3 = runtime2;
var getRuntimeFlags = runtimeFlags;
var patchRuntimeFlags = updateRuntimeFlags;
var withRuntimeFlagsPatch = withRuntimeFlags;
var withRuntimeFlagsPatchScoped = withRuntimeFlagsScoped;
var tagMetrics2 = tagMetrics;
var labelMetrics2 = labelMetrics;
var tagMetricsScoped2 = tagMetricsScoped;
var labelMetricsScoped2 = labelMetricsScoped;
var metricLabels2 = metricLabels;
var withMetric2 = withMetric;
var unsafeMakeSemaphore2 = unsafeMakeSemaphore;
var makeSemaphore2 = makeSemaphore;
var unsafeMakeLatch2 = unsafeMakeLatch;
var makeLatch2 = makeLatch;
var runFork2 = unsafeForkEffect;
var runCallback = unsafeRunEffect;
var runPromise = unsafeRunPromiseEffect;
var runPromiseExit = unsafeRunPromiseExitEffect;
var runSync = unsafeRunSyncEffect;
var runSyncExit = unsafeRunSyncExitEffect;
var validate2 = validate;
var validateWith2 = validateWith;
var zip5 = zipOptions;
var zipLeft3 = zipLeftOptions;
var zipRight3 = zipRightOptions;
var zipWith4 = zipWithOptions;
var ap = /* @__PURE__ */ dual(2, (self, that) => zipWith4(self, that, (f, a) => f(a)));
var blocked2 = blocked;
var runRequestBlock2 = runRequestBlock;
var step3 = step2;
var request = /* @__PURE__ */ dual((args2) => isRequest2(args2[0]), fromRequest);
var cacheRequestResult = cacheRequest;
var withRequestBatching2 = withRequestBatching;
var withRequestCaching2 = withRequestCaching;
var withRequestCache2 = withRequestCache;
var tracer2 = tracer;
var tracerWith4 = tracerWith;
var withTracer2 = withTracer;
var withTracerScoped2 = withTracerScoped;
var withTracerEnabled2 = withTracerEnabled;
var withTracerTiming2 = withTracerTiming;
var annotateSpans2 = annotateSpans;
var annotateCurrentSpan2 = annotateCurrentSpan;
var currentSpan2 = currentSpan;
var currentPropagatedSpan2 = currentPropagatedSpan;
var currentParentSpan2 = currentParentSpan;
var spanAnnotations2 = spanAnnotations;
var spanLinks2 = spanLinks;
var linkSpans2 = linkSpans;
var linkSpanCurrent2 = linkSpanCurrent;
var makeSpan2 = makeSpan;
var makeSpanScoped2 = makeSpanScoped;
var useSpan2 = useSpan;
var withSpan2 = withSpan;
var functionWithSpan2 = functionWithSpan;
var withSpanScoped2 = withSpanScoped;
var withParentSpan2 = withParentSpan;
var fromNullable3 = fromNullable2;
var optionFromOptional2 = optionFromOptional;
var transposeOption = /* @__PURE__ */ __name((self) => {
  return isNone(self) ? succeedNone2 : map13(self.value, some);
}, "transposeOption");
var transposeMapOption = /* @__PURE__ */ dual(2, (self, f) => isNone(self) ? succeedNone2 : map13(f(self.value), some));
var makeTagProxy = /* @__PURE__ */ __name((TagClass) => {
  const cache = /* @__PURE__ */ new Map();
  return new Proxy(TagClass, {
    get(target, prop, receiver) {
      if (prop in target) {
        return Reflect.get(target, prop, receiver);
      }
      if (cache.has(prop)) {
        return cache.get(prop);
      }
      const fn2 = /* @__PURE__ */ __name((...args2) => andThen3(target, (s) => {
        if (typeof s[prop] === "function") {
          cache.set(prop, (...args3) => andThen3(target, (s2) => s2[prop](...args3)));
          return s[prop](...args2);
        }
        cache.set(prop, andThen3(target, (s2) => s2[prop]));
        return s[prop];
      }), "fn");
      const cn = andThen3(target, (s) => s[prop]);
      Object.assign(fn2, cn);
      const apply = fn2.apply;
      const bind4 = fn2.bind;
      const call = fn2.call;
      const proto4 = Object.setPrototypeOf({}, Object.getPrototypeOf(cn));
      proto4.apply = apply;
      proto4.bind = bind4;
      proto4.call = call;
      Object.setPrototypeOf(fn2, proto4);
      cache.set(prop, fn2);
      return fn2;
    }
  });
}, "makeTagProxy");
var Tag2 = /* @__PURE__ */ __name((id) => () => {
  const limit = Error.stackTraceLimit;
  Error.stackTraceLimit = 2;
  const creationError = new Error();
  Error.stackTraceLimit = limit;
  function TagClass() {
  }
  __name(TagClass, "TagClass");
  Object.setPrototypeOf(TagClass, TagProto);
  TagClass.key = id;
  Object.defineProperty(TagClass, "use", {
    get() {
      return (body) => andThen3(this, body);
    }
  });
  Object.defineProperty(TagClass, "stack", {
    get() {
      return creationError.stack;
    }
  });
  return makeTagProxy(TagClass);
}, "Tag");
var Service = /* @__PURE__ */ __name(function() {
  return function() {
    const [id, maker] = arguments;
    const proxy = "accessors" in maker ? maker["accessors"] : false;
    const limit = Error.stackTraceLimit;
    Error.stackTraceLimit = 2;
    const creationError = new Error();
    Error.stackTraceLimit = limit;
    let patchState = "unchecked";
    const TagClass = /* @__PURE__ */ __name(function(service) {
      if (patchState === "unchecked") {
        const proto4 = Object.getPrototypeOf(service);
        if (proto4 === Object.prototype || proto4 === null) {
          patchState = "plain";
        } else {
          const selfProto = Object.getPrototypeOf(this);
          Object.setPrototypeOf(selfProto, proto4);
          patchState = "patched";
        }
      }
      if (patchState === "plain") {
        Object.assign(this, service);
      } else if (patchState === "patched") {
        Object.setPrototypeOf(service, Object.getPrototypeOf(this));
        return service;
      }
    }, "TagClass");
    TagClass.prototype._tag = id;
    Object.defineProperty(TagClass, "make", {
      get() {
        return (service) => new this(service);
      }
    });
    Object.defineProperty(TagClass, "use", {
      get() {
        return (body) => andThen3(this, body);
      }
    });
    TagClass.key = id;
    Object.assign(TagClass, TagProto);
    Object.defineProperty(TagClass, "stack", {
      get() {
        return creationError.stack;
      }
    });
    const hasDeps = "dependencies" in maker && maker.dependencies.length > 0;
    const layerName = hasDeps ? "DefaultWithoutDependencies" : "Default";
    let layerCache;
    let isFunction3 = false;
    if ("effect" in maker) {
      isFunction3 = typeof maker.effect === "function";
      Object.defineProperty(TagClass, layerName, {
        get() {
          if (isFunction3) {
            return function() {
              return fromEffect2(TagClass, map13(maker.effect.apply(null, arguments), (_) => new this(_)));
            }.bind(this);
          }
          return layerCache ??= fromEffect2(TagClass, map13(maker.effect, (_) => new this(_)));
        }
      });
    } else if ("scoped" in maker) {
      isFunction3 = typeof maker.scoped === "function";
      Object.defineProperty(TagClass, layerName, {
        get() {
          if (isFunction3) {
            return function() {
              return scoped(TagClass, map13(maker.scoped.apply(null, arguments), (_) => new this(_)));
            }.bind(this);
          }
          return layerCache ??= scoped(TagClass, map13(maker.scoped, (_) => new this(_)));
        }
      });
    } else if ("sync" in maker) {
      Object.defineProperty(TagClass, layerName, {
        get() {
          return layerCache ??= sync3(TagClass, () => new this(maker.sync()));
        }
      });
    } else {
      Object.defineProperty(TagClass, layerName, {
        get() {
          return layerCache ??= succeed4(TagClass, new this(maker.succeed));
        }
      });
    }
    if (hasDeps) {
      let layerWithDepsCache;
      Object.defineProperty(TagClass, "Default", {
        get() {
          if (isFunction3) {
            return function() {
              return provide(this.DefaultWithoutDependencies.apply(null, arguments), maker.dependencies);
            };
          }
          return layerWithDepsCache ??= provide(this.DefaultWithoutDependencies, maker.dependencies);
        }
      });
    }
    return proxy === true ? makeTagProxy(TagClass) : TagClass;
  };
}, "Service");
var fn = /* @__PURE__ */ __name(function(nameOrBody, ...pipeables) {
  const limit = Error.stackTraceLimit;
  Error.stackTraceLimit = 2;
  const errorDef = new Error();
  Error.stackTraceLimit = limit;
  if (typeof nameOrBody !== "string") {
    return defineLength(nameOrBody.length, function(...args2) {
      const limit2 = Error.stackTraceLimit;
      Error.stackTraceLimit = 2;
      const errorCall = new Error();
      Error.stackTraceLimit = limit2;
      return fnApply({
        self: this,
        body: nameOrBody,
        args: args2,
        pipeables,
        spanName: "<anonymous>",
        spanOptions: {
          context: DisablePropagation.context(true)
        },
        errorDef,
        errorCall
      });
    });
  }
  const name = nameOrBody;
  const options = pipeables[0];
  return (body, ...pipeables2) => defineLength(body.length, {
    [name](...args2) {
      const limit2 = Error.stackTraceLimit;
      Error.stackTraceLimit = 2;
      const errorCall = new Error();
      Error.stackTraceLimit = limit2;
      return fnApply({
        self: this,
        body,
        args: args2,
        pipeables: pipeables2,
        spanName: name,
        spanOptions: options,
        errorDef,
        errorCall
      });
    }
  }[name]);
}, "fn");
function defineLength(length2, fn2) {
  return Object.defineProperty(fn2, "length", {
    value: length2,
    configurable: true
  });
}
__name(defineLength, "defineLength");
function fnApply(options) {
  let effect;
  let fnError = void 0;
  if (isGeneratorFunction(options.body)) {
    effect = fromIterator(() => options.body.apply(options.self, options.args));
  } else {
    try {
      effect = options.body.apply(options.self, options.args);
    } catch (error) {
      fnError = error;
      effect = die5(error);
    }
  }
  if (options.pipeables.length > 0) {
    try {
      for (const x of options.pipeables) {
        effect = x(effect, ...options.args);
      }
    } catch (error) {
      effect = fnError ? failCause5(sequential(die(fnError), die(error))) : die5(error);
    }
  }
  let cache = false;
  const captureStackTrace = /* @__PURE__ */ __name(() => {
    if (cache !== false) {
      return cache;
    }
    if (options.errorCall.stack) {
      const stackDef = options.errorDef.stack.trim().split("\n");
      const stackCall = options.errorCall.stack.trim().split("\n");
      let endStackDef = stackDef.slice(2).join("\n").trim();
      if (!endStackDef.includes(`(`)) {
        endStackDef = endStackDef.replace(/at (.*)/, "at ($1)");
      }
      let endStackCall = stackCall.slice(2).join("\n").trim();
      if (!endStackCall.includes(`(`)) {
        endStackCall = endStackCall.replace(/at (.*)/, "at ($1)");
      }
      cache = `${endStackDef}
${endStackCall}`;
      return cache;
    }
  }, "captureStackTrace");
  const opts = options.spanOptions && "captureStackTrace" in options.spanOptions ? options.spanOptions : {
    captureStackTrace,
    ...options.spanOptions
  };
  return withSpan2(effect, options.spanName, opts);
}
__name(fnApply, "fnApply");
var fnUntraced2 = fnUntraced;
var ensureSuccessType = /* @__PURE__ */ __name(() => (effect) => effect, "ensureSuccessType");
var ensureErrorType = /* @__PURE__ */ __name(() => (effect) => effect, "ensureErrorType");
var ensureRequirementsType = /* @__PURE__ */ __name(() => (effect) => effect, "ensureRequirementsType");

// src/types.ts
var runtimeContractVersion = "v1";
function asRecord(value) {
  if (value && typeof value === "object" && !Array.isArray(value)) {
    return value;
  }
  return {};
}
__name(asRecord, "asRecord");

// src/core.ts
var defaultDependencies = {
  fetch: globalThis.fetch.bind(globalThis)
};
function resolveParentStepRef(payload, slug) {
  if (typeof payload.parentStepRef === "string" && payload.parentStepRef.trim().length > 0) {
    return payload.parentStepRef.trim();
  }
  return slug === "planner" ? "planner" : "plan";
}
__name(resolveParentStepRef, "resolveParentStepRef");
function toError(error) {
  return error instanceof Error ? error : new Error(String(error));
}
__name(toError, "toError");
function assertEnvelope(value) {
  if (!value || typeof value !== "object") {
    throw new Error("dispatch envelope must be an object");
  }
  const envelope = value;
  if (typeof envelope.version !== "string" || envelope.version.length === 0) {
    throw new Error("dispatch envelope version is required");
  }
  if (!envelope.run || typeof envelope.run.id !== "string" || typeof envelope.run.project_id !== "string") {
    throw new Error("dispatch envelope run is invalid");
  }
  if (!envelope.agent || typeof envelope.agent.id !== "string" || typeof envelope.agent.slug !== "string" || typeof envelope.agent.model !== "string") {
    throw new Error("dispatch envelope agent is invalid");
  }
  if (!envelope.deployment || typeof envelope.deployment.id !== "string" || typeof envelope.deployment.provider !== "string") {
    throw new Error("dispatch envelope deployment is invalid");
  }
  if (!envelope.callback || typeof envelope.callback.base_url !== "string" || typeof envelope.callback.run_id !== "string" || typeof envelope.callback.run_token !== "string") {
    throw new Error("dispatch envelope callback is invalid");
  }
  return envelope;
}
__name(assertEnvelope, "assertEnvelope");
function assertStringArray(value, field) {
  if (value == null) {
    return void 0;
  }
  if (!Array.isArray(value)) {
    throw new Error(`${field} must be an array`);
  }
  return value.map((item, index) => {
    if (typeof item !== "string" || item.trim().length === 0) {
      throw new Error(`${field}[${index}] must be a non-empty string`);
    }
    return item.trim();
  });
}
__name(assertStringArray, "assertStringArray");
function resolveRuntimeMode(payload) {
  const rawMode = payload._mode ?? payload.mode;
  if (rawMode === "dynamic_planner") {
    return "dynamic_planner";
  }
  if (rawMode === "synthesizer") {
    return "synthesizer";
  }
  if (rawMode === "worker") {
    return "worker";
  }
  return "generic";
}
__name(resolveRuntimeMode, "resolveRuntimeMode");
function createWorkerPayload(payload, index) {
  const baseLens = payload.workerLenses;
  const lens = Array.isArray(baseLens) && typeof baseLens[index] === "string" ? baseLens[index] : `track-${index + 1}`;
  return {
    lens,
    topic: payload.topic ?? "unknown-topic"
  };
}
__name(createWorkerPayload, "createWorkerPayload");
function createPlannerTasks(payload) {
  const workerAgentIds = assertStringArray(
    payload.workerAgentIds,
    "workerAgentIds"
  );
  if (workerAgentIds == null || workerAgentIds.length === 0) {
    throw new Error("dynamic_planner mode requires workerAgentIds");
  }
  return workerAgentIds.map((agentId, index) => ({
    agentId,
    payload: createWorkerPayload(payload, index),
    stepRef: `worker-${index + 1}`
  }));
}
__name(createPlannerTasks, "createPlannerTasks");
function buildResult(envelope, payload) {
  const mode = resolveRuntimeMode(payload);
  if (mode === "dynamic_planner") {
    const tasks = createPlannerTasks(payload);
    const parentStepRef = resolveParentStepRef(payload, envelope.agent.slug);
    const synthesisAgentId = typeof payload.synthesizerAgentId === "string" && payload.synthesizerAgentId.trim().length > 0 ? payload.synthesizerAgentId.trim() : "agent-synthesizer";
    return {
      contract_version: runtimeContractVersion,
      dynamic_steps: [
        ...tasks.map((task) => {
          const step4 = {
            agent_id: task.agentId,
            depends_on: [parentStepRef],
            step_ref: task.stepRef
          };
          if (task.payload != null) {
            step4.payload = task.payload;
          }
          return step4;
        }),
        {
          agent_id: synthesisAgentId,
          depends_on: tasks.map((task) => task.stepRef),
          payload: {
            summary_style: payload.summaryStyle ?? "brief",
            topic: payload.topic ?? "unknown-topic"
          },
          step_ref: "synthesis"
        }
      ],
      plan_summary: `planned ${tasks.length} dynamic worker steps`,
      run_id: envelope.run.id
    };
  }
  if (mode === "worker") {
    return {
      contract_version: runtimeContractVersion,
      finding: `Investigated ${String(payload.topic ?? "unknown-topic")} via ${String(payload.lens ?? "general")}.`,
      run_id: envelope.run.id
    };
  }
  if (mode === "synthesizer") {
    return {
      contract_version: runtimeContractVersion,
      run_id: envelope.run.id,
      summary: `Prepared a ${String(payload.summaryStyle ?? "brief")} summary for ${String(payload.topic ?? "unknown-topic")}.`
    };
  }
  return {
    ok: true,
    contract_version: runtimeContractVersion,
    agent_id: envelope.agent.id,
    run_id: envelope.run.id,
    payload
  };
}
__name(buildResult, "buildResult");
function buildCheckpointState(envelope, payload) {
  const mode = resolveRuntimeMode(payload);
  if (mode === "dynamic_planner") {
    return {
      agent_id: envelope.agent.id,
      phase: "planning",
      planned_workers: assertStringArray(
        payload.workerAgentIds,
        "workerAgentIds"
      )?.length ?? 0
    };
  }
  if (mode === "worker") {
    return {
      agent_id: envelope.agent.id,
      lens: payload.lens ?? "general",
      phase: "executing"
    };
  }
  if (mode === "synthesizer") {
    return {
      agent_id: envelope.agent.id,
      phase: "synthesizing",
      topic: payload.topic ?? "unknown-topic"
    };
  }
  return {
    agent_id: envelope.agent.id,
    deployment_id: envelope.deployment.id,
    phase: "planning"
  };
}
__name(buildCheckpointState, "buildCheckpointState");
function parseEnvelope(raw) {
  return Effect_exports.try({
    try: /* @__PURE__ */ __name(() => assertEnvelope(JSON.parse(raw)), "try"),
    catch: toError
  });
}
__name(parseEnvelope, "parseEnvelope");
function serializeOutputLine(output) {
  return output.kind === "raw" ? output.line : JSON.stringify(output.event);
}
__name(serializeOutputLine, "serializeOutputLine");
function buildRuntimeOutput(envelope, deps = defaultDependencies) {
  const payload = asRecord(envelope.payload);
  const config = asRecord(envelope.agent.config);
  const mergedPayload = {
    ...config,
    ...payload
  };
  const scenario = String(mergedPayload._scenario ?? "success");
  const delayMs = Number(mergedPayload._delay_ms ?? 0);
  return Effect_exports.gen(function* () {
    if (delayMs > 0) {
      yield* Effect_exports.sleep(Duration_exports.millis(delayMs));
    }
    if (scenario === "invalid_json") {
      return [
        {
          kind: "raw",
          line: "{not-json}"
        }
      ];
    }
    const outputs = [
      {
        kind: "event",
        event: {
          type: "stream",
          stream_id: "default",
          chunk: `agent:${envelope.agent.slug}:thinking `
        }
      },
      {
        kind: "event",
        event: {
          type: "checkpoint",
          state: buildCheckpointState(envelope, mergedPayload)
        }
      }
    ];
    const networkToolCall = yield* buildNetworkToolCall(
      envelope,
      mergedPayload,
      deps
    );
    if (networkToolCall) {
      outputs.push(networkToolCall);
    }
    if (scenario === "duplicate_checkpoint") {
      outputs.push({
        kind: "event",
        event: {
          type: "checkpoint",
          state: {
            duplicate: true,
            phase: "planning"
          }
        }
      });
    }
    outputs.push(
      {
        kind: "event",
        event: {
          type: "tool_call",
          tool_name: "local.echo",
          input: {
            payload: mergedPayload
          },
          output: {
            echoed: mergedPayload.prompt ?? mergedPayload.input ?? mergedPayload.topic ?? "ok"
          },
          duration_ms: 5,
          status: "completed"
        }
      },
      {
        kind: "event",
        event: {
          type: "usage",
          provider: "local",
          model: envelope.agent.model || "local-agent",
          prompt_tokens: 12,
          completion_tokens: 8,
          total_tokens: 20,
          cost_microusd: 200
        }
      },
      {
        kind: "event",
        event: {
          type: "stream",
          stream_id: "default",
          chunk: "done",
          done: true
        }
      }
    );
    if (scenario === "disconnect") {
      return outputs;
    }
    if (scenario === "fail") {
      outputs.push({
        kind: "event",
        event: {
          type: "fail",
          error: String(mergedPayload._error ?? "runtime requested failure")
        }
      });
      return outputs;
    }
    outputs.push({
      kind: "event",
      event: {
        type: "complete",
        result: buildResult(envelope, mergedPayload)
      }
    });
    return outputs;
  }).pipe(Effect_exports.mapError(toError));
}
__name(buildRuntimeOutput, "buildRuntimeOutput");
function parseSandboxPolicy(envelope) {
  const raw = envelope.deployment.sandbox_policy;
  if (!raw || typeof raw !== "object" || Array.isArray(raw)) {
    return null;
  }
  return raw;
}
__name(parseSandboxPolicy, "parseSandboxPolicy");
function buildNetworkToolCall(envelope, payload, deps) {
  const targetURL = payload._network_url;
  if (typeof targetURL !== "string" || targetURL.trim().length === 0) {
    return Effect_exports.succeed(null);
  }
  return Effect_exports.tryPromise({
    try: /* @__PURE__ */ __name(async () => {
      const response = await deps.fetch(targetURL, { method: "GET" });
      const bodyPreview = (await response.text()).slice(0, 256);
      const outboundStatus = response.headers.get("x-strait-outbound-status") ?? "allowed";
      const blocked3 = outboundStatus === "blocked";
      const policy = parseSandboxPolicy(envelope);
      return {
        kind: "event",
        event: {
          type: "tool_call",
          tool_name: "sandbox.fetch",
          input: {
            network_class: policy?.network_class ?? null,
            policy_tag: policy?.policy_tag ?? null,
            url: targetURL
          },
          output: {
            body_preview: bodyPreview,
            outbound_reason: response.headers.get("x-strait-outbound-reason") ?? null,
            status_code: response.status,
            url: targetURL
          },
          duration_ms: 5,
          status: blocked3 ? "blocked" : "completed"
        }
      };
    }, "try"),
    catch: toError
  }).pipe(
    Effect_exports.catchAll(
      (error) => Effect_exports.succeed({
        kind: "event",
        event: {
          type: "tool_call",
          tool_name: "sandbox.fetch",
          input: {
            url: targetURL
          },
          output: {
            error: error.message,
            url: targetURL
          },
          duration_ms: 5,
          status: "failed"
        }
      })
    )
  );
}
__name(buildNetworkToolCall, "buildNetworkToolCall");
function buildNDJSONResponseBody(outputs) {
  if (outputs.length === 0) {
    return "";
  }
  return `${outputs.map(serializeOutputLine).join("\n")}
`;
}
__name(buildNDJSONResponseBody, "buildNDJSONResponseBody");

// src/worker.ts
var defaultDeps = {
  fetch: globalThis.fetch.bind(globalThis)
};
function jsonResponse(status, body) {
  return new Response(JSON.stringify(body), {
    status,
    headers: {
      "cache-control": "no-store",
      "content-type": "application/json; charset=utf-8"
    }
  });
}
__name(jsonResponse, "jsonResponse");
function readBearerToken(request2) {
  const authorization = request2.headers.get("authorization")?.trim() ?? "";
  if (!authorization.toLowerCase().startsWith("bearer ")) {
    return "";
  }
  return authorization.slice("Bearer ".length).trim();
}
__name(readBearerToken, "readBearerToken");
function verifyRuntimeWorkerAuth(request2, env) {
  return Effect_exports.sync(() => {
    const expected = env.AGENT_RUNTIME_AUTH_TOKEN?.trim();
    if (!expected) {
      throw new Error("AGENT_RUNTIME_AUTH_TOKEN is required");
    }
    const received = readBearerToken(request2);
    if (!received) {
      throw new Error("authorization bearer token is required");
    }
    if (received !== expected) {
      throw new Error("authorization bearer token is invalid");
    }
  });
}
__name(verifyRuntimeWorkerAuth, "verifyRuntimeWorkerAuth");
function statusForWorkerError(error) {
  if (error.message.includes("authorization")) {
    return 401;
  }
  if (error.message.includes("AGENT_RUNTIME_AUTH_TOKEN")) {
    return 500;
  }
  if (error.message.includes("dispatch envelope")) {
    return 400;
  }
  return 422;
}
__name(statusForWorkerError, "statusForWorkerError");
function toError2(error) {
  return error instanceof Error ? error : new Error(String(error));
}
__name(toError2, "toError");
async function handleWorkerFetch(request2, env, deps = defaultDeps) {
  if (request2.method !== "POST") {
    return jsonResponse(405, {
      error: "method_not_allowed",
      message: "runtime worker only accepts POST requests"
    });
  }
  const program = verifyRuntimeWorkerAuth(request2, env).pipe(
    Effect_exports.flatMap(
      () => Effect_exports.tryPromise({
        try: /* @__PURE__ */ __name(() => request2.text(), "try"),
        catch: /* @__PURE__ */ __name((error) => error instanceof Error ? error : new Error(String(error)), "catch")
      })
    ),
    Effect_exports.flatMap(parseEnvelope),
    Effect_exports.flatMap((envelope) => buildRuntimeOutput(envelope, deps)),
    Effect_exports.map((outputs) => buildNDJSONResponseBody(outputs))
  );
  const exit4 = await Effect_exports.runPromiseExit(program);
  if (Exit_exports.isFailure(exit4)) {
    const error = toError2(Cause_exports.squash(exit4.cause));
    return jsonResponse(statusForWorkerError(error), {
      error: "runtime_worker_error",
      message: error.message
    });
  }
  return new Response(exit4.value, {
    status: 200,
    headers: {
      "cache-control": "no-store",
      "content-type": "application/x-ndjson; charset=utf-8"
    }
  });
}
__name(handleWorkerFetch, "handleWorkerFetch");
var worker = {
  fetch(request2, env) {
    return handleWorkerFetch(request2, env);
  }
};
var worker_default = worker;
export {
  worker_default as default,
  handleWorkerFetch,
  verifyRuntimeWorkerAuth
};
//# sourceMappingURL=worker.js.map
