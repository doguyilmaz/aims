# Legacy docs domain

This Worker redirects only `aims.apps.doguyilmaz.com` to `https://aims.doguyilmaz.com`, preserving the path and query string. The docs remain on GitHub Pages. Its exact-host Custom Domain supplies managed TLS for the old, deeper hostname; it needs no origin or secret. Query strings are redacted from telemetry.

Deployment uses the installed Cloudflare CLI `1.0.0-beta.18` and its Build Output `v0` format, without a bundler or project dependencies:

```sh
node build.mjs
cf deploy --prebuilt --dry-run
cf deploy --prebuilt
```

Run these from this directory. Review the DNS record before the first attachment: Cloudflare Custom Domains cannot replace an existing CNAME automatically. Keep the old record for rollback and attach only this hostname. Do not change other site rules or certificates.

The compatibility date follows the Cloudflare API’s UTC date (2026-11-16 at setup), which can lag the local calendar day.
