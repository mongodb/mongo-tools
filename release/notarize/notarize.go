package notarize

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

func FindInvalidNotarizations(zipPath string) ([]string, error) {
	// Open the ZIP archive
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, fmt.Errorf("opening %#q: %w", zipPath, err)
	}
	defer r.Close()

	// Create a temporary directory
	destDir, err := os.MkdirTemp("", "unzipped-*")
	if err != nil {
		return nil, fmt.Errorf("making temp directory: %w", err)
	}

	defer func() {
		os.RemoveAll(destDir)
	}()

	problems := []string{}

	// Iterate through each file in the archive
	for _, f := range r.File {
		fileMode := f.FileInfo().Mode()

		// Only check regular files.
		if !fileMode.IsRegular() {
			continue
		}

		// Only check executables.
		if fileMode.Perm()&0111 == 0 {
			continue
		}

		baseName := filepath.Base(f.Name)
		// Ensure the archive entry name cannot cause directory traversal or other issues.
		if baseName == "" || baseName == "." || baseName == ".." {
			continue
		}
		fpath := filepath.Join(destDir, baseName)

		// Create destination file
		outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return nil, fmt.Errorf("creating %#q: %w", fpath, err)
		}

		rc, err := f.Open()
		if err != nil {
			outFile.Close()
			return nil, fmt.Errorf("opening archive’s %#q: %w", f.Name, err)
		}

		// Copy file contents
		_, err = io.Copy(outFile, rc)

		// Close everything
		outFile.Close()
		rc.Close()

		if err != nil {
			return nil, fmt.Errorf("extracting %#q to %#q: %w", f.Name, fpath, err)
		}

		cmdPieces := []string{
			"/usr/sbin/spctl",
			"-vvvvv",
			"--assess",
			"--type", "install",
			fpath,
		}

		cmd := exec.Command(cmdPieces[0], cmdPieces[1:]...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		err = cmd.Run()

		if err != nil {
			exitErr := &exec.ExitError{}
			if errors.As(err, &exitErr) {
				problems = append(problems, f.Name)
			} else {
				return nil, fmt.Errorf("%#q: %w", cmdPieces, err)
			}
		}
	}

	return problems, nil
}
