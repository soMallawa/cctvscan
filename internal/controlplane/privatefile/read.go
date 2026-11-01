package privatefile

import (
	"errors"
	"io"
	"os"
	"syscall"
)

func Read(path string, max int64) ([]byte, error) {
	if path == "" {
		return nil, errors.New("required private file missing")
	}
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, errors.New("private file unavailable")
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() || st.Mode().Perm()&0077 != 0 || st.Size() > max {
		return nil, errors.New("private file must be bounded, regular and owner-only")
	}
	b, err := io.ReadAll(io.LimitReader(f, max+1))
	if err != nil || int64(len(b)) > max {
		return nil, errors.New("private file unreadable")
	}
	return b, nil
}
