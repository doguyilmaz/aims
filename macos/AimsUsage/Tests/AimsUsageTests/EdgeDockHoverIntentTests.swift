import Foundation
import Testing

@testable import AimsUsage

@Test @MainActor func aRevealedDockIgnoresStationaryPointersAndContinuedApproachToTheScreenEdge() {
    var intent = EdgeDockHoverIntent.awaitingEntry(anchorX: -40, side: .right)
    // Entering the marker, continuing toward the edge and vertical drift must not open a card.
    for point in [CGPoint(x: -40, y: 350), CGPoint(x: -20, y: 350), CGPoint(x: -20, y: 380), CGPoint(x: -22, y: 380)] {
        let accepted = intent.acceptsHover(at: point)
        #expect(!accepted)
    }
    let entered = intent.acceptsHover(at: CGPoint(x: -24, y: 380))
    #expect(entered)
    // Once entered intentionally, moving between providers has no further movement threshold.
    let continued = intent.acceptsHover(at: CGPoint(x: -24, y: 381))
    #expect(continued)
}

@Test @MainActor func intentionalEntryMirrorsWithTheEdgeAndWorksWithinTheSameProvider() {
    for side in EdgeDockSide.allCases {
        let origin: CGFloat = side == .right ? 3420 : -1900
        let direction: CGFloat = side == .right ? -1 : 1
        var intent = EdgeDockHoverIntent.awaitingEntry(anchorX: origin, side: side)
        let stationary = intent.acceptsHover(at: CGPoint(x: origin, y: -300))
        let jitter = intent.acceptsHover(at: CGPoint(x: origin + 3 * direction, y: -300))
        #expect(!stationary && !jitter)
        let entered = intent.acceptsHover(at: CGPoint(x: origin + 4 * direction, y: -300))
        #expect(entered)
    }
}

@Test @MainActor func normalHoverAndExplicitClicksNeedNoRevealMovement() {
    var intent = EdgeDockHoverIntent.ready
    let point = CGPoint(x: 100, y: 200)
    let first = intent.acceptsHover(at: point)
    let repeated = intent.acceptsHover(at: point)
    #expect(first && repeated)
    intent = .awaitingEntry(anchorX: point.x, side: .right)
    let stationary = intent.acceptsHover(at: point)
    #expect(!stationary)
    intent = .ready  // always-visible/pinned Dock or an explicit provider click
    let explicit = intent.acceptsHover(at: point)
    #expect(explicit)
    intent = .awaitingEntry(anchorX: point.x, side: .right)  // a later reveal starts its own entry
    let revealedAgain = intent.acceptsHover(at: point)
    #expect(!revealedAgain)
}
