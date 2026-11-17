//go:build (linux || darwin) && !android

package instance

import (
	"errors"
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
)

type Lock struct{ file *os.File }

func Acquire(role string) (*Lock, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	directory := filepath.Join(base, "ReiaClash")
	return AcquireIn(directory, role)
}

func AcquireIn(directory, role string) (*Lock, error) {
	if err := os.MkdirAll(directory, 0700); err != nil {
		return nil, err
	}
	// flock does not require write access; a root-owned service lock must also
	// be usable by the desktop user after the service has stopped.
	path := filepath.Join(directory, role+".lock")
	fd, err := unix.Open(path, unix.O_CREAT|unix.O_EXCL|unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0644)
	if err == nil {
		if chmodErr := unix.Fchmod(fd, 0644); chmodErr != nil {
			unix.Close(fd)
			return nil, chmodErr
		}
	} else if errors.Is(err, unix.EEXIST) {
		fd, err = unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	}
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), role+".lock")
	if err := unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB); err != nil {
		file.Close()
		if errors.Is(err, unix.EWOULDBLOCK) {
			return nil, ErrAlreadyRunning
		}
		return nil, err
	}
	return &Lock{file: file}, nil
}
func (l *Lock) Close() error { return l.file.Close() }
