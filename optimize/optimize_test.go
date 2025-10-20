package optimize

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gostdlib/base/statemachine"
)

func TestParsePackage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		setup           func(t *testing.T) string
		fieldAlignInput bool
		wantErr         bool
		wantFieldAlign  bool
		wantNextMethod  string
	}{
		{
			name: "Success: directory with Go files without reflect import",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()

				goMod := filepath.Join(tmpDir, "go.mod")
				modContent := "module test\n\ngo 1.24\n"
				if err := os.WriteFile(goMod, []byte(modContent), 0644); err != nil {
					t.Fatalf("failed to create go.mod: %v", err)
				}

				goFile := filepath.Join(tmpDir, "main.go")
				content := `package main

import "fmt"

func main() {
	fmt.Println("hello")
}
`
				if err := os.WriteFile(goFile, []byte(content), 0644); err != nil {
					t.Fatalf("failed to create Go file: %v", err)
				}
				return tmpDir
			},
			fieldAlignInput: true,
			wantErr:         false,
			wantFieldAlign:  true,
			wantNextMethod:  "github.com/johnsiilver/goptimizer/optimize.packageSM.fieldAlign",
		},
		{
			name: "Success: directory with Go file that imports reflect",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()

				goMod := filepath.Join(tmpDir, "go.mod")
				modContent := "module test\n\ngo 1.24\n"
				if err := os.WriteFile(goMod, []byte(modContent), 0644); err != nil {
					t.Fatalf("failed to create go.mod: %v", err)
				}

				goFile := filepath.Join(tmpDir, "main.go")
				content := `package main

import (
	"fmt"
	"reflect"
)

func main() {
	fmt.Println(reflect.TypeOf(42))
}
`
				if err := os.WriteFile(goFile, []byte(content), 0644); err != nil {
					t.Fatalf("failed to create Go file: %v", err)
				}
				return tmpDir
			},
			fieldAlignInput: true,
			wantErr:         false,
			wantFieldAlign:  false,
			wantNextMethod:  "github.com/johnsiilver/goptimizer/optimize.packageSM.fieldAlign",
		},
		{
			name: "Success: directory with no Go files",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				txtFile := filepath.Join(tmpDir, "readme.txt")
				if err := os.WriteFile(txtFile, []byte("not a go file"), 0644); err != nil {
					t.Fatalf("failed to create text file: %v", err)
				}
				return tmpDir
			},
			fieldAlignInput: true,
			wantErr:         false,
			wantFieldAlign:  true,
			wantNextMethod:  "",
		},
		{
			name: "Success: directory with mixed .go and non-.go files",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()

				goMod := filepath.Join(tmpDir, "go.mod")
				modContent := "module test\n\ngo 1.24\n"
				if err := os.WriteFile(goMod, []byte(modContent), 0644); err != nil {
					t.Fatalf("failed to create go.mod: %v", err)
				}

				goFile := filepath.Join(tmpDir, "code.go")
				goContent := `package main

func hello() string {
	return "world"
}
`
				if err := os.WriteFile(goFile, []byte(goContent), 0644); err != nil {
					t.Fatalf("failed to create Go file: %v", err)
				}

				txtFile := filepath.Join(tmpDir, "readme.txt")
				if err := os.WriteFile(txtFile, []byte("documentation"), 0644); err != nil {
					t.Fatalf("failed to create text file: %v", err)
				}

				return tmpDir
			},
			fieldAlignInput: true,
			wantErr:         false,
			wantFieldAlign:  true,
			wantNextMethod:  "github.com/johnsiilver/goptimizer/optimize.packageSM.fieldAlign",
		},
		{
			name: "Success: multiple Go files, one imports reflect",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()

				goMod := filepath.Join(tmpDir, "go.mod")
				modContent := "module test\n\ngo 1.24\n"
				if err := os.WriteFile(goMod, []byte(modContent), 0644); err != nil {
					t.Fatalf("failed to create go.mod: %v", err)
				}

				file1 := filepath.Join(tmpDir, "file1.go")
				content1 := `package main

import "fmt"

func hello() {
	fmt.Println("hello")
}
`
				if err := os.WriteFile(file1, []byte(content1), 0644); err != nil {
					t.Fatalf("failed to create file1.go: %v", err)
				}

				file2 := filepath.Join(tmpDir, "file2.go")
				content2 := `package main

import "reflect"

func inspect(v interface{}) {
	reflect.TypeOf(v)
}
`
				if err := os.WriteFile(file2, []byte(content2), 0644); err != nil {
					t.Fatalf("failed to create file2.go: %v", err)
				}

				return tmpDir
			},
			fieldAlignInput: true,
			wantErr:         false,
			wantFieldAlign:  false,
			wantNextMethod:  "github.com/johnsiilver/goptimizer/optimize.packageSM.fieldAlign",
		},
		{
			name: "Success: fieldAlign already false",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()

				goMod := filepath.Join(tmpDir, "go.mod")
				modContent := "module test\n\ngo 1.24\n"
				if err := os.WriteFile(goMod, []byte(modContent), 0644); err != nil {
					t.Fatalf("failed to create go.mod: %v", err)
				}

				goFile := filepath.Join(tmpDir, "main.go")
				content := `package main

import "fmt"

func main() {
	fmt.Println("hello")
}
`
				if err := os.WriteFile(goFile, []byte(content), 0644); err != nil {
					t.Fatalf("failed to create Go file: %v", err)
				}
				return tmpDir
			},
			fieldAlignInput: false,
			wantErr:         false,
			wantFieldAlign:  false,
			wantNextMethod:  "github.com/johnsiilver/goptimizer/optimize.packageSM.fieldAlign",
		},
		{
			name: "Error: directory does not exist",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				return filepath.Join(tmpDir, "nonexistent")
			},
			fieldAlignInput: true,
			wantErr:         true,
			wantFieldAlign:  true,
			wantNextMethod:  "",
		},
		{
			name: "Success: invalid Go syntax in file does not cause error in packages.Load",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()

				goMod := filepath.Join(tmpDir, "go.mod")
				modContent := "module test\n\ngo 1.24\n"
				if err := os.WriteFile(goMod, []byte(modContent), 0644); err != nil {
					t.Fatalf("failed to create go.mod: %v", err)
				}

				goFile := filepath.Join(tmpDir, "invalid.go")
				invalidContent := `package main

this is not valid Go syntax!!!
`
				if err := os.WriteFile(goFile, []byte(invalidContent), 0644); err != nil {
					t.Fatalf("failed to create invalid Go file: %v", err)
				}
				return tmpDir
			},
			fieldAlignInput: true,
			wantErr:         false,
			wantFieldAlign:  true,
			wantNextMethod:  "github.com/johnsiilver/goptimizer/optimize.packageSM.fieldAlign",
		},
		{
			name: "Success: empty directory",
			setup: func(t *testing.T) string {
				return t.TempDir()
			},
			fieldAlignInput: true,
			wantErr:         false,
			wantFieldAlign:  true,
			wantNextMethod:  "",
		},
		{
			name: "Success: Go file with reflect in string literal should not affect fieldAlign",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()

				goMod := filepath.Join(tmpDir, "go.mod")
				modContent := "module test\n\ngo 1.24\n"
				if err := os.WriteFile(goMod, []byte(modContent), 0644); err != nil {
					t.Fatalf("failed to create go.mod: %v", err)
				}

				goFile := filepath.Join(tmpDir, "main.go")
				content := `package main

import "fmt"

func main() {
	s := "reflect"
	fmt.Println(s)
}
`
				if err := os.WriteFile(goFile, []byte(content), 0644); err != nil {
					t.Fatalf("failed to create Go file: %v", err)
				}
				return tmpDir
			},
			fieldAlignInput: true,
			wantErr:         false,
			wantFieldAlign:  true,
			wantNextMethod:  "github.com/johnsiilver/goptimizer/optimize.packageSM.fieldAlign",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := test.setup(t)

			sm := packageSM{}
			req := statemachine.Request[data]{
				Data: data{
					path:       path,
					fieldAlign: test.fieldAlignInput,
				},
			}

			got := sm.parsePackage(req)

			switch {
			case got.Err == nil && test.wantErr:
				t.Errorf("TestParsePackage(%s): got err == nil, want err != nil", test.name)
				return
			case got.Err != nil && !test.wantErr:
				t.Errorf("TestParsePackage(%s): got err == %s, want err == nil", test.name, got.Err)
				return
			case got.Err != nil:
				return
			}

			if got.Data.fieldAlign != test.wantFieldAlign {
				t.Errorf("TestParsePackage(%s): got fieldAlign %v, want %v", test.name, got.Data.fieldAlign, test.wantFieldAlign)
			}

			gotNextMethod := statemachine.MethodName(got.Next)
			if gotNextMethod != test.wantNextMethod {
				t.Errorf("TestParsePackage(%s): got Next method %q, want %q", test.name, gotNextMethod, test.wantNextMethod)
			}
		})
	}
}
