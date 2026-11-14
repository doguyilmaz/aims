import Foundation
import Testing

@testable import AimsUsage

@Test @MainActor func dockOptionsHaveTheirOwnWidthAndStayInsideTheClickedScreen() {
    for screen in [
        CGRect(x: 0, y: 76, width: 3440, height: 1333),
        CGRect(x: -1512, y: -300, width: 1512, height: 872),
        CGRect(x: 1512, y: 40, width: 1024, height: 700),
    ] {
        for side in EdgeDockSide.allCases {
            for size in EdgeDockSize.allCases {
                let dock = EdgeDockLayout.frame(in: screen, providerCount: 6, side: side, size: size)
                for y in [screen.minY, dock.midY, screen.maxY] {
                    let panel = DockOptionsLayout.frame(in: screen, dock: dock, side: side, pointerY: y, height: 320)
                    #expect(panel.width == 260 && panel.height == 320)
                    #expect(screen.contains(panel))
                    #expect(side == .right ? panel.maxX < dock.minX : panel.minX > dock.maxX)
                }
                let tall = DockOptionsLayout.frame(
                    in: screen, dock: dock, side: side, pointerY: dock.midY, height: 5000)
                #expect(tall.minY == screen.minY + 12 && tall.maxY == screen.maxY - 12)
                #expect(tall.width == 260)
            }
        }
    }
}
