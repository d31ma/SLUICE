//go:build !darwin && !linux && !windows

package autostart

import "fmt"

func New(string) (Manager, error) {
	return nil, fmt.Errorf("autostart is not supported on this operating system")
}
