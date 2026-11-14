#!/usr/bin/env python3
"""Prepare one local cask from the verified release ZIP; never publish it."""
import argparse
import hashlib
import json
from pathlib import Path
import re
import sys
from urllib.parse import urlsplit


def render(archive, version, build, prefix):
    if not re.fullmatch(r"(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)", version):
        raise ValueError("App version must be MAJOR.MINOR.PATCH")
    if not re.fullmatch(r"[1-9][0-9]*", build):
        raise ValueError("Build number must be a positive integer without leading zeroes")
    if archive.name != f"Aims-Usage-{version}-{build}-universal.zip" or archive.is_symlink() or not archive.is_file():
        raise ValueError("Expected the matching regular universal release ZIP")
    url = urlsplit(prefix)
    if (url.scheme != "https" or not url.hostname or url.username or url.password
            or "?" in prefix or "#" in prefix or any(c.isspace() for c in prefix)):
        raise ValueError("Release URL prefix must be HTTPS without credentials, query or fragment")
    _ = url.port
    hasher = hashlib.sha256()
    with archive.open("rb") as stream:
        while chunk := stream.read(1024 * 1024):
            hasher.update(chunk)
    digest = hasher.hexdigest()
    # No Ruby interpolation: the URL pins the exact version/build archive.
    download = json.dumps(prefix.rstrip("/") + "/" + archive.name)
    return f'''# Prepared by aims' release tooling. Review before publishing to Casks/aims-usage.rb.
cask "aims-usage" do
  version "{version}"
  sha256 "{digest}"

  url {download}
  name "Aims Usage"
  desc "AI provider usage and reset windows in the menu bar"
  homepage "https://aims.apps.doguyilmaz.com"

  livecheck do
    skip "Updated from the signed Usage release."
  end

  auto_updates true
  depends_on macos: ">= :sonoma"

  app "Aims Usage.app"

  uninstall quit: "com.doguyilmaz.aims.usage"
end
'''


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("archive", type=Path)
    parser.add_argument("--version", required=True)
    parser.add_argument("--build", required=True)
    parser.add_argument("--url-prefix", required=True)
    parser.add_argument("--output", type=Path)
    args = parser.parse_args()
    cask = render(args.archive, args.version, args.build, args.url_prefix)
    if args.output:
        with args.output.open("x", encoding="utf-8") as stream:
            stream.write(cask)
    else:
        sys.stdout.write(cask)


if __name__ == "__main__":
    try:
        main()
    except (ValueError, OSError) as error:
        sys.exit(str(error))
