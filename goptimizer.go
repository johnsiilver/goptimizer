package main

import (
	"context"
	"flag"
	"fmt"
	"go/parser"
	"go/token"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/gostdlib/concurrency/goroutines/pooled"
	"github.com/gostdlib/concurrency/prim/wait"
)

var helpText = `
goptimizer is a wrapper around betteralign that aligns Go source files in a Go project and
the go command line tool to compile a project.

You simply call goptimizer instead of go and it will make a copy of the source files in
a temporary directory, align them with betteralign and then call the go command to create
the binary. The binary is put in the current directory.

The temporary directory is removed after the binary is created.

Usage:
  goptimizer [flags]

Flags:
  -generated bool
    	Field align generated files (default true)
  -testFiles bool
    	Field align test files (default true)
  -goflags array
        Additional flags to pass to the go command. Can be specified multiple times.
     	Does not require quotes around the flag as normally done. Aka 'go build --ldflags="-s -w"'
       	becomes 'goptimizer --goflags="--ldflags=-s -w"'
`

var (
	help           = flag.Bool("help", false, "Show help")
	generatedFiles = flag.Bool("generated", false, "Field align generated files")
	testFiles      = flag.Bool("testFiles", true, "Field align test files")
	runTests       = flag.Bool("runTests", false, "Will run tests before building the binary")
	goflags        stringArray
)

var (
	goExecPath, alignPath string
)

func init() {
	var err error
	goExecPath, err = exec.LookPath("go")
	if err != nil {
		fmt.Println("go binary not found on path")
		os.Exit(1)
	}

	alignPath, err = exec.LookPath("betteralign")
	if err != nil {
		fmt.Println("betteralign binary not found on path")
		os.Exit(1)
	}
}

// stringArray is a custom flag type that implements flag.Value to collect multiple strings
type stringArray []string

// String returns the string representation of the flag value (required by flag.Value interface)
func (s *stringArray) String() string {
	return strings.Join(*s, ",")
}

// Set appends the given value to the StringArray (required by flag.Value interface)
func (s *stringArray) Set(value string) error {
	*s = append(*s, value)
	return nil
}

// findGoMod returns the path to the go.mod file in the current directory.
func findGoMod() (string, error) {
	b, err := exec.Command(goExecPath, "env", "GOMOD").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to run go env GOMOD: %v", err)
	}

	modPath := strings.TrimSpace(string(b))
	switch modPath {
	case "":
		return "", fmt.Errorf("go mod not found")
	case "/dev/null":
		return "", fmt.Errorf("go mod not found")
	}

	return modPath, nil
}

// copyFiles copies all directories and files recursively from srcPath to dstPath,
// but only if a directory contains at least one .go file.
func copyFiles(srcPath, dstPath string) error {
	return filepath.WalkDir(
		srcPath,
		func(path string, d os.DirEntry, err error) error {
			switch {
			case path == srcPath:
				return nil
			case d.IsDir() && strings.HasPrefix(d.Name(), "."):
				// Skip this directory and all of its contents
				return filepath.SkipDir
			case err != nil:
				return err
			}
			if path == srcPath {
				return nil
			}
			if err != nil {
				return err
			}

			// Calculate the destination path
			relPath, err := filepath.Rel(srcPath, path)
			if err != nil {
				return err
			}
			dest := filepath.Join(dstPath, relPath)

			// Check if the current path is a directory
			if d.IsDir() {
				if err := os.MkdirAll(dest, 0750); err != nil {
					return err
				}
				return nil
			}

			fi, err := d.Info()
			if err != nil {
			}
			if err := copyFile(path, dest, fi.Mode()); err != nil {
			}
			return nil
		},
	)
}

func shouldOptimize(dir string) (bool, error) {
	df, err := os.ReadDir(dir)
	if err != nil {
		return false, err
	}
	fset := token.NewFileSet()

	foundGo := false
	for _, d := range df {
		path := filepath.Join(dir, d.Name())
		// Skip non-Go files
		if filepath.Ext(path) != ".go" {
			continue
		}
		foundGo = true

		// Parse the file
		node, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return false, err
		}

		// Check the imports in the file
		for _, imp := range node.Imports {
			// The path value includes quotes, so we need to trim them
			importPath := imp.Path.Value[1 : len(imp.Path.Value)-1]
			if importPath == "reflect" {
				return false, nil
			}
		}
	}
	if foundGo {
		return true, nil
	}
	return false, nil
}

// copyFile copies a file from src to dst
func copyFile(src, dst string, mode os.FileMode) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}

func diffDirs(a, b []os.DirEntry) []os.DirEntry {
	m := make(map[string]os.DirEntry)
	for _, f := range a {
		if f.IsDir() {
			continue
		}
		m[f.Name()] = f
	}

	var diff []os.DirEntry
	for _, f := range b {
		if f.IsDir() {
			continue
		}
		if _, ok := m[f.Name()]; !ok {
			diff = append(diff, f)
		}
	}

	return diff
}

// isExecutable checks if the given file path points to an executable file.
func isExecutable(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return false, err
	}

	// Check if the file is executable by the owner, group, or others
	mode := info.Mode()
	isExec := mode&0111 != 0 // Checks any executable bit (owner, group, others)

	return isExec, nil
}

func optimize(root string) error {
	pool, err := pooled.New("optimizer", 5)
	if err != nil {
		return err
	}
	defer pool.Close()

	wg := wait.Group{
		Pool: pool,
	}
	ctx := context.Background()

	wdErr := filepath.WalkDir(
		root,
		func(path string, d os.DirEntry, err error) error {
			switch {
			case err != nil:
				return err
			case d.IsDir() && strings.HasPrefix(d.Name(), "."):
				// Skip this directory and all of its contents
				return filepath.SkipDir
			case d.IsDir():
				optimize, err := shouldOptimize(path)
				if err != nil {
					return err
				}
				if optimize {
					args := []string{"-apply"}
					if *generatedFiles {
						args = append(args, "-generated_files")
					}
					if *testFiles {
						args = append(args, "-test_files")
					}
					args = append(args, ".")
					wg.Go(
						ctx,
						func(ctx context.Context) error {
							fmt.Println("Optimizing: ", path)
							defer fmt.Println("done with: ", path)
							// Run betteralign twice to ensure that the alignment is correct.
							for i := 0; i < 2; i++ {
								var out []byte
								cmd := exec.Command(alignPath, args...)
								cmd.Path = path
								out, err = exec.Command(alignPath, args...).CombinedOutput()
								if err != nil {
									fmt.Printf("Could not run betteralign: %v\n%s", err, out)
									return err
								}
							}
							return nil
						},
					)
				}
			}
			return nil
		},
	)

	log.Println("Waiting for all optimizations to finish")
	if err := wg.Wait(context.Background()); err != nil {
		return err
	}
	log.Println("All optimizations finished")

	if wdErr != nil {
		return wdErr
	}
	return nil
}

func main() {
	flag.Var(&goflags, "goflags", "Additional flags to pass to go compiler")
	flag.Parse()

	if *help {
		fmt.Println(helpText)
		os.Exit(0)
	}

	originalDir, err := os.Getwd()
	if err != nil {
		fmt.Printf("Could not get current directory: %v", err)
		return
	}

	modPath, err := findGoMod()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	modPath = filepath.Dir(modPath)

	defer func() {
		if err != nil {
			os.Exit(1)
		}
	}()

	// Make our temporary directory and copy all files to it.
	tmpDir := filepath.Join(os.TempDir(), "goptimizer", uuid.New().String())
	err = os.MkdirAll(tmpDir, 0755)
	if err != nil {
		fmt.Printf("Could not create temporary directory: %v", err)
		return
	}
	/*
		defer func() {
			if err := os.RemoveAll(tmpDir); err != nil {
				fmt.Printf("Could not remove temporary directory: %v", err)
			}
		}()
	*/
	if err = copyFiles(modPath, tmpDir); err != nil {
		fmt.Printf("Could not copy files to temporary directory: %v", err)
		return
	}

	if err = os.Chdir(tmpDir); err != nil {
		fmt.Printf("Could not change directory to temporary directory: %v", err)
		return
	}
	fmt.Println("temporary build directory: ", tmpDir)

	// Run go mod tidy and go mod vendor.
	if err = exec.Command(goExecPath, "mod", "tidy").Run(); err != nil {
		fmt.Printf("Could not run go mod tidy: %v", err)
		return
	}
	if err = exec.Command(goExecPath, "mod", "vendor").Run(); err != nil {
		fmt.Printf("Could not run go mod vendor: %v", err)
		return
	}

	// Run betteralign.
	if err := optimize(tmpDir); err != nil {
		fmt.Printf("Could not optimize files: %v", err)
		return
	}

	// Run tests if the flag is set.
	if *runTests {
		log.Println("running tests")
		cmd := exec.Command(goExecPath, "test", "./...")
		cmd.Dir = tmpDir
		out, err := cmd.CombinedOutput()
		if err != nil {
			fmt.Printf("Problem running tests: %v\n%s", err, string(out))
			return
		}
		fmt.Println("Test output:\n")
		fmt.Println(string(out))
	}

	log.Println("preparing for build")
	// Run go build.
	relPath, err := filepath.Rel(modPath, originalDir)
	if err != nil {
		panic(err)
	}

	p := filepath.Join(tmpDir, relPath)

	before, err := os.ReadDir(p)
	if err != nil {
		fmt.Printf("Could not stat temporary directory: %v", err)
		return
	}

	args := []string{"build"}
	if goflags != nil {
		args = append(args, goflags...)
	}
	out, err := exec.Command(goExecPath, args...).CombinedOutput()
	if err != nil {
		fmt.Printf("Could not run go build: %v\n%s", err, out)
		return
	}

	after, err := os.ReadDir(p)
	if err != nil {
		fmt.Printf("Could not stat temporary directory: %v", err)
		return
	}

	// Check if any files were modified.
	diff := diffDirs(before, after)
	var executable []os.DirEntry
	for _, f := range diff {
		execute, err := isExecutable(filepath.Join(tmpDir, f.Name()))
		if err != nil {
			fmt.Printf("Could not check if file is executable: %v", err)
			return
		}
		if execute {
			executable = append(executable, f)
		}
	}

	switch len(executable) {
	case 0:
		fmt.Println("No executable files were generated by go build")
		return
	case 1:
		// Do nothing
	default:
		fmt.Printf("Multiple executable files were generated by go build at: %v", tmpDir)
		return
	}

	// Copy the executable to the original directory.
	srcFile := filepath.Join(tmpDir, executable[0].Name())
	dstFile := filepath.Join(originalDir, executable[0].Name())
	if err := copyFile(srcFile, dstFile, 0755); err != nil {
		fmt.Printf("Could not copy executable to original directory: %v", err)
		return
	}
}
