import AppKit
import SwiftUI

@main
struct AimsUsageApp: App {
    @NSApplicationDelegateAdaptor(AimsAppDelegate.self) private var delegate

    var body: some Scene {
        MenuBarExtra {
            UsageDashboard(
                model: delegate.model, togglePanel: delegate.togglePanel, surface: .popover,
                width: delegate.model.displayStyle.menuWidth,
                maximumBodyHeight: maximumBodyHeight()
            )
        } label: {
            HStack(spacing: 4) {
                Image(systemName: "gauge.with.dots.needle.33percent")
                if let summary = delegate.model.menuSummary { Text(summary).monospacedDigit() }
            }
        }
        .menuBarExtraStyle(.window)
        Settings { UsageSettings(model: delegate.model).frame(width: 460, height: 620) }
        #if AIMS_USAGE_PREVIEW
            Window("Menu preview", id: "usage-menu-preview") {
                UsageDashboard(
                    model: delegate.model, togglePanel: delegate.togglePanel, surface: .popover,
                    width: delegate.model.displayStyle.menuWidth,
                    maximumBodyHeight: maximumBodyHeight()
                )
            }
            .windowResizability(.contentSize)
            Window("Small-screen preview", id: "usage-small-preview") {
                UsageDashboard(
                    model: delegate.model, togglePanel: delegate.togglePanel, surface: .popover,
                    width: delegate.model.displayStyle.menuWidth,
                    maximumBodyHeight: 300)
            }
            .windowResizability(.contentSize)
        #endif
    }

    private func maximumBodyHeight() -> CGFloat {
        let screen = NSScreen.screens.first { $0.frame.contains(NSEvent.mouseLocation) } ?? NSScreen.main
        return max(200, (screen?.visibleFrame.height ?? 900) - 108)
    }

}
