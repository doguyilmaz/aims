#!/usr/bin/env python3
"""Release configuration tests use dummy public keys, with no network or signing."""
import base64
import hashlib
import importlib.util
from pathlib import Path
import subprocess
import sys
import unittest
import tempfile
import xml.etree.ElementTree as ET

spec = importlib.util.spec_from_file_location("updates", Path(__file__).with_name("configure-macos-updates.py"))
updates = importlib.util.module_from_spec(spec)
spec.loader.exec_module(updates)


class UpdateConfigurationTests(unittest.TestCase):
    def test_development_and_preview_have_no_channel(self):
        self.assertEqual(updates.configuration("", "", "-", False), {})
        self.assertEqual(updates.configuration("https://updates.example.invalid/appcast.xml", "bad", "-", True), {})

    def test_rejects_partial_untrusted_or_unsigned_configuration(self):
        key = base64.b64encode(bytes(32)).decode()
        for feed, public, identity in [
            ("https://updates.example.invalid/appcast.xml", "", "identity"),
            ("", key, "identity"),
            ("http://updates.example.invalid/appcast.xml", key, "identity"),
            ("https://user:password@example.invalid/appcast.xml", key, "identity"),
            ("https://example.invalid/appcast.xml?token=dummy", key, "identity"),
            ("https://example.invalid/appcast.xml#fragment", key, "identity"),
            ("https://example.invalid/appcast.xml?", key, "identity"),
            ("https://example.invalid/appcast.xml#", key, "identity"),
            ("https://example.invalid/appcast.xml", "not-base64", "identity"),
            ("https://example.invalid/appcast.xml", base64.b64encode(bytes(31)).decode(), "identity"),
            ("https://example.invalid/appcast.xml", key, "-"),
        ]:
            with self.subTest(feed=feed, identity=identity), self.assertRaises(ValueError):
                updates.configuration(feed, public, identity, False)

    def test_release_verifies_before_extraction_without_silent_installs_or_profile_upload(self):
        key = base64.b64encode(bytes(32)).decode()
        result = updates.configuration("https://example.invalid/appcast.xml", key, "identity", False)
        self.assertEqual(result["SUPublicEDKey"], key)
        self.assertTrue(result["SUVerifyUpdateBeforeExtraction"])
        self.assertTrue(result["SURequireSignedFeed"])
        self.assertFalse(result["SUEnableAutomaticChecks"])
        self.assertFalse(result["SUAutomaticallyUpdate"])
        self.assertFalse(result["SUEnableSystemProfiling"])


class AppcastValidationTests(unittest.TestCase):
    def test_rejects_unsigned_or_mismatched_release_before_publication(self):
        spec = importlib.util.spec_from_file_location("appcast", Path(__file__).with_name("verify-macos-appcast.py"))
        appcast = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(appcast)
        with tempfile.TemporaryDirectory(prefix="aims-appcast-test-") as directory:
            archive = Path(directory) / "Aims-Usage-0.3.0-2-universal.zip"
            archive.write_bytes(b"dummy archive")
            root = ET.Element("rss")
            channel = ET.SubElement(root, "channel")
            item = ET.SubElement(channel, "item")
            ET.SubElement(item, appcast.SPARKLE + "version").text = "2"
            ET.SubElement(item, appcast.SPARKLE + "shortVersionString").text = "0.3.0"
            entry = ET.SubElement(item, "enclosure", url="https://example.invalid/updates/" + archive.name, length=str(archive.stat().st_size))
            feed = Path(directory) / "appcast.xml"
            ET.ElementTree(root).write(feed)
            with self.assertRaises(ValueError):
                appcast.enclosure(feed, archive, "0.3.0", "2", "https://example.invalid/updates")
            signature = base64.b64encode(bytes(64)).decode()
            entry.set(appcast.SPARKLE + "edSignature", signature)
            ET.ElementTree(root).write(feed)
            self.assertEqual(appcast.enclosure(feed, archive, "0.3.0", "2", "https://example.invalid/updates"), signature)
            for version, build, prefix in [("0.3.1", "2", "https://example.invalid/updates"), ("0.3.0", "3", "https://example.invalid/updates"), ("0.3.0", "2", "https://other.example.invalid/updates")]:
                with self.subTest(version=version, build=build, prefix=prefix), self.assertRaises(ValueError):
                    appcast.enclosure(feed, archive, version, build, prefix)
            archive.write_bytes(b"tampered archive, changed size")
            with self.assertRaises(ValueError):
                appcast.enclosure(feed, archive, "0.3.0", "2", "https://example.invalid/updates")


def load_script(name):
    spec = importlib.util.spec_from_file_location(name, Path(__file__).with_name(name + ".py"))
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


class UsageCaskTests(unittest.TestCase):
    def test_hashes_exact_release_without_claiming_cli_or_deleting_profile_data(self):
        cask = load_script("prepare-macos-usage-cask")
        with tempfile.TemporaryDirectory() as directory:
            archive = Path(directory) / "Aims-Usage-0.3.0-2-universal.zip"
            archive.write_bytes(b"dummy verified release archive")
            result = cask.render(archive, "0.3.0", "2", "https://example.invalid/usage-v0.3.0-2")
            self.assertIn(hashlib.sha256(archive.read_bytes()).hexdigest(), result)
            self.assertIn("https://example.invalid/usage-v0.3.0-2/" + archive.name, result)
            self.assertIn('cask "aims-usage"', result)
            self.assertIn("auto_updates true", result)
            self.assertNotIn("\n  binary ", result)
            self.assertNotIn("zap", result)
            self.assertNotIn(".aims", result.replace("com.doguyilmaz.aims.usage", ""))

    def test_rejects_mismatched_builds_and_ambiguous_or_credentialled_urls(self):
        cask = load_script("prepare-macos-usage-cask")
        with tempfile.TemporaryDirectory() as directory:
            archive = Path(directory) / "Aims-Usage-0.3.0-2-universal.zip"
            archive.write_bytes(b"dummy")
            for version, build, prefix in [
                ("0.3", "2", "https://example.invalid/releases"),
                ("0.3.0", "02", "https://example.invalid/releases"),
                ("0.3.1", "2", "https://example.invalid/releases"),
                ("0.3.0", "2", "http://example.invalid/releases"),
                ("0.3.0", "2", "https://user:secret@example.invalid/releases"),
                ("0.3.0", "2", "https://example.invalid/releases?"),
                ("0.3.0", "2", "https://example.invalid/releases#"),
            ]:
                with self.subTest(version=version, build=build, prefix=prefix), self.assertRaises(ValueError):
                    cask.render(archive, version, build, prefix)

    def test_existing_cask_output_is_preserved(self):
        with tempfile.TemporaryDirectory() as directory:
            archive = Path(directory) / "Aims-Usage-0.3.0-2-universal.zip"
            archive.write_bytes(b"dummy")
            output = Path(directory) / "aims-usage.rb"
            output.write_text("owner edits")
            result = subprocess.run([
                sys.executable, "-B", str(Path(__file__).with_name("prepare-macos-usage-cask.py")),
                str(archive), "--version", "0.3.0", "--build", "2",
                "--url-prefix", "https://example.invalid/releases", "--output", str(output),
            ], capture_output=True)
            self.assertNotEqual(result.returncode, 0)
            self.assertEqual(output.read_text(), "owner edits")


class NotarizationAuthenticationTests(unittest.TestCase):
    def test_existing_profile_api_and_apple_id_modes(self):
        notary = load_script("notarize-macos-usage")
        self.assertEqual(notary.authentication({"AIMS_NOTARY_KEYCHAIN_PROFILE": "dummy-profile"}),
                         ["--keychain-profile", "dummy-profile"])
        with tempfile.NamedTemporaryFile() as key:
            self.assertEqual(notary.authentication({"AIMS_NOTARY_KEY_FILE": key.name,
                "AIMS_NOTARY_KEY_ID": "DUMMY", "AIMS_NOTARY_ISSUER": "dummy-issuer"}),
                ["--key", key.name, "--key-id", "DUMMY", "--issuer", "dummy-issuer"])
        self.assertEqual(notary.authentication({"AIMS_NOTARY_APPLE_ID": "dummy@example.invalid",
            "AIMS_NOTARY_TEAM_ID": "DUMMY", "AIMS_NOTARY_PASSWORD": "dummy-password"}),
            ["--apple-id", "dummy@example.invalid", "--team-id", "DUMMY", "--password", "dummy-password"])
        for env in [{}, {"AIMS_NOTARY_KEY_ID": "DUMMY"}, {"AIMS_NOTARY_APPLE_ID": "dummy@example.invalid"}]:
            with self.subTest(env=env), self.assertRaises(ValueError):
                notary.authentication(env)

    def test_failed_submission_does_not_echo_password_in_command_error(self):
        with tempfile.TemporaryDirectory() as directory:
            tool = Path(directory) / "xcrun"
            tool.write_text("#!/bin/sh\nexit 23\n")
            tool.chmod(0o755)
            result = subprocess.run([
                sys.executable, "-B", str(Path(__file__).with_name("notarize-macos-usage.py")), "dummy.zip",
            ], env={"PATH": directory, "AIMS_NOTARY_APPLE_ID": "dummy@example.invalid",
                    "AIMS_NOTARY_TEAM_ID": "DUMMY", "AIMS_NOTARY_PASSWORD": "dummy-secret-must-stay-private"},
                capture_output=True, text=True)
            self.assertNotEqual(result.returncode, 0)
            self.assertIn("Notarization failed", result.stderr)
            self.assertNotIn("dummy-secret-must-stay-private", result.stdout + result.stderr)


if __name__ == "__main__":
    unittest.main()
