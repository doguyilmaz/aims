#!/usr/bin/env python3
"""Add a complete release update configuration, or leave development builds offline."""
import base64
import os
import plistlib
import sys
from urllib.parse import urlsplit


def require_https(value, name):
    url = urlsplit(value)
    if (url.scheme != "https" or not url.hostname or url.username or url.password
            or "?" in value or "#" in value or any(c.isspace() for c in value)):
        raise ValueError(name + " must be an HTTPS URL without credentials, query or fragment")
    _ = url.port


def configuration(feed, key, identity, preview):
    if preview:
        return {}
    if not feed and not key:
        return {}
    if not feed or not key:
        raise ValueError("AIMS_USAGE_FEED_URL and AIMS_USAGE_PUBLIC_ED_KEY must be set together")
    require_https(feed, "AIMS_USAGE_FEED_URL")
    try:
        decoded = base64.b64decode(key, validate=True)
    except ValueError as error:
        raise ValueError("AIMS_USAGE_PUBLIC_ED_KEY must be a base64 Ed25519 public key") from error
    if len(decoded) != 32:
        raise ValueError("AIMS_USAGE_PUBLIC_ED_KEY must decode to 32 bytes")
    if not identity or identity == "-":
        raise ValueError("Update-enabled builds require AIMS_USAGE_SIGNING_IDENTITY")
    return {"SUFeedURL": feed, "SUPublicEDKey": key,
            "SUEnableAutomaticChecks": False, "SUAutomaticallyUpdate": False,
            "SUVerifyUpdateBeforeExtraction": True, "SURequireSignedFeed": True,
            "SUEnableSystemProfiling": False}


def main():
    if sys.argv[1] == "--validate":
        configuration(os.environ.get("AIMS_USAGE_FEED_URL", ""),
                      os.environ.get("AIMS_USAGE_PUBLIC_ED_KEY", ""),
                      os.environ.get("AIMS_USAGE_SIGNING_IDENTITY", ""),
                      os.environ.get("AIMS_USAGE_PREVIEW", "0") == "1")
        prefix = os.environ.get("AIMS_USAGE_DOWNLOAD_URL_PREFIX")
        if prefix:
            require_https(prefix, "AIMS_USAGE_DOWNLOAD_URL_PREFIX")
        return
    with open(sys.argv[1], "rb") as stream:
        info = plistlib.load(stream)
    settings = configuration(os.environ.get("AIMS_USAGE_FEED_URL", ""),
                             os.environ.get("AIMS_USAGE_PUBLIC_ED_KEY", ""),
                             os.environ.get("AIMS_USAGE_SIGNING_IDENTITY", ""),
                             info.get("AimsUsagePreview", False))
    info.update(settings)
    with open(sys.argv[1], "wb") as stream:
        plistlib.dump(info, stream)


if __name__ == "__main__":
    try:
        main()
    except (ValueError, OSError) as error:
        sys.exit(str(error))
