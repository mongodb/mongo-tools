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

// These are the binaries that are part of mongo-tools, relative
// to the location of this go file.
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
		buildMSI()
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

func buildMSI() {
	win, err := platform.IsWindows()
	check(err, "check platform type")
	if !win {
		return
	}

	// The msi msiUpgradeCode must be updated when the minor version changes.
	msiUpgradeCode := "56c0fda6-289a-4fd0-a539-6711864146ba"

	binariesPath := filepath.Join("..", "bin")
	msiStaticFilesPath := ".."
	// Note that the file functions do not allow for drive letters on Windows, absolute paths
	// must be specified with a leading os.PathSeparator.
	saslDLLsPath := string(os.PathSeparator) + filepath.Join("sasl", "bin")
	msiFilesPath := filepath.Join("..", "installer", "msi")

	// These are the meta-text files that are part of mongo-tools, relative
	// to the location of this go file. We have to use an rtf verison of the
	// license, so we do not include the static files.
	var msiStaticFiles = []string{
		"README.md",
		"THIRD-PARTY-NOTICES",
	}

	var saslDLLs = []string{
		"libsasl.dll",
	}

	// location of the necessary data files to build the msi.
	var msiFiles = []string{
		"Banner_Tools.bmp",
		"BinaryFragment.wxs",
		"Dialog.bmp",
		"Dialog_Tools.bmp",
		"FeatureFragment.wxs",
		"Installer_Icon_16x16.ico",
		"Installer_Icon_32x32.ico",
		"LICENSE.rtf",
		"LicensingFragment.wxs",
		"Product.wxs",
		"UIFragment.wxs",
	}

	log.Printf("building msi installer\n")

	// set up build directory.
	msiBuildDir := "msi_build"
	check(os.RemoveAll(msiBuildDir), "removeAll "+msiBuildDir)
	check(os.MkdirAll(msiBuildDir, os.ModePerm), "mkdirAll "+msiBuildDir)
	check(os.Chdir(msiBuildDir), "cd to "+msiBuildDir)
	oldCwd, err := os.Getwd()
	// we'll want to go back to the original directory, just in case.
	defer os.Chdir(oldCwd)
	check(err, "get current directory")

	// Copy sasldlls. They need to be in this directory for Wix. Linking will
	// not work as the dlls are on a different file system.
	for _, name := range saslDLLs {
		err := copyFile(
			filepath.Join(saslDLLsPath, name),
			name,
		)
		check(err, "copy sasl dlls into "+msiBuildDir)
	}

	// make links to all the staticFiles. They need to be in this
	// directory for Wix.
	for _, name := range msiStaticFiles {
		err := os.Link(
			filepath.Join(msiStaticFilesPath, name),
			name,
		)
		check(err, "link msi static files into "+msiBuildDir)
	}

	for _, name := range msiFiles {
		err := os.Link(
			filepath.Join(msiFilesPath, name),
			name,
		)
		check(err, "link msi creation files into "+msiBuildDir)
	}

	for _, name := range binaries {
		err := os.Link(
			filepath.Join(binariesPath, name),
			name+".exe",
		)
		check(err, "link binary files into "+msiBuildDir)
	}

	// Wix requires the directories to end with a separator.
	cwd, err := os.Getwd()
	check(err, "getwd")
	cwd += "\\"
	wixPath := string(os.PathSeparator) + filepath.Join("wixtools", "bin")
	wixUIExtPath := filepath.Join(wixPath, "WixUIExtension.dll")
	projectName := "MongoDB Tools"
	sourceDir := cwd
	resourceDir := cwd
	binDir := cwd
	objDir := filepath.Join(cwd, "objs") + string(os.PathSeparator)
	arch := "x64"

	release := getVersion()
	wixVersion := getWixVersion(release)
	versionLabel := getVersionLabel(release)

	lastVersionLabel := "49"
	if versionLabel > lastVersionLabel {
		check(fmt.Errorf("msiUpgradeCode in release.go must be updated"), "msiUpgradeCode should be up-to-date, last version = "+lastVersionLabel)
	}

	candle := filepath.Join(wixPath, "candle.exe")
	out, err := run(candle,
		"-wx",
		`-dProductId=*`,
		`-dPlatform=x64`,
		`-dUpgradeCode=`+msiUpgradeCode,
		`-dVersion=`+wixVersion,
		`-dVersionLabel=`+versionLabel,
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

	check(err, "run candle.exe\n"+out)

	output := getReleaseName() + ".msi"
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
	check(err, "run light.exe\n"+out)

	// Copy to top level directory so we can upload it.
	check(os.Link(
		output,
		filepath.Join("..", output),
	), "linking output for s3 upload")
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

func getWixVersion(version string) string {
	// r49.3.2-39-g7f57f9a2 will be turned to 49.3.2
	rLabel := strings.Split(version, "-")[0]
	if rLabel[0] == 'r' {
		return rLabel[1:]
	}
	return rLabel
}

func getVersionLabel(version string) string {
	// r49.3.2-39-g7f57f9a2 will be turned to 49
	rLabel := strings.Split(version, ".")[0]
	if rLabel[0] == 'r' {
		return rLabel[1:]
	}
	return rLabel
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
		log.Printf("adding %s binary to zip\n", binName)
		src := filepath.Join(".", "bin", binName)
		dst := filepath.Join(releaseName, "bin", binName + ".exe")
		addToZip(zw, dst, src)
	}
}
