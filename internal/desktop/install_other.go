//go:build !darwin

package desktop

import (
	"context"
	"fmt"
	"io"
)

func Install(context.Context, string, io.Reader, io.Writer, io.Writer) error {
	return fmt.Errorf("Aims Usage requires macOS")
}
func Open(context.Context) error     { return fmt.Errorf("Aims Usage requires macOS") }
func LinkCLI(string) (string, error) { return "", fmt.Errorf("Aims Usage requires macOS") }

func binaryPlistXML([]byte) ([]byte, error) {
	return nil, fmt.Errorf("binary app metadata requires macOS")
}
