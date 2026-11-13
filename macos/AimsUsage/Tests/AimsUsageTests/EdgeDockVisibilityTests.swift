import Foundation
import Testing

@testable import AimsUsage

@Test @MainActor func hiddenDockRevealsOnlyAtItsSelectedEdgeRegion() {
    let screen = CGRect(x: -1512, y: 76, width: 1512, height: 872)
    for side in EdgeDockSide.allCases {
        let dock = EdgeDockLayout.frame(in: screen, providerCount: 6, side: side, size: .mini)
        let activation = EdgeDockLayout.handleFrame(for: dock, side: side)
        let nearEdgeX = side == .left ? screen.minX + 12 : screen.maxX - 12
        #expect(
            EdgeDockVisibility.shouldReveal(
                autoHide: true, expanded: false, pinned: false,
                pointer: CGPoint(x: nearEdgeX, y: dock.midY), dock: dock, peek: nil, activation: activation))
        #expect(
            EdgeDockVisibility.shouldReveal(
                autoHide: true, expanded: false, pinned: false,
                pointer: CGPoint(x: activation.midX, y: activation.midY), dock: dock, peek: nil, activation: activation)
        )
        #expect(
            !EdgeDockVisibility.shouldReveal(
                autoHide: true, expanded: false, pinned: false,
                pointer: CGPoint(x: dock.midX, y: dock.midY), dock: dock, peek: nil, activation: activation))
        #expect(
            !EdgeDockVisibility.shouldReveal(
                autoHide: true, expanded: false, pinned: false,
                pointer: CGPoint(x: activation.midX, y: screen.minY), dock: dock, peek: nil, activation: activation))
    }
}

@Test @MainActor func movingIntoDetailsKeepsDockOpenAndPinningSurvivesPointerExit() {
    let dock = CGRect(x: 1000, y: 200, width: 64, height: 472)
    let peek = CGRect(x: 588, y: 280, width: 400, height: 240)
    let activation = EdgeDockLayout.handleFrame(for: dock, side: .right)
    for point in [CGPoint(x: 1032, y: 300), CGPoint(x: 994, y: 350), CGPoint(x: 700, y: 350)] {
        #expect(
            EdgeDockVisibility.shouldReveal(
                autoHide: true, expanded: true, pinned: false, pointer: point,
                dock: dock, peek: peek, activation: activation))
    }
    let outside = CGPoint(x: 400, y: 100)
    #expect(
        !EdgeDockVisibility.shouldReveal(
            autoHide: true, expanded: true, pinned: false, pointer: outside,
            dock: dock, peek: peek, activation: activation))
    #expect(
        EdgeDockVisibility.shouldReveal(
            autoHide: true, expanded: true, pinned: true, pointer: outside,
            dock: dock, peek: peek, activation: activation))
    #expect(
        EdgeDockVisibility.shouldReveal(
            autoHide: false, expanded: false, pinned: false, pointer: outside,
            dock: dock, peek: nil, activation: activation))
}
