import AppKit
import SwiftUI

@main
struct AimsUsageApp: App {
    @NSApplicationDelegateAdaptor(AimsAppDelegate.self) private var delegate

    var body: some Scene {
        MenuBarExtra {
            UsageDashboard(model: delegate.model, togglePanel: delegate.togglePanel, surface: .popover)
                .frame(width: 420, height: 690)
        } label: {
            HStack(spacing: 4) {
                Image(systemName: "gauge.with.dots.needle.33percent")
                if let summary = delegate.model.menuSummary { Text(summary).monospacedDigit() }
            }
        }
        .menuBarExtraStyle(.window)
        Settings { UsageSettings(model: delegate.model).frame(width: 460) }
        #if AIMS_USAGE_PREVIEW
            Window("Menu preview", id: "usage-menu-preview") {
                UsageDashboard(model: delegate.model, togglePanel: delegate.togglePanel, surface: .popover)
                    .frame(width: 420, height: 690)
            }
            .windowResizability(.contentSize)
        #endif
    }

}
