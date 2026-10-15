import { normalizeAddress } from './address.mjs';
const log = document.querySelector('#log');
const address = document.querySelector('#address');
const tabs = document.querySelector('#tabs');
let currentState, refreshSequence = 0, renderedTabs = '', addressTabId;
const pendingAddresses = new Map();
let pendingCreate, selectOnMouseUp = false;
function showError(error) {
  document.querySelector('#error').hidden = false;
  document.querySelector('#error').textContent = error.message;
  log.textContent = error.message;
  document.querySelector('.result').hidden = false;
  document.querySelector('#result').hidden = false;
  document.querySelector('#result').textContent = '调用失败';
}
async function perform(action, value) {
  document.querySelector('#error').hidden = true;
  try {
    await window.iabExample.action(action, value);
    await refresh();
  } catch (error) { showError(error); }
}
async function refresh() {
  const sequence = ++refreshSequence;
  const state = await window.iabExample.state();
  if (sequence !== refreshSequence) return;
  currentState = state;
  const demo = state.demo;
  document.querySelector('#feedback-enabled').checked = !!state.feedbackEnabled;
  const result = document.querySelector('#result');
  result.textContent = demo?.message ?? '';
  result.hidden = !result.textContent;
  document.querySelector('.result').hidden = !result.textContent && demo?.detail===undefined && document.querySelector('#error').hidden;
  const play = document.querySelector('#play');
  play.dataset.mode = demo?.playing ? 'stop' : 'play';
  document.querySelector('#play-label').textContent = demo?.playing ? '停止播放' : demo?.completed.length ? '重新播放' : '一键播放';
  document.querySelector('#step-progress').textContent = `${demo?.completed.length ?? 0} / ${demo?.totalSteps ?? 5}`;
  play.disabled = !!state.controlBusy || !!demo?.busy && !demo?.playing || !!demo?.stopRequested;
  for (const step of document.querySelectorAll('[data-step]')) {
    const number = Number(step.dataset.step), done = demo?.completed.includes(number);
    step.disabled = !!state.controlBusy || !!demo?.busy || !!demo?.playing;
    step.classList.toggle('done',!!done); step.classList.toggle('running',demo?.current===number);
    step.querySelector('.step-state').textContent = demo?.current===number ? '…' : done ? '✓' : '';
  }
  address.disabled = false;
  document.querySelector('#result-details').hidden = demo?.detail===undefined;
  if (demo?.detail!==undefined) log.textContent = JSON.stringify(demo.results,null,2);
  const selected = state.tabs.find(tab => tab.id === state.selected);
  const submitted = pendingAddresses.get(state.selected) ?? (pendingCreate && !pendingCreate.knownIds.has(state.selected) ? pendingCreate : undefined);
  const loading = !!selected?.loading || !!submitted;
  const displayUrl = submitted?.url ?? selected?.pendingUrl ?? selected?.loadError?.url ?? selected?.url ?? '';
  document.querySelector('#status').textContent = `${loading?'加载中 · ':selected?.loadError?'加载失败 · ':''}${state.tabs.length} 个标签`;
  document.querySelector('#address-form').classList.toggle('loading',loading);
  document.querySelector('#viewport').setAttribute('aria-busy',String(loading));
  document.querySelector('#empty').hidden = !!selected;
  document.querySelector('#empty-heading').textContent = state.tabs.length ? '网页已隐藏' : demo?.next>demo?.totalSteps ? '演示已完成' : '开始演示';
  document.querySelector('#empty-description').textContent = state.tabs.length ? '选择上方标签即可恢复。' : demo?.next>demo?.totalSteps ? '可以重新播放演示。' : '点击左侧步骤，或一键播放。';
  const emptyPlay = document.querySelector('#empty-play');
  emptyPlay.textContent = state.tabs.length ? '显示网页' : demo?.next>demo?.totalSteps ? '重新播放' : '播放演示';
  emptyPlay.disabled = !state.tabs.length && (!!state.controlBusy || !!demo?.busy || !!demo?.playing);
  if (document.activeElement !== address) address.value = displayUrl === 'about:blank' ? '' : displayUrl;
  document.querySelector('#back').disabled = !state.canGoBack;
  document.querySelector('#forward').disabled = !state.canGoForward;
  document.querySelector('#reload').disabled = !selected;
  const key = JSON.stringify([state.tabs.map(tab=>[tab.id,tab.title,tab.url,tab.pendingUrl,tab.loading,!!pendingAddresses.get(tab.id),!!(pendingCreate && !pendingCreate.knownIds.has(tab.id)),tab.loadError]),state.selected]);
  if (key === renderedTabs) {
    return;
  }
  renderedTabs = key; tabs.replaceChildren();
  for (const tab of state.tabs) {
    const item = document.createElement('div'); item.className = `tab${state.selected===tab.id?' selected':''}`;
    item.dataset.tabId=String(tab.id);
    const select = document.createElement('button'); select.className = 'tab-select'; select.role = 'tab';
    select.setAttribute('aria-selected',String(state.selected===tab.id)); select.title = tab.url;
    const icon = document.createElement('span'); icon.className = 'tab-icon'; const globe=document.createElementNS('http://www.w3.org/2000/svg','svg'); globe.setAttribute('viewBox','0 0 24 24');
    const tabLoading=!!tab.loading || pendingAddresses.has(tab.id) || !!(pendingCreate && !pendingCreate.knownIds.has(tab.id));
    item.classList.toggle('loading',tabLoading); select.setAttribute('aria-busy',String(tabLoading));
    const lines=document.createElementNS('http://www.w3.org/2000/svg','path'); lines.setAttribute('d','M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0ZM3 12h18M12 3a18 18 0 0 1 0 18 18 18 0 0 1 0-18'); globe.append(lines); icon.append(globe); icon.setAttribute('aria-hidden','true');
    const title = document.createElement('span'); title.className = 'tab-title'; title.textContent = tab.title || (tab.url === 'about:blank' ? '新标签' : tab.url);
    select.append(icon,title); select.onclick = () => perform('select',tab.id);
    const close = document.createElement('button'); close.className = 'icon close'; close.textContent = '×'; close.title = '关闭标签'; close.setAttribute('aria-label',`关闭 ${title.textContent}`); close.onclick = () => perform('close',tab.id);
    item.append(select,close); tabs.append(item);
    if (state.selected === tab.id) item.scrollIntoView({block:'nearest',inline:'nearest'});
  }
}
for (const button of document.querySelectorAll('[data-action]')) {
  button.onclick = async () => {
    button.disabled = true;
    try { await perform(button.dataset.action); }
    finally { button.disabled = false; await refresh().catch(showError); }
  };
}
for (const step of document.querySelectorAll('[data-step]')) step.onclick = () => perform('demo-step',Number(step.dataset.step));
document.querySelector('#play').onclick = () => perform(currentState?.demo.playing ? 'demo-stop' : 'demo-play');
document.querySelector('#feedback-enabled').onchange = event => perform('feedback',event.target.checked);
document.querySelector('#empty-play').onclick = () => currentState?.tabs.length ? perform('select',currentState.tabs.at(-1).id) : perform('demo-play');
document.querySelector('#address-form').onsubmit = event => {
  event.preventDefault(); const value = address.value.trim(); if (!value) return;
  let url;
  try { url=normalizeAddress(value); } catch(error) { showError(error); return; }
  // Keep the tab the user began editing, even if the current demo step presents
  // another page while it finishes. Main validates ownership again.
  const tabId = addressTabId ?? currentState?.selected;
  const entry={url,knownIds:new Set(currentState?.tabs.map(tab=>tab.id) ?? [])};
  if (tabId===undefined) pendingCreate=entry; else pendingAddresses.set(tabId,entry);
  address.value=url; address.blur();
  void refresh().catch(showError);
  void perform(tabId === undefined ? 'create' : 'navigate',tabId === undefined ? url : {tabId,url}).finally(()=>{
    if (pendingAddresses.get(tabId)===entry) pendingAddresses.delete(tabId);
    if (pendingCreate===entry) pendingCreate=undefined;
    void refresh().catch(showError);
  });
};
address.onfocus = () => {
  addressTabId = currentState?.selected;
  address.select();
  // Chromium can restore the prior caret after focus dispatch. Select again at
  // the next paint, without disturbing a field that has already lost focus.
  requestAnimationFrame(()=>{if(document.activeElement===address) address.select();});
  if (currentState?.demo.playing) void perform('demo-stop');
};
address.onpointerdown = () => { selectOnMouseUp=document.activeElement!==address; };
address.onmouseup = event => { if (selectOnMouseUp) { event.preventDefault(); address.select(); selectOnMouseUp=false; } };
address.onblur = () => { selectOnMouseUp=false; void refresh().catch(showError); };
document.addEventListener('keydown',event=>{
  if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase()==='l') { event.preventDefault(); address.focus(); address.select(); }
});
window.iabExample.onFocusAddress(()=>{address.focus();address.select();});
// Native guests must match the actual DOM viewport, including all browser chrome.
// The main process validates sender and confines these bounds to its own window.
const viewport = document.querySelector('#viewport');
let previousBounds = '';
new ResizeObserver(() => {
  const {x,y,width,height} = viewport.getBoundingClientRect();
  const bounds = {x:Math.round(x),y:Math.round(y),width:Math.floor(width),height:Math.floor(height)};
  const key = JSON.stringify(bounds);
  if (bounds.width>0 && bounds.height>0 && key !== previousBounds) {
    previousBounds = key; void window.iabExample.action('viewport',bounds).catch(showError);
  }
}).observe(viewport);
window.iabExample.onChanged(() => { void refresh().catch(showError); });
void refresh().catch(showError);
