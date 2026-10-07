(args) => {
  const visible = e => !!e.getClientRects().length && getComputedStyle(e).visibility !== "hidden";
  const target = selector => {
    const matches = [...document.querySelectorAll(selector)];
    if (matches.length !== 1) throw new Error(`Selector must match exactly one element, found ${matches.length}`);
    const e = matches[0];
    if (!visible(e) || e.matches(":disabled") || e.closest("[inert]")) throw new Error("Target is hidden, disabled or inert");
    return e;
  };
  const selectorFor = e => {
    if (e.id && document.querySelectorAll(`#${CSS.escape(e.id)}`).length === 1) return `#${CSS.escape(e.id)}`;
    const parts = [];
    for (let node = e; node?.nodeType === 1; node = node.parentElement) {
      const siblings = node.parentElement ? [...node.parentElement.children].filter(s => s.tagName === node.tagName) : [node];
      parts.unshift(`${node.localName}:nth-of-type(${siblings.indexOf(node) + 1})`);
    }
    return parts.join(" > ");
  };
  if (args.operation === "snapshot") {
    const all = [...document.querySelectorAll("a,button,input,textarea,select,[contenteditable=true],[role=button]")].filter(visible);
    const text = document.body?.innerText ?? "";
    return {
      title: document.title, url: location.href, readyState: document.readyState,
      text: text.slice(0, args.maxChars), textTruncated: text.length > args.maxChars,
      elements: all.slice(0, args.maxElements).map(e => ({
        tag: e.localName, role: e.getAttribute("role"), type: e.type ?? null,
        name: (e.getAttribute("aria-label") || e.labels?.[0]?.innerText || e.innerText || e.getAttribute("placeholder") || e.name || "").trim().slice(0, 200),
        selector: selectorFor(e), disabled: e.matches(":disabled"),
        ...(e.localName === "a" ? {href: e.href} : {}),
        ...(e.type === "checkbox" || e.type === "radio" ? {checked: e.checked} : {})
      })), elementsTruncated: all.length > args.maxElements
    };
  }
  if (args.operation === "wait-for") {
    return {matched: (!args.selector || [...document.querySelectorAll(args.selector)].some(visible)) &&
      (!args.text || (document.body?.innerText ?? "").includes(args.text)) &&
      (!args.urlIncludes || location.href.includes(args.urlIncludes)), url: location.href};
  }
  if (args.operation === "click") {
    const e = target(args.selector);
    e.scrollIntoView({block: "center"});
    e.click();
    return {clicked: true, selector: args.selector};
  }
  if (args.operation === "fill") {
    // Validate all fields before changing any field. Never click submit or press Enter.
    const prepared = args.fields.map(f => {
      const e = target(f.selector);
      if (e.readOnly || e.type === "password") throw new Error("Read-only and password fields are not supported by fill; use an explicitly authorized login flow");
      if (f.expectedValue !== undefined && e.value !== f.expectedValue) throw new Error("Existing value does not match expectedValue; preserve the user's draft");
      if (f.checked !== undefined) {
        if (!e.matches('input[type=checkbox],input[type=radio]')) throw new Error("checked requires a checkbox or radio");
        if (e.type === "radio" && !f.checked) throw new Error("Unchecking a radio is not supported");
      } else if (!e.matches('textarea,input:not([type]),input[type=text],input[type=email],input[type=search],input[type=url],input[type=tel],input[type=number],select')) {
        throw new Error("Text requires an editable input, textarea or select");
      }
      if (e.localName === "select" && ![...e.options].some(o => o.value === f.text && !o.disabled)) throw new Error("Unknown or disabled select value");
      return {e, f};
    });
    return {fields: prepared.map(({e, f}) => {
      if (f.checked !== undefined) {
        if (e.checked !== f.checked) e.click();
      } else {
        const prototype = e.localName === "textarea" ? HTMLTextAreaElement.prototype : e.localName === "select" ? HTMLSelectElement.prototype : HTMLInputElement.prototype;
        Object.getOwnPropertyDescriptor(prototype, "value").set.call(e, f.text);
        e.dispatchEvent(new Event("input", {bubbles: true}));
        e.dispatchEvent(new Event("change", {bubbles: true}));
      }
      if (f.checked !== undefined ? e.checked !== f.checked : e.value !== f.text) throw new Error("Field did not retain the requested value");
      return {selector: f.selector, ...(f.checked !== undefined ? {checked: e.checked} : {value: e.value})};
    }), submitInvoked: false};
  }
  if (args.operation === "set-input-files") {
    const e = target(args.selector);
    if (!e.matches('input[type=file]')) throw new Error("Target must be a file input");
    if (e.files.length && !args.replace) throw new Error("File input already contains files; use replace only when authorized");
    if (!e.multiple && args.files.length !== 1) throw new Error("File input accepts one file");
    const transfer = new DataTransfer();
    for (const f of args.files) {
      const bytes = Uint8Array.from(atob(f.base64), c => c.charCodeAt(0));
      transfer.items.add(new File([bytes], f.name, {type: f.type}));
    }
    e.files = transfer.files;
    e.dispatchEvent(new Event("input", {bubbles: true}));
    e.dispatchEvent(new Event("change", {bubbles: true}));
    return {files: [...e.files].map(f => ({name: f.name, size: f.size, type: f.type})), submitInvoked: false};
  }
  throw new Error(`Unknown DOM operation: ${args.operation}`);
}
