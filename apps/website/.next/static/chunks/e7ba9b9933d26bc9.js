(globalThis.TURBOPACK||(globalThis.TURBOPACK=[])).push(["object"==typeof document?document.currentScript:void 0,43685,(e,t,r)=>{t.exports=e.r(68200)},39477,(e,t,r)=>{var o=0/0,a=/^\s+|\s+$/g,n=/^[-+]0x[0-9a-f]+$/i,i=/^0b[01]+$/i,l=/^0o[0-7]+$/i,s=parseInt,p=e.g&&e.g.Object===Object&&e.g,d="object"==typeof self&&self&&self.Object===Object&&self,b=p||d||Function("return this")(),c=Object.prototype.toString,f=Math.max,u=Math.min,_=function(){return b.Date.now()};function h(e){var t=typeof e;return!!e&&("object"==t||"function"==t)}function y(e){if("number"==typeof e)return e;if("symbol"==typeof(t=e)||t&&"object"==typeof t&&"[object Symbol]"==c.call(t))return o;if(h(e)){var t,r="function"==typeof e.valueOf?e.valueOf():e;e=h(r)?r+"":r}if("string"!=typeof e)return 0===e?e:+e;e=e.replace(a,"");var p=i.test(e);return p||l.test(e)?s(e.slice(2),p?2:8):n.test(e)?o:+e}t.exports=function(e,t,r){var o,a,n,i,l,s,p=0,d=!1,b=!1,c=!0;if("function"!=typeof e)throw TypeError("Expected a function");function g(t){var r=o,n=a;return o=a=void 0,p=t,i=e.apply(n,r)}function w(e){var r=e-s,o=e-p;return void 0===s||r>=t||r<0||b&&o>=n}function m(){var e,r,o,a=_();if(w(a))return x(a);l=setTimeout(m,(e=a-s,r=a-p,o=t-e,b?u(o,n-r):o))}function x(e){return(l=void 0,c&&o)?g(e):(o=a=void 0,i)}function v(){var e,r=_(),n=w(r);if(o=arguments,a=this,s=r,n){if(void 0===l)return p=e=s,l=setTimeout(m,t),d?g(e):i;if(b)return l=setTimeout(m,t),g(s)}return void 0===l&&(l=setTimeout(m,t)),i}return t=y(t)||0,h(r)&&(d=!!r.leading,n=(b="maxWait"in r)?f(y(r.maxWait)||0,t):n,c="trailing"in r?!!r.trailing:c),v.cancel=function(){void 0!==l&&clearTimeout(l),p=0,o=s=a=l=void 0},v.flush=function(){return void 0===l?i:x(_())},v}},45866,e=>{"use strict";var t=e.i(82117),r=e.i(39477),o=e.i(69137),a=e.i(43685),n="dffb3111f2dbe90df2c9f44aa745e1eaea5704ee2372695a11b6c736b47349b7",i=`._wrapper_ypbb5_1 {
  box-sizing: border-box;
  font-size: 16px;
}
._wrapper_ypbb5_1 *,
._wrapper_ypbb5_1 *:before,
._wrapper_ypbb5_1 *:after {
  box-sizing: inherit;
}
._wrapper_ypbb5_1 h1,
._wrapper_ypbb5_1 h2,
._wrapper_ypbb5_1 h3,
._wrapper_ypbb5_1 h4,
._wrapper_ypbb5_1 h5,
._wrapper_ypbb5_1 h6,
._wrapper_ypbb5_1 p,
._wrapper_ypbb5_1 ol,
._wrapper_ypbb5_1 ul {
  margin: 0;
  padding: 0;
  font-weight: normal;
}
._wrapper_ypbb5_1 ol,
._wrapper_ypbb5_1 ul {
  list-style: none;
}
._wrapper_ypbb5_1 img {
  max-width: 100%;
  height: auto;
}

._branch_ypbb5_32 {
  padding-left: 9px;
  padding-right: 12px;
  height: 100%;
  display: flex;
  align-items: center;
  font-weight: 500;
  user-select: none;
}

._wrapper_ypbb5_1 {
  position: fixed;
  bottom: 32px;
  right: 32px;
  background: #0c0c0c;
  z-index: 1000;
  border-radius: 7px;
  animation: _in_ypbb5_1 0.3s ease-out;
  display: flex;
}

._root_ypbb5_53 {
  --font-family: Inter, Segoe UI, Roboto, sans-serif, Apple Color Emoji,
    Segoe UI Emoji, Segoe UI Symbol, Noto Color Emoji, sans-serif;
  border-radius: 6px;
  height: 36px;
  color: white;
  display: flex;
  border: 1px solid #303030;
  font-family: var(--font-family);
}
._root_ypbb5_53[data-draft-active=true] {
  border-color: #ff6c02;
  background-color: rgba(255, 108, 2, 0.15);
}
._root_ypbb5_53[data-draft-active=true]:has(button._draft_ypbb5_67:enabled:hover) {
  border-color: #ff8b35;
}

._draft_ypbb5_67 {
  all: unset;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 8px 10px;
  cursor: pointer;
  color: #646464;
  border-left: 1px solid #303030;
  border-radius: 0 5px 5px 0;
  margin: -1px;
}
._draft_ypbb5_67:disabled:hover {
  cursor: not-allowed;
}
._draft_ypbb5_67[data-active=true] {
  border-color: #ff6c02;
}
._draft_ypbb5_67[data-active=true]:enabled:hover {
  border-color: #ff8b35;
  background-color: #ff8b35;
}
._draft_ypbb5_67[data-active=false] {
  border: 1px solid #303030;
}
._draft_ypbb5_67[data-active=false]:enabled:hover {
  background-color: #0c0c0c;
}
._draft_ypbb5_67:focus-visible {
  outline: 1px solid;
  outline-offset: -1px;
  outline-color: #303030;
  border-radius: 0 6px 6px 0;
}
._draft_ypbb5_67[data-active=true] {
  color: #f3f3f3;
  background-color: #ff6c02;
}
._draft_ypbb5_67[data-loading=false] ._draft_ypbb5_67[data-active=true] {
  transition: color 0.2s, background-color 0.2s;
}
._draft_ypbb5_67[data-loading=false] ._draft_ypbb5_67[data-active=true]:enabled:hover {
  color: #fff;
}
._draft_ypbb5_67[data-loading=true] {
  cursor: wait !important;
}
._draft_ypbb5_67[data-loading=true] svg {
  animation: _breathe_ypbb5_1 1s infinite;
}

._tooltipWrapper_ypbb5_122 {
  position: relative;
  display: flex;
  height: 100%;
}
._tooltipWrapper_ypbb5_122:hover ._tooltip_ypbb5_122 {
  visibility: visible;
}

._dragHandle_ypbb5_131 {
  all: unset;
  cursor: grab;
}
._dragHandle_ypbb5_131._dragging_ypbb5_135 {
  cursor: grabbing;
}
._dragHandle_ypbb5_131:active {
  cursor: grabbing;
}

._tooltip_ypbb5_122 {
  position: absolute;
  bottom: 40px;
  left: 50%;
  transform: translateX(-50%) translateY(0);
  background-color: #0c0c0c;
  border: 1px solid #303030;
  color: white;
  border-radius: 4px;
  max-width: 250px;
  width: max-content;
  font-size: 14px;
  z-index: 1000;
  visibility: hidden;
  --translate-x: -50%;
}
._tooltip_ypbb5_122._forceVisible_ypbb5_158 {
  visibility: visible;
}
._tooltip_ypbb5_122._top_ypbb5_161 {
  top: 40px;
  bottom: unset;
  transform: translateY(0) translateX(var(--translate-x));
}
._tooltip_ypbb5_122._top_ypbb5_161:before {
  mask-image: linear-gradient(135deg, rgb(0, 0, 0) 31%, rgba(0, 0, 0, 0) 31%, rgba(0, 0, 0, 0) 100%);
  top: -4.5px;
  bottom: unset;
  transform: translateX(var(--translate-x)) rotate(45deg);
}
._tooltip_ypbb5_122._bottom_ypbb5_172 {
  bottom: unset;
  top: -40px;
  transform: translateY(0) translateX(var(--translate-x));
}
._tooltip_ypbb5_122._bottom_ypbb5_172:before {
  bottom: -4.5px;
  top: unset;
  transform: translateX(0) rotate(45deg);
}
._tooltip_ypbb5_122._right_ypbb5_182 {
  right: 0;
  left: unset;
  transform: translateX(0);
  --translate-x: 0;
}
._tooltip_ypbb5_122._right_ypbb5_182:before {
  right: 8px;
  left: unset;
  transform: translateX(--translate-x) rotate(45deg);
}
._tooltip_ypbb5_122._left_ypbb5_193 {
  left: 50%;
  right: unset;
  transform: translateX(-50%);
  --translate-x: -50%;
}
._tooltip_ypbb5_122._left_ypbb5_193:before {
  left: 50%;
  right: unset;
  transform: translateX(-50%) rotate(45deg);
}
._tooltip_ypbb5_122:before {
  z-index: -1;
  mask-image: linear-gradient(-45deg, rgb(0, 0, 0) 31%, rgba(0, 0, 0, 0) 31%, rgba(0, 0, 0, 0) 100%);
  content: "";
  position: absolute;
  bottom: -4.5px;
  left: 50%;
  width: 20px;
  height: 20px;
  background-color: #0c0c0c;
  transform: rotate(45deg) translateX(-50%);
  border-radius: 2px;
  border: 1px solid #303030;
}

._branchSelect_ypbb5_219 {
  height: 100%;
  background: none;
  border: none;
  font-weight: 500;
  font-size: 16px;
  padding-right: 8px;
  padding-bottom: 0px;
  padding-top: 0px;
  margin-bottom: 2px;
  min-width: 80px;
  max-width: 250px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: normal;
  outline: none;
  color: inherit;
  text-overflow: ellipsis;
  white-space: nowrap;
  opacity: 1;
  font-family: var(--font-family);
  appearance: none;
  -webkit-appearance: none;
  -moz-appearance: none;
}

._branchSelectIcon_ypbb5_245 {
  position: absolute;
  top: 50%;
  transform: translateY(-50%);
  right: 0;
  pointer-events: none;
}

@keyframes _in_ypbb5_1 {
  0% {
    opacity: 0;
    transform: translateY(4px) scale(0.98);
  }
  100% {
    opacity: 1;
    transform: translateY(0) scale(1);
  }
}
@keyframes _breathe_ypbb5_1 {
  0% {
    opacity: 1;
  }
  50% {
    opacity: 0.45;
  }
  100% {
    opacity: 1;
  }
}`;if("u">typeof document&&!document.getElementById(n)){var l=document.createElement("style");l.id=n,l.textContent=i,document.head.appendChild(l)}var s="_tooltip_ypbb5_122",p="_top_ypbb5_161",d="_bottom_ypbb5_172",b="_right_ypbb5_182",c="_left_ypbb5_193",f="_branchSelect_ypbb5_219",u=t.forwardRef(({children:e,content:a,forceVisible:n},i)=>{let l=t.useRef(null),f=t.useCallback((0,r.default)(()=>{if(l.current){let e=l.current.getBoundingClientRect(),t=l.current.classList.contains(c)?0:e.width/2,r=e.height;(l.current.classList.contains(d)?e.top:e.top-80-r)<=0?(l.current.classList.remove(d),l.current.classList.add(p)):(l.current.classList.remove(p),l.current.classList.add(d)),e.right+t>window.innerWidth?(l.current.classList.remove(c),l.current.classList.add(b)):(l.current.classList.remove(b),l.current.classList.add(c))}},100),[]);return t.useEffect(()=>(f(),window.addEventListener("resize",f),()=>{window.removeEventListener("resize",f)}),[f]),t.useImperativeHandle(i,()=>({checkOverflow:f}),[f]),(0,o.jsxs)("div",{className:"_tooltipWrapper_ypbb5_122",children:[(0,o.jsx)("p",{ref:l,style:{padding:"3px 8px"},className:n?`${s} ${d} ${c} _forceVisible_ypbb5_158`:`${s} ${d} ${c}`,children:a}),e]})}),_=t.forwardRef(({onDrag:e,children:r},a)=>{let[n,i]=t.useState(!1),l=t.useRef({x:0,y:0}),s=t.useRef({x:0,y:0}),p=t.useRef(!1);t.useImperativeHandle(a,()=>({hasDragged:p.current}));let d=t.useCallback(t=>{if(!n)return;let r=t.clientX-l.current.x,o=t.clientY-l.current.y,a=s.current.x+r,i=s.current.y+o;(Math.abs(r)>2||Math.abs(o)>2)&&(p.current=!0),e({x:a,y:i})},[n,e]);return t.useLayoutEffect(()=>{if(n)return window.addEventListener("pointermove",d),()=>{window.removeEventListener("pointermove",d)}},[n,e,d]),t.useLayoutEffect(()=>{if(!n){p.current=!1;return}let e=()=>{i(!1)};return window.addEventListener("pointerup",e),()=>{window.removeEventListener("pointerup",e)}},[n]),(0,o.jsx)("span",{draggable:!0,className:`_dragHandle_ypbb5_131 ${n?"_dragging_ypbb5_135":""}`,onPointerDown:e=>{if(e.target instanceof HTMLElement&&("select"===e.target.nodeName.toLowerCase()||e.target.closest("select")))return;let t=e.currentTarget;if(!t)return;e.stopPropagation(),e.preventDefault(),l.current={x:e.clientX,y:e.clientY};let r=t.getBoundingClientRect();s.current.x=r.left,s.current.y=r.top,i(!0)},onPointerUp:()=>{i(!1)},children:r})}),h=({isForcedDraft:e,draft:r,apiRref:a,latestBranches:n,onRefChange:i,getAndSetLatestBranches:l})=>{let s=t.useRef(null),p=t.useRef(null),d=t.useMemo(()=>n?[...n].sort((e,t)=>e.isDefault?-1:t.isDefault?1:e.name.localeCompare(t.name)):[],[n]),b=t.useMemo(()=>{let e=new Set(d.map(e=>e.name));return e.add(a),Array.from(e)},[d,a]),[c,_]=t.useState(!1);t.useEffect(()=>{c&&l().then(()=>{_(!1)})},[c,l]),t.useEffect(()=>{let e=s.current,t=p.current;if(!e||!t)return;let r=()=>{let r=e.offsetWidth;t.style.width=`${r+20}px`};return r(),window.addEventListener("resize",r),()=>{window.removeEventListener("resize",r),t&&t.style.removeProperty("width")}},[a]);let h=e||r;return(0,o.jsxs)("div",{className:"_branch_ypbb5_32","data-draft-active":h,onMouseEnter:()=>{_(!0)},children:[(0,o.jsx)(y,{})," ",(0,o.jsxs)(u,{content:h?"Switch branch":"Switch branch and enter draft mode",children:[(0,o.jsx)("select",{ref:p,value:a,onChange:e=>i(e.target.value,{enableDraftMode:!h}),className:f,onMouseDown:e=>{e.stopPropagation()},onClick:e=>{e.stopPropagation(),_(!0)},children:b.map(e=>(0,o.jsx)("option",{value:e,children:e},e))}),(0,o.jsx)("svg",{width:"15",height:"15",viewBox:"0 0 15 15",fill:"none",xmlns:"http://www.w3.org/2000/svg",className:"_branchSelectIcon_ypbb5_245",children:(0,o.jsx)("path",{d:"M4.93179 5.43179C4.75605 5.60753 4.75605 5.89245 4.93179 6.06819C5.10753 6.24392 5.39245 6.24392 5.56819 6.06819L7.49999 4.13638L9.43179 6.06819C9.60753 6.24392 9.89245 6.24392 10.0682 6.06819C10.2439 5.89245 10.2439 5.60753 10.0682 5.43179L7.81819 3.18179C7.73379 3.0974 7.61933 3.04999 7.49999 3.04999C7.38064 3.04999 7.26618 3.0974 7.18179 3.18179L4.93179 5.43179ZM10.0682 9.56819C10.2439 9.39245 10.2439 9.10753 10.0682 8.93179C9.89245 8.75606 9.60753 8.75606 9.43179 8.93179L7.49999 10.8636L5.56819 8.93179C5.39245 8.75606 5.10753 8.75606 4.93179 8.93179C4.75605 9.10753 4.75605 9.39245 4.93179 9.56819L7.18179 11.8182C7.35753 11.9939 7.64245 11.9939 7.81819 11.8182L10.0682 9.56819Z",fill:"currentColor",fillRule:"evenodd",clipRule:"evenodd"})})]}),(0,o.jsx)("span",{className:f,style:{visibility:"hidden",opacity:0,pointerEvents:"none",position:"absolute",top:0,left:0},"aria-hidden":"true",ref:s,children:a})]})},y=()=>(0,o.jsxs)("svg",{xmlns:"http://www.w3.org/2000/svg",width:"18",height:"18",fill:"none",children:[(0,o.jsx)("path",{fill:"#F3F3F3",fillRule:"evenodd",d:"M12.765 5.365a1.25 1.25 0 1 0 .002-2.502 1.25 1.25 0 0 0-.002 2.502Zm0 1.063a2.315 2.315 0 1 0-2.315-2.313 2.315 2.315 0 0 0 2.316 2.313ZM5.234 15.137a1.25 1.25 0 1 0 .001-2.501 1.25 1.25 0 0 0 0 2.501Zm0 1.064a2.315 2.315 0 1 0-2.316-2.314 2.315 2.315 0 0 0 2.316 2.314Z",clipRule:"evenodd"}),(0,o.jsx)("path",{fill:"#F3F3F3",fillRule:"evenodd",d:"M5.767 8.98v3.648H4.702V8.98h1.065ZM13.298 5.798v2.694h-1.065V5.798h1.065Z",clipRule:"evenodd"}),(0,o.jsx)("path",{fill:"#F3F3F3",fillRule:"evenodd",d:"M13.298 8.448a.532.532 0 0 1-.533.532H5.29a.532.532 0 1 1 0-1.064h7.476c.294 0 .533.238.533.532ZM5.234 2.864a1.25 1.25 0 1 1 .001 2.502 1.25 1.25 0 0 1 0-2.502Zm0-1.063a2.315 2.315 0 1 1-2.316 2.314A2.315 2.315 0 0 1 5.234 1.8Z",clipRule:"evenodd"}),(0,o.jsx)("path",{fill:"#F3F3F3",fillRule:"evenodd",d:"M5.767 9.022V5.374H4.702v3.648h1.065Z",clipRule:"evenodd"})]}),g="bshb_toolbar_pos",w=({draft:e,isForcedDraft:a,bshbPreviewToken:n,shouldAutoEnableDraft:i,seekAndStoreBshbPreviewToken:l,resolvedRef:s,enableDraftMode:p,disableDraftMode:d,getLatestBranches:b})=>{let c=t.useRef(p);c.current=p;let f=t.useRef(d);f.current=d;let y=t.useRef(b);y.current=b;let[w,L]=t.useState(null),C=t.useRef(null),j=t.useRef(null),[E,R]=t.useState(""),[M,S]=t.useState(!1),[k,Z]=t.useState(s.ref),[$,P]=t.useState(!0),[T,D]=t.useState(!0),[z,H]=t.useState(),N=t.useRef(0),O=t.useCallback(e=>{window.clearTimeout(N.current),R(e),N.current=window.setTimeout(()=>R(""),5e3)},[R]),F=t.useCallback(e=>{S(!0),c.current({bshbPreviewToken:e}).then(({status:e,response:t})=>{200===e?(H(e=>t.latestBranches??e),window.location.reload()):"error"in t?O(`Draft mode activation error: ${t.error}`):O("Draft mode activation error")}).finally(()=>S(!1))},[O]),B=`bshb-preview-ref-${s.repoHash}`,I=t.useMemo(()=>({set:e=>{document.cookie=`${B}=${e}; path=/; Max-Age=946080000`},clear:()=>{document.cookie=`${B}=; path=/; Max-Age=-1`},get:()=>document.cookie.split("; ").find(e=>e.startsWith(B))?.split("=")[1]??null}),[B]),[X,U]=t.useState(!1);t.useLayoutEffect(()=>{e||X||!i||a||!n||(F(n),U(!0))},[a,p,l,n,O,F,e,i,X]);let V=t.useCallback(async()=>{let e=[],t=await y.current({bshbPreviewToken:n});t&&(Array.isArray(t.response)?e=t.response:"error"in t.response&&console.error(`BaseHub Toolbar Error: ${t.response.error}`),H(e))},[n]);t.useEffect(()=>{!async function(){for(;;)try{V(),await new Promise(e=>setTimeout(e,3e4))}catch(e){console.error(`BaseHub Toolbar Error: ${e}`);break}}()},[V]);let A=t.useCallback(e=>{Z(e),window.__bshb_ref=e,window.dispatchEvent(new CustomEvent("__bshb_ref_changed")),I.set(e),P(e===s.ref)},[I,s.ref]);t.useEffect(()=>{let e=new URL(window.location.href).searchParams.get("bshb-preview-ref");e||(e=I.get()),D(!1),e&&A(e)},[I,A,s.repoHash]),t.useEffect(()=>{T||P(k===s.ref)},[k,s.ref,T]),t.useEffect(()=>{if(!T&&$){A(s.ref),I.clear();let e=new URL(window.location.href);e.searchParams.delete("bshb-preview-ref"),window.history.replaceState(null,"",e.toString())}},[$,T,I,s.ref,A]),t.useLayoutEffect(()=>{j.current?.checkOverflow()},[E]);let W=t.useCallback(()=>{if(!w||"u"<typeof window||!window.sessionStorage)return;let e=window.sessionStorage.getItem(g);if(!e)return;let t=JSON.parse(e);if("x"in t&&"y"in t)return t},[w]),Y=t.useCallback((0,r.default)(e=>{if("u"<typeof window||!window.sessionStorage)return;let t=W()??{x:0,y:0};window.sessionStorage.setItem(g,JSON.stringify({...t,...e}))},250),[]),J=t.useCallback(e=>{if(!w)return;let t=w.getBoundingClientRect(),r={};e.x-32<0?(w.style.left="32px",w.style.right="unset",r.x=32):e.x+t.width+32>window.innerWidth?(w.style.right="32px",w.style.left="unset",r.x=32):(w.style.right="unset",w.style.left=`${e.x}px`,r.x=e.x),e.y-32<0?(w.style.bottom="unset",w.style.top="32px",r.y=32):e.y+t.height+32>window.innerHeight?(w.style.top="unset",w.style.bottom="32px",r.y=32):(w.style.bottom="unset",w.style.top=`${e.y}px`,r.x=e.y),Y({x:e.x,y:e.y})},[w,Y]);t.useEffect(()=>{if("u"<typeof window)return;let e=()=>{let e=W();e&&(J(e),j.current?.checkOverflow())};return e(),window.addEventListener("resize",e),()=>{window.removeEventListener("resize",e)}},[W,J]),t.useEffect(()=>{if(!z)return;let e=I.get();e&&(z.find(t=>t.name===e)||I.clear())},[z,I]);let K=a?"Draft enforced by dev env":`${e?"Disable":"Enable"} draft mode`;return(0,o.jsx)("div",{className:"_wrapper_ypbb5_1",ref:L,children:(0,o.jsx)(_,{ref:C,onDrag:e=>{J(e),j.current?.checkOverflow()},children:(0,o.jsxs)("div",{className:"_root_ypbb5_53","data-draft-active":a||e,children:[(0,o.jsx)(h,{isForcedDraft:a,draft:e,apiRref:k,latestBranches:z,onRefChange:(e,t)=>{let r=new URL(window.location.href);if(r.searchParams.set("bshb-preview-ref",e),window.history.replaceState(null,"",r.toString()),A(e),t.enableDraftMode){let e=n??l();if(!e)return O("Preview token not found");F(e)}},getAndSetLatestBranches:V}),(0,o.jsx)(m,{previewRef:k,resolvedRef:s,isDraftModeEnabled:a||e}),(0,o.jsx)(u,{content:E||K,ref:j,forceVisible:!!E,children:(0,o.jsx)("button",{className:"_draft_ypbb5_67","data-active":a||e,"aria-label":`${e?"Disable":"Enable"} draft mode`,"data-loading":M,disabled:a||M,onClick:()=>{if(!(M||C.current?.hasDragged))if(e)S(!0),f.current().then(()=>{let e=new URL(window.location.href);e.searchParams.delete("bshb-preview"),e.searchParams.delete("__vercel_draft"),window.location.href=e.toString()}).finally(()=>S(!1));else{let e=n??l();if(!e)return O("Preview token not found");F(e)}},children:e||a?(0,o.jsx)(v,{}):(0,o.jsx)(x,{})})})]})})})},m=({previewRef:e,resolvedRef:r,isDraftModeEnabled:o})=>{let n=(0,a.usePathname)(),[i,l]=t.useState(n);return t.useEffect(()=>{i||l(n)},[n,i]),t.useEffect(()=>{if(!o&&i!==n&&e!==r.ref){let t=new URL(window.location.href);t.searchParams.set("bshb-preview-ref",e),window.history.replaceState(null,"",t.toString())}},[o,e,r.ref,n,i]),null},x=()=>(0,o.jsx)("svg",{"data-testid":"geist-icon",height:"16",strokeLinejoin:"round",viewBox:"0 0 16 16",width:"16",style:{color:"currentcolor"},children:(0,o.jsx)("path",{fillRule:"evenodd",clipRule:"evenodd",d:"M6.51404 3.15793C7.48217 2.87411 8.51776 2.87411 9.48589 3.15793L9.90787 1.71851C8.66422 1.35392 7.33571 1.35392 6.09206 1.71851L6.51404 3.15793ZM10.848 3.78166C11.2578 4.04682 11.6393 4.37568 11.9783 4.76932L13.046 6.00934L14.1827 5.03056L13.1149 3.79054C12.6818 3.28761 12.1918 2.86449 11.6628 2.52224L10.848 3.78166ZM4.02168 4.76932C4.36065 4.37568 4.74209 4.04682 5.15195 3.78166L4.33717 2.52225C3.80815 2.86449 3.3181 3.28761 2.88503 3.79054L1.81723 5.03056L2.95389 6.00934L4.02168 4.76932ZM14.1138 7.24936L14.7602 7.99999L14.1138 8.75062L15.2505 9.72941L16.3183 8.48938V7.5106L15.2505 6.27058L14.1138 7.24936ZM1.88609 7.24936L1.23971 7.99999L1.88609 8.75062L0.749437 9.72941L-0.318359 8.48938V7.5106L0.749436 6.27058L1.88609 7.24936ZM13.0461 9.99064L11.9783 11.2307C11.6393 11.6243 11.2578 11.9532 10.848 12.2183L11.6628 13.4777C12.1918 13.1355 12.6818 12.7124 13.1149 12.2094L14.1827 10.9694L13.0461 9.99064ZM4.02168 11.2307L2.95389 9.99064L1.81723 10.9694L2.88503 12.2094C3.3181 12.7124 3.80815 13.1355 4.33717 13.4777L5.15195 12.2183C4.7421 11.9532 4.36065 11.6243 4.02168 11.2307ZM9.90787 14.2815L9.48589 12.8421C8.51776 13.1259 7.48217 13.1259 6.51405 12.8421L6.09206 14.2815C7.33572 14.6461 8.66422 14.6461 9.90787 14.2815ZM6.49997 7.99999C6.49997 7.17157 7.17154 6.49999 7.99997 6.49999C8.82839 6.49999 9.49997 7.17157 9.49997 7.99999C9.49997 8.82842 8.82839 9.49999 7.99997 9.49999C7.17154 9.49999 6.49997 8.82842 6.49997 7.99999ZM7.99997 4.99999C6.34311 4.99999 4.99997 6.34314 4.99997 7.99999C4.99997 9.65685 6.34311 11 7.99997 11C9.65682 11 11 9.65685 11 7.99999C11 6.34314 9.65682 4.99999 7.99997 4.99999Z",fill:"currentColor"})}),v=()=>(0,o.jsx)("svg",{"data-testid":"geist-icon",height:"16",strokeLinejoin:"round",viewBox:"0 0 16 16",width:"16",style:{color:"currentcolor"},children:(0,o.jsx)("path",{fillRule:"evenodd",clipRule:"evenodd",d:"M4.02168 4.76932C6.11619 2.33698 9.88374 2.33698 11.9783 4.76932L14.7602 7.99999L11.9783 11.2307C9.88374 13.663 6.1162 13.663 4.02168 11.2307L1.23971 7.99999L4.02168 4.76932ZM13.1149 3.79054C10.422 0.663244 5.57797 0.663247 2.88503 3.79054L-0.318359 7.5106V8.48938L2.88503 12.2094C5.57797 15.3367 10.422 15.3367 13.1149 12.2094L16.3183 8.48938V7.5106L13.1149 3.79054ZM6.49997 7.99999C6.49997 7.17157 7.17154 6.49999 7.99997 6.49999C8.82839 6.49999 9.49997 7.17157 9.49997 7.99999C9.49997 8.82842 8.82839 9.49999 7.99997 9.49999C7.17154 9.49999 6.49997 8.82842 6.49997 7.99999ZM7.99997 4.99999C6.34311 4.99999 4.99997 6.34314 4.99997 7.99999C4.99997 9.65685 6.34311 11 7.99997 11C9.65682 11 11 9.65685 11 7.99999C11 6.34314 9.65682 4.99999 7.99997 4.99999Z",fill:"currentColor"})});e.s(["ClientToolbar",()=>w])}]);