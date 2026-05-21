---
name: 🧹 New Cleanup Rule / Target Suggestion
about: Propose a new application cache, temp folder, or browser target for Duster to clean.
title: "[CLEAN] "
labels: cleanup-target
assignees: ""
---

**Application / Target Name**
Provide the exact name of the application or system component (e.g. "VLC Media Player", "DirectX Shader Cache").

**Typical Cache / Temp File Locations**
Where are these temporary or cache files stored? Please use standard Windows environment path notation (e.g. `%LocalAppData%\VLC\art`, `%SystemRoot%\Temp`).

**Safeguard Rules & Validation Warnings**
- What files or file extensions should be *excluded* or skipped to ensure safety?
- Are there any running processes related to this app that Duster must check or warn about before cleaning?

**Potential Reclaimable Space**
Approximately how much space does this target consume over time (e.g. 50MB, 2GB)?

**Additional Context**
Any additional information about registry keys or log locations associated with this application.
