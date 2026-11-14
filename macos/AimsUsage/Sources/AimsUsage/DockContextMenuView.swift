import AppKit

final class DockContextMenuView: NSView {
    var makeMenu: (() -> NSMenu)?

    override func hitTest(_ point: NSPoint) -> NSView? {
        guard let event = NSApp.currentEvent,
            event.type == .rightMouseDown
                || (event.type == .leftMouseDown && event.modifierFlags.contains(.control))
        else { return nil }
        return super.hitTest(point)
    }

    override func mouseDown(with event: NSEvent) { rightMouseDown(with: event) }

    override func rightMouseDown(with event: NSEvent) {
        guard let menu = makeMenu?() else { return }
        // AppKit positions and tracks a contextual menu against the clicked screen,
        // independently of the 44/64-point rail and its SwiftUI layout constraints.
        NSMenu.popUpContextMenu(menu, with: event, for: self)
    }
}
