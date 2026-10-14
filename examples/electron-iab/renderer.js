const log = document.querySelector('#log');
const address = document.querySelector('#address');
const tabs = document.querySelector('#tabs');
let currentState, refreshSequence = 0, renderedTabs = '';
function showError(error) {
  document.querySelector('#error').hidden = false;
  document.querySelector('#error').textContent = error.message;
  log.textContent = error.message;
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
  document.querySelector('#result').textContent = demo?.message ?? '准备就绪';
  const play = document.querySelector('#play');
  play.textContent = demo?.playing ? '■ 停止播放' : demo?.next>1 ? '↻ 重新播放' : '▶ 一键播放';
  play.disabled = !!demo?.busy && !demo?.playing || !!demo?.stopRequested;
  for (const step of document.querySelectorAll('[data-step]')) {
    const number = Number(step.dataset.step), done = demo?.completed.includes(number);
    step.disabled = !!demo?.busy || !!demo?.playing || (number!==demo?.next && !(number===1 && demo?.next===6) && !(number===5 && state.tabs.length));
    step.classList.toggle('done',!!done); step.classList.toggle('running',demo?.current===number);
    step.querySelector('.step-state').textContent = demo?.current===number ? '…' : done ? '✓' : '';
  }
  for (const button of document.querySelectorAll('[data-action]')) button.disabled = !!demo?.busy || !!demo?.playing;
  address.disabled = !!demo?.busy || !!demo?.playing;
  document.querySelector('#result-details').hidden = demo?.detail===undefined;
  if (demo?.detail!==undefined) log.textContent = JSON.stringify(demo.results,null,2);
  const selected = state.tabs.find(tab => tab.id === state.selected);
  document.querySelector('#status').textContent = `${state.tabs.length} 个标签`;
  document.querySelector('#empty').hidden = !!selected;
  document.querySelector('#empty-heading').textContent = state.tabs.length ? '网页已隐藏' : demo?.next===6 ? '演示已完成' : '开始演示';
  document.querySelector('#empty-description').textContent = state.tabs.length ? '选择上方标签即可恢复。' : demo?.next===6 ? '所有标签已关闭，可重新播放。' : '点击左侧步骤，或一键播放。';
  const emptyPlay = document.querySelector('#empty-play');
  emptyPlay.textContent = state.tabs.length ? '显示网页' : demo?.next===6 ? '重新播放' : '播放演示';
  emptyPlay.disabled = !!demo?.busy || !!demo?.playing;
  if (document.activeElement !== address) address.value = selected?.url === 'about:blank' ? '' : selected?.url ?? '';
  document.querySelector('#back').disabled = !state.canGoBack || !!demo?.busy || !!demo?.playing;
  document.querySelector('#forward').disabled = !state.canGoForward || !!demo?.busy || !!demo?.playing;
  document.querySelector('#reload').disabled = !selected || !!demo?.busy || !!demo?.playing;
  const key = JSON.stringify([state.tabs.map(tab=>[tab.id,tab.title,tab.url]),state.selected]);
  if (key === renderedTabs) {
    for (const button of tabs.querySelectorAll('button')) button.disabled = !!demo?.busy || !!demo?.playing;
    return;
  }
  renderedTabs = key; tabs.replaceChildren();
  for (const tab of state.tabs) {
    const item = document.createElement('div'); item.className = `tab${state.selected===tab.id?' selected':''}`;
    const select = document.createElement('button'); select.className = 'tab-select'; select.role = 'tab';
    select.setAttribute('aria-selected',String(state.selected===tab.id)); select.title = tab.url;
    const icon = document.createElement('span'); icon.className = 'tab-icon'; icon.textContent = '◎'; icon.setAttribute('aria-hidden','true');
    const title = document.createElement('span'); title.className = 'tab-title'; title.textContent = tab.title || (tab.url === 'about:blank' ? '新标签' : tab.url);
    select.disabled = !!demo?.busy || !!demo?.playing;
    select.append(icon,title); select.onclick = () => perform('select',tab.id);
    const close = document.createElement('button'); close.className = 'icon close'; close.textContent = '×'; close.title = '关闭标签'; close.setAttribute('aria-label',`关闭 ${title.textContent}`); close.onclick = () => perform('close',tab.id);
    close.disabled = !!demo?.busy || !!demo?.playing;
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
document.querySelector('#empty-play').onclick = () => currentState?.tabs.length ? perform('select',currentState.tabs.at(-1).id) : perform('demo-play');
document.querySelector('#address-form').onsubmit = event => {
  event.preventDefault(); const value = address.value.trim(); if (!value) return;
  const url = /^[a-z][a-z\d+.-]*:/i.test(value) ? value : `https://${value}`;
  void perform(currentState?.selected === undefined ? 'create' : 'navigate',url);
};
address.onblur = () => { void refresh().catch(showError); };
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
