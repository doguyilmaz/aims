import Foundation
import Testing

@testable import AimsUsage

@Test @MainActor func everyHiddenMarkerHasAUsableActivationAreaOnEitherEdge() {
    for screen in [
        CGRect(x: -1512, y: -180, width: 1512, height: 872), CGRect(x: 1512, y: 76, width: 1920, height: 1080),
    ] {
        for side in EdgeDockSide.allCases {
            for size in EdgeDockSize.allCases {
                let dock = EdgeDockLayout.frame(in: screen, providerCount: 6, side: side, size: size, position: 0.7)
                for style in EdgeDockMarkerStyle.allCases {
                    let handle = EdgeDockLayout.handleFrame(for: dock, side: side, marker: style, size: size)
                    #expect(dock.contains(handle))
                    #expect(handle.width >= EdgeDockLayout.handleWidth)
                    #expect(handle.width >= style.visualSize(for: size).width)
                    #expect(handle.height == dock.height && handle.midY == dock.midY)
                    #expect(side == .right ? handle.maxX == screen.maxX : handle.minX == screen.minX)
                    #expect(
                        EdgeDockVisibility.shouldReveal(
                            autoHide: true, expanded: false, pinned: false,
                            pointer: CGPoint(x: handle.midX, y: handle.midY),
                            dock: dock, peek: nil, activation: handle))
                    #expect(
                        !EdgeDockVisibility.shouldReveal(
                            autoHide: true, expanded: false, pinned: false,
                            pointer: CGPoint(x: screen.midX, y: handle.midY),
                            dock: dock, peek: nil, activation: handle))
                }
            }
        }
    }
}

@Test @MainActor func hiddenMarkersClampToSmallScreensAndScaleWithMiniDock() {
    for style in EdgeDockMarkerStyle.allCases {
        let mini = style.visualSize(for: .mini)
        let standard = style.visualSize(for: .standard)
        #expect(mini.width <= standard.width && mini.height < standard.height)
        for width: CGFloat in [0, 8, 16, 24] {
            for side in EdgeDockSide.allCases {
                let dock = CGRect(x: -200, y: 80, width: width, height: 32)
                let handle = EdgeDockLayout.handleFrame(for: dock, side: side, marker: style, size: .mini)
                #expect(handle.width >= 0 && handle.width <= dock.width)
                #expect(handle.minX >= dock.minX && handle.maxX <= dock.maxX)
            }
        }
    }
}

@Test @MainActor func markerPreferenceSurvivesRelaunchAndReconfiguresTheDock() {
    let defaults = UserDefaults.standard
    let key = "edgeDockMarkerStyle"
    let saved = defaults.object(forKey: key)
    defer {
        if let saved { defaults.set(saved, forKey: key) } else { defaults.removeObject(forKey: key) }
    }
    defaults.removeObject(forKey: key)
    let model = UsageModel(configHomeOverride: "")
    #expect(model.edgeDockMarkerStyle == .line)
    var changes = 0
    model.dockConfigurationDidChange = { changes += 1 }
    for style in [EdgeDockMarkerStyle.aims, .logo, .pill, .line] {
        model.edgeDockMarkerStyle = style
        #expect(UsageModel(configHomeOverride: "").edgeDockMarkerStyle == style)
    }
    #expect(changes == 4)
    #expect(EdgeDockMarkerStyle(savedValue: "future-style") == .line)
}
