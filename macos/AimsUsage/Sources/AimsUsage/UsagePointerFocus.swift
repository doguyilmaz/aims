import AppKit

@MainActor
enum UsagePointerFocus {
    static var isPointerEvent: Bool {
        guard let type = NSApp.currentEvent?.type else { return false }
        return [.leftMouseDown, .leftMouseUp, .rightMouseDown, .rightMouseUp, .otherMouseDown, .otherMouseUp]
            .contains(type)
    }

    static func clear(in window: NSWindow?, pointerInitiated: Bool) {
        guard pointerInitiated else { return }
        window?.makeFirstResponder(nil)
    }
}
