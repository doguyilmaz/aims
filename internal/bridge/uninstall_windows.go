package bridge

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

// A mapped Windows image also locks its hard-link aliases. The system helper
// waits on an inherited process handle, not a reusable PID, then rechecks the
// installation under the same lock before removing only the captured files.
// No helper executable, script file, scheduled task or reboot entry is created.
func deferLauncherUninstall(root string, m *Launchers) (bool, error) {
	self, err := os.Executable()
	if err != nil {
		return false, err
	}
	mapped := false
	for _, l := range m.Tools {
		if l.Kind == "hardlink" && samePath(filepath.Join(root, "bin", l.Name), self) {
			mapped = true
		}
	}
	if !mapped {
		return false, nil
	}
	parent, err := windows.OpenProcess(windows.SYNCHRONIZE, true, uint32(os.Getpid()))
	if err != nil {
		return false, err
	}
	defer windows.CloseHandle(parent)
	cmd, err := startUninstallHelper(root, m, parent)
	if err != nil {
		return false, err
	}
	go cmd.Wait()
	return true, nil
}

func startUninstallHelper(root string, m *Launchers, parent windows.Handle) (*exec.Cmd, error) {
	systemDir, err := windows.GetSystemDirectory()
	if err != nil {
		return nil, err
	}
	sa := &windows.SecurityAttributes{
		Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), InheritHandle: 1,
	}
	ready, err := windows.CreateEvent(sa, 1, 0, nil)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(ready)
	data := struct {
		Root         string            `json:"root"`
		ManifestHash string            `json:"manifestHash"`
		Files        map[string]string `json:"files"`
		Parent       uint64            `json:"parent"`
		Ready        uint64            `json:"ready"`
	}{Root: root, Files: map[string]string{}, Parent: uint64(parent), Ready: uint64(ready)}
	data.ManifestHash, err = digest(filepath.Join(root, manifestName))
	if err != nil {
		return nil, err
	}
	for _, l := range m.Tools {
		path := filepath.Join(root, "bin", l.Name)
		if _, err := os.Lstat(path); os.IsNotExist(err) {
			continue
		}
		d, err := digest(path)
		if err != nil {
			return nil, err
		}
		data.Files[l.Name] = d
	}
	payload, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	// Encode the fixed script and its JSON data literally, so Unicode paths,
	// quotes, percent signs and shell metacharacters are never evaluated.
	script := "$data = ConvertFrom-Json ([Text.Encoding]::UTF8.GetString([Convert]::FromBase64String('" +
		base64.StdEncoding.EncodeToString(payload) + "')))\n" + deferredUninstallScript
	code := utf16.Encode([]rune(script))
	encoded := make([]byte, len(code)*2)
	for i, c := range code {
		binary.LittleEndian.PutUint16(encoded[2*i:], c)
	}
	cmd := exec.Command(filepath.Join(systemDir, "WindowsPowerShell", "v1.0", "powershell.exe"),
		"-NoLogo", "-NoProfile", "-NonInteractive", "-EncodedCommand", base64.StdEncoding.EncodeToString(encoded))
	cmd.Env = []string{
		"SystemRoot=" + filepath.Dir(systemDir), "PATH=" + systemDir,
		"TEMP=" + os.TempDir(), "TMP=" + os.TempDir(),
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW,
		AdditionalInheritedHandles: []syscall.Handle{syscall.Handle(parent), syscall.Handle(ready)},
	}
	var startupError bytes.Buffer
	cmd.Stderr = &startupError
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("could not start Windows launcher cleanup; installation kept: %w", err)
	}
	state, err := windows.WaitForSingleObject(ready, 15000)
	if err != nil || state != windows.WAIT_OBJECT_0 {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return nil, fmt.Errorf("Windows launcher cleanup did not become ready; installation kept: state %d: %v: %s", state, err, startupError.String())
	}
	return cmd, nil
}

const deferredUninstallScript = `
$ErrorActionPreference = 'Stop'
$parentWait = [Threading.ManualResetEvent]::new($false)
$oldHandle = $parentWait.SafeWaitHandle
$parentWait.SafeWaitHandle = [Microsoft.Win32.SafeHandles.SafeWaitHandle]::new([IntPtr]([long]$data.parent), $true)
$oldHandle.Dispose()
$readyWait = [Threading.ManualResetEvent]::new($false)
$oldHandle = $readyWait.SafeWaitHandle
$readyWait.SafeWaitHandle = [Microsoft.Win32.SafeHandles.SafeWaitHandle]::new([IntPtr]([long]$data.ready), $true)
$oldHandle.Dispose()
$lockStream = $null
function FileHash([string]$path) {
    $stream = [IO.File]::OpenRead($path)
    $sha = [Security.Cryptography.SHA256]::Create()
    try { return [BitConverter]::ToString($sha.ComputeHash($stream)).Replace('-', '').ToLowerInvariant() }
    finally { $sha.Dispose(); $stream.Dispose() }
}
try {
    if ($parentWait.WaitOne(0)) { throw 'aims exited before cleanup became ready' }
    $null = $readyWait.Set()
    if (-not $parentWait.WaitOne(60000)) { throw 'aims did not exit before cleanup deadline' }
    $parentWait.Dispose()
    for ($attempt = 0; $attempt -lt 250; $attempt++) {
        try {
            $lockStream = [IO.File]::Open($data.root + '.lock', [IO.FileMode]::CreateNew, [IO.FileAccess]::Write, [IO.FileShare]::None)
            break
        } catch [IO.IOException] { [Threading.Thread]::Sleep(20) }
    }
    if ($null -eq $lockStream) { throw 'launcher installation is still locked' }
    $bytes = [Text.Encoding]::ASCII.GetBytes([string]$PID)
    $lockStream.Write($bytes, 0, $bytes.Length)
    $lockStream.Flush()
    $bin = [IO.Path]::Combine($data.root, 'bin')
    foreach ($directory in @($data.root, $bin)) {
        $attrs = [IO.File]::GetAttributes($directory)
        if (($attrs -band [IO.FileAttributes]::ReparsePoint) -ne 0) { throw 'launcher directory was redirected' }
    }
    $manifest = [IO.Path]::Combine($data.root, 'manifest.json')
    if ((FileHash $manifest) -ne $data.manifestHash) { throw 'launcher manifest changed; installation kept' }
    $paths = @()
    foreach ($file in $data.files.PSObject.Properties) {
        $path = [IO.Path]::Combine($bin, $file.Name)
        try { $attrs = [IO.File]::GetAttributes($path) }
        catch [IO.FileNotFoundException] { continue }
        catch [IO.DirectoryNotFoundException] { continue }
        if (($attrs -band [IO.FileAttributes]::Directory) -ne 0 -or (FileHash $path) -ne $file.Value) {
            throw 'launcher was changed; installation kept'
        }
        $paths += $path
    }
    foreach ($path in $paths) {
        for ($attempt = 0; ; $attempt++) {
            try { [IO.File]::Delete($path); break }
            catch [IO.IOException], [UnauthorizedAccessException] {
                if ($attempt -ge 99) { throw }
                [Threading.Thread]::Sleep(20)
            }
        }
    }
    [IO.File]::Delete($manifest)
    try { [IO.Directory]::Delete($bin) } catch [IO.IOException] {}
    try { [IO.Directory]::Delete($data.root) } catch [IO.IOException] {}
} finally {
    if ($null -ne $lockStream) {
        $lockStream.Dispose()
        [IO.File]::Delete($data.root + '.lock')
    }
    $readyWait.Dispose()
    $parentWait.Dispose()
}
`
