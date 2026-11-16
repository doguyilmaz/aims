import Foundation

/// A newly revealed rail must not select the provider that appeared beneath a stationary pointer.
enum EdgeDockHoverIntent {
    case ready
    case awaitingEntry(anchorX: CGFloat, side: EdgeDockSide)

    mutating func acceptsHover(at point: CGPoint) -> Bool {
        guard case .awaitingEntry(let anchorX, let side) = self else { return true }
        let inward = side == .right ? anchorX - point.x : point.x - anchorX
        // Track an approach toward the screen edge, then require a short movement back into the rail.
        guard inward >= 4 else {
            let nearestEdge = side == .right ? max(anchorX, point.x) : min(anchorX, point.x)
            self = .awaitingEntry(anchorX: nearestEdge, side: side)
            return false
        }
        self = .ready
        return true
    }
}
