import Foundation

enum EdgeDockLayout {
    static let width: CGFloat = 64
    static let rowStride: CGFloat = 64
    static let contentInset: CGFloat = 32
    static let chromeHeight: CGFloat = 88
    static let peekWidth: CGFloat = 400
    static let gap: CGFloat = 12

    static func frame(in visibleFrame: CGRect, providerCount: Int) -> CGRect {
        let desired = providerCount > 0 ? CGFloat(providerCount) * rowStride + chromeHeight : 142
        let height = min(max(0, visibleFrame.height - 16), desired)
        let dockWidth = min(width, max(0, visibleFrame.width))
        return CGRect(
            x: visibleFrame.maxX - dockWidth, y: visibleFrame.midY - height / 2, width: dockWidth, height: height)
    }
}
