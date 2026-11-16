import AppKit
import SwiftUI
import Testing

@testable import AimsUsage

@Test @MainActor func nativePanelLayersStayClippedOnFirstLayoutAndResize() throws {
    _ = NSApplication.shared
    let panel = UsagePanel(
        contentRect: NSRect(x: 0, y: 0, width: 300, height: 196),
        styleMask: [.borderless, .nonactivatingPanel], backing: .buffered, defer: false)
    panel.isReleasedWhenClosed = false
    panel.isOpaque = false
    panel.backgroundColor = .clear
    defer { panel.close() }
    let host = UsageRoundedHostingView(rootView: AnyView(Color.clear), cornerRadius: 14)
    panel.contentView = host

    for (size, radius) in [(NSSize(width: 300, height: 196), CGFloat(14)), (NSSize(width: 360, height: 310), 18)] {
        host.cornerRadius = radius
        host.rootView = AnyView(Color.clear)
        panel.setContentSize(size)
        host.layoutSubtreeIfNeeded()
        let layer = try #require(host.layer)
        // Native descendants, including material layers, must obey the host's outline.
        let fill = CALayer()
        fill.frame = host.bounds.insetBy(dx: -20, dy: -20)
        fill.backgroundColor = NSColor.red.cgColor
        layer.addSublayer(fill)
        defer { fill.removeFromSuperlayer() }

        let bitmap = try #require(
            NSBitmapImageRep(
                bitmapDataPlanes: nil, pixelsWide: Int(size.width), pixelsHigh: Int(size.height),
                bitsPerSample: 8, samplesPerPixel: 4, hasAlpha: true, isPlanar: false,
                colorSpaceName: .deviceRGB, bytesPerRow: 0, bitsPerPixel: 0))
        let context = try #require(NSGraphicsContext(bitmapImageRep: bitmap)?.cgContext)
        layer.render(in: context)
        for x in [1, Int(size.width) - 2] {
            for y in [1, Int(size.height) - 2] {
                let color = try #require(bitmap.colorAt(x: x, y: y))
                #expect(color.alphaComponent < 0.01)
            }
        }
        let center = try #require(bitmap.colorAt(x: Int(size.width / 2), y: Int(size.height / 2)))
        #expect(center.alphaComponent > 0.99)
    }
}
