# Code signing

Duster's Windows release files are Authenticode-signed through [SignPath Foundation](https://signpath.org/), which signs open source projects for free. The certificate belongs to SignPath Foundation, so Windows shows **SignPath Foundation** as the publisher.

## Code signing policy

Free code signing provided by [SignPath.io](https://about.signpath.io/), certificate by [SignPath Foundation](https://signpath.org/).

**Status:** signing starts once the project's SignPath Foundation application is approved. Releases published before that are unsigned.

**What is signed:** only files built by [release.yml](../.github/workflows/release.yml) on GitHub-hosted runners, from a pushed release tag:
- `duster-windows-amd64.exe` and `duster-windows-arm64.exe`, which are also the `du.exe` inside the portable zips and the setup exe
- `duw-windows-amd64.exe` and `duw-windows-arm64.exe`, the windowless launcher for scheduled cleans, which are also the `duw.exe` inside the portable zips and the setup exe
- `Duster-Setup-<version>-x64.exe`

Nothing built on a developer machine is signed.

**Team roles:**
- Committers and reviewers: [Nur Adnan](https://github.com/Nur-Adnan). Changes from anyone else are reviewed before they are merged.
- Approvers: [Nur Adnan](https://github.com/Nur-Adnan). Every signing request is approved by hand in SignPath.

**Privacy:** This program will not transfer any information to other networked systems unless specifically requested by the user or the person installing or operating it. Duster connects to the network only when you run `du update` (GitHub's release API and download servers) or the install and uninstall scripts (GitHub).

## What signing changes for users

- **UAC:** the elevation prompt names a verified publisher instead of "Unknown publisher", immediately.
- **SmartScreen:** a signed file is still flagged as unrecognized until the certificate and the file build reputation, which Microsoft says can take weeks of clean installs. Since 2024 no certificate type skips this. Every release signed with the same certificate adds to the same reputation.
- **Smart App Control** (Windows 11) blocks unsigned apps without reputation, so signing matters there too.

### Check a download

```powershell
Get-AuthenticodeSignature .\du.exe | Format-List Status, SignerCertificate, TimeStamperCertificate
```

`Status` must be `Valid`. The build provenance check still works as before: `gh attestation verify <file> --repo Nur-Adnan/Duster`.

## Limits

- **The uninstaller is unsigned.** Inno Setup builds its uninstaller into the setup exe, and signing the finished setup exe doesn't sign it. Inno Setup can only take a pre-signed uninstaller through an interactive compile step, which the CI build can't do. Uninstalling a per-machine install therefore shows a UAC prompt without a verified publisher. `du remove` and `uninstall.ps1` are unaffected.
- **No signature check on update.** `du update` and `install.ps1` verify the SHA-256 checksum, not the Authenticode signature.
- **Releases published before signing started stay unsigned.**

## Maintainer setup

This is done once. Until step 3 is finished, `release.yml` publishes unsigned files and prints a warning.

### 1. Apply to SignPath Foundation

Apply at [signpath.org](https://signpath.org/). Its [terms](https://signpath.org/terms) require:
- an OSI license with no proprietary parts (Duster is MIT)
- a released, maintained project whose download page describes what it does
- multi-factor authentication on GitHub and on SignPath for everyone with a role
- the code signing policy above, linked from the home page (README, "Code signing policy") and the release pages (the release notes add it when signing is on)
- warnings before the software changes the system, and a way to uninstall it (`du remove`, `uninstall.ps1`, the setup's uninstaller)

### 2. Configure SignPath

1. Add the predefined trusted build system **GitHub.com** to the organization, link it to the project, and install the SignPath GitHub App on the repository.
2. Create two artifact configurations with these slugs; release.yml refers to them by name.

   `binaries` signs all four exes in one request. The SignPath project's `binaries` configuration must list `duw-windows-amd64.exe` and `duw-windows-arm64.exe` too, or the sign job's "Verify Signatures" step fails the release:
   ```xml
   <artifact-configuration xmlns="http://signpath.io/artifact-configuration/v1">
     <zip-file>
       <pe-file path="duster-windows-amd64.exe"><authenticode-sign/></pe-file>
       <pe-file path="duster-windows-arm64.exe"><authenticode-sign/></pe-file>
       <pe-file path="duw-windows-amd64.exe"><authenticode-sign/></pe-file>
       <pe-file path="duw-windows-arm64.exe"><authenticode-sign/></pe-file>
     </zip-file>
   </artifact-configuration>
   ```

   `installer` signs the setup exe:
   ```xml
   <artifact-configuration xmlns="http://signpath.io/artifact-configuration/v1">
     <zip-file>
       <pe-file path="Duster-Setup-*-x64.exe"><authenticode-sign/></pe-file>
     </zip-file>
   </artifact-configuration>
   ```
   The root is `<zip-file>` because `actions/upload-artifact` zips what it uploads.
3. Use the release signing policy SignPath sets up for the project, with manual approval. Create a CI user with submitter rights on that policy and generate its API token.

### 3. Configure the GitHub repository

In **Settings > Secrets and variables > Actions**:

| Kind | Name | Value |
|---|---|---|
| Secret | `SIGNPATH_API_TOKEN` | the CI user's API token |
| Variable | `SIGNPATH_ORGANIZATION_ID` | the SignPath organization ID |
| Variable | `SIGNPATH_PROJECT_SLUG` | the SignPath project slug; setting it turns signing on |
| Variable | `SIGNPATH_SIGNING_POLICY_SLUG` | the release signing policy slug |

Only the token is secret: it can submit signing requests but not approve them.

### 4. Each release

`release.yml` sends two signing requests: the four exes from the `sign` job, then the setup exe from the `installer` job. Approve each in SignPath within an hour; the job waits that long and then fails. After signing, the job checks with `Get-AuthenticodeSignature` that every file has a valid, timestamped signature, and fails the release if not. The zips, checksums and attestations are built from the signed files.

To stop signing, delete the `SIGNPATH_PROJECT_SLUG` variable.
