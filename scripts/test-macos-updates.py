#!/usr/bin/env python3
"""Release configuration tests use dummy public keys, with no network or signing."""
import base64
import importlib.util
from pathlib import Path
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


if __name__ == "__main__":
    unittest.main()
