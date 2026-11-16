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

@Test @MainActor func dockPositionMovesBetweenUsableTopAndBottomOnEitherSide() {
    let screen = CGRect(x: -1512, y: -300, width: 1512, height: 872)
    for side in EdgeDockSide.allCases {
        for size in EdgeDockSize.allCases {
            let top = EdgeDockLayout.frame(in: screen, providerCount: 6, side: side, size: size, position: 0)
            let center = EdgeDockLayout.frame(in: screen, providerCount: 6, side: side, size: size, position: 0.5)
            let bottom = EdgeDockLayout.frame(in: screen, providerCount: 6, side: side, size: size, position: 1)
            #expect(top.maxY == screen.maxY - 8)
            #expect(center.midY == screen.midY)
            #expect(bottom.minY == screen.minY + 8)
            #expect(top.minY > center.minY && center.minY > bottom.minY)
            #expect(top.size == bottom.size && top.minX == bottom.minX)
            for position in [0.0, 0.25, 0.5, 0.75, 1.0] {
                let frame = EdgeDockLayout.frame(
                    in: screen, providerCount: 20, side: side, size: size, position: position)
                #expect(frame.minY == screen.minY + 8 && frame.maxY == screen.maxY - 8)
            }
            #expect(EdgeDockLayout.frame(in: screen, providerCount: 6, side: side, size: size, position: -1) == top)
            #expect(EdgeDockLayout.frame(in: screen, providerCount: 6, side: side, size: size, position: 2) == bottom)
            #expect(
                EdgeDockLayout.frame(in: screen, providerCount: 6, side: side, size: size, position: .nan) == center)
        }
    }
}

@Test @MainActor func providerDetailsFollowDockSizeAndRemainInsideTheUsableScreen() {
    for screen in [
        CGRect(x: 0, y: 76, width: 3440, height: 1333),
        CGRect(x: -1512, y: -300, width: 1512, height: 872),
        CGRect(x: 1512, y: 40, width: 1024, height: 700),
        CGRect(x: -280, y: -180, width: 280, height: 180),
    ] {
        for side in EdgeDockSide.allCases {
            for size in EdgeDockSize.allCases {
                for position in [0.0, 0.5, 1.0] {
                    let dock = EdgeDockLayout.frame(
                        in: screen, providerCount: 6, side: side, size: size, position: position)
                    for index in [0, 5] {
                        for height: CGFloat in [120, 5000] {
                            let peek = EdgeDockLayout.peekFrame(
                                in: screen, dock: dock, side: side, size: size,
                                providerIndex: index, height: height)
                            #expect(screen.insetBy(dx: 12, dy: 12).contains(peek))
                            #expect(peek.width <= size.peekWidth)
                            #expect(peek.height <= screen.height - 24)
                            if screen.width >= size.peekWidth + dock.width + size.peekGap + 12 {
                                #expect(side == .right ? peek.maxX < dock.minX : peek.minX > dock.maxX)
                            }
                        }
                    }
                }
                let maximumBody = EdgeDockLayout.peekMaximumBodyHeight(in: screen, size: size)
                #expect(maximumBody >= 0)
                #expect(maximumBody + size.peekChromeHeight <= screen.height - 24)
            }
        }
        if screen.width > EdgeDockSize.standard.peekWidth + 24 {
            #expect(
                EdgeDockLayout.peekWidth(in: screen, size: .mini)
                    < EdgeDockLayout.peekWidth(in: screen, size: .standard))
        }
    }
}
