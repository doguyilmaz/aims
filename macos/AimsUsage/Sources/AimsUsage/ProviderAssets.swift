import AppKit

enum ProviderAssets {
    static func prepare() {
        for id in ["claude", "codex", "kiro", "grok", "agy", "opencode"] {
            guard let url = Bundle.main.url(forResource: id, withExtension: "svg", subdirectory: "provider-marks"),
                let image = NSImage(contentsOf: url)
            else { continue }
            image.isTemplate = true
            image.setName("aims.provider.\(id)")
        }
    }
}
