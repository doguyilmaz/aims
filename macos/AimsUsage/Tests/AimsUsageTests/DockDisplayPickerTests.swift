import AppKit
import SwiftUI
import Testing

@testable import AimsUsage

@Test @MainActor func twoThreeAndFourMonitorsFitInOneCompactSelectorRow() {
    for count in 2...4 {
        let model = UsageModel(configHomeOverride: "")
        model.dockDisplays = (0..<count).map { index in
            let frame = CGRect(x: CGFloat(index) * 1920, y: 0, width: 1920, height: 1080)
            return DockDisplay(id: "display-\(index)", name: "Monitor \(index + 1)", frame: frame, visibleFrame: frame)
        }
        let host = NSHostingView(rootView: DockDisplayPicker(model: model).frame(width: 560))
        let fitting = host.fittingSize
        #expect(fitting.height > 0)
        #expect(fitting.height <= 140)
        #expect(fitting.width == 560)
    }
}
