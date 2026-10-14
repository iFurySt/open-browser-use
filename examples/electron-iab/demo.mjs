const labels = ['打开 Tab 1','打开 Tab 2','回到 Tab 1，点击按钮','回到 Tab 2，输入并提交','关闭所有标签'];

export function createActionDemo({ cli, fixtureUrl, present, changed, paceMs = 1000 }) {
  const state = { next:1, busy:false, playing:false, stopRequested:false, current:undefined, completed:[], results:[], tab1:undefined, tab2:undefined, message:'准备就绪', detail:undefined };
  const snapshot = () => ({...state,completed:[...state.completed],results:[...state.results]});
  const update = values => { Object.assign(state,values); changed(); };
  async function command(args) {
    const response = await cli(args);
    if (response.error) throw new Error('CLI 请求失败');
    return response.result;
  }
  const cdp = (id,method,params={}) => command(['cdp',String(id),method,JSON.stringify(params)]);
  async function evaluate(id,expression) {
    const result = await cdp(id,'Runtime.evaluate',{expression,returnByValue:true,awaitPromise:true});
    if (result.exceptionDetails) throw new Error('页面操作失败，请重新播放');
    return result.result.value;
  }
  const list = () => command(['tabs']);
  async function owned(id) {
    if (!id || !(await list()).some(tab=>tab.id===id)) throw new Error('演示标签已关闭，请重新播放');
    return id;
  }
  async function waitFor(id,expression) {
    const deadline = Date.now()+5000;
    while (!(await evaluate(id,expression))) {
      if (Date.now()>=deadline) throw new Error('等待页面响应超时');
      await new Promise(resolve=>setTimeout(resolve,50));
    }
  }
  async function click(id,selector) {
    const point = await evaluate(id,`(()=>{const e=document.querySelector(${JSON.stringify(selector)});if(!e)throw Error('Missing element');e.scrollIntoView({block:'center'});const r=e.getBoundingClientRect();return {x:r.x+r.width/2,y:r.y+r.height/2}})()`);
    for (const type of ['mouseMoved','mousePressed','mouseReleased']) await cdp(id,'Input.dispatchMouseEvent',{
      type,...point,button:type==='mouseMoved'?'none':'left',buttons:type==='mousePressed'?1:0,clickCount:type==='mouseMoved'?0:1,
    });
  }
  async function closeAll() {
    // Closing an opener can also destroy its popup. Re-read live CLI state,
    // rather than trying to close stale IDs from the original tab snapshot.
    for (let attempt=0; attempt<128; attempt++) {
      const tabs = await list(); if (!tabs.length) return;
      const id = tabs[0].id;
      try { await command(['close-tab',String(id)]); }
      catch(error) { if ((await list()).some(tab=>tab.id===id)) throw error; }
    }
    throw new Error('页面持续创建新标签，清理已停止');
  }
  async function execute(number) {
    if (number===1 && state.next===6) update({next:1,completed:[],results:[],detail:undefined});
    if (!Number.isInteger(number) || number<1 || number>5 || (number!==5 && number!==state.next)) throw new Error('请按顺序执行步骤');
    update({busy:true,current:number,message:`${labels[number-1]}…`});
    try {
      let detail;
      switch(number) {
        case 1: {
          const tab = await command(['open-tab',`${fixtureUrl}/Tab%201`]); state.tab1=tab.id; present(tab.id); detail={tab1:tab.id}; break;
        }
        case 2: {
          await owned(state.tab1);
          const tab = await command(['open-tab',`${fixtureUrl}/Tab%202`]); state.tab2=tab.id; present(tab.id); detail={tab2:tab.id}; break;
        }
        case 3: {
          const id = await owned(state.tab1); present(id); await click(id,'#increment');
          await waitFor(id,'Number(document.querySelector("#count").textContent)>0');
          detail=await evaluate(id,'({count:Number(document.querySelector("#count").textContent),trustedClick:window.trustedClick})'); break;
        }
        case 4: {
          const id = await owned(state.tab2); present(id); await click(id,'#message');
          await evaluate(id,'document.querySelector("#message").select()');
          await cdp(id,'Input.insertText',{text:'Hello from CLI'}); await click(id,'#submit');
          await waitFor(id,'document.querySelector("#submitted").textContent.includes("Hello from CLI")');
          detail=await evaluate(id,'({submitted:document.querySelector("#submitted").textContent,trustedInput:window.trustedInput,trustedSubmit:window.trustedSubmit})'); break;
        }
        case 5: await closeAll(); state.tab1=undefined; state.tab2=undefined; detail={tabs:await list()}; break;
      }
      update({next:number+1,completed:[...new Set([...state.completed,number])],results:[...state.results.filter(result=>result.step!==number),{step:number,detail}],detail,message:number===5?'演示完成':`${labels[number-1]} · 完成`});
    } catch(error) { update({message:error.message}); throw error; }
    finally { update({busy:false,current:undefined}); }
    return snapshot();
  }
  return {
    snapshot,
    async step(number) {
      if (state.busy || state.playing) throw new Error('演示正在运行');
      return execute(number);
    },
    stop() { if (state.playing) update({stopRequested:true,message:'当前步骤结束后停止'}); return snapshot(); },
    async play() {
      if (state.busy || state.playing) throw new Error('演示正在运行');
      update({playing:true,busy:true,stopRequested:false,message:'准备播放…'});
      try {
        // A fresh run closes only this example scope's pages through the CLI.
        await closeAll();
        update({next:1,completed:[],results:[],tab1:undefined,tab2:undefined,detail:undefined,busy:false});
        for (let number=1; number<=5; number++) {
          if (state.stopRequested) break;
          await execute(number);
          if (number<5 && paceMs>0) await new Promise(resolve=>setTimeout(resolve,paceMs));
        }
        if (state.stopRequested && state.next<=5) update({message:'播放已停止，可继续逐步执行'});
      } catch(error) { update({message:error.message}); throw error; }
      finally { update({playing:false,busy:false,current:undefined,stopRequested:false}); }
      return snapshot();
    },
  };
}
