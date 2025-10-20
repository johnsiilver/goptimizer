package optimize

import (
	"fmt"
	"go/ast"
	"go/format"
	"go/token"
	"go/types"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/gostdlib/base/context"
	"github.com/gostdlib/base/statemachine"
	"golang.org/x/tools/go/ast/astutil"
	"golang.org/x/tools/go/packages"
)

const replaceInterfaceComment = "//replaceInterface:"

// Args holds the arguments for the Packages function.
type Args struct {
	// Root is the root directory to start searching for Go packages.
	Root string
	// GoExecPath is the path to the Go executable.
	GoExecPath string
	// AlignPath is the path to the betteralign binary.
	AlignPath string
	// FieldAlign indicates whether to perform field alignment.
	FieldAlign bool
	// InterfaceReplacement indicates whether to perform interface replacement.
	InterfaceReplacement bool
	// Generated indicates whether to process generated files.
	Generated bool
	// TestFiles indicates whether to run tests on packages with test files.
	TestFiles bool
}

// Packages optimizes all Go packages found under the specified root directory.
func Packages(ctx context.Context, a Args) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	wg := context.Pool(ctx).Limited(50).Group()

	sm := packageSM{
		alignPath:      a.AlignPath,
		goExecPath:     a.GoExecPath,
		generatedFiles: a.Generated,
		testFiles:      a.TestFiles,
	}

	wdErr := filepath.WalkDir(
		a.Root,
		func(path string, d os.DirEntry, err error) error {
			switch {
			case err != nil:
				return err
			case d.IsDir() && strings.HasPrefix(d.Name(), "."):
				// Skip this directory and all of its contents
				return filepath.SkipDir
			case d.IsDir():
				_ = wg.Go(
					ctx,
					func(ctx context.Context) error {
						_, err := statemachine.Run(
							"",
							statemachine.Request[data]{
								Data: data{
									path:                 path,
									fieldAlign:           a.FieldAlign,
									interfaceReplacement: a.InterfaceReplacement,
								},
								Next: sm.parsePackage,
							},
						)
						if err != nil {
							cancel()
						}
						return err
					},
				)
			}
			return nil
		},
	)
	if wdErr != nil {
		return wdErr
	}

	return wg.Wait(ctx)
}

// data holds the data being passed through the state machine.
type data struct {
	// path to the package being processed
	path string
	// fieldAlign indicates whether we should do field alignment.
	fieldAlign bool
	// interfaceReplacement indicates whether we should do interface replacement.
	interfaceReplacement bool

	pkg *packages.Package
}

// packageSM is the state machine for processing a package.
type packageSM struct {
	// goExecPath is the path to the Go executable.
	goExecPath string
	// alignPath is the path to the betteralign binary.
	alignPath string

	// generatedFiles indicates whether to process generated files.
	generatedFiles bool
	// testFiles indicates whether to process test files.
	testFiles bool
}

// parsePackage parses the package and determines if it should be optimized.
func (p packageSM) parsePackage(req statemachine.Request[data]) statemachine.Request[data] {
	df, err := os.ReadDir(req.Data.path)
	if err != nil {
		req.Err = err
		return req
	}

	foundGo := false
	for _, d := range df {
		path := filepath.Join(req.Data.path, d.Name())
		// Skip non-Go files
		if filepath.Ext(path) != ".go" {
			continue
		}
		foundGo = true
	}
	if !foundGo {
		return req
	}

	cfg := &packages.Config{
		Dir:  req.Data.path,
		Fset: token.NewFileSet(),
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo,
	}
	pkgs, err := packages.Load(cfg, ".")
	if err != nil {
		req.Err = fmt.Errorf("failed to parse pacakge %q", req.Data.path)
		return req
	}
	if len(pkgs) == 0 {
		req.Err = fmt.Errorf("no packages found in %q", req.Data.path)
		return req
	}
	if len(pkgs) > 1 {
		req.Err = fmt.Errorf("multiple packages found in %q", req.Data.path)
		return req
	}

	pkg := pkgs[0]

	if req.Data.fieldAlign {
		for _, node := range pkg.Syntax {
			for _, imp := range node.Imports {
				// The path value includes quotes, so we need to trim them
				importPath := imp.Path.Value[1 : len(imp.Path.Value)-1]
				if importPath == "reflect" {
					req.Data.fieldAlign = false
					break
				}
			}
		}
	}
	req.Data.pkg = pkg

	req.Next = p.fieldAlign
	return req
}

// fieldAlign performs field alignment on the package if required.
func (p packageSM) fieldAlign(req statemachine.Request[data]) statemachine.Request[data] {
	if !req.Data.fieldAlign {
		return req
	}

	args := []string{"-apply"}
	if p.generatedFiles {
		args = append(args, "-generated_files")
	}
	if p.testFiles {
		args = append(args, "-test_files")
	}
	args = append(args, ".")

	// betteralign recommends running twice to ensure optimal alignment.
	for i := 0; i < 2; i++ {
		cmd := exec.Command(p.alignPath, args...)
		cmd.Path = req.Data.path
		_, err := exec.Command(p.alignPath, args...).CombinedOutput()
		if err != nil {
			req.Err = err
			return req
		}
	}
	req.Next = p.runTests

	return req
}

// runTests runs tests on the package if required.
func (p packageSM) runTests(req statemachine.Request[data]) statemachine.Request[data] {
	if p.testFiles {
		return req
	}

	if strings.Contains(req.Data.path, "vendor") {
		return req
	}

	cmd := exec.Command(p.goExecPath, "test", ".")
	cmd.Dir = req.Data.path
	out, err := cmd.CombinedOutput()
	if err != nil {
		req.Err = fmt.Errorf("problem running tests: %v\n%s", err, string(out))
	}
	req.Next = p.interfaceReplacement
	return req
}

// interfaceReplacement performs interface replacement on the package if required.
func (p packageSM) interfaceReplacement(req statemachine.Request[data]) statemachine.Request[data] {
	if !req.Data.interfaceReplacement {
		return req
	}

	if req.Data.pkg == nil {
		req.Err = fmt.Errorf("package not loaded")
		return req
	}

	pkg := req.Data.pkg
	modified := false

	// Iterate through all files in the package
	for _, file := range pkg.Syntax {
		fileModified := false

		// Walk the AST to find struct types
		ast.Inspect(file, func(n ast.Node) bool {
			// Look for struct types
			structType, ok := n.(*ast.StructType)
			if !ok {
				return true
			}

			// Check each field in the struct
			for _, field := range structType.Fields.List {
				replaceWith := extractReplaceComment(field)
				if replaceWith == "" {
					continue
				}

				if !isInterface(field, pkg.TypesInfo) {
					pos := pkg.Fset.Position(field.Pos())
					req.Err = fmt.Errorf("%s: field is not an interface type, cannot replace", pos)
					return false
				}

				replaceInfo, err := parseReplacement(replaceWith, file)
				if err != nil {
					pos := pkg.Fset.Position(field.Pos())
					req.Err = fmt.Errorf("%s: %w", pos, err)
					return false
				}

				if replaceInfo.importPath != "" {
					astutil.AddImport(pkg.Fset, file, replaceInfo.importPath)
				}

				field.Type = replaceInfo.expr
				fileModified = true
				modified = true
			}

			return true
		})

		if req.Err != nil {
			return req
		}

		if fileModified {
			filename := pkg.Fset.File(file.Pos()).Name()
			if err := writeFile(filename, pkg.Fset, file); err != nil {
				req.Err = fmt.Errorf("failed to write file %s: %w", filename, err)
				return req
			}
		}
	}

	if modified {
		// Reload package to get updated type information
		cfg := &packages.Config{
			Dir:  req.Data.path,
			Fset: token.NewFileSet(),
			Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo,
		}
		pkgs, err := packages.Load(cfg, ".")
		if err != nil {
			req.Err = fmt.Errorf("failed to reload package after interface replacement: %w", err)
			return req
		}
		if len(pkgs) > 0 {
			req.Data.pkg = pkgs[0]
		}
	}

	req.Next = p.goBuild
	return req
}

// goBuild runs 'go build' on the package.
func (p packageSM) goBuild(req statemachine.Request[data]) statemachine.Request[data] {
	cmd := exec.Command(p.goExecPath, "build")
	cmd.Dir = req.Data.path
	out, err := cmd.CombinedOutput()
	if err != nil {
		req.Err = fmt.Errorf("problem running go build: %v\n%s", err, string(out))
	}
	return req
}

// extractReplaceComment checks if a field has a replaceInterface comment and returns the replacement type.
// It checks both Doc (above field) and Comment (inline) comment groups.
func extractReplaceComment(field *ast.Field) string {
	if field.Doc != nil {
		for _, comment := range field.Doc.List {
			if strings.HasPrefix(strings.TrimSpace(comment.Text), replaceInterfaceComment) {
				return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(comment.Text), replaceInterfaceComment))
			}
		}
	}

	if field.Comment != nil {
		for _, comment := range field.Comment.List {
			if strings.HasPrefix(strings.TrimSpace(comment.Text), replaceInterfaceComment) {
				return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(comment.Text), replaceInterfaceComment))
			}
		}
	}

	return ""
}

// isInterface checks if the given field is of interface type.
func isInterface(field *ast.Field, info *types.Info) bool {
	if info == nil || info.Types == nil {
		return false
	}

	typeAndValue, ok := info.Types[field.Type]
	if !ok {
		return false
	}

	_, isInterface := typeAndValue.Type.Underlying().(*types.Interface)
	return isInterface
}

// replacementInfo holds information about a parsed replacement type.
type replacementInfo struct {
	// expr is the AST expression representing the type
	expr ast.Expr
	// importPath is the import path to add (empty if not needed)
	importPath string
	// importName is the package name/alias (empty if not needed)
	importName string
}

// parseReplacement parses the replacement type string and creates the appropriate AST node.
// Handles: "Type", "*Type", "pkg.Type", "*pkg.Type", "path/to/pkg.Type", "*path/to/pkg.Type"
func parseReplacement(replaceWith string, file *ast.File) (*replacementInfo, error) {
	replaceWith = strings.TrimSpace(replaceWith)
	if replaceWith == "" {
		return nil, fmt.Errorf("replacement type is empty")
	}

	isPointer := strings.HasPrefix(replaceWith, "*")
	if isPointer {
		replaceWith = strings.TrimPrefix(replaceWith, "*")
		replaceWith = strings.TrimSpace(replaceWith)
	}

	if strings.Contains(replaceWith, "/") {
		// Format: "path/to/package.TypeName" or "path/to/package".TypeName
		lastSlash := strings.LastIndex(replaceWith, "/")
		dotIndex := strings.Index(replaceWith[lastSlash:], ".")

		if dotIndex == -1 {
			return nil, fmt.Errorf("invalid import path format: %q, expected path/to/package.TypeName", replaceWith)
		}

		dotIndex += lastSlash
		importPath := replaceWith[:dotIndex]
		typeName := replaceWith[dotIndex+1:]

		pkgName := importPath[strings.LastIndex(importPath, "/")+1:]

		sel := &ast.SelectorExpr{
			X:   &ast.Ident{Name: pkgName},
			Sel: &ast.Ident{Name: typeName},
		}

		var expr ast.Expr = sel
		if isPointer {
			expr = &ast.StarExpr{X: sel}
		}

		return &replacementInfo{
			expr:       expr,
			importPath: importPath,
			importName: pkgName,
		}, nil
	}

	if strings.Contains(replaceWith, ".") {
		parts := strings.SplitN(replaceWith, ".", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid qualified type: %q", replaceWith)
		}

		pkgName := parts[0]
		typeName := parts[1]

		// Check if the import exists in the file
		importPath := findImportPath(file, pkgName)

		// If not found, assume it's a standard library or external package that needs to be imported
		// For standard library packages, the package name is the import path
		if importPath == "" {
			importPath = pkgName
		}

		sel := &ast.SelectorExpr{
			X:   &ast.Ident{Name: pkgName},
			Sel: &ast.Ident{Name: typeName},
		}

		var expr ast.Expr = sel
		if isPointer {
			expr = &ast.StarExpr{X: sel}
		}

		// If the import wasn't found, we need to add it
		needsImport := findImportPath(file, pkgName) == ""

		return &replacementInfo{
			expr: expr,
			importPath: func() string {
				if needsImport {
					return importPath
				}
				return ""
			}(),
			importName: pkgName,
		}, nil
	}

	ident := &ast.Ident{Name: replaceWith}
	var expr ast.Expr = ident
	if isPointer {
		expr = &ast.StarExpr{X: ident}
	}

	return &replacementInfo{
		expr:       expr,
		importPath: "",
		importName: "",
	}, nil
}

// findImportPath finds the import path for a given package name/alias in a file.
func findImportPath(file *ast.File, pkgName string) string {
	for _, imp := range file.Imports {
		importPath := strings.Trim(imp.Path.Value, `"`)

		if imp.Name != nil {
			if imp.Name.Name == pkgName {
				return importPath
			}
		} else {
			lastSlash := strings.LastIndex(importPath, "/")
			actualPkgName := importPath
			if lastSlash >= 0 {
				actualPkgName = importPath[lastSlash+1:]
			}
			if actualPkgName == pkgName {
				return importPath
			}
		}
	}
	return ""
}

// writeFile formats and writes an AST file to disk.
func writeFile(filename string, fset *token.FileSet, file *ast.File) error {
	info, err := os.Stat(filename)
	if err != nil {
		return err
	}

	tmpFile, err := os.CreateTemp(filepath.Dir(filename), ".tmp-"+filepath.Base(filename))
	if err != nil {
		return err
	}
	tmpName := tmpFile.Name()
	defer os.Remove(tmpName)

	if err := format.Node(tmpFile, fset, file); err != nil {
		tmpFile.Close()
		return err
	}

	if err := tmpFile.Close(); err != nil {
		return err
	}

	if err := os.Rename(tmpName, filename); err != nil {
		return err
	}

	if err := os.Chmod(filename, info.Mode()); err != nil {
		return err
	}

	return nil
}
