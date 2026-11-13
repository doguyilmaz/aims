import Foundation
import Testing

@testable import AimsUsage

@Test @MainActor func edgeDockFitsTheUsableEdgeAcrossScreenOriginsAndProviderCounts() {
    let screens = [
        CGRect(x: 0, y: 76, width: 1512, height: 872),
        CGRect(x: -1920, y: 0, width: 1840, height: 1056),
        CGRect(x: 1512, y: -200, width: 1440, height: 800),
    ]
    for screen in screens {
        for side in EdgeDockSide.allCases {
            for size in EdgeDockSize.allCases {
                for count in [0, 6, 20] {
                    let frame = EdgeDockLayout.frame(in: screen, providerCount: count, side: side, size: size)
                    #expect(side == .right ? frame.maxX == screen.maxX : frame.minX == screen.minX)
                    #expect(frame.minX >= screen.minX && frame.maxX <= screen.maxX)
                    #expect(frame.minY >= screen.minY + 8)
                    #expect(frame.maxY <= screen.maxY - 8)
                    #expect(frame.midY == screen.midY)
                    let handle = EdgeDockLayout.handleFrame(for: frame, side: side)
                    #expect(frame.contains(handle))
                    #expect(side == .right ? handle.maxX == screen.maxX : handle.minX == screen.minX)
                }
            }
        }
    }
}
