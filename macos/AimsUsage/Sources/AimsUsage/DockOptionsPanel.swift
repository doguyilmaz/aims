import AppKit

/// Pointer opening leaves the window itself focused, so it owns Escape too.
final class DockOptionsPanel: NSPanel {
    var onCancel: (@MainActor () -> Void)?

    override var canBecomeKey: Bool { true }
    override var canBecomeMain: Bool { false }

    override func cancelOperation(_ sender: Any?) { onCancel?() }
}
