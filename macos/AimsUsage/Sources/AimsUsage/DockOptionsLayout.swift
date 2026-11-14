import Foundation

enum DockOptionsLayout {
    static func width(in screen: CGRect) -> CGFloat { min(260, max(0, screen.width - 24)) }

    static func frame(in screen: CGRect, dock: CGRect, side: EdgeDockSide, pointerY: CGFloat, height: CGFloat) -> CGRect
    {
        let width = width(in: screen)
        let height = min(max(0, height), max(0, screen.height - 24))
        let preferredX = side == .right ? dock.minX - width - 8 : dock.maxX + 8
        return CGRect(
            x: min(screen.maxX - width - 12, max(screen.minX + 12, preferredX)),
            y: min(screen.maxY - height - 12, max(screen.minY + 12, pointerY - height / 2)),
            width: width, height: height)
    }
}
