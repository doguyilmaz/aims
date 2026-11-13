import Foundation

enum EdgeDockLayout {
    static let peekWidth: CGFloat = 400
    static let gap: CGFloat = 12
    static let handleWidth: CGFloat = 16

    static func normalizedPosition(_ position: Double) -> Double {
        position.isFinite ? min(1, max(0, position)) : 0.5
    }

    static func frame(
        in visibleFrame: CGRect, providerCount: Int, side: EdgeDockSide = .right, size: EdgeDockSize = .standard,
        position: Double = 0.5
    ) -> CGRect {
        let desired =
            providerCount > 0
            ? CGFloat(providerCount) * size.rowStride + size.chromeHeight : size.contentInset * 2 + 78
        let height = min(max(0, visibleFrame.height - 16), desired)
        let dockWidth = min(size.width, max(0, visibleFrame.width))
        let inset = min(8, max(0, visibleFrame.height / 2))
        let travel = max(0, visibleFrame.height - inset * 2 - height)
        return CGRect(
            x: side == .right ? visibleFrame.maxX - dockWidth : visibleFrame.minX,
            y: visibleFrame.maxY - inset - height - travel * normalizedPosition(position),
            width: dockWidth, height: height)
    }

    static func handleFrame(for dock: CGRect, side: EdgeDockSide) -> CGRect {
        let width = min(handleWidth, max(0, dock.width))
        return CGRect(
            x: side == .right ? dock.maxX - width : dock.minX, y: dock.minY,
            width: width, height: dock.height)
    }
}
