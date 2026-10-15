// Shared by the host renderer and main process.
export function normalizeAddress(value) {
  if (typeof value !== 'string' || !value.trim()) throw new Error('请输入网站地址');
  const input=value.trim();
  if (input==='about:blank') return input;
  if (/^(?:javascript|data|file|blob|chrome|devtools|vbscript):/i.test(input)) throw new Error('请输入 http 或 https 网站地址');
  const hostPort=/^[^/?#:\s]+:\d+(?:[/?#]|$)/.test(input);
  const local=/^(?:localhost|127(?:\.\d{1,3}){3}|\[::1\])(?::\d+)?(?:[/?#]|$)/i.test(input);
  const explicit=/^[a-z][a-z\d+.-]*:/i.test(input) && !hostPort;
  let url;
  try { url=new URL(explicit?input:input.startsWith('//')?'https:'+input:(local?'http':'https')+'://'+input); }
  catch { throw new Error('网站地址格式不正确'); }
  if (!['http:','https:'].includes(url.protocol) || url.username || url.password) throw new Error('请输入 http 或 https 网站地址');
  return url.href;
}
