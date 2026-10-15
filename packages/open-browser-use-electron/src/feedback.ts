import type { WebContents } from 'electron';
import { cursorSource, cursorBitmap } from './cursor.generated.js';

export type VisualFeedbackOptions = {
  cursor?: boolean;
  edgeGlow?: boolean;
  /** Shared edge color as #RGB or #RRGGBB. Defaults to #4f8fe8. */
  edgeGlowColor?: string;
};
type FeedbackPayload = Required<VisualFeedbackOptions> & { active: boolean; x?: number; y?: number; pressed: boolean; clicks: number };
type Entry = { active: boolean; pulse: boolean; x?: number; y?: number; pressed: boolean; clicks: number; installed: boolean; timer?: NodeJS.Timeout; queue: Promise<void> };

type CursorRenderer = { destroy(): void; setState(state: {cursor: {x?: number; y?: number; visible: boolean; animateMovement: boolean; moveSequence: number}; isVisible: boolean; turnKey: string; viewportSize: {width: number; height: number}}): void };
type CursorFactory = (root: HTMLElement, options: {assetUrl: string}) => CursorRenderer;

// Serialized into an isolated guest world. No host IPC, preload or credentials.
function renderFeedback(payload: FeedbackPayload, createCursorRenderer: CursorFactory, bitmap: typeof cursorBitmap): boolean {
  type Controller = { root: HTMLElement; cursorLayer: HTMLDivElement; renderer: CursorRenderer; edge: HTMLDivElement; ring: HTMLDivElement; clicks: number };
  const world = globalThis as typeof globalThis & { __obuIabFeedback?: Controller };
  let control = world.__obuIabFeedback;
  if (!payload.active || (!payload.cursor && !payload.edgeGlow)) {
    control?.renderer.destroy(); control?.root.remove(); delete world.__obuIabFeedback; return false;
  }
  if (!control || !control.root.isConnected) {
    control?.renderer.destroy();
    const root = document.createElement('div');
    root.id = 'obu-iab-visual-feedback'; root.setAttribute('aria-hidden','true'); root.inert = true;
    for (const [name,value] of Object.entries({position:'fixed',inset:'0',width:'100vw',height:'100vh',margin:'0',padding:'0',border:'0',background:'transparent',overflow:'hidden','pointer-events':'none','z-index':'2147483647'})) root.style.setProperty(name,value,'important');
    const shadow = root.attachShadow({mode:'closed'});
    const sheet = new CSSStyleSheet();
    // Constructed CSS and the original bitmap on canvas respect strict site CSP.
    // One blurred inset contour avoids the corner seams of four gradient strips.
    sheet.replaceSync(`
      :host{pointer-events:none!important}*{box-sizing:border-box;pointer-events:none!important}
      .edge{position:absolute;inset:0;box-shadow:inset 0 0 36px 4px rgba(var(--obu-edge-rgb),.32),inset 0 0 80px rgba(var(--obu-edge-rgb),.08);animation:obu-breathe 3.6s ease-in-out infinite}
      .ring{position:absolute;left:-16px;top:-16px;width:32px;height:32px;border:2px solid #70a2e5;border-radius:50%;opacity:0}
      @keyframes obu-breathe{50%{opacity:.8}}@keyframes obu-click{from{opacity:.8;scale:.3}to{opacity:0;scale:1.6}}
      @media(prefers-reduced-motion:reduce){.edge{animation:none}.ring{animation:none!important}}
    `);
    shadow.adoptedStyleSheets = [sheet];
    const edge = document.createElement('div'); edge.className='edge';
    const cursorLayer = document.createElement('div');
    const renderer = createCursorRenderer(cursorLayer,{assetUrl:''});
    // Only the image carrier changes: draw the extension PNG's exact RGBA pixels.
    // The renderer retains the original size, offsets, rotation and spring motion.
    const image = cursorLayer.querySelector<HTMLCanvasElement>('#open-browser-use-cursor-image')!;
    image.style.width=`${image.width}px`; image.style.height=`${image.height}px`;
    image.width=bitmap.width; image.height=bitmap.height;
    const pixels=Uint8ClampedArray.from(atob(bitmap.rgba),char=>char.charCodeAt(0));
    image.getContext('2d')!.putImageData(new ImageData(pixels,bitmap.width,bitmap.height),0,0);
    const ring = document.createElement('div'); ring.className='ring'; shadow.append(edge,ring,cursorLayer);
    document.documentElement.append(root);
    // A manual popover keeps the decoration above page stacking contexts.
    try { root.setAttribute('popover','manual'); root.showPopover(); } catch { root.removeAttribute('popover'); }
    control={root,cursorLayer,renderer,edge,ring,clicks:payload.clicks}; world.__obuIabFeedback=control;
  }
  const pointer = payload.cursor && Number.isFinite(payload.x) && Number.isFinite(payload.y);
  control.root.dataset.active='true'; control.root.dataset.cursorVisible=String(pointer); control.root.dataset.edgeGlow=String(payload.edgeGlow);
  control.edge.hidden=!payload.edgeGlow;
  const rgb = [1,3,5].map(start=>parseInt(payload.edgeGlowColor.slice(start,start+2),16)).join(',');
  control.edge.style.setProperty('--obu-edge-rgb',rgb);
  control.renderer.setState({cursor:{x:payload.x,y:payload.y,visible:pointer,animateMovement:!payload.pressed && !matchMedia('(prefers-reduced-motion: reduce)').matches,moveSequence:payload.clicks},isVisible:pointer,turnKey:'iab',viewportSize:{width:innerWidth,height:innerHeight}});
  control.ring.hidden=!pointer;
  if (pointer) {
    const x=payload.x!, y=payload.y!;
    control.root.dataset.x=String(x); control.root.dataset.y=String(y);
    if (payload.clicks!==control.clicks) {
      // Position separately from scale so the ripple stays at the input point.
      control.ring.style.left=`${x-16}px`; control.ring.style.top=`${y-16}px`;
      control.ring.style.animation='none'; void control.ring.getBoundingClientRect();
      control.ring.style.animation='obu-click 550ms ease-out'; control.clicks=payload.clicks;
    }
  }
  return true;
}

export class FeedbackController {
  private entries = new Map<WebContents,Entry>();
  private options: Required<VisualFeedbackOptions> = {cursor:false,edgeGlow:false,edgeGlowColor:'#4f8fe8'};
  constructor(options: VisualFeedbackOptions = {}) { this.options={...this.options,...this.normalize(options)}; }
  get configuration(): Required<VisualFeedbackOptions> { return {...this.options}; }
  private normalize(options: VisualFeedbackOptions): VisualFeedbackOptions {
    const normalized: VisualFeedbackOptions = {};
    for (const key of ['cursor','edgeGlow'] as const) if (options[key]!==undefined) {
      if (typeof options[key]!=='boolean') throw new Error('Visual feedback options must be boolean');
      normalized[key]=options[key];
    }
    if (options.edgeGlowColor!==undefined) {
      if (typeof options.edgeGlowColor!=='string' || !/^#(?:[\da-f]{3}|[\da-f]{6})$/i.test(options.edgeGlowColor)) throw new Error('edgeGlowColor must be a #RGB or #RRGGBB hex color');
      const color=options.edgeGlowColor.toLowerCase();
      normalized.edgeGlowColor=color.length===4?'#'+[...color.slice(1)].map(char=>char+char).join(''):color;
    }
    return normalized;
  }
  register(contents: WebContents): void { this.entries.set(contents,{active:false,pulse:false,pressed:false,clicks:0,installed:false,queue:Promise.resolve()}); }
  async configure(options: VisualFeedbackOptions): Promise<void> {
    this.options={...this.options,...this.normalize(options)};
    await Promise.all([...this.entries.keys()].map(contents=>this.paint(contents)));
  }
  async setActive(contents: WebContents,active: boolean): Promise<void> {
    if (typeof active!=='boolean') throw new Error('active must be boolean');
    const entry=this.entries.get(contents); if (!entry) return;
    clearTimeout(entry.timer); entry.timer=undefined; entry.pulse=false; entry.active=active; entry.pressed=false;
    if (!active) { entry.x=undefined; entry.y=undefined; }
    await this.paint(contents);
  }
  async input(contents: WebContents,method: string,params: Record<string,any>): Promise<void> {
    if (!this.options.cursor && !this.options.edgeGlow) return;
    const entry=this.entries.get(contents); if (!entry) return;
    if (Number.isFinite(params.x) && Number.isFinite(params.y)) { entry.x=params.x; entry.y=params.y; }
    entry.pressed=method==='Input.dispatchMouseEvent' && params.type==='mousePressed';
    if (entry.pressed) entry.clicks++;
    clearTimeout(entry.timer); entry.pulse=true;
    entry.timer=setTimeout(()=>{entry.pulse=false;entry.pressed=false;void this.paint(contents);},1200); entry.timer.unref();
    await this.paint(contents);
  }
  resetDocument(contents: WebContents): void {
    const entry=this.entries.get(contents); if (!entry) return;
    clearTimeout(entry.timer); entry.timer=undefined; entry.pulse=false; entry.x=undefined; entry.y=undefined; entry.pressed=false;
    void this.paint(contents);
  }
  private paint(contents: WebContents): Promise<void> {
    const entry=this.entries.get(contents); if (!entry) return Promise.resolve();
    entry.queue=entry.queue.catch(()=>{}).then(async()=>{
      if (this.entries.get(contents)!==entry || contents.isDestroyed()) return;
      const payload: FeedbackPayload={...this.options,active:entry.active || entry.pulse,x:entry.x,y:entry.y,pressed:entry.pressed,clicks:entry.clicks};
      if ((!payload.active || (!payload.cursor && !payload.edgeGlow)) && !entry.installed) return;
      // Visual rendering is best effort, and must not turn into input capability.
      try { entry.installed=await contents.executeJavaScriptInIsolatedWorld(999,[{code:`(()=>{${cursorSource}\nreturn (${renderFeedback.toString()})(${JSON.stringify(payload)},createCursorRenderer,${JSON.stringify(cursorBitmap)});})()`}])===true; } catch {}
    });
    return entry.queue;
  }
  release(contents: WebContents): void { const entry=this.entries.get(contents); clearTimeout(entry?.timer); this.entries.delete(contents); }
}
