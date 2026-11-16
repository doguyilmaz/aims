import AppKit

final class SettingsWindowView: NSView {
    override func hitTest(_ point: NSPoint) -> NSView? { nil }

    override func layout() {
        super.layout()
        guard let window else { return }
        // SwiftUI finishes configuring Settings chrome after the view attaches to its window.
        // Apply the native window style during layout, without replacing the system glass or its corner geometry.
        let mask: NSWindow.StyleMask = [.fullSizeContentView, .resizable]
        if !window.styleMask.contains(mask) { window.styleMask.formUnion(mask) }
        if !window.titlebarAppearsTransparent { window.titlebarAppearsTransparent = true }
        if window.titlebarSeparatorStyle != .none { window.titlebarSeparatorStyle = .none }
        if window.toolbarStyle != .unified { window.toolbarStyle = .unified }
        let content = SettingsWindowMetrics.minimumContentSize
        let minimum = NSSize(
            width: content.width, height: content.height + (window.contentView?.safeAreaInsets.top ?? 0))
        if window.minSize != minimum { window.minSize = minimum }
    }
}
