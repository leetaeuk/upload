#NoEnv
#SingleInstance Force
SetBatchLines, -1
SetTitleMatchMode, 2
DetectHiddenWindows, On
CoordMode, Mouse, Screen   ; ★ 마우스 좌표를 화면 전체 기준으로 읽기

; ==== 설정 ====
DEV := "1,2"
EXE := "D:\actionRing\offline_logi_flow_sim_test.exe"

global uiShown := false
global GuiHwnd := 0

; ==== GUI 생성 ====
Gui, 1:Destroy
Gui, 1:+AlwaysOnTop +ToolWindow -Caption hwndGuiHwnd
Gui, 1:Color, F9F9F9
Gui, 1:Font, s11, Segoe UI
Gui, 1:Margin, 15, 15

Gui, 1:Add, Picture, xm+10 ym w32 h32 BackgroundTrans, D:\actionRing\icon.png
Gui, 1:Add, Text, x+30 yp+6 w200 h32 , 디바이스 슬롯 선택  ; 타이틀을 살짝 왼쪽 정렬

BtnColor := "E6E6E6"
HoverColor := "D6D6D6"

Gui, 1:Add, Button, xm+10 y+10 w68 h36 gGo1 vBtn1, 1
Gui, 1:Add, Button, x+10 gGo2 w68 h36 vBtn2, 2
Gui, 1:Add, Button, x+10 gGo3 w68 h36 vBtn3, 3

Gui, 1:Font, s9
Gui, 1:Add, Text, xm w260 Center c888888, ESC / 바깥 클릭 닫기 · 숫자키 1/2/3

Loop, 3
    GuiControl, +Background%BtnColor%, Btn%A_Index%

Gui, 1:+Owner +LastFound
WinSet, Transparent, 245
DllCall("SetWindowRgn", "Ptr", WinExist(), "Ptr", DllCall("CreateRoundRectRgn","Int",0,"Int",0,"Int",280,"Int",140,"Int",25,"Int",25,"Ptr"), "Int",1)
DllCall("dwmapi\DwmSetWindowAttribute", "Ptr", WinExist(), "Int", 2, "Int*", 2, "Int", 4)

SetTimer, CheckOutsideClick, 50

~Esc::
if (uiShown)
    Gosub, HideUI
return

~^MButton:: Gosub, ToggleUI
^!p:: Gosub, ToggleUI
return


; ==== UI 토글 ====
ToggleUI:
if (uiShown)
    Gosub, HideUI
else
    Gosub, ShowNearCursor
return


; ==== 커서 위치 기준 팝업 ====
ShowNearCursor:
MouseGetPos, mx, my
SysGet, MonitorCount, MonitorCount
found := false

Loop, %MonitorCount% {
    SysGet, mon, MonitorWorkArea, %A_Index%
    if (mx >= monLeft && mx <= monRight && my >= monTop && my <= monBottom) {
        found := true
        break
    }
}
if (!found) {
    monLeft := 0, monTop := 0, monRight := A_ScreenWidth, monBottom := A_ScreenHeight
}

Gui, 1:Show, Hide, ActionRingUI
WinGetPos, gx, gy, gw, gh, ahk_id %GuiHwnd%

; 모니터 경계 보정
if (mx + gw > monRight)
    mx := monRight - gw - 10
if (my + gh > monBottom)
    my := monBottom - gh - 10
if (mx < monLeft)
    mx := monLeft + 10
if (my < monTop)
    my := monTop + 10

; ★ 커서 위치 기준 정확히 표시
Gui, 1:Show, x%mx% y%my%, ActionRingUI
uiShown := true

; 숫자키 활성화
Hotkey, 1, Quick1, On
Hotkey, 2, Quick2, On
Hotkey, 3, Quick3, On
return


HideUI:
Gui, 1:Hide
uiShown := false

; 핫키 비활성화 시 오류 방지
try Hotkey, 1, Off
try Hotkey, 2, Off
try Hotkey, 3, Off
return


; ==== 외부 클릭 닫기 ====
CheckOutsideClick:
if (uiShown){
    if (GetKeyState("LButton","P") or GetKeyState("RButton","P")) {
        MouseGetPos,,, hwndUnder, , 2
        if (hwndUnder != GuiHwnd && !DllCall("IsChild","Ptr",GuiHwnd,"Ptr",hwndUnder))
            Gosub, HideUI
        KeyWait, LButton
        KeyWait, RButton
    }
}
return


; ==== 숫자키 핸들러 ====
Quick1:
Quick2:
Quick3:
    if (!uiShown)
        return
    human := SubStr(A_ThisLabel, 6)
    slot := human - 1
    Gosub, HideUI
    Gosub, DoSwitch
return


; ==== 버튼 핸들러 ====
Go1:
Go2:
Go3:
    human := SubStr(A_ThisLabel, 3)
    slot := human - 1
    Gosub, HideUI
    Gosub, DoSwitch
return


; ==== 실제 전환 실행 ====
DoSwitch:
    if !FileExist(EXE){
        MsgBox, 48, 오류, 실행 파일을 찾을 수 없습니다.`n%EXE%
        return
    }
    cmd := """" EXE """ --devices=" DEV " --right-channels=" slot "," slot " --test"
    Run, %ComSpec% /c %cmd%,, Hide UseErrorLevel
    human := slot + 1
    TrayTip, ActionRing, 슬롯 %human% 전환 완료, 1000, 1
return


; ==== 버튼 호버 ====
#IfWinActive ActionRingUI
~LButton Up::
    Loop, 3
        GuiControl, +Background%BtnColor%, Btn%A_Index%
return

~MouseMove:
    if (!uiShown)
        return
    MouseGetPos,,,, ctrl
    Loop, 3 {
        if (ctrl = "Button" A_Index)
            GuiControl, +Background%HoverColor%, Btn%A_Index%
        else
            GuiControl, +Background%BtnColor%, Btn%A_Index%
    }
return
#IfWinActive
