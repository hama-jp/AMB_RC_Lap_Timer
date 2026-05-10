@echo off
REM ===========================================================================
REM AMB RC Lap Timer - Defender Firewall 許可ルール追加スクリプト
REM ===========================================================================
REM
REM このファイルを **ダブルクリック** するだけで、同じフォルダにある
REM gateway.exe の inbound TCP を Defender Firewall に許可します。
REM
REM   1. UAC ダイアログが出たら「はい」を押してください(管理者権限が必要)
REM   2. 終わったら何かキーを押してウィンドウを閉じます
REM
REM 別の PC や別のドライブ(C:, F: など)に移動したときは、その都度
REM この .bat を再ダブルクリックすれば最新パスでルールが上書きされます。
REM
REM ルールを後で消したいときは、管理者 PowerShell で:
REM   Remove-NetFirewallRule -DisplayName "AMB RC Lap Timer (gateway.exe)"
REM ===========================================================================

setlocal
title AMB RC Lap Timer - firewall setup

REM ── 自動 UAC 昇格 ────────────────────────────────────────────────────
REM fltmc は管理者でないと開けないので、admin 判定に使う(net session より速い)
fltmc >nul 2>&1
if %errorlevel% neq 0 (
    echo 管理者権限で再実行します...
    powershell -NoProfile -Command "Start-Process -FilePath '%~f0' -Verb RunAs"
    exit /b
)

REM ── gateway.exe のパスをこの .bat の場所から決める ─────────────────
set "GW_EXE=%~dp0gateway.exe"

if not exist "%GW_EXE%" (
    echo.
    echo ERROR: gateway.exe が見つかりません:
    echo   "%GW_EXE%"
    echo.
    echo この .bat と gateway.exe は同じフォルダに置いてください。
    pause
    exit /b 1
)

echo.
echo Defender Firewall に inbound TCP の許可ルールを追加します:
echo   Program: %GW_EXE%
echo.

powershell -NoProfile -Command ^
  "$ErrorActionPreference='Stop';" ^
  "$name = 'AMB RC Lap Timer (gateway.exe)';" ^
  "Get-NetFirewallRule -DisplayName $name -ErrorAction SilentlyContinue | Remove-NetFirewallRule;" ^
  "New-NetFirewallRule -DisplayName $name -Direction Inbound -Action Allow -Profile Any -Program '%GW_EXE%' -Protocol TCP | Out-Null;" ^
  "Write-Host 'OK: ルール追加完了' -ForegroundColor Green"

if %errorlevel% neq 0 (
    echo.
    echo ERROR: ルール追加に失敗しました(上のログを確認してください)
    pause
    exit /b 1
)

echo.
echo ============================================================
echo  完了
echo ============================================================
echo.
echo 続いて gateway.exe をダブルクリックして起動してください。
echo スマホで http://^<このPCのIP^>:8080/ を開けば PASSING が表示されます。
echo.
pause
endlocal
