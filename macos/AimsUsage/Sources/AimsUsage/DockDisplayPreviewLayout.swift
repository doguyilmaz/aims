import Foundation

/// Projects global, bottom-up display coordinates into a top-down thumbnail.
struct DockDisplayPreviewLayout {
    let screen: CGRect
    let dock: CGRect

    init(
        display: DockDisplay, bounds: CGRect, side: EdgeDockSide, size: EdgeDockSize,
        position: Double, providerCount: Int
    ) {
        guard display.frame.width > 0, display.frame.height > 0, bounds.width > 0, bounds.height > 0 else {
            screen = .zero
            dock = .zero
            return
        }
        let scale = min(bounds.width / display.frame.width, bounds.height / display.frame.height)
        screen = CGRect(
            x: bounds.midX - display.frame.width * scale / 2,
            y: bounds.midY - display.frame.height * scale / 2,
            width: display.frame.width * scale, height: display.frame.height * scale)
        let rail = EdgeDockLayout.frame(
            in: display.visibleFrame, providerCount: providerCount, side: side, size: size, position: position)
        dock = CGRect(
            x: screen.minX + (rail.minX - display.frame.minX) * scale,
            y: screen.maxY - (rail.maxY - display.frame.minY) * scale,
            width: rail.width * scale, height: rail.height * scale)
    }
}
