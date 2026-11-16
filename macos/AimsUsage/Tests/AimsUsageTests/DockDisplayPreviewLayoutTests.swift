import Foundation
import Testing

@testable import AimsUsage

@Test @MainActor func monitorPreviewsIgnoreGlobalOriginAndReflectTheChosenEdgeSizeAndVerticalPosition() {
    let bounds = CGRect(x: 8, y: 7, width: 200, height: 64)
    for dimensions in [CGSize(width: 1920, height: 1080), CGSize(width: 1080, height: 1920)] {
        for origin in [CGPoint.zero, CGPoint(x: 1512, y: 982), CGPoint(x: -2560, y: -1080)] {
            let frame = CGRect(origin: origin, size: dimensions)
            let display = DockDisplay(id: "test", name: "Monitor", frame: frame, visibleFrame: frame)
            for side in EdgeDockSide.allCases {
                let top = DockDisplayPreviewLayout(
                    display: display, bounds: bounds, side: side, size: .mini, position: 0, providerCount: 6)
                let bottom = DockDisplayPreviewLayout(
                    display: display, bounds: bounds, side: side, size: .mini, position: 1, providerCount: 6)
                let standard = DockDisplayPreviewLayout(
                    display: display, bounds: bounds, side: side, size: .standard, position: 0, providerCount: 6)
                #expect(bounds.contains(top.screen))
                #expect(top.screen.insetBy(dx: -0.001, dy: -0.001).contains(top.dock))
                #expect(top.dock.minY < bottom.dock.minY)
                #expect(top.dock.width < standard.dock.width)
                #expect(
                    abs(
                        (side == .right ? top.dock.maxX : top.dock.minX)
                            - (side == .right ? top.screen.maxX : top.screen.minX)) < 0.001)
                let zeroOrigin = DockDisplay(
                    id: "test", name: "Monitor", frame: CGRect(origin: .zero, size: dimensions),
                    visibleFrame: CGRect(origin: .zero, size: dimensions))
                let reference = DockDisplayPreviewLayout(
                    display: zeroOrigin, bounds: bounds, side: side, size: .mini, position: 0, providerCount: 6)
                #expect(abs(top.dock.minX - reference.dock.minX) < 0.001)
                #expect(abs(top.dock.minY - reference.dock.minY) < 0.001)
            }
        }
    }
}

@Test @MainActor func monitorPreviewKeepsTheSystemDockInsetAndHandlesMissingGeometry() {
    let display = DockDisplay(
        id: "test", name: "Monitor", frame: CGRect(x: -1920, y: -1080, width: 1920, height: 1080),
        visibleFrame: CGRect(x: -1840, y: -1000, width: 1840, height: 968))
    let preview = DockDisplayPreviewLayout(
        display: display, bounds: CGRect(x: 0, y: 0, width: 192, height: 108),
        side: .left, size: .standard, position: 1, providerCount: 6)
    #expect(abs(preview.dock.minX - 8) < 0.001)
    #expect(preview.dock.maxY < preview.screen.maxY)
    let empty = DockDisplayPreviewLayout(
        display: display, bounds: .zero, side: .right, size: .mini, position: 0.5, providerCount: 6)
    #expect(empty.screen == .zero && empty.dock == .zero)
}
