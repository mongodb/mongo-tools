#!/usr/bin/env python

# This script needs to be compatible with Python 2.6, which is the version we
# have on RHEL 6.2.

"""
Command line utility returns the URL of the most recent archive file
satisfying given version, edition, and operating system requirements.
"""

import argparse
import glob
import json
import os
import platform
import shutil
import sys
import tarfile
import tempfile
import zipfile

if sys.version_info.major == 3:
    import urllib.request as urllib2
else:
    import urllib2


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument(
        "--arch", help="processor architecture (e.g. 'x86_64', 'arm64')"
    )
    parser.add_argument(
        "--edition",
        help="edition of MongoDB to use (e.g. 'targeted', 'enterprise'); defaults to 'base'",
    )
    parser.add_argument(
        "--target",
        help="system in use (e.g. 'ubuntu1204', 'windows_x86_64-2008plus-ssl', 'rhel71')",
    )
    parser.add_argument(
        "--version", help="version branch (e.g. '2.6', '3.2.8-rc1', 'latest')"
    )
    opts = parser.parse_args()

    dl = Main(opts)
    dl.download_and_extract_components()


class Main:
    def __init__(self, opts):
        self.opts = opts
        self.wanted = Wanted(opts.edition, opts.version, opts.target, opts.arch)

    def download_and_extract_components(self):
        if not os.path.exists("bin"):
            os.mkdir("bin")

        self.dir = tempfile.mkdtemp()

        finder = UrlFinder(self.wanted, False)
        url = finder.url_for_wanted()
        if not url:
            raise Exception("Could not find a url to download the server")
        log("Downloading server from {}".format(url))
        self.download_url_and_extract(url, False)

        if self.wanted.is_latest() or version_is_greater_or_equal(
            self.wanted.version, "6.0"
        ):
            for version in ["5.3", "5.2", "5.1", "5.0"]:
                finder.wanted.version = version
                url = finder.url_for_wanted()
                if url:
                    log("Downloading shell from {}".format(url))
                    self.download_url_and_extract(url, True)
                    return
            raise Exception("Could not find a 5.x release to download the shell from")

    def download_url_and_extract(self, url, shell_only):
        resp = urllib2.urlopen(url)
        if resp.getcode() != 200:
            raise Exception(
                "Got error response from {}: {}".format(url, resp.getcode())
            )

        local = os.path.join(self.dir, os.path.basename(url))
        with open(local, "wb") as file:
            file.write(resp.read())
            file.close()

        if local.endswith(".zip"):
            log("Extracting downloaded zip file at {}".format(local))
            zip = zipfile.ZipFile(local, "r")
            zip.extractall(self.dir)
            zip.close()
        else:
            log("Extracting downloaded tarball at {}".format(local))
            tar = tarfile.open(local, "r:gz")
            tar.extractall(self.dir)
            tar.close()

        extracted = glob.glob(os.path.join(self.dir, "mongodb-*", "bin"))
        if len(extracted) != 1:
            raise Exception(
                "Could not find the extracted tarball/zip in the temp dir: {}".format(
                    extracted
                )
            )

        if shell_only:
            wanted = ["mongo"]
        elif self.wanted.is_60_plus():
            wanted = ["mongos", "mongod"]
        else:
            wanted = ["mongo", "mongos", "mongod"]

        for exe in wanted:
            if platform.system() == "Windows":
                exe += ".exe"
            os.rename(os.path.join(extracted[0], exe), os.path.join("bin", exe))

        os.remove(local)
        shutil.rmtree(extracted[0])


class Wanted:
    def __init__(self, edition, version, target, arch):
        if not edition:
            edition = "base"
        if not arch:
            sys.exit("must specify --arch")
        if not target:
            sys.exit("must specify --target")
        if not version:
            sys.exit("must specify --version")

        if version == "latest" or version_is_greater_or_equal(version, "4.1.0"):
            if target in ("osx-ssl", "osx"):
                target = "macos"

        if version_is_greater_or_equal(version, "4.2.0") and arch == "arm64":
            arch = "aarch64"

        self.arch = arch
        self.edition = edition
        self.target = target
        self.version = version

    def is_latest(self):
        return self.version == "latest"

    def is_60_plus(self):
        return self.version == "latest" or version_is_greater_or_equal(
            self.version, "6.0"
        )


class UrlFinder:
    CURRENT_VERSIONS_JSON_URL = "http://downloads.mongodb.org/current.json"
    FULL_VERSIONS_JSON_URL = "http://downloads.mongodb.org/full.json"

    def __init__(self, wanted, shell):
        self.wanted = wanted
        self.shell = shell
        self.downloaded = {"current": None, "full": None}

    def url_for_wanted(self):
        url = self.find_url_in_spec(self.current_spec())
        if url:
            return url
        url = self.find_url_in_spec(self.full_spec())
        if url:
            return url

    def current_spec(self):
        if self.downloaded["current"]:
            return self.downloaded["current"]
        self.downloaded["current"] = self.download_spec(self.CURRENT_VERSIONS_JSON_URL)
        return self.downloaded["current"]

    def full_spec(self):
        if self.downloaded["full"]:
            return self.downloaded["full"]
        self.downloaded["full"] = self.download_spec(self.FULL_VERSIONS_JSON_URL)
        return self.downloaded["full"]

    def download_spec(self, url):
        log("Downloading spec at {}".format(url))
        contents = json.load(urllib2.urlopen(url))
        return contents

    def find_url_in_spec(self, spec):
        urls = []
        for version in spec["versions"]:
            if not self.is_correct_version(version):
                continue
            for download in version["downloads"]:
                if self.is_correct_download(download):
                    url = self.url_for_component(download)
                    if url:
                        urls.append(url)

        if len(urls) > 0:
            return urls[0]

    def is_correct_version(self, version):
        # We'll return all the versions and then pick the first, which will always be the most
        # recent.
        if self.wanted.is_latest():
            return True

        # For an approximate match, ignore '-rcX' part, but due to json file ordering x.y.z
        # will always be before x.y.z-rcX, which is what we want
        parts = version["version"].split("-")
        actual = parts[0].split(".")
        desired = self.wanted.version.split(".")
        for i in range(len(desired)):
            if desired[i] and not actual[i] == desired[i]:
                return False
        return True

    def is_correct_download(self, download):
        return (
            download["edition"] == self.wanted.edition
            and download["target"] == self.wanted.target
            and download["arch"] == self.wanted.arch
        )

    def url_for_component(self, download):
        dl_key = "shell" if self.shell else "archive"
        entry = download.get(dl_key)
        if entry is None:
            return None
        return entry["url"]


def version_is_greater_or_equal(left, right):
    l = left.split(".")
    r = right.split(".")
    for i in range(len(l)):
        if l[i] < r[i]:
            return False
        elif l[i] > r[i]:
            return True
    return True


def log(msg):
    sys.stderr.write(msg + "\n")


main()
