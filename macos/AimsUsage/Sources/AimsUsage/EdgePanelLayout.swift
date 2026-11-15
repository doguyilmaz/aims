import Foundation

enum EdgePanelLayout {
    static func frame(in visibleFrame: CGRect, width: CGFloat, height: CGFloat) -> CGRect {
        let leadingInset = min(12, max(0, visibleFrame.width / 2))
        // Reserve the widest rail plus a gap, regardless of Dock preferences.
        let trailingInset = min(76, max(0, visibleFrame.width - leadingInset))
        let verticalInset = min(12, max(0, visibleFrame.height / 2))
        let width = min(max(0, width), max(0, visibleFrame.width - leadingInset - trailingInset))
        let height = min(max(0, height), max(0, visibleFrame.height - verticalInset * 2))
        return CGRect(
            x: visibleFrame.maxX - trailingInset - width,
            y: visibleFrame.maxY - verticalInset - height, width: width, height: height)
    }
}
