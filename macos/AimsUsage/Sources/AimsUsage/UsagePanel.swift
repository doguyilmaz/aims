import AppKit

/// A borderless utility panel still needs key focus for menus, popovers and sheets.
final class UsagePanel: NSPanel {
    override var canBecomeKey: Bool { true }
    override var canBecomeMain: Bool { false }
}
