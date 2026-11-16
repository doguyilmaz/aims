import Foundation
import Testing

@testable import AimsUsage

@Test @MainActor func dockDisplaySelectionRoundTripsAndDefaultsToThePrimaryDisplay() {
    for selection in [DockDisplaySelection.primary, .display("external-uuid"), .display("other-uuid")] {
        #expect(DockDisplaySelection(savedValue: selection.id) == selection)
    }
    for saved in [nil, "", "display:", "unknown"] {
        #expect(DockDisplaySelection(savedValue: saved) == .primary)
    }
}

@Test @MainActor func selectedDisplaySurvivesReorderingAndDuplicateMonitorNames() {
    let primary = DockDisplay(
        id: "built-in", name: "Retina", frame: CGRect(x: 0, y: 0, width: 1512, height: 982),
        visibleFrame: CGRect(x: 0, y: 73, width: 1512, height: 875))
    let external = DockDisplay(
        id: "external", name: "Monitor", frame: CGRect(x: 1512, y: 0, width: 1920, height: 1080),
        visibleFrame: CGRect(x: 1512, y: 0, width: 1920, height: 1080))
    let third = DockDisplay(
        id: "third", name: "Monitor", frame: CGRect(x: -2560, y: 982, width: 2560, height: 1440),
        visibleFrame: CGRect(x: -2560, y: 982, width: 2560, height: 1400))
    #expect(DockDisplaySelection.primary.resolve(in: [primary, external, third]) == primary)
    #expect(DockDisplaySelection.primary.resolve(in: [third, external, primary]) == third)
    for displays in [[primary, external, third], [third, primary, external], [external, third, primary]] {
        #expect(DockDisplaySelection.display("external").resolve(in: displays) == external)
        #expect(DockDisplaySelection.display("third").resolve(in: displays) == third)
    }
}

@Test @MainActor func disconnectedDisplayFallsBackWithoutForgettingTheSelectionAndReturnsOnReconnect() {
    let primary = DockDisplay(
        id: "built-in", name: "Retina", frame: CGRect(x: 0, y: 0, width: 1512, height: 982),
        visibleFrame: CGRect(x: 0, y: 73, width: 1512, height: 875))
    let external = DockDisplay(
        id: "external", name: "Monitor", frame: CGRect(x: 1512, y: 0, width: 1920, height: 1080),
        visibleFrame: CGRect(x: 1512, y: 0, width: 1920, height: 1080))
    let selection = DockDisplaySelection.display("external")
    #expect(selection.isConnected(in: [primary, external]))
    #expect(selection.resolve(in: [primary, external]) == external)
    #expect(!selection.isConnected(in: [primary]))
    #expect(selection.resolve(in: [primary]) == primary)
    #expect(selection.resolve(in: []) == nil)
    #expect(!DockDisplaySelection.primary.isConnected(in: []))
    #expect(selection.resolve(in: [primary, external]) == external)
    #expect(selection.id == "display:external")
}

@Test @MainActor func reconnectedAndRearrangedDisplaysAlwaysUseTheirFreshGlobalCoordinates() throws {
    let selection = DockDisplaySelection.display("external")
    // A display can become primary while disconnected from the laptop, then be
    // placed anywhere in the desktop again. Never reuse its old zero origin.
    let old = DockDisplay(
        id: "external", name: "Monitor", frame: CGRect(x: 0, y: 0, width: 1920, height: 1080),
        visibleFrame: CGRect(x: 0, y: 0, width: 1920, height: 1080))
    for origin in [CGPoint(x: 1512, y: 0), CGPoint(x: -1920, y: -300), CGPoint(x: 0, y: 982)] {
        let frame = CGRect(origin: origin, size: old.frame.size)
        let fresh = DockDisplay(id: old.id, name: old.name, frame: frame, visibleFrame: frame)
        let selected = try #require(selection.resolve(in: [fresh]))
        #expect(selected.visibleFrame == frame)
        for side in EdgeDockSide.allCases {
            for size in EdgeDockSize.allCases {
                for position in [0.0, 0.5, 1.0] {
                    let dock = EdgeDockLayout.frame(
                        in: selected.visibleFrame, providerCount: 6, side: side, size: size, position: position)
                    #expect(frame.contains(dock))
                    #expect(side == .right ? dock.maxX == frame.maxX : dock.minX == frame.minX)
                    let peek = EdgeDockLayout.peekFrame(
                        in: selected.visibleFrame, dock: dock, side: side, size: size, providerIndex: 5, height: 240)
                    #expect(frame.contains(peek))
                    let options = DockOptionsLayout.frame(
                        in: selected.visibleFrame, dock: dock, side: side, pointerY: dock.midY, height: 400)
                    #expect(frame.contains(options))
                }
            }
        }
    }
}
