//go:build darwin

package lifecycle

func newNativePackaged(executable, stateRoot, runtimeVersion string) (*Coordinator, error) {
	return NewMacOSPackaged(executable, stateRoot, runtimeVersion)
}
