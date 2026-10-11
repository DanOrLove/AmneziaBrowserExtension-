const STORAGE_KEY = "amneziaProfiles";
const byId = (id) => document.getElementById(id);
const profileSelect = byId("profiles");
const fileInput = byId("config-file");
const configText = byId("config-text");
const configName = byId("config-name");
const notice = byId("notice");

function setNotice(message, isError = false) {
  notice.textContent = message;
  notice.classList.toggle("error", isError);
}

function parseConfig(source) {
  const sections = new Set();
  const values = new Map();
  let current = "";
  for (const rawLine of source.split(/\r?\n/)) {
    const line = rawLine.trim();
    if (!line || line.startsWith("#") || line.startsWith(";")) continue;
    const section = line.match(/^\[([^\]]+)\]$/);
    if (section) { current = section[1].toLowerCase(); sections.add(current); continue; }
    const pair = line.match(/^([A-Za-z][A-Za-z0-9_]*)\s*=\s*(.*?)\s*$/);
    if (!pair || !current) continue;
    const key = pair[1].toLowerCase();
    if (!values.has(`${current}.${key}`)) values.set(`${current}.${key}`, pair[2]);
  }
  const required = ["interface.privatekey", "interface.address", "peer.publickey", "peer.endpoint", "peer.allowedips"];
  const missing = required.filter((key) => !values.get(key));
  if (!sections.has("interface") || !sections.has("peer") || missing.length) {
    throw new Error("Не удалось распознать AmneziaWG/WireGuard конфиг. Проверьте секции [Interface], [Peer] и обязательные поля.");
  }
  return {
    endpoint: values.get("peer.endpoint"),
    address: values.get("interface.address"),
    protocol: ["s1", "s2", "s3", "s4", "jc", "jmin", "jmax", "h1", "h2", "h3", "h4", "i1", "i2"].some((key) => values.has(`interface.${key}`)) ? "AmneziaWG" : "WireGuard"
  };
}

async function loadProfiles() {
  const stored = await chrome.storage.local.get(STORAGE_KEY);
  return Array.isArray(stored[STORAGE_KEY]) ? stored[STORAGE_KEY] : [];
}

async function renderProfiles(selectId) {
  const profiles = await loadProfiles();
  profileSelect.replaceChildren();
  byId("profile-count").textContent = String(profiles.length);
  profileSelect.disabled = profiles.length === 0;
  byId("delete-profile").disabled = profiles.length === 0;
  byId("connect").disabled = profiles.length === 0;
  if (profiles.length === 0) {
    profileSelect.add(new Option("Нет сохранённых конфигов", ""));
    byId("profile-details").textContent = "Добавьте конфиг AmneziaWG в формате .conf.";
    return;
  }
  for (const profile of profiles) profileSelect.add(new Option(profile.name, profile.id));
  profileSelect.value = selectId && profiles.some((item) => item.id === selectId) ? selectId : profiles[0].id;
  const active = profiles.find((item) => item.id === profileSelect.value);
  byId("profile-details").textContent = `${active.meta.protocol} · ${active.meta.endpoint} · ${active.meta.address}`;
}

async function renderConnection() {
  const response = await chrome.runtime.sendMessage({ type: "vpn:status" });
  const connected = Boolean(response?.active);
  byId("status-dot").classList.toggle("connected", connected);
  byId("connection-title").textContent = connected ? `Туннель активен · ${response.profileName || "AmneziaWG"}` : "Туннель не подключён";
  byId("connection-subtitle").textContent = connected ? "Веб-трафик Chromium направлен в локальный VPN прокси" : "Выберите профиль и нажмите «Подключить»";
  const button = byId("connect");
  button.textContent = connected ? "Отключить VPN" : "Подключить";
  button.classList.toggle("connected", connected);
  button.disabled = !connected && profileSelect.disabled;
}

fileInput.addEventListener("change", async () => {
  const file = fileInput.files?.[0];
  if (!file) return;
  try {
    configText.value = await file.text();
    if (!configName.value.trim()) configName.value = file.name.replace(/\.conf$/i, "");
    setNotice(`Файл «${file.name}» загружен. Нажмите «Сохранить конфиг».`);
  } catch {
    setNotice("Не удалось прочитать этот файл.", true);
  } finally {
    fileInput.value = "";
  }
});

byId("save-config").addEventListener("click", async () => {
  const source = configText.value.trim();
  if (!source) { setNotice("Выберите файл или вставьте текст конфига.", true); return; }
  let meta;
  try { meta = parseConfig(source); }
  catch (error) { setNotice(error.message, true); return; }
  const profiles = await loadProfiles();
  const id = crypto.randomUUID();
  const name = configName.value.trim() || `Конфиг ${profiles.length + 1}`;
  profiles.push({ id, name, config: source, meta, createdAt: new Date().toISOString() });
  await chrome.storage.local.set({ [STORAGE_KEY]: profiles });
  configText.value = "";
  configName.value = "";
  setNotice("Конфиг сохранён локально.");
  await renderProfiles(id);
});

profileSelect.addEventListener("change", () => renderProfiles(profileSelect.value));

byId("connect").addEventListener("click", async () => {
  const button = byId("connect");
  const connected = button.classList.contains("connected");
  button.disabled = true;
  setNotice(connected ? "Отключаю VPN…" : "Запускаю AmneziaWG…");
  try {
    let response;
    if (connected) {
      response = await chrome.runtime.sendMessage({ type: "vpn:disconnect" });
    } else {
      const profiles = await loadProfiles();
      const profile = profiles.find((item) => item.id === profileSelect.value);
      response = await chrome.runtime.sendMessage({ type: "vpn:connect", profile });
    }
    if (response?.error) throw new Error(response.error);
    setNotice(response?.message || (connected ? "VPN отключён." : "VPN включён для Chromium."));
  } catch (error) {
    setNotice(error.message || "Не удалось изменить состояние VPN.", true);
  } finally {
    await renderConnection();
  }
});

byId("delete-profile").addEventListener("click", async () => {
  const profiles = await loadProfiles();
  const remaining = profiles.filter((item) => item.id !== profileSelect.value);
  await chrome.storage.local.set({ [STORAGE_KEY]: remaining });
  setNotice("Конфиг удалён.");
  await renderProfiles();
});

renderProfiles().then(renderConnection);
