import AppKit

final class SettingsWindowView: NSView {
    override func viewDidMoveToWindow() {
        super.viewDidMoveToWindow()
        window?.styleMask.insert(.fullSizeContentView)
        window?.titlebarAppearsTransparent = true
        window?.titlebarSeparatorStyle = .none
        window?.toolbarStyle = .unifiedCompact
    }
}
