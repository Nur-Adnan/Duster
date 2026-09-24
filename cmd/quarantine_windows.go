//go:build windows

package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unsafe"

	"github.com/Nur-Adnan/duster/internal/logging"
	"github.com/Nur-Adnan/duster/lib/fs"
	"golang.org/x/sys/windows"
)

// Windows quarantine primitives. A kept item is renamed, never copied, into a
// quarantine on its own volume:
//   - the volume of %LOCALAPPDATA%: logging.Dir()\quarantine (already private
//     to the user, like the rest of the profile);
//   - any other fixed or removable drive X: X:\.duster-quarantine\<user SID>,
//     created with a protected DACL (full control for the user and SYSTEM,
//     nothing inherited) and owned by the user.
//
// The guarantee: other users cannot list what you have kept, and nothing
// becomes readable that was not readable before. A same-volume move keeps the
// item's own ACL (it does not re-inherit from the SID folder), so someone who
// knows an item's full path can still open it if its ACL allowed that at its
// original location. Item ACLs are deliberately left alone: restore must bring
// back exactly the original permissions.
//
// Network drives and volumes mounted in a folder get no quarantine: the item
// stays where it was and the caller reports errNoQuarantine.

// extendedPath gives a long absolute path the `\\?\` prefix (`\\?\UNC\` for a
// share) so raw Win32 calls accept it past MAX_PATH, as Go's os package does
// internally. Like Go, it leaves paths under 248 characters alone: they keep
// Win32's normal parsing (trailing dots and spaces), which is what os.Lstat
// and the fs path checks saw. Relative and already-prefixed paths pass through.
func extendedPath(p string) string {
	if len(p) < 248 || strings.HasPrefix(p, `\\?\`) || strings.HasPrefix(p, `\\.\`) || !filepath.IsAbs(p) {
		return p
	}
	p = filepath.Clean(p) // `\\?\` turns off Win32's own ".." and "/" handling
	if strings.HasPrefix(p, `\\`) {
		return `\\?\UNC\` + p[2:]
	}
	return `\\?\` + p
}

// plainPath undoes extendedPath's prefix on a path Windows returned.
func plainPath(p string) string {
	switch {
	case strings.HasPrefix(p, `\\?\UNC\`):
		return `\\` + p[len(`\\?\UNC\`):]
	case strings.HasPrefix(p, `\\?\`) && len(p) >= 6 && p[5] == ':':
		return p[len(`\\?\`):]
	}
	return p
}

// volumeRoot returns the root of the volume holding path, such as `D:\`.
func volumeRoot(path string) (string, error) {
	p, err := windows.UTF16PtrFromString(extendedPath(path))
	if err != nil {
		return "", err
	}
	root := make([]uint16, windows.MAX_PATH+1)
	if err := windows.GetVolumePathName(p, &root[0], uint32(len(root))); err != nil {
		return "", fmt.Errorf("volume of %s: %w", path, err)
	}
	return plainPath(windows.UTF16ToString(root)), nil
}

// isDriveLetterRoot reports whether a volume root is a drive letter (`D:\`).
// quarantineRoots only finds quarantines by drive letter, so a volume mounted
// in a folder gets none: anything kept there could never be listed, restored
// or expired.
func isDriveLetterRoot(root string) bool {
	return len(root) == 3 && root[1] == ':' && root[2] == '\\' &&
		(root[0]|0x20) >= 'a' && (root[0]|0x20) <= 'z'
}

// quarantineDriveType reports whether a volume root is a fixed or removable
// drive, the only kinds that get a quarantine.
func quarantineDriveType(root string) bool {
	p, err := windows.UTF16PtrFromString(root)
	if err != nil {
		return false
	}
	t := windows.GetDriveType(p)
	return t == windows.DRIVE_FIXED || t == windows.DRIVE_REMOVABLE
}

// persistentACLs reports whether the volume at root keeps file ACLs (NTFS,
// ReFS). FAT and exFAT do not, so a private folder cannot exist there.
func persistentACLs(root string) (bool, error) {
	p, err := windows.UTF16PtrFromString(root)
	if err != nil {
		return false, err
	}
	var flags uint32
	if err := windows.GetVolumeInformation(p, nil, 0, nil, nil, &flags, nil, 0); err != nil {
		return false, fmt.Errorf("volume information for %s: %w", root, err)
	}
	return flags&windows.FILE_PERSISTENT_ACLS != 0, nil
}

// currentUserSID returns the SID of the user this process runs as, such as
// S-1-5-21-...-1001. An elevated process of the same user has the same SID.
func currentUserSID() (string, error) {
	tu, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return "", fmt.Errorf("current user: %w", err)
	}
	return tu.User.Sid.String(), nil
}

// localQuarantineVolume returns the volume root of logging.Dir(), or "" when
// there is no profile folder.
func localQuarantineVolume() string {
	d := logging.Dir()
	if d == "" {
		return ""
	}
	vr, err := volumeRoot(d)
	if err != nil {
		return ""
	}
	return vr
}

// createPrivateDir creates path so that only sid and SYSTEM can list it: the
// DACL is protected (nothing inherited from the parent) and sid is the owner.
// The owner is set explicitly because an elevated administrator's objects are
// otherwise owned by the Administrators group. On a volume without persistent
// ACLs (FAT, exFAT) there is nothing to protect, so it is a plain folder.
func createPrivateDir(path, sid string) error {
	vr, err := volumeRoot(path)
	if err != nil {
		return err
	}
	acls, err := persistentACLs(vr)
	if err != nil {
		return err
	}
	if !acls {
		return os.Mkdir(path, 0o700)
	}
	sd, err := windows.SecurityDescriptorFromString("O:" + sid + "D:P(A;OICI;FA;;;" + sid + ")(A;OICI;FA;;;SY)")
	if err != nil {
		return fmt.Errorf("security descriptor for %s: %w", sid, err)
	}
	p, err := windows.UTF16PtrFromString(extendedPath(path))
	if err != nil {
		return err
	}
	sa := windows.SecurityAttributes{
		Length:             uint32(unsafe.Sizeof(windows.SecurityAttributes{})),
		SecurityDescriptor: sd,
	}
	if err := windows.CreateDirectory(p, &sa); err != nil {
		return &os.PathError{Op: "mkdir", Path: path, Err: err}
	}
	return nil
}

// dirOwner returns the SID string of path's owner.
func dirOwner(path string) (string, error) {
	sd, err := windows.GetNamedSecurityInfo(extendedPath(path), windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION)
	if err != nil {
		return "", fmt.Errorf("owner of %s: %w", path, err)
	}
	owner, _, err := sd.Owner()
	if err != nil {
		return "", fmt.Errorf("owner of %s: %w", path, err)
	}
	if owner == nil {
		return "", fmt.Errorf("%s has no owner", path)
	}
	return owner.String(), nil
}

// checkOwnQuarantine refuses dir unless it is a plain folder (no link or
// junction) and, on a volume that keeps ACLs, owned by sid. Without the owner
// check another user could pre-create a folder under our SID that they can
// list and open, and everything kept there would be theirs to see.
func checkOwnQuarantine(dir, volRoot, sid string) error {
	if !realDir(dir) {
		return fmt.Errorf("%s is not a plain folder, so Duster will not keep items there", dir)
	}
	acls, err := persistentACLs(volRoot)
	if err != nil {
		return err
	}
	if !acls {
		return nil
	}
	owner, err := dirOwner(dir)
	if err != nil {
		return err
	}
	if !strings.EqualFold(owner, sid) {
		return fmt.Errorf("%s belongs to someone else (%s), so Duster will not keep items there", dir, owner)
	}
	return nil
}

// pinnedQuarantines holds an open handle to each X:\.duster-quarantine\<SID>
// folder this process verified, for the rest of the process. See
// pinQuarantineDir.
var (
	pinnedMu          sync.Mutex
	pinnedQuarantines = map[string]windows.Handle{}
)

func pinKey(dir string) string { return strings.ToLower(filepath.Clean(dir)) }

// quarantinePinned reports whether dir is already pinned by this process.
func quarantinePinned(dir string) bool {
	pinnedMu.Lock()
	defer pinnedMu.Unlock()
	_, ok := pinnedQuarantines[pinKey(dir)]
	return ok
}

// pinQuarantineDir opens dir and keeps the handle open for the rest of the
// process, then verifies through that handle what checkOwnQuarantine checked
// by path: a directory, not a reparse point, at exactly dir (no junction on
// the way), owned by sid on a volume that keeps ACLs.
//
// The handle does not share delete access, so while it is open Windows refuses
// to rename or delete dir, and refuses to rename any folder above it. Another
// user who owns X:\.duster-quarantine therefore cannot swap the verified
// folder for a junction or a folder of their own between the check and the
// moves: kept items and session.json always land in the folder that was
// verified. A second call for the same dir reuses the handle.
func pinQuarantineDir(dir, volRoot, sid string) error {
	pinnedMu.Lock()
	defer pinnedMu.Unlock()
	key := pinKey(dir)
	if _, ok := pinnedQuarantines[key]; ok {
		return nil
	}
	p, err := windows.UTF16PtrFromString(extendedPath(dir))
	if err != nil {
		return err
	}
	h, err := windows.CreateFile(p,
		windows.FILE_READ_ATTRIBUTES|windows.READ_CONTROL,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, // no FILE_SHARE_DELETE: that is the pin
		nil, windows.OPEN_EXISTING,
		windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return &os.PathError{Op: "open", Path: dir, Err: err}
	}
	if err := verifyPinnedDir(h, dir, volRoot, sid); err != nil {
		windows.CloseHandle(h)
		return err
	}
	pinnedQuarantines[key] = h
	return nil
}

func verifyPinnedDir(h windows.Handle, dir, volRoot, sid string) error {
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(h, &info); err != nil {
		return &os.PathError{Op: "stat", Path: dir, Err: err}
	}
	if info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0 || info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return fmt.Errorf("%s is not a plain folder, so Duster will not keep items there", dir)
	}
	buf := make([]uint16, windows.MAX_PATH+1)
	for {
		// Flags 0: FILE_NAME_NORMALIZED | VOLUME_NAME_DOS, a `\\?\X:\...` path.
		n, err := windows.GetFinalPathNameByHandle(h, &buf[0], uint32(len(buf)), 0)
		if err != nil {
			return &os.PathError{Op: "resolve", Path: dir, Err: err}
		}
		if int(n) < len(buf) {
			if final := plainPath(windows.UTF16ToString(buf[:n])); !strings.EqualFold(final, filepath.Clean(dir)) {
				return fmt.Errorf("%s leads to %s, so Duster will not keep items there", dir, final)
			}
			break
		}
		buf = make([]uint16, n)
	}
	acls, err := persistentACLs(volRoot)
	if err != nil {
		return err
	}
	if !acls {
		return nil
	}
	sd, err := windows.GetSecurityInfo(h, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION)
	if err != nil {
		return fmt.Errorf("owner of %s: %w", dir, err)
	}
	owner, _, err := sd.Owner()
	if err != nil {
		return fmt.Errorf("owner of %s: %w", dir, err)
	}
	if owner == nil {
		return fmt.Errorf("%s has no owner", dir)
	}
	if !strings.EqualFold(owner.String(), sid) {
		return fmt.Errorf("%s belongs to someone else (%s), so Duster will not keep items there", dir, owner)
	}
	return nil
}

// unpinQuarantineDir closes a pin. Only tests use it: in the product a pin
// lasts until the process exits.
func unpinQuarantineDir(dir string) {
	pinnedMu.Lock()
	defer pinnedMu.Unlock()
	if h, ok := pinnedQuarantines[pinKey(dir)]; ok {
		windows.CloseHandle(h)
		delete(pinnedQuarantines, pinKey(dir))
	}
}

// quarantineRoot returns the quarantine folder on path's volume, creating it
// when needed. It returns errNoQuarantine for network drives and volumes
// mounted in a folder, and an error for a root that is a link or that another
// user owns. The caller then leaves the item in place. A folder on another
// drive is pinned (pinQuarantineDir) before it is returned, so it cannot be
// swapped while this process uses it.
func quarantineRoot(path string) (string, error) {
	vr, err := volumeRoot(path)
	if err != nil {
		return "", err
	}
	if !quarantineDriveType(vr) {
		return "", errNoQuarantine
	}

	if local := localQuarantineVolume(); local != "" && strings.EqualFold(vr, local) {
		d := logging.Dir()
		q := filepath.Join(d, "quarantine")
		for _, p := range []string{d, q} {
			if err := ensureRealDir(p); err != nil {
				return "", err
			}
		}
		return q, nil
	}
	if !isDriveLetterRoot(vr) {
		return "", errNoQuarantine
	}

	sid, err := currentUserSID()
	if err != nil {
		return "", err
	}
	base := filepath.Join(vr, quarantineDirName)
	dir := filepath.Join(base, sid)
	if quarantinePinned(dir) {
		return dir, nil
	}
	if err := ensureRealDir(base); err != nil {
		return "", err
	}
	if p, err := windows.UTF16PtrFromString(base); err == nil {
		// Best effort, hiding is cosmetic. Add the bit, keep the others.
		if attrs, err := windows.GetFileAttributes(p); err == nil && attrs&windows.FILE_ATTRIBUTE_HIDDEN == 0 {
			_ = windows.SetFileAttributes(p, attrs|windows.FILE_ATTRIBUTE_HIDDEN)
		}
	}

	if _, err := os.Lstat(dir); errors.Is(err, os.ErrNotExist) {
		// ERROR_ALREADY_EXISTS means another Duster won the race; the checks
		// below decide whether that folder is ours.
		if err := createPrivateDir(dir, sid); err != nil && !errors.Is(err, os.ErrExist) {
			return "", err
		}
	} else if err != nil {
		return "", err
	}
	if err := checkOwnQuarantine(dir, vr, sid); err != nil {
		return "", err
	}
	if err := pinQuarantineDir(dir, vr, sid); err != nil {
		return "", err
	}
	return dir, nil
}

// quarantineRoots lists every quarantine on this machine that belongs to the
// current user: the local one plus X:\.duster-quarantine\<SID> on each fixed
// or removable drive. It only reads; a root that is a link or owned by another
// user is left out, so restore and expiry never act on a planted folder.
func quarantineRoots() []string {
	var roots []string
	if d := logging.Dir(); d != "" {
		if q := filepath.Join(d, "quarantine"); realDir(d) && realDir(q) {
			roots = append(roots, q)
		}
	}
	sid, err := currentUserSID()
	if err != nil {
		return roots
	}
	local := localQuarantineVolume()
	mask, err := windows.GetLogicalDrives()
	if err != nil {
		return roots
	}
	for i := 0; i < 26; i++ {
		if mask&(1<<uint(i)) == 0 {
			continue
		}
		vr := string(rune('A'+i)) + `:\`
		if strings.EqualFold(vr, local) || !quarantineDriveType(vr) {
			continue
		}
		dir := filepath.Join(vr, quarantineDirName, sid)
		if _, err := os.Lstat(dir); err != nil {
			continue
		}
		if checkOwnQuarantine(dir, vr, sid) == nil {
			roots = append(roots, dir)
		}
	}
	return roots
}

// moveNoReplace renames from to to on the same volume and never overwrites:
// it calls MoveFileEx without MOVEFILE_REPLACE_EXISTING (os.Rename always
// passes it) and without MOVEFILE_COPY_ALLOWED, so an existing target and a
// cross-volume move both fail and leave from where it was. A link is moved as
// the link itself. An existing target satisfies errors.Is(err, os.ErrExist).
// Paths past MAX_PATH work (extendedPath).
func moveNoReplace(from, to string) error {
	f, err := windows.UTF16PtrFromString(extendedPath(fs.LongPath(from)))
	if err != nil {
		return &os.LinkError{Op: "move", Old: from, New: to, Err: err}
	}
	t, err := windows.UTF16PtrFromString(extendedPath(fs.LongPath(to)))
	if err != nil {
		return &os.LinkError{Op: "move", Old: from, New: to, Err: err}
	}
	if err := windows.MoveFileEx(f, t, 0); err != nil {
		return &os.LinkError{Op: "move", Old: from, New: to, Err: err}
	}
	return nil
}
