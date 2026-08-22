//go:build linux

package lifecycle

func newNativePackaged(executable, stateRoot, runtimeVersion string) (*Coordinator, error) {
	return NewLinuxPackaged(executable, stateRoot, runtimeVersion)
}
