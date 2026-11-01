package events

import (
	"errors"
	"os"
	"syscall"
)

// OpenFD takes ownership of an explicitly supplied private pipe descriptor.
// It is close-on-exec so media/discovery helpers cannot keep the IPC stream alive.
func OpenFD(fd int) (*Writer, func(), error) {
	if fd < 3 {
		return nil, func() {}, errors.New("event descriptor must be a private pipe")
	}
	file := os.NewFile(uintptr(fd), "scanner-events")
	if file == nil {
		return nil, func() {}, errors.New("event descriptor unavailable")
	}
	info, e := file.Stat()
	if e != nil || info.Mode()&os.ModeNamedPipe == 0 {
		_ = file.Close()
		return nil, func() {}, errors.New("event descriptor must be a private pipe")
	}
	syscall.CloseOnExec(fd)
	return New(file), func() { _ = file.Close() }, nil
}
