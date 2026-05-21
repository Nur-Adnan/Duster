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

setlocal EnableDelayedExpansion

:: -- Configuration -----------------------------------------------
set "VERSION="
set "INSTALL_DIR=%LOCALAPPDATA%\Duster"
set "SILENT=0"

:: -- Parse Arguments ---------------------------------------------
:parse_args
if "%~1"=="" goto :args_done
if /i "%~1"=="--silent"  ( set "SILENT=1" & shift & goto :parse_args )
if /i "%~1"=="--dir"     ( set "INSTALL_DIR=%~2" & shift & shift & goto :parse_args )
:: Treat bare argument as version number (e.g. install.cmd 1.0.1)
echo %~1 | findstr /r "^[0-9]" >nul 2>&1
if not errorlevel 1 ( set "VERSION=%~1" & shift & goto :parse_args )
shift
goto :parse_args
:args_done

:: -- Banner ------------------------------------------------------
if "%SILENT%"=="0" (
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

:: -- Build PowerShell arguments -----------------------------------
set "PS_ARGS=-NoProfile -NonInteractive -ExecutionPolicy Bypass"

:: Check if local copy of install.ps1 exists
if exist "%~dp0install.ps1" (
    set "PS_SCRIPT=%~dp0install.ps1"
    if "%SILENT%"=="0" echo   Using local installer: !PS_SCRIPT!
) else (
    :: Download from GitHub and run
    set "PS_SCRIPT="
    if "%SILENT%"=="0" echo   Downloading installer from GitHub...
)

:: Build parameter string
set "PS_PARAMS=-InstallDir '%INSTALL_DIR%'"
if not "%VERSION%"=="" set "PS_PARAMS=!PS_PARAMS! -Version '%VERSION%'"
if "%SILENT%"=="1"     set "PS_PARAMS=!PS_PARAMS! -Silent"

:: -- Execute PowerShell installer ---------------------------------
if defined PS_SCRIPT (
    powershell.exe %PS_ARGS% -File "!PS_SCRIPT!" !PS_PARAMS!
) else (
    powershell.exe %PS_ARGS% -Command "& { [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12; $wc = New-Object Net.WebClient; $wc.Encoding = [System.Text.Encoding]::UTF8; $s = $wc.DownloadString('https://raw.githubusercontent.com/Nur-Adnan/Duster/main/scripts/install.ps1'); $sb = [ScriptBlock]::Create($s); & $sb !PS_PARAMS! }"
)

if errorlevel 1 (
    echo.
    echo   Installation failed. See errors above.
    echo   Manual install: https://github.com/Nur-Adnan/Duster/releases/latest
    exit /b 1
)

:: -- Refresh PATH in this CMD session -----------------------------
:: The PowerShell script updated the registry PATH but that doesn't
:: affect this parent CMD process. Read it fresh from the registry.
for /f "tokens=2,*" %%A in ('reg query "HKCU\Environment" /v Path 2^>nul') do (
    set "USER_PATH=%%B"
)
for /f "tokens=2,*" %%A in ('reg query "HKLM\SYSTEM\CurrentControlSet\Control\Session Manager\Environment" /v Path 2^>nul') do (
    set "SYS_PATH=%%B"
)
if defined USER_PATH (
    if defined SYS_PATH (
        set "PATH=!SYS_PATH!;!USER_PATH!"
    ) else (
        set "PATH=!USER_PATH!"
    )
)

:: -- Verify du is now accessible ----------------------------------
where du.exe >nul 2>&1
if errorlevel 1 (
    if "%SILENT%"=="0" (
        echo.
        echo   'du' is not yet available in this window.
        echo   Try opening a NEW Command Prompt window, then run:
        echo     du --version
        echo.
    )
) else (
    if "%SILENT%"=="0" (
        echo.
        echo   Verifying: du --version
        du.exe --version
        echo.
    )
)

endlocal
exit /b 0
