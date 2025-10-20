package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/gostdlib/base/context"
	"github.com/johnsiilver/goptimizer/files"
	"github.com/johnsiilver/goptimizer/optimize"
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
  -keep bool
  		Keep the temporary directory with the aligned files.
`

var (
	help              = flag.Bool("help", false, "Show help")
	goBinary          = flag.String("goBinary", "go", "Name of the go binary to use")
	betterAlignBinary = flag.String("betterAlignBinary", "betteralign", "Name of the betteralign binary to use")
	fieldAlign        = flag.Bool("fieldAlign", true, "Field align source files")
	replaceInterfaces = flag.Bool("replaceInterfaces", true, "Replace interface types with concrete types where annotated")
	generatedFiles    = flag.Bool("generated", false, "Field align generated files")
	runTests          = flag.Bool("runTests", false, "Will run tests before building the binary")
	keep              = flag.Bool("keep", false, "Keep the temporary directory with the aligned files")
	doNotVendor       = flag.Bool("doNotVendor", false, "Do not run 'go mod vendor' before building")
	goflags           stringArray
)

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

var (
	goExecPath, alignPath string
)

func lookups() {
	var err error
	goExecPath, err = exec.LookPath(*goBinary)
	if err != nil {
		fmt.Println("go binary not found on path")
		os.Exit(1)
	}

	alignPath, err = exec.LookPath(*betterAlignBinary)
	if err != nil {
		fmt.Println("betteralign binary not found on path")
		os.Exit(1)
	}
}

func main() {
	ctx := context.Background()

	flag.Var(&goflags, "goflags", "Additional flags to pass to go compiler")
	flag.Parse()

	lookups()

	if *help {
		fmt.Println(helpText)
		os.Exit(0)
	}

	originalDir, err := os.Getwd()
	if err != nil {
		fmt.Printf("Could not get current directory: %v", err)
		os.Exit(1)
	}

	modPath, err := files.FindGoMod(goExecPath)
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
	defer func() {
		if !*keep {
			if err := os.RemoveAll(tmpDir); err != nil {
				fmt.Printf("Could not remove temporary directory: %v", err)
			}
		}
		if err != nil {
			os.Exit(1)
		}
	}()

	if err := files.CreateOptimized(modPath, tmpDir); err != nil {
		fmt.Printf("Could not copy files to temporary directory: %v", err)
		return
	}

	if err = os.Chdir(tmpDir); err != nil {
		fmt.Printf("Could not change directory to temporary directory: %v", err)
		return
	}
	fmt.Println("temporary build directory: ", tmpDir)

	if err = exec.Command(goExecPath, "mod", "tidy").Run(); err != nil {
		fmt.Printf("Could not run go mod tidy: %v", err)
		return
	}

	if !*doNotVendor {
		if err = exec.Command(goExecPath, "mod", "vendor").Run(); err != nil {
			fmt.Printf("Could not run go mod vendor: %v", err)
			return
		}
	}

	err = optimize.Packages(
		ctx,
		optimize.Args{
			Root:       tmpDir,
			GoExecPath: goExecPath,
			AlignPath:  alignPath,
			FieldAlign: *fieldAlign,
			Generated:  *generatedFiles,
			RunTests:   *runTests,
		},
	)
	if err != nil {
		fmt.Printf("Could not optimize packages: %v", err)
		return
	}

	log.Println("preparing for build")
	// Run go build.
	relPath, err := filepath.Rel(modPath, originalDir)
	if err != nil {
		panic(err)
	}

	p := filepath.Join(tmpDir, relPath)

	ac, err := files.NewAddedOrChanged(p)
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

	diff, err := ac.Compare()
	if err != nil {
		fmt.Printf("Could not check for modified files: %v", err)
		return
	}
	var executable []os.DirEntry
	for _, f := range diff {
		execute, err := files.IsExecutable(filepath.Join(tmpDir, f.DirEntry.Name()))
		if err != nil {
			fmt.Printf("Could not check if file is executable: %v", err)
			return
		}
		if execute {
			executable = append(executable, f.DirEntry)
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
	if err := files.CopyFile(srcFile, dstFile, 0755); err != nil {
		fmt.Printf("Could not copy executable to original directory: %v", err)
		return
	}
}
