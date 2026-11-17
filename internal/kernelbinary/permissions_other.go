//go:build (!linux && !darwin) || android

package kernelbinary

import "fmt"

func prepareExecutable(path string) error {
	return fmt.Errorf("内核 %q 没有执行权限，请由所有者设置 chmod u+x", path)
}
