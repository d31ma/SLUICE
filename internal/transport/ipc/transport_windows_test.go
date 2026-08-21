//go:build windows

package ipc

import (
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func TestNativeEndpointProducesRestrictedNamedPipeAddress(t *testing.T) {
	capability, err := NewCapability()
	if err != nil {
		t.Fatalf("create capability: %v", err)
	}
	network, address, err := nativeEndpoint(`C:\Users\sluice\session.json`, capability)
	if err != nil {
		t.Fatalf("create native endpoint: %v", err)
	}
	if network != "npipe" || !privatePipeAddress.MatchString(address) {
		t.Fatalf("native endpoint = %q %q, want validated named pipe", network, address)
	}
}

func TestCurrentUserPipeSecurityDescriptorGrantsOnlyCurrentUser(t *testing.T) {
	descriptor, err := currentUserPipeSecurityDescriptor()
	if err != nil {
		t.Fatalf("create named-pipe ACL: %v", err)
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatalf("resolve current user: %v", err)
	}
	want := "D:P(A;;GA;;;" + user.User.Sid.String() + ")"
	if descriptor != want {
		t.Fatalf("named-pipe ACL = %q, want %q", descriptor, want)
	}
	if strings.Contains(descriptor, ";;;WD)") || strings.Contains(descriptor, ";;;AU)") {
		t.Fatalf("named-pipe ACL grants a broad principal: %q", descriptor)
	}
	if _, err := windows.SecurityDescriptorFromString(descriptor); err != nil {
		t.Fatalf("parse named-pipe ACL: %v", err)
	}
}

func TestNamedPipeDescriptorRejectsArbitraryPipeNames(t *testing.T) {
	if err := validateTransportAddress("npipe", `\\.\pipe\other-process`); err == nil {
		t.Fatal("validateTransportAddress() error = nil, want arbitrary-pipe rejection")
	}
}
