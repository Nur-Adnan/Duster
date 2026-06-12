//go:build windows

package cmd

import (
	"time"

	"golang.org/x/sys/windows"
)

type cpuTimes struct {
	user time.Duration
	sys  time.Duration
}

func getProcessCPUTime() (cpuTimes, error) {
	var creation, exit, kernel, user windows.Filetime
	h, err := windows.GetCurrentProcess()
	if err != nil {
		return cpuTimes{}, err
	}
	err = windows.GetProcessTimes(h, &creation, &exit, &kernel, &user)
	if err != nil {
		return cpuTimes{}, err
	}
	return cpuTimes{
		user: filetimeToDuration(user),
		sys:  filetimeToDuration(kernel),
	}, nil
}

func filetimeToDuration(ft windows.Filetime) time.Duration {
	ns := int64(ft.HighDateTime)<<32 | int64(ft.LowDateTime)
	return time.Duration(ns * 100)
}
