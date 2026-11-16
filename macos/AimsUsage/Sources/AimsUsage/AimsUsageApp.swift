import AppKit
import SwiftUI

@main
struct AimsUsageApp: App {
    @NSApplicationDelegateAdaptor(AimsAppDelegate.self) private var delegate

    var body: some Scene {
        Settings {
            UsageSettings(model: delegate.model)
                .frame(
                    minWidth: SettingsWindowMetrics.minimumContentSize.width,
                    minHeight: SettingsWindowMetrics.minimumContentSize.height)
        }
        .commands {
            CommandGroup(after: .appSettings) {
                Button("Show usage") { delegate.showOverview(from: .menuBar) }
                    .keyboardShortcut("u", modifiers: [.command, .shift])
            }
        }
    }
}
