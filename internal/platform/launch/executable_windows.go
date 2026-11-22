//go:build windows

package launch

import (
	"errors"
	"fmt"
	"strings"

	"golang.org/x/sys/windows"
)

// ResolveExecutable follows junctions and links through the file handle. This
// works when Windows grants access to the executable but restricts querying a
// parent directory during a component-by-component path walk.
func ResolveExecutable(path string) (string, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return "", err
	}
	handle, err := windows.CreateFile(name, windows.FILE_READ_ATTRIBUTES,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		return "", err
	}
	defer windows.CloseHandle(handle)
	buffer := make([]uint16, 32768)
	length, err := windows.GetFinalPathNameByHandle(handle, &buffer[0], uint32(len(buffer)), 0)
	if errors.Is(err, windows.ERROR_ACCESS_DENIED) {
		// A normalized name can require directory traversal rights on SMB.
		const fileNameOpened = 0x8
		length, err = windows.GetFinalPathNameByHandle(handle, &buffer[0], uint32(len(buffer)), fileNameOpened)
	}
	if err != nil {
		return "", err
	}
	if length >= uint32(len(buffer)) {
		return "", fmt.Errorf("Windows 返回的程序路径过长")
	}
	resolved := windows.UTF16ToString(buffer[:length])
	if unc, ok := strings.CutPrefix(resolved, `\\?\UNC\`); ok {
		return `\\` + unc, nil
	}
	if dos, ok := strings.CutPrefix(resolved, `\\?\`); ok && len(dos) >= 3 && dos[1:3] == `:\` {
		if len(dos) < 260 {
			return dos, nil
		}
	}
	return resolved, nil
}
