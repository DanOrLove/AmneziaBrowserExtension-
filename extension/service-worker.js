const HOST_NAME = "com.amnezia.browser_vpn";
const PROXY_PORT = 1080;
const STATE_KEY = "vpnState";

function invokeHost(command, config = "") {
  return new Promise((resolve, reject) => {
    chrome.runtime.sendNativeMessage(HOST_NAME, { command, config }, (response) => {
      const error = chrome.runtime.lastError;
      if (error) { reject(new Error(error.message)); return; }
      if (!response?.ok) { reject(new Error(response?.message || "Локальный компонент вернул ошибку.")); return; }
      resolve(response);
    });
  });
}

function proxyValue() {
  return {
    mode: "fixed_servers",
    rules: {
      singleProxy: { scheme: "socks5", host: "127.0.0.1", port: PROXY_PORT },
      bypassList: ["127.0.0.1", "[::1]", "localhost", "<local>"]
    }
  };
}

function getProxySettings() {
  return new Promise((resolve, reject) => {
    chrome.proxy.settings.get({ incognito: false }, (details) => {
      const error = chrome.runtime.lastError;
      if (error) reject(new Error(error.message));
      else resolve(details);
    });
  });
}

function setProxy(settings) {
  return new Promise((resolve, reject) => {
    chrome.proxy.settings.set({ value: settings, scope: "regular" }, () => {
      const error = chrome.runtime.lastError;
      if (error) reject(new Error(error.message));
      else resolve();
    });
  });
}

function clearProxy() {
  return new Promise((resolve, reject) => {
    chrome.proxy.settings.clear({ scope: "regular" }, () => {
      const error = chrome.runtime.lastError;
      if (error) reject(new Error(error.message));
      else resolve();
    });
  });
}

function getWebRTCPolicy() {
  return new Promise((resolve, reject) => {
    chrome.privacy.network.webRTCIPHandlingPolicy.get({ incognito: false }, (details) => {
      const error = chrome.runtime.lastError;
      if (error) reject(new Error(error.message));
      else resolve(details);
    });
  });
}

function setWebRTCPolicy(value) {
  return new Promise((resolve, reject) => {
    chrome.privacy.network.webRTCIPHandlingPolicy.set({ value, scope: "regular" }, () => {
      const error = chrome.runtime.lastError;
      if (error) reject(new Error(error.message));
      else resolve();
    });
  });
}

function clearWebRTCPolicy() {
  return new Promise((resolve, reject) => {
    chrome.privacy.network.webRTCIPHandlingPolicy.clear({ scope: "regular" }, () => {
      const error = chrome.runtime.lastError;
      if (error) reject(new Error(error.message));
      else resolve();
    });
  });
}

async function restorePreviousProxy(state) {
  if (state?.previousProxy?.value && state.previousProxy.levelOfControl === "controllable_by_this_extension") {
    await setProxy(state.previousProxy.value);
  } else {
    await clearProxy();
  }
  if (state?.previousWebRTC?.value && state.previousWebRTC.levelOfControl === "controllable_by_this_extension") {
    await setWebRTCPolicy(state.previousWebRTC.value);
  } else {
    await clearWebRTCPolicy();
  }
}

async function connect(profile) {
  if (!profile?.config || !profile.id) throw new Error("Сначала выберите сохранённый конфиг.");
  const current = await getProxySettings();
  if (current.levelOfControl !== "controlled_by_this_extension" && current.levelOfControl !== "controllable_by_this_extension") {
    throw new Error("Настройки прокси Chromium контролируются другим расширением или политикой.");
  }
  const currentWebRTC = await getWebRTCPolicy();
  if (currentWebRTC.levelOfControl !== "controlled_by_this_extension" && currentWebRTC.levelOfControl !== "controllable_by_this_extension") {
    throw new Error("Настройка WebRTC контролируется политикой или другим расширением, поэтому нельзя исключить прямой UDP-трафик браузера.");
  }

  const oldState = await chrome.storage.local.get(STATE_KEY);
  let previousProxy = oldState[STATE_KEY]?.previousProxy;
  let previousWebRTC = oldState[STATE_KEY]?.previousWebRTC;
  if (!oldState[STATE_KEY]?.active) { previousProxy = current; previousWebRTC = currentWebRTC; }
  const response = await invokeHost("start", profile.config);
  try {
    await setProxy(proxyValue());
    await setWebRTCPolicy("disable_non_proxied_udp");
  } catch (error) {
    const stopped = await invokeHost("stop").then(() => true).catch(() => false);
    const pendingState = { active: true, profileId: profile.id, profileName: profile.name, previousProxy, previousWebRTC, tunnelStopped: stopped };
    try {
      await restorePreviousProxy(pendingState);
    } catch {
      await chrome.storage.local.set({ [STATE_KEY]: pendingState });
    }
    throw error;
  }
  const state = { active: true, profileId: profile.id, profileName: profile.name, previousProxy, previousWebRTC, startedAt: new Date().toISOString() };
  await chrome.storage.local.set({ [STATE_KEY]: state });
  return { active: true, message: response.message };
}

async function disconnect() {
  const stored = await chrome.storage.local.get(STATE_KEY);
  const state = stored[STATE_KEY];
  // Stop the tunnel first. The browser remains pointed at the local SOCKS port
  // until it is restored, so a failed stop cannot send requests outside VPN.
  const response = await invokeHost("stop");
  try {
    await restorePreviousProxy(state);
  } catch (error) {
    await chrome.storage.local.set({ [STATE_KEY]: { ...state, tunnelStopped: true } });
    throw error;
  }
  await chrome.storage.local.set({ [STATE_KEY]: { active: false } });
  return { active: false, message: response.message };
}

async function syncStatus() {
  const stored = await chrome.storage.local.get(STATE_KEY);
  const state = stored[STATE_KEY] || { active: false };
  if (!state.active) {
    try {
      const orphan = await invokeHost("status");
      if (orphan.running) await invokeHost("stop");
    } catch { /* Native host may not be installed yet. */ }
    return { active: false };
  }
  if (state.tunnelStopped) {
    await restorePreviousProxy(state);
    await chrome.storage.local.set({ [STATE_KEY]: { active: false } });
    return { active: false, message: "Локальный туннель остановлен." };
  }
  try {
    const response = await invokeHost("status");
    if (response.running) return { active: true, profileName: state.profileName };
  } catch { /* The helper may not be installed or its process may have exited. */ }
  await restorePreviousProxy(state).catch(() => {});
  await chrome.storage.local.set({ [STATE_KEY]: { active: false } });
  return { active: false, message: "Локальный VPN-компонент остановлен." };
}

chrome.runtime.onMessage.addListener((message, _sender, sendResponse) => {
  (async () => {
    if (message?.type === "vpn:connect") return connect(message.profile);
    if (message?.type === "vpn:disconnect") return disconnect();
    if (message?.type === "vpn:status") return syncStatus();
    throw new Error("Неизвестная команда.");
  })().then(sendResponse).catch((error) => sendResponse({ error: error.message || String(error) }));
  return true;
});

chrome.runtime.onStartup.addListener(() => { syncStatus(); });
