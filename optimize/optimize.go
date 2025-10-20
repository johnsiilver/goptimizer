package optimize

import (
	"fmt"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/gostdlib/base/context"
	"github.com/gostdlib/base/statemachine"
	"golang.org/x/tools/go/packages"
)

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
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax,
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
	//req.Next = p.interfaceReplacement

	if strings.Contains(req.Data.path, "vendor") {
		return req
	}

	cmd := exec.Command(p.goExecPath, "test", ".")
	cmd.Dir = req.Data.path
	out, err := cmd.CombinedOutput()
	if err != nil {
		req.Err = fmt.Errorf("problem running tests: %v\n%s", err, string(out))
	}
	return req
}

/*
// interfaceReplacement performs interface replacement on the package if required.
func (p packageSM) interfaceReplacement(req statemachine.Request[data]) statemachine.Request[data] {
	if !req.Data.interfaceReplacement {
		return req
	}
}
*/
