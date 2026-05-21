# Duster — Windows Defender Preemptive Whitelisting Guide

To prevent heuristic false-positives and avoid SmartScreen blockades on release, follow this standard procedure for submitting Duster binaries directly to Microsoft Security Intelligence.

## 1. Prerequisites
- Authenticode Code-Signed binaries (`duster-windows-amd64.exe` and `duster-windows-arm64.exe`).
- Valid Microsoft Partner Center account (recommended but not required for submission).

## 2. Submission Portal
Navigate to the official Microsoft Security Intelligence Submission Portal:
👉 **[Microsoft Security Intelligence - Submit a File](https://www.microsoft.com/en-us/wdsi/filesubmission)**

## 3. Fill Out Submission Details
Select the **"Developer"** option:
- **Product**: Microsoft Defender Antivirus
- **Company Name**: *[Insert Company/Maintainer Name]*
- **File Name**: `duster-windows-amd64.exe` (repeat submission for ARM64 version)
- **Is this file signed?**: Yes
- **Detection Name (if any)**: *Leave blank unless a heuristic flag was raised*
- **Description/Context**: 
  > Duster is a high-performance, open-source deep cleaning utility for Windows. It deletes user caches (such as browser caches, delivery optimization buffers, and local logs) to reclaim disk space. It interfaces with native Win32 APIs (e.g., `SHFileOperationW`) to move files securely to the Recycle Bin. It contains zero malicious behaviors. We are submitting this release binary preemptively for whitelisting and cleaning reputation score.

## 4. Post-Submission Processing
1. You will receive a **Submission ID** (e.g., `MWSID1234567`).
2. Microsoft's automated sandbox analysis takes between **2 to 12 hours**.
3. Once approved, the detection signature will be updated globally. Microsoft Defender signature database updates (typically downloaded automatically on client PCs via Windows Update) will clear any active heuristic triggers.
