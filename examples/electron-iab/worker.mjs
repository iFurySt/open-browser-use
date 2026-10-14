import { connectPrivateIab } from 'open-browser-use-electron/client';
const client = await connectPrivateIab(); // FD 3 carries bootstrap; selects private/socket.
try {
  const browser = client.browser();
  const tab = await browser.newTab({ url: process.argv[2] });
  await tab.evaluate(`document.querySelector('#message').value = 'Connected through ${client.bootstrap.transport}'`);
  console.log(JSON.stringify({ transport: client.bootstrap.transport, tabId: tab.id, tabs: await browser.getTabs(), snapshot: await tab.domSnapshot() }));
} finally { client.close(); }
