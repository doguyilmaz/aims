import Foundation

enum EdgeDockLayout {
    static let handleWidth: CGFloat = 16

    static func peekWidth(in visibleFrame: CGRect, size: EdgeDockSize) -> CGFloat {
        min(size.peekWidth, max(0, visibleFrame.width - 24))
    }

    static func peekMaximumBodyHeight(in visibleFrame: CGRect, size: EdgeDockSize) -> CGFloat {
        max(0, visibleFrame.height - size.peekChromeHeight - 24)
    }

    static func peekFrame(
        in visibleFrame: CGRect, dock: CGRect, side: EdgeDockSide, size: EdgeDockSize,
        providerIndex: Int, height: CGFloat
    ) -> CGRect {
        let width = peekWidth(in: visibleFrame, size: size)
        let height = min(max(0, height), max(0, visibleFrame.height - 24))
        let anchorY = dock.maxY - size.contentInset - (size.rowStride - 4) / 2 - CGFloat(providerIndex) * size.rowStride
        let preferredX = side == .right ? dock.minX - width - size.peekGap : dock.maxX + size.peekGap
        return CGRect(
            x: min(visibleFrame.maxX - width - 12, max(visibleFrame.minX + 12, preferredX)),
            y: min(visibleFrame.maxY - height - 12, max(visibleFrame.minY + 12, anchorY - height / 2)),
            width: width, height: height)
    }

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
