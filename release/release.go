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

// The msi msiUpgradeCode must be updated when the minor version changes.
var msiUpgradeCode string = "56c0fda6-289a-4fd0-a539-6711864146ba"

// These are the binaries that are part of mongo-tools, relative
// to the location of this go file.
var binariesPath string = filepath.Join("..", "bin")
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

// These are the meta-text files that are part of mongo-tools, relative
// to the location of this go file.
var staticFilesPath string = ".."
var staticFiles = []string{
	"LICENSE.md",
	"README.md",
	"THIRD-PARTY-NOTICES",
}

// note that the os.Link function does not allow for drive letters on Windows, absolute paths
// must be specified with a leading os.PathSeparator.
var saslDLLsPath string = string(os.PathSeparator) + filepath.Join("sasl", "bin")
var saslDLLs = []string{
	"libsasl.dll",
}

// location of the necessary data files to build the msi.
var msiFilesPath string = filepath.Join("..", "installer", "msi")
var msiFiles = []string{
	"Banner_Tools.bmp",
	"BinaryFragment.wxs",
	"Dialog.bmp",
	"Dialog_Tools.bmp",
	"FeatureFragment.wxs",
	"Installer_Icon_16x16.ico",
	"Installer_Icon_32x32.ico",
	"LicensingFragment.wxs",
	"Product.wxs",
	"UIFragment.wxs",
}

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
	case "build-msi":
		err := buildMSI()
		if err != nil {
			log.Fatalf("%v", err)
		}
	case "build-rpm":
		log.Fatal("not implemented")
	case "build-deb":
		log.Fatal("not implemented")
	case "build-linux":
		log.Fatal("not implemented")
	default:
		log.Fatalf("unknown subcommand '%s'", cmd)
	}
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

func buildMSI() error {
	win, err := platform.IsWindows()
	check(err, "check platform type")
	if !win {
		return nil
	}
	log.Printf("building msi installer\n")

	// set up build directory.
	msiBuildDir := "msi_build"
	os.RemoveAll(msiBuildDir)
	os.MkdirAll(msiBuildDir, os.ModePerm)
	os.Chdir(msiBuildDir)
	oldCwd, err := os.Getwd()
	if err != nil {
		return err
	}
	// we'll want to go back to the original directory, just in case.
	defer os.Chdir(oldCwd)

	// Copy sasldlls. They need to be in this directory for Wix. Linking will
	// not work as the dlls are on a different file system.
	for _, name := range saslDLLs{
		err := copyFile(
			filepath.Join(saslDLLsPath, name),
			name,
		)
		if err != nil {
			return err
		}
	}

	// make links to all the staticFiles. They need to be in this
	// directory for Wix.
	for _, name := range staticFiles {
		err := os.Link(
			filepath.Join(staticFilesPath, name),
			name,
		)
		if err != nil {
			return err
		}
	}

	for _, name := range msiFiles {
		err := os.Link(
			filepath.Join(msiFilesPath, name),
			name,
		)
		if err != nil {
			return err
		}
	}

	for _, name := range binaries {
		err := os.Link(
			filepath.Join(binariesPath, name) + ".exe",
			name + ".exe",
		)
		if err != nil {
			return err
		}
	}

	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	// Wix requires the directories to end with a separator.
	cwd += string(os.PathSeparator)
	wixPath := string(os.PathSeparator) + filepath.Join("wixtools", "bin")
	wixUIExtPath := filepath.Join(wixPath, "WixUIExtension.dll")
	projectName := "MongoDB Tools"
	sourceDir := cwd
	resourceDir := cwd
	binDir := cwd
	objDir := filepath.Join(cwd, "objs") + string(os.PathSeparator)
	arch := "x64"

	version := getVersion()

	if version > "r49.0" {
		return fmt.Errorf("msiUpgradeCode in release.go must be updated")
	}

	candle := filepath.Join(wixPath, "candle.exe")
	out, err := run(candle,
		"-wx",
		`-dProductId=*`,
		`-dPlatform=x64`,
		`-dUpgradeCode=`+msiUpgradeCode,
		`-dVersion=49.0.0`,
		`-dVersionLabel=`+version,
		`-dProjectName=`+projectName,
		`-dSourceDir=`+sourceDir,
		`-dResourceDir=`+resourceDir,
		`-dSslDir=`+binDir,
		`-dBinaryDir=`+binDir,
		`-dTargetDir=`+objDir,
		`-dTargetExt=".msi"`,
		`-dTargetFileName="release"`,
		`-dOutDir=`+objDir,
		`-dConfiguration="Release"`,
		`-arch`, arch,
		`-out`, objDir,
		`-ext`, wixUIExtPath,
		`Product.wxs`,
		`FeatureFragment.wxs`,
		`BinaryFragment.wxs`,
		`LicensingFragment.wxs`,
		`UIFragment.wxs`,
	)

	if err != nil {
		log.Fatalf("%v", out)
		return err
	}

	output := "mongodb-cli-tools-" + version + "-win-x86-64.msi"
	light := filepath.Join(wixPath, "light.exe")
	out, err = run(light,
		"-wx",
		`-cultures:en-us`,
		`-out`, output,
		`-ext`, wixUIExtPath,
		filepath.Join(objDir, `Product.wixobj`),
		filepath.Join(objDir, `FeatureFragment.wixobj`),
		filepath.Join(objDir, `BinaryFragment.wixobj`),
		filepath.Join(objDir, `LicensingFragment.wixobj`),
		filepath.Join(objDir, `UIFragment.wixobj`),
	)
	if err != nil {
		log.Fatalf("%v", out)
		return err
	}

	// Copy to top level directory so we can upload it.
	os.Link(
		output,
		filepath.Join("..", output),
	)
	return nil
}

func copyFile(src, dst string) error {
	file, err := os.Open(src)
	check(err, "open src")
	defer file.Close()

	out, err := os.Create(dst)
	check(err, "create dst")
	defer out.Close()

	_, err = io.Copy(out, file)
	check(err, "copy src -> dst")
	return out.Close()
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