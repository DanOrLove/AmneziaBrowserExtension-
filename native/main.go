package main

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/amnezia-vpn/amneziawg-go/v3/conn"
	"github.com/amnezia-vpn/amneziawg-go/v3/device"
	"github.com/amnezia-vpn/amneziawg-go/v3/tun/netstack"
)

const proxyAddress = "127.0.0.1:1080"
const maxMessage = 2 * 1024 * 1024

type request struct {
	Command string `json:"command"`
	Config  string `json:"config"`
}

type response struct {
	OK      bool   `json:"ok"`
	Running bool   `json:"running,omitempty"`
	Message string `json:"message,omitempty"`
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "proxy" {
		if len(os.Args) != 3 {
			os.Exit(2)
		}
		if err := runProxy(os.Args[2]); err != nil {
			log.Print(err)
			os.Exit(1)
		}
		return
	}
	runNativeHost()
}

func runNativeHost() {
	requestBytes, err := readNativeMessage(os.Stdin)
	if err != nil {
		writeNativeResponse(response{Message: "Не удалось прочитать команду расширения."})
		return
	}
	var req request
	if err := json.Unmarshal(requestBytes, &req); err != nil {
		writeNativeResponse(response{Message: "Команда расширения повреждена."})
		return
	}

	var result response
	switch req.Command {
	case "start":
		result = startProxy(req.Config)
	case "stop":
		result = stopProxy()
	case "status":
		result = response{OK: true, Running: proxyRunning()}
	default:
		result = response{Message: "Неизвестная команда."}
	}
	writeNativeResponse(result)
}

func readNativeMessage(r io.Reader) ([]byte, error) {
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return nil, err
	}
	size := binary.LittleEndian.Uint32(header[:])
	if size == 0 || size > maxMessage {
		return nil, errors.New("invalid message size")
	}
	data := make([]byte, size)
	_, err := io.ReadFull(r, data)
	return data, err
}

func writeNativeResponse(value response) {
	data, err := json.Marshal(value)
	if err != nil || len(data) > maxMessage {
		return
	}
	var header [4]byte
	binary.LittleEndian.PutUint32(header[:], uint32(len(data)))
	_, _ = os.Stdout.Write(header[:])
	_, _ = os.Stdout.Write(data)
}

func dataDirectory() (string, error) {
	root := os.Getenv("LOCALAPPDATA")
	if root == "" {
		return "", errors.New("LOCALAPPDATA is unavailable")
	}
	dir := filepath.Join(root, "AmneziaBrowserVPN")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	return dir, nil
}

func startProxy(config string) response {
	if _, err := parseConfig(config); err != nil {
		return response{Message: "Конфиг не подходит для AmneziaWG: " + err.Error()}
	}
	if proxyRunning() {
		return response{Message: "Локальный прокси уже запущен."}
	}
	if proxyListening() {
		return response{Message: "Порт 1080 занят другой программой. Закройте её и повторите подключение."}
	}
	dir, err := dataDirectory()
	if err != nil {
		return response{Message: "Не удалось подготовить локальное хранилище: " + err.Error()}
	}
	executable, err := os.Executable()
	if err != nil {
		return response{Message: "Не удалось найти компонент VPN."}
	}
	configPath := filepath.Join(dir, "active.conf")
	if err := os.WriteFile(configPath, []byte(config), 0600); err != nil {
		return response{Message: "Не удалось сохранить рабочий конфиг: " + err.Error()}
	}
	logPath := filepath.Join(dir, "proxy.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return response{Message: "Не удалось открыть журнал локального компонента."}
	}
	cmd := exec.Command(executable, "proxy", configPath)
	cmd.Stdout, cmd.Stderr = logFile, logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x00000008}
	if err := cmd.Start(); err != nil {
		logFile.Close()
		return response{Message: "Не удалось запустить AmneziaWG: " + err.Error()}
	}
	_ = logFile.Close()
	_ = os.WriteFile(filepath.Join(dir, "proxy.pid"), []byte(strconv.Itoa(cmd.Process.Pid)), 0600)
	for attempt := 0; attempt < 100; attempt++ {
		if proxyListening() && processAlive(cmd.Process.Pid) {
			return response{OK: true, Running: true, Message: "AmneziaWG-прокси запущен. Трафик браузера пойдёт через туннель."}
		}
		if !processAlive(cmd.Process.Pid) {
			_ = os.Remove(filepath.Join(dir, "proxy.pid"))
			_ = os.Remove(configPath)
			return response{Message: "Компонент остановился при запуске. Подробности записаны в локальный журнал."}
		}
		time.Sleep(100 * time.Millisecond)
	}
	_ = cmd.Process.Kill()
	_ = os.Remove(filepath.Join(dir, "proxy.pid"))
	_ = os.Remove(configPath)
	return response{Message: "Прокси не открыл локальный порт за 10 секунд. Проверьте конфиг и журнал."}
}

func stopProxy() response {
	dir, err := dataDirectory()
	if err != nil {
		return response{Message: err.Error()}
	}
	pidPath := filepath.Join(dir, "proxy.pid")
	pidBytes, err := os.ReadFile(pidPath)
	if err != nil {
		if !proxyListening() {
			_ = os.Remove(filepath.Join(dir, "active.conf"))
			return response{OK: true, Message: "VPN уже отключён."}
		}
		return response{Message: "Нашёл локальный прокси, но не его процесс. Остановите его вручную."}
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(pidBytes)))
	if err != nil || pid < 1 {
		return response{Message: "Файл состояния локального компонента повреждён."}
	}
	if !processAlive(pid) {
		_ = os.Remove(pidPath)
		_ = os.Remove(filepath.Join(dir, "active.conf"))
		if proxyListening() {
			return response{Message: "Порт 1080 занят другой программой; её процесс не тронут."}
		}
		return response{OK: true, Message: "VPN уже был остановлен."}
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return response{Message: "Не удалось открыть процесс локального туннеля."}
	}
	if err := process.Kill(); err != nil {
		return response{Message: "Не удалось остановить AmneziaWG: " + err.Error()}
	}
	for attempt := 0; attempt < 30 && proxyListening(); attempt++ {
		time.Sleep(100 * time.Millisecond)
	}
	if proxyListening() {
		return response{Message: "Локальный прокси всё ещё отвечает; браузерный прокси восстановлен, но процесс требует остановки."}
	}
	_ = os.Remove(pidPath)
	_ = os.Remove(filepath.Join(dir, "active.conf"))
	return response{OK: true, Message: "VPN отключён для Chromium."}
}

func processAlive(pid int) bool {
	// On Windows, FindProcess doesn't report whether the process has exited.
	handle, err := syscall.OpenProcess(0x1000, false, uint32(pid)) // PROCESS_QUERY_LIMITED_INFORMATION
	if err != nil {
		return false
	}
	defer syscall.CloseHandle(handle)
	var exitCode uint32
	if syscall.GetExitCodeProcess(handle, &exitCode) != nil {
		return false
	}
	if exitCode != 259 {
		return false
	} // STILL_ACTIVE
	imagePath, err := processImagePath(handle)
	if err != nil {
		return false
	}
	executable, err := os.Executable()
	return err == nil && strings.EqualFold(filepath.Clean(imagePath), filepath.Clean(executable))
}

func processImagePath(handle syscall.Handle) (string, error) {
	proc := syscall.NewLazyDLL("kernel32.dll").NewProc("QueryFullProcessImageNameW")
	var buffer [2048]uint16
	size := uint32(len(buffer))
	result, _, callErr := proc.Call(uintptr(handle), 0, uintptr(unsafe.Pointer(&buffer[0])), uintptr(unsafe.Pointer(&size)))
	if result == 0 {
		return "", callErr
	}
	return syscall.UTF16ToString(buffer[:size]), nil
}

func proxyListening() bool {
	conn, err := net.DialTimeout("tcp", proxyAddress, 150*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func proxyRunning() bool {
	dir, err := dataDirectory()
	if err != nil {
		return false
	}
	pidBytes, err := os.ReadFile(filepath.Join(dir, "proxy.pid"))
	if err != nil {
		return false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(pidBytes)))
	return err == nil && pid > 0 && processAlive(pid) && proxyListening()
}

type wgConfig struct {
	addresses []netip.Addr
	dns       []netip.Addr
	mtu       int
	ipc       strings.Builder
}

func parseConfig(source string) (*wgConfig, error) {
	cfg := &wgConfig{mtu: 1420}
	section := ""
	seenPrivateKey, seenPublicKey, seenEndpoint, seenAllowedIPs := false, false, false, false
	scanner := bufio.NewScanner(strings.NewReader(source))
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	for lineNo := 1; scanner.Scan(); lineNo++ {
		line := strings.TrimSpace(scanner.Text())
		if cut := strings.IndexAny(line, "#;"); cut >= 0 {
			line = strings.TrimSpace(line[:cut])
		}
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.ToLower(strings.TrimSpace(line[1 : len(line)-1]))
			if section != "interface" && section != "peer" {
				return nil, fmt.Errorf("строка %d: неизвестная секция", lineNo)
			}
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || section == "" {
			return nil, fmt.Errorf("строка %d: ожидалось имя = значение", lineNo)
		}
		key, value = strings.ToLower(strings.TrimSpace(key)), strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if section == "interface" {
			switch key {
			case "privatekey":
				encoded, err := decodeKey(value)
				if err != nil {
					return nil, fmt.Errorf("строка %d: неверный PrivateKey", lineNo)
				}
				cfg.ipc.WriteString("private_key=" + hex.EncodeToString(encoded) + "\n")
				seenPrivateKey = true
			case "address":
				for _, part := range strings.Split(value, ",") {
					p, err := netip.ParsePrefix(strings.TrimSpace(part))
					if err != nil {
						a, addrErr := netip.ParseAddr(strings.TrimSpace(part))
						if addrErr != nil {
							return nil, fmt.Errorf("строка %d: неверный Address", lineNo)
						}
						cfg.addresses = append(cfg.addresses, a)
					} else {
						cfg.addresses = append(cfg.addresses, p.Addr())
					}
				}
			case "dns":
				for _, part := range strings.Split(value, ",") {
					a, err := netip.ParseAddr(strings.TrimSpace(part))
					if err != nil {
						return nil, fmt.Errorf("строка %d: DNS должен содержать IP-адреса", lineNo)
					}
					cfg.dns = append(cfg.dns, a)
				}
			case "mtu":
				parsed, err := strconv.Atoi(value)
				if err != nil || parsed < 576 || parsed > 65535 {
					return nil, fmt.Errorf("строка %d: неверный MTU", lineNo)
				}
				cfg.mtu = parsed
			case "headerprotectionkey":
				encoded, err := decodeKey(value)
				if err != nil {
					return nil, fmt.Errorf("строка %d: неверный HeaderProtectionKey", lineNo)
				}
				cfg.ipc.WriteString("header_protection_key=" + hex.EncodeToString(encoded) + "\n")
			case "randomtrailers", "disablecookies":
				flag := strings.ToLower(value)
				if flag == "on" {
					flag = "true"
				}
				if flag == "off" {
					flag = "false"
				}
				if flag != "true" && flag != "false" {
					return nil, fmt.Errorf("строка %d: ожидалось on/off", lineNo)
				}
				ipcKey := strings.ReplaceAll(key, "trailers", "_trailers")
				ipcKey = strings.ReplaceAll(ipcKey, "cookies", "_cookies")
				cfg.ipc.WriteString(ipcKey + "=" + flag + "\n")
			default:
				cfg.ipc.WriteString(key + "=" + value + "\n")
			}
		} else {
			switch key {
			case "publickey":
				encoded, err := decodeKey(value)
				if err != nil {
					return nil, fmt.Errorf("строка %d: неверный PublicKey", lineNo)
				}
				cfg.ipc.WriteString("public_key=" + hex.EncodeToString(encoded) + "\n")
				seenPublicKey = true
			case "presharedkey":
				encoded, err := decodeKey(value)
				if err != nil {
					return nil, fmt.Errorf("строка %d: неверный PresharedKey", lineNo)
				}
				cfg.ipc.WriteString("preshared_key=" + hex.EncodeToString(encoded) + "\n")
			case "endpoint":
				if _, _, err := net.SplitHostPort(value); err != nil {
					return nil, fmt.Errorf("строка %d: неверный Endpoint", lineNo)
				}
				cfg.ipc.WriteString("endpoint=" + value + "\n")
				seenEndpoint = true
			case "allowedips":
				for _, part := range strings.Split(value, ",") {
					p, err := netip.ParsePrefix(strings.TrimSpace(part))
					if err != nil {
						return nil, fmt.Errorf("строка %d: неверный AllowedIPs", lineNo)
					}
					cfg.ipc.WriteString("allowed_ip=" + p.String() + "\n")
				}
				seenAllowedIPs = true
			case "persistentkeepalive":
				cfg.ipc.WriteString("persistent_keepalive_interval=" + value + "\n")
			default:
				cfg.ipc.WriteString(key + "=" + value + "\n")
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if !seenPrivateKey || !seenPublicKey || !seenEndpoint || !seenAllowedIPs || len(cfg.addresses) == 0 {
		return nil, errors.New("обязательные поля PrivateKey, Address, PublicKey, Endpoint и AllowedIPs должны быть заполнены")
	}
	return cfg, nil
}

func decodeKey(value string) ([]byte, error) {
	if raw, err := hex.DecodeString(value); err == nil && len(raw) == 32 {
		return raw, nil
	}
	raw, err := base64.StdEncoding.DecodeString(value)
	if err != nil || len(raw) != 32 {
		return nil, errors.New("ожидался 32-байтовый ключ base64 или hex")
	}
	return raw, nil
}

func runProxy(configPath string) error {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return err
	}
	cfg, err := parseConfig(string(data))
	if err != nil {
		return err
	}
	tun, tunnel, err := netstack.CreateNetTUN(cfg.addresses, cfg.dns, cfg.mtu)
	if err != nil {
		return err
	}
	dev := device.NewDevice(tun, conn.NewDefaultBind(), device.NewLogger(device.LogLevelError, ""))
	if err := dev.IpcSet(cfg.ipc.String()); err != nil {
		dev.Close()
		return err
	}
	if err := dev.Up(); err != nil {
		dev.Close()
		return err
	}
	defer dev.Close()
	ln, err := net.Listen("tcp", proxyAddress)
	if err != nil {
		return err
	}
	defer ln.Close()
	for {
		client, err := ln.Accept()
		if err != nil {
			return err
		}
		go serveSocks5(client, tunnel.DialContext)
	}
}

func serveSocks5(client net.Conn, dial func(context.Context, string, string) (net.Conn, error)) {
	defer client.Close()
	_ = client.SetDeadline(time.Now().Add(15 * time.Second))
	var header [2]byte
	if _, err := io.ReadFull(client, header[:]); err != nil || header[0] != 5 {
		return
	}
	methods := make([]byte, int(header[1]))
	if _, err := io.ReadFull(client, methods); err != nil {
		return
	}
	noAuth := false
	for _, method := range methods {
		if method == 0 {
			noAuth = true
			break
		}
	}
	if !noAuth {
		_, _ = client.Write([]byte{5, 255})
		return
	}
	if _, err := client.Write([]byte{5, 0}); err != nil {
		return
	} // no-auth; loopback only
	var req [4]byte
	if _, err := io.ReadFull(client, req[:]); err != nil || req[0] != 5 || req[1] != 1 || req[2] != 0 {
		return
	}
	host := ""
	switch req[3] {
	case 1:
		addr := make([]byte, 4)
		if _, err := io.ReadFull(client, addr); err != nil {
			return
		}
		host = net.IP(addr).String()
	case 4:
		addr := make([]byte, 16)
		if _, err := io.ReadFull(client, addr); err != nil {
			return
		}
		host = net.IP(addr).String()
	case 3:
		size := []byte{0}
		if _, err := io.ReadFull(client, size); err != nil {
			return
		}
		name := make([]byte, int(size[0]))
		if _, err := io.ReadFull(client, name); err != nil {
			return
		}
		host = string(name)
	default:
		return
	}
	var portBytes [2]byte
	if _, err := io.ReadFull(client, portBytes[:]); err != nil {
		return
	}
	port := binary.BigEndian.Uint16(portBytes[:])
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	upstream, err := dial(ctx, "tcp", net.JoinHostPort(host, strconv.Itoa(int(port))))
	if err != nil {
		_, _ = client.Write([]byte{5, 4, 0, 1, 0, 0, 0, 0, 0, 0})
		return
	}
	defer upstream.Close()
	_ = client.SetDeadline(time.Time{})
	if _, err := client.Write([]byte{5, 0, 0, 1, 0, 0, 0, 0, 0, 0}); err != nil {
		return
	}
	done := make(chan struct{}, 2)
	go func() {
		_, _ = io.Copy(upstream, client)
		if half, ok := upstream.(interface{ CloseWrite() error }); ok {
			_ = half.CloseWrite()
		}
		done <- struct{}{}
	}()
	go func() {
		_, _ = io.Copy(client, upstream)
		if half, ok := client.(interface{ CloseWrite() error }); ok {
			_ = half.CloseWrite()
		}
		done <- struct{}{}
	}()
	<-done
	<-done
}
