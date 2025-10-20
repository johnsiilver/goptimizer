package optimize

import (
	"go/ast"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gostdlib/base/statemachine"
	"golang.org/x/tools/go/packages"
)

func TestExtractReplaceComment(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		setup func(t *testing.T) *ast.Field
		want  string
	}{
		{
			name: "Success: doc comment above field",
			setup: func(t *testing.T) *ast.Field {
				return &ast.Field{
					Doc: &ast.CommentGroup{
						List: []*ast.Comment{
							{Text: "//replaceInterface: *bytes.Buffer"},
						},
					},
					Names: []*ast.Ident{{Name: "Reader"}},
					Type:  &ast.Ident{Name: "io.Reader"},
				}
			},
			want: "*bytes.Buffer",
		},
		{
			name: "Success: inline comment",
			setup: func(t *testing.T) *ast.Field {
				return &ast.Field{
					Comment: &ast.CommentGroup{
						List: []*ast.Comment{
							{Text: "//replaceInterface: string"},
						},
					},
					Names: []*ast.Ident{{Name: "Data"}},
					Type:  &ast.Ident{Name: "Interface"},
				}
			},
			want: "string",
		},
		{
			name: "Success: no comment",
			setup: func(t *testing.T) *ast.Field {
				return &ast.Field{
					Names: []*ast.Ident{{Name: "Field"}},
					Type:  &ast.Ident{Name: "int"},
				}
			},
			want: "",
		},
		{
			name: "Success: comment with extra whitespace",
			setup: func(t *testing.T) *ast.Field {
				return &ast.Field{
					Doc: &ast.CommentGroup{
						List: []*ast.Comment{
							{Text: "  //replaceInterface:   *bytes.Buffer  "},
						},
					},
					Names: []*ast.Ident{{Name: "Reader"}},
					Type:  &ast.Ident{Name: "io.Reader"},
				}
			},
			want: "*bytes.Buffer",
		},
		{
			name: "Success: full import path",
			setup: func(t *testing.T) *ast.Field {
				return &ast.Field{
					Doc: &ast.CommentGroup{
						List: []*ast.Comment{
							{Text: "//replaceInterface: github.com/foo/bar.CustomReader"},
						},
					},
					Names: []*ast.Ident{{Name: "Reader"}},
					Type:  &ast.Ident{Name: "io.Reader"},
				}
			},
			want: "github.com/foo/bar.CustomReader",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			field := test.setup(t)
			got := extractReplaceComment(field)

			if got != test.want {
				t.Errorf("TestExtractReplaceComment(%s): got %q, want %q", test.name, got, test.want)
			}
		})
	}
}

func TestParseReplacementType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		replaceWith    string
		setupFile      func(t *testing.T) *ast.File
		wantErr        bool
		wantImportPath string
		checkExpr      func(t *testing.T, expr ast.Expr)
	}{
		{
			name:        "Success: simple local type",
			replaceWith: "MyType",
			setupFile: func(t *testing.T) *ast.File {
				return &ast.File{Name: &ast.Ident{Name: "main"}}
			},
			wantErr:        false,
			wantImportPath: "",
			checkExpr: func(t *testing.T, expr ast.Expr) {
				ident, ok := expr.(*ast.Ident)
				if !ok {
					t.Errorf("expected *ast.Ident, got %T", expr)
					return
				}
				if ident.Name != "MyType" {
					t.Errorf("got name %q, want %q", ident.Name, "MyType")
				}
			},
		},
		{
			name:        "Success: pointer to local type",
			replaceWith: "*MyType",
			setupFile: func(t *testing.T) *ast.File {
				return &ast.File{Name: &ast.Ident{Name: "main"}}
			},
			wantErr:        false,
			wantImportPath: "",
			checkExpr: func(t *testing.T, expr ast.Expr) {
				star, ok := expr.(*ast.StarExpr)
				if !ok {
					t.Errorf("expected *ast.StarExpr, got %T", expr)
					return
				}
				ident, ok := star.X.(*ast.Ident)
				if !ok {
					t.Errorf("expected *ast.Ident in StarExpr, got %T", star.X)
					return
				}
				if ident.Name != "MyType" {
					t.Errorf("got name %q, want %q", ident.Name, "MyType")
				}
			},
		},
		{
			name:        "Success: qualified type from existing import",
			replaceWith: "bytes.Buffer",
			setupFile: func(t *testing.T) *ast.File {
				return &ast.File{
					Name: &ast.Ident{Name: "main"},
					Imports: []*ast.ImportSpec{
						{Path: &ast.BasicLit{Value: `"bytes"`}},
					},
				}
			},
			wantErr:        false,
			wantImportPath: "",
			checkExpr: func(t *testing.T, expr ast.Expr) {
				sel, ok := expr.(*ast.SelectorExpr)
				if !ok {
					t.Errorf("expected *ast.SelectorExpr, got %T", expr)
					return
				}
				x, ok := sel.X.(*ast.Ident)
				if !ok || x.Name != "bytes" {
					t.Errorf("got package %v, want bytes", x)
				}
				if sel.Sel.Name != "Buffer" {
					t.Errorf("got type %q, want Buffer", sel.Sel.Name)
				}
			},
		},
		{
			name:        "Success: full import path with type",
			replaceWith: "github.com/foo/bar.CustomType",
			setupFile: func(t *testing.T) *ast.File {
				return &ast.File{Name: &ast.Ident{Name: "main"}}
			},
			wantErr:        false,
			wantImportPath: "github.com/foo/bar",
			checkExpr: func(t *testing.T, expr ast.Expr) {
				sel, ok := expr.(*ast.SelectorExpr)
				if !ok {
					t.Errorf("expected *ast.SelectorExpr, got %T", expr)
					return
				}
				x, ok := sel.X.(*ast.Ident)
				if !ok || x.Name != "bar" {
					t.Errorf("got package %v, want bar", x)
				}
				if sel.Sel.Name != "CustomType" {
					t.Errorf("got type %q, want CustomType", sel.Sel.Name)
				}
			},
		},
		{
			name:        "Error: empty replacement type",
			replaceWith: "",
			setupFile: func(t *testing.T) *ast.File {
				return &ast.File{Name: &ast.Ident{Name: "main"}}
			},
			wantErr: true,
		},
		{
			name:        "Success: qualified type without import (standard library)",
			replaceWith: "bytes.Buffer",
			setupFile: func(t *testing.T) *ast.File {
				return &ast.File{Name: &ast.Ident{Name: "main"}}
			},
			wantErr:        false,
			wantImportPath: "bytes",
			checkExpr: func(t *testing.T, expr ast.Expr) {
				sel, ok := expr.(*ast.SelectorExpr)
				if !ok {
					t.Errorf("expected *ast.SelectorExpr, got %T", expr)
					return
				}
				x, ok := sel.X.(*ast.Ident)
				if !ok || x.Name != "bytes" {
					t.Errorf("got package %v, want bytes", x)
				}
				if sel.Sel.Name != "Buffer" {
					t.Errorf("got type %q, want Buffer", sel.Sel.Name)
				}
			},
		},
		{
			name:        "Error: invalid import path format",
			replaceWith: "github.com/foo/bar",
			setupFile: func(t *testing.T) *ast.File {
				return &ast.File{Name: &ast.Ident{Name: "main"}}
			},
			wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file := test.setupFile(t)
			got, err := parseReplacement(test.replaceWith, file)

			switch {
			case err == nil && test.wantErr:
				t.Errorf("TestParseReplacementType(%s): got err == nil, want err != nil", test.name)
				return
			case err != nil && !test.wantErr:
				t.Errorf("TestParseReplacementType(%s): got err == %s, want err == nil", test.name, err)
				return
			case err != nil:
				return
			}

			if got.importPath != test.wantImportPath {
				t.Errorf("TestParseReplacementType(%s): got importPath %q, want %q", test.name, got.importPath, test.wantImportPath)
			}

			if test.checkExpr != nil {
				test.checkExpr(t, got.expr)
			}
		})
	}
}

func TestInterfaceReplacement(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		setup          func(t *testing.T) string
		interfaceRepl  bool
		wantErr        bool
		wantNextMethod string
		validateFile   func(t *testing.T, path string)
	}{
		{
			name: "Success: replace io.Reader with *bytes.Buffer",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()

				goMod := filepath.Join(tmpDir, "go.mod")
				modContent := "module test\n\ngo 1.24\n"
				if err := os.WriteFile(goMod, []byte(modContent), 0644); err != nil {
					t.Fatalf("failed to create go.mod: %v", err)
				}

				goFile := filepath.Join(tmpDir, "main.go")
				content := `package main

import "io"

type Config struct {
	//replaceInterface: *bytes.Buffer
	Reader io.Reader
}

func main() {}
`
				if err := os.WriteFile(goFile, []byte(content), 0644); err != nil {
					t.Fatalf("failed to create Go file: %v", err)
				}
				return tmpDir
			},
			interfaceRepl:  true,
			wantErr:        false,
			wantNextMethod: "github.com/johnsiilver/goptimizer/optimize.packageSM.goBuild",
			validateFile: func(t *testing.T, path string) {
				content, err := os.ReadFile(filepath.Join(path, "main.go"))
				if err != nil {
					t.Fatalf("failed to read file: %v", err)
				}
				contentStr := string(content)
				if !strings.Contains(contentStr, "*bytes.Buffer") {
					t.Errorf("expected file to contain *bytes.Buffer")
				}
				if strings.Contains(contentStr, "io.Reader") {
					t.Errorf("expected io.Reader to be replaced")
				}
				if !strings.Contains(contentStr, `"bytes"`) {
					t.Errorf("expected bytes import to be added")
				}
			},
		},
		{
			name: "Success: replace with local type",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()

				goMod := filepath.Join(tmpDir, "go.mod")
				modContent := "module test\n\ngo 1.24\n"
				if err := os.WriteFile(goMod, []byte(modContent), 0644); err != nil {
					t.Fatalf("failed to create go.mod: %v", err)
				}

				goFile := filepath.Join(tmpDir, "main.go")
				content := `package main

import "io"

type MyReader struct{}

type Config struct {
	//replaceInterface: *MyReader
	Reader io.Reader
}

func main() {}
`
				if err := os.WriteFile(goFile, []byte(content), 0644); err != nil {
					t.Fatalf("failed to create Go file: %v", err)
				}
				return tmpDir
			},
			interfaceRepl:  true,
			wantErr:        false,
			wantNextMethod: "github.com/johnsiilver/goptimizer/optimize.packageSM.goBuild",
			validateFile: func(t *testing.T, path string) {
				content, err := os.ReadFile(filepath.Join(path, "main.go"))
				if err != nil {
					t.Fatalf("failed to read file: %v", err)
				}
				contentStr := string(content)
				if !strings.Contains(contentStr, "*MyReader") {
					t.Errorf("expected file to contain *MyReader")
				}
				if strings.Contains(contentStr, "io.Reader") {
					t.Errorf("expected io.Reader to be replaced")
				}
			},
		},
		{
			name: "Success: interface replacement disabled",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()

				goMod := filepath.Join(tmpDir, "go.mod")
				modContent := "module test\n\ngo 1.24\n"
				if err := os.WriteFile(goMod, []byte(modContent), 0644); err != nil {
					t.Fatalf("failed to create go.mod: %v", err)
				}

				goFile := filepath.Join(tmpDir, "main.go")
				content := `package main

import "io"

type Config struct {
	//replaceInterface: string
	Reader io.Reader
}

func main() {}
`
				if err := os.WriteFile(goFile, []byte(content), 0644); err != nil {
					t.Fatalf("failed to create Go file: %v", err)
				}
				return tmpDir
			},
			interfaceRepl:  false,
			wantErr:        false,
			wantNextMethod: "",
			validateFile: func(t *testing.T, path string) {
				content, err := os.ReadFile(filepath.Join(path, "main.go"))
				if err != nil {
					t.Fatalf("failed to read file: %v", err)
				}
				contentStr := string(content)
				// Should not be replaced
				if !strings.Contains(contentStr, "io.Reader") {
					t.Errorf("expected io.Reader to remain unchanged")
				}
			},
		},
		{
			name: "Error: non-interface field",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()

				goMod := filepath.Join(tmpDir, "go.mod")
				modContent := "module test\n\ngo 1.24\n"
				if err := os.WriteFile(goMod, []byte(modContent), 0644); err != nil {
					t.Fatalf("failed to create go.mod: %v", err)
				}

				goFile := filepath.Join(tmpDir, "main.go")
				content := `package main

type Config struct {
	//replaceInterface: int
	Count string
}

func main() {}
`
				if err := os.WriteFile(goFile, []byte(content), 0644); err != nil {
					t.Fatalf("failed to create Go file: %v", err)
				}
				return tmpDir
			},
			interfaceRepl:  true,
			wantErr:        true,
			wantNextMethod: "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := test.setup(t)

			// Load package
			cfg := &packages.Config{
				Dir:  path,
				Fset: token.NewFileSet(),
				Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo,
			}
			pkgs, err := packages.Load(cfg, ".")
			if err != nil || len(pkgs) == 0 {
				t.Fatalf("failed to load package: %v", err)
			}

			sm := packageSM{}
			req := statemachine.Request[data]{
				Data: data{
					path:                 path,
					interfaceReplacement: test.interfaceRepl,
					pkg:                  pkgs[0],
				},
			}

			got := sm.interfaceReplacement(req)

			switch {
			case got.Err == nil && test.wantErr:
				t.Errorf("TestInterfaceReplacement(%s): got err == nil, want err != nil", test.name)
				return
			case got.Err != nil && !test.wantErr:
				t.Errorf("TestInterfaceReplacement(%s): got err == %s, want err == nil", test.name, got.Err)
				return
			case got.Err != nil:
				return
			}

			gotNextMethod := statemachine.MethodName(got.Next)
			if gotNextMethod != test.wantNextMethod {
				t.Errorf("TestInterfaceReplacement(%s): got Next method %q, want %q", test.name, gotNextMethod, test.wantNextMethod)
			}

			if test.validateFile != nil {
				test.validateFile(t, path)
			}
		})
	}
}
