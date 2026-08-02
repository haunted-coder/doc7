//go:build windows

package i18n

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

func detectSystemLocale() string {
	kernel32 := windows.NewLazySystemDLL("kernel32.dll")
	procedure := kernel32.NewProc("GetUserDefaultLocaleName")
	buffer := make([]uint16, 85)
	result, _, _ := procedure.Call(uintptr(unsafe.Pointer(&buffer[0])), uintptr(len(buffer)))
	if result == 0 {
		return ""
	}
	return windows.UTF16ToString(buffer)
}
