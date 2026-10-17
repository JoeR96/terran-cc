package main

import (
	"syscall"
	"unsafe"
)

func messageBox(title, text string) {
	t, _ := syscall.UTF16PtrFromString(title)
	m, _ := syscall.UTF16PtrFromString(text)
	const mbIconError = 0x10
	syscall.NewLazyDLL("user32.dll").NewProc("MessageBoxW").Call(0, uintptr(unsafe.Pointer(m)), uintptr(unsafe.Pointer(t)), mbIconError)
}
