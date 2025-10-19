// Package files provides utilities for file and directory operations.
package files

import (
	"crypto/md5"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// CreateOptimized copies all directories and files recursively from srcPath to dstPath,
// but only if a directory contains at least one .go file. The files now in dstPath will
// be used to build our optimized version.
func CreateOptimized(srcPath, dstPath string) error {
	if _, err := os.Stat(srcPath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("source path does not exist: %s", srcPath)
		}
		return err
	}

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
				return err
			}
			if err := CopyFile(path, dest, fi.Mode()); err != nil {
				return err
			}
			return nil
		},
	)
}

// CopyFile copies a file from src to dst
func CopyFile(src, dst string, mode os.FileMode) error {
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

// IsExecutable checks if the given file path points to an executable file.
func IsExecutable(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return false, err
	}

	// Check if the file is executable by the owner, group, or others
	mode := info.Mode()
	isExec := mode&0111 != 0 // Checks any executable bit (owner, group, others)

	return isExec, nil
}

type ChangeType uint8

const (
	// CTUnknown indicates an unknown change type. This is always a bug.
	CTUnknown ChangeType = iota
	// CTAdded means the files was added to the directory.
	Added
	// CTModified means the file was modified in the directory.
	Modified
)

// DiffResult holds the result of a diff between two directories.
type DiffResult struct {
	// Type is the type of change.
	Type ChangeType
	// DirEntry is the file that was changed.
	DirEntry os.DirEntry
}

type found struct {
	de  os.DirEntry
	md5 string
}

// AddedOrChanged returns the files are either new in b or have changed from a to b.
// This is intended to be used to compare two directory listings of the same directory at different times.
type AddedOrChanged struct {
	path    string
	initial map[string]found
}

// NewAddedOrChanged creates a new AddedOrChanged struct and initializes it with the contents of the directory at path.
func NewAddedOrChanged(path string) (*AddedOrChanged, error) {
	a := &AddedOrChanged{}
	if err := a.init(path); err != nil {
		return nil, err
	}
	return a, nil
}

// Init initializes the AddedOrChanged struct with the contents of the directory at path.
func (a *AddedOrChanged) init(path string) error {
	if a.initial != nil {
		return fmt.Errorf("AddedOrChanged.A: already initialized")
	}

	before, err := os.ReadDir(path)
	if err != nil {
		return fmt.Errorf("Could not stat temporary directory: %v", err)
	}

	m := make(map[string]found, len(before))
	for _, f := range before {
		if f.IsDir() {
			continue
		}
		sum, err := md5Sum(filepath.Join(path, f.Name()))
		if err != nil {
			return err
		}
		m[f.Name()] = found{de: f, md5: sum}
	}
	a.initial = m
	a.path = path
	return nil
}

// Compare compares the current contents of the directory at path to the initial contents set by A.
func (a *AddedOrChanged) Compare() ([]DiffResult, error) {
	if a.initial == nil {
		return nil, fmt.Errorf("AddedOrChanged.B: not initialized")
	}

	after, err := os.ReadDir(a.path)
	if err != nil {
		return nil, fmt.Errorf("Could not stat temporary directory: %v", err)
	}

	var diff []DiffResult
	for _, f := range after {
		if f.IsDir() {
			continue
		}
		if _, ok := a.initial[f.Name()]; ok {
			sum, err := md5Sum(filepath.Join(a.path, f.Name()))
			if err != nil {
				return nil, err
			}
			if a.initial[f.Name()].md5 != sum {
				diff = append(diff, DiffResult{Type: Modified, DirEntry: f})
			}
		} else {
			diff = append(diff, DiffResult{Type: Added, DirEntry: f})
		}
	}

	return diff, nil
}

// md5Sum returns the MD5 checksum of the file at fPath.
func md5Sum(fPath string) (string, error) {
	f, err := os.Open(fPath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := md5.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

// FindGoMod returns the path to the go.mod file in the current module.
func FindGoMod(goExecPath string) (string, error) {
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
