package sh

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/craiggwilson/goke/task"
)

// Copy copies either a file or a directory recursively.
func Copy(ctx *task.Context, fromPath, toPath string) error {
	ctx.Logf("cp: %s -> %s\n", fromPath, toPath)

	fromPath = filepath.Clean(fromPath)
	toPath = filepath.Clean(toPath)

	fi, err := os.Stat(fromPath)
	if err != nil {
		return err
	}
	if fi.IsDir() {
		return copyDirectory(fromPath, toPath)
	}

	return copyFile(fromPath, toPath)
}

func copyDirectory(fromPath, toPath string) error {
	_, err := os.Stat(toPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	} else if err == nil {
		return fmt.Errorf("destination already exists")
	}

	return filepath.Walk(fromPath, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		to := filepath.Join(toPath, strings.TrimPrefix(path, fromPath))

		if fi.IsDir() {
			if err = os.MkdirAll(to, fi.Mode()); err != nil {
				return err
			}
			return nil
		}

		return copyFile(path, to)
	})
}

func copyFile(fromPath, toPath string) error {
	from, err := os.Open(fromPath)
	if err != nil {
		return fmt.Errorf("failed opening %s: %v", fromPath, err)
	}
	defer from.Close()

	fi, err := from.Stat()
	if err != nil {
		return fmt.Errorf("failed statting %s: %v", fromPath, err)
	}

	return copyTo(fromPath, from, toPath, fi.Mode())
}

func copyTo(fromPath string, from io.Reader, toPath string, toMode os.FileMode) error {
	to, err := os.OpenFile(toPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, toMode)
	if err != nil {
		return fmt.Errorf("failed creating/opening %s: %v", toPath, err)
	}
	defer to.Close()

	_, err = io.Copy(to, from)
	if err != nil {
		return fmt.Errorf("failed copying %s to %s: %v", fromPath, toPath, err)
	}

	return nil
}

// CreateDirectory creates a directory.
func CreateDirectory(ctx *task.Context, path string) error {
	ctx.Logf("mkdir: %s\n", path)
	err := os.Mkdir(path, os.ModePerm)
	if err != nil {
		return fmt.Errorf("failed making directory %s: %v", path, err)
	}

	return nil
}

// CreateDirectoryR creates a directory recursively.
func CreateDirectoryR(ctx *task.Context, path string) error {
	ctx.Logf("mkdir -r: %s\n", path)
	err := os.MkdirAll(path, os.ModePerm)
	if err != nil {
		return fmt.Errorf("failed making directory %s: %v", path, err)
	}

	return nil
}

// CreateFile creates a file.
func CreateFile(ctx *task.Context, path string) (*os.File, error) {
	ctx.Logf("touch: %s\n", path)
	f, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("failed creating file %s: %v", path, err)
	}

	return f, nil
}

// CreateFileR creates a file ensuring all the directories are created recursively.
func CreateFileR(ctx *task.Context, path string) (*os.File, error) {
	ctx.Logf("touch -r: %s\n", path)
	dir := filepath.Dir(path)

	err := CreateDirectoryR(ctx, dir)
	if err != nil {
		return nil, err // already has a good error message
	}

	return CreateFile(ctx, path)
}

// DirectoryExists indicates if the directory exists.
func DirectoryExists(path string) (bool, error) {
	fi, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}

		if exists, err := DirectoryExists(filepath.Dir(path)); !exists || err != nil {
			return false, err
		}

		return false, fmt.Errorf("failed statting path %s: %v", path, err)
	}

	return fi.IsDir(), nil
}

// FileExists indicates if the file exists.
func FileExists(path string) (bool, error) {
	fi, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}

		if exists, err := DirectoryExists(filepath.Dir(path)); !exists || err != nil {
			return false, err
		}

		return false, fmt.Errorf("failed statting path %s: %v", path, err)
	}

	return !fi.IsDir(), nil
}

// IsDirectoryEmpty indicates if the directory is empty.
func IsDirectoryEmpty(path string) (bool, error) {
	fi, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, fmt.Errorf("directory %s does not exist", path)
		}

		return false, fmt.Errorf("failed statting path %s: %v", path, err)
	}

	if !fi.IsDir() {
		return false, fmt.Errorf("%s is not a directory", path)
	}

	f, err := os.Open(path)
	if err != nil {
		return false, fmt.Errorf("failed opening %s: %v", path, err)
	}
	defer f.Close()
	entries, _ := f.Readdir(-1)
	return len(entries) == 0, nil
}

// IsFileEmpty indicates if the file is empty.
func IsFileEmpty(path string) (bool, error) {
	fi, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, fmt.Errorf("file %s does not exist", path)
		}

		return false, fmt.Errorf("failed statting path %s: %v", path, err)
	}

	if fi.IsDir() {
		return false, fmt.Errorf("%s is not a file", path)
	}

	return fi.Size() == 0, nil
}

// Move moves a file or directory.
func Move(ctx *task.Context, fromPath, toPath string) error {
	ctx.Logf("mv: %s -> %s\n", fromPath, toPath)
	fromPath = filepath.Clean(fromPath)
	toPath = filepath.Clean(toPath)

	fi, err := os.Stat(fromPath)
	if err != nil {
		return err
	}
	if fi.IsDir() {
		return moveDirectory(fromPath, toPath)
	}

	return moveFile(fromPath, toPath)
}

func moveDirectory(fromPath, toPath string) error {
	err := filepath.Walk(fromPath, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		to := filepath.Join(toPath, strings.TrimPrefix(path, fromPath))

		if fi.IsDir() {
			if err = os.MkdirAll(to, fi.Mode()); err != nil {
				return err
			}
			return nil
		}

		return copyFile(path, to)
	})
	if err != nil {
		return err
	}

	return removeDirectory(fromPath)
}

func moveFile(fromPath, toPath string) error {
	err := copyFile(fromPath, toPath)
	if err != nil {
		return err
	}

	return removeFile(fromPath)
}

// Remove either removes a file or a directory recursively.
func Remove(ctx *task.Context, path string) error {
	ctx.Logf("rm: %s\n", path)

	path = filepath.Clean(path)

	fi, err := os.Stat(path)
	if err != nil {
		return err
	}
	if fi.IsDir() {
		return removeDirectory(path)
	}

	return removeFile(path)
}

func removeDirectory(path string) error {
	_, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}

		return err
	}

	err = os.RemoveAll(path)
	if err != nil {
		return fmt.Errorf("failed removing %s: %v", path, err)
	}

	return nil
}

func removeFile(path string) error {
	_, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}

		return err
	}

	err = os.Remove(path)
	if err != nil {
		return fmt.Errorf("failed removing %s: %v", path, err)
	}

	return nil
}
