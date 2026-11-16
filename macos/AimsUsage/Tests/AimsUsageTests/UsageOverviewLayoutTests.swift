import Foundation
import Testing

@testable import AimsUsage

@Test @MainActor func nativeOverviewContentFitsDifferentScreensWithoutChoosingItsOwnWindowPosition() {
    for screen in [
        CGRect(x: 0, y: 24, width: 1440, height: 850),
        CGRect(x: -1920, y: -200, width: 1920, height: 1080),
        CGRect(x: 200, y: 1200, width: 300, height: 480),
    ] {
        for style in UsageDisplayStyle.allCases {
            let width = UsageOverviewLayout.width(in: screen, style: style)
            let body = UsageOverviewLayout.maximumBodyHeight(in: screen, style: style)
            #expect(width > 0 && width <= style.preferredWidth)
            #expect(width <= screen.width - 24)
            #expect(body >= 0 && body + style.chromeHeight <= screen.height - 24)
        }
    }
}

@Test @MainActor func dockOverviewAnchorFollowsTheFooterInFlippedAndUnflippedViews() {
    for size in EdgeDockSize.allCases {
        let bounds = CGRect(x: 10, y: 20, width: size.width, height: 400)
        let bottomUp = EdgeDockLayout.overviewAnchor(in: bounds, flipped: false, size: size)
        let topDown = EdgeDockLayout.overviewAnchor(in: bounds, flipped: true, size: size)
        #expect(bottomUp.minY == bounds.minY + size.contentInset)
        #expect(topDown.maxY == bounds.maxY - size.contentInset)
        #expect(bottomUp.minX == topDown.minX && bottomUp.width == topDown.width)
        #expect(bounds.contains(bottomUp) && bounds.contains(topDown))
        let tiny = CGRect(x: 0, y: 0, width: 10, height: 10)
        #expect(tiny.contains(EdgeDockLayout.overviewAnchor(in: tiny, flipped: true, size: size)))
    }
}
