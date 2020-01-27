package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/mongodb/mongo-tools/release/platform"
)

func main() {
	// don't prefix log messages with anything
	log.SetFlags(0)

	if len(os.Args) != 2 {
		log.Fatal("please provide exactly one subcommand name")
	}
	cmd := os.Args[1]

	switch cmd {
	case "build-archive":
		buildArchive()
	case "build-installer":
		log.Fatal("not implemented")
	case "build-linux":
		log.Fatal("not implemented")
	default:
		log.Fatalf("unknown subcommand '%s'", cmd)
	}
}

var binaries = []string{
	"bsondump",
	"mongodump",
	"mongoexport",
	"mongofiles",
	"mongoimport",
	"mongorestore",
	"mongostat",
	"mongotop",
}

var staticFiles = []string{
	"LICENSE.md",
	"README.md",
	"THIRD-PARTY-NOTICES",
}

func check(err error, format ...interface{}) {
	if err == nil {
		return
	}
	msg := err.Error()
	if len(format) != 0 {
		task := fmt.Sprintf(format[0].(string), format[1:]...)
		msg = fmt.Sprintf("'%s' failed: %v", task, err)
	}
	log.Fatal(msg)
}

func run(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}

func getVersion() string {
	desc, err := run("git", "describe")
	check(err, "git describe")
	return desc
}

func getReleaseName() string {
	p, err := platform.Get()
	check(err, "get platform")
	version := getVersion()

	return fmt.Sprintf(
		"mongodb-cli-tools-%s-%s-%s",
		p.Name, p.Arch, version,
	)
}

func buildArchive() {
	win, err := platform.IsWindows()
	check(err, "check platform type")
	if win {
		buildZip()
	} else {
		buildTarball()
	}
}

func addToTarball(tw *tar.Writer, dst, src string) {
	file, err := os.Open(src)
	check(err, "open file")
	defer file.Close()

	stat, err := file.Stat()
	check(err, "stat file")

	header := &tar.Header{
		Name: dst,
		Size: stat.Size(),
		Mode: 0755,
	}

	err = tw.WriteHeader(header)
	check(err, "write header to archive")

	_, err = io.Copy(tw, file)
	check(err, "write file to archive")
}

func buildTarball() {
	log.Printf("building tarball archive\n")

	releaseName := getReleaseName()
	archiveFile, err := os.Create(releaseName + ".tgz")
	check(err, "create archive file")
	defer archiveFile.Close()

	gw := gzip.NewWriter(archiveFile)
	defer gw.Close()

	tw := tar.NewWriter(gw)
	defer tw.Close()

	for _, name := range staticFiles {
		log.Printf("adding %s to tarball\n", name)
		src := name
		dst := filepath.Join(releaseName, name)
		addToTarball(tw, dst, src)
	}

	for _, binName := range binaries {
		log.Printf("adding %s binary to tarball\n", binName)
		src := filepath.Join("bin", binName)
		dst := filepath.Join(releaseName, "bin", binName)
		addToTarball(tw, dst, src)
	}
}

func addToZip(zw *zip.Writer, dst, src string) {
	file, err := os.Open(src)
	check(err, "open file")
	defer file.Close()

	stat, err := file.Stat()
	check(err, "stat file")

	header, err := zip.FileInfoHeader(stat)
	check(err, "construct zip header from stat")
	header.Name = dst
	header.Method = 8

	fw, err := zw.CreateHeader(header)
	check(err, "create header")

	_, err = io.Copy(fw, file)
	check(err, "write file to zip")
}

func buildZip() {
	log.Printf("building zip archive\n")

	releaseName := getReleaseName()
	archiveFile, err := os.Create(releaseName + ".zip")
	check(err, "create archive file")
	defer archiveFile.Close()

	zw := zip.NewWriter(archiveFile)
	defer zw.Close()

	for _, name := range staticFiles {
		log.Printf("adding %s to zip\n", name)
		src := name
		dst := filepath.Join(releaseName, name)
		addToZip(zw, dst, src)
	}

	for _, binName := range binaries {
		binName = binName + ".exe"
		log.Printf("adding %s binary to zip\n", binName)
		src := filepath.Join(".", "bin", binName)
		dst := filepath.Join(releaseName, "bin", binName)
		addToZip(zw, dst, src)
	}
}
