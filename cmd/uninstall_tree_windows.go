//go:build windows

package cmd

import (
	"fmt"
	"os/exec"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// jobAccounting mirrors JOBOBJECT_BASIC_ACCOUNTING_INFORMATION (x/sys has no type for it).
type jobAccounting struct {
	TotalUserTime, TotalKernelTime, ThisPeriodTotalUserTime, ThisPeriodTotalKernelTime int64
	TotalPageFaultCount, TotalProcesses, ActiveProcesses, TotalTerminatedProcesses     uint32
}

// runTree runs cmd and waits for it and every process it starts. Inno Setup
// and NSIS uninstallers copy themselves to %TEMP%, start the copy and exit at
// once, so waiting on the first process alone checked the registry while the
// vendor's "Are you sure?" dialog was still open.
//
// The child starts suspended inside a job object, so no process it starts can
// be missed. Once done reports true (the app's entry is gone), the tree gets
// 30 s more to exit, so a survey page left open in a browser can't stall
// Duster. The job never kills anything: closing Duster leaves the uninstaller
// running.
func runTree(cmd *exec.Cmd, done func() bool) error {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return cmd.Run()
	}
	defer windows.CloseHandle(job)

	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= windows.CREATE_SUSPENDED
	if err := cmd.Start(); err != nil {
		return err
	}
	pid := uint32(cmd.Process.Pid)
	inJob := assignToJob(job, pid) == nil // if not, fall back to waiting on the first process
	if err := resumeProcess(pid); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return fmt.Errorf("start uninstaller: %w", err)
	}
	err = cmd.Wait()

	// ponytail: 1 s poll; a job completion port would wake at once.
	var goneAt time.Time
	for inJob && jobActive(job) {
		if goneAt.IsZero() && done() {
			goneAt = time.Now()
		}
		if !goneAt.IsZero() && time.Since(goneAt) > 30*time.Second {
			break
		}
		time.Sleep(time.Second)
	}
	return err
}

func assignToJob(job windows.Handle, pid uint32) error {
	h, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, pid)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(h)
	return windows.AssignProcessToJobObject(job, h)
}

// resumeProcess resumes a process created suspended; its main thread is its only one.
func resumeProcess(pid uint32) error {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(snap)
	te := windows.ThreadEntry32{Size: uint32(unsafe.Sizeof(windows.ThreadEntry32{}))}
	for err = windows.Thread32First(snap, &te); err == nil; err = windows.Thread32Next(snap, &te) {
		if te.OwnerProcessID != pid {
			continue
		}
		th, oerr := windows.OpenThread(windows.THREAD_SUSPEND_RESUME, false, te.ThreadID)
		if oerr != nil {
			return oerr
		}
		_, rerr := windows.ResumeThread(th)
		windows.CloseHandle(th)
		return rerr
	}
	return fmt.Errorf("no thread found for process %d", pid)
}

func jobActive(job windows.Handle) bool {
	var info jobAccounting
	err := windows.QueryInformationJobObject(job, windows.JobObjectBasicAccountingInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)), nil)
	return err == nil && info.ActiveProcesses > 0
}
