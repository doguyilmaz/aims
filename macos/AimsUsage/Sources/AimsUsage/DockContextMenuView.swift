import AppKit

final class DockContextMenuView: NSView {
    var showOptions: ((CGPoint) -> Void)?

    override func hitTest(_ point: NSPoint) -> NSView? {
        guard let event = NSApp.currentEvent,
            event.type == .rightMouseDown
                || (event.type == .leftMouseDown && event.modifierFlags.contains(.control))
        else { return nil }
        return super.hitTest(point)
    }

    override func mouseDown(with event: NSEvent) { rightMouseDown(with: event) }

    override func rightMouseDown(with event: NSEvent) {
        guard let window else { return }
        showOptions?(window.convertPoint(toScreen: event.locationInWindow))
    }
}
