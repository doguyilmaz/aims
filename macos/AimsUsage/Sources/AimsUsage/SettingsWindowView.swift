import AppKit

final class SettingsWindowView: NSView {
    override func viewDidMoveToWindow() {
        super.viewDidMoveToWindow()
        window?.toolbarStyle = .unifiedCompact
    }
}
