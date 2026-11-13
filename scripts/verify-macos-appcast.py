#!/usr/bin/env python3
"""Refuse an unsigned/mismatched appcast before retaining release artifacts."""
import base64
import os
from pathlib import Path
import subprocess
import sys
import xml.etree.ElementTree as ET

SPARKLE = "{http://www.andymatuschak.org/xml-namespaces/sparkle}"


def enclosure(feed, archive, version, build, prefix):
    root = ET.parse(feed).getroot()
    items = root.findall("./channel/item")
    if len(items) != 1:
        raise ValueError("Expected exactly one prepared release item")
    item = items[0]
    if item.findtext(SPARKLE + "version") != build or item.findtext(SPARKLE + "shortVersionString") != version:
        raise ValueError("Appcast build/version does not match release inputs")
    entry = item.find("enclosure")
    if entry is None:
        raise ValueError("Appcast has no archive enclosure")
    signature = entry.get(SPARKLE + "edSignature", "")
    if len(base64.b64decode(signature, validate=True)) != 64:
        raise ValueError("Appcast archive signature is missing or invalid; check matching Sparkle keys")
    if entry.get("length") != str(archive.stat().st_size):
        raise ValueError("Appcast archive size is incorrect")
    if entry.get("url") != prefix.rstrip("/") + "/" + archive.name:
        raise ValueError("Appcast archive URL does not match the prepared release")
    return signature


def main():
    feed, archive = map(Path, sys.argv[1:])
    signature = enclosure(feed, archive, os.environ["AIMS_USAGE_VERSION"],
                          os.environ["AIMS_USAGE_BUILD_NUMBER"], os.environ["AIMS_USAGE_DOWNLOAD_URL_PREFIX"])
    tool = str(Path(os.environ["AIMS_SPARKLE_TOOLS"]) / "sign_update")
    args = [tool, "--verify", "--ed-key-file", os.environ["AIMS_SPARKLE_PRIVATE_KEY_FILE"]]
    subprocess.run(args + [str(archive), signature], check=True)
    subprocess.run(args + [str(feed)], check=True)


if __name__ == "__main__":
    try:
        main()
    except (ValueError, OSError, ET.ParseError, subprocess.CalledProcessError) as error:
        sys.exit(str(error))
