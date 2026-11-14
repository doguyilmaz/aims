import Foundation
import Testing

@testable import AimsUsage

@Test @MainActor func overviewKeepsItsUpperRightAnchorAcrossStylesAndScreens() {
    for screen in [
        CGRect(x: -1920, y: -200, width: 1840, height: 1056),
        CGRect(x: 1512, y: 76, width: 1440, height: 800),
    ] {
        for width in UsageDisplayStyle.allCases.map(\.preferredWidth) {
            let frame = EdgePanelLayout.frame(in: screen, width: width, height: 600)
            #expect(frame.maxX == screen.maxX - 76)
            #expect(frame.maxY == screen.maxY - 12)
            #expect(frame.width == width && frame.height == 600)
            let dock = EdgeDockLayout.frame(in: screen, providerCount: 6, position: 0)
            #expect(frame.maxX <= dock.minX - 12)
        }
        let overflow = EdgePanelLayout.frame(in: screen, width: 2000, height: 2000)
        #expect(overflow.minX == screen.minX + 12 && overflow.maxX == screen.maxX - 76)
        #expect(overflow.minY == screen.minY + 12 && overflow.maxY == screen.maxY - 12)
    }
}
