package main

/*
#cgo CFLAGS: -Wall
#cgo LDFLAGS: -luser32 -lgdi32
#cgo CFLAGS: -I./winapi
#include "lib/winapi/system_api.c"

// Forward declarations required by system_api.c
extern void OnMouseMove(int x, int y);
extern void OnEnabledHotKey();

#define MOD_ALT     0x0001
#define MOD_CONTROL 0x0002
#define MOD_SHIFT   0x0004
#define VK_W        0x57
*/
import "C"

import (
	"flag"
	"fmt"
	"github.com/getlantern/systray"
	"github.com/karalabe/hid"
	"log"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

/**
 * go get github.com/getlantern/systray 다운로드 필요
 *
 * 1. 지정 devIdx(1,2)만 채널 0으로: 키보드+마우스 동시 (인덱스 후보 기본 0x0E)
 * offline_logi_flow_sim_test.exe --devices=1,2 --right-channels=0,0 --log-level=2 --test
 *
 * 2. Easy-Switch 누르면 마우스도 동기 전환 (입력 리포트 감지):
 * offline_logi_flow_sim_test.exe --devices=1,2 --follow-kb-switch --log-level=2
 *
 * 3. 0x0E가 안 먹는 경우 후보 늘리기
 * offline_logi_flow_sim_test.exe --devices=2,3 --right-channels=0,0 --mouse-indexes=0x0E,0x1B,0x15 --log-level=2 --test
 *
 * 빌드방법(트레이로 들어감)
 * go build -ldflags="-H=windowsgui" -buildvcs=false -o offline_logi_flow_sim_tray.exe
 * offline_logi_flow_sim_test.exe --devices=1,2 --follow-kb-switch --tray --log-level=1
 **/

//
// ===================== 전역 기본값(플래그로 오버라이드 가능) =====================
//

// Bolt 수신기 VID/PID
var (
	vendorID  uint16 = 0x046D
	productID uint16 = 0xC548
)

// 벤더 페이지 / 사용 용도
var (
	usagePageFF00 uint16 = 0xFF00
	usageOut      uint16 = 0x0001 // 키보드: 7B Output
	usageFeat     uint16 = 0x0002 // 마우스: 20B Long (Feature/Long write)
)

// 리포트 포맷
var (
	kbReportID        byte = 0x10 // 7B
	mouseReportID     byte = 0x11 // 20B long
	mouseLongTotalLen int  = 20
)

// 마우스용 HID++ feature index 후보(룩업 없이 사용)
// 환경마다 다를 수 있으므로 여러 개 지정 가능 (기본 0x0E)
var (
	mouseIndexCandidates = []byte{0x0E}
)

// 로깅/동작 옵션
var (
	logLevel        = 1
	logEnabled      = false
	testMode        = false
	openFirstOnly   = false
	kbOnly          = false
	mouseOnly       = false
	followKbSwitch  = false
	trayMode        = false
	logFilePath     = "flow.log"
	iconPath        = "icon.ico"
	writePauseMs    = 10
	readTimeoutMs   = 3000
)

// devIdx / 채널 파라미터
type byteSlice []byte

func (s *byteSlice) String() string { return fmt.Sprint(*s) }
func (s *byteSlice) Set(value string) error {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	for _, part := range strings.Split(value, ",") {
		v := strings.TrimSpace(part)
		base := 10
		if strings.HasPrefix(v, "0x") || strings.HasPrefix(v, "0X") {
			base = 0
		}
		i, err := strconv.ParseUint(v, base, 8)
		if err != nil {
			return fmt.Errorf("invalid byte '%s': %w", v, err)
		}
		*s = append(*s, byte(i))
	}
	return nil
}

var (
	deviceIds     byteSlice // 내부 devIdx 목록
	leftChannels  byteSlice
	rightChannels byteSlice
)

//
// ===================== 로깅 헬퍼 =====================
//

func Log(level int, a ...any) {
    if !logEnabled {
        return
    }
	if level <= logLevel {
		log.Println(a...)
	}
}
func Logf(level int, format string, a ...any) {
    if !logEnabled {
        return
    }
	if level <= logLevel {
		log.Printf(format, a...)
	}
}

//
// ===================== Windows 입력 (기존 flow 보존용) =====================
//

type MOUSEINPUT struct {
	Dx          int32
	Dy          int32
	MouseData   uint32
	DwFlags     uint32
	Time        uint32
	DwExtraInfo uintptr
}
type INPUT struct {
	Type uint32
	MI   MOUSEINPUT
}

const (
	INPUT_MOUSE          = 0
	MOUSEEVENTF_MOVE     = 0x0001
	MOUSEEVENTF_ABSOLUTE = 0x8000
)

var (
	user32            = syscall.NewLazyDLL("user32.dll")
	procSendInput     = user32.NewProc("SendInput")
	procSystemMetrics = user32.NewProc("GetSystemMetrics")

	leftBorder          = 0
	rightBorder         = 0
	enabled             = true
	offline             = true
	lastSwitched        = time.Now()
	forwardThreshold, _ = time.ParseDuration("1500ms")
	backwardThreshold, _ = time.ParseDuration("500ms")
)

func MoveMouse(x int, y int) {
	w, _, _ := procSystemMetrics.Call(C.SM_CXSCREEN)
	h, _, _ := procSystemMetrics.Call(C.SM_CYSCREEN)
	rightBorder = int(w)
	var input INPUT
	input.Type = INPUT_MOUSE
	input.MI.Dx = int32(x * 65535 / int(w))
	input.MI.Dy = int32(y * 65535 / int(h))
	input.MI.DwFlags = MOUSEEVENTF_MOVE | MOUSEEVENTF_ABSOLUTE
	procSendInput.Call(1, uintptr(unsafe.Pointer(&input)), unsafe.Sizeof(input))
}

//export OnMouseMove
func OnMouseMove(x C.int, y C.int) {
	// 훅 이벤트를 쓰지 않더라도 스텁이 필요함(링크용). 원하면 기존 로직을 붙이세요.
}

//export OnEnabledHotKey
func OnEnabledHotKey() {
	// 필요 시 핫키 토글 로직 연동
}

//
// ===================== HID 열기/닫기 =====================
//

var (
	devsOut  []hid.Device // FF00/0001 (키보드 7B)
	devsFeat []hid.Device // FF00/0002 (마우스 20B, IN/OUT 모두 사용)
)

func openCollections() {
	all, err := hid.Enumerate(vendorID, productID)
	if err != nil {
		log.Fatalf("hid.Enumerate failed: %v", err)
	}
	for i, di := range all {
		Logf(2, "#%d Path=%s VID=%04X PID=%04X UsagePage=0x%X Usage=0x%X If=%d",
			i, di.Path, di.VendorID, di.ProductID, di.UsagePage, di.Usage, di.Interface)
	}
	for _, di := range all {
		if di.UsagePage == usagePageFF00 && di.Usage == usageOut {
			if d, e := di.Open(); e == nil {
				Logf(1, "Opened OUT: Path=%s If=%d", di.Path, di.Interface)
				devsOut = append(devsOut, d)
				if openFirstOnly {
					break
				}
			} else {
				Logf(1, "Open OUT failed: %v", e)
			}
		}
	}
	for _, di := range all {
		if di.UsagePage == usagePageFF00 && di.Usage == usageFeat {
			if d, e := di.Open(); e == nil {
				Logf(1, "Opened FEAT: Path=%s If=%d", di.Path, di.Interface)
				devsFeat = append(devsFeat, d)
				if openFirstOnly {
					break
				}
			} else {
				Logf(1, "Open FEAT failed: %v", e)
			}
		}
	}
	if len(devsOut) == 0 && len(devsFeat) == 0 {
		//log.Fatal("No HID collections opened (FF00/0001 and 0002 both empty)")
		Log(1, "No HID collections opened (FF00/0001 and 0002 both empty)")
	}
}

func cleanup() {
	for _, d := range devsOut {
		_ = d.Close()
	}
	for _, d := range devsFeat {
		_ = d.Close()
	}
}

//
// ===================== 패킷 빌더 & 전송 =====================
//

func buildKB(devIdx, ch byte) []byte {
	p := make([]byte, 7)
	p[0] = kbReportID
	p[1] = devIdx
	p[2] = 0x0A
	p[3] = 0x1E
	p[4] = ch
	p[5] = 0x00
	p[6] = 0x00
	return p
}

func buildMouseWithIndex(devIdx, idx, ch byte) []byte {
	p := make([]byte, mouseLongTotalLen)
	p[0] = mouseReportID
	p[1] = devIdx
	p[2] = idx     // ← 룩업 없이 후보 인덱스 사용
	p[3] = 0x1C
	p[4] = ch
	// [5..] = 0
	return p
}

func sendKB(pkt []byte) {
	for i, d := range devsOut {
		if _, err := d.Write(pkt); err != nil {
			Logf(1, "KB write failed dev#%d [% x]: %v", i, pkt, err)
		} else {
			Logf(2, "KB write OK dev#%d [% x]", i, pkt)
		}
	}
}

func sendMouseTryCandidates(devIdx, ch byte) bool {
	sentAny := false
	for _, idx := range mouseIndexCandidates {
		pkt := buildMouseWithIndex(devIdx, idx, ch)
		for i, d := range devsFeat {
			if _, err := d.Write(pkt); err != nil {
				Logf(1, "MOUSE write failed dev#%d idx=0x%X [% x]: %v", i, idx, pkt, err)
			} else {
				Logf(2, "MOUSE write OK dev#%d idx=0x%X [% x]", i, idx, pkt)
				sentAny = true
			}
		}
	}
	return sentAny
}

//
// ===================== 키보드 스위치 미러링(입력 리포트 수신) =====================
//

// FEAT(0x0002)에서 올라오는 0x11 헤더 입력 리포트 후보 판별
// 형식(경험적): [0]=0x11, [1]=devIdx, [2]=featureIndex, [3]=0x1C, [4]=channel(0..3)
func looksLikeKbChannelEvent(pkt []byte) (devIdx, ch byte, ok bool) {
	if len(pkt) < 8 {
		return 0, 0, false
	}
	if pkt[0] != 0x11 {
		return 0, 0, false
	}
	idx := pkt[2]
	okIdx := false
	for _, c := range mouseIndexCandidates {
		if idx == c {
			okIdx = true
			break
		}
	}
	if !okIdx {
		return 0, 0, false
	}
	if pkt[3] != 0x1C {
		return 0, 0, false
	}
	ch = pkt[4]
	if ch > 3 {
		return 0, 0, false
	}
	return pkt[1], ch, true
}

func startVendorInputMirror() {
	if len(devsFeat) == 0 {
		Log(1, "follow-kb-switch: no FEAT devices opened; cannot listen")
		return
	}
	for i, d := range devsFeat {
		go func(idx int, dev hid.Device) {
			buf := make([]byte, 64)
			for {
				n, err := dev.ReadTimeout(buf, int(readTimeoutMs))
				if err != nil || n <= 0 {
					// Timeout/일시적 오류는 계속 시도
					continue
				}
				pkt := make([]byte, n)
				copy(pkt, buf[:n])
				Logf(2, "FEAT IN dev#%d: [% x]", idx, pkt)

				if devIdx, ch, ok := looksLikeKbChannelEvent(pkt); ok {
					// 지정 devIdx만 동기화 (요구사항: --devices에 준 devIdx만)
					if containsByte(deviceIds, devIdx) {
						if !kbOnly {
							Logf(1, "KB switch detected dev=%d ch=%d -> mirror mouse", devIdx, ch)
							sendMouseTryCandidates(devIdx, ch)
						}
					}
				}
			}
		}(i, d)
	}
}

func containsByte(arr []byte, v byte) bool {
	for _, x := range arr {
		if x == v {
			return true
		}
	}
	return false
}

//
// ===================== 인자 파싱 =====================
//
var trayOnly = false // 리시버 없어도 트레이만 띄우기

func parseArgs() {
    flag.BoolVar(&logEnabled, "log", true, "Enable/disable logging globally (true/false)")

	flag.Var(&deviceIds, "devices", "Comma-separated HID++ devIdx bytes (e.g., 0,1,2). Only these will be targeted.")
	flag.Var(&leftChannels, "left-channels", "Channels for left actions (e.g., 1,2).")
	flag.Var(&rightChannels, "right-channels", "Channels for right actions (e.g., 0,0).")

	flag.IntVar(&logLevel, "log-level", 1, "0=silent,1=info,2=debug")
	flag.BoolVar(&testMode, "test", false, "Send packets immediately and exit")
	flag.BoolVar(&openFirstOnly, "open-first", false, "Open only first matching collection per type")
	flag.BoolVar(&kbOnly, "kb-only", false, "Send only keyboard(7B) packets")
	flag.BoolVar(&mouseOnly, "mouse-only", false, "Send only mouse(20B) packets")
	flag.BoolVar(&followKbSwitch, "follow-kb-switch", false, "Mirror keyboard channel switch to mouse (same devIdx/channel)")
	flag.BoolVar(&trayMode, "tray", false, "Show system tray icon with menu")
	flag.BoolVar(&trayOnly, "tray-only", false, "Show tray even if no HID devices are present")

	flag.StringVar(&logFilePath, "log-file", "flow.log", "Log file path (when running GUI/tray)")
	flag.StringVar(&iconPath, "icon", "icon.ico", "Tray icon .ico path (optional)")
	flag.IntVar(&writePauseMs, "write-pause-ms", 10, "Pause (ms) between writes")
	flag.IntVar(&readTimeoutMs, "read-timeout-ms", 3000, "Read timeout (ms) for vendor IN")

	// 마우스 feature index 후보들 커스터마이즈
	var idxList string
	flag.StringVar(&idxList, "mouse-indexes", "0x0E", "Comma-separated candidate indexes for mouse long reports (e.g., 0x0E,0x1B)")

	flag.Parse()

	// mouseIndexCandidates 파싱
	if strings.TrimSpace(idxList) != "" {
		var lst byteSlice
		_ = lst.Set(idxList)
		if len(lst) > 0 {
			mouseIndexCandidates = lst
		}
	}

	Log(1, "Devices (devIdx):", deviceIds)
	Log(1, "Left Channels:", leftChannels)
	Log(1, "Right Channels:", rightChannels)
	Logf(1, "Mouse index candidates: %v", mouseIndexCandidates)
	Log(1, "Options - test:", testMode, "open-first:", openFirstOnly, "kb-only:", kbOnly, "mouse-only:", mouseOnly, "follow-kb-switch:", followKbSwitch, "tray:", trayMode)

	if len(deviceIds) == 0 {
		log.Fatalln("Device list must be provided via --devices")
	}
	if kbOnly && mouseOnly {
		log.Fatalln("--kb-only and --mouse-only cannot both be true")
	}
}

//
// ===================== 테스트 실행(지정 devIdx만) =====================
//

func runTest() {
	Log(1, "Running test mode...")

	// 오른쪽 (우선)
	min := len(deviceIds)
	if len(rightChannels) < min {
		min = len(rightChannels)
	}
	for i := 0; i < min; i++ {
		dev := deviceIds[i]
		ch := rightChannels[i]

		if !mouseOnly {
			kb := buildKB(dev, ch)
			Logf(1, "KB RIGHT [% x]", kb)
			sendKB(kb)
			time.Sleep(time.Duration(writePauseMs) * time.Millisecond)
		}
		if !kbOnly {
			Logf(1, "MOUSE RIGHT dev=%d ch=%d", dev, ch)
			sendMouseTryCandidates(dev, ch)
			time.Sleep(time.Duration(writePauseMs) * time.Millisecond)
		}
	}

	// 왼쪽 (있으면)
	min = len(deviceIds)
	if len(leftChannels) < min {
		min = len(leftChannels)
	}
	for i := 0; i < min; i++ {
		dev := deviceIds[i]
		ch := leftChannels[i]

		if !mouseOnly {
			kb := buildKB(dev, ch)
			Logf(1, "KB LEFT  [% x]", kb)
			sendKB(kb)
			time.Sleep(time.Duration(writePauseMs) * time.Millisecond)
		}
		if !kbOnly {
			Logf(1, "MOUSE LEFT dev=%d ch=%d", dev, ch)
			sendMouseTryCandidates(dev, ch)
			time.Sleep(time.Duration(writePauseMs) * time.Millisecond)
		}
	}

	Log(1, "Done. Exiting.")
}

//
// ===================== Tray =====================
//

func runTray() {
	systray.Run(onTrayReady, onTrayExit)
}

func onTrayReady() {
	Log(1, "onTrayReady()")
	if b, err := os.ReadFile(iconPath); err == nil && len(b) > 0 {
		systray.SetIcon(b)
	}
	systray.SetTitle("Logi Flow Helper")
	systray.SetTooltip("Keyboard→Mouse sync running")

	mToggle := systray.AddMenuItem("Toggle follow-kb-switch", "Enable/Disable mirroring")
	mQuit := systray.AddMenuItem("Quit", "Exit application")

	go func() {
		for {
			select {
			case <-mToggle.ClickedCh:
				followKbSwitch = !followKbSwitch
				Logf(1, "follow-kb-switch: %v", followKbSwitch)
				// 리스너는 이미 시작되어 있으면 그대로 두고, 꺼진 상태에서 켜면 시작
				if followKbSwitch {
					startVendorInputMirror()
				}
			case <-mQuit.ClickedCh:
				systray.Quit()
				return
			}
		}
	}()
}

func onTrayExit() {
	Log(1, "onTrayExit()")
	cleanup()
}

func processesOK() bool {
    // 장치 하나도 못 열었으면 비정상
    if len(devsOut) == 0 && len(devsFeat) == 0 {
        return false
    }
    // 키보드(7B) 패킷을 내보내야 하는데 OUT이 없으면 비정상
    if !mouseOnly && len(devsOut) == 0 {
        return false
    }
    // 마우스(20B)나 미러링이 필요한데 FEAT가 없으면 비정상
    if (!kbOnly && len(devsFeat) == 0) || (followKbSwitch && len(devsFeat) == 0) {
        return false
    }
    return true
}



//
// ===================== main =====================
//

func main() {
	// GUI 빌드에서도 로그를 확인할 수 있게 파일 로깅 설정
	f, _ := os.OpenFile(logFilePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	log.SetOutput(f)
	defer f.Close()

	parseArgs()
	Logf(1, "START tray=%v tray-only=%v followKbSwitch=%v", trayMode, trayOnly, followKbSwitch)

	openCollections()
	defer cleanup()

	if followKbSwitch && len(devsFeat) == 0 {
        Log(1, "follow-kb-switch requested but FEAT device not opened")
    }

	if testMode {
		runTest()
		return
	}

	if trayMode && trayOnly {
        Log(1, "Tray-only mode: skipping HID check and entering tray")
        runTray()
        return
    }

    // 여기부터는 정상일 때만 트레이 진입
    if trayMode && !trayOnly {
        if !processesOK() {
            Log(1, "Tray requested, but processes are NOT healthy. Exiting without tray.")
            return // 필요하면 os.Exit(2)
        }
        Log(1, "All processes healthy. Entering tray.")
        runTray()
        return
    }


    if trayMode && !trayOnly {
        if !processesOK() {
            Log(1, "Tray requested, but not all processes are healthy. Exiting without tray.")
            return // 조용히 종료 (원하면 os.Exit(1) 로 바꿔도 됨)
        }
        Log(1, "All processes healthy. Entering tray.")
        runTray()
        return
    }

	// (트레이 모드가 아닐 때만) 기존 메시지 루프 유지
	go func() { C.SetMouseHook() }()
	ok := C.RegisterHotKey(nil, C.int(1), C.MOD_CONTROL|C.MOD_SHIFT, C.VK_W)
	if ok == 0 {
		Log(1, "Failed to register hotkey")
	}
	defer C.UnregisterHotKey(nil, C.int(1))

	var msg C.MSG
	for {
		ret := C.GetMessageW(&msg, nil, 0, 0)
		if ret == -1 {
			Log(1, "Error in GetMessage")
		} else if msg.message == C.WM_HOTKEY && msg.wParam == 1 {
			enabled = !enabled
			Logf(1, "Enabled: %v", enabled)
		}
	}
}
