package files

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCopyFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		setup    func(t *testing.T) (src, dst string, mode os.FileMode)
		wantErr  bool
		validate func(t *testing.T, dst string, mode os.FileMode)
	}{
		{
			name: "Success: copy regular file with 0644 permissions",
			setup: func(t *testing.T) (src, dst string, mode os.FileMode) {
				tmpDir := t.TempDir()
				src = filepath.Join(tmpDir, "source.txt")
				dst = filepath.Join(tmpDir, "dest.txt")
				mode = 0644

				if err := os.WriteFile(src, []byte("test content"), mode); err != nil {
					t.Fatalf("failed to create test file: %v", err)
				}
				return src, dst, mode
			},
			wantErr: false,
			validate: func(t *testing.T, dst string, mode os.FileMode) {
				content, err := os.ReadFile(dst)
				if err != nil {
					t.Errorf("failed to read destination file: %v", err)
					return
				}
				if string(content) != "test content" {
					t.Errorf("got content %q, want %q", string(content), "test content")
				}
				info, err := os.Stat(dst)
				if err != nil {
					t.Errorf("failed to stat destination file: %v", err)
					return
				}
				if info.Mode() != mode {
					t.Errorf("got mode %v, want %v", info.Mode(), mode)
				}
			},
		},
		{
			name: "Success: copy file with 0755 permissions",
			setup: func(t *testing.T) (src, dst string, mode os.FileMode) {
				tmpDir := t.TempDir()
				src = filepath.Join(tmpDir, "source.sh")
				dst = filepath.Join(tmpDir, "dest.sh")
				mode = 0755

				if err := os.WriteFile(src, []byte("#!/bin/bash\necho test"), mode); err != nil {
					t.Fatalf("failed to create test file: %v", err)
				}
				return src, dst, mode
			},
			wantErr: false,
			validate: func(t *testing.T, dst string, mode os.FileMode) {
				info, err := os.Stat(dst)
				if err != nil {
					t.Errorf("failed to stat destination file: %v", err)
					return
				}
				if info.Mode() != mode {
					t.Errorf("got mode %v, want %v", info.Mode(), mode)
				}
			},
		},
		{
			name: "Error: source file does not exist",
			setup: func(t *testing.T) (src, dst string, mode os.FileMode) {
				tmpDir := t.TempDir()
				src = filepath.Join(tmpDir, "nonexistent.txt")
				dst = filepath.Join(tmpDir, "dest.txt")
				mode = 0644
				return src, dst, mode
			},
			wantErr: true,
		},
		{
			name: "Error: destination directory does not exist",
			setup: func(t *testing.T) (src, dst string, mode os.FileMode) {
				tmpDir := t.TempDir()
				src = filepath.Join(tmpDir, "source.txt")
				dst = filepath.Join(tmpDir, "nonexistent", "dest.txt")
				mode = 0644

				if err := os.WriteFile(src, []byte("test"), mode); err != nil {
					t.Fatalf("failed to create test file: %v", err)
				}
				return src, dst, mode
			},
			wantErr: true,
		},
		{
			name: "Success: overwrite existing file",
			setup: func(t *testing.T) (src, dst string, mode os.FileMode) {
				tmpDir := t.TempDir()
				src = filepath.Join(tmpDir, "source.txt")
				dst = filepath.Join(tmpDir, "dest.txt")
				mode = 0644

				if err := os.WriteFile(src, []byte("new content"), mode); err != nil {
					t.Fatalf("failed to create source file: %v", err)
				}
				if err := os.WriteFile(dst, []byte("old content"), mode); err != nil {
					t.Fatalf("failed to create dest file: %v", err)
				}
				return src, dst, mode
			},
			wantErr: false,
			validate: func(t *testing.T, dst string, mode os.FileMode) {
				content, err := os.ReadFile(dst)
				if err != nil {
					t.Errorf("failed to read destination file: %v", err)
					return
				}
				if string(content) != "new content" {
					t.Errorf("got content %q, want %q", string(content), "new content")
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			src, dst, mode := test.setup(t)

			err := CopyFile(src, dst, mode)

			switch {
			case err == nil && test.wantErr:
				t.Errorf("TestCopyFile(%s): got err == nil, want err != nil", test.name)
				return
			case err != nil && !test.wantErr:
				t.Errorf("TestCopyFile(%s): got err == %s, want err == nil", test.name, err)
				return
			case err != nil:
				return
			}

			if test.validate != nil {
				test.validate(t, dst, mode)
			}
		})
	}
}

func TestIsExecutable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setup   func(t *testing.T) string
		want    bool
		wantErr bool
	}{
		{
			name: "Success: file with owner executable permissions returns true",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				path := filepath.Join(tmpDir, "executable.sh")
				if err := os.WriteFile(path, []byte("#!/bin/bash"), 0755); err != nil {
					t.Fatalf("failed to create test file: %v", err)
				}
				return path
			},
			want:    true,
			wantErr: false,
		},
		{
			name: "Success: file without executable permissions returns false",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				path := filepath.Join(tmpDir, "notexec.txt")
				if err := os.WriteFile(path, []byte("content"), 0644); err != nil {
					t.Fatalf("failed to create test file: %v", err)
				}
				return path
			},
			want:    false,
			wantErr: false,
		},
		{
			name: "Error: file does not exist",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				return filepath.Join(tmpDir, "nonexistent")
			},
			want:    false,
			wantErr: true,
		},
		{
			name: "Success: file with group executable bit returns true",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				path := filepath.Join(tmpDir, "groupexec")
				if err := os.WriteFile(path, []byte("content"), 0650); err != nil {
					t.Fatalf("failed to create test file: %v", err)
				}
				return path
			},
			want:    true,
			wantErr: false,
		},
		{
			name: "Success: file with other executable bit returns true",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				path := filepath.Join(tmpDir, "otherexec")
				if err := os.WriteFile(path, []byte("content"), 0705); err != nil {
					t.Fatalf("failed to create test file: %v", err)
				}
				return path
			},
			want:    true,
			wantErr: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := test.setup(t)

			got, err := IsExecutable(path)

			switch {
			case err == nil && test.wantErr:
				t.Errorf("TestIsExecutable(%s): got err == nil, want err != nil", test.name)
				return
			case err != nil && !test.wantErr:
				t.Errorf("TestIsExecutable(%s): got err == %s, want err == nil", test.name, err)
				return
			case err != nil:
				return
			}

			if got != test.want {
				t.Errorf("TestIsExecutable(%s): got %v, want %v", test.name, got, test.want)
			}
		})
	}
}

func TestMd5Sum(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setup   func(t *testing.T) string
		want    string
		wantErr bool
	}{
		{
			name: "Success: calculate correct MD5 for file",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				path := filepath.Join(tmpDir, "test.txt")
				if err := os.WriteFile(path, []byte("hello world"), 0644); err != nil {
					t.Fatalf("failed to create test file: %v", err)
				}
				return path
			},
			want:    "5eb63bbbe01eeed093cb22bb8f5acdc3",
			wantErr: false,
		},
		{
			name: "Error: file does not exist",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				return filepath.Join(tmpDir, "nonexistent.txt")
			},
			want:    "",
			wantErr: true,
		},
		{
			name: "Success: empty file has specific MD5",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				path := filepath.Join(tmpDir, "empty.txt")
				if err := os.WriteFile(path, []byte(""), 0644); err != nil {
					t.Fatalf("failed to create test file: %v", err)
				}
				return path
			},
			want:    "d41d8cd98f00b204e9800998ecf8427e",
			wantErr: false,
		},
		{
			name: "Success: different content has different MD5",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				path := filepath.Join(tmpDir, "test2.txt")
				if err := os.WriteFile(path, []byte("different content"), 0644); err != nil {
					t.Fatalf("failed to create test file: %v", err)
				}
				return path
			},
			want:    "fb9ca3de466c5e579dc8aaf5f1e6940e",
			wantErr: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := test.setup(t)

			got, err := md5Sum(path)

			switch {
			case err == nil && test.wantErr:
				t.Errorf("TestMd5Sum(%s): got err == nil, want err != nil", test.name)
				return
			case err != nil && !test.wantErr:
				t.Errorf("TestMd5Sum(%s): got err == %s, want err == nil", test.name, err)
				return
			case err != nil:
				return
			}

			if got != test.want {
				t.Errorf("TestMd5Sum(%s): got %s, want %s", test.name, got, test.want)
			}
		})
	}
}

func TestNewAddedOrChanged(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setup   func(t *testing.T) string
		wantErr bool
	}{
		{
			name: "Success: initialize with existing directory",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				file1 := filepath.Join(tmpDir, "file1.txt")
				if err := os.WriteFile(file1, []byte("content"), 0644); err != nil {
					t.Fatalf("failed to create file: %v", err)
				}
				return tmpDir
			},
			wantErr: false,
		},
		{
			name: "Error: directory does not exist",
			setup: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "nonexistent")
			},
			wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := test.setup(t)

			tracker, err := NewAddedOrChanged(path)

			switch {
			case err == nil && test.wantErr:
				t.Errorf("TestNewAddedOrChanged(%s): got err == nil, want err != nil", test.name)
				return
			case err != nil && !test.wantErr:
				t.Errorf("TestNewAddedOrChanged(%s): got err == %s, want err == nil", test.name, err)
				return
			case err != nil:
				return
			}

			if tracker == nil {
				t.Errorf("TestNewAddedOrChanged(%s): got nil tracker", test.name)
			}
		})
	}
}

func TestAddedOrChangedCompare(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		setup    func(t *testing.T) (*AddedOrChanged, error)
		want     []DiffResult
		wantErr  bool
	}{
		{
			name: "Success: detect newly added files",
			setup: func(t *testing.T) (*AddedOrChanged, error) {
				tmpDir := t.TempDir()

				file1 := filepath.Join(tmpDir, "file1.txt")
				if err := os.WriteFile(file1, []byte("content1"), 0644); err != nil {
					t.Fatalf("failed to create file1: %v", err)
				}

				tracker, err := NewAddedOrChanged(tmpDir)
				if err != nil {
					return nil, err
				}

				file2 := filepath.Join(tmpDir, "file2.txt")
				if err := os.WriteFile(file2, []byte("content2"), 0644); err != nil {
					t.Fatalf("failed to create file2: %v", err)
				}

				return tracker, nil
			},
			want: []DiffResult{
				{Type: Added},
			},
			wantErr: false,
		},
		{
			name: "Success: detect modified files",
			setup: func(t *testing.T) (*AddedOrChanged, error) {
				tmpDir := t.TempDir()

				file1 := filepath.Join(tmpDir, "file1.txt")
				if err := os.WriteFile(file1, []byte("original"), 0644); err != nil {
					t.Fatalf("failed to create file1: %v", err)
				}

				tracker, err := NewAddedOrChanged(tmpDir)
				if err != nil {
					return nil, err
				}

				if err := os.WriteFile(file1, []byte("modified"), 0644); err != nil {
					t.Fatalf("failed to modify file1: %v", err)
				}

				return tracker, nil
			},
			want: []DiffResult{
				{Type: Modified},
			},
			wantErr: false,
		},
		{
			name: "Success: no changes when files are identical",
			setup: func(t *testing.T) (*AddedOrChanged, error) {
				tmpDir := t.TempDir()

				file1 := filepath.Join(tmpDir, "file1.txt")
				if err := os.WriteFile(file1, []byte("content"), 0644); err != nil {
					t.Fatalf("failed to create file1: %v", err)
				}

				tracker, err := NewAddedOrChanged(tmpDir)
				if err != nil {
					return nil, err
				}

				return tracker, nil
			},
			want:    nil,
			wantErr: false,
		},
		{
			name: "Success: mixed added and modified files",
			setup: func(t *testing.T) (*AddedOrChanged, error) {
				tmpDir := t.TempDir()

				file1 := filepath.Join(tmpDir, "file1.txt")
				if err := os.WriteFile(file1, []byte("original"), 0644); err != nil {
					t.Fatalf("failed to create file1: %v", err)
				}

				tracker, err := NewAddedOrChanged(tmpDir)
				if err != nil {
					return nil, err
				}

				if err := os.WriteFile(file1, []byte("modified"), 0644); err != nil {
					t.Fatalf("failed to modify file1: %v", err)
				}

				file2 := filepath.Join(tmpDir, "file2.txt")
				if err := os.WriteFile(file2, []byte("new"), 0644); err != nil {
					t.Fatalf("failed to create file2: %v", err)
				}

				return tracker, nil
			},
			want: []DiffResult{
				{Type: Modified},
				{Type: Added},
			},
			wantErr: false,
		},
		{
			name: "Success: ignore directories in comparison",
			setup: func(t *testing.T) (*AddedOrChanged, error) {
				tmpDir := t.TempDir()

				file1 := filepath.Join(tmpDir, "file1.txt")
				if err := os.WriteFile(file1, []byte("content"), 0644); err != nil {
					t.Fatalf("failed to create file1: %v", err)
				}

				tracker, err := NewAddedOrChanged(tmpDir)
				if err != nil {
					return nil, err
				}

				subdir := filepath.Join(tmpDir, "subdir")
				if err := os.Mkdir(subdir, 0755); err != nil {
					t.Fatalf("failed to create subdir: %v", err)
				}

				return tracker, nil
			},
			want:    nil,
			wantErr: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tracker, err := test.setup(t)
			if err != nil {
				t.Fatalf("setup failed: %v", err)
			}

			got, err := tracker.Compare()

			switch {
			case err == nil && test.wantErr:
				t.Errorf("TestAddedOrChangedCompare(%s): got err == nil, want err != nil", test.name)
				return
			case err != nil && !test.wantErr:
				t.Errorf("TestAddedOrChangedCompare(%s): got err == %s, want err == nil", test.name, err)
				return
			case err != nil:
				return
			}

			if len(got) != len(test.want) {
				t.Errorf("TestAddedOrChangedCompare(%s): got %d results, want %d results", test.name, len(got), len(test.want))
				return
			}

			for i := range got {
				if got[i].Type != test.want[i].Type {
					t.Errorf("TestAddedOrChangedCompare(%s): result[%d] got type %v, want type %v", test.name, i, got[i].Type, test.want[i].Type)
				}
			}
		})
	}
}

func TestFindGoMod(t *testing.T) {
	tests := []struct {
		name       string
		goExecPath string
		wantErr    bool
	}{
		{
			name:       "Success: find go.mod in current module",
			goExecPath: "go",
			wantErr:    false,
		},
		{
			name:       "Error: invalid go executable path",
			goExecPath: "/nonexistent/go",
			wantErr:    true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := FindGoMod(test.goExecPath)

			switch {
			case err == nil && test.wantErr:
				t.Errorf("TestFindGoMod(%s): got err == nil, want err != nil", test.name)
				return
			case err != nil && !test.wantErr:
				t.Errorf("TestFindGoMod(%s): got err == %s, want err == nil", test.name, err)
				return
			case err != nil:
				return
			}

			if got == "" {
				t.Errorf("TestFindGoMod(%s): got empty path", test.name)
			}
			if got == "/dev/null" {
				t.Errorf("TestFindGoMod(%s): got /dev/null, should have returned error", test.name)
			}
		})
	}
}

func TestCreateOptimized(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		setup    func(t *testing.T) (srcPath, dstPath string)
		wantErr  bool
		validate func(t *testing.T, srcPath, dstPath string)
	}{
		{
			name: "Success: copy directory structure with files",
			setup: func(t *testing.T) (srcPath, dstPath string) {
				baseDir := t.TempDir()
				srcPath = filepath.Join(baseDir, "src")
				dstPath = filepath.Join(baseDir, "dst")

				if err := os.MkdirAll(srcPath, 0755); err != nil {
					t.Fatalf("failed to create src dir: %v", err)
				}
				if err := os.MkdirAll(dstPath, 0755); err != nil {
					t.Fatalf("failed to create dst dir: %v", err)
				}

				file1 := filepath.Join(srcPath, "file1.txt")
				if err := os.WriteFile(file1, []byte("content1"), 0644); err != nil {
					t.Fatalf("failed to create file1: %v", err)
				}

				return srcPath, dstPath
			},
			wantErr: false,
			validate: func(t *testing.T, srcPath, dstPath string) {
				dstFile := filepath.Join(dstPath, "file1.txt")
				if _, err := os.Stat(dstFile); err != nil {
					t.Errorf("destination file not created: %v", err)
				}
			},
		},
		{
			name: "Success: skip hidden directories",
			setup: func(t *testing.T) (srcPath, dstPath string) {
				baseDir := t.TempDir()
				srcPath = filepath.Join(baseDir, "src")
				dstPath = filepath.Join(baseDir, "dst")

				if err := os.MkdirAll(srcPath, 0755); err != nil {
					t.Fatalf("failed to create src dir: %v", err)
				}
				if err := os.MkdirAll(dstPath, 0755); err != nil {
					t.Fatalf("failed to create dst dir: %v", err)
				}

				hiddenDir := filepath.Join(srcPath, ".hidden")
				if err := os.MkdirAll(hiddenDir, 0755); err != nil {
					t.Fatalf("failed to create hidden dir: %v", err)
				}

				hiddenFile := filepath.Join(hiddenDir, "secret.txt")
				if err := os.WriteFile(hiddenFile, []byte("secret"), 0644); err != nil {
					t.Fatalf("failed to create hidden file: %v", err)
				}

				file1 := filepath.Join(srcPath, "file1.txt")
				if err := os.WriteFile(file1, []byte("content1"), 0644); err != nil {
					t.Fatalf("failed to create file1: %v", err)
				}

				return srcPath, dstPath
			},
			wantErr: false,
			validate: func(t *testing.T, srcPath, dstPath string) {
				hiddenDstDir := filepath.Join(dstPath, ".hidden")
				if _, err := os.Stat(hiddenDstDir); err == nil {
					t.Errorf("hidden directory should not be copied")
				}

				dstFile := filepath.Join(dstPath, "file1.txt")
				if _, err := os.Stat(dstFile); err != nil {
					t.Errorf("regular file should be copied: %v", err)
				}
			},
		},
		{
			name: "Success: create nested directory structure",
			setup: func(t *testing.T) (srcPath, dstPath string) {
				baseDir := t.TempDir()
				srcPath = filepath.Join(baseDir, "src")
				dstPath = filepath.Join(baseDir, "dst")

				nestedDir := filepath.Join(srcPath, "level1", "level2")
				if err := os.MkdirAll(nestedDir, 0755); err != nil {
					t.Fatalf("failed to create nested dir: %v", err)
				}
				if err := os.MkdirAll(dstPath, 0755); err != nil {
					t.Fatalf("failed to create dst dir: %v", err)
				}

				nestedFile := filepath.Join(nestedDir, "deep.txt")
				if err := os.WriteFile(nestedFile, []byte("deep content"), 0644); err != nil {
					t.Fatalf("failed to create nested file: %v", err)
				}

				return srcPath, dstPath
			},
			wantErr: false,
			validate: func(t *testing.T, srcPath, dstPath string) {
				dstFile := filepath.Join(dstPath, "level1", "level2", "deep.txt")
				content, err := os.ReadFile(dstFile)
				if err != nil {
					t.Errorf("nested file not created: %v", err)
					return
				}
				if string(content) != "deep content" {
					t.Errorf("got content %q, want %q", string(content), "deep content")
				}
			},
		},
		{
			name: "Success: copy file permissions correctly",
			setup: func(t *testing.T) (srcPath, dstPath string) {
				baseDir := t.TempDir()
				srcPath = filepath.Join(baseDir, "src")
				dstPath = filepath.Join(baseDir, "dst")

				if err := os.MkdirAll(srcPath, 0755); err != nil {
					t.Fatalf("failed to create src dir: %v", err)
				}
				if err := os.MkdirAll(dstPath, 0755); err != nil {
					t.Fatalf("failed to create dst dir: %v", err)
				}

				execFile := filepath.Join(srcPath, "script.sh")
				if err := os.WriteFile(execFile, []byte("#!/bin/bash"), 0755); err != nil {
					t.Fatalf("failed to create executable file: %v", err)
				}

				return srcPath, dstPath
			},
			wantErr: false,
			validate: func(t *testing.T, srcPath, dstPath string) {
				dstFile := filepath.Join(dstPath, "script.sh")
				info, err := os.Stat(dstFile)
				if err != nil {
					t.Errorf("destination file not created: %v", err)
					return
				}
				if info.Mode() != 0755 {
					t.Errorf("got mode %v, want %v", info.Mode(), os.FileMode(0755))
				}
			},
		},
		{
			name: "Error: source directory does not exist",
			setup: func(t *testing.T) (srcPath, dstPath string) {
				tmpDir := t.TempDir()
				srcPath = filepath.Join(tmpDir, "nonexistent")
				dstPath = filepath.Join(tmpDir, "dst")
				if err := os.MkdirAll(dstPath, 0755); err != nil {
					t.Fatalf("failed to create dst dir: %v", err)
				}
				return srcPath, dstPath
			},
			wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			srcPath, dstPath := test.setup(t)

			err := CreateOptimized(srcPath, dstPath)

			switch {
			case err == nil && test.wantErr:
				t.Errorf("TestCreateOptimized(%s): got err == nil, want err != nil", test.name)
				return
			case err != nil && !test.wantErr:
				t.Errorf("TestCreateOptimized(%s): got err == %s, want err == nil", test.name, err)
				return
			case err != nil:
				return
			}

			if test.validate != nil {
				test.validate(t, srcPath, dstPath)
			}
		})
	}
}

func TestDiffResultTypes(t *testing.T) {
	tests := []struct {
		name string
		ct   ChangeType
		want ChangeType
	}{
		{
			name: "Success: Added constant has correct value",
			ct:   Added,
			want: 1,
		},
		{
			name: "Success: Modified constant has correct value",
			ct:   Modified,
			want: 2,
		},
		{
			name: "Success: CTUnknown has zero value",
			ct:   CTUnknown,
			want: 0,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.ct != test.want {
				t.Errorf("TestDiffResultTypes(%s): got %v, want %v", test.name, test.ct, test.want)
			}
		})
	}
}
