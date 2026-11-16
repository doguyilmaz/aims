import AppKit

/// A borderless utility panel still needs key focus for menus, popovers and sheets.
final class UsagePanel: NSPanel {
    var onCancel: (@MainActor () -> Void)?

    override var canBecomeKey: Bool { true }
    override var canBecomeMain: Bool { false }

    func presentForInteraction() {
        let pointerOpening = UsagePointerFocus.isPointerEvent
        makeKeyAndOrderFront(nil)
        UsagePointerFocus.clear(in: self, pointerInitiated: pointerOpening)
    }

    override func cancelOperation(_ sender: Any?) {
        if let onCancel { onCancel() } else { super.cancelOperation(sender) }
    }
}
