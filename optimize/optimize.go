package optimize

import (
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/gostdlib/base/context"
	"github.com/gostdlib/base/statemachine"
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
	defers := []statemachine.DeferFn[data]{sm.runTests}

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
						req, err := statemachine.Run(
							"",
							statemachine.Request[data]{
								Data: data{
									path:       path,
									fieldAlign: a.FieldAlign,
								},
								Next:   sm.parsePackage,
								Defers: defers,
							},
						)
						if err != nil || req.Data.testErr != nil {
							cancel()
							if req.Data.testErr != nil {
								err = req.Data.testErr
							}
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

	// testErr is any error encountered while running tests on the package.
	testErr error
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
	fset := token.NewFileSet()

	foundGo := false
	for _, d := range df {
		path := filepath.Join(req.Data.path, d.Name())
		// Skip non-Go files
		if filepath.Ext(path) != ".go" {
			continue
		}
		foundGo = true

		// Parse the file
		node, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			req.Err = err
			return req
		}

		if req.Data.fieldAlign {
			// Check the imports in the file
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
	if !foundGo {
		return req
	}

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

	return req
}

// runTests runs the tests for the package if required. This implements the statemachine.DeferFn[data] signature.
func (p packageSM) runTests(ctx context.Context, data data, err error) data {
	if err != nil || p.testFiles {
		return data
	}
	if strings.Contains(data.path, "vendor") {
		return data
	}

	cmd := exec.Command(p.goExecPath, "test", ".")
	cmd.Dir = data.path
	out, err := cmd.CombinedOutput()
	if err != nil {
		data.testErr = fmt.Errorf("problem running tests: %v\n%s", err, string(out))
	}
	return data
}
