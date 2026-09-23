const ROOT_ID = "open-browser-use-cursor-root";
const DEFAULT_X_RATIO = 0.58;
const DEFAULT_Y_RATIO = 0.55;

let latestState = {
  cursor: null,
  isVisible: false,
  sessionId: null,
  turnId: null
};
let cursorRenderer = null;
let interactionFeedback = null;
let stateRevision = 0;
let feedbackTurnKey = null;

chrome.runtime.onMessage.addListener((message, _sender, sendResponse) => {
  if (message?.type === "OPEN_BROWSER_USE_PING") {
    sendResponse({ ok: true });
    return true;
  }
  if (message?.type === "OPEN_BROWSER_USE_CURSOR_STATE") {
    stateRevision += 1;
    latestState = normalizeState(message.state);
    renderState(latestState);
    sendResponse({ ok: true });
    return true;
  }
  if (message?.type === "OPEN_BROWSER_USE_CURSOR") {
    stateRevision += 1;
    latestState = {
      cursor: {
        ...(message.animateMovement === false ? { animateMovement: false } : {}),
        moveSequence: message.moveSequence,
        visible: message.visible !== false,
        x: Number(message.x) || 0,
        y: Number(message.y) || 0
      },
      isVisible: true,
      sessionId: typeof message.sessionId === "string" ? message.sessionId : latestState.sessionId,
      turnId: typeof message.turnId === "string" ? message.turnId : latestState.turnId
    };
    renderState(latestState);
    sendResponse({ ok: true });
    return true;
  }
  if (message?.type === "OPEN_BROWSER_USE_POINTER_EVENT") {
    if (
      message.eventType === "mousePressed" &&
      latestState.isVisible &&
      latestState.cursor?.visible !== false &&
      message.sessionId === latestState.sessionId &&
      (latestState.turnId === null || message.turnId === latestState.turnId)
    ) {
      interactionFeedback?.click(message.x, message.y);
    }
    sendResponse({ ok: true });
    return true;
  }
  return false;
});

function ensureCursorRenderer() {
  if (cursorRenderer) {
    return cursorRenderer;
  }
  let root = document.getElementById(ROOT_ID);
  if (!root) {
    root = document.createElement("div");
    root.id = ROOT_ID;
    root.setAttribute("aria-hidden", "true");
    document.documentElement.appendChild(root);
  }
  cursorRenderer = createCursorRenderer(root, {
    assetUrl: chrome.runtime.getURL("images/cursor-chat.png"),
    onArrived: notifyCursorArrived
  });
  interactionFeedback = createInteractionFeedback(root);
  return cursorRenderer;
}

function renderState(state) {
  const renderer = ensureCursorRenderer();
  const cursor = normalizeCursor(state.cursor);
  const viewportSize = readViewportSize();
  const visible = state.isVisible === true && cursor?.visible !== false;
  const turnKey = typeof state.sessionId === "string" ? `${state.sessionId}:${state.turnId ?? ""}` : null;
  if (feedbackTurnKey !== turnKey) interactionFeedback.clear();
  feedbackTurnKey = turnKey;
  if (visible && cursor) {
    interactionFeedback.highlight(cursor.x, cursor.y);
  } else {
    interactionFeedback.clear();
  }
  renderer.setState({
    cursor,
    isVisible: visible,
    reducedMotion: prefersReducedMotion(),
    turnKey,
    viewportSize
  });
}

function createInteractionFeedback(root) {
  const interactiveSelector = [
    "button", "a[href]", "input", "textarea", "select", "summary",
    '[contenteditable]:not([contenteditable="false"])',
    '[role="button"]', '[role="link"]', '[role="checkbox"]',
    '[role="radio"]', '[role="switch"]', '[role="textbox"]',
    '[tabindex]:not([tabindex="-1"])'
  ].join(",");
  const outline = document.createElement("div");
  outline.dataset.testid = "browser-agent-target";
  Object.assign(outline.style, {
    all: "initial", position: "absolute", pointerEvents: "none",
    boxSizing: "border-box", border: "2px solid #4f8cff",
    borderRadius: "5px", background: "rgba(79, 140, 255, 0.08)",
    display: "none"
  });
  root.appendChild(outline);
  const rings = new Map();

  const validPoint = (x, y) => {
    const { width, height } = readViewportSize();
    return Number.isFinite(x) && Number.isFinite(y) && x >= 0 && y >= 0 && x < width && y < height;
  };
  const highlight = (x, y) => {
    outline.style.display = "none";
    if (!validPoint(x, y)) return;
    // Hit-test only this document. An iframe is highlighted as a frame; no
    // coordinates or page state are read from its child document.
    let hit = document.elementsFromPoint(x, y).find((element) => !root.contains(element));
    // Web Components expose their host to document hit testing. Descend only
    // through open shadow roots, keeping closed roots and frames opaque.
    while (hit?.shadowRoot) {
      const shadow = hit.shadowRoot;
      const inner = shadow.elementFromPoint(x, y);
      if (!inner || inner === hit || !shadow.contains(inner)) break;
      hit = inner;
    }
    const target = hit?.closest(interactiveSelector) ?? hit;
    if (!target?.isConnected || target === document.documentElement || target === document.body) return;
    const rect = target.getBoundingClientRect();
    if (![rect.left, rect.top, rect.width, rect.height].every(Number.isFinite) || rect.width <= 0 || rect.height <= 0) return;
    Object.assign(outline.style, {
      left: `${rect.left}px`, top: `${rect.top}px`,
      width: `${rect.width}px`, height: `${rect.height}px`, display: "block"
    });
  };
  const removeRing = (ring) => {
    const effect = rings.get(ring);
    if (!effect) return;
    window.clearTimeout(effect.timer);
    effect.animation?.cancel();
    ring.remove();
    rings.delete(ring);
  };
  const clear = () => {
    outline.style.display = "none";
    for (const ring of rings.keys()) removeRing(ring);
  };
  return {
    clear,
    highlight,
    click: (x, y) => {
      if (!validPoint(x, y)) return;
      highlight(x, y);
      // Each click owns its effect, so a second click never restarts cursor
      // travel or leaves the first click's timer behind.
      if (rings.size >= 8) removeRing(rings.keys().next().value);
      const ring = document.createElement("div");
      ring.dataset.testid = "browser-agent-click";
      Object.assign(ring.style, {
        all: "initial", position: "absolute", pointerEvents: "none",
        boxSizing: "border-box", left: `${x}px`, top: `${y}px`,
        width: "28px", height: "28px", border: "2px solid #14b8a6",
        borderRadius: "50%", background: "rgba(20, 184, 166, 0.15)",
        transform: "translate(-50%, -50%)", opacity: "1"
      });
      root.appendChild(ring);
      const reducedMotion = prefersReducedMotion();
      const duration = reducedMotion ? 200 : 450;
      const animation = reducedMotion ? null : ring.animate?.([
        { transform: "translate(-50%, -50%) scale(0.5)", opacity: 1 },
        { transform: "translate(-50%, -50%) scale(1.5)", opacity: 0 }
      ], { duration, easing: "ease-out" });
      const timer = window.setTimeout(() => removeRing(ring), duration);
      rings.set(ring, { animation, timer });
    }
  };
}

function prefersReducedMotion() {
  return window.matchMedia?.("(prefers-reduced-motion: reduce)").matches === true;
}

function normalizeState(state) {
  if (!state || typeof state !== "object") {
    return {
      cursor: null,
      isVisible: false,
      sessionId: null,
      turnId: null
    };
  }
  return {
    cursor: normalizeCursor(state.cursor),
    isVisible: state.isVisible === true,
    sessionId: typeof state.sessionId === "string" ? state.sessionId : null,
    turnId: typeof state.turnId === "string" ? state.turnId : null
  };
}

function normalizeCursor(cursor) {
  if (!cursor || typeof cursor !== "object") {
    return null;
  }
  if (!Number.isFinite(cursor.x) || !Number.isFinite(cursor.y)) {
    return null;
  }
  return {
    ...(cursor.animateMovement === false ? { animateMovement: false } : {}),
    ...(Number.isInteger(cursor.moveSequence) ? { moveSequence: cursor.moveSequence } : {}),
    visible: cursor.visible !== false,
    x: cursor.x,
    y: cursor.y
  };
}

function readViewportSize() {
  const viewport = window.visualViewport;
  return {
    width: viewport?.width ?? window.innerWidth,
    height: viewport?.height ?? window.innerHeight
  };
}

function notifyCursorArrived(moveSequence) {
  if (
    typeof latestState.sessionId !== "string" ||
    typeof latestState.turnId !== "string" ||
    !Number.isInteger(moveSequence)
  ) {
    return;
  }
  chrome.runtime
    .sendMessage({
      type: "OPEN_BROWSER_USE_CURSOR_ARRIVED",
      sessionId: latestState.sessionId,
      turnId: latestState.turnId,
      moveSequence
    })
    .catch(() => {});
}

function refreshCursorState() {
  const revision = stateRevision;
  chrome.runtime
    .sendMessage({ type: "GET_OPEN_BROWSER_USE_CURSOR_STATE" })
    .then((response) => {
      if (response?.ok && revision === stateRevision) {
        latestState = normalizeState(response.state);
        renderState(latestState);
      }
    })
    .catch(() => {});
}

const clearInteractionFeedback = () => interactionFeedback?.clear();
window.addEventListener("scroll", clearInteractionFeedback, true);
window.addEventListener("resize", clearInteractionFeedback);
window.visualViewport?.addEventListener("resize", clearInteractionFeedback);
window.visualViewport?.addEventListener("scroll", clearInteractionFeedback);
window.addEventListener("pagehide", () => {
  stateRevision += 1;
  latestState = normalizeState(null);
  interactionFeedback?.clear();
  interactionFeedback = null;
  cursorRenderer?.destroy();
  cursorRenderer = null;
});
refreshCursorState();

const BASE_ROTATION = -44;
const CURSOR_SIZE = 24;
const CURSOR_CENTER = CURSOR_SIZE / 2;
const IMAGE_WIDTH = 23;
const IMAGE_HEIGHT = 24;
const IMAGE_OFFSET_X = 12;
const IMAGE_OFFSET_Y = -2.5;
const IMAGE_ROTATION = 44;
const MIN_FRAME_SECONDS = 1 / 60;
const INTEGRATION_STEP_SECONDS = 1 / 240;
const MAX_SCRIPT_LAG_SECONDS = 1;
const SPRING_SETTLE_EPSILON = 0.001 * 60;
const ARRIVAL_DISTANCE = 0.85;
const ARRIVAL_VELOCITY = 12;
const SHORT_DISTANCE = 196;
const SCOOT_ROTATION_LIMIT = 70;
const SCOOT_STRETCH_MIX = 0.15;
const MIN_SQUASH = 0;
const VISIBILITY_SCALE_MIN = 0.4;
const VISIBILITY_BLUR_MAX = 5;
const THINK_DELAY_SECONDS = 0;
const THINK_DURATION_SECONDS = 1.41;
const THINK_PERIOD_SECONDS = 0.66;
const THINK_ROTATION_DEGREES = 12.5;
const PATH_CONFIG = {
  arcFlow: 0.5783555327868779,
  arcSize: 0.2765523188064277,
  boundsMargin: 20,
  candidateCount: 20,
  clickAngleDegrees: -44,
  endpointHandle: 0.15,
  startHandle: 0.41960295031576633
};
const DEFAULT_POSITION_SPRING = { dampingFraction: 0.9, response: 0.19 };
const STRETCH_SPRING = { dampingFraction: 0.85, response: 0.2 };
const VISIBILITY_SPRING = { dampingFraction: 0.86, response: 0.42 };
const SCOOT_PROGRESS_SPRING = { dampingFraction: 0.94, response: 0.19 };
const ROTATION_SPRING = { dampingFraction: 0.9, response: 0.12 };
const SCOOT_ROTATION_SPRING = { dampingFraction: 0.82, response: 0.055 };
const SCOOT_STRETCH_SPRING = { dampingFraction: 0.86, response: 0.12 };

function createCursorRenderer(root, { assetUrl, onArrived }) {
  const parts = createCursorParts(root, assetUrl);
  let frameId = null;
  let previousTimestamp = now();
  let model = null;
  let activeMoveSequence = null;
  let activeArrivalKey = null;
  let reportedArrivalKey = null;
  let thinkingTurnKey = null;
  let appearedTurnKey = null;
  let forceFirstFrame = false;
  let destroyed = false;

  const reportArrival = () => {
    if (
      activeMoveSequence == null ||
      activeArrivalKey == null ||
      reportedArrivalKey === activeArrivalKey
    ) {
      return;
    }
    reportedArrivalKey = activeArrivalKey;
    onArrived?.(activeMoveSequence);
  };

  const requestFrame = () => {
    if (frameId != null || model == null || destroyed) {
      return;
    }
    frameId = requestCursorFrame((timestamp) => {
      frameId = null;
      const current = model;
      if (!current) {
        return;
      }
      const deltaSeconds = forceFirstFrame
        ? MIN_FRAME_SECONDS
        : Math.max(MIN_FRAME_SECONDS, (timestamp - previousTimestamp) / 1000);
      forceFirstFrame = false;
      previousTimestamp = timestamp;
      const arrived = stepCursor(current, deltaSeconds, timestamp);
      applyCursorTransform(parts.cursor, current);
      if (arrived) {
        reportArrival();
      }
      if (isCursorAnimating(current)) {
        requestFrame();
      }
    });
  };

  return {
    destroy: () => {
      destroyed = true;
      if (frameId != null) {
        cancelCursorFrame(frameId);
        frameId = null;
      }
      parts.layer.remove();
    },
    setState: (state) => {
      if (state.isVisible === false || state.cursor?.visible === false) {
        if (frameId != null) cancelCursorFrame(frameId);
        frameId = null;
        model = null;
        parts.cursor.style.opacity = "0";
        return;
      }
      const turnKey = state.turnKey ?? "";
      const hasCursor = state.cursor != null;
      const point = normalizePoint({
        cursorX: state.cursor?.x,
        cursorY: state.cursor?.y,
        viewportHeight: state.viewportSize.height,
        viewportWidth: state.viewportSize.width
      });
      const visible = state.isVisible !== false && state.cursor?.visible !== false;
      const animateMovement = state.cursor?.animateMovement !== false;
      const becameVisibleWithoutCursor = visible && !hasCursor;

      activeMoveSequence = state.cursor?.moveSequence ?? null;
      activeArrivalKey = activeMoveSequence == null ? null : `${turnKey}:${activeMoveSequence}`;
      if (model == null) {
        model = createCursorModel(point, visible);
      }
      model.visibilitySpring.target = visible ? 1 : 0;
      if (state.reducedMotion === true) {
        if (frameId != null) cancelCursorFrame(frameId);
        frameId = null;
        model.thinkStartedAt = null;
        resetSpring(model.visibilitySpring, 1);
        snapCursor(model, point);
        applyCursorTransform(parts.cursor, model);
        reportArrival();
        return;
      }
      if (becameVisibleWithoutCursor && thinkingTurnKey !== turnKey) {
        thinkingTurnKey = turnKey;
        resetSpring(model.visibilitySpring, 1);
        model.thinkStartedAt = now();
      }
      if (!hasCursor) {
        snapCursor(model, point);
        applyCursorTransform(parts.cursor, model);
        requestFrame();
        return;
      }

      const shouldAppear =
        state.cursor?.moveSequence != null &&
        visible &&
        model.visibilitySpring.value <= 0.001 &&
        appearedTurnKey !== turnKey;
      const distance = distanceBetween(model.point, point);
      model.thinkStartedAt = null;
      if (!animateMovement || shouldAppear || distance < 0.5) {
        if (shouldAppear) {
          appearedTurnKey = turnKey;
          resetSpring(model.visibilitySpring, 1);
        }
        snapCursor(model, point);
        if (!animateMovement) {
          model.stretchSpring.force = 0;
          model.stretchSpring.value = 1;
          model.stretchSpring.velocity = 0;
        }
        applyCursorTransform(parts.cursor, model);
        reportArrival();
        requestFrame();
        return;
      }
      beginCursorMotion(model, point, state.viewportSize);
      forceFirstFrame = true;
      applyCursorTransform(parts.cursor, model);
      requestFrame();
    }
  };
}

function createCursorParts(root, assetUrl) {
  root.replaceChildren();
  root.style.inset = "0";
  root.style.overflow = "hidden";
  root.style.pointerEvents = "none";
  root.style.position = "fixed";
  root.style.zIndex = "2147483647";

  const cursor = document.createElement("div");
  cursor.dataset.testid = "browser-agent-cursor";
  cursor.id = "open-browser-use-cursor";
  cursor.style.height = `${CURSOR_SIZE}px`;
  cursor.style.left = "0";
  cursor.style.position = "absolute";
  cursor.style.top = "0";
  cursor.style.transform = "translate3d(-9999px, -9999px, 0)";
  cursor.style.transformOrigin = `${CURSOR_CENTER}px ${CURSOR_CENTER}px`;
  cursor.style.willChange = "transform, opacity, filter";
  cursor.style.width = `${CURSOR_SIZE}px`;

  const assetWrap = document.createElement("div");
  assetWrap.style.transform = `translate3d(${IMAGE_OFFSET_X}px, ${IMAGE_OFFSET_Y}px, 0)`;

  const image = document.createElement("img");
  image.alt = "";
  image.dataset.browserAgentCursorAsset = "";
  image.dataset.testid = "browser-agent-cursor-asset";
  image.draggable = false;
  image.height = IMAGE_HEIGHT;
  image.id = "open-browser-use-cursor-image";
  image.src = assetUrl;
  image.style.display = "block";
  image.style.transform = `rotate(${IMAGE_ROTATION}deg) scale(1)`;
  image.style.transformOrigin = "0 0";
  image.width = IMAGE_WIDTH;

  assetWrap.appendChild(image);
  cursor.appendChild(assetWrap);
  root.appendChild(cursor);
  return { cursor, layer: root };
}

function createCursorModel(point, visible) {
  const visibility = visible ? 1 : 0;
  const rotation = normalizeDegrees(BASE_ROTATION);
  return {
    motion: null,
    point,
    positionXSpring: createSpring(point.x, point.x, DEFAULT_POSITION_SPRING),
    positionYSpring: createSpring(point.y, point.y, DEFAULT_POSITION_SPRING),
    rotation,
    rotationSpring: createSpring(rotation, rotation, ROTATION_SPRING),
    scootAxisRotation: 0,
    scootAxisSpring: createSpring(0, 0, ROTATION_SPRING),
    scootRotationSpring: createSpring(0, 0, SCOOT_ROTATION_SPRING),
    scootStretchSpring: createSpring(1, 1, SCOOT_STRETCH_SPRING),
    stretchSpring: createSpring(1, 1, STRETCH_SPRING),
    thinkStartedAt: null,
    visibilitySpring: createSpring(visibility, visibility, VISIBILITY_SPRING)
  };
}

function beginCursorMotion(model, targetPoint, viewportSize) {
  model.thinkStartedAt = null;
  const startPoint = { x: model.point.x, y: model.point.y };
  if (distanceBetween(startPoint, targetPoint) <= SHORT_DISTANCE) {
    beginScootMotion(model, startPoint, targetPoint);
    return;
  }
  const path = choosePath({ bounds: viewportSize, end: targetPoint, start: startPoint });
  const springParams = pathSpringParams(path);
  setPositionSpring(model, adjustedPathResponse(springParams.response), springParams.dampingFraction);
  model.motion = {
    mode: "bezier",
    path,
    progressSpring: createSpring(0, 1, springParams)
  };
}

function beginScootMotion(model, startPoint, targetPoint) {
  const scoot = scootGeometry(startPoint, targetPoint);
  setPositionSpring(model, DEFAULT_POSITION_SPRING.response, DEFAULT_POSITION_SPRING.dampingFraction);
  model.positionXSpring.target = targetPoint.x;
  model.positionYSpring.target = targetPoint.y;
  setShortestRotationTarget(model.rotationSpring, normalizeDegrees(BASE_ROTATION));
  setShortestRotationTarget(model.scootAxisSpring, scoot.axisRotation);
  model.motion = {
    axisRotation: scoot.axisRotation,
    end: targetPoint,
    mode: "scoot",
    progressSpring: createSpring(0, 1, SCOOT_PROGRESS_SPRING),
    rotationTarget: scoot.rotationTarget,
    start: startPoint
  };
}

function stepCursor(model, deltaSeconds, timestamp) {
  const arrived = stepMotion(model, Math.max(0, deltaSeconds), timestamp);
  stepSpring(model.visibilitySpring, deltaSeconds);
  stepSpring(model.stretchSpring, deltaSeconds);
  stepSpring(model.scootStretchSpring, deltaSeconds);
  stepSpring(model.scootRotationSpring, deltaSeconds);
  return arrived;
}

function stepMotion(model, deltaSeconds, timestamp) {
  if (model.motion == null) {
    model.stretchSpring.target = 1;
    model.scootStretchSpring.target = 1;
    model.scootRotationSpring.target = 0;
    return false;
  }
  model.thinkStartedAt = null;
  return model.motion.mode === "scoot"
    ? stepScootMotion(model, deltaSeconds, timestamp)
    : stepBezierMotion(model, deltaSeconds, timestamp);
}

function stepBezierMotion(model, deltaSeconds, timestamp) {
  const motion = model.motion;
  if (motion?.mode !== "bezier") {
    return false;
  }
  model.scootStretchSpring.target = 1;
  model.scootRotationSpring.target = 0;
  stepSpring(motion.progressSpring, deltaSeconds);
  const progress = clamp(motion.progressSpring.value, 0, 1);
  const sample = samplePath(motion.path, progress);
  const rotation = angleFromTangent(sample.tangent);
  model.positionXSpring.target = sample.point.x;
  model.positionYSpring.target = sample.point.y;
  setShortestRotationTarget(model.rotationSpring, rotation);
  setShortestRotationTarget(model.scootAxisSpring, 0);
  const movement = stepPosition(model, deltaSeconds);
  model.stretchSpring.target = stretchFromSpeed(movement.speed);
  if (
    progress >= 0.999 &&
    Math.abs(motion.progressSpring.velocity) < 0.01 &&
    isAtPoint(model, sample.point)
  ) {
    const finalSample = samplePath(motion.path, 1);
    const finalRotation = angleFromTangent(finalSample.tangent);
    setPoint(model, finalSample.point);
    resetSpring(model.rotationSpring, finalRotation);
    model.rotation = finalRotation;
    resetSpring(model.scootAxisSpring, 0);
    model.scootAxisRotation = 0;
    resetSpring(model.stretchSpring, 1);
    model.motion = null;
    model.thinkStartedAt = timestamp;
    return true;
  }
  return false;
}

function stepScootMotion(model, deltaSeconds, timestamp) {
  const motion = model.motion;
  if (motion?.mode !== "scoot") {
    return false;
  }
  stepSpring(motion.progressSpring, deltaSeconds);
  model.positionXSpring.target = motion.end.x;
  model.positionYSpring.target = motion.end.y;
  setShortestRotationTarget(model.scootAxisSpring, motion.axisRotation);
  setShortestRotationTarget(model.rotationSpring, normalizeDegrees(BASE_ROTATION));
  const progress = projectedProgress(stepPosition(model, deltaSeconds).point, motion.start, motion.end);
  const eased = Math.sin(Math.min(1, progress) * Math.PI);
  model.stretchSpring.target = 1;
  model.scootStretchSpring.target = scootStretch(progress);
  model.scootRotationSpring.target = motion.rotationTarget * eased;
  if (
    progress >= 0.999 &&
    Math.abs(motion.progressSpring.velocity) < 0.01 &&
    isAtPoint(model, motion.end)
  ) {
    setPoint(model, motion.end);
    resetSpring(model.rotationSpring, normalizeDegrees(BASE_ROTATION));
    model.rotation = model.rotationSpring.value;
    resetScoot(model);
    resetSpring(model.stretchSpring, 1);
    model.motion = null;
    model.thinkStartedAt = timestamp;
    return true;
  }
  return false;
}

function isCursorAnimating(model) {
  return (
    model.motion != null ||
    model.thinkStartedAt != null ||
    !springSettled(model.positionXSpring) ||
    !springSettled(model.positionYSpring) ||
    !springSettled(model.rotationSpring) ||
    !springSettled(model.scootAxisSpring) ||
    !springSettled(model.scootRotationSpring) ||
    !springSettled(model.scootStretchSpring) ||
    !springSettled(model.stretchSpring) ||
    !springSettled(model.visibilitySpring)
  );
}

function applyCursorTransform(cursor, model) {
  const transform = computeCursorTransform({
    point: model.point,
    rotation: idleRotation(model, now()),
    scootAxisRotation: model.scootAxisRotation,
    scootRotation: model.scootRotationSpring.value,
    scootStretch: model.scootStretchSpring.value,
    stretch: model.stretchSpring.value,
    visibility: model.visibilitySpring.value
  });
  cursor.style.transform = transform.transform;
  cursor.style.opacity = `${transform.opacity}`;
  cursor.style.filter = transform.filter;
}

function computeCursorTransform({
  point,
  rotation,
  scootAxisRotation,
  scootRotation,
  scootStretch,
  stretch,
  visibility
}) {
  const opacity = clamp(visibility, 0, 1);
  const visibilityScale = lerp(VISIBILITY_SCALE_MIN, 1, opacity);
  const blur = lerp(VISIBILITY_BLUR_MAX, 0, opacity);
  const verticalStretch = clamp(scootStretch, MIN_SQUASH, 1);
  const transforms = [
    `translate3d(${round(point.x - CURSOR_CENTER)}px, ${round(point.y - CURSOR_CENTER)}px, 0)`
  ];
  if (Math.abs(shortestAngleDelta(0, scootAxisRotation)) > 0.001 || Math.abs(verticalStretch - 1) > 0.001) {
    transforms.push(
      `rotate(${round(scootAxisRotation)}deg)`,
      `scale(1, ${round(verticalStretch)})`,
      `rotate(${round(-scootAxisRotation)}deg)`
    );
  }
  transforms.push(
    `rotate(${round(normalizeDegrees(rotation + scootRotation))}deg)`,
    `scale(${round(stretch * visibilityScale)}, ${round(visibilityScale)})`
  );
  return {
    filter: `blur(${round(blur)}px)`,
    opacity: round(opacity),
    transform: transforms.join(" ")
  };
}

function normalizePoint({ cursorX, cursorY, viewportHeight, viewportWidth }) {
  return {
    x: clamp(cursorX ?? Math.round(viewportWidth * DEFAULT_X_RATIO), 0, viewportWidth),
    y: clamp(cursorY ?? Math.round(viewportHeight * DEFAULT_Y_RATIO), 0, viewportHeight)
  };
}

function idleRotation(model, timestamp) {
  if (model.thinkStartedAt == null) {
    return model.rotation;
  }
  const elapsed = (timestamp - model.thinkStartedAt) / 1000 - THINK_DELAY_SECONDS;
  if (elapsed < 0) {
    return model.rotation;
  }
  const envelopeProgress = Math.min(1, elapsed / THINK_DURATION_SECONDS);
  const envelope = Math.sin(envelopeProgress * Math.PI);
  const swing = Math.sin((elapsed / THINK_PERIOD_SECONDS) * Math.PI * 2) * envelope;
  if (envelopeProgress >= 1) {
    model.thinkStartedAt = null;
    return model.rotation;
  }
  return model.rotation + swing * THINK_ROTATION_DEGREES;
}

function stretchFromSpeed(speed) {
  return clamp(1 - speed / 5500, 0.65, 1);
}

function scootStretch(progress) {
  return lerp(1, lerp(1, MIN_SQUASH, Math.sin(clamp(progress, 0, 1) * Math.PI)), SCOOT_STRETCH_MIX);
}

function adjustedPathResponse(response) {
  return clamp(response * 0.18, 0.035, 0.12);
}

function setPositionSpring(model, response, dampingFraction) {
  model.positionXSpring.response = response;
  model.positionYSpring.response = response;
  model.positionXSpring.dampingFraction = dampingFraction;
  model.positionYSpring.dampingFraction = dampingFraction;
}

function stepPosition(model, deltaSeconds) {
  const previous = model.point;
  stepSpring(model.positionXSpring, deltaSeconds);
  stepSpring(model.positionYSpring, deltaSeconds);
  stepSpring(model.rotationSpring, deltaSeconds);
  stepSpring(model.scootAxisSpring, deltaSeconds);
  const point = { x: model.positionXSpring.value, y: model.positionYSpring.value };
  const speed = distanceBetween(previous, point) / Math.max(deltaSeconds, INTEGRATION_STEP_SECONDS);
  model.point = point;
  model.rotation = model.rotationSpring.value;
  model.scootAxisRotation = model.scootAxisSpring.value;
  return { point, speed };
}

function isAtPoint(model, point) {
  return (
    distanceBetween(model.point, point) <= ARRIVAL_DISTANCE &&
    Math.abs(model.positionXSpring.velocity) <= ARRIVAL_VELOCITY &&
    Math.abs(model.positionYSpring.velocity) <= ARRIVAL_VELOCITY
  );
}

function setPoint(model, point) {
  model.point = point;
  resetSpring(model.positionXSpring, point.x);
  resetSpring(model.positionYSpring, point.y);
}

function snapCursor(model, point) {
  model.motion = null;
  setPoint(model, point);
  resetSpring(model.rotationSpring, normalizeDegrees(BASE_ROTATION));
  model.rotation = model.rotationSpring.value;
  resetScoot(model);
  resetSpring(model.stretchSpring, 1);
}

function resetScoot(model) {
  resetSpring(model.scootAxisSpring, 0);
  resetSpring(model.scootRotationSpring, 0);
  resetSpring(model.scootStretchSpring, 1);
  model.scootAxisRotation = 0;
}

function scootGeometry(start, end) {
  const direction = normalizeVector({ x: end.x - start.x, y: end.y - start.y });
  return {
    axisRotation: vectorDegrees(direction),
    rotationTarget: clamp(direction.x * 0.75 + -direction.y * 0.62, -1, 1) * SCOOT_ROTATION_LIMIT
  };
}

function projectedProgress(point, start, end) {
  const vector = { x: end.x - start.x, y: end.y - start.y };
  const lengthSquared = vector.x * vector.x + vector.y * vector.y;
  if (lengthSquared < 0.001) {
    return 1;
  }
  return clamp(((point.x - start.x) * vector.x + (point.y - start.y) * vector.y) / lengthSquared, 0, 1);
}

function setShortestRotationTarget(spring, target) {
  spring.target = spring.value + shortestAngleDelta(spring.value, target);
}

function shortestAngleDelta(from, to) {
  let delta = to - from;
  while (delta > 180) {
    delta -= 360;
  }
  while (delta < -180) {
    delta += 360;
  }
  return delta;
}

function createSpring(value, target, params) {
  return {
    dampingFraction: params.dampingFraction,
    force: 0,
    response: params.response,
    simulationTime: 0,
    scriptTime: 0,
    target,
    value,
    velocity: 0
  };
}

function resetSpring(spring, value) {
  spring.force = 0;
  spring.simulationTime = 0;
  spring.scriptTime = 0;
  spring.target = value;
  spring.value = value;
  spring.velocity = 0;
}

function stepSpring(spring, deltaSeconds) {
  const response = Math.max(0.001, spring.response);
  const maxStiffness = 1 / (2 * INTEGRATION_STEP_SECONDS ** 2);
  const stiffness = Math.min((Math.PI * 2) ** 2 / response ** 2, maxStiffness);
  const damping = Math.sqrt(stiffness) * 2 * spring.dampingFraction;
  spring.scriptTime += Math.max(0, deltaSeconds);
  if (spring.scriptTime - spring.simulationTime > MAX_SCRIPT_LAG_SECONDS) {
    spring.simulationTime = spring.scriptTime - MIN_FRAME_SECONDS;
  }
  while (spring.simulationTime < spring.scriptTime) {
    integrateSpring(spring, stiffness, damping);
    spring.simulationTime += INTEGRATION_STEP_SECONDS;
  }
  if (springAtRest(spring)) {
    spring.value = spring.target;
  }
}

function integrateSpring(spring, stiffness, damping) {
  const halfStep = INTEGRATION_STEP_SECONDS / 2;
  const midpointVelocity = spring.velocity + spring.force * halfStep;
  spring.value += midpointVelocity * INTEGRATION_STEP_SECONDS;
  spring.force = midpointVelocity * -damping + (spring.target - spring.value) * stiffness;
  spring.velocity = midpointVelocity + spring.force * halfStep;
}

function springAtRest(spring) {
  if (Math.max(spring.velocity * spring.velocity, spring.force * spring.force) > SPRING_SETTLE_EPSILON * SPRING_SETTLE_EPSILON) {
    return false;
  }
  const targetWindow = spring.target * 0.01;
  const delta = spring.target - spring.value;
  return targetWindow === 0 || delta * delta <= targetWindow * targetWindow;
}

function springSettled(spring) {
  return spring.value === spring.target && springAtRest(spring);
}

function choosePath({ bounds, end, start }) {
  return selectPath(buildPathCandidates({ bounds, config: PATH_CONFIG, end, start }), bounds, PATH_CONFIG);
}

function samplePath(path, progress) {
  const boundedProgress = clamp(progress, 0, 1);
  const segmentIndexFloat =
    boundedProgress === 1 ? path.segments.length - 1 : boundedProgress * path.segments.length;
  const segmentIndex = Math.floor(segmentIndexFloat);
  const segment = path.segments[segmentIndex];
  if (!segment) {
    throw new Error("Cursor motion path has no segment for progress");
  }
  const previousSegment = path.segments[segmentIndex - 1];
  const start = segmentIndex === 0 ? path.start : previousSegment?.end;
  if (!start) {
    throw new Error("Cursor motion path segment is missing its start point");
  }
  const segmentProgress = boundedProgress === 1 ? 1 : segmentIndexFloat - segmentIndex;
  return {
    point: cubicPoint(start, segment, segmentProgress),
    tangent: cubicTangent(start, segment, segmentProgress)
  };
}

function angleFromTangent(tangent) {
  if (distanceBetween({ x: 0, y: 0 }, tangent) < 0.001) {
    return normalizeDegrees(BASE_ROTATION);
  }
  const normal = normalizeVector(tangent);
  return normalizeDegrees(Math.atan2(normal.y, normal.x) * (180 / Math.PI) + 90);
}

function pathSpringParams(path) {
  return { dampingFraction: 0.9, response: pathResponse(path) };
}

function buildPathCandidates({ bounds, config, end, start }) {
  const clickDirection = vectorFromDegrees(config.clickAngleDegrees);
  const distance = distanceBetween(start, end);
  const vector = { x: end.x - start.x, y: end.y - start.y };
  const direction = normalizeVector(vector);
  const startHandleDistance = Math.max(48, Math.min(640, distance * config.startHandle, distance * 0.9));
  const endHandleDistance = Math.max(48, Math.min(640, distance * config.endpointHandle, distance * 0.9));
  const reverseClickDirection = { x: -clickDirection.x, y: -clickDirection.y };
  const startControl = boundedControlPoint(bounds, start, clickDirection, startHandleDistance);
  const endControl = boundedControlPoint(bounds, end, reverseClickDirection, endHandleDistance);
  const naturalNormal = { x: -direction.y, y: direction.x };
  const normalSign = naturalNormal.x * clickDirection.x + naturalNormal.y * clickDirection.y >= 0 ? 1 : -1;
  const arcNormal = { x: naturalNormal.x * normalSign, y: naturalNormal.y * normalSign };
  const midpoint = { x: (start.x + end.x) / 2, y: (start.y + end.y) / 2 };
  const closerStartControl = boundedControlPoint(bounds, start, clickDirection, startHandleDistance * 0.65);
  const closerEndControl = boundedControlPoint(bounds, end, reverseClickDirection, endHandleDistance * 0.65);
  const arcTangent = normalizeVector(vector);
  const arcDistance = Math.max(50, Math.min(520, distance * config.arcSize));
  const arcHandleDistance = Math.max(38, Math.min(440, distance * config.arcFlow));
  const arcDistanceScales = [0.55, 0.8, 1.05];
  const arcHandleScales = [0.65, 1, 1.35];
  const candidates = [
    directPath(start, end, startControl, endControl),
    directPath(start, end, closerStartControl, closerEndControl)
  ];
  for (const arcDistanceScale of arcDistanceScales) {
    for (const arcHandleScale of arcHandleScales) {
      pushArcCandidates({
        arcDistanceBase: arcDistance,
        arcDistanceScale,
        arcHandleDistanceBase: arcHandleDistance,
        arcHandleScale,
        arcTangent,
        candidates,
        clickTangent: clickDirection,
        end,
        endControl,
        midpoint,
        naturalArcNormal: arcNormal,
        start,
        startControl,
        startControlDistance: startHandleDistance
      });
    }
  }
  return candidates.slice(0, config.candidateCount);
}

function pushArcCandidates(options) {
  pushArcCandidate({ ...options, arcNormal: options.naturalArcNormal });
  pushArcCandidate({
    ...options,
    arcNormal: { x: -options.naturalArcNormal.x, y: -options.naturalArcNormal.y }
  });
}

function pushArcCandidate({
  arcDistanceBase,
  arcDistanceScale,
  arcHandleDistanceBase,
  arcHandleScale,
  arcNormal,
  arcTangent,
  candidates,
  clickTangent,
  end,
  endControl,
  midpoint,
  start,
  startControl,
  startControlDistance
}) {
  const arcDistance = arcDistanceBase * arcDistanceScale;
  const arcHandleDistance = arcHandleDistanceBase * arcHandleScale;
  const arc = {
    x: midpoint.x + arcNormal.x * arcDistance + clickTangent.x * startControlDistance * 0.16,
    y: midpoint.y + arcNormal.y * arcDistance + clickTangent.y * startControlDistance * 0.16
  };
  const arcIn = { x: arc.x - arcTangent.x * arcHandleDistance, y: arc.y - arcTangent.y * arcHandleDistance };
  const arcOut = { x: arc.x + arcTangent.x * arcHandleDistance, y: arc.y + arcTangent.y * arcHandleDistance };
  candidates.push(arcPath({ arc, arcIn, arcOut, end, endControl, start, startControl }));
}

function directPath(start, end, startControl, endControl) {
  return {
    arc: null,
    arcIn: null,
    arcOut: null,
    end,
    endControl,
    segments: [{ control1: startControl, control2: endControl, end }],
    start,
    startControl
  };
}

function arcPath({ arc, arcIn, arcOut, end, endControl, start, startControl }) {
  return {
    arc,
    arcIn,
    arcOut,
    end,
    endControl,
    segments: [
      { control1: startControl, control2: arcIn, end: arc },
      { control1: arcOut, control2: endControl, end }
    ],
    start,
    startControl
  };
}

function selectPath(candidates, bounds, config) {
  const first = candidates[0];
  if (!first) {
    throw new Error("Cursor motion requires at least one candidate");
  }
  let bestInBounds = first;
  let bestInBoundsScore = Number.POSITIVE_INFINITY;
  let bestAny = first;
  let bestAnyScore = Number.POSITIVE_INFINITY;
  for (const candidate of candidates) {
    const metrics = pathMetrics(candidate, bounds, config.boundsMargin);
    const score = pathScore(candidate, metrics);
    if (score < bestAnyScore) {
      bestAny = candidate;
      bestAnyScore = score;
    }
    if (metrics.staysInBounds && score < bestInBoundsScore) {
      bestInBounds = candidate;
      bestInBoundsScore = score;
    }
  }
  return bestInBoundsScore === Number.POSITIVE_INFINITY ? bestAny : bestInBounds;
}

function pathMetrics(path, bounds, margin) {
  let length = 0;
  let angleChangeEnergy = 0;
  let maxAngleChange = 0;
  let totalTurn = 0;
  let previousAngle = null;
  let staysInBounds = !bounds || pointInBounds(path.start, bounds, margin);
  let segmentStart = path.start;
  let previousPoint = path.start;
  for (const segment of path.segments) {
    for (let i = 1; i <= 24; i += 1) {
      const progress = i / 24;
      const point = cubicPointFromControls(segmentStart, segment.control1, segment.control2, segment.end, progress);
      length += distanceBetween(previousPoint, point);
      if (bounds) {
        staysInBounds = staysInBounds && pointInBounds(point, bounds, margin);
      }
      const delta = { x: point.x - previousPoint.x, y: point.y - previousPoint.y };
      if (distanceBetween({ x: 0, y: 0 }, delta) > 0.01) {
        const angle = Math.atan2(delta.y, delta.x);
        if (previousAngle != null) {
          const angleDelta = shortestRadiansDelta(previousAngle, angle);
          angleChangeEnergy += angleDelta * angleDelta;
          maxAngleChange = Math.max(maxAngleChange, Math.abs(angleDelta));
          totalTurn += Math.abs(angleDelta);
        }
        previousAngle = angle;
      }
      previousPoint = point;
    }
    segmentStart = segment.end;
  }
  return { angleChangeEnergy, length, maxAngleChange, staysInBounds, totalTurn };
}

function pathScore(path, metrics) {
  const directDistance = Math.max(1, distanceBetween(path.start, path.end));
  const extraLength = Math.max(0, metrics.length / directDistance - 1);
  const arcPenalty = path.arc == null ? 0 : 45;
  const directionPenalty = pathDirectionPenalty(path);
  return (
    metrics.length +
    extraLength * 320 +
    metrics.angleChangeEnergy * 140 +
    metrics.maxAngleChange * 180 +
    metrics.totalTurn * 18 +
    directionPenalty * 90 +
    arcPenalty
  );
}

function pathDirectionPenalty(path) {
  const clickDirection = vectorFromDegrees(BASE_ROTATION);
  const direction = normalizeVector({ x: path.end.x - path.start.x, y: path.end.y - path.start.y });
  return clamp((-(direction.x * clickDirection.x + direction.y * clickDirection.y) - 0.08) / 0.92, 0, 1);
}

function pathResponse(path) {
  const metrics = pathMetrics(path);
  const directDistance = Math.max(1, distanceBetween(path.start, path.end));
  const extraLength = Math.max(0, metrics.length / directDistance - 1);
  const distanceFactor = clamp((metrics.length - 180) / 760, 0, 1);
  const extraFactor = clamp(extraLength / 0.55, 0, 1);
  const turnFactor = clamp(metrics.totalTurn / (Math.PI * 1.4), 0, 1);
  const energyFactor = clamp(metrics.angleChangeEnergy / 1.25, 0, 1);
  const complexity = clamp(extraFactor * 0.42 + turnFactor * 0.38 + energyFactor * 0.2, 0, 1);
  const directionFactor = pathDirectionPenalty(path);
  const arcBonus = path.arc == null ? 0 : 0.04;
  const directionBonus = directionFactor * 0.28;
  const arcMultiplier = path.arc == null ? 1 : 0.9;
  return clamp((0.42 + distanceFactor * 0.22 + complexity * 0.12 + directionBonus + arcBonus) * 0.7 * arcMultiplier, 0.12, 2.2);
}

function boundedControlPoint(bounds, point, direction, distance) {
  let allowedDistance = distance;
  if (direction.x < 0) {
    allowedDistance = Math.min(allowedDistance, point.x / -direction.x);
  }
  if (direction.x > 0) {
    allowedDistance = Math.min(allowedDistance, (bounds.width - point.x) / direction.x);
  }
  if (direction.y < 0) {
    allowedDistance = Math.min(allowedDistance, point.y / -direction.y);
  }
  if (direction.y > 0) {
    allowedDistance = Math.min(allowedDistance, (bounds.height - point.y) / direction.y);
  }
  return {
    x: point.x + direction.x * Math.max(0, allowedDistance),
    y: point.y + direction.y * Math.max(0, allowedDistance)
  };
}

function vectorFromDegrees(degrees) {
  const radians = degrees * (Math.PI / 180);
  return { x: Math.sin(radians), y: -Math.cos(radians) };
}

function cubicPoint(start, segment, progress) {
  return cubicPointFromControls(start, segment.control1, segment.control2, segment.end, progress);
}

function cubicPointFromControls(start, control1, control2, end, progress) {
  const inverse = 1 - progress;
  const a = inverse * inverse * inverse;
  const b = 3 * inverse * inverse * progress;
  const c = 3 * inverse * progress * progress;
  const d = progress * progress * progress;
  return {
    x: start.x * a + control1.x * b + control2.x * c + end.x * d,
    y: start.y * a + control1.y * b + control2.y * c + end.y * d
  };
}

function cubicTangent(start, segment, progress) {
  const inverse = 1 - progress;
  return {
    x:
      3 * inverse * inverse * (segment.control1.x - start.x) +
      6 * inverse * progress * (segment.control2.x - segment.control1.x) +
      3 * progress * progress * (segment.end.x - segment.control2.x),
    y:
      3 * inverse * inverse * (segment.control1.y - start.y) +
      6 * inverse * progress * (segment.control2.y - segment.control1.y) +
      3 * progress * progress * (segment.end.y - segment.control2.y)
  };
}

function normalizeVector(vector) {
  const length = Math.sqrt(vector.x * vector.x + vector.y * vector.y);
  return length < 0.001 ? { x: 1, y: 0 } : { x: vector.x / length, y: vector.y / length };
}

function pointInBounds(point, bounds, margin) {
  return (
    point.x >= margin &&
    point.x <= bounds.width - margin &&
    point.y >= margin &&
    point.y <= bounds.height - margin
  );
}

function shortestRadiansDelta(from, to) {
  let delta = to - from;
  while (delta > Math.PI) {
    delta -= Math.PI * 2;
  }
  while (delta < -Math.PI) {
    delta += Math.PI * 2;
  }
  return delta;
}

function vectorDegrees(vector) {
  return distanceBetween({ x: 0, y: 0 }, vector) < 0.001
    ? 0
    : Math.atan2(vector.y, vector.x) * (180 / Math.PI);
}

function normalizeDegrees(degrees) {
  const value = degrees % 360;
  return value < 0 ? value + 360 : value;
}

function distanceBetween(a, b) {
  const x = b.x - a.x;
  const y = b.y - a.y;
  return Math.sqrt(x * x + y * y);
}

function clamp(value, min, max) {
  return Math.max(min, Math.min(max, value));
}

function lerp(from, to, progress) {
  return from + (to - from) * progress;
}

function round(value) {
  return Math.round(value * 1000) / 1000;
}

function now() {
  return typeof performance === "undefined" ? Date.now() : performance.now();
}

function requestCursorFrame(callback) {
  if (typeof window === "undefined") {
    callback(now());
    return 0;
  }
  if (window.requestAnimationFrame == null) {
    return window.setTimeout(() => callback(now()), MIN_FRAME_SECONDS * 1000);
  }
  let done = false;
  const frame = {
    rafId: 0,
    timeoutId: 0
  };
  const run = (timestamp) => {
    if (done) {
      return;
    }
    done = true;
    window.clearTimeout(frame.timeoutId);
    window.cancelAnimationFrame(frame.rafId);
    callback(timestamp);
  };
  frame.rafId = window.requestAnimationFrame(run);
  frame.timeoutId = window.setTimeout(() => run(now()), MIN_FRAME_SECONDS * 1000);
  return frame;
}

function cancelCursorFrame(id) {
  if (typeof window === "undefined") {
    return;
  }
  if (typeof id === "object" && id !== null) {
    window.clearTimeout(id.timeoutId);
    window.cancelAnimationFrame?.(id.rafId);
    return;
  }
  if (window.cancelAnimationFrame != null) {
    window.cancelAnimationFrame(id);
    return;
  }
  window.clearTimeout(id);
}
