@echo off
:: ================================================================
::  Duster - CMD/Batch Installer
::  https://github.com/Nur-Adnan/Duster
::
::  Usage:
::    install.cmd                   Install latest version
::    install.cmd 1.0.1             Install specific version
::    install.cmd --silent          Install silently
::    install.cmd --dir C:\MyTools  Install to custom directory
:: ================================================================

:: No delayed expansion: a "!" in a path would be eaten by it.
setlocal

:: -- Configuration -----------------------------------------------
:: Values reach PowerShell through DUSTER_* environment variables, never as
:: command-line text, so spaces, quotes and apostrophes in a path need no
:: escaping. -InstallDir is passed only when --dir was given, so install.ps1
:: keeps its WDAC/AppLocker fallback to Program Files.
set "DUSTER_VERSION="
set "DUSTER_INSTALL_DIR="
set "DUSTER_SILENT=0"

:: -- Parse Arguments ---------------------------------------------
:parse_args
if "%~1"=="" goto :args_done
if /i "%~1"=="--silent"  ( set "DUSTER_SILENT=1" & shift & goto :parse_args )
if /i "%~1"=="--dir"     ( set "DUSTER_INSTALL_DIR=%~2" & shift & shift & goto :parse_args )
:: Treat bare argument as version number (e.g. install.cmd 1.0.1)
echo %~1 | findstr /r "^[0-9]" >nul 2>&1
if not errorlevel 1 ( set "DUSTER_VERSION=%~1" & shift & goto :parse_args )
shift
goto :parse_args
:args_done

:: -- Banner ------------------------------------------------------
if "%DUSTER_SILENT%"=="0" (
    echo.
    echo   =================================================
    echo     Duster - Windows System Cleaner  [CMD Installer]
    echo     https://github.com/Nur-Adnan/Duster
    echo   =================================================
    echo.
)

:: -- Check PowerShell availability --------------------------------
where powershell.exe >nul 2>&1
if errorlevel 1 (
    echo   ERROR: PowerShell is required but not found.
    echo   Download Duster manually from:
    echo   https://github.com/Nur-Adnan/Duster/releases/latest
    exit /b 1
)

:: A local install.ps1 next to this script (both downloaded from the same
:: release) is used as-is; otherwise the script is fetched from GitHub.
set "DUSTER_LOCAL_SCRIPT="
if exist "%~dp0install.ps1" set "DUSTER_LOCAL_SCRIPT=%~dp0install.ps1"
if "%DUSTER_SILENT%"=="0" if defined DUSTER_LOCAL_SCRIPT echo   Using local installer: %DUSTER_LOCAL_SCRIPT%
if "%DUSTER_SILENT%"=="0" if not defined DUSTER_LOCAL_SCRIPT echo   Downloading installer from GitHub...

:: -- Execute PowerShell installer ---------------------------------
powershell.exe -NoProfile -NonInteractive -ExecutionPolicy Bypass -Command "$ErrorActionPreference = 'Stop'; $p = @{}; if ($env:DUSTER_INSTALL_DIR) { $p.InstallDir = $env:DUSTER_INSTALL_DIR }; if ($env:DUSTER_VERSION) { $p.Version = $env:DUSTER_VERSION }; if ($env:DUSTER_SILENT -eq '1') { $p.Silent = $true }; try { if ($env:DUSTER_LOCAL_SCRIPT) { & $env:DUSTER_LOCAL_SCRIPT @p } else { [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12; & ([scriptblock]::Create((New-Object Net.WebClient).DownloadString('https://raw.githubusercontent.com/Nur-Adnan/Duster/main/scripts/install.ps1'))) @p } } catch { Write-Host $_ -ForegroundColor Red; exit 1 }"

if errorlevel 1 (
    echo.
    echo   Installation failed. See errors above.
    echo.
    echo   If you see "Application Control policy has blocked this file":
    echo   Your organization's security policy blocks executables from AppData.
    echo   FIX: Run as Administrator to install to Program Files:
    echo     powershell -NoProfile -ExecutionPolicy Bypass -Command "Start-Process powershell -Verb RunAs -ArgumentList '-NoProfile -ExecutionPolicy Bypass -Command ""irm https://raw.githubusercontent.com/Nur-Adnan/Duster/main/scripts/install.ps1 | iex""'"
    echo.
    echo   Manual install: https://github.com/Nur-Adnan/Duster/releases/latest
    exit /b 1
)

:: -- Verify ------------------------------------------------------
:: This window's PATH predates the install, so run du from where it landed.
set "DUSTER_EXE="
if defined DUSTER_INSTALL_DIR if exist "%DUSTER_INSTALL_DIR%\du.exe" set "DUSTER_EXE=%DUSTER_INSTALL_DIR%\du.exe"
if not defined DUSTER_EXE if exist "%LOCALAPPDATA%\Duster\du.exe" set "DUSTER_EXE=%LOCALAPPDATA%\Duster\du.exe"
if not defined DUSTER_EXE if exist "%ProgramFiles%\Duster\du.exe" set "DUSTER_EXE=%ProgramFiles%\Duster\du.exe"

if "%DUSTER_SILENT%"=="0" (
    echo.
    if defined DUSTER_EXE "%DUSTER_EXE%" --version
    echo   Open a NEW terminal window to use 'du' from anywhere.
    echo.
)

endlocal
exit /b 0
