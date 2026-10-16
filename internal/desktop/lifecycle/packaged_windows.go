//go:build windows

package lifecycle

func newNativePackaged(executable, stateRoot, runtimeVersion string) (*Coordinator, error) {
	return NewWindowsPackaged(executable, stateRoot, runtimeVersion)
}
