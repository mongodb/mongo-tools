package main

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/mongodb/mongo-tools/release/aws"
	"github.com/mongodb/mongo-tools/release/download"
	"github.com/mongodb/mongo-tools/release/env"
	"github.com/mongodb/mongo-tools/release/evergreen"
	"github.com/mongodb/mongo-tools/release/platform"
	"github.com/mongodb/mongo-tools/release/version"
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

	var cmd string
	var v version.Version
	var err error

	switch len(os.Args) {
	case 1:
		log.Fatal("please provide a subcommand")
	case 2:
		cmd = os.Args[1]
		v, err = version.GetCurrent()
		if err != nil {
			log.Fatalf("failed to get version: %v", err)
		}

	case 3:
		cmd = os.Args[1]
		v, err = version.GetFromRev(os.Args[2])
		if err != nil {
			log.Fatalf("failed to get version: %v", err)
		}
	default:
		log.Fatalf("expected one or two arguments, got %d", len(os.Args))
	}

	switch cmd {
	case "build-archive":
		buildArchive()
	case "build-packages":
		buildMSI()
		buildLinuxPackages()
	case "get-version":
		fmt.Println(v)
	case "list-deps":
		listLinuxDeps()
	case "upload-release":
		uploadRelease(v)
	case "upload-json":
		uploadReleaseJSON(v)
	case "generate-full-json":
		generateFullReleaseJSON(v)
	case "linux-release":
		linuxRelease(v)
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
	if err != nil {
		if exerr, ok := err.(*exec.ExitError); ok {
			err = fmt.Errorf("ExitError: %v. Stderr: %q", err, string(exerr.Stderr))
		}
	}
	return strings.TrimSpace(string(out)), err
}

func streamOutput(prefix string, reader io.Reader) {
	log := func(txt string) {
		log.Printf("[%s] %s\n", prefix, txt)
	}

	scanner := bufio.NewScanner(reader)
	for {
		hasNext := scanner.Scan()
		if !hasNext {
			err := scanner.Err()
			log("DONE")
			if err != nil {
				log("streaming error: " + err.Error())
			}
			return
		}
		txt := scanner.Text()
		log(txt)
	}
}

func runAndStreamStderr(logPrefix string, name string, args ...string) error {
	cmd := exec.Command(name, args...)

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	err = cmd.Start()
	if err != nil {
		return err
	}

	streamOutput(logPrefix, stderrPipe)

	return cmd.Wait()
}

func isTaggedRelease(rev string) bool {
	_, err := run("git", "describe", "--exact", rev)
	return err == nil
}

func getReleaseName() string {
	p, err := platform.GetFromEnv()
	check(err, "get platform")

	v, err := version.GetCurrent()
	check(err, "get version")

	return fmt.Sprintf(
		"mongodb-database-tools-%s-%s-%s",
		p.Name, p.Arch, v,
	)
}

func getDebFileName() string {
	p, err := platform.GetFromEnv()
	check(err, "get platform")

	v, err := version.GetCurrent()
	check(err, "get version")

	vStr := fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
	if v.Pre != "" {
		vStr += "~latest"
	}

	return fmt.Sprintf(
		"mongodb-database-tools_%s_%s.deb",
		vStr, p.DebianArch(),
	)
}

func getRPMFileName() string {
	p, err := platform.GetFromEnv()
	check(err, "get platform")

	v, err := version.GetCurrent()
	check(err, "get version")

	vStr := fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
	if v.Pre != "" {
		vStr += ".latest"
	}

	return fmt.Sprintf(
		"mongodb-database-tools-%s.%s.rpm",
		vStr, p.RPMArch(),
	)
}

func buildArchive() {
	pf, err := platform.GetFromEnv()
	check(err, "get platform")
	if pf.OS == platform.OSWindows {
		buildZip()
	} else {
		buildTarball()
	}
}

func listLinuxDeps() {
	pf, err := platform.GetFromEnv()
	check(err, "get platform")

	if pf.OS != platform.OSLinux {
		return
	}

	check(err, "get platform")
	libraryPaths := getLibraryPaths()
	deps := make(map[string]struct{})

	switch pf.Pkg {
	case platform.PkgRPM:
		for _, libPath := range libraryPaths {
			out, err := run("rpm", "-q", "--whatprovides", libPath)
			check(err, "rpm -q --whatprovides "+libPath+": "+out)
			deps[strings.Trim(out, " \t\n")] = struct{}{}
		}
	case platform.PkgDeb:
		for _, libPath := range libraryPaths {
			out, err := run("dpkg", "-S", libPath)
			check(err, "dpkg -S "+libPath+": "+out)
			sp := strings.Split(out, ":")
			deps[strings.Trim(sp[0], " \t\n")] = struct{}{}
		}
	default:
		log.Fatalf("linux platform %q is neither deb nor rpm based", pf.Name)
	}

	orderedDeps := make([]string, 0, len(deps))
	for dep := range deps {
		orderedDeps = append(orderedDeps, dep)
	}
	sort.Strings(orderedDeps)
	for _, dep := range orderedDeps {
		log.Printf("%s\n", dep)
	}
}

func getLibraryPaths() []string {
	out, err := run("ldd", filepath.Join("bin", "mongodump"))
	check(err, "ldd\n"+out)

	ret := []string{}
	for _, line := range strings.Split(out, "\n") {
		sp := strings.Split(line, "=>")
		if len(sp) < 2 {
			continue
		}
		sp = strings.Split(sp[1], "(")
		libPath := strings.Trim(sp[0], " \t")
		if libPath != "" {
			ret = append(ret, libPath)
		}
	}
	return ret
}

func buildLinuxPackages() {
	pf, err := platform.GetFromEnv()
	check(err, "get platform")
	if pf.OS != platform.OSLinux {
		return
	}

	switch pf.Pkg {
	case platform.PkgRPM:
		buildRPM()
	case platform.PkgDeb:
		buildDeb()
	default:
		log.Fatalf("found linux platform with no Pkg value: %+v", pf)
	}
}

func buildRPM() {
	mdt := "mongodb-database-tools"
	home := os.Getenv("HOME")

	// set up build working directory.
	cdBack := useWorkingDir("rpm_build")
	// we'll want to go back to the original directory, just in case.
	defer cdBack()

	// The goal here is to set up  directory with the following structure:
	// rpmbuild/
	// |----- SOURCES/
	// |         |----- mongodb-database-tools.tar.gz:
	//                       |
	//                      mongodb-database-tools/
	//                               |------ usr/
	//                               |-- bin/
	//                               |    |--- bsondump
	//                               |    |--- mongo*
	//                               |-- share/
	//                                      |---- doc/
	//                                             |----- mongodb-database-tools/
	//                                                              |--- staticFiles

	// create tar file
	log.Printf("tarring necessary files\n")
	createTar := func() {
		staticFilesPath := ".."
		binariesPath := filepath.Join("..", "bin")
		sources := filepath.Join(home, "rpmbuild", "SOURCES")
		check(os.MkdirAll(sources, os.ModePerm), "create "+sources)
		archiveFile, err := os.Create(filepath.Join(sources, mdt+".tar.gz"))
		check(err, "create archive file")
		defer archiveFile.Close()

		gw := gzip.NewWriter(archiveFile)
		defer gw.Close()

		tw := tar.NewWriter(gw)
		defer tw.Close()

		for _, name := range staticFiles {
			log.Printf("adding %s to tarball\n", name)
			src := filepath.Join(staticFilesPath, name)
			dst := filepath.Join(mdt, "usr", "share", "doc", mdt, name)
			addToTarball(tw, dst, src)
		}

		for _, name := range binaries {
			log.Printf("adding %s to tarball\n", name)
			src := filepath.Join(binariesPath, name)
			dst := filepath.Join(mdt, "usr", "bin", name)
			addToTarball(tw, dst, src)
		}
	}
	createTar()

	pf, err := platform.GetFromEnv()
	check(err, "get platform")
	specFile := mdt + ".spec"

	v, err := version.GetCurrent()
	check(err, "get version")

	rpmVersion := v.StringWithoutPre()
	rpmRelease := v.RPMRelease()

	createSpecFile := func() {
		log.Printf("create spec file\n")
		f, err := os.Create(specFile)
		check(err, "create spec")
		defer f.Close()

		// get the control file content.
		contentBytes, err := ioutil.ReadFile(filepath.Join("..", "installer", "rpm", specFile))
		content := string(contentBytes)
		check(err, "reading spec file content")
		content = strings.Replace(content, "@TOOLS_VERSION@", rpmVersion, -1)
		content = strings.Replace(content, "@TOOLS_RELEASE@", rpmRelease, -1)
		content = strings.Replace(content, "@ARCHITECTURE@", pf.RPMArch(), -1)
		_, err = f.WriteString(content)
		check(err, "write content to spec file")
	}
	createSpecFile()

	outputFile := mdt + "-" + rpmVersion + "-" + rpmRelease + "." + pf.RPMArch() + ".rpm"
	outputPath := filepath.Join(home, "rpmbuild", "RPMS", outputFile)

	// ensure that the _topdir macro used by rpmbuild references a writeable location
	topdirDefine := "_topdir " + filepath.Join(home, "rpmbuild")

	// create the .rpm file.
	log.Printf("running: rpmbuild -bb %s\n", specFile)
	out, err := run("rpmbuild", "--define", topdirDefine, "-bb", specFile)
	check(err, "rpmbuild\n"+out)
	// Copy to top level directory so we can upload it.
	check(copyFile(
		outputPath,
		filepath.Join("..", getRPMFileName()),
	), "linking output for s3 upload")
}

func buildDeb() {
	pf, err := platform.GetFromEnv()
	check(err, "get platform")

	mdt := "mongodb-database-tools"
	releaseName := getReleaseName()

	// set up build working directory.
	cdBack := useWorkingDir("deb_build")
	// we'll want to go back to the original directory, just in case.
	defer cdBack()

	// The goal here is to set up  directory with the following structure:
	// releaseName/
	// |----- DEBIAN/
	// |        |----- control
	// |        |----- postinst
	// |        |----- prerm
	// |        |----- md5sums
	// |------ usr/
	//          |-- bin/
	//          |    |--- bsondump
	//          |    |--- mongo*
	//          |-- share/
	//                 |---- doc/
	//                        |----- mongodb-database-tools/
	//                                         |--- staticFiles

	log.Printf("create deb directory tree\n")

	// create DEBIAN dir
	controlDir := filepath.Join(releaseName, "DEBIAN")
	check(os.MkdirAll(controlDir, os.ModePerm), "mkdirAll "+controlDir)

	// create usr/bin and usr/share/doc
	binDir := filepath.Join(releaseName, "usr", "bin")
	check(os.MkdirAll(binDir, os.ModePerm), "mkdirAll "+binDir)
	docDir := filepath.Join(releaseName, "usr", "share", "doc", mdt)
	check(os.MkdirAll(docDir, os.ModePerm), "mkdirAll "+docDir)

	md5sums := make(map[string]string)
	// We use the order just to make sure the md5sums are always in the same order.
	// This probably doesn't matter, but it looks nicer for anyone inspecting the md5sums file.
	md5sumsOrder := make([]string, 0, len(binaries)+len(staticFiles))
	logCopy := func(src, dst string) {
		log.Printf("copying %s to %s\n", src, dst)
	}
	// Copy over the data files.
	{
		binariesPath := filepath.Join("..", "bin")
		// Add binaries.
		for _, binName := range binaries {
			src := filepath.Join(binariesPath, binName)
			dst := filepath.Join(binDir, binName)
			logCopy(src, dst)
			check(os.Link(src, dst), "link file")
			md5sums[dst] = computeMD5(src)
			md5sumsOrder = append(md5sumsOrder, dst)
		}
		// Add static files.
		for _, file := range staticFiles {
			src := filepath.Join("..", file)
			dst := filepath.Join(docDir, file)
			logCopy(src, dst)
			check(os.Link(src, dst), "link file")
			md5sums[dst] = computeMD5(src)
			md5sumsOrder = append(md5sumsOrder, dst)
		}
	}

	controlFile := "control"
	createControlFile := func() {
		f, err := os.Create(controlFile)
		check(err, "create control")
		defer f.Close()

		v, err := version.GetCurrent()
		check(err, "get version")

		// get the control file content.
		contentBytes, err := ioutil.ReadFile(filepath.Join("..", "installer", "deb", "control"))
		content := string(contentBytes)
		check(err, "reading control file content")
		content = strings.Replace(content, "@TOOLS_VERSION@", v.String(), -1)

		content = strings.Replace(content, "@ARCHITECTURE@", pf.DebianArch(), 1)
		_, err = f.WriteString(content)
		check(err, "write content to control file")
	}
	createControlFile()

	md5sumsFile := "md5sums"
	createMD5Sums := func() {
		f, err := os.Create(md5sumsFile)
		check(err, "create md5sums")
		defer f.Close()
		os.Chmod(md5sumsFile, 0644)
		// create the md5sums file.
		for _, path := range md5sumsOrder {
			md5sum, ok := md5sums[path]
			if !ok {
				log.Fatalf("could not find md5sum for " + path)
			}
			_, err = f.WriteString(md5sum + " ")
			check(err, "write md5sum to md5sums")
			_, err = f.WriteString(path + "\n")
			check(err, "write path to md5sums")
		}
	}
	createMD5Sums()

	// Copy the control files to our controlDir
	// control -- metadata
	// md5sums (optional) -- sums for all files
	// postinst (optional) -- post install script, we don't need this
	// prerm (optional) -- removing old documentation
	{
		staticControlFiles := []string{
			"postinst",
			"prerm",
		}
		// add the control file.
		dst := filepath.Join(controlDir, controlFile)
		logCopy(controlFile, dst)
		check(os.Link(controlFile, dst), "link file")

		// add the md5sumsFile.
		dst = filepath.Join(controlDir, md5sumsFile)
		logCopy(md5sumsFile, dst)
		check(os.Link(md5sumsFile, dst), "link file")

		// add the static control files.
		for _, file := range staticControlFiles {
			// add the static control files.
			src := filepath.Join("..", "installer", "deb", file)
			dst = filepath.Join(controlDir, file)
			logCopy(src, dst)
			check(os.Link(src, dst), "link file")
		}
	}

	output := releaseName + ".deb"
	// create the .deb file.
	log.Printf("running: dpkg -D1 -b %s %s", releaseName, output)
	out, err := run("dpkg", "-D1", "-b", releaseName, output)
	check(err, "run dpkg\n"+out)
	// Copy to top level directory so we can upload it.
	check(os.Link(
		output,
		filepath.Join("..", getDebFileName()),
	), "linking output for s3 upload")
}

func buildMSI() {
	pf, err := platform.GetFromEnv()
	check(err, "get platform")
	if pf.OS != platform.OSWindows {
		return
	}

	// The msi msiUpgradeCode must be updated when the major version changes.
	msiUpgradeCode := "effc2f80-8f82-413f-a3ba-4a96f3d2883a"

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

	// set up build working directory.
	msiBuildDir := "msi_build"
	cdBack := useWorkingDir(msiBuildDir)
	// we'll want to go back to the original directory, just in case.
	defer cdBack()

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
			filepath.Join(binariesPath, name+".exe"),
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

	v, err := version.GetCurrent()
	check(err, "get version")

	wixVersion := fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
	versionLabel := fmt.Sprintf("%d", v.Major)

	currentVersionLabel := "100"
	if versionLabel != currentVersionLabel {
		check(fmt.Errorf("msiUpgradeCode in release.go must be updated"), "msiUpgradeCode should be updated")
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

	output := "release.msi"
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

func downloadFile(url, dst string) {
	out, err := os.Create(dst)
	check(err, "create release file")
	defer out.Close()

	resp, err := http.Get(url)
	check(err, "download release file")
	defer resp.Body.Close()

	_, err = io.Copy(out, resp.Body)
	check(err, "write release file from http body")
}

func computeMD5(filename string) string {
	content, err := ioutil.ReadFile(filename)
	check(err, "reading file during md5 summing")
	return fmt.Sprintf("%x", md5.Sum([]byte(content)))
}

func computeSHA1(filename string) string {
	content, err := ioutil.ReadFile(filename)
	check(err, "reading file during sha1 summing")
	return fmt.Sprintf("%x", sha1.Sum([]byte(content)))
}

func computeSHA256(filename string) string {
	content, err := ioutil.ReadFile(filename)
	check(err, "reading file during sha256 summing")
	return fmt.Sprintf("%x", sha256.Sum256([]byte(content)))
}

func useWorkingDir(dir string) func() {
	check(os.RemoveAll(dir), "removeAll "+dir)
	check(os.MkdirAll(dir, os.ModePerm), "mkdirAll "+dir)
	check(os.Chdir(dir), "cd to "+dir)

	cur, err := os.Getwd()
	check(err, "get current directory")
	return func() {
		os.Chdir(cur)
	}
}

func addToTarball(tw *tar.Writer, dst, src string) {
	file, err := os.Open(src)
	check(err, "open file")
	defer file.Close()

	stat, err := file.Stat()
	check(err, "stat file")

	header := &tar.Header{
		Name:    dst,
		Size:    stat.Size(),
		Mode:    int64(stat.Mode()),
		ModTime: time.Now(),
	}

	err = tw.WriteHeader(header)
	check(err, "write header to archive")

	_, err = io.Copy(tw, file)
	check(err, "write file to archive")
}

func buildTarball() {
	log.Printf("building tarball archive\n")

	archiveFile, err := os.Create("release.tgz")
	check(err, "create archive file")
	defer archiveFile.Close()

	gw := gzip.NewWriter(archiveFile)
	defer gw.Close()

	tw := tar.NewWriter(gw)
	defer tw.Close()

	releaseName := getReleaseName()

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

	archiveFile, err := os.Create("release.zip")
	check(err, "create archive file")
	defer archiveFile.Close()

	zw := zip.NewWriter(archiveFile)
	defer zw.Close()

	releaseName := getReleaseName()

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

func generateFullReleaseJSON(v version.Version) {
	if env.EvgIsPatch() {
		log.Println("current build is a patch; not generating and uploading full JSON feed")
		return
	}

	if !canPerformStableRelease(v) {
		log.Println("current build is not a stable release task; not generating and uploading full JSON feed")
		return
	}

	awsClient, err := aws.GetClient()
	check(err, "get aws client")

	feed, err := awsClient.GenerateFullReleaseFeedFromObjects()
	check(err, "generate full release feed from s3 objects")

	uploadFeedFile("full.json", feed, awsClient)
}

func uploadReleaseJSON(v version.Version) {
	if env.EvgIsPatch() {
		log.Println("current build is a patch; not uploading release JSON feed")
		return
	}

	if !canPerformStableRelease(v) {
		log.Println("current build is not a stable release task; not uploading release JSON feed")
		return
	}

	versionID, err := env.EvgVersionID()
	check(err, "get evergreen version ID")
	tasks, err := evergreen.GetTasksForVersion(versionID)
	check(err, "get evergreen tasks")

	signTasks := []evergreen.Task{}
	for _, task := range tasks {
		if task.IsPatch() || task.DisplayName != "sign" {
			continue
		}

		if _, ok := platform.GetByVariant(task.Variant); !ok {
			continue
		}

		signTasks = append(signTasks, task)
	}

	pfCount := platform.Count()
	if len(signTasks) != pfCount {
		log.Fatalf("found %d sign tasks, but expected %d", len(signTasks), pfCount)
	}

	awsClient, err := aws.GetClient()
	check(err, "get aws client")

	// Accumulate all downloaded artifacts from sign tasks for JSON feed.
	var dls []*download.ToolsDownload

	for _, task := range signTasks {
		pf, ok := platform.GetByVariant(task.Variant)
		if !ok {
			log.Fatalf("could not find platform for variant %q", task.Variant)
		}

		log.Printf("\ngetting artifacts for %s\n", task.Variant)

		artifacts, err := evergreen.GetArtifactsForTask(task.TaskID)
		check(err, "getting artifacts list")

		if len(artifacts) != len(pf.ArtifactExtensions()) {
			log.Fatalf(
				"expected %d artifacts but found %d for %s",
				len(pf.ArtifactExtensions()), len(artifacts), task.Variant,
			)
		}

		var dl download.ToolsDownload
		dl.Name = pf.Name
		dl.Arch = pf.Arch
		for _, a := range artifacts {
			ext := path.Ext(a.URL)
			if ext == ".sig" {
				continue
			}

			stableFile := fmt.Sprintf(
				"mongodb-database-tools-%s-%s-%s%s",
				pf.Name, pf.Arch, v, ext,
			)
			artifactURL := fmt.Sprintf("https://fastdl.mongodb.org/tools/db/%s", stableFile)

			log.Printf("  downloading %s\n", a.URL)
			downloadFile(artifactURL, stableFile)

			md5sum := computeMD5(stableFile)
			sha1sum := computeSHA1(stableFile)
			sha256sum := computeSHA256(stableFile)

			// The extension indicates whether the artifact is an archive or a package.
			// We assume there's at most one archive artifact and one package artifact
			// for a given download entry.
			if ext == ".tgz" || ext == ".zip" {
				dl.Archive = download.ToolsArchive{URL: artifactURL, Md5: md5sum, Sha1: sha1sum, Sha256: sha256sum}
			} else {
				dl.Package = &download.ToolsPackage{URL: artifactURL, Md5: md5sum, Sha1: sha1sum, Sha256: sha256sum}
			}
		}

		dls = append(dls, &dl)
	}

	// Download the current full.json
	buff, err := awsClient.DownloadFile("downloads.mongodb.org", "tools/db/full.json")
	check(err, "download full.json")

	var fullFeed download.JSONFeed

	err = json.Unmarshal(buff, &fullFeed)
	check(err, "unmarshal full.json into download.JSONFeed")

	// Append the new version to full.json and upload
	fullFeed.Versions = append(fullFeed.Versions, &download.ToolsVersion{Version: v.StringWithoutPre(), Downloads: dls})
	uploadFeedFile("full.json", &fullFeed, awsClient)

	// Upload only the most recent version to release.json
	var feed download.JSONFeed
	feed.Versions = append(feed.Versions, &download.ToolsVersion{Version: v.StringWithoutPre(), Downloads: dls})

	uploadFeedFile("release.json", &feed, awsClient)
}

func uploadFeedFile(filename string, feed *download.JSONFeed, awsClient *aws.AWS) {
	var feedBuffer bytes.Buffer

	jsonEncoder := json.NewEncoder(&feedBuffer)
	jsonEncoder.SetIndent("", "  ")
	err := jsonEncoder.Encode(*feed)
	check(err, "encode json feed")

	log.Printf("uploading download feed to https://s3.amazonaws.com/downloads.mongodb.org/tools/db/%s\n", filename)
	awsClient.UploadBytes("downloads.mongodb.org", "/tools/db", filename, &feedBuffer)
}

func uploadRelease(v version.Version) {
	if env.EvgIsPatch() {
		log.Println("current build is a patch; not uploading a release")
		return
	}

	pf, err := platform.GetFromEnv()
	check(err, "get platform")

	buildID, err := env.EvgBuildID()
	check(err, "get evergreen build ID")
	tasks, err := evergreen.GetTasksForBuild(buildID)
	check(err, "get evergreen tasks")

	signTasks := []evergreen.Task{}
	for _, task := range tasks {
		if task.IsPatch() || task.DisplayName != "sign" {
			continue
		}

		signTasks = append(signTasks, task)
	}

	if len(signTasks) != 1 {
		log.Fatalf("found %d sign tasks, but expected one", len(signTasks))
	}

	awsClient, err := aws.GetClient()
	check(err, "get aws client")

	for _, task := range signTasks {
		log.Printf("\ngetting artifacts for %s\n", task.Variant)

		artifacts, err := evergreen.GetArtifactsForTask(task.TaskID)
		check(err, "getting artifacts list")

		if len(artifacts) != len(pf.ArtifactExtensions()) {
			log.Fatalf(
				"expected %d artifacts but found %d for %s",
				len(pf.ArtifactExtensions()), len(artifacts), task.Variant,
			)
		}

		for _, a := range artifacts {
			ext := path.Ext(a.URL)
			if ext == ".sig" {
				ext = a.URL[len(a.URL)-8:]
			}

			unstableFile := fmt.Sprintf(
				"mongodb-database-tools-%s-%s-unstable%s",
				pf.Name, pf.Arch, ext,
			)

			stableFile := fmt.Sprintf(
				"mongodb-database-tools-%s-%s-%s%s",
				pf.Name, pf.Arch, v, ext,
			)

			latestStableFile := fmt.Sprintf(
				"mongodb-database-tools-%s-%s-latest-stable%s",
				pf.Name, pf.Arch, ext,
			)

			log.Printf("  downloading %s\n", a.URL)
			downloadFile(a.URL, unstableFile)
			if canPerformStableRelease(v) {
				copyFile(unstableFile, stableFile)
				copyFile(unstableFile, latestStableFile)
			}

			log.Printf("    uploading to https://s3.amazonaws.com/downloads.mongodb.org/tools/db/%s\n", unstableFile)
			awsClient.UploadFile("downloads.mongodb.org", "/tools/db", unstableFile)
			if canPerformStableRelease(v) {
				log.Printf("    uploading to https://s3.amazonaws.com/downloads.mongodb.org/tools/db/%s\n", stableFile)
				awsClient.UploadFile("downloads.mongodb.org", "/tools/db", stableFile)
				log.Printf("    uploading to https://s3.amazonaws.com/downloads.mongodb.org/tools/db/%s\n", latestStableFile)
				awsClient.UploadFile("downloads.mongodb.org", "/tools/db", latestStableFile)
			}
		}
	}
}

var linuxRepoVersionsStable = map[string]string{
	"development": "4.0.0-15-gabcde123", // any non-rc pre-release version will send the package to the "development" repo
	"testing":     "4.0.0-rc0",          // any rc version will send the package to the "testing" repo
	"4.3":         "4.3.0",              // any 4.3 stable release version will send the package to the "4.3" repo
	"4.4":         "4.4.0",              // any 4.4 stable release version will send the package to the "4.4" repo
}

var linuxRepoVersionsUnstable = map[string]string{
	"development": "4.0.0-15-gabcde123", // any non-rc pre-release version will send the package to the "development" repo
}

func linuxRelease(v version.Version) {
	if env.EvgIsPatch() {
		log.Println("current build is a patch; not performing a linux release")
		return
	}

	pf, err := platform.GetFromEnv()
	check(err, "get platform")

	if pf.OS != platform.OSLinux {
		log.Printf("cannot release linux packages for non-linux platform")
		return
	}

	buildID, err := env.EvgBuildID()
	check(err, "get evergreen build ID")
	tasks, err := evergreen.GetTasksForBuild(buildID)
	check(err, "get evergreen tasks")

	distTasks := []evergreen.Task{}
	for _, task := range tasks {
		if task.IsPatch() || task.DisplayName != "dist" {
			continue
		}

		distTasks = append(distTasks, task)
	}

	if len(distTasks) != 1 {
		log.Fatalf("found %d dist tasks, but expected one", len(distTasks))
	}

	wg := &sync.WaitGroup{}
	for _, task := range distTasks {
		log.Printf("\ngetting artifacts for %s\n", task.Variant)

		artifacts, err := evergreen.GetArtifactsForTask(task.TaskID)
		check(err, "getting artifacts list")

		packagesURL := ""
		for _, a := range artifacts {
			if strings.HasPrefix(a.Name, "All Release Artifacts") {
				packagesURL = a.URL
				break
			}
		}

		editionsToRelease := pf.Repos
		versionsToRelease := linuxRepoVersionsStable
		if !canPerformStableRelease(v) {
			versionsToRelease = linuxRepoVersionsUnstable
		}

		for mongoVersionName, mongoVersionNumber := range versionsToRelease {
			for _, mongoEdition := range editionsToRelease {
				wg.Add(1)
				go func() {
					var err error
					prefix := fmt.Sprintf("%s-%s-%s", pf.Variant(), mongoEdition, mongoVersionName)
					// retry twice on failure.
					maxRetries := 2
					for retries := maxRetries; retries >= 0; retries-- {
						curatorArgs := []string{
							"--level", "debug",
							"repo", "submit",
							"--service", "https://barque.corp.mongodb.com",
							"--config", "etc/repo-config.yml",
							"--distro", pf.Name,
							"--arch", pf.Arch,
							"--edition", mongoEdition,
							"--version", mongoVersionNumber,
							"--packages", packagesURL,
							"--username", os.Getenv("BARQUE_USERNAME"),
							"--api_key", os.Getenv("BARQUE_API_KEY"),
						}

						if retries == maxRetries {
							log.Printf("starting curator for %s\n", prefix)
						} else {
							log.Printf("restarting curator for %s after failure\n", prefix)
						}
						err = runAndStreamStderr(prefix, "./curator", curatorArgs...)
						if err == nil {
							log.Printf("finished curator for %s\n", prefix)
							wg.Done()
							return
						}
					}
					check(err, "run curator for %s", prefix)
				}()

				// We need to sleep briefly between curator
				// invocations because of an auth race condition in
				// barque.
				// Sleep a bit more to try to keep the flakiness from showing up.
				time.Sleep(5 * time.Second)
			}
		}
	}
	log.Println("waiting for curator invocations to finish")
	wg.Wait()
}

// canPerformStableRelease returns whether we can perform a stable
// release for the provided version. It returns true if the provided
// version is a stable version and the current evg task was triggered
// by a git tag.
func canPerformStableRelease(v version.Version) bool {
	return v.IsStable() && env.EvgIsTagTriggered()
}
