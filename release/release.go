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

// The wix upgradeCode must be updated when the minor version changes.
var upgradeCode string = "56c0fda6-289a-4fd0-a539-6711864146ba"

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

var opensslDLLs = []string{
	"ssleay.dll",
	"libeay.dll",
}

var msiFiles = []string {
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

func buildPath(parts ...string) string {
	return strings.Join(parts, string(os.PathSeparator))
}

func buildMSI() error {
	win, err := platform.IsWindows()
	check(err, "check platform type")
	if !win {
		return nil
	}

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

	// make links to opensslDLLs. They need to be in this directory for Wix.
	for _, name := range opensslDLLs {
		os.Link(
			buildPath("C:", "openssl", "bin", name),
			name,
		)
	}

	// make links to all the staticFiles. They need to be in this
	// directory for Wix.
	for _, name := range staticFiles {
		os.Link(
			buildPath("..", name),
			name,
		)
	}

	for _, name := range msiFiles {
		os.Link(
			buildPath("..", "installer", "msi", name),
			name,
		)
	}

	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
    wixPath := buildPath("C:", "wixtools", "bin")
	wisUiExtPath := buildPath(wixPath, "WixUIExtension.dll")
	projectName := "MongoDB Tools"
	sourceDir := cwd
	resourceDir := cwd
	binDir := cwd
	objDir := buildPath(cwd, "objs")

	version := getVersion()

	if version > "r49.0" {
		return fmt.Errorf("upgradeCode in release.go must be updated")
	}

//# upgrade code needs to change everytime we
//# rev the minor version (1.0 -> 1.1). That way, we
//# will allow multiple minor versions to be installed
//# side-by-side.
//if ([double]$version -gt 49.0) {
//    throw "You must change the upgrade code for a minor revision.
//Once that is done, change the version number above to
//account for the next revision that will require being
//upgradeable. Make sure to change both x64 and x86 upgradeCode"
//}
//
//$upgradeCode = 
//$Arch = "x64"
//
//# compile wxs into .wixobjs
//& $WixPath\candle.exe -wx `
//    -dProductId="*" `
//    -dPlatform="$Arch" `
//    -dUpgradeCode="$upgradeCode" `
//    -dVersion="$version" `
//    -dVersionLabel="$VersionLabel" `
//    -dProjectName="$ProjectName" `
//    -dSourceDir="$sourceDir" `
//    -dResourceDir="$resourceDir" `
//    -dSslDir="$binDir" `
//    -dBinaryDir="$binDir" `
//    -dTargetDir="$objDir" `
//    -dTargetExt=".msi" `
//    -dTargetFileName="release" `
//    -dOutDir="$objDir" `
//    -dConfiguration="Release" `
//    -arch "$Arch" `
//    -out "$objDir" `
//    -ext "$wixUiExt" `
//    "$resourceDir\Product.wxs" `
//    "$resourceDir\FeatureFragment.wxs" `
//    "$resourceDir\BinaryFragment.wxs" `
//    "$resourceDir\LicensingFragment.wxs" `
//    "$resourceDir\UIFragment.wxs"
//
//if(-not $?) {
//    exit 1
//}
//
//$artifactsDir = pwd
//
//# link wixobjs into an msi
//& $WixPath\light.exe -wx `
//    -cultures:en-us `
//    -out "$artifactsDir\mongodb-tools-$VersionLabel-win-x86-64.msi" `
//    -ext "$wixUiExt" `
//    $objDir\Product.wixobj `
//    $objDir\FeatureFragment.wixobj `
//    $objDir\BinaryFragment.wixobj `
//    $objDir\LicensingFragment.wixobj `
//    $objDir\UIFragment.wixobj
	return nil
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
