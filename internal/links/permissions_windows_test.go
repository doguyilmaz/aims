package links

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/doguyilmaz/aims/internal/testutil"
	"golang.org/x/sys/windows"
)

func TestWindowsAdoptionPreservesPrivateDACL(t *testing.T) {
	hub, dir := dirs(t)
	src := filepath.Join(dir, "skills")
	testutil.Write(t, filepath.Join(src, "private", "SKILL.md"), "private instructions")
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	// Propagate the private owner grant to existing children. A non-inheritable
	// replacement removes their inherited access and makes the fixture unreadable
	// before adoption, correctly causing the open-handle check to refuse it.
	sd, err := windows.SecurityDescriptorFromString("D:P(A;OICI;FA;;;" + user.User.Sid.String() + ")")
	if err != nil {
		t.Fatal(err)
	}
	acl, _, err := sd.DACL()
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{src, filepath.Join(src, "private")} {
		if err := windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil); err != nil {
			t.Fatal(err)
		}
	}
	runtime.KeepAlive(sd)
	if b, err := os.ReadFile(filepath.Join(src, "private", "SKILL.md")); err != nil || string(b) != "private instructions" {
		t.Fatalf("private fixture must be readable before adoption: %q %v", b, err)
	}
	before, err := windows.GetNamedSecurityInfo(src, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	if r := ensure(hub, dir)["skills"]; r.Action != Adopted {
		t.Fatalf("adoption failed: %+v", r)
	}
	for _, path := range []string{filepath.Join(hub, "skills"), filepath.Join(hub, "skills", "private")} {
		after, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
		if err != nil || after.String() != before.String() {
			t.Fatalf("private ACL changed: %s %v", path, err)
		}
	}
	if testutil.Read(t, filepath.Join(hub, "skills", "private", "SKILL.md")) != "private instructions" {
		t.Fatal("adoption lost content")
	}
	if _, err := os.Lstat(src); err != nil {
		t.Fatal("profile link missing after restoring captured ACL")
	}
}
