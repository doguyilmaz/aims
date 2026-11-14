#!/usr/bin/env python3
"""Submit using an existing Keychain profile, API key, or Apple ID credentials."""
import os
from pathlib import Path
import subprocess
import sys


def authentication(env):
    if profile := env.get("AIMS_NOTARY_KEYCHAIN_PROFILE"):
        return ["--keychain-profile", profile]
    api = [env.get(name, "") for name in ("AIMS_NOTARY_KEY_FILE", "AIMS_NOTARY_KEY_ID", "AIMS_NOTARY_ISSUER")]
    if any(api):
        if not all(api) or not Path(api[0]).is_file():
            raise ValueError("Set a readable AIMS_NOTARY_KEY_FILE plus AIMS_NOTARY_KEY_ID and AIMS_NOTARY_ISSUER")
        return ["--key", api[0], "--key-id", api[1], "--issuer", api[2]]
    apple = [env.get(name, "") for name in ("AIMS_NOTARY_APPLE_ID", "AIMS_NOTARY_TEAM_ID", "AIMS_NOTARY_PASSWORD")]
    if not all(apple):
        raise ValueError("Set a Keychain profile, complete notary API inputs, or AIMS_NOTARY_APPLE_ID/TEAM_ID/PASSWORD")
    return ["--apple-id", apple[0], "--team-id", apple[1], "--password", apple[2]]


def main():
    auth = authentication(os.environ)
    if sys.argv[1:] == ["--validate"]:
        return
    if len(sys.argv) != 2:
        raise ValueError("Usage: notarize-macos-usage.py <archive> or --validate")
    subprocess.run(["xcrun", "notarytool", "submit", sys.argv[1], *auth, "--wait"], check=True)


if __name__ == "__main__":
    try:
        main()
    except subprocess.CalledProcessError:
        # CalledProcessError includes argv, which can contain an app password.
        sys.exit("Notarization failed; see notarytool output")
    except (ValueError, OSError) as error:
        sys.exit(str(error))
