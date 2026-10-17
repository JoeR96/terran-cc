package main

import (
	"syscall"
	"unsafe"
)

// chime is kept for the life of the process: PlaySound plays it
// asynchronously from this memory.
var chime = chimeWAV()

func playChime() {
	const sndAsync, sndNoDefault, sndMemory = 0x1, 0x2, 0x4
	syscall.NewLazyDLL("winmm.dll").NewProc("PlaySoundW").Call(uintptr(unsafe.Pointer(&chime[0])), 0, sndAsync|sndNoDefault|sndMemory)
}
