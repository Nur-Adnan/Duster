//go:build windows

package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

// volumeRoot returns the root of the volume holding path, such as `D:\`.
func volumeRoot(path string) (string, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return "", err
	}
	root := make([]uint16, windows.MAX_PATH+1)
	if err := windows.GetVolumePathName(p, &root[0], uint32(len(root))); err != nil {
		return "", fmt.Errorf("volume of %s: %w", path, err)
	}
	return windows.UTF16ToString(root), nil
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
	p, err := windows.UTF16PtrFromString(path)
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
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION)
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

// quarantineRoot returns the quarantine folder on path's volume, creating it
// when needed. It returns errNoQuarantine for network drives and volumes
// mounted in a folder, and an error for a root that is a link or that another
// user owns. The caller then leaves the item in place.
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
	if err := ensureRealDir(base); err != nil {
		return "", err
	}
	if p, err := windows.UTF16PtrFromString(base); err == nil {
		_ = windows.SetFileAttributes(p, windows.FILE_ATTRIBUTE_HIDDEN) // best effort: hiding is cosmetic
	}

	dir := filepath.Join(base, sid)
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
	return dir, nil
}

// quarantineRoots lists every quarantine on this machine that belongs to the
// current user: the local one plus X:\.duster-quarantine\<SID> on each fixed
// or removable drive. It only reads; a root that is a link or owned by another
// user is left out, so restore and expiry never act on a planted folder.
func quarantineRoots() []string {
	var roots []string
	if d := logging.Dir(); d != "" {
		if q := filepath.Join(d, "quarantine"); realDir(q) {
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
func moveNoReplace(from, to string) error {
	f, err := windows.UTF16PtrFromString(fs.LongPath(from))
	if err != nil {
		return &os.LinkError{Op: "move", Old: from, New: to, Err: err}
	}
	t, err := windows.UTF16PtrFromString(fs.LongPath(to))
	if err != nil {
		return &os.LinkError{Op: "move", Old: from, New: to, Err: err}
	}
	if err := windows.MoveFileEx(f, t, 0); err != nil {
		return &os.LinkError{Op: "move", Old: from, New: to, Err: err}
	}
	return nil
}
