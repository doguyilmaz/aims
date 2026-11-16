import AppKit
import Testing

@testable import AimsUsage

@Test @MainActor func pointerInteractionClearsFocusInAnOrdinaryWindow() {
    _ = NSApplication.shared
    let window = NSWindow(
        contentRect: NSRect(x: 0, y: 0, width: 200, height: 100), styleMask: [.titled], backing: .buffered, defer: false
    )
    window.isReleasedWhenClosed = false
    defer { window.close() }
    let field = NSTextField(frame: NSRect(x: 10, y: 10, width: 100, height: 24))
    window.contentView?.addSubview(field)
    window.initialFirstResponder = field
    #expect(window.makeFirstResponder(field))
    #expect(window.firstResponder !== window)

    UsagePointerFocus.clear(in: window, pointerInitiated: true)
    #expect(window.firstResponder === window)

    window.selectNextKeyView(nil)
    #expect(window.firstResponder !== window)
}

@Test @MainActor func keyboardInteractionKeepsTheCurrentResponder() {
    _ = NSApplication.shared
    let window = NSWindow(
        contentRect: NSRect(x: 0, y: 0, width: 200, height: 100), styleMask: [.titled], backing: .buffered, defer: false
    )
    window.isReleasedWhenClosed = false
    defer { window.close() }
    let field = NSTextField(frame: NSRect(x: 10, y: 10, width: 100, height: 24))
    window.contentView?.addSubview(field)
    #expect(window.makeFirstResponder(field))
    let responder = window.firstResponder

    UsagePointerFocus.clear(in: window, pointerInitiated: false)
    #expect(window.firstResponder === responder)
}
