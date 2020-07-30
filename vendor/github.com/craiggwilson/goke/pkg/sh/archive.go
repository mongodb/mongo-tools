package sh

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/craiggwilson/goke/task"
)

// Archive will create an archive from the src file or directory and use the destination's
// extension to determine which format to use.
func Archive(ctx *task.Context, src, dest string) error {
	if strings.HasSuffix(dest, ".zip") {
		return ArchiveZip(ctx, src, dest)
	} else if strings.HasSuffix(dest, ".tgz") || strings.HasSuffix(dest, ".tar.gz") {
		return ArchiveTGZ(ctx, src, dest)
	}

	return errors.New("unable to determine archive format")
}

// ArchiveTGZ will archive using tar and gzip to the destination.
func ArchiveTGZ(ctx *task.Context, src, dest string) error {
	ctx.Logf("tgz: %s -> %s\n", src, dest)

	src, err := filepath.Abs(src)
	if err != nil {
		return err
	}

	dest, err = filepath.Abs(dest)
	if err != nil {
		return err
	}

	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()

	gw := gzip.NewWriter(f)
	defer gw.Close()

	tw := tar.NewWriter(gw)
	defer tw.Close()

	srcFileInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	var baseDir string
	if srcFileInfo.IsDir() {
		baseDir = filepath.Base(src)
	}

	return filepath.Walk(src, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if baseDir == path {
			return nil
		}

		header, err := tar.FileInfoHeader(fi, fi.Name())
		if err != nil {
			return err
		}

		if baseDir != "" {
			header.Name, _ = filepath.Rel("/", strings.TrimPrefix(path, src))
		}

		// If this 'file' is the same as the base directory we're traversing, ignore it.
		if header.Name == "" {
			return nil
		}

		if err = tw.WriteHeader(header); err != nil {
			return err
		}

		if fi.IsDir() {
			return nil
		}

		srcFile, err := os.Open(path)
		if err != nil {
			return err
		}
		defer srcFile.Close()

		_, err = io.Copy(tw, srcFile)
		return err
	})
}

// ArchiveZip will zip the src into a a zipped file at the destination.
func ArchiveZip(ctx *task.Context, src, dest string) error {
	ctx.Logf("zip: %s -> %s\n", src, dest)

	src, err := filepath.Abs(src)
	if err != nil {
		return err
	}

	dest, err = filepath.Abs(dest)
	if err != nil {
		return err
	}

	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()

	zw := zip.NewWriter(f)
	defer zw.Close()

	srcFileInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	var baseDir string
	if srcFileInfo.IsDir() {
		baseDir = filepath.Base(src)
	}

	return filepath.Walk(src, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		header, err := zip.FileInfoHeader(fi)
		if err != nil {
			return err
		}

		if baseDir != "" {
			header.Name, _ = filepath.Rel("/", strings.TrimPrefix(path, src))
		}

		if fi.IsDir() {
			header.Name += "/"
		} else {
			header.Method = zip.Deflate
		}

		destFile, err := zw.CreateHeader(header)
		if err != nil {
			return err
		}

		if fi.IsDir() {
			return nil
		}

		srcFile, err := os.Open(path)
		if err != nil {
			return err
		}
		defer srcFile.Close()

		_, err = io.Copy(destFile, srcFile)
		return err
	})
}

// Unarchive decompresses the archive according to the source's extension.
func Unarchive(ctx *task.Context, src, dest string) error {
	if strings.HasSuffix(src, ".zip") {
		return UnarchiveZip(ctx, src, dest)
	} else if strings.HasSuffix(src, ".tgz") || strings.HasSuffix(src, ".tar.gz") {
		return UnarchiveTGZ(ctx, src, dest)
	}

	return errors.New("unable to determine archive format")
}

// UnarchiveTGZ decompresses the src tgz file into the destination.
func UnarchiveTGZ(ctx *task.Context, src, dest string) error {
	ctx.Logf("untgz: %s -> %s\n", src, dest)

	src, err := filepath.Abs(src)
	if err != nil {
		return err
	}

	dest, err = filepath.Abs(dest)
	if err != nil {
		return err
	}

	r, err := os.Open(src)
	if err != nil {
		return err
	}
	defer r.Close()

	gr, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	for {
		header, err := tr.Next()
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		path := filepath.Join(dest, header.Name)
		fi := header.FileInfo()
		if fi.IsDir() {
			if err = os.MkdirAll(path, fi.Mode()); err != nil {
				return err
			}
			continue
		}

		_, err = os.Stat(filepath.Dir(path))
		if err != nil && os.IsNotExist(err) {
			if err = os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				return err
			}
		}

		destFile, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, fi.Mode())
		if err != nil {
			return err
		}
		defer destFile.Close()

		if _, err = io.Copy(destFile, tr); err != nil {
			return err
		}
	}

	return nil
}

// UnarchiveZip decompresses the src zip file into the destination.
func UnarchiveZip(ctx *task.Context, src, dest string) error {
	ctx.Logf("unzip: %s -> %s\n", src, dest)

	src, err := filepath.Abs(src)
	if err != nil {
		return err
	}

	dest, err = filepath.Abs(dest)
	if err != nil {
		return err
	}

	zr, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer zr.Close()

	if err = os.MkdirAll(dest, 0755); err != nil {
		return err
	}

	for _, file := range zr.File {
		path := filepath.Join(dest, file.Name)
		if file.FileInfo().IsDir() {
			if err = os.MkdirAll(path, file.Mode()); err != nil {
				return err
			}
			continue
		}

		srcFile, err := file.Open()
		if err != nil {
			return err
		}

		defer srcFile.Close()

		_, err = os.Stat(filepath.Dir(path))
		if err != nil && os.IsNotExist(err) {
			if err = os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				return err
			}
		}

		destFile, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, file.Mode())
		if err != nil {
			return err
		}
		defer destFile.Close()

		if _, err = io.Copy(destFile, srcFile); err != nil {
			return err
		}
	}

	return nil
}
