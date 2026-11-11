package fsx

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"golang.org/x/sys/windows"
)

func TestPrivateCreationOverridesPublicInheritedDACL(t *testing.T) {
	root := t.TempDir()
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	sid := user.User.Sid.String()
	public, err := windows.SecurityDescriptorFromString("D:P(A;OICI;FA;;;WD)(A;OICI;FA;;;" + sid + ")")
	if err != nil {
		t.Fatal(err)
	}
	acl, _, err := public.DACL()
	if err != nil {
		t.Fatal(err)
	}
	if err := windows.SetNamedSecurityInfo(root, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil); err != nil {
		t.Fatal(err)
	}
	runtime.KeepAlive(public)
	file, dir := filepath.Join(root, "export.json"), filepath.Join(root, "private-directory")
	f, err := CreatePrivateFile(file)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := MkdirPrivate(dir); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{file, dir} {
		inherit := ""
		if path == dir {
			inherit = "OICI"
		}
		want, err := windows.SecurityDescriptorFromString("D:P(A;" + inherit + ";FA;;;" + sid + ")")
		if err != nil {
			t.Fatal(err)
		}
		sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
		if err != nil || sd.String() != want.String() {
			t.Fatalf("public inherited DACL survived private creation: %s %v", path, err)
		}
	}
	child := filepath.Join(dir, "native-tool-auth.json")
	if err := os.WriteFile(child, []byte("DUMMY-CREDENTIAL"), 0o666); err != nil {
		t.Fatal(err)
	}
	childSD, err := windows.GetNamedSecurityInfo(child, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	// The private directory must pass its owner-only grant to a native tool's
	// ordinary file creation, without relying on Windows mode bits.
	dacl, _, err := childSD.DACL()
	if err != nil || dacl.AceCount != 1 {
		t.Fatalf("credential child inherited extra grants: %v", err)
	}
	if _, err := f.WriteString(`{"apiKey":"DUMMY-SECRET"}`); err != nil {
		t.Fatal(err)
	}
}
