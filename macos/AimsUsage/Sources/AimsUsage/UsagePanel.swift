import AppKit

/// A borderless utility panel still needs key focus for menus, popovers and sheets.
final class UsagePanel: NSPanel {
    var onCancel: (@MainActor () -> Void)?

    override var canBecomeKey: Bool { true }
    override var canBecomeMain: Bool { false }

    func presentForInteraction() {
        let pointerOpening = Self.pointerInteraction
        makeKeyAndOrderFront(nil)
        if pointerOpening { makeFirstResponder(nil) }
    }

    func clearPointerFocus() {
        if Self.pointerInteraction { makeFirstResponder(nil) }
    }

    override func cancelOperation(_ sender: Any?) {
        if let onCancel { onCancel() } else { super.cancelOperation(sender) }
    }

    private static var pointerInteraction: Bool {
        guard let type = NSApp.currentEvent?.type else { return false }
        return [.leftMouseDown, .leftMouseUp, .rightMouseDown, .rightMouseUp, .otherMouseDown, .otherMouseUp].contains(
            type)
    }
}
