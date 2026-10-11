# Amnezia Browser VPN

Chromium extension for sending the browser's web traffic through an AmneziaWG configuration. The extension imports local `.conf` profiles and controls a Windows Native Messaging host. The host runs AmneziaWG in a userspace network stack and exposes a loopback SOCKS5 proxy; Windows system routes and other applications are not changed.

> **Status:** early prototype. The native component has been built for Windows x64. Test with a profile you control before relying on it. See [Limitations and security](#limitations-and-security).

## Русский

### Возможности

- Импорт конфигурации AmneziaWG/WireGuard из файла `.conf` или вставкой текста.
- Локальное хранение нескольких профилей, выбор и удаление профиля.
- Подключение и отключение из всплывающего окна расширения.
- Веб-трафик Chromium направляется через локальный SOCKS5 и AmneziaWG userspace-туннель.
- Системные маршруты Windows и маршруты остальных приложений не меняются.
- При подключении расширение блокирует WebRTC UDP, который не может пройти через SOCKS5.

### Требования

- Windows 10/11 x64.
- Chromium, Google Chrome или Microsoft Edge.
- Go 1.25 или новее для сборки локального компонента.
- Конфиг AmneziaWG в формате WireGuard INI (`[Interface]`, `[Peer]`).

Установка и регистрация компонента выполняются для текущего пользователя без прав администратора. Для использования администраторами или другими Windows-учётными записями компонент нужно собрать и зарегистрировать в их профилях.

### Установка

1. Скачайте ZIP репозитория с GitHub и распакуйте его в постоянную локальную папку. Не удаляйте и не перемещайте папку после регистрации Native Messaging: запись Windows указывает на полный путь к собранной программе.
2. Откройте PowerShell в корневой папке проекта и выполните:

   ```powershell
   powershell -ExecutionPolicy Bypass -File .\native\build.ps1
   powershell -ExecutionPolicy Bypass -File .\native\install.ps1
   ```

   Сборка скачивает Go-модуль `github.com/amnezia-vpn/amneziawg-go/v3` и создаёт `native\AmneziaBrowserVpnHost.exe`. Скрипт установки регистрирует Native Messaging в HKCU для Chrome и Edge и привязывает его к ID расширения из `extension\manifest.json`.

3. В Chrome откройте `chrome://extensions`, в Edge — `edge://extensions`.
4. Включите **Режим разработчика**, нажмите **Загрузить распакованное расширение** и выберите папку `extension` внутри распакованного репозитория.
5. Подтвердите разрешения расширения для прокси, настроек приватности и Native Messaging.

Если браузер был открыт во время сборки и регистрации, перезагрузите расширение на странице расширений или перезапустите браузер.

### macOS и Linux

**Важно:** текущая версия не устанавливает VPN-туннель на macOS или Linux. Native host использует Windows API, native/build.ps1 собирает EXE, а native/install.ps1 регистрирует Native Messaging через реестр Windows.

Отдельный репозиторий для macOS-версии: [DanOrLove/AmneziaBrowserExtensionMacOS](https://github.com/DanOrLove/AmneziaBrowserExtensionMacOS).

### Если нужно только открыть интерфейс расширения:

1. Скачайте и распакуйте репозиторий.
2. В Chrome или Chromium откройте chrome://extensions.
3. Включите режим разработчика, нажмите **Загрузить распакованное расширение** и выберите папку extension.
4. Интерфейс можно осмотреть, но импорт профиля не подключит VPN. При нажатии **Подключить** native host не запустится, и трафик через туннель не пойдёт.

Не запускайте native/build.ps1 и native/install.ps1 на этих системах. Для полноценной поддержки нужно отдельно адаптировать native host под macOS/Linux, добавить сборку и регистрацию Native Messaging для каждой ОС, а затем проверить подключение на реальных системах.
### Первое подключение и использование

1. Нажмите значок **Amnezia Browser VPN** на панели расширений.
2. В разделе **Добавить конфиг** выберите свой `.conf`-файл или вставьте текст конфига. Укажите название профиля и нажмите **Сохранить конфиг**.
3. Выберите профиль в списке и нажмите **Подключить**.
4. Откройте сайт в Chromium. AmneziaWG выполняет рукопожатие при первом сетевом запросе; запуск локального прокси сам по себе ещё не доказывает доступность VPN-сервера.
5. Чтобы вернуть прежние настройки прокси и WebRTC, нажмите **Отключить VPN**.

Проверьте внешний IP в браузере до и после подключения. Другие приложения Windows должны продолжать использовать обычное системное соединение.

### Сборка из исходников и обновление

После изменения кода повторно соберите и зарегистрируйте локальный компонент:

```powershell
powershell -ExecutionPolicy Bypass -File .\native\build.ps1
powershell -ExecutionPolicy Bypass -File .\native\install.ps1
```

Затем откройте `chrome://extensions` или `edge://extensions` и нажмите кнопку перезагрузки расширения. Профили остаются в локальном хранилище браузера при перезагрузке расширения; удаление самого расширения может удалить их, поэтому храните исходные конфиги отдельно.

### Удаление

1. Если VPN включён, нажмите **Отключить VPN** в расширении.
2. Откройте страницу расширений и удалите расширение.
3. Из корневой папки проекта удалите регистрацию Native Messaging:

   ```powershell
   powershell -ExecutionPolicy Bypass -File .\native\install.ps1 -Uninstall
   ```

4. При необходимости удалите `%LOCALAPPDATA%\AmneziaBrowserVPN` после проверки, что VPN отключён. В этой папке находятся временная копия активного конфига, PID-файл и журнал.

### Устранение неполадок

- **«Native host has exited» / компонент не найден:** проверьте, что `native\AmneziaBrowserVpnHost.exe` собран и `native\install.ps1` выполнен из папки, где проект останется установлен. После этого перезапустите браузер.
- **Ошибка Go при сборке:** установите Go 1.25+ с [официальной страницы загрузки](https://go.dev/dl/) и повторите `native\build.ps1`.
- **Порт 1080 занят:** закройте программу, которая слушает `127.0.0.1:1080`, и попробуйте подключиться снова.
- **Сайт не открывается:** проверьте активность и доступность сервера, соответствие `PrivateKey`/`PublicKey`, `Endpoint` и параметров обфускации на стороне сервера. Журнал локального компонента: `%LOCALAPPDATA%\AmneziaBrowserVPN\proxy.log`.
- **Расширение не управляет прокси или WebRTC:** проверьте политики браузера и другие расширения, которые могут контролировать те же настройки.

### Ограничения и безопасность

- Это прототип: локальный мост передаёт TCP CONNECT через SOCKS5. UDP-приложения и QUIC не туннелируются. Настройка Chromium блокирует WebRTC UDP без прокси; корпоративная политика браузера может запретить изменить эту настройку.
- Успешный запуск означает, что локальный прокси слушает порт. Работу VPN-сервера нужно подтвердить реальным запросом из браузера.
- `DNS` в конфиге должен содержать IP-адреса DNS-серверов. Конфиги с DNS-именами не поддерживаются.
- SOCKS5 без аутентификации слушает только `127.0.0.1:1080`. Расширение автоматически настраивает прокси браузера; другие программы не перенаправляются, но локальное приложение при желании может обратиться к этому порту.
- Сохранённые профили в `chrome.storage.local` и временный файл `active.conf` содержат приватный ключ в открытом виде для текущей Windows-учётной записи. Не публикуйте конфиги и удаляйте рабочую папку данных после отключения, если больше не используете приложение.
- В репозитории включён `.gitignore` для `.conf`, `.vpn`, собранного `.exe`, локальных SDK и файлов регистрации. Перед коммитом всегда проверяйте `git status` и список staged-файлов.

### Устройство проекта

- `extension/` — Manifest V3, интерфейс, локальные профили и управление прокси Chromium.
- `native/main.go` — Native Messaging host, конфигурация AmneziaWG через официальную Go-библиотеку и SOCKS5 TCP CONNECT-мост.
- `native/build.ps1` — сборка Windows x64 исполняемого файла.
- `native/install.ps1` — регистрация/удаление Native Messaging host для Chrome и Edge текущего пользователя.
- `native/THIRD_PARTY.md` — сведения об основной внешней зависимости.

## English

### Features

- Import an AmneziaWG/WireGuard profile from a `.conf` file or pasted text.
- Store multiple profiles locally, select one, or delete it.
- Connect and disconnect from the extension popup.
- Route Chromium web traffic through a local SOCKS5 proxy backed by an AmneziaWG userspace tunnel.
- Do not change Windows system routes or automatically redirect other applications.
- Disable non-proxied WebRTC UDP while connected because SOCKS5 cannot carry it.

### Requirements

- Windows 10/11 x64.
- Chromium, Google Chrome, or Microsoft Edge.
- Go 1.25 or later to build the native component.
- An AmneziaWG profile in WireGuard INI format (`[Interface]`, `[Peer]`).

The component is built and registered for the current Windows user and does not require administrator rights. Build and register it separately for each Windows account that needs to use it.

### Installation

1. Download the repository ZIP from GitHub and extract it to a permanent local folder. Do not move or delete that folder after registering Native Messaging: the Windows registry entry contains the absolute path to the host executable.
2. Open PowerShell in the project root and run:

   ```powershell
   powershell -ExecutionPolicy Bypass -File .\native\build.ps1
   powershell -ExecutionPolicy Bypass -File .\native\install.ps1
   ```

   The build downloads `github.com/amnezia-vpn/amneziawg-go/v3` and creates `native\AmneziaBrowserVpnHost.exe`. The installer registers Native Messaging under HKCU for Chrome and Edge and allows the extension ID defined by `extension\manifest.json`.

3. Open `chrome://extensions` in Chrome or `edge://extensions` in Edge.
4. Enable **Developer mode**, choose **Load unpacked**, and select the repository's `extension` folder.
5. Approve the extension permissions for proxy settings, privacy settings, and Native Messaging.

If the browser was open during the build or registration, reload the extension on the extensions page or restart the browser.

### macOS and Linux

**Important:** the current version does not establish a VPN tunnel on macOS or Linux. The native host uses Windows APIs, native/build.ps1 produces an EXE, and native/install.ps1 registers Native Messaging in the Windows registry.

Separate repository for the macOS edition: [DanOrLove/AmneziaBrowserExtensionMacOS](https://github.com/DanOrLove/AmneziaBrowserExtensionMacOS).

To open the extension interface only:

1. Download and extract the repository.
2. Open chrome://extensions in Chrome or Chromium.
3. Enable Developer mode, click **Load unpacked**, and select the extension folder.
4. You can inspect the interface, but importing a profile will not connect the VPN. Clicking **Connect** cannot start the native host, and browser traffic will not go through the tunnel.

Do not run native/build.ps1 or native/install.ps1 on these systems. Full support requires a native host port for macOS/Linux, build and Native Messaging registration scripts for each OS, and connection testing on those systems.
### First connection and usage

1. Click the **Amnezia Browser VPN** toolbar icon.
2. Under **Add config**, choose a `.conf` file or paste the profile text. Enter a profile name and click **Save config**.
3. Select a profile and click **Connect**.
4. Open a website in Chromium. AmneziaWG performs its handshake on the first network request; starting the local proxy alone does not prove that the VPN server is reachable.
5. Click **Disconnect VPN** to restore the previous browser proxy and WebRTC settings.

Check the public IP in the browser before and after connecting. Other Windows applications should continue using the normal system connection.

### Rebuild and update

After changing the code, rebuild and register the native component again:

```powershell
powershell -ExecutionPolicy Bypass -File .\native\build.ps1
powershell -ExecutionPolicy Bypass -File .\native\install.ps1
```

Then reload the extension on `chrome://extensions` or `edge://extensions`. Profiles remain in the browser's local extension storage when the extension is reloaded. Removing the extension may delete them, so keep the original configuration files separately.

### Uninstall

1. If the VPN is connected, click **Disconnect VPN** in the extension.
2. Remove the extension from the browser's extensions page.
3. From the project root, remove the Native Messaging registration:

   ```powershell
   powershell -ExecutionPolicy Bypass -File .\native\install.ps1 -Uninstall
   ```

4. If desired, delete `%LOCALAPPDATA%\AmneziaBrowserVPN` after confirming the tunnel is stopped. It contains a temporary active config copy, a PID file, and a log.

### Troubleshooting

- **“Native host has exited” / host not found:** make sure `native\AmneziaBrowserVpnHost.exe` was built and `native\install.ps1` was run from the folder where the project will remain installed. Restart the browser afterward.
- **Go build error:** install Go 1.25+ from the [official downloads page](https://go.dev/dl/) and rerun `native\build.ps1`.
- **Port 1080 is busy:** close the application listening on `127.0.0.1:1080` and retry.
- **A website does not load:** check server availability, `PrivateKey`/`PublicKey`, `Endpoint`, and the obfuscation parameters configured on the server. The native component log is `%LOCALAPPDATA%\AmneziaBrowserVPN\proxy.log`.
- **The extension cannot control proxy or WebRTC settings:** check browser policies and other extensions that may control the same settings.

### Limitations and security

- This is an early prototype. The local bridge carries TCP CONNECT over SOCKS5. UDP applications and QUIC are not tunneled. Chromium is configured to block WebRTC UDP that cannot use the proxy; enterprise browser policy may prevent changing this setting.
- A successful start only confirms that the local proxy is listening. Confirm VPN connectivity with an actual browser request.
- The config's `DNS` setting must contain DNS server IP addresses. DNS hostnames are unsupported.
- An unauthenticated SOCKS5 proxy listens only on `127.0.0.1:1080`. The extension configures the browser automatically; other applications are not redirected, although a local application can connect to the proxy port if it chooses.
- Profiles in `chrome.storage.local` and the temporary `active.conf` contain the private key in plaintext accessible to the current Windows account. Do not publish your profiles. Remove the app data folder after disconnecting if you no longer need it.
- `.gitignore` excludes `.conf`, `.vpn`, the generated `.exe`, local SDKs, and the generated Native Messaging manifest. Always review `git status` and the staged file list before committing.

### Project layout

- `extension/` — Manifest V3 UI, local profile storage, and Chromium proxy control.
- `native/main.go` — Native Messaging host, AmneziaWG setup using the official Go library, and SOCKS5 TCP CONNECT bridge.
- `native/build.ps1` — builds the Windows x64 executable.
- `native/install.ps1` — registers/unregisters the Native Messaging host for the current Chrome/Edge user.
- `native/THIRD_PARTY.md` — information about the main third-party dependency.
