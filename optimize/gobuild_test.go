package optimize

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/gostdlib/base/statemachine"
)

func TestGoBuild(t *testing.T) {
	t.Parallel()

	// Get the go executable path once for all tests
	goExecPath, err := exec.LookPath("go")
	if err != nil {
		t.Fatalf("go binary not found on path: %v", err)
	}

	tests := []struct {
		name    string
		setup   func(t *testing.T) string
		wantErr bool
	}{
		{
			name: "Success: valid Go package builds successfully",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()

				goMod := filepath.Join(tmpDir, "go.mod")
				modContent := "module testbuild\n\ngo 1.24\n"
				if err := os.WriteFile(goMod, []byte(modContent), 0644); err != nil {
					t.Fatalf("failed to create go.mod: %v", err)
				}

				goFile := filepath.Join(tmpDir, "main.go")
				content := `package main

import "fmt"

func main() {
	fmt.Println("Hello, World!")
}
`
				if err := os.WriteFile(goFile, []byte(content), 0644); err != nil {
					t.Fatalf("failed to create Go file: %v", err)
				}
				return tmpDir
			},
			wantErr: false,
		},
		{
			name: "Success: library package without main builds successfully",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()

				goMod := filepath.Join(tmpDir, "go.mod")
				modContent := "module testlib\n\ngo 1.24\n"
				if err := os.WriteFile(goMod, []byte(modContent), 0644); err != nil {
					t.Fatalf("failed to create go.mod: %v", err)
				}

				goFile := filepath.Join(tmpDir, "lib.go")
				content := `package testlib

func Add(a, b int) int {
	return a + b
}
`
				if err := os.WriteFile(goFile, []byte(content), 0644); err != nil {
					t.Fatalf("failed to create Go file: %v", err)
				}
				return tmpDir
			},
			wantErr: false,
		},
		{
			name: "Error: package with compilation errors",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()

				goMod := filepath.Join(tmpDir, "go.mod")
				modContent := "module testerror\n\ngo 1.24\n"
				if err := os.WriteFile(goMod, []byte(modContent), 0644); err != nil {
					t.Fatalf("failed to create go.mod: %v", err)
				}

				goFile := filepath.Join(tmpDir, "main.go")
				content := `package main

func main() {
	undeclaredVariable = 42
	fmt.Println("This will fail")
}
`
				if err := os.WriteFile(goFile, []byte(content), 0644); err != nil {
					t.Fatalf("failed to create Go file: %v", err)
				}
				return tmpDir
			},
			wantErr: true,
		},
		{
			name: "Error: package with syntax errors",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()

				goMod := filepath.Join(tmpDir, "go.mod")
				modContent := "module testsyntax\n\ngo 1.24\n"
				if err := os.WriteFile(goMod, []byte(modContent), 0644); err != nil {
					t.Fatalf("failed to create go.mod: %v", err)
				}

				goFile := filepath.Join(tmpDir, "main.go")
				content := `package main

func main( {
	println("missing closing paren")
}
`
				if err := os.WriteFile(goFile, []byte(content), 0644); err != nil {
					t.Fatalf("failed to create Go file: %v", err)
				}
				return tmpDir
			},
			wantErr: true,
		},
		{
			name: "Error: package with missing import",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()

				goMod := filepath.Join(tmpDir, "go.mod")
				modContent := "module testimport\n\ngo 1.24\n"
				if err := os.WriteFile(goMod, []byte(modContent), 0644); err != nil {
					t.Fatalf("failed to create go.mod: %v", err)
				}

				goFile := filepath.Join(tmpDir, "main.go")
				content := `package main

func main() {
	fmt.Println("fmt is not imported")
}
`
				if err := os.WriteFile(goFile, []byte(content), 0644); err != nil {
					t.Fatalf("failed to create Go file: %v", err)
				}
				return tmpDir
			},
			wantErr: true,
		},
		{
			name: "Success: package with multiple files",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()

				goMod := filepath.Join(tmpDir, "go.mod")
				modContent := "module testmulti\n\ngo 1.24\n"
				if err := os.WriteFile(goMod, []byte(modContent), 0644); err != nil {
					t.Fatalf("failed to create go.mod: %v", err)
				}

				mainFile := filepath.Join(tmpDir, "main.go")
				mainContent := `package main

func main() {
	println(GetMessage())
}
`
				if err := os.WriteFile(mainFile, []byte(mainContent), 0644); err != nil {
					t.Fatalf("failed to create main.go: %v", err)
				}

				utilFile := filepath.Join(tmpDir, "util.go")
				utilContent := `package main

func GetMessage() string {
	return "Hello from util"
}
`
				if err := os.WriteFile(utilFile, []byte(utilContent), 0644); err != nil {
					t.Fatalf("failed to create util.go: %v", err)
				}

				return tmpDir
			},
			wantErr: false,
		},
		{
			name: "Success: empty package builds successfully",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()

				goMod := filepath.Join(tmpDir, "go.mod")
				modContent := "module testempty\n\ngo 1.24\n"
				if err := os.WriteFile(goMod, []byte(modContent), 0644); err != nil {
					t.Fatalf("failed to create go.mod: %v", err)
				}

				goFile := filepath.Join(tmpDir, "empty.go")
				content := `package testempty

// Empty package
`
				if err := os.WriteFile(goFile, []byte(content), 0644); err != nil {
					t.Fatalf("failed to create Go file: %v", err)
				}
				return tmpDir
			},
			wantErr: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := test.setup(t)

			sm := packageSM{
				goExecPath: goExecPath,
			}

			req := statemachine.Request[data]{
				Data: data{
					path: path,
				},
			}

			got := sm.goBuild(req)

			switch {
			case got.Err == nil && test.wantErr:
				t.Errorf("TestGoBuild(%s): got err == nil, want err != nil", test.name)
				return
			case got.Err != nil && !test.wantErr:
				t.Errorf("TestGoBuild(%s): got err == %s, want err == nil", test.name, got.Err)
				return
			case got.Err != nil:
				return
			}
		})
	}
}
