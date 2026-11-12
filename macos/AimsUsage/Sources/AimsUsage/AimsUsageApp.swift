import AppKit
import SwiftUI

@main
struct AimsUsageApp: App {
    @NSApplicationDelegateAdaptor(AimsAppDelegate.self) private var delegate

    var body: some Scene {
        MenuBarExtra {
            UsageDashboard(model: delegate.model, togglePanel: delegate.togglePanel)
                .frame(width: 390, height: 650)
        } label: {
            Image(systemName: "gauge.with.dots.needle.33percent")
            if let summary = delegate.model.menuSummary { Text(summary).monospacedDigit() }
        }
        .menuBarExtraStyle(.window)
        Settings { UsageSettings(model: delegate.model).frame(width: 460) }
    }

}
